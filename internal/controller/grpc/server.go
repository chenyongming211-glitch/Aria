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
	registerHandler func(interface{}) (assignedIP, metricsGateway string, err error)
	syncHandler     func(publicKey string) (peers interface{}, assignedIP string, aclRules interface{}, metricsGateway string, err error)
	store           *controllerstorage.Storage
}

var commandStreamPollInterval = 2 * time.Second

// NewControllerServer 创建新的 gRPC 服务端
// 参数是 REST API 的 handler 函数，用于复用逻辑
func NewControllerServer(
	registerHandler func(interface{}) (string, string, error),
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
// 复用 REST API 的 HandleRegister 逻辑
func (s *ControllerServer) Register(ctx context.Context, req *agentpb.RegisterRequest) (*agentpb.RegisterResponse, error) {
	// 将 protobuf 请求转换为 REST API 请求格式
	restReq := map[string]interface{}{
		"public_key":        req.PublicKey,
		"endpoint":          req.Endpoint,
		"private_ip":        req.PrivateIp,
		"public_ip":         req.PublicIp,
		"hostname":          req.Hostname,
		"registered_at":     req.RegisteredAt,
		"token":             req.Token,
		"advertised_routes": req.AdvertisedRoutes,
		"region":            req.Region,
		"customer_id":       req.CustomerId,
		"runtime_mode":      req.RuntimeMode,
		"kernel_version":    req.KernelVersion,
		"has_aesni":         req.HasAesni,
		"machine_id":        req.MachineId,
	}

	// 调用 REST API handler
	assignedIP, metricsGateway, err := s.registerHandler(restReq)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	nodeID := registeredNodeID(s.store, req.PublicKey)

	// 生成运行期凭据
	if nodeID == "" {
		return nil, fmt.Errorf("registration failed: registered node was not persisted")
	}
	node, nodeErr := s.store.GetNodeByID(parseUUIDOrZero(nodeID))
	if nodeErr != nil {
		return nil, fmt.Errorf("registration failed: failed to load registered node: %w", nodeErr)
	}
	if node == nil {
		return nil, fmt.Errorf("registration failed: registered node was not found")
	}
	runtimeToken, runtimeTokenExpiresAt, err := generateRuntimeTokenForNode(node)
	if err != nil {
		return nil, fmt.Errorf("registration failed: failed to issue runtime token: %w", err)
	}

	return &agentpb.RegisterResponse{
		AssignedIp:            assignedIP,
		MetricsPushGateway:    metricsGateway,
		NodeId:                nodeID,
		RuntimeToken:          runtimeToken,
		RuntimeTokenExpiresAt: runtimeTokenExpiresAt,
	}, nil
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

	// 转换 ACL Rules
	var aclRules []*agentpb.ACLRule
	if aclRulesInterface != nil {
		if aclBytes, err := json.Marshal(aclRulesInterface); err == nil {
			var ruleList []map[string]interface{}
			if err := json.Unmarshal(aclBytes, &ruleList); err == nil {
				for _, r := range ruleList {
					aclRules = append(aclRules, &agentpb.ACLRule{
						SrcNet:   getString(r, "src_net"),
						DstNet:   getString(r, "dst_net"),
						Protocol: getUint32(r, "protocol"),
						MinPort:  getUint32(r, "min_port"),
						MaxPort:  getUint32(r, "max_port"),
						Action:   defaultACLAction(getString(r, "action")),
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
		AclRules:              aclRules,
		MetricsPushGateway:    metricsGateway,
		QosRules:              qosRules,
		BlacklistRules:        blacklistRules,
		DesiredStateVersion:   desiredVersion,
		RuntimeToken:          runtimeToken,
		RuntimeTokenExpiresAt: runtimeTokenExpiresAt,
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
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "deleted", "suspended", "banned":
		return status.Errorf(codes.PermissionDenied, "node access denied: status '%s'", node.Status)
	default:
		return nil
	}
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

	// 仅对合法授权节点更新心跳
	node.LastSeen = time.Now().Unix()
	if err := s.store.SaveNode(node); err != nil {
		fmt.Printf("[ERROR] Failed to update node heartbeat: %v\n", err)
	}

	return &agentpb.MetricsReportResponse{
		Success: true,
		Message: "Metrics reported successfully",
	}, nil
}

func registeredNodeID(store *controllerstorage.Storage, publicKey string) string {
	if store == nil || publicKey == "" {
		return ""
	}

	node, err := store.GetNode(publicKey)
	if err != nil || node == nil {
		return ""
	}

	return node.ID.String()
}

func parseUUIDOrZero(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
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
	return err
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

	// 5. ❌ 状态拦截：禁止已停用节点接入
	if resolvedNode.Status == "deleted" || resolvedNode.Status == "suspended" || resolvedNode.Status == "banned" {
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

	if parsedNodeID, err := uuid.Parse(agentID); err == nil {
		return s.store.GetNodeByID(parsedNodeID)
	}

	return s.store.GetNode(agentID)
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
			SrcIp:         rule.SrcCIDR,
			DstIp:         rule.DstCIDR,
			SrcPort:       uint32(rule.SrcPort),
			DstPort:       uint32(rule.DstPort),
			Protocol:      uint32(rule.Protocol),
			BandwidthMbps: uint64(rule.BandwidthMbps),
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
