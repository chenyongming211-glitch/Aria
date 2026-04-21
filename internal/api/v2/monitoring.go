package v2

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// handleMonitoringStats returns tenant-level monitoring statistics.
// GET /api/v2/tenants/{tenant_id}/monitoring/stats
func (r *Router) handleMonitoringStats(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	totalNodes, onlineNodes, offlineNodes, err := r.store.CountNodesByTenantAndStatus(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to count nodes: "+err.Error(), nil)
		return
	}

	syncRate, err := r.store.CalcSyncSuccessRate(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to calculate sync rate: "+err.Error(), nil)
		return
	}

	aclCount, err := r.store.CountACLRulesByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to count ACL rules: "+err.Error(), nil)
		return
	}

	qosCount, err := r.store.CountQoSRulesByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to count QoS rules: "+err.Error(), nil)
		return
	}

	failedCmds, err := r.store.CountFailedCommandsByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to count failed commands: "+err.Error(), nil)
		return
	}

	activeAlerts, err := r.store.CountActiveAlerts(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to count active alerts: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"total_nodes":           totalNodes,
		"online_nodes":          onlineNodes,
		"offline_nodes":         offlineNodes,
		"sync_success_rate":     syncRate,
		"total_peers":           onlineNodes,
		"total_acl_rules":       aclCount,
		"total_qos_rules":       qosCount,
		"failed_commands_count": failedCmds,
		"active_alerts_count":   activeAlerts,
	}, "Monitoring stats retrieved")
}

// handleMonitoringNodeDetail returns detailed control state for a single node.
// GET /api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}
func (r *Router) handleMonitoringNodeDetail(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "Node not found", nil)
			return
		}
		r.writeNodeLookupError(w, err)
		return
	}

	controlState, err := r.store.GetNodeControlState(tenantID, node.ID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to get control state: "+err.Error(), nil)
		return
	}

	data := map[string]interface{}{
		"node_id":             node.ID.String(),
		"hostname":            node.Hostname,
		"availability_status": nodeAvailabilityStatus(node),
	}

	if controlState != nil {
		data["desired_state_version"] = controlState.DesiredStateVersion
		data["applied_state_version"] = controlState.AppliedStateVersion
		data["observed_state"] = controlState.ObservedState
		data["observed_message"] = controlState.ObservedMessage
		data["last_sync_at"] = controlState.LastSyncAt
		data["last_sync_error"] = controlState.LastSyncError
		data["state_convergence"] = computeStateConvergence(controlState)
	} else {
		data["desired_state_version"] = ""
		data["applied_state_version"] = ""
		data["observed_state"] = ""
		data["observed_message"] = ""
		data["last_sync_at"] = nil
		data["last_sync_error"] = ""
		data["state_convergence"] = "idle"
	}

	// Recent agent commands (up to 20)
	commands, err := r.store.ListRecentAgentCommands(node.PublicKey, 20)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list commands: "+err.Error(), nil)
		return
	}
	cmdItems := make([]map[string]interface{}, 0, len(commands))
	for _, cmd := range commands {
		cmdItems = append(cmdItems, agentCommandToMap(cmd))
	}
	data["recent_commands"] = cmdItems

	// Recent policy deliveries (up to 20)
	deliveries, err := r.store.ListRecentPolicyDeliveriesByNode(tenantID, node.ID, 20)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list policy deliveries: "+err.Error(), nil)
		return
	}
	pdItems := make([]map[string]interface{}, 0, len(deliveries))
	for _, pd := range deliveries {
		pdItems = append(pdItems, policyDeliveryToMap(pd))
	}
	data["recent_policy_deliveries"] = pdItems

	alerts, _, err := r.store.ListAlerts(tenantID, controllerstorage.AlertFilter{
		Status: "active",
		NodeID: &node.ID,
		Limit:  10,
	})
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to list active alerts: "+err.Error(), nil)
		return
	}
	data["active_alerts"] = alerts

	// 计算已学习的路由 (Learned Routes)
	// 在 Mesh 网络中，本节点会学习到同租户其他所有节点宣告的路由
	tenantNodes, err := r.store.GetNodesByTenant(tenantID)
	if err == nil {
		learnedRoutes := make([]map[string]interface{}, 0)
		for _, tn := range tenantNodes {
			if tn.ID == node.ID {
				continue // 跳过自己
			}
			for _, route := range tn.AdvertisedRoutes {
				learnedRoutes = append(learnedRoutes, map[string]interface{}{
					"cidr":           route,
					"next_hop_node":  tn.Hostname,
					"next_hop_ip":    tn.AssignedIP,
					"region":         tn.Region,
					"status":         nodeAvailabilityStatus(tn),
				})
			}
		}
		data["learned_routes"] = learnedRoutes
	}

	apibase.WriteSuccess(w, data, "Node monitoring detail retrieved")
}

// handleMonitoringEvents returns a unified event feed of alerts and audit events.
// GET /api/v2/tenants/{tenant_id}/monitoring/events
func (r *Router) handleMonitoringEvents(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	query := req.URL.Query()

	limit := 50
	if raw := query.Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
			if limit > 200 {
				limit = 200
			}
		}
	}

	offset := 0
	if raw := query.Get("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	filter := controllerstorage.EventFeedFilter{
		EventType: query.Get("event_type"),
		Severity:  query.Get("severity"),
		Limit:     limit,
		Offset:    offset,
	}

	if nodeIDStr := query.Get("node_id"); nodeIDStr != "" {
		parsed, err := uuid.Parse(nodeIDStr)
		if err != nil {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid node_id format", nil)
			return
		}
		filter.NodeID = &parsed
	}

	if sinceStr := query.Get("since"); sinceStr != "" {
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid since format, use ISO 8601", nil)
			return
		}
		filter.Since = &parsed
	}

	items, total, err := r.store.ListEventFeed(tenantID, filter)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to query events: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}, "Events retrieved")
}

// handleMonitoringAlerts returns the alerts list for a tenant.
// GET /api/v2/tenants/{tenant_id}/monitoring/alerts
func (r *Router) handleMonitoringAlerts(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	query := req.URL.Query()

	status := query.Get("status")
	if status == "" {
		status = "active"
	}

	limit := 50
	if raw := query.Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
			if limit > 200 {
				limit = 200
			}
		}
	}

	offset := 0
	if raw := query.Get("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	filter := controllerstorage.AlertFilter{
		Status:    status,
		AlertType: query.Get("alert_type"),
		Limit:     limit,
		Offset:    offset,
	}

	if nodeIDStr := query.Get("node_id"); nodeIDStr != "" {
		parsed, err := uuid.Parse(nodeIDStr)
		if err != nil {
			apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid node_id format", nil)
			return
		}
		filter.NodeID = &parsed
	}

	alerts, total, err := r.store.ListAlerts(tenantID, filter)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to query alerts: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"items":  alerts,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}, "Alerts retrieved")
}

// handleMonitoringAlertResolve resolves a single alert and creates an audit event.
// POST /api/v2/tenants/{tenant_id}/monitoring/alerts/{alert_id}/resolve
func (r *Router) handleMonitoringAlertResolve(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, alertID string) {
	alertUUID, err := uuid.Parse(alertID)
	if err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid alert_id format", nil)
		return
	}

	// Verify alert exists and belongs to tenant
	existing, err := r.store.GetAlertByID(alertUUID)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusNotFound, "ALERT_NOT_FOUND", "Alert not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to get alert: "+err.Error(), nil)
		return
	}
	if existing.TenantID != tenantID {
		apibase.WriteError(w, http.StatusNotFound, "ALERT_NOT_FOUND", "Alert not found", nil)
		return
	}
	if existing.Status == "resolved" {
		apibase.WriteError(w, http.StatusBadRequest, "ALERT_ALREADY_RESOLVED", "Alert is already resolved", nil)
		return
	}

	resolved, err := r.store.ResolveAlert(alertUUID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to resolve alert: "+err.Error(), nil)
		return
	}

	// Create alert_resolved audit event
	r.store.CreateAuditEvent(&controllerstorage.AuditEvent{
		TenantID:  tenantID,
		NodeID:    existing.NodeID,
		EventType: "alert_resolved",
		Actor:     "user",
		Summary:   "Alert resolved: " + existing.Title,
		Detail: map[string]interface{}{
			"alert_id":   existing.ID.String(),
			"alert_type": existing.AlertType,
		},
	})

	apibase.WriteSuccess(w, resolved, "Alert resolved")
}

// computeStateConvergence determines the convergence status from a NodeControlState.
func computeStateConvergence(cs *controllerstorage.NodeControlState) string {
	if cs.DesiredStateVersion == "" {
		return "idle"
	}
	if cs.DesiredStateVersion == cs.AppliedStateVersion && cs.AppliedStateVersion != "" {
		return "converged"
	}
	if cs.DesiredStateVersion != cs.AppliedStateVersion && cs.LastSyncError != "" {
		return "diverged"
	}
	return "pending"
}

// handleMonitoringTraffic returns tenant-level traffic time series data.
// GET /api/v2/tenants/{tenant_id}/monitoring/traffic?range=24h
func (r *Router) handleMonitoringTraffic(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	rangeParam := req.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}

	var duration time.Duration
	var step time.Duration
	switch rangeParam {
	case "1h":
		duration = time.Hour
		step = 60 * time.Second
	case "24h":
		duration = 24 * time.Hour
		step = 5 * time.Minute
	case "7d":
		duration = 7 * 24 * time.Hour
		step = 30 * time.Minute
	case "30d":
		duration = 30 * 24 * time.Hour
		step = 2 * time.Hour
	default:
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid range parameter, must be 1h/24h/7d/30d", nil)
		return
	}

	end := time.Now()
	start := end.Add(-duration)

	// 获取租户下所有节点的标识用于 PromQL 过滤
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil || len(nodes) == 0 {
		apibase.WriteSuccess(w, map[string]interface{}{
			"timestamps":          []int64{},
			"upload_bytes":        []float64{},
			"download_bytes":      []float64{},
			"peak_bandwidth_mbps": 0,
		}, "Traffic data retrieved")
		return
	}

	// 构建 instance 过滤列表
	instances := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.PublicIP != "" {
			instances = append(instances, regexp.QuoteMeta(n.PublicIP)+":.*")
		}
	}
	instanceFilter := strings.Join(instances, "|")

	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()

	// 查询上传流量
	txQuery := fmt.Sprintf(`sum(rate(wireguard_total_tx_bytes{instance=~"%s"}[5m]))`, instanceFilter)
	txResult, _ := r.vmClient.QueryRange(ctx, txQuery, start, end, step)

	// 查询下载流量
	rxQuery := fmt.Sprintf(`sum(rate(wireguard_total_rx_bytes{instance=~"%s"}[5m]))`, instanceFilter)
	rxResult, _ := r.vmClient.QueryRange(ctx, rxQuery, start, end, step)

	timestamps := []int64{}
	uploadBytes := []float64{}
	downloadBytes := []float64{}
	peakBandwidth := 0.0

	// 1. 处理上传流量和初始化时间戳
	if txResult != nil && len(txResult.Data.Result) > 0 {
		for _, v := range txResult.Data.Result[0].Values {
			if len(v) >= 2 {
				if ts, ok := v[0].(float64); ok {
					timestamps = append(timestamps, int64(ts))
				}
				if val, ok := v[1].(string); ok {
					f, _ := strconv.ParseFloat(val, 64)
					uploadBytes = append(uploadBytes, f)
					mbps := f * 8 / 1_000_000
					if mbps > peakBandwidth {
						peakBandwidth = mbps
					}
				}
			}
		}
	}

	// 2. 处理下载流量（如果 timestamps 仍为空，尝试从 rxResult 补齐）
	if rxResult != nil && len(rxResult.Data.Result) > 0 {
		for i, v := range rxResult.Data.Result[0].Values {
			if len(v) >= 2 {
				// 如果上传流量没数据，从这里获取时间戳
				if len(timestamps) <= i {
					if ts, ok := v[0].(float64); ok {
						timestamps = append(timestamps, int64(ts))
					}
				}
				if val, ok := v[1].(string); ok {
					f, _ := strconv.ParseFloat(val, 64)
					downloadBytes = append(downloadBytes, f)
					mbps := f * 8 / 1_000_000
					if mbps > peakBandwidth {
						peakBandwidth = mbps
					}
				}
			}
		}
	}

	// 3. 补全缺失的上传数据点
	if len(uploadBytes) < len(timestamps) {
		for len(uploadBytes) < len(timestamps) {
			uploadBytes = append(uploadBytes, 0)
		}
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"timestamps":          timestamps,
		"upload_bytes":        uploadBytes,
		"download_bytes":      downloadBytes,
		"peak_bandwidth_mbps": math.Round(peakBandwidth*100) / 100,
	}, "Traffic data retrieved")
}

// handleMonitoringHealth returns tenant-level health indicators.
// GET /api/v2/tenants/{tenant_id}/monitoring/health
func (r *Router) handleMonitoringHealth(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	totalNodes, onlineNodes, _, err := r.store.CountNodesByTenantAndStatus(tenantID)
	nodeOnlineRate := 100.0
	if err == nil && totalNodes > 0 {
		nodeOnlineRate = float64(onlineNodes) * 100.0 / float64(totalNodes)
	}

	syncRate, _ := r.store.CalcSyncSuccessRate(tenantID)
	activeAlerts, _ := r.store.CountActiveAlerts(tenantID)
	failedCmds, _ := r.store.CountFailedCommandsByTenant(tenantID)

	apibase.WriteSuccess(w, map[string]interface{}{
		"node_online_rate":      math.Round(nodeOnlineRate*10) / 10,
		"sync_success_rate":     math.Round(syncRate*10) / 10,
		"active_alerts_count":   activeAlerts,
		"failed_commands_count": failedCmds,
	}, "Health data retrieved")
}

// handleMonitoringNodeMetrics returns per-node bandwidth and latency metrics.
// GET /api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}/metrics
func (r *Router) handleMonitoringNodeMetrics(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "Node not found", nil)
			return
		}
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to get node", nil)
		return
	}

	uploadMbps := 0.0
	downloadMbps := 0.0
	latencyMs := 0.0

	if r.vmClient != nil && node.PublicIP != "" {
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()

		instanceFilter := regexp.QuoteMeta(node.PublicIP) + ":.*"

		// 查询上传速率
		txQuery := fmt.Sprintf(`sum(rate(wireguard_peer_tx_bytes{instance=~"%s"}[5m]))`, instanceFilter)
		txResult, _ := r.vmClient.QueryInstant(ctx, txQuery)
		if txResult != nil && len(txResult.Data.Result) > 0 {
			if val, ok := txResult.Data.Result[0].Value[1].(string); ok {
				f, _ := strconv.ParseFloat(val, 64)
				uploadMbps = f * 8 / 1_000_000
			}
		}

		// 查询下载速率
		rxQuery := fmt.Sprintf(`sum(rate(wireguard_peer_rx_bytes{instance=~"%s"}[5m]))`, instanceFilter)
		rxResult, _ := r.vmClient.QueryInstant(ctx, rxQuery)
		if rxResult != nil && len(rxResult.Data.Result) > 0 {
			if val, ok := rxResult.Data.Result[0].Value[1].(string); ok {
				f, _ := strconv.ParseFloat(val, 64)
				downloadMbps = f * 8 / 1_000_000
			}
		}

		// 查询延迟（最近握手时间作为参考）
		latencyQuery := fmt.Sprintf(`min(wireguard_peer_last_handshake_secs{instance=~"%s"})`, instanceFilter)
		latencyResult, _ := r.vmClient.QueryInstant(ctx, latencyQuery)
		if latencyResult != nil && len(latencyResult.Data.Result) > 0 {
			if val, ok := latencyResult.Data.Result[0].Value[1].(string); ok {
				f, _ := strconv.ParseFloat(val, 64)
				latencyMs = f * 1000 // 秒转毫秒
			}
		}
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"upload_mbps":   math.Round(uploadMbps*100) / 100,
		"download_mbps": math.Round(downloadMbps*100) / 100,
		"latency_ms":    math.Round(latencyMs*100) / 100,
	}, "Node metrics retrieved")
}

// handleMonitoringTopology returns the mesh topology for a tenant.
// GET /api/v2/tenants/{tenant_id}/monitoring/topology
func (r *Router) handleMonitoringTopology(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeInternalServerError, "Failed to get nodes", nil)
		return
	}

	topoNodes := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		topoNodes = append(topoNodes, map[string]interface{}{
			"id":          n.ID.String(),
			"hostname":    n.Hostname,
			"region":      n.Region,
			"status":      nodeAvailabilityStatus(n),
			"assigned_ip": n.AssignedIP,
		})
	}

	// 构建全连接 peer 关系（WireGuard mesh）
	links := make([]map[string]interface{}, 0)
	
	// 获取实时流量数据（如果有 VM 客户端）
	peerTraffic := make(map[string]float64) // key: "src_pubkey:dst_pubkey", value: bps
	if r.vmClient != nil {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		
		// 查询所有 peer 的发送速率
		// 注意：此处不按 instance 过滤，而是获取所有数据后在内存中按租户节点匹配
		query := `rate(wireguard_peer_tx_bytes[5m]) * 8`
		result, err := r.vmClient.QueryInstant(ctx, query)
		if err == nil && result != nil {
			for _, item := range result.Data.Result {
				metrics := item.Metric
				valStr, ok := item.Value[1].(string)
				if !ok {
					continue
				}
				val, _ := strconv.ParseFloat(valStr, 64)
				
				// 我们需要知道发送者是谁，接收者是谁
				// 假设 instance 包含发送者的 IP，public_key 是接收者的公钥
				instance := metrics["instance"]
				remotePubKey := metrics["public_key"]
				
				if instance != "" && remotePubKey != "" {
					// 去掉 instance 的端口号
					host := instance
					if idx := strings.Index(instance, ":"); idx != -1 {
						host = instance[:idx]
					}
					peerTraffic[host+":"+remotePubKey] = val
				}
			}
		}
	}

	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			nodeA := nodes[i]
			nodeB := nodes[j]
			
			status := "inactive"
			if nodeAvailabilityStatus(nodeA) == "online" && nodeAvailabilityStatus(nodeB) == "online" {
				status = "active"
			}
			
			// 计算两个方向的流量之和
			trafficAB := peerTraffic[nodeA.PublicIP+":"+nodeB.PublicKey]
			trafficBA := peerTraffic[nodeB.PublicIP+":"+nodeA.PublicKey]
			totalTraffic := trafficAB + trafficBA
			
			links = append(links, map[string]interface{}{
				"source":  nodeA.ID.String(),
				"target":  nodeB.ID.String(),
				"status":  status,
				"traffic": math.Round(totalTraffic*100) / 100, // bps
			})
		}
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"nodes": topoNodes,
		"links": links,
	}, "Topology data retrieved")
}
