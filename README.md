# 知乎博主监控程序

一个用于监控知乎博主动态并通过飞书通知的 Go 语言程序。

## 功能特性

- 监控多个知乎博主的回答、文章和想法
- 分钟级定时检查（默认每60分钟检查一次）
- 发现新内容时通过飞书机器人发送通知
- 持久化存储已监控内容，避免重复通知

## 安装依赖

```bash
go mod tidy
```

## 配置说明

### 1. 获取知乎用户ID

打开知乎博主的个人主页，URL格式通常为：
`https://www.zhihu.com/people/xxx`

其中 `xxx` 就是用户ID。

例如：`https://www.zhihu.com/people/zhang-san`，用户ID为 `zhang-san`

### 2. 获取飞书Webhook地址

1. 打开飞书客户端，进入要接收通知的群聊
2. 点击群设置 -> 添加机器人 -> 创建自定义机器人
3. 设置机器人名称和头像，复制生成的Webhook地址

### 3. 修改配置文件

编辑 `config.yaml` 文件：

```yaml
zhihu_user_ids:
  - "你的知乎用户ID1"
  - "你的知乎用户ID2"
feishu_webhook_url: "你的飞书Webhook地址"
monitor_interval: 60
storage_file: monitored_items.json
user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"
```

## 运行程序

```bash
go run main.go
```

## 编译程序

```bash
go build -o zhihu-monitor
./zhihu-monitor
```

## 文件说明

```
├── config.yaml          # 配置文件
├── main.go              # 主程序
├── go.mod               # Go模块依赖
├── go.sum               # Go依赖校验
├── utils/
│   ├── zhihu.go         # 知乎API相关函数
│   ├── feishu.go        # 飞书通知相关函数
│   └── storage.go       # 持久化存储相关函数
└── monitored_items.json # 已监控内容记录（自动生成）
```

## 使用提示

- 首次运行时，程序会自动获取博主最近的内容并记录，不会发送通知
- 之后每次检查到新内容时，会自动发送飞书通知
- 按 `Ctrl+C` 可以停止程序
- 程序重启后会自动读取已监控内容记录，不会重复通知