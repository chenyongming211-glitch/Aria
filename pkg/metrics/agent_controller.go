package metrics

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// AgentControllerConnected Agent 到 Controller 的连接状态（1=connected, 0=disconnected）
	AgentControllerConnected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_agent_controller_connected",
			Help: "Agent controller connection status (1=connected, 0=disconnected)",
		},
	)

	// AgentControllerSyncLatency Agent 同步延迟（秒）
	AgentControllerSyncLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "aria_agent_controller_sync_latency_seconds",
			Help:    "Agent controller sync request latency",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
		},
	)

	// AgentControllerSyncErrors Agent 同步错误计数（按错误类型）
	// 使用 Counter 而非 GaugeVec，防止标签泄漏
	AgentControllerSyncErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aria_agent_controller_sync_errors_total",
			Help: "Total number of agent controller sync errors by type",
		},
		[]string{"error_type"}, // timeout, http_error, parse_error
	)

	// ConfigSyncLastSuccess 最后成功同步配置的时间戳
	ConfigSyncLastSuccess = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_config_sync_last_success_timestamp",
			Help: "Timestamp of last successful config sync (Unix timestamp)",
		},
	)

	// ConfigSyncFailureCount 配置同步失败总数
	ConfigSyncFailureCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aria_config_sync_failure_count",
			Help: "Total number of config sync failures",
		},
	)

	// ConfigCacheAge 配置缓存年龄（秒）
	ConfigCacheAge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_config_cache_age_seconds",
			Help: "Age of cached configuration (seconds since last update)",
		},
		[]string{"config_type"}, // peers, acl_rules
	)
)

// AgentStorageInterface 定义 storage 接口，避免循环依赖
type AgentStorageInterface interface {
	GetPeerUpdateTime() (int64, error)
	GetACLUpdateTime() (int64, error)
}

// AgentControllerCollector Agent Controller 连接状态采集器
type AgentControllerCollector struct {
	lastSyncTime    time.Time
	lastSyncSuccess bool
	lastError       error
	storage         AgentStorageInterface
	mu              sync.RWMutex
}

// NewAgentControllerCollector 创建 Agent Controller 采集器
func NewAgentControllerCollector(registerer prometheus.Registerer, storage AgentStorageInterface) *AgentControllerCollector {
	// 注册指标
	registerer.MustRegister(
		AgentControllerConnected,
		AgentControllerSyncLatency,
		AgentControllerSyncErrors,
		ConfigSyncLastSuccess,
		ConfigSyncFailureCount,
		ConfigCacheAge,
	)

	return &AgentControllerCollector{
		storage: storage,
	}
}

// Name 返回采集器名称
func (acc *AgentControllerCollector) Name() string {
	return "agent_controller"
}

// Collect 采集 Agent Controller 连接状态
func (acc *AgentControllerCollector) Collect(ctx context.Context) error {
	acc.mu.RLock()
	defer acc.mu.RUnlock()

	// 如果超过 60 秒没有同步，标记为断开
	if time.Since(acc.lastSyncTime) > 60*time.Second {
		AgentControllerConnected.Set(0)
	}

	// 采集配置缓存年龄
	if acc.storage != nil {
		now := time.Now().Unix()

		// Peer 缓存年龄
		if peerUpdateTime, err := acc.storage.GetPeerUpdateTime(); err == nil && peerUpdateTime > 0 {
			ConfigCacheAge.WithLabelValues("peers").Set(float64(now - peerUpdateTime))
		}

		// ACL 缓存年龄
		if aclUpdateTime, err := acc.storage.GetACLUpdateTime(); err == nil && aclUpdateTime > 0 {
			ConfigCacheAge.WithLabelValues("acl_rules").Set(float64(now - aclUpdateTime))
		}
	}

	return nil
}

// RecordSync 记录同步结果
func (acc *AgentControllerCollector) RecordSync(duration time.Duration, err error) {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	acc.lastSyncTime = time.Now()
	acc.lastSyncSuccess = (err == nil)
	acc.lastError = err

	AgentControllerSyncLatency.Observe(duration.Seconds())

	if err == nil {
		AgentControllerConnected.Set(1)
		ConfigSyncLastSuccess.SetToCurrentTime()
		// 成功时无需额外操作（Counter 不会自动递减）
	} else {
		AgentControllerConnected.Set(0)
		ConfigSyncFailureCount.Inc()
		// 记录错误类型计数
		AgentControllerSyncErrors.WithLabelValues(classifyAgentControllerError(err)).Inc()

		// 详细错误信息记录到日志（便于运维排查）
		log.Printf("[metrics] Agent controller sync failed (%s): %v", classifyAgentControllerError(err), err)
	}
}

// classifyAgentControllerError 对错误进行分类
func classifyAgentControllerError(err error) string {
	if err == nil {
		return "none"
	}
	errStr := err.Error()
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return "timeout"
	}
	if strings.Contains(errStr, "HTTP") || strings.Contains(errStr, "status") {
		return "http_error"
	}
	return "parse_error"
}
