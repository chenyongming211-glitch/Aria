package v2

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	apibase "aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

const codeCommandDispatchFailed = "COMMAND_DISPATCH_FAILED"

type v2AgentCommandRequest struct {
	Command  string                 `json:"command"`
	Params   map[string]interface{} `json:"params"`
	Timeout  int                    `json:"timeout"`
	Priority int                    `json:"priority"`
}

type v2BatchAgentCommandRequest struct {
	NodeIDs []string              `json:"node_ids"`
	Command v2AgentCommandRequest `json:"command"`
}

func (r *Router) handleTenantAgents(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermCommandsWrite) {
		return
	}

	parts := splitPath(req.URL.Path)
	if len(parts) == 6 && parts[4] == "agents" && parts[5] == "command" && req.Method == http.MethodPost {
		r.handleTenantBatchAgentCommand(w, req, tenantID)
		return
	}

	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid agent path", nil)
}

func (r *Router) handleTenantNodeAgent(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) != 8 {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid node agent path", nil)
		return
	}

	node, err := r.getTenantNodeRecord(parts[5], tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	switch parts[7] {
	case "command":
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermCommandsWrite) {
			return
		}
		r.handleTenantNodeAgentCommand(w, req, node)
	case "commands":
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermMonitoringRead) {
			return
		}
		r.handleTenantNodeAgentCommands(w, req, node)
	case "status":
		if !r.authorizeTenantPermission(w, req, tenantID, middleware.PermMonitoringRead) {
			return
		}
		r.handleTenantNodeAgentStatus(w, node)
	default:
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeEndpointNotFound, "Unknown node agent endpoint", nil)
	}
}

func (r *Router) handleTenantNodeAgentCommand(w http.ResponseWriter, req *http.Request, node *controllerstorage.Node) {
	if req.Method != http.MethodPost {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var body v2AgentCommandRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}
	if body.Command == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "command is required", nil)
		return
	}
	body.Command = strings.TrimSpace(body.Command)
	if !controllerstorage.IsAllowedAgentCommand(body.Command) {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "unsupported command", nil)
		return
	}
	if body.Timeout == 0 {
		body.Timeout = 30
	}

	cmd, err := r.store.QueueAgentCommand(node.PublicKey, body.Command, body.Params, body.Priority, body.Timeout)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to queue command: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"command_id":      cmd.ID,
		"node_id":         node.ID.String(),
		"node_public_key": node.PublicKey,
		"status":          cmd.Status,
		"message":         "Command queued for delivery",
		"created_at":      cmd.CreatedAt,
		"updated_at":      cmd.UpdatedAt,
		"command":         cmd.Command,
		"timeout_seconds": cmd.TimeoutSeconds,
		"priority":        cmd.Priority,
	}, "Command queued successfully")
}

func (r *Router) handleTenantNodeAgentStatus(w http.ResponseWriter, node *controllerstorage.Node) {
	summary, err := r.buildNodeOperationsSummary(node)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to query agent status: "+err.Error(), nil)
		return
	}

	apibase.WriteSuccess(w, summary, "Agent status retrieved")
}

func (r *Router) handleTenantNodeAgentCommands(w http.ResponseWriter, req *http.Request, node *controllerstorage.Node) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	limit := 10
	if rawLimit := req.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	commands, err := r.store.ListRecentAgentCommands(node.PublicKey, limit)
	if err != nil {
		apibase.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to load agent commands: "+err.Error(), nil)
		return
	}

	items := make([]map[string]interface{}, 0, len(commands))
	for _, cmd := range commands {
		items = append(items, agentCommandToMap(cmd))
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"node_id":         node.ID.String(),
		"node_public_key": node.PublicKey,
		"items":           items,
	}, "Agent commands retrieved")
}

func (r *Router) handleTenantBatchAgentCommand(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	var body v2BatchAgentCommandRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}
	if body.Command.Command == "" {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "command is required", nil)
		return
	}
	body.Command.Command = strings.TrimSpace(body.Command.Command)
	if !controllerstorage.IsAllowedAgentCommand(body.Command.Command) {
		apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "unsupported command", nil)
		return
	}
	if body.Command.Timeout == 0 {
		body.Command.Timeout = 30
	}

	var (
		nodes []*controllerstorage.Node
		err   error
	)
	if len(body.NodeIDs) == 0 {
		nodes, err = r.store.GetNodesByTenant(tenantID)
		if err != nil {
			apibase.WriteError(w, http.StatusInternalServerError, apibase.CodeGetNodesFailed, "Failed to load tenant nodes", nil)
			return
		}
	} else {
		for _, nodeID := range body.NodeIDs {
			node, err := r.getTenantNodeRecord(nodeID, tenantID)
			if err == nil && node != nil {
				nodes = append(nodes, node)
			}
		}
	}
	if len(nodes) == 0 {
		apibase.WriteError(w, http.StatusNotFound, apibase.CodeNodeNotFound, "No nodes found", nil)
		return
	}

	results := make([]map[string]interface{}, 0, len(nodes))
	successCount := 0
	failedCount := 0
	for _, node := range nodes {
		cmd, err := r.store.QueueAgentCommand(node.PublicKey, body.Command.Command, body.Command.Params, body.Command.Priority, body.Command.Timeout)
		if err != nil {
			failedCount++
			results = append(results, map[string]interface{}{
				"node_id":         node.ID.String(),
				"node_public_key": node.PublicKey,
				"status":          controllerstorage.AgentCommandStatusFailed,
				"message":         "Failed to queue command: " + err.Error(),
			})
			continue
		}

		successCount++
		results = append(results, map[string]interface{}{
			"command_id":      cmd.ID,
			"node_id":         node.ID.String(),
			"node_public_key": node.PublicKey,
			"status":          cmd.Status,
			"message":         "Command queued for delivery",
			"created_at":      cmd.CreatedAt,
			"updated_at":      cmd.UpdatedAt,
		})
	}

	apibase.WriteSuccess(w, map[string]interface{}{
		"total_count":   len(nodes),
		"success_count": successCount,
		"failed_count":  failedCount,
		"results":       results,
	}, "Batch command processed")
}

func (r *Router) handleTenantMonitoring(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	requiredPermission := middleware.PermMonitoringRead
	if req.Method == http.MethodPost {
		requiredPermission = middleware.PermCommandsWrite
	}
	if !r.authorizeTenantPermission(w, req, tenantID, requiredPermission) {
		return
	}

	parts := splitPath(req.URL.Path)

	// GET /monitoring/stats
	if len(parts) == 6 && parts[5] == "stats" && req.Method == http.MethodGet {
		r.handleMonitoringStats(w, req, tenantID)
		return
	}

	// GET /monitoring/events
	if len(parts) == 6 && parts[5] == "events" && req.Method == http.MethodGet {
		r.handleMonitoringEvents(w, req, tenantID)
		return
	}

	// GET /monitoring/alerts
	if len(parts) == 6 && parts[5] == "alerts" && req.Method == http.MethodGet {
		r.handleMonitoringAlerts(w, req, tenantID)
		return
	}

	// GET /monitoring/nodes/{node_id}
	if len(parts) == 7 && parts[5] == "nodes" && req.Method == http.MethodGet {
		r.handleMonitoringNodeDetail(w, req, tenantID, parts[6])
		return
	}

	// GET /monitoring/nodes/{node_id}/metrics
	if len(parts) == 8 && parts[5] == "nodes" && parts[7] == "metrics" && req.Method == http.MethodGet {
		r.handleMonitoringNodeMetrics(w, req, tenantID, parts[6])
		return
	}

	// GET /monitoring/traffic
	if len(parts) == 6 && parts[5] == "traffic" && req.Method == http.MethodGet {
		r.handleMonitoringTraffic(w, req, tenantID)
		return
	}

	// GET /monitoring/health
	if len(parts) == 6 && parts[5] == "health" && req.Method == http.MethodGet {
		r.handleMonitoringHealth(w, req, tenantID)
		return
	}

	// GET /monitoring/topology
	if len(parts) == 6 && parts[5] == "topology" && req.Method == http.MethodGet {
		r.handleMonitoringTopology(w, req, tenantID)
		return
	}

	// POST /monitoring/alerts/{alert_id}/resolve
	if len(parts) == 8 && parts[5] == "alerts" && parts[7] == "resolve" && req.Method == http.MethodPost {
		r.handleMonitoringAlertResolve(w, req, tenantID, parts[6])
		return
	}

	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeInvalidPath, "Invalid monitoring path", nil)
}

func nodeAvailabilityStatus(node *controllerstorage.Node) string {
	if node.Status == "deleted" {
		return "deleted"
	}
	if node.LastSeen <= 0 {
		return "unknown"
	}
	if time.Now().Unix()-node.LastSeen <= 60 {
		return "online"
	}
	return "offline"
}

func nodeUptimeSeconds(node *controllerstorage.Node) int64 {
	if node.RegisteredAt <= 0 {
		return 0
	}
	return maxInt64(0, time.Now().Unix()-node.RegisteredAt)
}

func (r *Router) buildNodeOperationsSummary(node *controllerstorage.Node) (map[string]interface{}, error) {
	pendingCmds, err := r.store.CountIncompleteAgentCommands(node.PublicKey)
	if err != nil {
		return nil, err
	}
	lastCommand, err := r.store.GetLastAgentCommand(node.PublicKey)
	if err != nil {
		return nil, err
	}

	controlState, err := r.store.GetNodeControlState(node.TenantID, node.ID)
	if err != nil {
		return nil, err
	}

	configurationStatus := deriveConfigurationStatus(controlState, lastCommand, pendingCmds)
	lastSyncAt := interface{}(node.LastSeen)
	if controlState != nil && controlState.LastSyncAt != nil {
		lastSyncAt = controlState.LastSyncAt
	}

	lastAppliedAt := lastAppliedTimestamp(lastCommand)
	if controlState != nil && controlState.AppliedStateUpdatedAt != nil {
		lastAppliedAt = controlState.AppliedStateUpdatedAt
	}

	summary := map[string]interface{}{
		"node_id":              node.ID.String(),
		"node_public_key":      node.PublicKey,
		"hostname":             node.Hostname,
		"region":               node.Region,
		"public_ip":            node.PublicIP,
		"assigned_ip":          node.AssignedIP,
		"status":               nodeAvailabilityStatus(node),
		"availability_status":  nodeAvailabilityStatus(node),
		"sync_status":          nodeAvailabilityStatus(node),
		"configuration_status": configurationStatus,
		"last_seen":            node.LastSeen,
		"last_sync_at":         lastSyncAt,
		"pending_cmds":         pendingCmds,
		"uptime":               nodeUptimeSeconds(node),
		"last_command":         agentCommandToMap(lastCommand),
		"last_command_status":  valueOrEmpty(lastCommand, func(cmd *controllerstorage.AgentCommand) string { return cmd.Status }),
		"last_command_message": valueOrEmpty(lastCommand, func(cmd *controllerstorage.AgentCommand) string { return cmd.Message }),
		"last_command_error":   lastCommandError(lastCommand),
		"last_command_at":      lastCommandTimestamp(lastCommand),
		"last_applied_at":      lastAppliedAt,
	}

	if controlState != nil {
		summary["desired_state_version"] = controlState.DesiredStateVersion
		summary["desired_state_updated_at"] = controlState.DesiredStateUpdatedAt
		summary["applied_state_version"] = controlState.AppliedStateVersion
		summary["applied_state_updated_at"] = controlState.AppliedStateUpdatedAt
		summary["observed_state"] = firstNonEmptyString(controlState.ObservedState, configurationStatus)
		summary["observed_message"] = controlState.ObservedMessage
		summary["observed_at"] = controlState.ObservedAt
		summary["last_sync_error"] = controlState.LastSyncError
		summary["state_convergence"] = stateConvergenceStatus(node, controlState, configurationStatus)
	} else {
		summary["desired_state_version"] = ""
		summary["desired_state_updated_at"] = nil
		summary["applied_state_version"] = ""
		summary["applied_state_updated_at"] = nil
		summary["observed_state"] = configurationStatus
		summary["observed_message"] = ""
		summary["observed_at"] = nil
		summary["last_sync_error"] = ""
		summary["state_convergence"] = "idle"
	}

	return summary, nil
}

func agentCommandToMap(cmd *controllerstorage.AgentCommand) map[string]interface{} {
	if cmd == nil {
		return nil
	}

	return map[string]interface{}{
		"id":              cmd.ID,
		"command":         cmd.Command,
		"params":          cmd.RawParams,
		"status":          cmd.Status,
		"message":         cmd.Message,
		"priority":        cmd.Priority,
		"timeout_seconds": cmd.TimeoutSeconds,
		"created_at":      cmd.CreatedAt,
		"updated_at":      cmd.UpdatedAt,
		"sent_at":         cmd.SentAt,
		"acknowledged_at": cmd.AcknowledgedAt,
		"completed_at":    cmd.CompletedAt,
		"result":          cmd.Result,
	}
}

func lastCommandTimestamp(cmd *controllerstorage.AgentCommand) interface{} {
	if cmd == nil {
		return nil
	}
	if cmd.CompletedAt != nil {
		return cmd.CompletedAt
	}
	if cmd.AcknowledgedAt != nil {
		return cmd.AcknowledgedAt
	}
	if cmd.SentAt != nil {
		return cmd.SentAt
	}
	return cmd.CreatedAt
}

func lastAppliedTimestamp(cmd *controllerstorage.AgentCommand) interface{} {
	if cmd == nil || cmd.Status != controllerstorage.AgentCommandStatusCompleted {
		return nil
	}
	if cmd.CompletedAt != nil {
		return cmd.CompletedAt
	}
	return cmd.UpdatedAt
}

func lastCommandError(cmd *controllerstorage.AgentCommand) string {
	if cmd == nil || cmd.Status != controllerstorage.AgentCommandStatusFailed {
		return ""
	}
	return cmd.Message
}

func valueOrEmpty(cmd *controllerstorage.AgentCommand, getter func(*controllerstorage.AgentCommand) string) string {
	if cmd == nil {
		return ""
	}
	return getter(cmd)
}

func deriveConfigurationStatus(
	controlState *controllerstorage.NodeControlState,
	lastCommand *controllerstorage.AgentCommand,
	pendingCmds int,
) string {
	if pendingCmds > 0 {
		if lastCommand != nil {
			switch lastCommand.Status {
			case controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged:
				return "in_progress"
			}
		}
		return "pending"
	}

	if controlState != nil {
		if controlState.LastSyncError != "" || controlState.ObservedState == "error" {
			return "error"
		}
		if controlState.DesiredStateVersion != "" && controlState.DesiredStateVersion == controlState.AppliedStateVersion {
			return "applied"
		}
		if controlState.DesiredStateVersion != "" && controlState.AppliedStateVersion != controlState.DesiredStateVersion {
			return "pending"
		}
		if controlState.ObservedState == "healthy" {
			return "applied"
		}
	}

	if lastCommand != nil {
		switch lastCommand.Status {
		case controllerstorage.AgentCommandStatusCompleted:
			return "applied"
		case controllerstorage.AgentCommandStatusFailed:
			return "error"
		case controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged:
			return "in_progress"
		}
	}

	return "idle"
}

func stateConvergenceStatus(node *controllerstorage.Node, controlState *controllerstorage.NodeControlState, configurationStatus string) string {
	isOnline := nodeAvailabilityStatus(node) == "online"
	if controlState == nil {
		if isOnline {
			return string(controllerstorage.StatusConverged)
		}
		return string(controllerstorage.StatusOffline)
	}

	// 优先使用存储层定义的计算逻辑
	status := controlState.GetConvergenceStatus(isOnline)

	// 如果有未完成命令，且存储层认为是收敛的，强制降级为 pending
	if status == controllerstorage.StatusConverged && configurationStatus == "in_progress" {
		return string(controllerstorage.StatusPending)
	}

	return string(status)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
