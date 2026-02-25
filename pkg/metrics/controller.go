package metrics

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// NodesTotal 节点总数（按状态分类）
	NodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_controller_nodes_total",
			Help: "Total number of nodes by status",
		},
		[]string{"status"}, // status: online, offline, stale
	)

	// SyncRequestsTotal 同步请求计数
	SyncRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aria_controller_sync_requests_total",
			Help: "Total number of sync requests received",
		},
	)

	// RegisterRequestsTotal 注册请求计数
	RegisterRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aria_controller_register_requests_total",
			Help: "Total number of register requests received",
		},
	)

	// NodeLastSeen 节点最后上报时间（按 hostname）
	NodeLastSeen = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_controller_node_last_seen_seconds",
			Help: "Node last seen timestamp (Unix timestamp)",
		},
		[]string{"hostname", "vpc_id"},
	)
)

// NodeInfo 节点信息接口（避免直接依赖 controllerstorage）
type NodeInfo interface {
	GetStatus() string
	GetHostname() string
	GetVPCID() string
	GetLastSeen() int64
}

// NodeStorage 节点存储接口
type NodeStorage interface {
	GetAllNodes() ([]NodeInfo, error)
}

// ControllerCollector Controller 采集器
type ControllerCollector struct {
	storage NodeStorage
}

// NewControllerCollector 创建 Controller 采集器
func NewControllerCollector(storage NodeStorage, registerer prometheus.Registerer) *ControllerCollector {
	// 注册指标
	registerer.MustRegister(NodesTotal, SyncRequestsTotal, RegisterRequestsTotal, NodeLastSeen)

	return &ControllerCollector{
		storage: storage,
	}
}

// Name 返回采集器名称
func (cc *ControllerCollector) Name() string {
	return "controller"
}

// Collect 采集 Controller 指标
func (cc *ControllerCollector) Collect(ctx context.Context) error {
	nodes, err := cc.storage.GetAllNodes()
	if err != nil {
		return err
	}

	// 按状态统计
	statusCount := make(map[string]int)
	now := time.Now().Unix()

	for _, node := range nodes {
		status := node.GetStatus()

		// 如果 status 为空，根据 LastSeen 判断
		if status == "" {
			if now-node.GetLastSeen() > 300 { // 5分钟未上报
				status = "offline"
			} else {
				status = "online"
			}
		}

		statusCount[status]++

		// 记录节点最后上报时间
		NodeLastSeen.WithLabelValues(node.GetHostname(), node.GetVPCID()).Set(float64(node.GetLastSeen()))
	}

	// 更新指标
	for status, count := range statusCount {
		NodesTotal.WithLabelValues(status).Set(float64(count))
	}

	// 确保所有状态都有指标（即使为 0）
	for _, status := range []string{"online", "offline", "stale"} {
		if _, exists := statusCount[status]; !exists {
			NodesTotal.WithLabelValues(status).Set(0)
		}
	}

	log.Printf("[metrics] Controller: collected %d nodes", len(nodes))
	return nil
}

// IncrementSyncRequests 增加同步请求计数（由 controller handler 调用）
func IncrementSyncRequests() {
	SyncRequestsTotal.Inc()
}

// IncrementRegisterRequests 增加注册请求计数（由 controller handler 调用）
func IncrementRegisterRequests() {
	RegisterRequestsTotal.Inc()
}
