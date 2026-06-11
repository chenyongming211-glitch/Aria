package brain

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"aria/internal/agent/tools"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// Agent AI 智能助手
type Agent struct {
	llm          llms.Model
	systemPrompt string
	tools        []tools.Tool
}

// NewAgent 初始化 Agent
func NewAgent() *Agent {
	// 从环境变量获取 DeepSeek 配置
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	model := os.Getenv("DEEPSEEK_MODEL")
	systemPrompt := os.Getenv("DEEPSEEK_SYSTEM_PROMPT")

	// 默认配置
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	if model == "" {
		model = "deepseek-chat"
	}
	if systemPrompt == "" {
		systemPrompt = "You are Aria's intelligent operations assistant. Help users manage their SD-WAN network."
	}

	// 创建 OpenAI 兼容客户端（DeepSeek 使用 OpenAI 协议）
	llm, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL),
		openai.WithModel(model),
	)
	if err != nil {
		// 如果初始化失败，返回 nil，后续在 Think 中会处理
		return &Agent{llm: nil, systemPrompt: systemPrompt}
	}

	return &Agent{llm: llm, systemPrompt: systemPrompt, tools: []tools.Tool{}}
}

// RegisterTool 注册一个工具
func (a *Agent) RegisterTool(tool tools.Tool) {
	a.tools = append(a.tools, tool)
}

// GetTools 获取所有工具
func (a *Agent) GetTools() []tools.Tool {
	return a.tools
}

// CleanLLMResponse 清洗 DeepSeek 的回复，提取纯 JSON
// DeepSeek 经常在 JSON 前后加上 Markdown 标记或思考过程，导致解析失败
func CleanLLMResponse(raw string) string {
	if raw == "" {
		return raw
	}

	// 1. 移除思考过程 (如果是 R1 模型，会返回 <think>...</think>)
	thinkRe := regexp.MustCompile(`(?s)<think>.*?</think>`)
	cleaned := thinkRe.ReplaceAllString(raw, "")

	// 2. 移除 Markdown 代码块标记 ```json 和 ```
	cleaned = strings.ReplaceAll(cleaned, "```json", "")
	cleaned = strings.ReplaceAll(cleaned, "```", "")

	// 3. 移除其他可能的 Markdown 标记
	cleaned = strings.ReplaceAll(cleaned, "```text", "")
	cleaned = strings.ReplaceAll(cleaned, "```", "")

	// 4. 移除首尾空白
	cleaned = strings.TrimSpace(cleaned)

	// 5. 移除可能的 JSON 前缀/后缀文字（如 "json:"）
	// 寻找第一个 { 和最后一个 }
	firstBrace := strings.Index(cleaned, "{")
	lastBrace := strings.LastIndex(cleaned, "}")
	if firstBrace >= 0 && lastBrace >= 0 {
		cleaned = cleaned[firstBrace : lastBrace+1]
	}

	return cleaned
}

func toolDefinitions(toolList []tools.Tool) []llms.Tool {
	llmTools := make([]llms.Tool, 0, len(toolList))
	for _, t := range toolList {
		llmTools = append(llmTools, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return llmTools
}

func runTool(toolList []tools.Tool, name, args string) (string, bool) {
	for _, t := range toolList {
		if t.Name == name {
			res, err := t.Run(args)
			if err != nil {
				return fmt.Sprintf("Tool Error: %v", err), true
			}
			return res, true
		}
	}
	return "Error: Tool not found", false
}

// Think 让 Agent 思考并回答问题（支持 Function Calling）
func (a *Agent) Think(ctx context.Context, prompt string) (string, error) {
	return a.ThinkWithTools(ctx, prompt, a.tools)
}

// ThinkWithTools 使用调用方提供的工具列表思考并回答问题。
func (a *Agent) ThinkWithTools(ctx context.Context, prompt string, toolList []tools.Tool) (string, error) {
	if a.llm == nil {
		// LLM 未初始化，返回提示
		return "AI 服务未正确配置，请检查 DEEPSEEK_API_KEY 环境变量", nil
	}

	llmTools := toolDefinitions(toolList)
	// 告诉它：用户说了啥，以及你有啥工具
	fmt.Printf("🤖 Agent 思考中... 用户Prompt: %s\n", prompt)
	resp, err := a.llm.GenerateContent(ctx,
		[]llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, a.systemPrompt),
			llms.TextParts(llms.ChatMessageTypeHuman, prompt),
		},
		llms.WithTools(llmTools), // <--- 关键：注入工具
	)
	if err != nil {
		return "", err
	}

	choice := resp.Choices[0]

	// 3. 检查 LLM 是否想调用工具
	if len(choice.ToolCalls) > 0 {
		toolCall := choice.ToolCalls[0]
		fmt.Printf("🛠️ Agent 决定调用工具: %s\n", toolCall.FunctionCall.Name)

		// 清洗参数：DeepSeek 可能返回带 Markdown 标记的参数
		cleanedArgs := CleanLLMResponse(toolCall.FunctionCall.Arguments)
		fmt.Printf("🔧 参数清洗: 原始=%s -> 清洗后=%s\n", toolCall.FunctionCall.Arguments, cleanedArgs)

		executionResult, _ := runTool(toolList, toolCall.FunctionCall.Name, cleanedArgs)

		fmt.Printf("✅ 工具执行结果: %s\n", executionResult)

		// 检查是否为卡片数据（特殊标记 CARD_DATA:）
		if len(executionResult) > 10 && executionResult[0:10] == "CARD_DATA:" {
			// 直接返回工具的原始结果，不进行第二次 LLM 调用
			fmt.Printf("🎯 Think() 检测到卡片数据，直接返回原始值\n")
			return executionResult, nil
		}

		finalResp, err := a.llm.GenerateContent(ctx, []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, a.systemPrompt),
			llms.TextParts(llms.ChatMessageTypeHuman, prompt), // 用户的原话
			{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					// 必须带上它刚才的"想调用工具"的念头，否则上下文接不上
					llms.ToolCall{
						ID:           toolCall.ID,
						Type:         "function",
						FunctionCall: toolCall.FunctionCall,
					},
				},
			},
			{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					// 喂给它工具的执行结果
					llms.ToolCallResponse{
						ToolCallID: toolCall.ID,
						Name:       toolCall.FunctionCall.Name,
						Content:    executionResult,
					},
				},
			},
		})

		if err != nil {
			return "", err
		}

		return finalResp.Choices[0].Content, nil
	}

	// 如果不需要调工具，直接返回它的回复
	return choice.Content, nil
}

// ThinkWithHistory 带历史记录的思考方法
func (a *Agent) ThinkWithHistory(ctx context.Context, prompt string, history []Message) (string, error) {
	return a.ThinkWithHistoryTools(ctx, prompt, history, a.tools)
}

// ThinkWithHistoryTools 使用调用方提供的工具列表进行带历史记录的思考。
func (a *Agent) ThinkWithHistoryTools(ctx context.Context, prompt string, history []Message, toolList []tools.Tool) (string, error) {
	if a.llm == nil {
		// LLM 未初始化，返回提示
		return "AI 服务未正确配置，请检查 DEEPSEEK_API_KEY 环境变量", nil
	}

	return a.thinkWithHistory(ctx, prompt, history, toolList)
}

// thinkWithHistory 内部实现
func (a *Agent) thinkWithHistory(ctx context.Context, prompt string, history []Message, toolList []tools.Tool) (string, error) {
	llmTools := toolDefinitions(toolList)
	// 构建消息列表
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, a.systemPrompt),
	}

	// 添加历史消息（如果有）
	if len(history) > 0 {
		for _, msg := range history {
			if msg.Role == "user" {
				messages = append(messages, llms.TextParts(llms.ChatMessageTypeHuman, msg.Content))
			} else if msg.Role == "assistant" {
				messages = append(messages, llms.TextParts(llms.ChatMessageTypeAI, msg.Content))
			}
		}
	}

	// 添加当前用户消息
	messages = append(messages, llms.TextParts(llms.ChatMessageTypeHuman, prompt))

	resp, err := a.llm.GenerateContent(ctx, messages, llms.WithTools(llmTools))
	if err != nil {
		return "", err
	}

	choice := resp.Choices[0]

	// 检查 LLM 是否想调用工具
	if len(choice.ToolCalls) > 0 {
		toolCall := choice.ToolCalls[0]
		fmt.Printf("🛠️ Agent 决定调用工具: %s\n", toolCall.FunctionCall.Name)

		// 清洗参数：DeepSeek 可能返回带 Markdown 标记的参数
		cleanedArgs := CleanLLMResponse(toolCall.FunctionCall.Arguments)
		fmt.Printf("🔧 参数清洗: 原始=%s -> 清洗后=%s\n", toolCall.FunctionCall.Arguments, cleanedArgs)

		executionResult, _ := runTool(toolList, toolCall.FunctionCall.Name, cleanedArgs)

		fmt.Printf("✅ 工具执行结果: %s\n", executionResult)

		// 检查是否为卡片数据（特殊标记 CARD_DATA:）
		if len(executionResult) > 10 && executionResult[0:10] == "CARD_DATA:" {
			// 直接返回工具的原始结果，不进行第二次 LLM 调用
			fmt.Printf("🎯 检测到卡片数据，直接返回原始值\n")
			return executionResult, nil
		}

		// 第二次调用 LLM，带上完整历史和工具执行结果
		messages = append(messages,
			llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					// 必须带上它刚才的"想调用工具"的念头
					llms.ToolCall{
						ID:           toolCall.ID,
						Type:         "function",
						FunctionCall: toolCall.FunctionCall,
					},
				},
			},
			llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: toolCall.ID,
						Name:       toolCall.FunctionCall.Name,
						Content:    executionResult,
					},
				},
			},
		)

		finalResp, err := a.llm.GenerateContent(ctx, messages)
		if err != nil {
			return "", err
		}

		return finalResp.Choices[0].Content, nil
	}

	// 如果不需要调工具，直接返回它的回复
	return choice.Content, nil
}

// Message 消息（历史记录）
type Message struct {
	Role    string
	Content string
}
