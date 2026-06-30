package tools

import (
	"encoding/json"
	"fmt"

	"aria/pkg/controllerstorage"
)

// ToolFunc 定义工具函数的签名
type ToolFunc func(args string) (string, error)

// Tool 定义一个工具
type Tool struct {
	Name               string
	Description        string
	Parameters         map[string]interface{} // OpenAI Function Calling 参数定义
	RequiredPermission string
	TenantScoped       bool
	Run                ToolFunc
}

// NewListNodesToolWithStore 创建一个查询节点工具（使用真实数据）
func NewListNodesToolWithStore(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "list_nodes",
		Description:        "查询当前系统中所有 WireGuard 节点的列表、状态和 IP 地址。支持按区域过滤，如只查询北京区域节点",
		RequiredPermission: "nodes:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"region": map[string]interface{}{
					"type":        "string",
					"description": "区域名称，如 'bj'（北京）或 'sh'（上海）。如果不指定，返回所有节点",
					"enum":        []string{"bj", "sh"},
				},
			},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			filterRegion := ""
			if err := json.Unmarshal([]byte(args), &req); err == nil {
				if region, ok := req["region"].(string); ok {
					filterRegion = region
					fmt.Printf("[Tool:list_nodes] Filter by region: %s\n", filterRegion)
				}
			}
			tenantID, tenantScoped, tenantErr := parseOptionalTenantID(req)
			if tenantErr != nil {
				return "", tenantErr
			}

			// 从真实数据库获取节点
			fmt.Printf("[Tool:list_nodes] Calling store.GetAllNodes()...\n")
			nodes, err := store.GetAllNodes()
			if err != nil {
				fmt.Printf("[Tool:list_nodes] Error: %v\n", err)
				return "", fmt.Errorf("查询节点失败: %v", err)
			}
			fmt.Printf("[Tool:list_nodes] Got %d nodes from database\n", len(nodes))
			nodes = filterNodesByTenant(nodes, tenantID, tenantScoped)

			// 如果指定了区域，过滤节点
			var filteredNodes []*controllerstorage.Node
			if filterRegion != "" {
				for _, node := range nodes {
					if node.Region == filterRegion {
						filteredNodes = append(filteredNodes, node)
					}
				}
				fmt.Printf("[Tool:list_nodes] Filtered to %d nodes in region %s\n", len(filteredNodes), filterRegion)
				nodes = filteredNodes
			}

			// 转换为易读格式（用于飞书卡片）
			nodeInfos := make([]map[string]interface{}, 0, len(nodes))
			for _, node := range nodes {
				// 状态处理：空值默认为 online
				status := "online"
				if node.Status != "" && node.Status != "online" {
					status = node.Status
				}

				// 获取收敛状态
				convergenceStatus := "unknown"
				lastSyncError := ""
				if controlState, err := store.GetNodeControlState(node.TenantID, node.ID); err == nil && controlState != nil {
					isOnline := status == "online"
					convergenceStatus = string(controlState.GetConvergenceStatus(isOnline))
					lastSyncError = controlState.LastSyncError
				}

				nodeInfo := map[string]interface{}{
					"name":               node.Hostname,
					"public_key":         node.PublicKey,
					"endpoint":           node.Endpoint,
					"assigned_ip":        node.AssignedIP,
					"region":             node.Region,
					"status":             status,
					"convergence_status": convergenceStatus,
					"role":               node.Role,
					"last_seen":          node.LastSeen,
				}
				if lastSyncError != "" {
					nodeInfo["last_sync_error"] = lastSyncError
				}
				if node.PublicIP != "" {
					nodeInfo["public_ip"] = node.PublicIP
				}
				nodeInfos = append(nodeInfos, nodeInfo)
				fmt.Printf("[Tool:list_nodes] Node: %s (%s) - %s\n", node.Hostname, node.AssignedIP, status)
			}

			// 如果没有节点，返回友好的提示信息
			if len(nodeInfos) == 0 {
				fmt.Printf("[Tool:list_nodes] No nodes found\n")
				// 返回统计信息而不是空数组
				stats := map[string]interface{}{
					"title":         "节点列表",
					"total_nodes":   0,
					"online_nodes":  0,
					"offline_nodes": 0,
					"message":       "当前没有注册的节点。请使用 Token 将 Agent 注册到 Controller。",
					"by_region":     make(map[string]int),
					"by_status":     make(map[string]int),
				}
				data, err := json.Marshal(stats)
				if err != nil {
					return "", fmt.Errorf("数据序列化失败: %v", err)
				}
				return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
			}

			// 返回 JSON 数据，标记为卡片格式
			data, err := json.Marshal(nodeInfos)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			fmt.Printf("[Tool:list_nodes] Result JSON: %s\n", string(data))
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// NewGetNodeDetailTool 创建一个查询节点详情工具（支持参数）
func NewGetNodeDetailTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "get_node_detail",
		Description:        "根据节点名称查询节点的详细信息，包括 IP、地区、状态、配置路由等。hostname 在多个租户中重复时需提供 tenant_id。",
		RequiredPermission: "nodes:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"node_name": map[string]interface{}{
					"type":        "string",
					"description": "节点的主机名",
				},
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "租户 ID。hostname 在多个租户中重复时必须提供。",
				},
			},
			"required": []string{"node_name"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			nodeName, ok := req["node_name"].(string)
			if !ok || nodeName == "" {
				return "", fmt.Errorf("缺少必需参数: node_name")
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}

			// 查询节点
			node, matchCount, err := findUniqueNodeByHostnameForScope(store, nodeName, tenantID, tenantScoped)
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}
			if matchCount > 1 {
				return "", fmt.Errorf("节点名称 [%s] 在多个租户中重复，请提供 tenant_id", nodeName)
			}
			if node == nil {
				return fmt.Sprintf("节点 [%s] 不存在", nodeName), nil
			}

			// 转换为易读格式
			nodeInfo := map[string]interface{}{
				"name":              node.Hostname,
				"tenant_id":         node.TenantID.String(),
				"public_key":        node.PublicKey,
				"endpoint":          node.Endpoint,
				"assigned_ip":       node.AssignedIP,
				"private_ip":        node.PrivateIP,
				"public_ip":         node.PublicIP,
				"region":            node.Region,
				"vpc_id":            node.VPCID,
				"role":              node.Role,
				"runtime_mode":      node.RuntimeMode,
				"kernel_version":    node.KernelVersion,
				"status":            node.Status,
				"last_seen":         node.LastSeen,
				"advertised_routes": node.AdvertisedRoutes,
				"registered_at":     node.RegisteredAt,
				"created_at":        node.CreatedAt,
				"updated_at":        node.UpdatedAt,
			}

			data, err := json.Marshal(nodeInfo)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}
