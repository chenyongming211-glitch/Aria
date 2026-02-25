// Package routing 实现 SD-WAN 路由选择算法
package routing

import "net"

// Router 定义路由选择器接口
type Router interface {
	// SelectPath 根据目标地址选择最优路径
	SelectPath(dst net.IP) (*Path, error)
	// UpdateMetrics 更新链路质量指标
	UpdateMetrics(pathID string, metrics *LinkMetrics)
}

// Path 表示一条可用路径
type Path struct {
	ID       string
	Endpoint string
	Metrics  *LinkMetrics
	Priority int
}

// LinkMetrics 链路质量指标
type LinkMetrics struct {
	Latency    float64 // 延迟（毫秒）
	Jitter     float64 // 抖动（毫秒）
	PacketLoss float64 // 丢包率（0-1）
	Bandwidth  uint64  // 带宽（bps）
}

// SelectionPolicy 路径选择策略
type SelectionPolicy int

const (
	// PolicyLatency 最低延迟优先
	PolicyLatency SelectionPolicy = iota
	// PolicyBandwidth 最高带宽优先
	PolicyBandwidth
	// PolicyCost 最低成本优先
	PolicyCost
	// PolicyLoadBalance 负载均衡
	PolicyLoadBalance
)
