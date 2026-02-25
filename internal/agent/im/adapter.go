package im

import (
	"encoding/json"
	"net/http"
	"strings"

	"aria/pkg/logging"
)

// DingTalkWebhook 钉钉 Webhook
type DingTalkWebhook struct {
	logger *logging.Logger
}

func NewDingTalkWebhook(logger *logging.Logger) *DingTalkWebhook {
	return &DingTalkWebhook{
		Logger: logger,
	}
}

// HandleWebhook 处理钉钉 Webhook
func (d *DingTalkWebhook) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event struct {
		ChatID string `json:"ChatId"`
		Text    string `json:"Text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		d.Logger.Error("Failed to decode webhook: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	d.Logger.Info("Received DingTalk webhook: chat_id=%s, text=%s", event.ChatID, event.Text)

	// TODO: 调用 Agent 处理消息
	// 目前返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// FeishuWebhook 飞书 Webhook
type FeishuWebhook struct {
	logger *logging.Logger
}

func NewFeishuWebhook(logger *logging.Logger) *FeishuWebhook {
	return &FeishuWebhook{
		Logger: logger,
	}
}

// HandleWebhook 处理飞书 Webhook
func (f *FeishuWebhook) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 简单处理，实际需要解析飞书 Webhook 格式
	var event struct {
		Challenge string `json:"challenge"`
	}

	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		f.Logger.Error("Failed to decode webhook: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	f.Logger.Info("Received Feishu webhook")

	// TODO: 调用 Agent 处理消息
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Adapter IM 适配器
type Adapter struct {
	logger *logging.Logger
	dingtalk *DingTalkWebhook
	feishu   *FeishuWebhook
}

// NewAdapter 创建 IM 适配器
func NewAdapter(logger *logging.Logger) *Adapter {
	return &Adapter{
		logger:   logger,
		dingtalk: NewDingTalkWebhook(logger),
		feishu:   NewFeishuWebhook(logger),
	}
}

// Start 启动 IM 适配器
func (a *Adapter) Start(addr string) error {
	mux := http.NewServeMux()

	// 钉钉 Webhook
	mux.HandleFunc("/im/dingtalk/webhook", a.dingtalk.HandleWebhook)
	// 飞书 Webhook
	mux.HandleFunc("/im/feishu/webhook", a.feishu.HandleWebhook)

	a.logger.Info("IM adapter listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}
