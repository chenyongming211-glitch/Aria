package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"aria/internal/agent/brain"
	"aria/internal/agent/tools"
	"aria/pkg/controllerstorage"
)

// AIService 定义 AI 相关的业务接口
type AIService interface {
	Chat(ctx context.Context, sessionID, prompt string) (string, error)
	ChatWithContext(ctx context.Context, chatID, prompt string) (string, error)
	ExecuteTool(ctx context.Context, sessionID, toolName string, params map[string]any, confirmed bool) (map[string]any, error)
}

type aiServiceImpl struct {
	agent     *brain.Agent
	history   sync.Map // chatID -> []brain.Message
	maxTokens int      // 历史记录限制
}

func NewAIService(store *controllerstorage.Storage) AIService {
	myAgent := brain.NewAgent("Aria Assistant")
	myAgent.SetSystemPrompt(`你是一个 Aria SD-WAN 网络专家助手。你可以协助用户管理网络节点、配置防火墙规则、QoS策略和路由。`)

	// 注册工具
	// 节点管理
	myAgent.RegisterTool(tools.NewListNodesToolWithStore(store))
	myAgent.RegisterTool(tools.NewGetNodeTool(store))
	myAgent.RegisterTool(tools.NewUpdateNodeTool(store))

	// 令牌管理
	myAgent.RegisterTool(tools.NewListTokensTool(store))
	myAgent.RegisterTool(tools.NewCreateTokenTool(store))
	myAgent.RegisterTool(tools.NewRevokeTokenTool(store))

	// 策略管理（ACL）
	myAgent.RegisterTool(tools.NewListPoliciesTool(store))
	myAgent.RegisterTool(tools.NewCreatePolicyTool(store))
	myAgent.RegisterTool(tools.NewDeletePolicyTool(store))

	// 监控与诊断
	myAgent.RegisterTool(tools.NewGetMonitorStatsTool(store))
	myAgent.RegisterTool(tools.NewDiagnoseConnectivityTool(store))

	return &aiServiceImpl{
		agent:     myAgent,
		maxTokens: 20, // ✅ 修复 BUG-14：初始化最大上下文令牌数
	}
}

func (s *aiServiceImpl) Chat(ctx context.Context, sessionID, prompt string) (string, error) {
	return s.agent.Chat(ctx, prompt)
}

func (s *aiServiceImpl) ChatWithContext(ctx context.Context, chatID, prompt string) (string, error) {
	// 获取历史记录
	var history []brain.Message
	if val, ok := s.history.Load(chatID); ok {
		history = val.([]brain.Message)
	}

	// 执行对话
	answer, err := s.agent.ChatWithHistory(ctx, prompt, history)
	if err != nil {
		return "", err
	}

	// 更新并保存历史记录
	if chatID != "" {
		// 添加用户消息
		history = append(history, brain.Message{
			Role:    "user",
			Content: prompt,
		})

		// 添加 AI 回复
		history = append(history, brain.Message{
			Role:    "assistant",
			Content: answer,
		})

		// 限制历史记录长度
		if len(history) > s.maxTokens {
			history = history[len(history)-s.maxTokens:]
		}

		s.history.Store(chatID, history)
	}

	return answer, nil
}

// ExecuteTool 执行工具（用于前端确认后执行）
func (s *aiServiceImpl) ExecuteTool(ctx context.Context, sessionID, toolName string, params map[string]any, confirmed bool) (map[string]any, error) {
	fmt.Printf("[AIService] ExecuteTool: sessionID=%s, toolName=%s, confirmed=%v\n", sessionID, toolName, confirmed)

	// 查找工具
	var selectedTool tools.Tool
	for _, t := range s.agent.GetTools() {
		if t.Name == toolName {
			selectedTool = t
			break
		}
	}

	if selectedTool.Name == "" {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// 序列化参数
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// 执行工具
	result, err := selectedTool.Run(string(paramsJSON))
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// 返回结果
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": result},
		},
		"tool_name": toolName,
	}, nil
}
