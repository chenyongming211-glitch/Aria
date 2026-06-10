package grpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"aria/internal/auth"
	"aria/internal/nodeidentity"
	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ControllerServer 实现 gRPC ControllerService
// 通过包装 REST API handler 来复用业务逻辑
type ControllerServer struct {
	agentpb.UnimplementedControllerServiceServer
	registerHandler RegisterHandler
	syncHandler     func(publicKey string) (peers interface{}, assignedIP string, aclRules interface{}, metricsGateway string, err error)
	store           *controllerstorage.Storage
}

var commandStreamPollInterval = 2 * time.Second

type RegistrationRequest struct {
	PublicKey        string
	Endpoint         string
	PrivateIP        string
	PublicIP         string
	Hostname         string
	RegisteredAt     int64
	Token            string
	RuntimeToken     string
	AdvertisedRoutes []string
	Region           string
	CustomerID       string
	RuntimeMode      string
	KernelVersion    string
	HasAESNI         bool
	MachineID        string
}

type RegistrationResult struct {
	AssignedIP            string
	MetricsPushGateway    string
	NodeID                string
	RuntimeToken          string
	RuntimeTokenExpiresAt int64
}

type RegisterHandler func(*RegistrationRequest) (*RegistrationResult, error)

// NewControllerServer 创建新的 gRPC 服务端
func NewControllerServer(
	registerHandler RegisterHandler,
	syncHandler func(string) (interface{}, string, interface{}, string, error),
	store *controllerstorage.Storage,
) *ControllerServer {
	return &ControllerServer{
		registerHandler: registerHandler,
		syncHandler:     syncHandler,
		store:           store,
	}
}

// Register 处理 Agent 注册请求
func (s *ControllerServer) Register(ctx context.Context, req *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	if s.registerHandler == nil {
		return nil, fmt.Errorf("registration failed: register handler is not configured")
	}

	result, err := s.registerHandler(registrationRequestFromProto(req))
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("registration failed: register handler returned no result")
	}
	if strings.TrimSpace(result.NodeID) == "" {
		return nil, fmt.Errorf("registration failed: registered node was not persisted")
	}
	if strings.TrimSpace(result.RuntimeToken) == "" {
		return nil, fmt.Errorf("registration failed: runtime token was not issued")
	}

	return &agentpb.RegisterResponse{
		AssignedIp:            result.AssignedIP,
		MetricsPushGateway:    result.MetricsPushGateway,
		NodeId:                result.NodeID,
		RuntimeToken:          result.RuntimeToken,
		RuntimeTokenExpiresAt: result.RuntimeTokenExpiresAt,
	}, nil
}

func registrationRequestFromProto(req *agentpb.RegisterRequest) *RegistrationRequest {
	if req == nil {
		return &RegistrationRequest{}
	}

	return &RegistrationRequest{
		PublicKey:        req.PublicKey,
		Endpoint:         req.Endpoint,
		PrivateIP:        req.PrivateIp,
		PublicIP:         req.PublicIp,
		Hostname:         req.Hostname,
		RegisteredAt:     req.RegisteredAt,
		Token:            req.Token,
		AdvertisedRoutes: append([]string(nil), req.AdvertisedRoutes...),
		Region:           req.Region,
		CustomerID:       req.CustomerId,
		RuntimeMode:      req.RuntimeMode,
		KernelVersion:    req.KernelVersion,
		HasAESNI:         req.HasAesni,
		MachineID:        req.MachineId,
	}
}

// Sync 处理 Agent 定期同步请求
// 复用 REST API 的 HandleSync 逻辑
func (s *ControllerServer) Sync(ctx context.Context, req *agentpb.SyncRequest) (*agentpb.SyncResponse, error) {
	node, err := s.resolveRuntimeNodeForRequest(ctx, req.NodeId, req.PublicKey)
	if err != nil {
		return nil, err
	}

	if err := s.reportRuntimeSyncState(node, req); err != nil {
		return nil, fmt.Errorf("failed to persist runtime sync state: %w", err)
	}

	// 调用 REST API handler
	peersInterface, assignedIP, aclRulesInterface, metricsGateway, err := s.syncHandler(node.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}

	// 转换 Peers
	var peers []*agentpb.PeerInfo
	if peersInterface != nil {
		if peersBytes, err := json.Marshal(peersInterface); err == nil {
			var peerList []map[string]interface{}
			if err := json.Unmarshal(peersBytes, &peerList); err == nil {
				for _, p := range peerList {
					peers = append(peers, &agentpb.PeerInfo{
						PublicKey:        getString(p, "public_key"),
						Endpoint:         getString(p, "endpoint"),
						PrivateIp:        getString(p, "private_ip"),
						PublicIp:         getString(p, "public_ip"),
						Region:           getString(p, "region"),
						VpcId:            getString(p, "vpc_id"),
						Hostname:         getString(p, "hostname"),
						AssignedIp:       getString(p, "assigned_ip"),
						Role:             getString(p, "role"),
						AdvertisedRoutes: getStringSlice(p, "advertised_routes"),
					})
				}
			}
		}
	}

	// 查询 QoS 规则
	qosRules, err := s.getQoSRules(ctx, node.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get QoS rules: %w", err)
	}

	blacklistRules, err := s.getBlacklistRules(ctx, node.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get blacklist rules: %w", err)
	}

	policySnapshot, err := compileAgentPolicySnapshot(aclRulesInterface, qosRules, blacklistRules)
	if err != nil {
		return nil, fmt.Errorf("failed to compile policy snapshot: %w", err)
	}

	desiredVersion, err := s.ensureDesiredStateVersion(node)
	if err != nil {
		return nil, fmt.Errorf("failed to determine desired state version: %w", err)
	}

	// 刷新运行期凭据
	runtimeToken, runtimeTokenExpiresAt, err := generateRuntimeTokenForNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh runtime token: %w", err)
	}

	return &agentpb.SyncResponse{
		Peers:                 peers,
		AssignedIp:            assignedIP,
		LastUpdate:            time.Now().Unix(),
		AclRules:              policySnapshot.ACLRules,
		MetricsPushGateway:    metricsGateway,
		QosRules:              policySnapshot.QoSRules,
		BlacklistRules:        policySnapshot.BlacklistRules,
		DesiredStateVersion:   desiredVersion,
		RuntimeToken:          runtimeToken,
		RuntimeTokenExpiresAt: runtimeTokenExpiresAt,
		SnapshotComplete:      true,
		DomainVersions:        domainVersionsFromDesiredVersion(desiredVersion),
	}, nil
}

// CommandStream 处理双向流命令
// Agent 连接后，Controller 可以推送命令，Agent 返回执行结果
func (s *ControllerServer) CommandStream(stream agentpb.ControllerService_CommandStreamServer) error {
	resp, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stream recv error: %w", err)
	}
	if resp == nil || resp.CommandId != "init" {
		return fmt.Errorf("command stream init payload is required")
	}

	node, err := s.resolveCommandStreamNodeForRequest(stream.Context(), resp)
	if err != nil {
		return err
	}
	nodePublicKey := node.PublicKey
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return fmt.Errorf("stream send header error: %w", err)
	}
	if err := s.sendNextPendingCommand(stream, nodePublicKey); err != nil {
		return err
	}

	responses := make(chan *agentpb.CommandResponse, 1)
	recvErrs := make(chan error, 1)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				recvErrs <- err
				return
			}
			select {
			case responses <- resp:
			case <-stream.Context().Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(commandStreamPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case err := <-recvErrs:
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("stream recv error: %w", err)
		case resp := <-responses:
			if resp == nil {
				continue
			}

			if resp.CommandId == "init" {
				continue
			}

			if resp.CommandId != "" && s.store != nil {
				if err := s.ensureCommandStreamNodeActive(nodePublicKey); err != nil {
					return err
				}
				if err := s.store.UpdateAgentCommandStatusForNode(resp.CommandId, nodePublicKey, resp.Status, resp.Message, resp.Result); err != nil {
					return fmt.Errorf("failed to update command status: %w", err)
				}
			}

			if isTerminalCommandStatus(resp.Status) {
				if err := s.sendNextPendingCommand(stream, nodePublicKey); err != nil {
					return err
				}
			}
		case <-ticker.C:
			if err := s.sendNextPendingCommand(stream, nodePublicKey); err != nil {
				return err
			}
		}
	}
}

func (s *ControllerServer) sendNextPendingCommand(stream agentpb.ControllerService_CommandStreamServer, agentID string) error {
	if s.store == nil || agentID == "" {
		return nil
	}
	if err := s.ensureCommandStreamNodeActive(agentID); err != nil {
		return err
	}

	cmd, err := s.store.GetNextPendingAgentCommand(agentID)
	if err != nil {
		return fmt.Errorf("failed to get pending command: %w", err)
	}
	if cmd == nil {
		return nil
	}

	if err := stream.Send(&agentpb.CommandRequest{
		CommandId: cmd.ID,
		Command:   cmd.Command,
		Params:    cmd.Params,
		Timeout:   int32(cmd.TimeoutSeconds),
		Priority:  int32(cmd.Priority),
		CreatedAt: cmd.CreatedAt.Unix(),
	}); err != nil {
		if requeueErr := s.store.RequeueSentAgentCommand(cmd.ID, agentID, "stream send failed: "+err.Error()); requeueErr != nil {
			return fmt.Errorf("stream send error: %w; failed to requeue command: %v", err, requeueErr)
		}
		return fmt.Errorf("stream send error: %w", err)
	}

	return nil
}

func (s *ControllerServer) ensureCommandStreamNodeActive(nodePublicKey string) error {
	if s.store == nil || nodePublicKey == "" {
		return nil
	}

	node, err := s.store.GetNode(nodePublicKey)
	if err != nil || node == nil {
		return status.Error(codes.Unauthenticated, "node not found")
	}
	if isInactiveNodeStatus(node.Status) {
		return status.Errorf(codes.PermissionDenied, "node access denied: status '%s'", node.Status)
	}
	return nil
}

func isTerminalCommandStatus(status string) bool {
	switch status {
	case controllerstorage.AgentCommandStatusCompleted, controllerstorage.AgentCommandStatusFailed:
		return true
	default:
		return false
	}
}

// ReportMetrics 处理 Agent 指标上报
func (s *ControllerServer) ReportMetrics(ctx context.Context, req *agentpb.MetricsReportRequest) (*agentpb.MetricsReportResponse, error) {
	node, err := s.resolveMetricsNodeForRequest(ctx, req)
	if err != nil {
		fmt.Printf("[WARN] Dropping metrics from unknown/unauthorized node: %v\n", err)
		return &agentpb.MetricsReportResponse{Success: false, Message: "Unauthorized node"}, nil
	}

	// Only touch heartbeat fields here. SaveNode is registration-oriented and
	// rewrites node metadata/status, which is too broad for metrics heartbeats.
	if err := s.store.UpdateNodeHeartbeat(node.ID, time.Now().Unix()); err != nil {
		fmt.Printf("[ERROR] Failed to update node heartbeat: %v\n", err)
		return &agentpb.MetricsReportResponse{Success: false, Message: "Failed to persist metrics"}, fmt.Errorf("failed to persist metrics: %w", err)
	}
	if stats := policyStatsFromCustomMetrics(req.CustomMetrics); len(stats) > 0 {
		if err := s.store.UpsertNodePolicyStats(node.TenantID, node.ID, stats); err != nil {
			fmt.Printf("[ERROR] Failed to update node policy stats: %v\n", err)
			return &agentpb.MetricsReportResponse{Success: false, Message: "Failed to persist policy stats"}, fmt.Errorf("failed to persist policy stats: %w", err)
		}
	}

	return &agentpb.MetricsReportResponse{
		Success: true,
		Message: "Metrics reported successfully",
	}, nil
}

func policyStatsFromCustomMetrics(metrics map[string]float64) map[string]interface{} {
	if len(metrics) == 0 {
		return nil
	}
	keys := []string{
		"acl_packets",
		"acl_bytes",
		"acl_dropped_packets",
		"acl_dropped_bytes",
		"qos_passed_bytes",
		"qos_dropped_bytes",
		"qos_shaped_bytes",
	}
	stats := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if value, ok := metrics[key]; ok {
			stats[key] = value
		}
	}
	aclRules := make(map[string]map[string]interface{})
	qosRules := make(map[string]map[string]interface{})
	for key, value := range metrics {
		parts := strings.Split(key, ".")
		if len(parts) != 3 {
			continue
		}
		domain, ruleID, metric := parts[0], parts[1], parts[2]
		if strings.TrimSpace(ruleID) == "" {
			continue
		}
		switch domain {
		case "acl_rule":
			if !allowedACLRuleStatMetric(metric) {
				continue
			}
			if aclRules[ruleID] == nil {
				aclRules[ruleID] = make(map[string]interface{})
			}
			aclRules[ruleID][metric] = value
		case "qos_rule":
			if !allowedQoSRuleStatMetric(metric) {
				continue
			}
			if qosRules[ruleID] == nil {
				qosRules[ruleID] = make(map[string]interface{})
			}
			qosRules[ruleID][metric] = value
		}
	}
	if len(aclRules) > 0 {
		stats["acl_rules"] = mapStringRuleStats(aclRules)
	}
	if len(qosRules) > 0 {
		stats["qos_rules"] = mapStringRuleStats(qosRules)
	}
	if len(stats) == 0 {
		return nil
	}
	stats["reported_at"] = time.Now().UTC().Format(time.RFC3339)
	return stats
}

func allowedACLRuleStatMetric(metric string) bool {
	switch metric {
	case "packets", "bytes", "dropped_packets", "dropped_bytes":
		return true
	default:
		return false
	}
}

func allowedQoSRuleStatMetric(metric string) bool {
	switch metric {
	case "passed_bytes", "dropped_bytes", "shaped_bytes":
		return true
	default:
		return false
	}
}

func mapStringRuleStats(input map[string]map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{}, len(input))
	for ruleID, stats := range input {
		output[ruleID] = stats
	}
	return output
}

func generateRuntimeTokenForNode(node *controllerstorage.Node) (string, int64, error) {
	if node == nil {
		return "", 0, fmt.Errorf("node is required")
	}
	token, expiresAt, err := auth.GenerateRuntimeToken(node.ID.String(), node.TenantID.String())
	if err != nil {
		return "", 0, err
	}
	return token, expiresAt.Unix(), nil
}

func domainVersionsFromDesiredVersion(version string) map[string]string {
	version = strings.TrimSpace(version)
	if version == "" {
		return map[string]string{}
	}

	return map[string]string{
		"peer":        version,
		"acl":         version,
		"qos":         version,
		"route":       version,
		"blacklist":   version,
		"certificate": version,
	}
}

func (s *ControllerServer) reportRuntimeSyncState(node *controllerstorage.Node, req *agentpb.SyncRequest) error {
	if s.store == nil || node == nil {
		return nil
	}

	now := time.Now()
	lastSyncError := ""
	if req.ObservedState == "error" {
		lastSyncError = req.ObservedMessage
	}

	_, err := s.store.ReportNodeControlState(node.TenantID, node.ID, controllerstorage.NodeControlStateReport{
		AppliedStateVersion: req.AppliedStateVersion,
		ObservedState:       req.ObservedState,
		ObservedMessage:     req.ObservedMessage,
		LastSyncAt:          &now,
		LastSyncError:       lastSyncError,
	})
	if err != nil {
		return err
	}

	publicIP, endpoint := nodeidentity.NormalizeReportedNetwork(req.PublicIp, req.Endpoint)
	return s.store.UpdateNodePublicIdentity(node.ID, publicIP, endpoint)
}

func (s *ControllerServer) ensureDesiredStateVersion(node *controllerstorage.Node) (string, error) {
	if s.store == nil || node == nil {
		return "", nil
	}

	state, err := s.store.GetNodeControlState(node.TenantID, node.ID)
	if err != nil {
		return "", err
	}
	if state != nil && state.DesiredStateVersion != "" {
		return state.DesiredStateVersion, nil
	}

	created, err := s.store.UpsertNodeDesiredState(node.TenantID, node.ID, controllerstorage.NewDesiredStateVersion(), map[string]interface{}{
		"source": "sync-baseline",
	})
	if err != nil {
		return "", err
	}
	return created.DesiredStateVersion, nil
}

func (s *ControllerServer) resolveRuntimeNode(nodeID, publicKey string) (*controllerstorage.Node, error) {
	if s.store == nil {
		return nil, fmt.Errorf("controller storage is not configured")
	}

	var nodeByNodeID, nodeByPubKey *controllerstorage.Node

	// 1. 尝试按 node_id 查找
	if nodeID != "" {
		parsedNodeID, err := uuid.Parse(nodeID)
		if err == nil {
			nodeByNodeID, _ = s.store.GetNodeByID(parsedNodeID)
		}
	}

	// 2. 尝试按 public_key 查找
	if publicKey != "" {
		nodeByPubKey, _ = s.store.GetNode(publicKey)
	}

	// 3. 身份冲突校验
	if nodeByNodeID != nil && nodeByPubKey != nil {
		if nodeByNodeID.ID != nodeByPubKey.ID {
			return nil, fmt.Errorf("SECURITY ALERT: identity mismatch between node_id and public_key")
		}
	}

	// 4. 确定最终节点对象
	resolvedNode := nodeByNodeID
	if resolvedNode == nil {
		resolvedNode = nodeByPubKey
	}

	if resolvedNode == nil {
		return nil, fmt.Errorf("node identity not found (node_id: %s, pubkey: %s)", nodeID, publicKey)
	}

	// 5. 状态拦截：禁止已停用节点接入
	if isInactiveNodeStatus(resolvedNode.Status) {
		return nil, fmt.Errorf("node access denied: current status is '%s'", resolvedNode.Status)
	}

	return resolvedNode, nil
}

func (s *ControllerServer) resolveRuntimeNodeForRequest(ctx context.Context, nodeID, publicKey string) (*controllerstorage.Node, error) {
	node, err := s.resolveRuntimeNode(nodeID, publicKey)
	if err != nil {
		return nil, err
	}
	if err := bindRuntimeTokenToNode(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

func bindRuntimeTokenToNode(ctx context.Context, node *controllerstorage.Node) error {
	if node == nil {
		return status.Error(codes.Unauthenticated, "runtime node is required")
	}

	tokenNodeID, ok := GetRuntimeNodeID(ctx)
	if !ok || strings.TrimSpace(tokenNodeID) == "" {
		return status.Error(codes.Unauthenticated, "runtime token node missing")
	}

	parsedTokenNodeID, err := uuid.Parse(tokenNodeID)
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid runtime token node")
	}
	if parsedTokenNodeID != node.ID {
		return status.Error(codes.PermissionDenied, "runtime token node mismatch")
	}

	tokenTenantID, ok := GetRuntimeTenantID(ctx)
	if !ok || strings.TrimSpace(tokenTenantID) == "" {
		return status.Error(codes.Unauthenticated, "runtime token tenant missing")
	}
	parsedTokenTenantID, err := uuid.Parse(tokenTenantID)
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid runtime token tenant")
	}
	if parsedTokenTenantID != node.TenantID {
		return status.Error(codes.PermissionDenied, "runtime token tenant mismatch")
	}

	return nil
}

func (s *ControllerServer) resolveLegacyAgentIdentity(agentID string) (*controllerstorage.Node, error) {
	if s.store == nil {
		return nil, fmt.Errorf("controller storage is not configured")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent identity is required")
	}

	var (
		node *controllerstorage.Node
		err  error
	)
	if parsedNodeID, parseErr := uuid.Parse(agentID); parseErr == nil {
		node, err = s.store.GetNodeByID(parsedNodeID)
	} else {
		node, err = s.store.GetNode(agentID)
	}
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node not found")
	}
	if isInactiveNodeStatus(node.Status) {
		return nil, status.Errorf(codes.PermissionDenied, "node access denied: status '%s'", node.Status)
	}
	return node, nil
}

func isInactiveNodeStatus(nodeStatus string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeStatus)) {
	case "deleted", "suspended", "banned":
		return true
	default:
		return false
	}
}

func (s *ControllerServer) resolveCommandStreamNodeForRequest(ctx context.Context, resp *agentpb.CommandResponse) (*controllerstorage.Node, error) {
	node, err := s.resolveNodeFromCommandStream(resp)
	if err != nil {
		return nil, err
	}
	if err := bindRuntimeTokenToNode(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (s *ControllerServer) resolveNodeFromCommandStream(resp *agentpb.CommandResponse) (*controllerstorage.Node, error) {
	if resp == nil {
		return nil, fmt.Errorf("command stream init payload is required")
	}

	if node, err := s.resolveRuntimeNode(resp.NodeId, resp.PublicKey); err == nil {
		return node, nil
	}

	if identity, ok := resp.Result["node_id"]; ok && identity != "" {
		if node, err := s.resolveRuntimeNode(identity, resp.Result["public_key"]); err == nil {
			return node, nil
		}
	}

	if identity, ok := resp.Result["agent_id"]; ok && identity != "" {
		node, err := s.resolveLegacyAgentIdentity(identity)
		if err == nil {
			return node, nil
		}
	}

	if identity, ok := resp.Result["public_key"]; ok && identity != "" {
		node, err := s.resolveRuntimeNode("", identity)
		if err == nil {
			return node, nil
		}
	}

	return nil, fmt.Errorf("failed to resolve node identity from command stream init")
}

func (s *ControllerServer) resolveMetricsNode(req *agentpb.MetricsReportRequest) (*controllerstorage.Node, error) {
	if req == nil {
		return nil, fmt.Errorf("metrics request is required")
	}

	if node, err := s.resolveRuntimeNode(req.NodeId, req.PublicKey); err == nil {
		return node, nil
	}

	if req.AgentId != "" {
		node, err := s.resolveLegacyAgentIdentity(req.AgentId)
		if err == nil {
			return node, nil
		}
	}

	return nil, fmt.Errorf("node_id or public_key is required for metrics reporting")
}

func (s *ControllerServer) resolveMetricsNodeForRequest(ctx context.Context, req *agentpb.MetricsReportRequest) (*controllerstorage.Node, error) {
	node, err := s.resolveMetricsNode(req)
	if err != nil {
		return nil, err
	}
	if err := bindRuntimeTokenToNode(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

// 辅助函数：从 map 中安全获取字符串
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// 辅助函数：从 map 中安全获取字符串数组
func getStringSlice(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, v := range slice {
				if str, ok := v.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// 辅助函数：从 map 中安全获取 uint32
func getUint32(m map[string]interface{}, key string) uint32 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return uint32(v)
		case int:
			return uint32(v)
		case uint32:
			return v
		}
	}
	return 0
}

func defaultACLAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return "allow"
	}
	return action
}

// getQoSRules 查询适用于该 Agent 的 QoS 规则
func (s *ControllerServer) getQoSRules(ctx context.Context, publicKey string) ([]*agentpb.QoSRule, error) {
	var qosRules []*agentpb.QoSRule
	_ = ctx

	// 防御性编程：确保 store 存在
	if s.store == nil {
		return qosRules, nil // 返回空列表
	}

	rules, err := s.store.GetNodeQoSRulesByPublicKey(publicKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return qosRules, nil
		}
		return qosRules, err
	}

	for _, rule := range rules {
		qosRules = append(qosRules, &agentpb.QoSRule{
			Id:            rule.ID.String(),
			SrcIp:         rule.SrcCIDR,
			DstIp:         rule.DstCIDR,
			SrcPort:       uint32(rule.SrcPort),
			DstPort:       uint32(rule.DstPort),
			Protocol:      uint32(rule.Protocol),
			BandwidthMbps: uint64(rule.BandwidthMbps),
			Direction:     rule.Direction,
			RateBps:       rule.RateBps,
			BurstBytes:    rule.BurstBytes,
			Priority:      uint32(rule.Priority),
			Mode:          rule.Mode,
		})
	}

	return qosRules, nil
}

func (s *ControllerServer) getBlacklistRules(ctx context.Context, publicKey string) ([]*agentpb.BlacklistRule, error) {
	var blacklistRules []*agentpb.BlacklistRule
	_ = ctx

	if s.store == nil {
		return blacklistRules, nil
	}

	rules, err := s.store.GetNodeBlacklistRulesByPublicKey(publicKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return blacklistRules, nil
		}
		return blacklistRules, err
	}

	for _, rule := range rules {
		blacklistRules = append(blacklistRules, &agentpb.BlacklistRule{
			Scope: rule.Scope,
			Cidr:  rule.CIDR,
			Port:  uint32(rule.Port),
		})
	}

	return blacklistRules, nil
}
