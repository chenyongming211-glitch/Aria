package im

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"aria/internal/service"
)

// FeishuHandler 飞书机器人处理器
type FeishuHandler struct {
	aiService   service.AIService
	appID       string // 飞书 App ID
	appSecret   string // 飞书 App Secret
	encryptKey  string // 加密密钥（可选）
	verifyToken string // 验证 Token（用于验证飞书请求）
	mu          sync.RWMutex
	processed   map[string]time.Time // 已处理的事件 ID + 时间
}

// ChatWithContextProvider 提供带上下文的聊天接口
type ChatWithContextProvider interface {
	ChatWithContext(ctx context.Context, chatID, prompt string) (string, error)
}

// NewFeishuHandler 创建飞书处理器
func NewFeishuHandler(aiSvc service.AIService, appID, appSecret, encryptKey, verifyToken string) *FeishuHandler {
	h := &FeishuHandler{
		aiService:   aiSvc,
		appID:       appID,
		appSecret:   appSecret,
		encryptKey:  encryptKey,
		verifyToken: verifyToken,
		processed:   make(map[string]time.Time),
	}

	// 启动定期清理任务
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.mu.Lock()
			for id, t := range h.processed {
				if time.Since(t) > 2*time.Hour {
					delete(h.processed, id)
				}
			}
			h.mu.Unlock()
		}
	}()

	return h
}

// FeishuEvent 飞书事件
type FeishuEvent struct {
	Schema string `json:"schema"`
	Header struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
		TenantKey string `json:"tenant_key"`
		Token     string `json:"token"`
		AppID     string `json:"app_id"`
		Timestamp string `json:"create_time"` // 飞书返回的是字符串
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID string `json:"open_id"`
				UserID string `json:"user_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"`
		} `json:"sender"`
		Message struct {
			MessageID string `json:"message_id"`
			ChatType  string `json:"chat_type"`
			ChatID    string `json:"chat_id"`
			Content   string `json:"content"` // JSON string
		} `json:"message"`
	} `json:"event"`

	// URL 校验字段
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
}

// FeishuMessageContent 飞书消息内容
type FeishuMessageContent struct {
	Text string `json:"text"`
}

// FeishuSendMessageRequest 飞书发送消息请求
type FeishuSendMessageRequest struct {
	ReceiveIDType string `json:"receive_id_type"` // open_id, user_id, union_id
	ReceiveID     string `json:"receive_id"`
	MsgType       string `json:"msg_type"`
	Content       string `json:"content"` // JSON string
}

// FeishuAccessTokenResponse 飞书 Token 响应
type FeishuAccessTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	AppAccessToken    string `json:"app_access_token"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

func writeFeishuJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Printf("[Feishu] 写入 JSON 响应失败: %v\n", err)
	}
}

func (h *FeishuHandler) verifyIncomingToken(event FeishuEvent) bool {
	if h.verifyToken == "" {
		return true
	}
	token := event.Header.Token
	if token == "" {
		token = event.Token
	}
	return token == h.verifyToken
}

// HandleWebhook 处理飞书 Webhook 回调（长连接方式）
func (h *FeishuHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeFeishuJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
		return
	}

	// 2. 解析事件（长连接方式，明文推送）
	var event FeishuEvent
	if err := json.Unmarshal(body, &event); err != nil {
		fmt.Printf("[Feishu] 解析事件失败: %v, body: %s\n", err, string(body))
		writeFeishuJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to parse event"})
		return
	}

	// 2.1 验证 Token（如果配置了 verify_token）
	if !h.verifyIncomingToken(event) {
		fmt.Printf("[Feishu] Token 验证失败\n")
		writeFeishuJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
		return
	}

	// 2.1 去重检查：防止飞书重复发送请求
	eventID := event.Header.EventID
	if eventID != "" {
		h.mu.RLock()
		_, exists := h.processed[eventID]
		h.mu.RUnlock()

		if exists {
			fmt.Printf("[Feishu] 重复事件，跳过处理: %s\n", eventID)
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "ok", "reason": "duplicate"})
			return
		}

		// 标记为已处理
		h.mu.Lock()
		h.processed[eventID] = time.Now()
		h.mu.Unlock()
	}

	// 3. 处理 URL 校验（飞书配置 Webhook 时触发）
	if event.Type == "url_verification" {
		fmt.Printf("[Feishu] URL 校验请求: challenge=%s\n", event.Challenge)
		writeFeishuJSON(w, http.StatusOK, map[string]string{
			"challenge": event.Challenge,
		})
		return
	}

	// 4. 只处理文本消息事件（使用新版事件类型）
	if event.Header.EventType != "im.message.receive_v1" {
		writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// 5. 提取消息内容
	var content FeishuMessageContent
	if err := json.Unmarshal([]byte(event.Event.Message.Content), &content); err != nil {
		fmt.Printf("[Feishu] 解析消息内容失败: %v\n", err)
		writeFeishuJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to parse message content"})
		return
	}

	userMessage := content.Text
	if userMessage == "" {
		writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// 6. 处理消息
	h.processMessage(userMessage, event.Event.Message.ChatID, w, r)
}

// ReplyFeishu 发送消息 (统一入口)
// content: 可以是普通文本，也可以是卡片 JSON 字符串
// msgType: "text" 或 "interactive"
func (h *FeishuHandler) ReplyFeishu(chatID string, content string, msgType string) error {
	if h.appID == "" || h.appSecret == "" {
		return fmt.Errorf("feishu app ID or secret not configured")
	}

	token, err := h.getAppAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get app access token: %w", err)
	}

	reqData := map[string]interface{}{
		"receive_id": chatID,
		"msg_type":   msgType,
		"content":    content,
	}

	// 特殊处理：如果是文本类型，飞书要求 content 必须也是个 JSON {"text": "..."}
	if msgType == "text" {
		textMap := map[string]string{"text": content}
		textBytes, _ := json.Marshal(textMap)
		reqData["content"] = string(textBytes)
	}

	reqBytes, _ := json.Marshal(reqData)
	apiURL := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"

	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(reqBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Feishu API Error: %s", string(body))
	}

	return nil
}

// processMessage 处理用户消息
func (h *FeishuHandler) processMessage(message, chatID string, w http.ResponseWriter, r *http.Request) {
	// 记录收到消息
	fmt.Printf("[Feishu] 收到消息: %s\n", message)

	// ✅ 修复点：大幅增加超时时间到 120 秒
	// DeepSeek V3/R1 进行工具调用时，涉及 "Think -> Tool -> Think" 多轮交互，耗时较长
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 调用 AI Service（传入 chatID 以支持会话上下文）
	var reply string
	var err error
	if ctxProvider, ok := h.aiService.(ChatWithContextProvider); ok {
		reply, err = ctxProvider.ChatWithContext(ctx, chatID, message)
	} else {
		// 降级到无上下文模式
		reply, err = h.aiService.Chat(ctx, "", message)
	}
	if err != nil {
		fmt.Printf("[Feishu] AI 调用失败: %v\n", err)
		reply = fmt.Sprintf("抱歉，处理您的请求时出错：%v", err)
	}

	// 记录 AI 回复
	fmt.Printf("[Feishu] AI 回复: %s\n", reply)

	// ---------------------------------------------------------
	// ✅ 新增：智能卡片检测逻辑
	// ---------------------------------------------------------

	// 1. 检测 CARD_DATA: 前缀（工具返回的标准格式）
	trimmedReply := strings.TrimSpace(reply)
	var jsonData string
	if len(trimmedReply) > 10 && trimmedReply[:10] == "CARD_DATA:" {
		jsonData = strings.TrimSpace(trimmedReply[10:])
		fmt.Printf("[Feishu] 🎯 检测到 CARD_DATA 标记\n")
	} else {
		jsonData = trimmedReply
	}

	// 2. 尝试检测是否为节点列表 (JSON Array)
	if strings.Contains(jsonData, "assigned_ip") && strings.HasPrefix(jsonData, "[") {
		var nodes []NodeInfo
		if err := json.Unmarshal([]byte(jsonData), &nodes); err == nil {
			fmt.Println("🎨 检测到节点列表数据，正在渲染卡片...")
			cardJSON := BuildNodeListCard(nodes)
			if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
				fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
				writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
			return
		}
	}

	// 3. 尝试检测是否为单个节点详情 (JSON Object)
	if strings.Contains(jsonData, "assigned_ip") && strings.HasPrefix(jsonData, "{") {
		var node NodeInfo
		if err := json.Unmarshal([]byte(jsonData), &node); err == nil {
			fmt.Println("🎨 检测到节点详情数据，正在渲染卡片...")
			cardJSON := BuildNodeDetailCard(node)
			if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
				fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
				writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
			return
		}
	}

	// 4. 尝试检测是否为策略列表（JSON Array，包含 src_net 字段）
	if strings.Contains(jsonData, "src_net") && strings.HasPrefix(jsonData, "[") {
		fmt.Println("🎨 检测到策略列表数据，正在渲染卡片...")
		var policies []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &policies); err != nil {
			fmt.Println("❌ 解析策略列表失败:", err)
			// 解析失败，使用通用 JSON 渲染
			cardJSON := BuildGenericJSONCard(jsonData)
			if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
				fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
				writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
			return
		}
		cardJSON := BuildGenericJSONCard(jsonData)
		if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
			fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
			writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
		return
	}

	// 5. 尝试检测是否为路由列表（JSON Array，包含 routes 字段）
	if strings.Contains(jsonData, "routes") && strings.HasPrefix(jsonData, "[") {
		var routes []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &routes); err == nil {
			fmt.Println("🎨 检测到路由列表数据，正在渲染卡片...")
			cardJSON := BuildRouteListCard(routes)
			if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
				fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
				writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
			return
		}
	}

	// 5. 尝试检测是否为通用 JSON 数据（Object 或 Array）
	// 如果没有被前面的检测逻辑捕获，则使用通用渲染
	if len(jsonData) > 0 {
		// 尝试解析为对象
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &obj); err == nil && !strings.Contains(jsonData, "assigned_ip") && !strings.Contains(jsonData, "routes") {
			fmt.Println("🎨 检测到通用 JSON 数据，正在渲染卡片...")
			cardJSON := BuildGenericJSONCard(jsonData)
			if err := h.ReplyFeishu(chatID, cardJSON, "interactive"); err != nil {
				fmt.Printf("[Feishu] 发送卡片失败: %v\n", err)
				writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
			return
		}
	}

	// 6. 默认兜底：发送普通文本
	if err := h.ReplyFeishu(chatID, reply, "text"); err != nil {
		fmt.Printf("[Feishu] 发送回复失败: %v\n", err)
		writeFeishuJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// 返回成功
	writeFeishuJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// sendMessage 发送消息到飞书
func (h *FeishuHandler) sendMessage(chatID, message string) error {
	if h.appID == "" || h.appSecret == "" {
		return fmt.Errorf("feishu app ID or secret not configured")
	}

	// 1. 获取 App Access Token
	fmt.Printf("[Feishu] 准备发送消息: chatID=%s, content=%s\n", chatID, message)
	token, err := h.getAppAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get app access token: %w", err)
	}

	// 2. 构造消息内容
	content := FeishuMessageContent{Text: message}
	contentJSON, _ := json.Marshal(content)

	req := FeishuSendMessageRequest{
		ReceiveIDType: "chat_id",
		ReceiveID:     chatID,
		MsgType:       "text",
		Content:       string(contentJSON),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 3. 发送消息
	fmt.Printf("[Feishu] 发送消息到 chatID=%s: %s\n", chatID, message)
	httpReq, err := http.NewRequest("POST", "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
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

// getAppAccessToken 获取飞书 App Access Token
func (h *FeishuHandler) getAppAccessToken() (string, error) {
	reqBody := map[string]string{
		"app_id":     h.appID,
		"app_secret": h.appSecret,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var tokenResp FeishuAccessTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if tokenResp.Code != 0 {
		return "", fmt.Errorf("failed to get token: %s", tokenResp.Msg)
	}

	return tokenResp.AppAccessToken, nil
}

// verifySign 验证飞书签名
func (h *FeishuHandler) verifySign(timestamp, nonce, signature, body string) bool {
	if h.encryptKey == "" {
		return true // 没有配置密钥则跳过验证
	}

	if timestamp == "" || nonce == "" || signature == "" {
		return false
	}

	// 拼接字符串: timestamp + nonce + body
	stringToSign := timestamp + nonce + body

	// 计算 HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(h.encryptKey))
	mac.Write([]byte(stringToSign))
	expectedSign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return signature == expectedSign
}

// SendNotification 发送通知到飞书（需要指定 chatID）
func (h *FeishuHandler) SendNotification(chatID, message string) error {
	return h.sendMessage(chatID, message)
}

// SendAlert 发送告警到飞书
func (h *FeishuHandler) SendAlert(chatID, title, content string) error {
	message := fmt.Sprintf("### %s\n\n%s", title, content)
	return h.sendMessage(chatID, message)
}
