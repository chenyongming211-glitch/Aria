package metrics

import (
	"context"
	"log"
	"math"
	"time"

	"aria/pkg/monitor"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ProbeRTT 探测 RTT（毫秒）
	ProbeRTT = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_probe_rtt_milliseconds",
			Help: "Round-trip time to peer in milliseconds",
		},
		[]string{"peer_ip", "public_key"},
	)

	// ProbeLossRatio 丢包率（0.0-1.0）
	ProbeLossRatio = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_probe_loss_ratio",
			Help: "Packet loss ratio (0.0-1.0)",
		},
		[]string{"peer_ip", "public_key"},
	)

	// LinkHealthScore 链路健康分数（0=down, 1=up）
	LinkHealthScore = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_link_health_score",
			Help: "Link health score (0=down, 1=up)",
		},
		[]string{"peer_ip", "public_key"},
	)

	// LinkFailureCount 链路失败次数
	LinkFailureCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_link_failure_count",
			Help: "Number of link failures since start",
		},
		[]string{"peer_ip", "public_key"},
	)

	// ProbeJitter RTT 抖动（标准差）
	ProbeJitter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_probe_jitter_milliseconds",
			Help: "RTT standard deviation (jitter) in milliseconds",
		},
		[]string{"peer_ip", "public_key"},
	)
)

// HealthCollector 链路质量采集器
type HealthCollector struct {
	prober *monitor.Prober
	health *monitor.HealthManager
}

// NewHealthCollector 创建链路质量采集器
func NewHealthCollector(prober *monitor.Prober, health *monitor.HealthManager, registerer prometheus.Registerer) *HealthCollector {
	// 注册指标
	registerer.MustRegister(ProbeRTT, ProbeLossRatio, LinkHealthScore, LinkFailureCount, ProbeJitter)

	return &HealthCollector{
		prober: prober,
		health: health,
	}
}

// Name 返回采集器名称
func (hc *HealthCollector) Name() string {
	return "health"
}

// Collect 采集链路质量指标
func (hc *HealthCollector) Collect(ctx context.Context) error {
	// 获取所有 peer 的探测数据
	peers := hc.prober.GetAllPeers()

	for pubKey, peer := range peers {
		// 截断 public key
		pubKeyShort := pubKey
		if len(pubKeyShort) > 16 {
			pubKeyShort = pubKeyShort[:16]
		}

		peerIP := peer.IP

		// 获取 RTT 和丢包率（使用 Prober 的方法）
		rtt, loss, ok := hc.prober.GetPeerStats(pubKey)
		if ok {
			// 有 RTT 数据
			ProbeRTT.WithLabelValues(peerIP, pubKeyShort).Set(float64(rtt.Milliseconds()))
			ProbeLossRatio.WithLabelValues(peerIP, pubKeyShort).Set(loss / 100.0)

			// 计算抖动（标准差）
			if len(peer.RTTs) >= 2 {
				jitter := calculateJitter(peer.RTTs)
				ProbeJitter.WithLabelValues(peerIP, pubKeyShort).Set(jitter)
			}
		} else {
			// 没有 RTT 数据，设置为 0（表示无法探测）
			ProbeRTT.WithLabelValues(peerIP, pubKeyShort).Set(0)
			ProbeLossRatio.WithLabelValues(peerIP, pubKeyShort).Set(1.0) // 100% 丢包
		}

		// 获取健康状态
		isHealthy, failureCount := hc.health.GetPeerState(pubKey)
		score := float64(0)
		if isHealthy {
			score = 1.0
		}
		LinkHealthScore.WithLabelValues(peerIP, pubKeyShort).Set(score)
		LinkFailureCount.WithLabelValues(peerIP, pubKeyShort).Set(float64(failureCount))
	}

	log.Printf("[metrics] Health: collected %d peers", len(peers))
	return nil
}

// calculateJitter 计算 RTT 抖动（标准差）
// 注意：输入的 rtts 应该是最近采集周期的样本（而非历史累计）
// Prober 的 StatsWindow=75 已经保证了这一点
func calculateJitter(rtts []time.Duration) float64 {
	if len(rtts) < 2 {
		return 0
	}

	// 计算平均值
	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	mean := float64(sum) / float64(len(rtts))

	// 计算方差
	var variance float64
	for _, rtt := range rtts {
		diff := float64(rtt) - mean
		variance += diff * diff
	}
	variance /= float64(len(rtts))

	// 返回标准差（单位：毫秒）
	// 注：RFC 1889 (RTP) 中的 Jitter 定义略有不同（平滑后的包到达间隔差）
	// 但标准差对于通用 SD-WAN 监控已足够准确
	return math.Sqrt(variance) / 1e6 // 纳秒转毫秒
}
