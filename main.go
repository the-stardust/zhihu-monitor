package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
	"zhihu-monitor/utils"
)

type Config struct {
	ZhihuUserIDs    []string `yaml:"zhihu_user_ids"`
	FeishuWebhook   string   `yaml:"feishu_webhook_url"`
	MonitorInterval int      `yaml:"monitor_interval"`
	StorageFile     string   `yaml:"storage_file"`
	UserAgent       string   `yaml:"user_agent"`
	ZhihuCookie     string   `yaml:"zhihu_cookie"`
}

const configFile = "config.yaml"

var (
	// stateMu 保护 config / storage / userInfos / lastConfigData 这组共享状态，
	// 允许检查任务与热加载任务并发安全地访问。
	stateMu        sync.RWMutex
	config         Config
	storage        *utils.Storage
	userInfos      map[string]*utils.UserInfo
	lastConfigData []byte

	cronScheduler *cron.Cron
	checkEntryID  cron.EntryID
)

// readConfig 读取并解析 config.yaml，同时返回原始字节用于变更比对。
func readConfig() (Config, []byte, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return Config{}, nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, nil, err
	}
	return c, data, nil
}

// buildUserInfos 校验配置并拉取待监控博主信息。校验/拉取失败返回 ok=false。
func buildUserInfos(cfg Config) (map[string]*utils.UserInfo, bool) {
	if len(cfg.ZhihuUserIDs) == 0 {
		log.Println("请先在 config.yaml 中配置知乎用户ID列表 (zhihu_user_ids)")
		return nil, false
	}
	if cfg.FeishuWebhook == "" {
		log.Println("请先在 config.yaml 中配置飞书Webhook地址")
		return nil, false
	}

	infos := make(map[string]*utils.UserInfo)
	for _, userID := range cfg.ZhihuUserIDs {
		info, err := utils.FetchUserInfo(userID, cfg.UserAgent)
		if err != nil {
			log.Printf("无法获取用户信息(%s): %v，已跳过该博主\n", userID, err)
			continue
		}
		infos[userID] = info
	}

	if len(infos) == 0 {
		log.Println("没有可监控的博主，请检查配置和网络")
		return nil, false
	}

	log.Println("监控目标:")
	for _, userID := range cfg.ZhihuUserIDs {
		if info, ok := infos[userID]; ok {
			log.Printf("  - %s (%s)\n", info.Name, info.Headline)
		}
	}
	log.Printf("监控频率: 每 %d 秒\n", cfg.MonitorInterval)
	log.Println(strings.Repeat("=", 50))
	return infos, true
}

// initMonitor 首次启动时构建全部共享状态。
func initMonitor(cfg Config, rawData []byte) bool {
	infos, ok := buildUserInfos(cfg)
	if !ok {
		return false
	}

	stateMu.Lock()
	config = cfg
	userInfos = infos
	storage = utils.NewStorage(cfg.StorageFile)
	lastConfigData = rawData
	stateMu.Unlock()
	return true
}

// reloadConfigIfChanged 比对 config.yaml 是否有变动，有则热加载新配置。
// 解析失败或校验不通过时保留旧配置继续运行。
func reloadConfigIfChanged() {
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("热加载: 读取配置文件失败: %v\n", err)
		return
	}

	stateMu.RLock()
	unchanged := bytes.Equal(data, lastConfigData)
	oldCfg := config
	oldStorage := storage
	stateMu.RUnlock()
	if unchanged {
		return
	}

	log.Println("检测到 config.yaml 发生变动，开始热加载...")

	var newCfg Config
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		log.Printf("热加载: 配置解析失败，继续沿用旧配置: %v\n", err)
		return
	}

	infos, ok := buildUserInfos(newCfg)
	if !ok {
		log.Println("热加载: 新配置校验未通过，继续沿用旧配置")
		return
	}

	// storage_file 未变则复用旧实例，避免丢失内存中已记录的 ID 而重复推送。
	newStorage := oldStorage
	if newCfg.StorageFile != oldCfg.StorageFile {
		newStorage = utils.NewStorage(newCfg.StorageFile)
	}

	stateMu.Lock()
	config = newCfg
	userInfos = infos
	storage = newStorage
	lastConfigData = data
	stateMu.Unlock()

	log.Println("热加载完成，已应用新配置")

	// 监控频率变化时，重新调度检查任务。
	if newCfg.MonitorInterval != oldCfg.MonitorInterval {
		rescheduleCheck(newCfg.MonitorInterval)
	}
}

// rescheduleCheck 按新的间隔重新调度内容检查任务。
func rescheduleCheck(interval int) {
	if checkEntryID != 0 {
		cronScheduler.Remove(checkEntryID)
	}
	spec := fmt.Sprintf("@every %ds", interval)
	id, err := cronScheduler.AddFunc(spec, checkNewContent)
	if err != nil {
		log.Printf("热加载: 重新调度检查任务失败: %v\n", err)
		return
	}
	checkEntryID = id
	log.Printf("热加载: 检查频率已更新为每 %d 秒\n", interval)
}

func checkNewContent() {
	log.Printf("开始检查新内容...")

	// 快照当前状态，避免与热加载并发冲突，同时不在网络请求期间持锁。
	stateMu.RLock()
	cfg := config
	infos := userInfos
	st := storage
	stateMu.RUnlock()

	var test bool
	if len(os.Args) > 1 && os.Args[1] == "test" {
		test = true
	}

	var allItems []utils.ContentItem
	for _, userID := range cfg.ZhihuUserIDs {
		info, ok := infos[userID]
		if !ok {
			log.Printf("跳过博主 %s: 启动时未获取到用户信息\n", userID)
			continue
		}
		log.Printf("正在检查博主: %s\n", info.Name)

		answers, err := utils.FetchUserAnswers(userID, cfg.UserAgent, cfg.ZhihuCookie, 3)
		if err != nil {
			log.Printf("获取 %s 的回答列表失败: %v\n", info.Name, err)
		} else {
			for i := range answers {
				answers[i].TypeLabel = "回答"
				answers[i].AuthorName = info.Name
				allItems = append(allItems, answers[i])
			}
		}

		pins, err := utils.FetchUserPins(userID, cfg.UserAgent, 3)
		if err != nil {
			log.Printf("获取 %s 的想法列表失败: %v\n", info.Name, err)
		} else {
			for i := range pins {
				pins[i].TypeLabel = "想法"
				pins[i].AuthorName = info.Name
				allItems = append(allItems, pins[i])
			}
		}

		articles, err := utils.FetchUserArticles(userID, cfg.UserAgent, cfg.ZhihuCookie, 3)
		if err != nil {
			log.Printf("获取 %s 的文章列表失败: %v\n", info.Name, err)
		} else {
			for i := range articles {
				articles[i].TypeLabel = "文章"
				articles[i].AuthorName = info.Name
				allItems = append(allItems, articles[i])
			}
		}
	}

	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].CreatedTime > allItems[j].CreatedTime
	})

	newCount := 0
	for _, item := range allItems {
		content := item.Excerpt
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		var title, notificationContent string
		if item.TypeLabel == "回答" {
			title = fmt.Sprintf("%s回答了新问题", item.AuthorName)
			notificationContent = fmt.Sprintf("问题: %s\n\n摘要: %s", item.Title, content)
		} else {
			title = fmt.Sprintf("%s发布了新%s", item.AuthorName, item.TypeLabel)
			notificationContent = fmt.Sprintf("标题: %s\n\n摘要: %s", item.Title, content)
		}
		if test {
			log.Println(title)
			log.Println(notificationContent)
			continue
		}
		if !st.Contains(item.ID) {
			st.Add(item.ID)
			success := utils.SendFeishuNotification(cfg.FeishuWebhook, title, notificationContent, item.URL)
			if success {
				log.Println("飞书通知发送成功")
			}
			newCount++
		}
	}

	if newCount == 0 {
		log.Println("暂无新内容")
	}
}

func main() {
	utils.InitLogger("logs")

	cfg, rawData, err := readConfig()
	if err != nil {
		log.Printf("加载配置文件失败: %v\n", err)
		return
	}

	if !initMonitor(cfg, rawData) {
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "test" {
		checkNewContent()
		log.Println("测试模式: 仅执行一次检查，不启动定时器")
		return
	}
	checkNewContent()

	cronScheduler = cron.New(cron.WithSeconds())

	spec := fmt.Sprintf("@every %ds", config.MonitorInterval)
	checkEntryID, err = cronScheduler.AddFunc(spec, checkNewContent)
	if err != nil {
		log.Printf("设置定时任务失败: %v\n", err)
		return
	}

	// 每 1 分钟检查一次 config.yaml 是否变动，有变动则热加载。
	if _, err := cronScheduler.AddFunc("@every 1m", reloadConfigIfChanged); err != nil {
		log.Printf("设置配置热加载任务失败: %v\n", err)
		return
	}

	cronScheduler.Start()
	log.Println("监控已启动，config.yaml 每分钟自动热加载，按 Ctrl+C 停止")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	cronScheduler.Stop()
	log.Println("监控已停止")
}
