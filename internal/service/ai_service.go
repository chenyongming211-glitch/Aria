package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aria/internal/agent/brain"
	"aria/internal/agent/tools"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// AIService 定义 AI 相关的业务接口
type AIService interface {
	Chat(ctx context.Context, sessionID, prompt string) (string, error)
	ChatWithContext(ctx context.Context, chatID, prompt string) (string, error)
	ExecuteTool(ctx context.Context, sessionID, toolName string, params map[string]any, confirmed bool) (map[string]any, error)
}

// aiServiceImpl 是具体实现
type aiServiceImpl struct {
	agent     *brain.Agent
	store     *controllerstorage.Storage
	history   sync.Map // chatID -> []brain.Message
	maxTokens int      // 历史记录限制
}

// NewAIService 初始化 AI 服务（依赖注入的构造函数）
func NewAIService(store *controllerstorage.Storage) AIService {
	myAgent := brain.NewAgent()

	// 注册工具
	// 节点管理
	myAgent.RegisterTool(tools.NewListNodesToolWithStore(store))
	myAgent.RegisterTool(tools.NewGetNodeDetailTool(store))

	// 网络配置
	myAgent.RegisterTool(tools.NewGetNetworkConfigTool(store))

	// 令牌管理
	myAgent.RegisterTool(tools.NewListTokensTool(store))
	myAgent.RegisterTool(tools.NewGetTokenDetailTool(store))
	myAgent.RegisterTool(tools.NewCreateTokenTool(store))

	// 路由管理
	myAgent.RegisterTool(tools.NewAddRouteTool(store))
	myAgent.RegisterTool(tools.NewRemoveRouteTool(store))
	myAgent.RegisterTool(tools.NewGetNodeRoutesTool(store))
	myAgent.RegisterTool(tools.NewListAllRoutesTool(store))

	// 监控与诊断
	myAgent.RegisterTool(tools.NewGetMonitorStatsTool(store))
	myAgent.RegisterTool(tools.NewDiagnoseConnectivityTool(store))

	return &aiServiceImpl{
		agent:     myAgent,
		store:     store,
		maxTokens: 20,
	}
}

func (s *aiServiceImpl) Chat(ctx context.Context, sessionID, prompt string) (string, error) {
	return s.agent.ThinkWithTools(ctx, prompt, s.scopedTools(ctx))
}

// ChatWithContext 带上下文的聊天（用于飞书等需要会话的场景）
func (s *aiServiceImpl) ChatWithContext(ctx context.Context, chatID, prompt string) (string, error) {
	// 获取历史记录
	var history []brain.Message
	if val, ok := s.history.Load(chatID); ok {
		history = val.([]brain.Message)
	}

	// 执行对话
	answer, err := s.agent.ThinkWithHistoryTools(ctx, prompt, history, s.scopedTools(ctx))
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
	for _, t := range s.scopedTools(ctx) {
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

func (s *aiServiceImpl) scopedTools(ctx context.Context) []tools.Tool {
	rawTools := s.agent.GetTools()
	scoped := make([]tools.Tool, 0, len(rawTools))
	for _, raw := range rawTools {
		toolCopy := raw
		run := raw.Run
		toolCopy.Run = func(args string) (string, error) {
			return s.runScopedTool(ctx, toolCopy, run, args)
		}
		scoped = append(scoped, toolCopy)
	}
	return scoped
}

func (s *aiServiceImpl) runScopedTool(ctx context.Context, tool tools.Tool, run tools.ToolFunc, args string) (string, error) {
	if err := s.authorizeTool(ctx, tool.RequiredPermission); err != nil {
		return "", err
	}

	if tool.TenantScoped {
		tenantID, ok := middleware.GetTenantID(ctx)
		if !ok || tenantID == uuid.Nil {
			return "", errors.New("tenant context is required for this AI tool")
		}
		scopedArgs, err := injectTenantIDArg(args, tenantID)
		if err != nil {
			return "", err
		}
		args = scopedArgs
	}

	return run(args)
}

func (s *aiServiceImpl) authorizeTool(ctx context.Context, permission string) error {
	if permission == "" {
		return nil
	}

	role, ok := middleware.GetUserRole(ctx)
	if !ok {
		return errors.New("user role is required for AI tool execution")
	}
	roleName := controllerstorage.NormalizeRoleName(role)
	if roleName == "super_admin" {
		return nil
	}

	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok || tenantID == uuid.Nil {
		return errors.New("tenant context is required for AI tool authorization")
	}

	permissions, err := s.store.GetRolePermissions(tenantID, roleName)
	if err != nil {
		return fmt.Errorf("AI tool permission lookup failed: %w", err)
	}
	if !containsPermission(permissions, permission) {
		return fmt.Errorf("AI tool requires permission %s", permission)
	}
	return nil
}

func injectTenantIDArg(args string, tenantID uuid.UUID) (string, error) {
	params := map[string]interface{}{}
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", fmt.Errorf("invalid tool params: %w", err)
		}
	}
	params["tenant_id"] = tenantID.String()
	out, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("invalid tool params: %w", err)
	}
	return string(out), nil
}

func containsPermission(permissions []string, permission string) bool {
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}
