package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"aria/pkg/controllerstorage"
)

// NewDiagnoseConnectivityTool 创建连接诊断工具
func NewDiagnoseConnectivityTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:        "diagnose_connectivity",
		Description: "诊断两个节点之间的连接问题。分析 ACL 规则、版本收敛状态和在线情况。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source_hostname": map[string]interface{}{
					"type":        "string",
					"description": "源节点主机名",
				},
				"target_hostname": map[string]interface{}{
					"type":        "string",
					"description": "目标节点主机名",
				},
			},
			"required": []string{"source_hostname", "target_hostname"},
		},
		Run: func(args string) (string, error) {
			var req struct {
				SourceHostname string `json:"source_hostname"`
				TargetHostname string `json:"target_hostname"`
			}
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("参数解析失败: %v", err)
			}

			// 查找节点
			allNodes, err := store.GetAllNodes()
			if err != nil {
				return "", fmt.Errorf("查询节点失败: %v", err)
			}

			var srcNode, dstNode *controllerstorage.Node
			for _, n := range allNodes {
				if n.Hostname == req.SourceHostname {
					srcNode = n
				}
				if n.Hostname == req.TargetHostname {
					dstNode = n
				}
			}

			if srcNode == nil {
				return fmt.Sprintf("未找到源节点: %s", req.SourceHostname), nil
			}
			if dstNode == nil {
				return fmt.Sprintf("未找到目标节点: %s", req.TargetHostname), nil
			}

			if srcNode.TenantID != dstNode.TenantID {
				return "诊断失败：两个节点属于不同的租户，默认无法互通。", nil
			}

			results := []string{fmt.Sprintf("--- 诊断报告: %s -> %s ---", srcNode.Hostname, dstNode.Hostname)}

			// 1. 在线状态检查
			isSrcOnline := time.Now().Unix()-srcNode.LastSeen <= 60
			isDstOnline := time.Now().Unix()-dstNode.LastSeen <= 60
			
			if !isSrcOnline {
				results = append(results, fmt.Sprintf("❌ 源节点 [%s] 离线 (最后上线: %v)", srcNode.Hostname, time.Unix(srcNode.LastSeen, 0)))
			} else {
				results = append(results, fmt.Sprintf("✅ 源节点 [%s] 在线", srcNode.Hostname))
			}

			if !isDstOnline {
				results = append(results, fmt.Sprintf("❌ 目标节点 [%s] 离线 (最后上线: %v)", dstNode.Hostname, time.Unix(dstNode.LastSeen, 0)))
			} else {
				results = append(results, fmt.Sprintf("✅ 目标节点 [%s] 在线", dstNode.Hostname))
			}

			// 2. 收敛状态检查
			srcState, _ := store.GetNodeControlState(srcNode.TenantID, srcNode.ID)
			dstState, _ := store.GetNodeControlState(dstNode.TenantID, dstNode.ID)

			if srcState != nil {
				status := srcState.GetConvergenceStatus(isSrcOnline)
				if status != controllerstorage.StatusConverged {
					results = append(results, fmt.Sprintf("⚠️ 源节点配置未完全收敛 (%s): %s", status, srcState.LastSyncError))
				} else {
					results = append(results, "✅ 源节点配置已收敛")
				}
			}

			if dstState != nil {
				status := dstState.GetConvergenceStatus(isDstOnline)
				if status != controllerstorage.StatusConverged {
					results = append(results, fmt.Sprintf("⚠️ 目标节点配置未完全收敛 (%s): %s", status, dstState.LastSyncError))
				} else {
					results = append(results, "✅ 目标节点配置已收敛")
				}
			}

			// 3. ACL 规则检查
			aclRules, _ := store.GetACLRulesByTenant(srcNode.TenantID)
			blocked := false
			for _, rule := range aclRules {
				if !rule.Enabled || rule.Action != "deny" {
					continue
				}
				// 简单的 IP/CIDR 匹配逻辑（简化版）
				if rule.SourceID == srcNode.ID.String() && rule.DestinationID == dstNode.ID.String() {
					blocked = true
					results = append(results, fmt.Sprintf("❌ 发现拦截规则: ACL ID %s 禁止了从 %s 到 %s 的访问", rule.ID.String(), srcNode.Hostname, dstNode.Hostname))
					break
				}
			}
			if !blocked {
				results = append(results, "✅ 未发现拦截此路径的 ACL 规则")
			}

			// 4. 总结建议
			if isSrcOnline && isDstOnline && !blocked {
				results = append(results, "\n结论: 控制平面配置正常。如果仍无法通信，请检查节点本地防火墙（iptables/nftables）或底层云网络安全组设置。")
			} else {
				results = append(results, "\n建议: 请先解决上述标记为 ❌ 或 ⚠️ 的问题。")
			}

			data, _ := json.Marshal(results)
			return string(data), nil
		},
	}
}
