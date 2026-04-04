package v2

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	v1 "aria/internal/api/v1"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

// handleMonitoringStats returns tenant-level monitoring statistics.
// GET /api/v2/tenants/{tenant_id}/monitoring/stats
func (r *Router) handleMonitoringStats(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	totalNodes, onlineNodes, offlineNodes, err := r.store.CountNodesByTenantAndStatus(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to count nodes: "+err.Error(), nil)
		return
	}

	syncRate, err := r.store.CalcSyncSuccessRate(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to calculate sync rate: "+err.Error(), nil)
		return
	}

	aclCount, err := r.store.CountACLRulesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to count ACL rules: "+err.Error(), nil)
		return
	}

	qosCount, err := r.store.CountQoSRulesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to count QoS rules: "+err.Error(), nil)
		return
	}

	failedCmds, err := r.store.CountFailedCommandsByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to count failed commands: "+err.Error(), nil)
		return
	}

	activeAlerts, err := r.store.CountActiveAlerts(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to count active alerts: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
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
			v1.WriteError(w, http.StatusNotFound, v1.CodeNodeNotFound, "Node not found", nil)
			return
		}
		r.writeNodeLookupError(w, err)
		return
	}

	controlState, err := r.store.GetNodeControlState(tenantID, node.ID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to get control state: "+err.Error(), nil)
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
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to list commands: "+err.Error(), nil)
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
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to list policy deliveries: "+err.Error(), nil)
		return
	}
	pdItems := make([]map[string]interface{}, 0, len(deliveries))
	for _, pd := range deliveries {
		pdItems = append(pdItems, policyDeliveryToMap(pd))
	}
	data["recent_policy_deliveries"] = pdItems

	v1.WriteSuccess(w, data, "Node monitoring detail retrieved")
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
			v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid node_id format", nil)
			return
		}
		filter.NodeID = &parsed
	}

	if sinceStr := query.Get("since"); sinceStr != "" {
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid since format, use ISO 8601", nil)
			return
		}
		filter.Since = &parsed
	}

	items, total, err := r.store.ListEventFeed(tenantID, filter)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to query events: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
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
			v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid node_id format", nil)
			return
		}
		filter.NodeID = &parsed
	}

	alerts, total, err := r.store.ListAlerts(tenantID, filter)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to query alerts: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
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
		v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid alert_id format", nil)
		return
	}

	// Verify alert exists and belongs to tenant
	existing, err := r.store.GetAlertByID(alertUUID)
	if err != nil {
		if err == sql.ErrNoRows {
			v1.WriteError(w, http.StatusNotFound, "ALERT_NOT_FOUND", "Alert not found", nil)
			return
		}
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to get alert: "+err.Error(), nil)
		return
	}
	if existing.TenantID != tenantID {
		v1.WriteError(w, http.StatusNotFound, "ALERT_NOT_FOUND", "Alert not found", nil)
		return
	}
	if existing.Status == "resolved" {
		v1.WriteError(w, http.StatusBadRequest, "ALERT_ALREADY_RESOLVED", "Alert is already resolved", nil)
		return
	}

	resolved, err := r.store.ResolveAlert(alertUUID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeInternalServerError, "Failed to resolve alert: "+err.Error(), nil)
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

	v1.WriteSuccess(w, resolved, "Alert resolved")
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
