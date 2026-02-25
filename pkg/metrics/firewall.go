package metrics

import (
	"context"
	"log"

	"aria/pkg/datapath"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// FirewallDropPacketsTotal 防火墙丢弃的数据包总数
	FirewallDropPacketsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_firewall_drop_packets_total",
			Help: "Total packets dropped by firewall ACL rules",
		},
	)

	// FirewallDropBytesTotal 防火墙丢弃的字节总数
	FirewallDropBytesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_firewall_drop_bytes_total",
			Help: "Total bytes dropped by firewall ACL rules",
		},
	)

	// FirewallInvalidPacketsTotal 无效数据包总数
	FirewallInvalidPacketsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_firewall_invalid_packets_total",
			Help: "Total invalid packets (ct state invalid)",
		},
	)

	// FirewallRuleCount 当前活动的 ACL 规则数量
	FirewallRuleCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_firewall_rule_count",
			Help: "Number of active ACL rules",
		},
	)

	// TrafficBytesTotal 按协议分类的流量总数
	TrafficBytesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_traffic_bytes_total",
			Help: "Total traffic by protocol (Layer 4)",
		},
		[]string{"proto"}, // tcp, udp, icmp
	)
)

// FirewallCollector 防火墙和流量统计采集器
type FirewallCollector struct {
	firewall datapath.FirewallManager
}

// NewFirewallCollector 创建防火墙采集器
func NewFirewallCollector(firewall datapath.FirewallManager, registerer prometheus.Registerer) *FirewallCollector {
	registerer.MustRegister(
		FirewallDropPacketsTotal,
		FirewallDropBytesTotal,
		FirewallInvalidPacketsTotal,
		FirewallRuleCount,
		TrafficBytesTotal,
	)

	return &FirewallCollector{
		firewall: firewall,
	}
}

// Name 返回采集器名称
func (fc *FirewallCollector) Name() string {
	return "firewall"
}

// Collect 采集防火墙和流量指标
func (fc *FirewallCollector) Collect(ctx context.Context) error {
	stats, err := fc.firewall.GetStats()
	if err != nil {
		return err
	}

	FirewallDropPacketsTotal.Set(float64(stats.DroppedPackets))
	FirewallDropBytesTotal.Set(float64(stats.DroppedBytes))
	FirewallInvalidPacketsTotal.Set(float64(stats.InvalidPackets))
	FirewallRuleCount.Set(float64(stats.RuleCount))

	// 协议流量分布
	TrafficBytesTotal.WithLabelValues("tcp").Set(float64(stats.TCPBytes))
	TrafficBytesTotal.WithLabelValues("udp").Set(float64(stats.UDPBytes))
	TrafficBytesTotal.WithLabelValues("icmp").Set(float64(stats.ICMPBytes))

	log.Printf("[metrics] Firewall: drop=%d invalid=%d rules=%d",
		stats.DroppedPackets, stats.InvalidPackets, stats.RuleCount)

	return nil
}
