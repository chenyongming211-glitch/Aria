package tools

import (
	"encoding/json"
	"fmt"
	"net"

	"aria/pkg/controllerstorage"
)

// NewAddRouteTool 创建添加路由工具
func NewAddRouteTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "add_route",
		Description:        "为指定节点添加路由（Site-to-Site VPN）。参数：hostname（节点名称）、cidr（网段，如 192.168.1.0/24）、tenant_id（可选，租户 ID）",
		RequiredPermission: "routes:write",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "节点名称（如 beijing-01）",
				},
				"cidr": map[string]interface{}{
					"type":        "string",
					"description": "要添加的网段 CIDR（如 192.168.1.0/24）",
				},
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "租户 ID。hostname 在多个租户中重复时必须提供。",
				},
			},
			"required": []string{"hostname", "cidr"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			hostname, ok := req["hostname"].(string)
			if !ok || hostname == "" {
				return "", fmt.Errorf("缺少必需参数: hostname")
			}

			cidr, ok := req["cidr"].(string)
			if !ok || cidr == "" {
				return "", fmt.Errorf("缺少必需参数: cidr")
			}

			// 验证 CIDR 格式
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return "", fmt.Errorf("无效的 CIDR 格式: %s", cidr)
			}

			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}

			// 查找节点
			allNodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}

			targetNode, matchCount := findUniqueNodeByHostnameInNodesForScope(allNodes, hostname, tenantID, tenantScoped)
			if matchCount > 1 {
				return "", fmt.Errorf("节点名称 [%s] 在多个租户中重复，请提供 tenant_id", hostname)
			}
			if targetNode == nil {
				return fmt.Sprintf("节点 [%s] 不存在", hostname), nil
			}
			scopedNodes := filterNodesByTenant(allNodes, tenantID, tenantScoped)

			// 检查是否已存在
			for _, route := range targetNode.AdvertisedRoutes {
				if route == cidr {
					return fmt.Sprintf("路由 %s 已存在于节点 %s", cidr, hostname), nil
				}
			}

			// 检查跨 Region 冲突
			for _, node := range scopedNodes {
				if node.Region == targetNode.Region || node.PublicKey == targetNode.PublicKey {
					continue
				}

				for _, existingRoute := range node.AdvertisedRoutes {
					_, newNetwork, err := net.ParseCIDR(cidr)
					_, existingNetwork, err2 := net.ParseCIDR(existingRoute)
					if err != nil || err2 != nil {
						continue
					}

					if cidrsOverlap(newNetwork, existingNetwork) {
						return fmt.Sprintf("路由 %s 与 Region %s 的节点 %s 冲突（不同 Region 不能有重叠路由）",
							cidr, node.Region, node.Hostname), nil
					}
				}
			}

			// 添加路由
			targetNode.AdvertisedRoutes = append(targetNode.AdvertisedRoutes, cidr)
			if err := store.SaveNode(targetNode); err != nil {
				return "", fmt.Errorf("保存节点失败: %v", err)
			}

			result := map[string]interface{}{
				"hostname":  hostname,
				"tenant_id": targetNode.TenantID.String(),
				"cidr":      cidr,
				"region":    targetNode.Region,
				"added":     true,
				"routes":    targetNode.AdvertisedRoutes,
			}

			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// NewRemoveRouteTool 创建删除路由工具
func NewRemoveRouteTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "remove_route",
		Description:        "从指定节点删除路由。参数：hostname（节点名称）、cidr（网段）、tenant_id（可选，租户 ID）",
		RequiredPermission: "routes:write",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "节点名称",
				},
				"cidr": map[string]interface{}{
					"type":        "string",
					"description": "要删除的网段 CIDR",
				},
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "租户 ID。hostname 在多个租户中重复时必须提供。",
				},
			},
			"required": []string{"hostname", "cidr"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			hostname, ok := req["hostname"].(string)
			if !ok || hostname == "" {
				return "", fmt.Errorf("缺少必需参数: hostname")
			}

			cidr, ok := req["cidr"].(string)
			if !ok || cidr == "" {
				return "", fmt.Errorf("缺少必需参数: cidr")
			}

			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}

			// 查找节点
			allNodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}

			targetNode, matchCount := findUniqueNodeByHostnameInNodesForScope(allNodes, hostname, tenantID, tenantScoped)
			if matchCount > 1 {
				return "", fmt.Errorf("节点名称 [%s] 在多个租户中重复，请提供 tenant_id", hostname)
			}
			if targetNode == nil {
				return fmt.Sprintf("节点 [%s] 不存在", hostname), nil
			}

			// 删除路由
			found := false
			newRoutes := make([]string, 0, len(targetNode.AdvertisedRoutes))
			for _, route := range targetNode.AdvertisedRoutes {
				if route == cidr {
					found = true
					continue
				}
				newRoutes = append(newRoutes, route)
			}

			if !found {
				return fmt.Sprintf("路由 %s 不存在于节点 %s", cidr, hostname), nil
			}

			targetNode.AdvertisedRoutes = newRoutes
			if err := store.SaveNode(targetNode); err != nil {
				return "", fmt.Errorf("保存节点失败: %v", err)
			}

			result := map[string]interface{}{
				"hostname":  hostname,
				"tenant_id": targetNode.TenantID.String(),
				"cidr":      cidr,
				"removed":   true,
				"routes":    targetNode.AdvertisedRoutes,
			}

			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			return string(data), nil
		},
	}
}

// NewGetNodeRoutesTool 创建查询节点路由工具
func NewGetNodeRoutesTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "get_node_routes",
		Description:        "查询指定节点的所有路由（Site-to-Site VPN）。参数：hostname（节点名称）、tenant_id（可选，租户 ID）",
		RequiredPermission: "routes:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "节点名称",
				},
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "租户 ID。hostname 在多个租户中重复时必须提供。",
				},
			},
			"required": []string{"hostname"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			hostname, ok := req["hostname"].(string)
			if !ok || hostname == "" {
				return "", fmt.Errorf("缺少必需参数: hostname")
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}

			// 查找节点
			node, matchCount, err := findUniqueNodeByHostnameForScope(store, hostname, tenantID, tenantScoped)
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}
			if matchCount > 1 {
				return "", fmt.Errorf("节点名称 [%s] 在多个租户中重复，请提供 tenant_id", hostname)
			}
			if node == nil {
				return fmt.Sprintf("节点 [%s] 不存在", hostname), nil
			}

			result := map[string]interface{}{
				"hostname":    hostname,
				"tenant_id":   node.TenantID.String(),
				"region":      node.Region,
				"routes":      node.AdvertisedRoutes,
				"route_count": len(node.AdvertisedRoutes),
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

// cidrsOverlap 检查两个 CIDR 是否重叠
func cidrsOverlap(a, b *net.IPNet) bool {
	if a.Contains(b.IP) {
		return true
	}
	if b.Contains(a.IP) {
		return true
	}
	return false
}

// NewListAllRoutesTool 创建查询所有节点路由工具
func NewListAllRoutesTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:               "list_all_routes",
		Description:        "查询所有节点及其路由情况（Site-to-Site VPN）。支持按区域过滤，如只查询北京（bj）或上海（sh）区域的节点路由",
		RequiredPermission: "routes:read",
		TenantScoped:       true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"region": map[string]interface{}{
					"type":        "string",
					"description": "区域名称，如 'bj'（北京）或 'sh'（上海）。如果不指定，返回所有节点的路由",
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
					fmt.Printf("[Tool:list_all_routes] Filter by region: %s\n", filterRegion)
				}
			}
			tenantID, tenantScoped, err := parseOptionalTenantID(req)
			if err != nil {
				return "", err
			}

			// 查询所有节点
			nodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}
			nodes = filterNodesByTenant(nodes, tenantID, tenantScoped)

			// 如果指定了区域，过滤节点
			var filteredNodes []*controllerstorage.Node
			if filterRegion != "" {
				for _, node := range nodes {
					if node.Region == filterRegion {
						filteredNodes = append(filteredNodes, node)
					}
				}
				fmt.Printf("[Tool:list_all_routes] Filtered to %d nodes in region %s\n", len(filteredNodes), filterRegion)
				nodes = filteredNodes
			}

			// 构建所有节点的路由信息
			nodeRoutes := make([]map[string]interface{}, 0, len(nodes))
			for _, node := range nodes {
				routeInfo := map[string]interface{}{
					"hostname":    node.Hostname,
					"region":      node.Region,
					"routes":      node.AdvertisedRoutes,
					"route_count": len(node.AdvertisedRoutes),
				}
				if node.PublicIP != "" {
					routeInfo["public_ip"] = node.PublicIP
				}
				nodeRoutes = append(nodeRoutes, routeInfo)
				fmt.Printf("[Tool:list_all_routes] Node %s (%s): %d routes\n",
					node.Hostname, node.Region, len(node.AdvertisedRoutes))
			}

			data, err := json.Marshal(nodeRoutes)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}

			fmt.Printf("[Tool:list_all_routes] Returning %d nodes\n", len(nodeRoutes))
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}
