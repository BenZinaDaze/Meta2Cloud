// Package telegram Telegram Bot API 传输层：消息与图片发送。
// 仅负责「把文本/图片送到会话」，不含任何业务文案构建（文案在调用方，如
// pipeline 的入库通知、webui 的健康/测试通知），使 webui 复用发送能力时
// 不必依赖 pipeline 包。
package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"meta2cloud/internal/config"
)

// SendMessage 用配置的 Bot 发送一条 HTML 文本消息。
func SendMessage(tg config.Telegram, text string) error {
	if tg.BotToken == "" || tg.ChatID == "" {
		return fmt.Errorf("Telegram Bot Token 或 Chat ID 未配置")
	}
	return post(tg, "sendMessage", map[string]any{
		"chat_id":    tg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}, 10)
}

// SendPhoto 发送一条带图片的 HTML 通知。
// imageURL 为完整图片地址（如 TMDB 横版背景图），拉取失败时返回错误。
func SendPhoto(tg config.Telegram, imageURL, caption string) error {
	if tg.BotToken == "" || tg.ChatID == "" {
		return fmt.Errorf("Telegram Bot Token 或 Chat ID 未配置")
	}
	return post(tg, "sendPhoto", map[string]any{
		"chat_id":    tg.ChatID,
		"photo":      imageURL,
		"caption":    caption,
		"parse_mode": "HTML",
	}, 15)
}

// post 调用一个 Telegram Bot API 方法（失败时记日志并返回错误）。
func post(tg config.Telegram, method string, payload map[string]any, timeoutSecs int) error {
	resp, err := postJSON(fmt.Sprintf("https://api.telegram.org/bot%s/%s", tg.BotToken, method), payload, timeoutSecs)
	if err != nil {
		err = fmt.Errorf("请求异常：%w", redactURLError(err))
		log.Printf("Telegram %s %v", method, err)
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(body, &result) == nil && !result.OK {
		err = fmt.Errorf("%s", strings.TrimSpace(string(body)))
		log.Printf("Telegram %s 发送失败：%s", method, string(body))
		return err
	}
	return nil
}

// postJSON POST JSON 并返回响应（供本包与测试使用）。
func postJSON(url string, payload any, timeoutSecs int) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	return client.Post(url, "application/json", strings.NewReader(string(data)))
}

// redactURLError 去掉 *url.Error 携带的完整 URL（路径含 bot token），只保留底层错误，
// 防止请求失败时凭据随错误日志与上层调用方日志外泄。
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
