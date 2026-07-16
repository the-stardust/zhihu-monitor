package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
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

var (
	config    Config
	storage   *utils.Storage
	userInfos map[string]*utils.UserInfo
)

func loadConfig() error {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &config)
}

func initMonitor() bool {
	if len(config.ZhihuUserIDs) == 0 {
		log.Println("请先在 config.yaml 中配置知乎用户ID列表 (zhihu_user_ids)")
		return false
	}
	if config.FeishuWebhook == "" {
		log.Println("请先在 config.yaml 中配置飞书Webhook地址")
		return false
	}

	userInfos = make(map[string]*utils.UserInfo)
	for _, userID := range config.ZhihuUserIDs {
		info, err := utils.FetchUserInfo(userID, config.UserAgent)
		if err != nil {
			log.Printf("无法获取用户信息(%s): %v，已跳过该博主\n", userID, err)
			continue
		}
		userInfos[userID] = info
	}

	if len(userInfos) == 0 {
		log.Println("没有可监控的博主，请检查配置和网络")
		return false
	}

	storage = utils.NewStorage(config.StorageFile)

	log.Println("监控目标:")
	for _, userID := range config.ZhihuUserIDs {
		if info, ok := userInfos[userID]; ok {
			log.Printf("  - %s (%s)\n", info.Name, info.Headline)
		}
	}
	log.Printf("监控频率: 每 %d 秒\n", config.MonitorInterval)
	log.Println(strings.Repeat("=", 50))
	return true
}

func checkNewContent() {
	log.Printf("开始检查新内容...")

	var test bool
	if len(os.Args) > 1 && os.Args[1] == "test" {
		test = true
	}

	var allItems []utils.ContentItem
	for _, userID := range config.ZhihuUserIDs {
		info, ok := userInfos[userID]
		if !ok {
			log.Printf("跳过博主 %s: 启动时未获取到用户信息\n", userID)
			continue
		}
		log.Printf("正在检查博主: %s\n", info.Name)

		answers, err := utils.FetchUserAnswers(userID, config.UserAgent, config.ZhihuCookie, 3)
		if err != nil {
			log.Printf("获取 %s 的回答列表失败: %v\n", info.Name, err)
		} else {
			for i := range answers {
				answers[i].TypeLabel = "回答"
				answers[i].AuthorName = info.Name
				allItems = append(allItems, answers[i])
			}
		}

		pins, err := utils.FetchUserPins(userID, config.UserAgent, 3)
		if err != nil {
			log.Printf("获取 %s 的想法列表失败: %v\n", info.Name, err)
		} else {
			for i := range pins {
				pins[i].TypeLabel = "想法"
				pins[i].AuthorName = info.Name
				allItems = append(allItems, pins[i])
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
		if !storage.Contains(item.ID) {
			storage.Add(item.ID)
			success := utils.SendFeishuNotification(config.FeishuWebhook, title, notificationContent, item.URL)
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

	if err := loadConfig(); err != nil {
		log.Printf("加载配置文件失败: %v\n", err)
		return
	}

	if !initMonitor() {
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "test" {
		checkNewContent()
		log.Println("测试模式: 仅执行一次检查，不启动定时器")
		return
	}
	checkNewContent()

	c := cron.New(cron.WithSeconds())
	spec := fmt.Sprintf("@every %ds", config.MonitorInterval)
	_, err := c.AddFunc(spec, checkNewContent)
	if err != nil {
		log.Printf("设置定时任务失败: %v\n", err)
		return
	}

	c.Start()
	log.Println("监控已启动，按 Ctrl+C 停止")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	c.Stop()
	log.Println("监控已停止")
}
