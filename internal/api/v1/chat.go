package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aria/internal/service"
)

type ChatHandler struct {
	aiSvc service.AIService
}

func NewChatHandler(aiSvc service.AIService) *ChatHandler {
	return &ChatHandler{aiSvc: aiSvc}
}

// HandleChat 处理 POST /api/v1/ai/chat
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Tools     bool   `json:"tools"`
	}

	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request", nil)
		return
	}

	if req.Message == "" {
		WriteError(w, http.StatusBadRequest, CodeMessageEmpty, "message cannot be empty", nil)
		return
	}

	// 调用 Service 层（支持会话历史）
	reply, err := h.aiSvc.ChatWithContext(r.Context(), req.SessionID, req.Message)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeAIServiceError, "AI service error: "+err.Error(), nil)
		return
	}

	// 如果没有 session_id，生成一个（用于前端跟踪）
	if req.SessionID == "" {
		req.SessionID = generateSessionID()
	}

	// 检测 CARD_DATA: 前缀（工具返回的卡片数据）
	var cardData interface{}
	var needsConfirm bool
	var toolCalls []map[string]interface{}

	trimmedReply := strings.TrimSpace(reply)
	if len(trimmedReply) > 10 && trimmedReply[:10] == "CARD_DATA:" {
		jsonStr := strings.TrimSpace(trimmedReply[10:])
		if err := json.Unmarshal([]byte(jsonStr), &cardData); err == nil {
			fmt.Printf("[Chat] 🎯 检测到 CARD_DATA，解析成功\n")
			// 卡片数据直接返回，不需要 LLM 总结
		} else {
			fmt.Printf("[Chat] ⚠️ CARD_DATA 解析失败: %v\n", err)
		}
	} else {
		// 普通文本回复
	}

	// 返回结果（前端期望的格式）
	responseData := map[string]interface{}{
		"session_id":   req.SessionID,
		"reply":        reply,
		"card_data":    cardData,
		"tool_calls":   toolCalls,
		"needs_confirm": needsConfirm,
	}

	WriteSuccess(w, responseData, "Chat response generated successfully")
}

// HandleConfirm 处理 POST /api/v1/ai/confirm
func (h *ChatHandler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req struct {
		SessionID string            `json:"session_id"`
		ToolName  string            `json:"tool_name"`
		Params    map[string]any    `json:"params"`
		Confirmed bool              `json:"confirmed"`
	}

	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidRequest, "Invalid request", nil)
		return
	}

	// 调用 Service 层执行确认的工具
	result, err := h.aiSvc.ExecuteTool(r.Context(), req.SessionID, req.ToolName, req.Params, req.Confirmed)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, CodeAIServiceError, "AI service error: "+err.Error(), nil)
		return
	}

	// 返回结果
	responseData := map[string]interface{}{
		"session_id": req.SessionID,
		"result":     result,
	}

	WriteSuccess(w, responseData, "Tool execution completed successfully")
}

// generateSessionID 生成简单的会话 ID
func generateSessionID() string {
	return "sess_" + randomString(8)
}
