package utils

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type UserInfo struct {
	Name     string `json:"name"`
	Headline string `json:"headline"`
	URL      string `json:"url"`
}

type ContentItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	TypeLabel   string `json:"type_label"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	CreatedTime int64  `json:"created_time"`
	URL         string `json:"url"`
	AuthorName  string `json:"-"`
}

type PinsResp struct {
	Paging interface{} `json:"paging"`
	Data   []PinsData  `json:"data"`
}

type PinsData struct {
	Updated int       `json:"updated"`
	ID      string    `json:"id"`
	URL     string    `json:"url"`
	Created int64     `json:"created"`
	Content []Content `json:"content"`
	Type    string    `json:"type"`
}

type Content struct {
	Content  string `json:"content"`
	FoldType string `json:"fold_type"`
	OwnText  string `json:"own_text"`
	Type     string `json:"type"`
}

type AnswerResp struct {
	Paging interface{}  `json:"paging"`
	Data   []AnswerData `json:"data"`
}

type AnswerData struct {
	CreatedTime int64    `json:"created_time"`
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Type        string   `json:"type"`
	Question    Question `json:"question"`
}

type Question struct {
	QuestionType string `json:"question_type"`
	Created      int    `json:"created"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	ID           string `json:"id"`
}

const maxRetries = 3

func FetchUserInfo(userID, userAgent string) (*UserInfo, error) {
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	url := fmt.Sprintf("https://www.zhihu.com/api/v4/members/%s", userID)
	resp, err := client.R().
		SetHeader("User-Agent", userAgent).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
		SetHeader("Referer", "https://www.zhihu.com/").
		Get(url)

	if err != nil {
		return nil, err
	}
	var result UserInfo
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func FetchUserAnswers(userID, userAgent, cookie string, limit int) ([]ContentItem, error) {
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	url := fmt.Sprintf("https://www.zhihu.com/api/v4/members/%s/answers", userID)

	var lastItems []ContentItem
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.R().
			SetHeader("User-Agent", userAgent).
			SetHeader("Accept", "application/json, text/plain, */*").
			SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
			SetHeader("Referer", "https://www.zhihu.com/").
			SetHeader("Cookie", cookie).
			SetQueryParam("limit", fmt.Sprintf("%d", limit)).
			Get(url)

		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				fmt.Printf("获取回答列表失败(第%d次): %v, 2秒后重试...\n", attempt, err)
				time.Sleep(2 * time.Second)
			}
			continue
		}

		var response AnswerResp
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			lastErr = err
			continue
		}

		var items []ContentItem
		for _, raw := range response.Data {
			items = append(items, ContentItem{
				ID:          fmt.Sprintf("%s", raw.ID),
				Type:        "answer",
				TypeLabel:   "回答",
				Title:       raw.Question.Title,
				CreatedTime: raw.CreatedTime,
				URL:         fmt.Sprintf("https://www.zhihu.com/question/%s/answer/%s", raw.Question.ID, raw.ID),
			})
		}
		lastItems = items

		if len(items) >= limit {
			return items, nil
		}

		if attempt < maxRetries {
			fmt.Printf("获取回答数量不足(第%d次): 期望%d条, 实际%d条, 2秒后重试...\n", attempt, limit, len(items))
			time.Sleep(2 * time.Second)
		}
	}

	if lastItems != nil {
		fmt.Printf("重试%d次后获取到%d条回答\n", maxRetries, len(lastItems))
		return lastItems, nil
	}
	return nil, lastErr
}

func FetchUserPins(userID, userAgent string, limit int) ([]ContentItem, error) {

	client := resty.New()
	client.SetTimeout(10 * time.Second)

	url := fmt.Sprintf("https://www.zhihu.com/api/v4/members/%s/pins", userID)

	var lastItems []ContentItem
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.R().
			SetHeader("User-Agent", userAgent).
			SetHeader("Accept", "application/json, text/plain, */*").
			SetHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8").
			SetHeader("Referer", "https://www.zhihu.com/").
			SetQueryParam("limit", fmt.Sprintf("%d", limit)).
			Get(url)

		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				fmt.Printf("获取想法列表失败(第%d次): %v, 2秒后重试...\n", attempt, err)
				time.Sleep(2 * time.Second)
			}
			continue
		}

		var response PinsResp
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			lastErr = err
			continue
		}

		fmt.Println("获取想法结果：", len(response.Data))
		var items []ContentItem
		for _, raw := range response.Data {
			if len(raw.Content) == 0 {
				continue
			}
			content := raw.Content[0]
			fmt.Println("content", content)
			items = append(items, ContentItem{
				ID:          fmt.Sprintf("%s", raw.ID),
				Type:        "pin",
				TypeLabel:   "想法",
				Title:       "想法",
				Excerpt:     content.Content,
				CreatedTime: raw.Created,
				URL:         fmt.Sprintf("https://www.zhihu.com/pin/%s", raw.ID),
			})
		}
		lastItems = items

		if len(items) >= limit {
			return items, nil
		}

		if attempt < maxRetries {
			fmt.Printf("获取想法数量不足(第%d次): 期望%d条, 实际%d条, 2秒后重试...\n", attempt, limit, len(items))
			time.Sleep(2 * time.Second)
		}
	}

	if lastItems != nil {
		fmt.Printf("重试%d次后获取到%d条想法\n", maxRetries, len(lastItems))
		return lastItems, nil
	}
	return nil, lastErr
}
