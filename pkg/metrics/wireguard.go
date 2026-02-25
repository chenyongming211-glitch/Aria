package metrics

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"aria/pkg/datapath"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// PeerLastHandshake 最后握手时间（距离现在的秒数）
	PeerLastHandshake = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wireguard_peer_last_handshake_seconds",
			Help: "Seconds since last successful handshake",
		},
		[]string{"public_key", "endpoint"},
	)

	// PeerRxBytes 接收字节数
	PeerRxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wireguard_peer_rx_bytes",
			Help: "Total bytes received from peer",
		},
		[]string{"public_key", "endpoint"},
	)

	// PeerTxBytes 发送字节数
	PeerTxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wireguard_peer_tx_bytes",
			Help: "Total bytes sent to peer",
		},
		[]string{"public_key", "endpoint"},
	)

	// PeerConnected 连接状态（1=已连接, 0=断开）
	PeerConnected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wireguard_peer_connected",
			Help: "Peer connection status (1=connected, 0=disconnected)",
		},
		[]string{"public_key", "endpoint"},
	)

	// WireguardDroppedMTUPackets 因 MTU 问题丢弃的数据包数
	WireguardDroppedMTUPackets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wireguard_dropped_mtu_packets",
			Help: "Packets dropped due to MTU issues",
		},
		[]string{"interface"},
	)

	// TunnelTxBytes 每条隧道的发送字节数
	TunnelTxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_tunnel_tx_bytes",
			Help: "Bytes transmitted per tunnel",
		},
		[]string{"tunnel"}, // tunnel: aria0, aria1, aria2, aria3
	)

	// TunnelRxBytes 每条隧道的接收字节数
	TunnelRxBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_tunnel_rx_bytes",
			Help: "Bytes received per tunnel",
		},
		[]string{"tunnel"},
	)

	// TunnelBalanceScore 隧道流量均衡度评分 (0-100)
	TunnelBalanceScore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_tunnel_balance_score",
			Help: "Tunnel traffic balance score (0-100, higher is better)",
		},
	)
)

// WireGuardCollector WireGuard 指标采集器
type WireGuardCollector struct {
	tunnel datapath.TunnelManager
}

// NewWireGuardCollector 创建 WireGuard 采集器
func NewWireGuardCollector(tunnel datapath.TunnelManager, registerer prometheus.Registerer) *WireGuardCollector {
	// 注册指标
	registerer.MustRegister(
		PeerLastHandshake,
		PeerRxBytes,
		PeerTxBytes,
		PeerConnected,
		WireguardDroppedMTUPackets,
		TunnelTxBytes,
		TunnelRxBytes,
		TunnelBalanceScore,
	)

	return &WireGuardCollector{
		tunnel: tunnel,
	}
}

// Name 返回采集器名称
func (wc *WireGuardCollector) Name() string {
	return "wireguard"
}

// Collect 采集 WireGuard 指标（带超时控制）
func (wc *WireGuardCollector) Collect(ctx context.Context) error {
	type result struct {
		peers []*datapath.PeerInfo
		err   error
	}

	done := make(chan result, 1)

	// 异步调用 ListPeers（防止 wgctrl 阻塞）
	go func() {
		peers, err := wc.tunnel.ListPeers()
		done <- result{peers: peers, err: err}
	}()

	// 等待结果或超时
	select {
	case res := <-done:
		if res.err != nil {
			return fmt.Errorf("wgctrl ListPeers failed: %w", res.err)
		}
		return wc.updateMetrics(res.peers)

	case <-ctx.Done():
		return fmt.Errorf("wgctrl timeout: %w", ctx.Err())
	}
}

// updateMetrics 更新指标值
func (wc *WireGuardCollector) updateMetrics(peers []*datapath.PeerInfo) error {
	now := time.Now()

	for _, peer := range peers {
		// 截断 public key（防止高基数）
		pubKey := peer.PublicKey
		if len(pubKey) > 16 {
			pubKey = pubKey[:16]
		}

		endpoint := peer.Endpoint
		if endpoint == "" {
			endpoint = "unknown"
		}

		// 最后握手时间（距离现在的秒数）
		handshakeAge := float64(0)
		if !peer.LastHandshake.IsZero() {
			handshakeAge = now.Sub(peer.LastHandshake).Seconds()
		}
		PeerLastHandshake.WithLabelValues(pubKey, endpoint).Set(handshakeAge)

		// 流量统计
		PeerRxBytes.WithLabelValues(pubKey, endpoint).Set(float64(peer.RxBytes))
		PeerTxBytes.WithLabelValues(pubKey, endpoint).Set(float64(peer.TxBytes))

		// 连接状态（握手时间 > 180s 认为断开）
		connected := float64(0)
		if !peer.LastHandshake.IsZero() && now.Sub(peer.LastHandshake) < 180*time.Second {
			connected = 1
		}
		PeerConnected.WithLabelValues(pubKey, endpoint).Set(connected)
	}

	log.Printf("[metrics] WireGuard: collected %d peers", len(peers))

	// 采集隧道流量统计（仅 Linux）
	if err := wc.collectTunnelTraffic(); err != nil {
		log.Printf("[metrics] Failed to collect tunnel traffic: %v", err)
	}

	// 采集 MTU 统计（仅 Linux）
	if err := wc.collectMTUStats(); err != nil {
		log.Printf("[metrics] Failed to collect MTU stats: %v", err)
	}

	return nil
}

// collectMTUStats 采集 MTU/分片相关统计（仅 Linux）
func (wc *WireGuardCollector) collectMTUStats() error {
	// 仅 Linux 支持
	if runtime.GOOS != "linux" {
		return nil
	}

	iface := wc.tunnel.GetInterfaceName()
	if iface == "" {
		return fmt.Errorf("interface name is empty")
	}

	// 读取 tx_dropped 统计
	droppedPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_dropped", iface)
	data, err := os.ReadFile(droppedPath)
	if err == nil {
		dropped, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		WireguardDroppedMTUPackets.WithLabelValues(iface).Set(float64(dropped))
	}

	return nil
}

// collectTunnelTraffic 采集每条隧道的流量统计
func (wc *WireGuardCollector) collectTunnelTraffic() error {
	// 仅 Linux 支持
	if runtime.GOOS != "linux" {
		return nil
	}

	tunnelNames := []string{"aria0", "aria1", "aria2", "aria3"}
	var tunnelLoads []float64

	for _, tunnelName := range tunnelNames {
		// 读取 rx_bytes
		rxPath := fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", tunnelName)
		rxData, err := os.ReadFile(rxPath)
		if err != nil {
			// 隧道可能不存在（单隧道模式），跳过
			continue
		}
		rxBytes, _ := strconv.ParseUint(strings.TrimSpace(string(rxData)), 10, 64)
		TunnelRxBytes.WithLabelValues(tunnelName).Set(float64(rxBytes))

		// 读取 tx_bytes
		txPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", tunnelName)
		txData, err := os.ReadFile(txPath)
		if err != nil {
			continue
		}
		txBytes, _ := strconv.ParseUint(strings.TrimSpace(string(txData)), 10, 64)
		TunnelTxBytes.WithLabelValues(tunnelName).Set(float64(txBytes))

		// 计算总流量（用于均衡度计算）
		totalBytes := float64(rxBytes + txBytes)
		tunnelLoads = append(tunnelLoads, totalBytes)
	}

	// 计算隧道均衡度评分
	if len(tunnelLoads) > 1 {
		balanceScore := calculateTunnelBalanceScore(tunnelLoads)
		TunnelBalanceScore.Set(balanceScore)
	}

	return nil
}

// calculateTunnelBalanceScore 计算隧道均衡度评分（0-100，越高越均衡）
func calculateTunnelBalanceScore(loads []float64) float64 {
	if len(loads) == 0 {
		return 100
	}

	// 计算平均值
	var sum float64
	for _, load := range loads {
		sum += load
	}
	mean := sum / float64(len(loads))

	// 如果平均值为 0（所有隧道都没有流量），返回满分
	if mean == 0 {
		return 100
	}

	// 计算标准差
	var variance float64
	for _, load := range loads {
		diff := load - mean
		variance += diff * diff
	}
	variance /= float64(len(loads))
	stdDev := math.Sqrt(variance)

	// 计算变异系数（CV = stdDev / mean）
	cv := stdDev / mean

	// 转换为评分（CV 越小，评分越高）
	// CV > 0.3 时评分为 0
	score := 100 - (cv * 333.33)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}
