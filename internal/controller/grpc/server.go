package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aria/pkg/grpc/agentpb"
)

// ControllerServer 实现 gRPC ControllerService
// 通过包装 REST API handler 来复用业务逻辑
type ControllerServer struct {
	agentpb.UnimplementedControllerServiceServer
	registerHandler func(interface{}) (assignedIP, metricsGateway string, err error)
	syncHandler     func(publicKey string) (peers interface{}, assignedIP string, aclRules interface{}, metricsGateway string, err error)
}

// NewControllerServer 创建新的 gRPC 服务端
// 参数是 REST API 的 handler 函数，用于复用逻辑
func NewControllerServer(
	registerHandler func(interface{}) (string, string, error),
	syncHandler func(string) (interface{}, string, interface{}, string, error),
) *ControllerServer {
	return &ControllerServer{
		registerHandler: registerHandler,
		syncHandler:     syncHandler,
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
	}

	// 调用 REST API handler
	assignedIP, metricsGateway, err := s.registerHandler(restReq)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	return &agentpb.RegisterResponse{
		AssignedIp:         assignedIP,
		MetricsPushGateway: metricsGateway,
	}, nil
}

// Sync 处理 Agent 定期同步请求
// 复用 REST API 的 HandleSync 逻辑
func (s *ControllerServer) Sync(ctx context.Context, req *agentpb.SyncRequest) (*agentpb.SyncResponse, error) {
	// 验证公钥
	if req.PublicKey == "" {
		return nil, fmt.Errorf("public_key is required")
	}

	// 调用 REST API handler
	peersInterface, assignedIP, aclRulesInterface, metricsGateway, err := s.syncHandler(req.PublicKey)
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
					})
				}
			}
		}
	}

	return &agentpb.SyncResponse{
		Peers:              peers,
		AssignedIp:         assignedIP,
		LastUpdate:         time.Now().Unix(),
		AclRules:           aclRules,
		MetricsPushGateway: metricsGateway,
	}, nil
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
