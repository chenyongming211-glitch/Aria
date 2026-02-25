package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds LLM adapter configuration
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	Temperature float64
	Timeout    time.Duration
}

// Adapter represents an LLM adapter
type Adapter struct {
	config *Config
	client *http.Client
}

// NewAdapter creates a new LLM adapter
func NewAdapter(config *Config) *Adapter {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	if config.Model == "" {
		config.Model = "deepseek-chat"
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}
	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	return &Adapter{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from LLM
type ToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ChatRequest represents a chat request
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

// Tool represents a tool definition for LLM
type Tool struct {
	Type     string `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition represents a function definition
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatResponse represents a chat response
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage   `json:"usage"`
}

// Choice represents a response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat sends a chat request and returns the response
func (a *Adapter) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	req := ChatRequest{
		Model:       a.config.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: a.config.Temperature,
		MaxTokens:   a.config.MaxTokens,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.config.BaseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s %s", resp.Status, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// ChatSimple sends a simple chat request without tools
func (a *Adapter) ChatSimple(ctx context.Context, messages []Message) (*ChatResponse, error) {
	return a.Chat(ctx, messages, nil)
}

// StreamChat sends a streaming chat request
func (a *Adapter) StreamChat(ctx context.Context, messages []Message, tools []Tool, onChunk func(string)) error {
	req := ChatRequest{
		Model:       a.config.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: a.config.Temperature,
		MaxTokens:   a.config.MaxTokens,
		Stream:      true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.config.BaseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s %s", resp.Status, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		line, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		text, ok := line.(string)
		if !ok {
			continue
		}

		if text == "data: [DONE]" {
			break
		}

		if len(text) > 6 && text[:6] == "data: " {
			onChunk(text[6:])
		}
	}

	return nil
}

// ConvertToolsToLLMFormat converts MCP tools to LLM tool format
func ConvertToolsToLLMFormat(mcpTools []interface{}) []Tool {
	tools := make([]Tool, 0, len(mcpTools))
	for range mcpTools {
		// This is a simplified conversion - in practice you'd need proper type assertion
		tools = append(tools, Tool{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "tool",
				Description: "MCP Tool",
				Parameters:  map[string]interface{}{},
			},
		})
	}
	return tools
}

// Session represents a chat session with history
const MaxSessionMessages = 20 // 最多保留最近 10 轮对话 (user + assistant = 2 条/轮)

type Session struct {
	ID       string
	Messages []Message
}

// NewSession creates a new chat session
func NewSession(id string) *Session {
	return &Session{
		ID:       id,
		Messages: []Message{},
	}
}

// AddMessage adds a message to the session
func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{
		Role:    role,
		Content: content,
	})
	// Token 裁剪：只保留最近的消息
	if len(s.Messages) > MaxSessionMessages {
		s.Messages = s.Messages[len(s.Messages)-MaxSessionMessages:]
	}
}

// AddToolCall adds a tool call to the session
func (s *Session) AddToolCall(callID, name, arguments string) {
	s.Messages = append(s.Messages, Message{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:   callID,
			Type: "function",
			Function: FunctionCall{
				Name:      name,
				Arguments: json.RawMessage(arguments),
			},
		}},
	})
}

// AddToolResult adds a tool result to the session
func (s *Session) AddToolResult(callID, content string) {
	s.Messages = append(s.Messages, Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    content,
	})
	// Token 裁剪：只保留最近的消息
	if len(s.Messages) > MaxSessionMessages {
		s.Messages = s.Messages[len(s.Messages)-MaxSessionMessages:]
	}
}
