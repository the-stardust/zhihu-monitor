package utils

import (
	"log"
	"time"

	"github.com/go-resty/resty/v2"
)

type feishuPayload struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

func SendFeishuNotification(webhookURL, title, content, link string) bool {
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	payload := feishuPayload{
		MsgType: "text",
	}
	payload.Content.Text = "<at user_id=\"ou_24885417697cf3ee51ad2c6a9a5eee76\"></at>【" + title + "】\n\n" + content + "\n\n🔗 查看链接：" + link

	log.Println("发送飞书通知:", payload.Content.Text)
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(webhookURL)

	return err == nil
}
