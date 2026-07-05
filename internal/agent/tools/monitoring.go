package tools

import (
	"encoding/json"
	"fmt"

	"aria/pkg/controllerstorage"
)

// NewGetMonitorStatsTool 创建查询监控统计工具
func NewGetMonitorStatsTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "get_monitor_stats",
		Description:        "查询当前租户监控统计信息，包括节点总数、在线节点数、离线节点数、按 Region 分组统计等",
		RequiredPermission: "monitoring:read",
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

			nodes, err := listNodesForToolScope(store, tenantID, tenantScoped)
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}

			// 统计数据
			stats := map[string]interface{}{
				"total_nodes":   len(nodes),
				"online_nodes":  0,
				"offline_nodes": 0,
				"by_region":     make(map[string]int),
				"by_status":     make(map[string]int),
			}

			byRegion := stats["by_region"].(map[string]int)
			byStatus := stats["by_status"].(map[string]int)

			for _, node := range nodes {
				status := node.Status
				if status == "" {
					status = "online"
				}

				// 状态统计
				byStatus[status]++
				if status == "online" {
					stats["online_nodes"] = stats["online_nodes"].(int) + 1
				} else if status == "offline" {
					stats["offline_nodes"] = stats["offline_nodes"].(int) + 1
				}

				// Region 统计
				if node.Region != "" {
					byRegion[node.Region]++
				}
			}

			data, err := json.Marshal(stats)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}
