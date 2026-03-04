package v1

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"aria/pkg/controllerstorage"
)

// AgentProxyHandler 处理 Agent 代理 API
type AgentProxyHandler struct {
	store *controllerstorage.Storage
}

// NewAgentProxyHandler 创建新的 Agent 代理处理器
func NewAgentProxyHandler(store *controllerstorage.Storage) *AgentProxyHandler {
	return &AgentProxyHandler{
		store: store,
	}
}

// CommandRequest 命令请求
type CommandRequest struct {
	Command  string                 `json:"command"`  // 命令类型: "sync", "restart", "config_reload", "health_check"
	Params   map[string]interface{} `json:"params"`   // 命令参数
	Timeout  int                    `json:"timeout"`  // 超时时间（秒），默认30
	Priority int                    `json:"priority"` // 优先级，0=普通, 1=高, 2=紧急
}

// CommandResponse 命令响应
type CommandResponse struct {
	CommandID string    `json:"command_id"` // 命令ID
	NodeID    string    `json:"node_id"`    // 节点ID
	Status    string    `json:"status"`     // pending, sent, acknowledged, completed, failed
	Message   string    `json:"message"`    // 状态消息
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentStatus Agent状态
type AgentStatus struct {
	NodeID      string `json:"node_id"`
	Hostname    string `json:"hostname"`
	Region      string `json:"region"`
	PublicIP    string `json:"public_ip"`
	AssignedIP  string `json:"assigned_ip"`
	Status      string `json:"status"`       // online, offline, unknown
	LastSeen    int64  `json:"last_seen"`    // Unix timestamp
	LastCommand string `json:"last_command"` // 最后一条命令ID
	PendingCmds int    `json:"pending_cmds"` // 待执行命令数
	Uptime      int64  `json:"uptime"`       // 运行时长（秒）
}

// BatchCommandRequest 批量命令请求
type BatchCommandRequest struct {
	NodeIDs []string       `json:"node_ids"` // 目标节点ID列表，空=所有节点
	Command CommandRequest `json:"command"`  // 命令内容
}

// BatchCommandResponse 批量命令响应
type BatchCommandResponse struct {
	TotalCount   int               `json:"total_count"`   // 总数
	SuccessCount int               `json:"success_count"` // 成功数
	FailedCount  int               `json:"failed_count"`  // 失败数
	Results      []CommandResponse `json:"results"`       // 每个节点的结果
}

// 错误码
const (
	CodeNodeNotFound    = "NODE_NOT_FOUND"
	CodeCommandFailed   = "COMMAND_FAILED"
	CodeInvalidNodeID   = "INVALID_NODE_ID"
	CodeNodeIDsRequired = "NODE_IDS_REQUIRED"
	CodeNoNodesFound    = "NO_NODES_FOUND"
)

// HandleAgentCommand 处理单个Agent命令
// POST /api/v1/agent/{node_id}/command
func (h *AgentProxyHandler) HandleAgentCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	// 从URL中提取node_id
	nodeID := extractNodeID(r.URL.Path)
	if nodeID == "" {
		WriteError(w, http.StatusBadRequest, CodeInvalidNodeID, "Invalid node ID", nil)
		return
	}

	// 解析请求
	var req CommandRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}

	// 验证命令
	if req.Command == "" {
		WriteError(w, http.StatusBadRequest, CodeBadRequest, "Command is required", nil)
		return
	}

	// 检查节点是否存在（支持通过 public_key 或 hostname 查询）
	var node *controllerstorage.Node
	var err error

	// 首先尝试通过 public_key 查询
	node, err = h.store.GetNode(nodeID)
	if err != nil || node == nil {
		// 如果找不到，尝试通过 hostname 查询
		node, err = h.store.GetNodeByHostname(nodeID)
		if err != nil || node == nil {
			WriteError(w, http.StatusNotFound, CodeNodeNotFound, "Node not found: "+nodeID, nil)
			return
		}
	}

	// 设置默认值
	if req.Timeout == 0 {
		req.Timeout = 30
	}

	// 创建命令记录
	commandID := uuid.New().String()
	now := time.Now()

	// TODO: 将命令存储到Redis或数据库
	// cmd := map[string]interface{}{
	// 	"id":         commandID,
	// 	"node_id":    nodeID,
	// 	"command":    req.Command,
	// 	"params":     req.Params,
	// 	"status":     "pending",
	// 	"priority":   req.Priority,
	// 	"timeout":    req.Timeout,
	// 	"created_at": now,
	// 	"updated_at": now,
	// }
	// store.SaveCommand(cmd)

	response := CommandResponse{
		CommandID: commandID,
		NodeID:    nodeID,
		Status:    "pending",
		Message:   "Command queued for delivery",
		CreatedAt: now,
		UpdatedAt: now,
	}

	WriteSuccess(w, response, "Command queued successfully")
}

// HandleAgentStatus 获取Agent状态
// GET /api/v1/agent/{node_id}/status
func (h *AgentProxyHandler) HandleAgentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	// 从URL中提取node_id
	nodeID := extractNodeID(r.URL.Path)
	if nodeID == "" {
		WriteError(w, http.StatusBadRequest, CodeInvalidNodeID, "Invalid node ID", nil)
		return
	}

	// 获取节点信息（支持通过 public_key 或 hostname 查询）
	var node *controllerstorage.Node
	var err error

	// 首先尝试通过 public_key 查询
	node, err = h.store.GetNode(nodeID)
	if err != nil || node == nil {
		// 如果找不到，尝试通过 hostname 查询
		node, err = h.store.GetNodeByHostname(nodeID)
		if err != nil || node == nil {
			WriteError(w, http.StatusNotFound, CodeNodeNotFound, "Node not found: "+nodeID, nil)
			return
		}
	}

	// 计算状态
	status := "unknown"
	var uptime int64 = 0
	if node.LastSeen > 0 {
		now := time.Now().Unix()
		offlineThreshold := int64(60) // 60秒未响应视为离线
		if now-node.LastSeen < offlineThreshold {
			status = "online"
			uptime = now - node.LastSeen
		} else {
			status = "offline"
		}
	}

	// TODO: 从命令队列查询待执行命令数
	pendingCmds := 0

	response := AgentStatus{
		NodeID:      nodeID,
		Hostname:    node.Hostname,
		Region:      node.Region,
		PublicIP:    node.PublicIP,
		AssignedIP:  node.AssignedIP,
		Status:      status,
		LastSeen:    node.LastSeen,
		PendingCmds: pendingCmds,
		Uptime:      uptime,
	}

	WriteSuccess(w, response, "Agent status retrieved")
}

// HandleBatchCommand 处理批量Agent命令
// POST /api/v1/agents/command
func (h *AgentProxyHandler) HandleBatchCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}

	// 解析请求
	var req BatchCommandRequest
	if err := ParseRequestJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, CodeBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}

	// 获取目标节点列表
	var nodes []*controllerstorage.Node
	var err error

	if len(req.NodeIDs) == 0 {
		// 获取所有节点
		nodes, err = h.store.GetAllNodes()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, CodeGetNodesFailed, "Failed to get nodes: "+err.Error(), nil)
			return
		}
	} else {
		// 获取指定节点
		for _, nodeID := range req.NodeIDs {
			node, err := h.store.GetNode(nodeID)
			if err == nil && node != nil {
				nodes = append(nodes, node)
			}
		}
	}

	if len(nodes) == 0 {
		WriteError(w, http.StatusNotFound, CodeNoNodesFound, "No nodes found", nil)
		return
	}

	// 批量发送命令
	var results []CommandResponse
	var successCount, failedCount int
	now := time.Now()

	for _, node := range nodes {
		commandID := uuid.New().String()

		// TODO: 创建并存储命令记录到队列
		// cmd := map[string]interface{}{
		// 	"id":         commandID,
		// 	"node_id":    node.PublicKey,
		// 	"command":    req.Command.Command,
		// 	"params":     req.Command.Params,
		// 	"status":     "pending",
		// 	"priority":   req.Command.Priority,
		// 	"timeout":    req.Command.Timeout,
		// 	"created_at": now,
		// 	"updated_at": now,
		// }
		// store.SaveCommand(cmd)

		results = append(results, CommandResponse{
			CommandID: commandID,
			NodeID:    node.PublicKey,
			Status:    "pending",
			Message:   "Command queued for delivery",
			CreatedAt: now,
			UpdatedAt: now,
		})
		successCount++
	}

	response := BatchCommandResponse{
		TotalCount:   len(nodes),
		SuccessCount: successCount,
		FailedCount:  failedCount,
		Results:      results,
	}

	WriteSuccess(w, response, strconv.Itoa(successCount)+" commands queued successfully")
}

// extractNodeID 从URL路径中提取节点ID
// 例如: /api/v1/agent/{node_id}/command -> node_id
func extractNodeID(urlPath string) string {
	parts := strings.Split(strings.TrimSuffix(urlPath, "/"), "/")
	for i, part := range parts {
		if part == "agent" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// SetupAgentProxyRoutes 设置Agent代理API路由
func SetupAgentProxyRoutes(mux *http.ServeMux, store *controllerstorage.Storage) {
	handler := NewAgentProxyHandler(store)

	// 单个Agent命令
	mux.HandleFunc("/api/v1/agent/", func(w http.ResponseWriter, r *http.Request) {
		// 检查路径是否匹配 /api/v1/agent/{node_id}/command 或 /api/v1/agent/{node_id}/status
		// 路径分割后: ["", "api", "v1", "agent", "{node_id}", "command" 或 "status"]
		pathParts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
		if len(pathParts) >= 6 && pathParts[5] == "command" {
			handler.HandleAgentCommand(w, r)
		} else if len(pathParts) >= 6 && pathParts[5] == "status" {
			handler.HandleAgentStatus(w, r)
		} else {
			WriteError(w, http.StatusNotFound, CodeEndpointNotFound, "Endpoint not found", nil)
		}
	})

	// 批量Agent命令
	mux.HandleFunc("/api/v1/agents/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleBatchCommand(w, r)
		} else {
			WriteError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "Method not allowed", nil)
		}
	})
}
