package metrics

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector 定义指标采集器接口
type Collector interface {
	// Collect 执行指标采集（带超时控制）
	Collect(ctx context.Context) error
	// Name 返回采集器名称
	Name() string
}

// CollectorManager 管理所有采集器
type CollectorManager struct {
	collectors []Collector
	registry   *prometheus.Registry
	mu         sync.RWMutex
}

// NewCollectorManager 创建采集器管理器
func NewCollectorManager(registry *prometheus.Registry) *CollectorManager {
	return &CollectorManager{
		collectors: make([]Collector, 0),
		registry:   registry,
	}
}

// Register 注册采集器
func (m *CollectorManager) Register(collector Collector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectors = append(m.collectors, collector)
	log.Printf("[metrics] Registered collector: %s", collector.Name())
}

// CollectAll 执行所有采集器（带超时）
func (m *CollectorManager) CollectAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var firstErr error
	for _, collector := range m.collectors {
		if err := collector.Collect(ctx); err != nil {
			log.Printf("[metrics] Collector %s failed: %v", collector.Name(), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if firstErr != nil {
		return fmt.Errorf("collection errors occurred: %w", firstErr)
	}
	return nil
}

// Registry 返回 Prometheus 注册表
func (m *CollectorManager) Registry() *prometheus.Registry {
	return m.registry
}

// Count 返回已注册的采集器数量
func (m *CollectorManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.collectors)
}
