package v2

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	v1 "aria/internal/api/v1"
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
	if !r.authorizeTenantAdmin(w, req, tenantID) {
		return
	}

	parts := splitPath(req.URL.Path)
	if len(parts) == 6 && parts[4] == "agents" && parts[5] == "command" && req.Method == http.MethodPost {
		r.handleTenantBatchAgentCommand(w, req, tenantID)
		return
	}

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid agent path", nil)
}

func (r *Router) handleTenantNodeAgent(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID, parts []string) {
	if len(parts) != 8 {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid node agent path", nil)
		return
	}

	node, err := r.getTenantNodeRecord(parts[5], tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	switch parts[7] {
	case "command":
		if !r.authorizeTenantAdmin(w, req, tenantID) {
			return
		}
		r.handleTenantNodeAgentCommand(w, req, node)
	case "commands":
		if !r.authorizeTenant(w, req, tenantID, false) {
			return
		}
		r.handleTenantNodeAgentCommands(w, req, node)
	case "status":
		if !r.authorizeTenant(w, req, tenantID, false) {
			return
		}
		r.handleTenantNodeAgentStatus(w, node)
	default:
		v1.WriteError(w, http.StatusNotFound, v1.CodeEndpointNotFound, "Unknown node agent endpoint", nil)
	}
}

func (r *Router) handleTenantNodeAgentCommand(w http.ResponseWriter, req *http.Request, node *controllerstorage.Node) {
	if req.Method != http.MethodPost {
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var body v2AgentCommandRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}
	if body.Command == "" {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "command is required", nil)
		return
	}
	if body.Timeout == 0 {
		body.Timeout = 30
	}

	cmd, err := r.store.QueueAgentCommand(node.PublicKey, body.Command, body.Params, body.Priority, body.Timeout)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to queue command: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, map[string]interface{}{
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
		v1.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to query agent status: "+err.Error(), nil)
		return
	}

	v1.WriteSuccess(w, summary, "Agent status retrieved")
}

func (r *Router) handleTenantNodeAgentCommands(w http.ResponseWriter, req *http.Request, node *controllerstorage.Node) {
	if req.Method != http.MethodGet {
		v1.WriteError(w, http.StatusMethodNotAllowed, v1.CodeMethodNotAllowed, "Method not allowed", nil)
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
		v1.WriteError(w, http.StatusInternalServerError, codeCommandDispatchFailed, "Failed to load agent commands: "+err.Error(), nil)
		return
	}

	items := make([]map[string]interface{}, 0, len(commands))
	for _, cmd := range commands {
		items = append(items, agentCommandToMap(cmd))
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"node_id":         node.ID.String(),
		"node_public_key": node.PublicKey,
		"items":           items,
	}, "Agent commands retrieved")
}

func (r *Router) handleTenantBatchAgentCommand(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	var body v2BatchAgentCommandRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}
	if body.Command.Command == "" {
		v1.WriteError(w, http.StatusBadRequest, v1.CodeBadRequest, "command is required", nil)
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
			v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load tenant nodes", nil)
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
		v1.WriteError(w, http.StatusNotFound, v1.CodeNodeNotFound, "No nodes found", nil)
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

	v1.WriteSuccess(w, map[string]interface{}{
		"total_count":   len(nodes),
		"success_count": successCount,
		"failed_count":  failedCount,
		"results":       results,
	}, "Batch command processed")
}

func (r *Router) handleTenantMonitoring(w http.ResponseWriter, req *http.Request, tenantID uuid.UUID) {
	if !r.authorizeTenant(w, req, tenantID, false) {
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

	v1.WriteError(w, http.StatusBadRequest, v1.CodeInvalidPath, "Invalid monitoring path", nil)
}

func (r *Router) handleTenantMonitoringStats(w http.ResponseWriter, tenantID uuid.UUID) {
	nodes, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load tenant nodes", nil)
		return
	}

	regions := make(map[string]struct{})
	peerDetails := make([]map[string]interface{}, 0, len(nodes))
	onlineNodes := 0
	totalRoutes := 0
	for _, node := range nodes {
		if node.Region != "" {
			regions[node.Region] = struct{}{}
		}
		if nodeAvailabilityStatus(node) == "online" {
			onlineNodes++
		}
		totalRoutes += len(node.AdvertisedRoutes)
		peerDetails = append(peerDetails, map[string]interface{}{
			"publicKey":     node.PublicKey,
			"peerIp":        node.AssignedIP,
			"localIp":       node.AssignedIP,
			"publicIp":      node.PublicIP,
			"hostId":        node.ID.String(),
			"hostname":      node.Hostname,
			"region":        node.Region,
			"connected":     nodeAvailabilityStatus(node) == "online",
			"lastHandshake": node.LastSeen,
			"rtt":           0,
			"lossRatio":     0,
			"healthScore":   map[bool]float64{true: 1, false: 0}[nodeAvailabilityStatus(node) == "online"],
			"failureCount":  0,
			"rxBytes":       0,
			"txBytes":       0,
			"rxRate":        0,
			"txRate":        0,
		})
	}

	regionList := make([]string, 0, len(regions))
	for region := range regions {
		regionList = append(regionList, region)
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"peers":        len(peerDetails),
		"avgRtt":       "-",
		"packetLoss":   "-",
		"totalTraffic": "-",
		"trafficData": map[string]interface{}{
			"timestamps": []string{},
			"入向":         []float64{},
			"出向":         []float64{},
		},
		"peerDetails": peerDetails,
		"systemStats": map[string]interface{}{
			"totalNodes":      len(nodes),
			"onlineNodes":     onlineNodes,
			"avgRtt":          0,
			"packetLoss":      0,
			"avgCpu":          0,
			"avgMemory":       0,
			"totalGoroutines": 0,
		},
		"globalStats": map[string]interface{}{
			"totalNodes":   len(nodes),
			"onlineNodes":  onlineNodes,
			"totalRegions": len(regionList),
			"regionList":   regionList,
			"totalRoutes":  totalRoutes,
			"directRoutes": totalRoutes,
			"relayRoutes":  0,
			"totalRxRate":  0,
			"totalTxRate":  0,
			"totalTraffic": "-",
		},
	}, "Monitoring stats retrieved")
}

func (r *Router) handleTenantMonitoringNodeDetail(w http.ResponseWriter, tenantID uuid.UUID, nodeID string) {
	node, err := r.getTenantNodeRecord(nodeID, tenantID)
	if err != nil {
		r.writeNodeLookupError(w, err)
		return
	}

	peers, err := r.store.GetNodesByTenant(tenantID)
	if err != nil {
		v1.WriteError(w, http.StatusInternalServerError, v1.CodeGetNodesFailed, "Failed to load peers", nil)
		return
	}

	peerItems := make([]map[string]interface{}, 0, len(peers))
	for _, peer := range peers {
		if peer.ID == node.ID {
			continue
		}
		peerItems = append(peerItems, map[string]interface{}{
			"publicKey":    peer.PublicKey,
			"peerIp":       peer.AssignedIP,
			"endpoint":     peer.Endpoint,
			"handshakeAge": 0,
			"rtt":          0,
			"lossRatio":    0,
			"rxRate":       0,
			"txRate":       0,
			"status":       nodeAvailabilityStatus(peer),
			"rxBytes":      0,
			"txBytes":      0,
		})
	}

	v1.WriteSuccess(w, map[string]interface{}{
		"hostId":           node.ID.String(),
		"hostname":         node.Hostname,
		"publicIp":         node.PublicIP,
		"localIp":          node.AssignedIP,
		"region":           node.Region,
		"ip":               node.AssignedIP,
		"version":          "unknown",
		"role":             node.Role,
		"runtimeMode":      node.RuntimeMode,
		"advertisedRoutes": node.AdvertisedRoutes,
		"uptime":           nodeUptimeSeconds(node),
		"cpuUsage":         0,
		"memoryUsage":      0,
		"goroutines":       0,
		"cpuCores":         []interface{}{},
		"cpuBalance":       0,
		"tunnels":          []interface{}{},
		"tunnelBalance":    0,
		"peers":            peerItems,
		"firewall": map[string]interface{}{
			"acceptPackets":    0,
			"dropPackets":      0,
			"invalidPackets":   0,
			"tcpFlagsPackets":  0,
			"notrackRules":     0,
			"processedPackets": 0,
			"droppedPackets":   0,
		},
	}, "Node monitoring detail retrieved")
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
		summary["state_convergence"] = stateConvergenceStatus(controlState, configurationStatus)
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

func stateConvergenceStatus(controlState *controllerstorage.NodeControlState, configurationStatus string) string {
	if controlState == nil {
		return "idle"
	}
	if configurationStatus == "error" {
		return "diverged"
	}
	if controlState.DesiredStateVersion == "" {
		return "idle"
	}
	if controlState.DesiredStateVersion == controlState.AppliedStateVersion {
		return "converged"
	}
	return "pending"
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
