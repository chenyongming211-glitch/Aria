package tools

import (
	"encoding/json"
	"fmt"

	"aria/pkg/controllerstorage"
)

// NewListPoliciesTool 创建查询策略列表工具
func NewListPoliciesTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:        "list_policies",
		Description: "查询 ACL 访问控制策略列表（防火墙规则）。支持按区域或节点过滤。参数：region（区域名称，如 bj/sh）、hostname（节点名称）",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"region": map[string]interface{}{
					"type":        "string",
					"description": "区域名称，如 'bj'（北京）或 'sh'（上海）。如果不指定，返回所有区域的策略",
					"enum":        []string{"bj", "sh"},
				},
				"hostname": map[string]interface{}{
					"type":        "string",
					"description": "节点名称，如 'VM-0-4-ubuntu'。如果不指定，返回所有节点的策略",
				},
			},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			filterRegion := ""
			filterHostname := ""
			if err := json.Unmarshal([]byte(args), &req); err == nil {
				if region, ok := req["region"].(string); ok {
					filterRegion = region
					fmt.Printf("[Tool:list_policies] Filter by region: %s\n", filterRegion)
				}
				if hostname, ok := req["hostname"].(string); ok {
					filterHostname = hostname
					fmt.Printf("[Tool:list_policies] Filter by hostname: %s\n", filterHostname)
				}
			}

			// 查询所有规则
			allRules, err := store.GetAllACLRules()
			if err != nil {
				return "", fmt.Errorf("查询策略失败: %v", err)
			}

			// 查询所有节点，用于获取节点所属区域
			allNodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}

			// 构建节点名称到区域的映射
			nodeRegionMap := make(map[string]string)
			for _, node := range allNodes {
				nodeRegionMap[node.Hostname] = node.Region
			}

			// 过滤规则
			filteredRules := make([]*controllerstorage.ACLRule, 0, len(allRules))
			for _, rule := range allRules {
				include := true

				// 按区域过滤
				if filterRegion != "" {
					srcRegion, srcOk := nodeRegionMap[rule.SrcNode]
					dstRegion, dstOk := nodeRegionMap[rule.DstNode]
					// 只有源或目标在指定区域才包含
					if srcOk && srcRegion != filterRegion && dstOk && dstRegion != filterRegion {
						include = false
					}
				}

				// 按节点过滤
				if filterHostname != "" && rule.SrcNode != filterHostname && rule.DstNode != filterHostname {
					include = false
				}

				if include {
					filteredRules = append(filteredRules, rule)
				}
			}

			fmt.Printf("[Tool:list_policies] Filtered %d rules from %d total\n", len(filteredRules), len(allRules))

			data, err := json.Marshal(filteredRules)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}

// NewCreatePolicyTool 创建策略工具
func NewCreatePolicyTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:        "create_policy",
		Description: "创建新的 ACL 策略。参数：src_node（源节点名）、src_ip（源 IP/CIDR）、dst_node（目标节点名）、dst_ip（目标 IP/CIDR）、protocol（tcp/udp/icmp）、dst_port（端口，范围如 '22' 或 '22-80'）、action（allow/deny）",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"src_node": map[string]interface{}{
					"type":        "string",
					"description": "源节点名称",
				},
				"src_ip": map[string]interface{}{
					"type":        "string",
					"description": "源 IP 地址或 CIDR（如 192.168.1.0/24）",
				},
				"dst_node": map[string]interface{}{
					"type":        "string",
					"description": "目标节点名称",
				},
				"dst_ip": map[string]interface{}{
					"type":        "string",
					"description": "目标 IP 地址或 CIDR（如 10.0.0.1/32）",
				},
				"protocol": map[string]interface{}{
					"type":        "string",
					"description": "协议类型：tcp、udp 或 icmp",
				},
				"dst_port": map[string]interface{}{
					"type":        "string",
					"description": "目标端口，可以是单端口（如 '80'）或范围（如 '80-443'），留空表示所有端口",
				},
				"action": map[string]interface{}{
					"type":        "string",
					"description": "动作：allow 或 deny",
				},
			},
			"required": []string{"src_ip", "dst_ip", "action"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			srcIp, _ := req["src_ip"].(string)
			dstIp, _ := req["dst_ip"].(string)
			action, _ := req["action"].(string)
			srcNode, _ := req["src_node"].(string)
			dstNode, _ := req["dst_node"].(string)
			protocol, _ := req["protocol"].(string)
			dstPort, _ := req["dst_port"].(string)

			if srcIp == "" || dstIp == "" || action == "" {
				return "", fmt.Errorf("缺少必需参数: src_ip, dst_ip, action")
			}

			if action != "allow" && action != "deny" {
				return "", fmt.Errorf("action 必须是 'allow' 或 'deny'")
			}

			// 解析端口范围
			minPort, maxPort := parsePortRange(dstPort)

			// 解析协议
			var protocolNum uint8 = 6 // 默认 TCP
			switch protocol {
			case "udp":
				protocolNum = 17
			case "icmp":
				protocolNum = 1
			}

			rule := &controllerstorage.ACLRule{
				SrcNode:  srcNode,
				SrcNet:   srcIp,
				DstNode:  dstNode,
				DstNet:   dstIp,
				Protocol: protocolNum,
				MinPort:  minPort,
				MaxPort:  maxPort,
				Action:   action,
				Enabled:  true,
				Priority: 100,
			}

			if err := store.SaveACLRule(rule); err != nil {
				return "", fmt.Errorf("创建策略失败: %v", err)
			}

			result := map[string]interface{}{
				"id":       rule.ID,
				"src_node": rule.SrcNode,
				"src_ip":   rule.SrcNet,
				"dst_node": rule.DstNode,
				"dst_ip":   rule.DstNet,
				"protocol": protocol,
				"dst_port": dstPort,
				"action":   rule.Action,
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

// NewDeletePolicyTool 创建删除策略工具
func NewDeletePolicyTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:        "delete_policy",
		Description: "根据策略 ID 删除 ACL 策略",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "策略 ID",
				},
			},
			"required": []string{"id"},
		},
		Run: func(args string) (string, error) {
			// 解析参数
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			idFloat, ok := req["id"].(float64)
			if !ok {
				return "", fmt.Errorf("缺少必需参数: id")
			}

			id := int(idFloat)

			if err := store.DeleteACLRule(id); err != nil {
				return "", fmt.Errorf("删除策略失败: %v", err)
			}

			return fmt.Sprintf("策略 ID %d 删除成功", id), nil
		},
	}
}

// parsePortRange 解析端口范围
func parsePortRange(portStr string) (uint16, uint16) {
	if portStr == "" {
		return 0, 65535
	}

	// 处理单端口或端口范围
	var minPort, maxPort uint16
	n, _ := fmt.Sscanf(portStr, "%d-%d", &minPort, &maxPort)
	if n == 2 {
		return minPort, maxPort
	}

	n, _ = fmt.Sscanf(portStr, "%d", &minPort)
	if n == 1 {
		return minPort, minPort
	}

	return 0, 65535
}
