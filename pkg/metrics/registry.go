package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// CommonLabels 公共标签
type CommonLabels struct {
	HostID  string // 设备唯一 ID
	Version string // Aria 版本
	Region  string // 区域
	Role    string // 角色（agent/controller）
}

// NewRegistry 创建带公共标签的 Prometheus Registry
func NewRegistry(labels CommonLabels) *prometheus.Registry {
	registry := prometheus.NewRegistry()

	// 构建公共标签 map
	commonLabels := prometheus.Labels{
		"host_id":      labels.HostID,
		"aria_version": labels.Version, // 改名避免与 go_info 的 version 冲突
		"role":         labels.Role,
	}

	// 仅在 region 非空时添加
	if labels.Region != "" {
		commonLabels["region"] = labels.Region
	}

	// 注册 Go runtime 指标（带公共标签）
	goCollector := prometheus.NewGoCollector()
	wrapped := prometheus.WrapRegistererWith(commonLabels, registry)
	wrapped.MustRegister(goCollector)

	return registry
}

// WrapRegistererWithLabels 包装 Registerer 并注入公共标签
func WrapRegistererWithLabels(registry *prometheus.Registry, labels CommonLabels) prometheus.Registerer {
	commonLabels := prometheus.Labels{
		"host_id":      labels.HostID,
		"aria_version": labels.Version,
		"role":         labels.Role,
	}

	if labels.Region != "" {
		commonLabels["region"] = labels.Region
	}

	return prometheus.WrapRegistererWith(commonLabels, registry)
}
