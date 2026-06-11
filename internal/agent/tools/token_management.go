package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aria/internal/token"
	"aria/pkg/controllerstorage"
)

// NewListTokensTool 创建查询 Token 列表工具
func NewListTokensTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "list_tokens",
		Description:        "查询当前租户 Token 的列表，包括 tag、状态、使用次数等信息",
		RequiredPermission: "tokens:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Run: func(args string) (string, error) {
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}
			if !tenantScoped {
				return "", fmt.Errorf("tenant_id is required")
			}

			tokenStore := token.NewStore(store.DB())
			tokens, err := tokenStore.List("")
			if err != nil {
				return "", fmt.Errorf("查询 Token 失败: %v", err)
			}

			// 更新状态
			filtered := make([]*token.Token, 0, len(tokens))
			for _, t := range tokens {
				if t.TenantID != tenantID.String() {
					continue
				}
				if t.Status == token.StatusActive {
					if t.IsExpired() {
						t.Status = token.StatusExpired
					} else if t.UsedCount >= t.MaxUses && t.MaxUses > 0 {
						t.Status = token.StatusExhausted
					}
				}
				filtered = append(filtered, t)
			}

			data, err := json.Marshal(filtered)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// NewGetTokenDetailTool 创建查询 Token 详情工具
func NewGetTokenDetailTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "get_token_detail",
		Description:        "根据 token 查询当前租户内的详细信息，包括哪些节点使用了该 token",
		RequiredPermission: "tokens:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"token": map[string]interface{}{
					"type":        "string",
					"description": "Token 字符串",
				},
			},
			"required": []string{"token"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			tokenStr, ok := req["token"].(string)
			if !ok || tokenStr == "" {
				return "", fmt.Errorf("缺少必需参数: token")
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}
			if !tenantScoped {
				return "", fmt.Errorf("tenant_id is required")
			}

			tokenStore := token.NewStore(store.DB())
			tkn, err := tokenStore.GetByToken(tokenStr)
			if err != nil || tkn == nil {
				return fmt.Sprintf("Token [%s] 不存在", tokenStr), nil
			}
			if tkn.TenantID != tenantID.String() {
				return fmt.Sprintf("Token [%s] 不存在", tokenStr), nil
			}

			// 更新状态
			status := string(tkn.Status)
			if tkn.Status == token.StatusActive {
				if tkn.IsExpired() {
					status = string(token.StatusExpired)
				} else if tkn.UsedCount >= tkn.MaxUses && tkn.MaxUses > 0 {
					status = string(token.StatusExhausted)
				}
			}

			// 获取使用该 token 的节点
			allNodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}
			allNodes = filterNodesByTenant(allNodes, tenantID, tenantScoped)

			var usedByNodes []map[string]interface{}
			for _, node := range allNodes {
				if node.EnrolledWithToken == tokenStr {
					usedByNodes = append(usedByNodes, map[string]interface{}{
						"hostname":    node.Hostname,
						"assigned_ip": node.AssignedIP,
						"region":      node.Region,
						"status":      node.Status,
					})
				}
			}

			result := map[string]interface{}{
				"token":      tkn.Token,
				"tag":        tkn.Tag,
				"max_uses":   tkn.MaxUses,
				"used_count": tkn.UsedCount,
				"status":     status,
				"expires_at": tkn.ExpiresAt.Format(time.RFC3339),
				"created_at": tkn.CreatedAt.Format(time.RFC3339),
				"nodes":      usedByNodes,
			}

			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// NewCreateTokenTool 创建 Token 工具
func NewCreateTokenTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "create_token",
		Description:        "为当前租户创建新的注册 Token。参数：tag（标签）、max_uses（最大使用次数，默认1）、ttl（有效期，默认24小时，如 '1h', '24h', '7d'）",
		RequiredPermission: "tokens:write",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tag": map[string]interface{}{
					"type":        "string",
					"description": "Token 标签，用于标识用途",
				},
				"max_uses": map[string]interface{}{
					"type":        "integer",
					"description": "最大使用次数，默认为 1",
				},
				"ttl": map[string]interface{}{
					"type":        "string",
					"description": "有效期，格式如 '1h', '24h', '7d'，默认为 '24h'",
				},
			},
			"required": []string{"tag"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			tag, ok := req["tag"].(string)
			if !ok || tag == "" {
				return "", fmt.Errorf("缺少必需参数: tag")
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}
			if !tenantScoped {
				return "", fmt.Errorf("tenant_id is required")
			}

			maxUses := 1
			if mu, ok := req["max_uses"].(float64); ok {
				maxUses = int(mu)
			}

			ttl := "24h"
			if t, ok := req["ttl"].(string); ok && t != "" {
				ttl = t
			}

			tokenStore := token.NewStore(store.DB())

			// 生成 token ID
			newToken := generateRandomToken()
			tokenDuration := parseTTL(ttl)

			tkn := &token.Token{
				Token:     newToken,
				Tag:       tag,
				TenantID:  tenantID.String(),
				MaxUses:   maxUses,
				UsedCount: 0,
				ExpiresAt: time.Now().Add(tokenDuration),
				CreatedAt: time.Now(),
				Status:    token.StatusActive,
			}

			if err := tokenStore.Create(tkn); err != nil {
				return "", fmt.Errorf("创建 Token 失败: %v", err)
			}

			result := map[string]interface{}{
				"token":      tkn.Token,
				"tag":        tkn.Tag,
				"max_uses":   tkn.MaxUses,
				"status":     tkn.Status,
				"expires_at": tkn.ExpiresAt.Format(time.RFC3339),
				"created_at": tkn.CreatedAt.Format(time.RFC3339),
			}

			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// generateRandomToken 生成随机 token
func generateRandomToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return "tk_" + string(b)
}

// parseTTL 解析 TTL 字符串
func parseTTL(ttl string) time.Duration {
	if ttl == "" {
		return 24 * time.Hour
	}

	var multiplier time.Duration
	var value int

	if len(ttl) < 2 {
		return 24 * time.Hour
	}

	unit := strings.ToLower(ttl[len(ttl)-1:])
	fmt.Sscanf(ttl[:len(ttl)-1], "%d", &value)

	switch unit {
	case "h":
		multiplier = time.Hour
	case "d":
		multiplier = 24 * time.Hour
	case "m":
		multiplier = time.Minute
	default:
		return 24 * time.Hour
	}

	return time.Duration(value) * multiplier
}
