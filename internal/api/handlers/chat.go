package handlers

import (
	"net/http"

	"aria/internal/api/apibase"
	"aria/internal/service"
)

type ChatHandler struct {
	aiSvc service.AIService
}

func NewChatHandler(aiSvc service.AIService) *ChatHandler {
	return &ChatHandler{aiSvc: aiSvc}
}

// HandleChat 处理 POST /ai/chat
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
	}

	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	if req.Message == "" {
		apibase.WriteError(w, http.StatusBadRequest, "MESSAGE_EMPTY", "Message cannot be empty", nil)
		return
	}

	// 调用 AI 服务
	reply, err := h.aiSvc.ChatWithContext(r.Context(), req.SessionID, req.Message)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, "AI_SERVICE_ERROR", err.Error(), nil)
		return
	}

	responseData := map[string]interface{}{
		"session_id": req.SessionID,
		"reply":      reply,
	}

	apibase.WriteSuccess(w, responseData, "Chat response generated successfully")
}

// HandleConfirm 处理 POST /ai/confirm
func (h *ChatHandler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req struct {
		SessionID string            `json:"session_id"`
		ToolName  string            `json:"tool_name"`
		Params    map[string]any    `json:"params"`
	}

	if err := apibase.ParseRequestJSON(r, &req); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidRequest, "Invalid request body", nil)
		return
	}

	result, err := h.aiSvc.ExecuteTool(r.Context(), req.SessionID, req.ToolName, req.Params, true)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, "AI_SERVICE_ERROR", err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, result, "Tool confirmed and executed")
}
