package im

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"aria/internal/service"
)

// DingTalkHandler 钉钉机器人处理器
type DingTalkHandler struct {
	aiService service.AIService
	webhook   string // 钉钉 Webhook URL（用于主动推送消息）
	secret    string // 钉钉机器人加签密钥
}

// NewDingTalkHandler 创建钉钉处理器
func NewDingTalkHandler(aiSvc service.AIService, webhook, secret string) *DingTalkHandler {
	return &DingTalkHandler{
		aiService: aiSvc,
		webhook:   webhook,
		secret:    secret,
	}
}

// DingTalkMessage 钉钉消息格式
type DingTalkMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
	Markdown struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"markdown"`
}

// DingTalkCallbackRequest 钉钉 Webhook 回调请求
type DingTalkCallbackRequest struct {
	MsgID    string `json:"msgId"`
	Content  struct {
		Text string `json:"text"`
	} `json:"content"`
	SenderNick string `json:"senderNick"`
	SenderID   string `json:"senderId"`
	ChatType   string `json:"chatType"` // single, group
}

// HandleWebhook 处理钉钉 Webhook 回调
func (h *DingTalkHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read request body"})
		return
	}

	// 2. 验证签名（如果配置了 secret）
	if h.secret != "" {
		timestamp := r.Header.Get("timestamp")
		sign := r.Header.Get("sign")
		if !h.verifySign(timestamp, sign) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid signature"})
			return
		}
	}

	// 3. 解析消息
	var req DingTalkCallbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		// 钉钉群机器人的格式可能不同，尝试另一种解析
		plainText := string(body)
		h.processMessage(plainText, w, r)
		return
	}

	// 4. 提取用户消息
	userMessage := ""
	if req.Content.Text != "" {
		userMessage = req.Content.Text
	}

	// 5. 处理消息
	h.processMessage(userMessage, w, r)
}

// processMessage 处理用户消息
func (h *DingTalkHandler) processMessage(message string, w http.ResponseWriter, r *http.Request) {
	if message == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// 记录收到消息
	fmt.Printf("[DingTalk] 收到消息: %s\n", message)

	// 调用 AI Service
	ctx := r.Context()
	reply, err := h.aiService.Chat(ctx, message)
	if err != nil {
		fmt.Printf("[DingTalk] AI 调用失败: %v\n", err)
		reply = fmt.Sprintf("抱歉，处理您的请求时出错：%v", err)
	}

	// 记录 AI 回复
	fmt.Printf("[DingTalk] AI 回复: %s\n", reply)

	// 如果是 Webhook 回调模式，直接返回
	if w != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"reply":  reply,
		})
		return
	}

	// 如果是主动推送模式，调用钉钉 Webhook
	if h.webhook != "" {
		h.sendReply(reply)
	}
}

// sendReply 通过钉钉 Webhook 发送回复
func (h *DingTalkHandler) sendReply(reply string) error {
	if h.webhook == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	// 构造消息
	msg := DingTalkMessage{
		MsgType: "text",
	}
	msg.Text.Content = reply

	// 如果启用了 Markdown，可以切换为 Markdown 格式
	// msg.MsgType = "markdown"
	// msg.Markdown.Title = "Aria AI 助手"
	// msg.Markdown.Text = reply

	// 序列化
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发送请求
	req, err := http.NewRequest("POST", h.webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// verifySign 验证钉钉签名
func (h *DingTalkHandler) verifySign(timestamp, sign string) bool {
	if h.secret == "" {
		return true // 没有配置密钥则跳过验证
	}

	if timestamp == "" || sign == "" {
		return false
	}

	// 拼接字符串: timestamp + \n + secret
	stringToSign := fmt.Sprintf("%s\n%s", timestamp, h.secret)

	// 计算 HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(stringToSign))
	expectedSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return sign == expectedSign
}

// SendNotification 发送通知到钉钉
func (h *DingTalkHandler) SendNotification(message string) error {
	return h.sendReply(message)
}

// SendAlert 发送告警到钉钉
func (h *DingTalkHandler) SendAlert(title, content string) error {
	msg := DingTalkMessage{
		MsgType: "markdown",
	}
	msg.Markdown.Title = title
	msg.Markdown.Text = fmt.Sprintf("### %s\n\n%s", title, content)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", h.webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send alert: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}
