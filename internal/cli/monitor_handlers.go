package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aria/pkg/controllerstorage"
	"aria/pkg/logging"
)

// MonitorStats 监控统计数据
type MonitorStats struct {
	Peers        int          `json:"peers"`
	AvgRTT       string       `json:"avgRtt"`
	PacketLoss   string       `json:"packetLoss"`
	TotalTraffic string       `json:"totalTraffic"`
	TrafficData  *TrafficData `json:"trafficData"`
	PeerDetails  []PeerDetail `json:"peerDetails"`  // 新增：peer 详情
	SystemStats  *SystemStats `json:"systemStats"`  // 新增：系统统计
	GlobalStats  *GlobalStats `json:"globalStats"`  // 新增：全局统计（前端需要）
}

// GlobalStats 全局统计（前端监控中心需要）
type GlobalStats struct {
	TotalNodes   int      `json:"totalNodes"`   // 节点总数
	OnlineNodes  int      `json:"onlineNodes"`  // 在线节点数
	TotalRegions int      `json:"totalRegions"` // Region 总数
	RegionList   []string `json:"regionList"`   // Region 列表
	TotalRoutes  int      `json:"totalRoutes"`  // 路由总条数
	DirectRoutes int      `json:"directRoutes"` // 直连路由数
	RelayRoutes  int      `json:"relayRoutes"`  // 中继路由数
	TotalRxRate  float64  `json:"totalRxRate"`  // 总接收速率 (Mbps)
	TotalTxRate  float64  `json:"totalTxRate"`  // 总发送速率 (Mbps)
	TotalTraffic string   `json:"totalTraffic"` // 累计流量
}

// PeerDetail peer 详细信息
type PeerDetail struct {
	PublicKey       string  `json:"publicKey"`
	PeerIP          string  `json:"peerIp"`
	LocalIP         string  `json:"localIp"`        // 新增：本地 WireGuard IP
	PublicIP        string  `json:"publicIp"`       // 新增：公网 IP
	HostID          string  `json:"hostId"`
	Hostname        string  `json:"hostname"`       // 主机名
	Region          string  `json:"region"`
	Connected       bool    `json:"connected"`
	LastHandshake   int64   `json:"lastHandshake"`  // Unix timestamp
	RTT             float64 `json:"rtt"`            // milliseconds
	LossRatio       float64 `json:"lossRatio"`      // 0.0-1.0
	HealthScore     float64 `json:"healthScore"`    // 0 or 1
	FailureCount    float64 `json:"failureCount"`
	RxBytes         float64 `json:"rxBytes"`
	TxBytes         float64 `json:"txBytes"`
	RxRate          float64 `json:"rxRate"`         // Mbps (last 1min)
	TxRate          float64 `json:"txRate"`         // Mbps (last 1min)
}

// SystemStats 系统统计
type SystemStats struct {
	TotalNodes      int     `json:"totalNodes"`
	OnlineNodes     int     `json:"onlineNodes"`     // 在线节点数
	AvgRtt          float64 `json:"avgRtt"`          // 平均延迟 (ms)
	PacketLoss      float64 `json:"packetLoss"`      // 丢包率 (%)
	AvgCPU          float64 `json:"avgCpu"`          // percentage
	AvgMemory       float64 `json:"avgMemory"`       // MB
	TotalGoroutines int     `json:"totalGoroutines"`
}

// TrafficData 流量图表数据
type TrafficData struct {
	Timestamps []string  `json:"timestamps"`
	Inbound    []float64 `json:"入向"`  // Mbps
	Outbound   []float64 `json:"出向"`  // Mbps
}

// NodeDetail 节点详情
type NodeDetail struct {
	HostID           string            `json:"hostId"`
	Hostname         string            `json:"hostname"`      // 主机名
	PublicIP         string            `json:"publicIp"`      // 新增：节点公网 IP
	LocalIP          string            `json:"localIp"`       // 新增：节点 WireGuard IP
	Region           string            `json:"region"`
	IP               string            `json:"ip"`            // 保留兼容性
	Version          string            `json:"version"`
	Role             string            `json:"role"`          // 角色：agent
	RuntimeMode      string            `json:"runtimeMode"`   // 运行模式：kernel/userspace
	AdvertisedRoutes []string          `json:"advertisedRoutes"` // 广播路由
	Uptime           int64             `json:"uptime"`        // 新增：运行时间（秒）
	CPUUsage         float64           `json:"cpuUsage"`      // 新增：总 CPU 使用率
	MemoryUsage      float64           `json:"memoryUsage"`   // 新增：内存使用率
	Goroutines       int               `json:"goroutines"`    // 新增：Goroutine 数量
	CPUCores         []CPUCore         `json:"cpuCores"`
	CPUBalance       float64           `json:"cpuBalance"`
	Tunnels          []TunnelTraffic   `json:"tunnels"`
	TunnelBalance    float64           `json:"tunnelBalance"`
	Peers            []PeerConnection  `json:"peers"`
	Firewall         *FirewallStats    `json:"firewall"`
}

// CPUCore Per-Core CPU 信息
type CPUCore struct {
	Core     string  `json:"core"`
	User     float64 `json:"user"`
	System   float64 `json:"system"`
	SoftIRQ  float64 `json:"softirq"`
	Idle     float64 `json:"idle"`
	Total    float64 `json:"total"`
	Usage    float64 `json:"usage"`    // 新增：使用率百分比（前端需要）
}

// TunnelTraffic 隧道流量信息
type TunnelTraffic struct {
	Tunnel     string  `json:"tunnel"`     // 保留兼容性
	Name       string  `json:"name"`       // 新增：前端需要
	RxBytes    float64 `json:"rxBytes"`
	TxBytes    float64 `json:"txBytes"`
	Total      float64 `json:"total"`
	RxRate     float64 `json:"rxRate"`     // 新增：接收速率 (Mbps)
	TxRate     float64 `json:"txRate"`     // 新增：发送速率 (Mbps)
	Percentage float64 `json:"percentage"` // 新增：流量占比
}

// PeerConnection Peer 连接信息
type PeerConnection struct {
	PublicKey    string  `json:"publicKey"`
	PeerIP       string  `json:"peerIp"`       // 新增：前端需要
	Endpoint     string  `json:"endpoint"`
	HandshakeAge float64 `json:"handshakeAge"` // seconds
	RTT          float64 `json:"rtt"`          // 新增：延迟 (ms)
	LossRatio    float64 `json:"lossRatio"`    // 新增：丢包率 (0-1)
	RxRate       float64 `json:"rxRate"`       // 新增：接收速率 (Mbps)
	TxRate       float64 `json:"txRate"`       // 新增：发送速率 (Mbps)
	Status       string  `json:"status"`       // healthy, warning, dead
	RxBytes      float64 `json:"rxBytes"`
	TxBytes      float64 `json:"txBytes"`
}

// FirewallStats 防火墙统计
type FirewallStats struct {
	AcceptPackets   float64 `json:"acceptPackets"`
	DropPackets     float64 `json:"dropPackets"`
	InvalidPackets  float64 `json:"invalidPackets"`
	TCPFlagsPackets float64 `json:"tcpFlagsPackets"`
	NotrackRules    int     `json:"notrackRules"`    // 新增：前端需要
	ProcessedPackets float64 `json:"processedPackets"` // 新增：前端需要
	DroppedPackets  float64 `json:"droppedPackets"`   // 新增：前端需要
}

// VictoriaMetrics 配置
const (
	// 使用容器名访问，所有服务在 aria_shared_net 网络中
	vmBaseURL = "http://aria-victoria-metrics:8428"
)

var (
	monitorLogger *logging.Logger
	httpClient    = &http.Client{
		Timeout: 5 * time.Second, // 5 秒超时
	}
)

// 全局 node store，用于获取节点信息（包括广播路由）
var nodeStore *controllerstorage.Storage

// InitMonitorHandlers 初始化监控处理器
func InitMonitorHandlers(logger *logging.Logger, store *controllerstorage.Storage) {
	monitorLogger = logger
	nodeStore = store
}

// HandleMonitorStats 处理监控统计请求
func HandleMonitorStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := fetchMonitoringStats()
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Error("Failed to fetch monitoring stats: %v", err)
		}
		http.Error(w, "Failed to fetch stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleNodeDetail 处理节点详情请求
func HandleNodeDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 路径中提取 host_id
	// URL 格式: /v1/monitor/node/{host_id}
	path := r.URL.Path
	hostID := path[len("/v1/monitor/node/"):]
	if hostID == "" {
		http.Error(w, "Missing host_id", http.StatusBadRequest)
		return
	}

	detail, err := fetchNodeDetail(hostID)
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Error("Failed to fetch node detail for %s: %v", hostID, err)
		}
		http.Error(w, "Failed to fetch node detail", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// fetchMonitoringStats 从 VictoriaMetrics 获取监控数据
func fetchMonitoringStats() (*MonitorStats, error) {
	stats := &MonitorStats{}

	// 1. 查询所有 peer 数量（不管健康与否）
	peers, err := queryVMInstant("count(aria_link_health_score)")
	if err == nil && len(peers) > 0 {
		stats.Peers = int(peers[0])
	}

	// 2. 查询平均 RTT（排除 RTT=0 的 peer）
	avgRTT, err := queryVMInstant("avg(aria_probe_rtt_milliseconds > 0)")
	if err == nil && len(avgRTT) > 0 {
		stats.AvgRTT = fmt.Sprintf("%.0fms", avgRTT[0])
	} else {
		stats.AvgRTT = "-"
	}

	// 3. 查询平均丢包率（排除 loss=1.0 的 peer）
	avgLoss, err := queryVMInstant("avg(aria_probe_loss_ratio < 1) * 100")
	if err == nil && len(avgLoss) > 0 {
		stats.PacketLoss = fmt.Sprintf("%.1f%%", avgLoss[0])
	} else {
		stats.PacketLoss = "-"
	}

	// 4. 查询总流量（最近 5 分钟）
	totalRx, _ := queryVMInstant("sum(increase(wireguard_peer_rx_bytes[5m]))")
	totalTx, _ := queryVMInstant("sum(increase(wireguard_peer_tx_bytes[5m]))")
	var totalBytes float64
	if len(totalRx) > 0 {
		totalBytes += totalRx[0]
	}
	if len(totalTx) > 0 {
		totalBytes += totalTx[0]
	}
	stats.TotalTraffic = formatBytesForMonitor(totalBytes)

	// 5. 查询流量时序数据（最近 12 分钟，1 分钟间隔）
	trafficData, err := fetchTrafficTimeSeries()
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Warn("Failed to fetch traffic time series: %v", err)
		}
		// 使用空数据
		trafficData = &TrafficData{
			Timestamps: []string{},
			Inbound:    []float64{},
			Outbound:   []float64{},
		}
	}
	stats.TrafficData = trafficData

	// 6. 查询 peer 详情
	peerDetails, err := fetchPeerDetails()
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Warn("Failed to fetch peer details: %v", err)
		}
		peerDetails = []PeerDetail{}
	}
	stats.PeerDetails = peerDetails

	// 7. 查询系统统计
	systemStats, err := fetchSystemStats()
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Warn("Failed to fetch system stats: %v", err)
		}
		systemStats = &SystemStats{}
	}
	stats.SystemStats = systemStats

	// 8. 计算全局统计（前端监控中心需要）
	globalStats, err := fetchGlobalStats(peerDetails)
	if err != nil {
		if monitorLogger != nil {
			monitorLogger.Warn("Failed to fetch global stats: %v", err)
		}
		globalStats = &GlobalStats{}
	}
	stats.GlobalStats = globalStats

	return stats, nil
}

// fetchTrafficTimeSeries 获取流量时序数据
func fetchTrafficTimeSeries() (*TrafficData, error) {
	now := time.Now()
	end := now.Unix()
	start := now.Add(-12 * time.Minute).Unix()

	// 查询入站流量速率（Mbps）
	inboundQuery := "sum(rate(wireguard_peer_rx_bytes[1m])) * 8 / 1000000"
	inboundData, timestamps, err := queryVMRange(inboundQuery, start, end, "60s")
	if err != nil {
		return nil, err
	}

	// 查询出站流量速率（Mbps）
	outboundQuery := "sum(rate(wireguard_peer_tx_bytes[1m])) * 8 / 1000000"
	outboundData, _, err := queryVMRange(outboundQuery, start, end, "60s")
	if err != nil {
		return nil, err
	}

	// 格式化时间戳
	formattedTimestamps := make([]string, len(timestamps))
	for i, ts := range timestamps {
		t := time.Unix(int64(ts), 0)
		formattedTimestamps[i] = t.Format("15:04")
	}

	return &TrafficData{
		Timestamps: formattedTimestamps,
		Inbound:    inboundData,
		Outbound:   outboundData,
	}, nil
}

// queryVMInstant 查询 VictoriaMetrics 即时数据
func queryVMInstant(query string) ([]float64, error) {
	apiURL := fmt.Sprintf("%s/api/v1/query?query=%s", vmBaseURL, url.QueryEscape(query))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("VM query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("VM returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode VM response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("VM query status: %s", result.Status)
	}

	values := make([]float64, 0, len(result.Data.Result))
	for _, r := range result.Data.Result {
		if len(r.Value) >= 2 {
			if val, ok := r.Value[1].(string); ok {
				var f float64
				fmt.Sscanf(val, "%f", &f)
				values = append(values, f)
			}
		}
	}

	return values, nil
}

// queryVMRange 查询 VictoriaMetrics 时序数据
func queryVMRange(query string, start, end int64, step string) ([]float64, []float64, error) {
	apiURL := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%s",
		vmBaseURL, url.QueryEscape(query), start, end, step)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, nil, fmt.Errorf("VM range query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("VM returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("failed to decode VM response: %w", err)
	}

	if result.Status != "success" {
		return nil, nil, fmt.Errorf("VM query status: %s", result.Status)
	}

	if len(result.Data.Result) == 0 {
		return []float64{}, []float64{}, nil
	}

	// 提取第一个时间序列的数据
	values := result.Data.Result[0].Values
	timestamps := make([]float64, len(values))
	data := make([]float64, len(values))

	for i, v := range values {
		if len(v) >= 2 {
			// 时间戳
			if ts, ok := v[0].(float64); ok {
				timestamps[i] = ts
			}
			// 值
			if val, ok := v[1].(string); ok {
				var f float64
				fmt.Sscanf(val, "%f", &f)
				data[i] = f
			}
		}
	}

	return data, timestamps, nil
}

// formatBytesForMonitor 格式化字节数
func formatBytesForMonitor(bytes float64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%.0fB", bytes)
	}
	div, exp := float64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", bytes/div, "KMGTPE"[exp])
}

// fetchPeerDetails 获取所有 peer 的详细信息
func fetchPeerDetails() ([]PeerDetail, error) {
	// 查询所有指标，按 peer 分组
	query := `{__name__=~"aria_probe_rtt_milliseconds|aria_probe_loss_ratio|aria_link_health_score|aria_link_failure_count|wireguard_peer_connected|wireguard_peer_last_handshake_seconds|wireguard_peer_rx_bytes|wireguard_peer_tx_bytes"}`

	apiURL := fmt.Sprintf("%s/api/v1/query?query=%s", vmBaseURL, url.QueryEscape(query))
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("VM query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("VM returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode VM response: %w", err)
	}

	// 按 peer 分组数据
	peerMap := make(map[string]*PeerDetail)
	endpointMap := make(map[string]string)

	// 先查询一次 wireguard_peer_connected 获取 endpoint 映射
	endpointQuery := `wireguard_peer_connected`
	endpointResp, err := httpClient.Get(fmt.Sprintf("%s/api/v1/query?query=%s", vmBaseURL, url.QueryEscape(endpointQuery)))
	if err == nil {
		defer endpointResp.Body.Close()
		var endpointResult struct {
			Status string `json:"status"`
			Data   struct {
				Result []struct {
					Metric map[string]string `json:"metric"`
				} `json:"result"`
			} `json:"data"`
		}
		if json.NewDecoder(endpointResp.Body).Decode(&endpointResult) == nil {
			for _, r := range endpointResult.Data.Result {
				if peerIP, ok := r.Metric["peer_ip"]; ok {
					if ep, ok := r.Metric["endpoint"]; ok {
						if idx := strings.Index(ep, ":"); idx > 0 {
							endpointMap[peerIP] = ep[:idx]
						}
					}
				}
			}
		}
	}

	for _, r := range result.Data.Result {
		metricName := r.Metric["__name__"]
		peerIP := r.Metric["peer_ip"]
		if peerIP == "" {
			continue
		}

		// 使用 public_key 的前 20 字符作为 HostID（与节点管理一致）
		hostID := r.Metric["host_id"]
		if pk := r.Metric["public_key"]; len(pk) >= 20 {
			hostID = pk[:20]
		}

		// 初始化 peer
		if _, exists := peerMap[peerIP]; !exists {
			peerMap[peerIP] = &PeerDetail{
				PublicKey: r.Metric["public_key"],
				PeerIP:    peerIP,
				LocalIP:   r.Metric["local_ip"], // 新增：本地 WireGuard IP
				PublicIP:  endpointMap[peerIP], // 从 endpointMap 获取
				HostID:    hostID,
				Region:    r.Metric["region"],
			}
		}

		peer := peerMap[peerIP]

		// 解析值
		var value float64
		if len(r.Value) >= 2 {
			if val, ok := r.Value[1].(string); ok {
				fmt.Sscanf(val, "%f", &value)
			}
		}

		// 根据指标名称填充数据
		switch metricName {
		case "aria_probe_rtt_milliseconds":
			peer.RTT = value
		case "aria_probe_loss_ratio":
			peer.LossRatio = value
		case "aria_link_health_score":
			peer.HealthScore = value
		case "aria_link_failure_count":
			peer.FailureCount = value
		case "wireguard_peer_connected":
			peer.Connected = value == 1
		case "wireguard_peer_last_handshake_seconds":
			peer.LastHandshake = int64(value)
		case "wireguard_peer_rx_bytes":
			peer.RxBytes = value
		case "wireguard_peer_tx_bytes":
			peer.TxBytes = value
		}
	}

	// 查询流量速率（最近 1 分钟）
	for peerIP, peer := range peerMap {
		// 入站速率
		rxRateQuery := fmt.Sprintf(`rate(wireguard_peer_rx_bytes{peer_ip="%s"}[1m]) * 8 / 1000000`, peerIP)
		rxRate, _ := queryVMInstant(rxRateQuery)
		if len(rxRate) > 0 {
			peer.RxRate = rxRate[0]
		}

		// 出站速率
		txRateQuery := fmt.Sprintf(`rate(wireguard_peer_tx_bytes{peer_ip="%s"}[1m]) * 8 / 1000000`, peerIP)
		txRate, _ := queryVMInstant(txRateQuery)
		if len(txRate) > 0 {
			peer.TxRate = txRate[0]
		}
	}

	// 转换为数组
	peers := make([]PeerDetail, 0, len(peerMap))

	// 从 nodeStore 获取 hostname 映射
	hostnameMap := make(map[string]string) // public_key[:20] -> hostname
	if nodeStore != nil {
		allNodes, err := nodeStore.GetAllNodes()
		if err == nil {
			for _, node := range allNodes {
				if len(node.PublicKey) >= 20 {
					hostnameMap[node.PublicKey[:20]] = node.Hostname
				}
			}
		}
	}

	for _, peer := range peerMap {
		// 填充 hostname
		if hostname, ok := hostnameMap[peer.HostID]; ok {
			peer.Hostname = hostname
		}
		peers = append(peers, *peer)
	}

	return peers, nil
}

// fetchSystemStats 获取系统统计信息
func fetchSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}

	// 1. 总节点数（去重 host_id）
	totalNodes, err := queryVMInstant("count(count by (host_id) (aria_cpu_usage_percent))")
	if err == nil && len(totalNodes) > 0 {
		stats.TotalNodes = int(totalNodes[0])
	}

	// 2. 在线节点数（有 CPU 指标上报的节点）
	onlineNodes, err := queryVMInstant("count(count by (host_id) (aria_cpu_usage_percent))")
	if err == nil && len(onlineNodes) > 0 {
		stats.OnlineNodes = int(onlineNodes[0])
	}

	// 3. 平均延迟（所有 peer 的平均 RTT，单位：ms）
	avgRtt, err := queryVMInstant("avg(aria_probe_rtt_milliseconds > 0)")
	if err == nil && len(avgRtt) > 0 {
		stats.AvgRtt = avgRtt[0]
	}

	// 4. 平均丢包率（所有 peer 的平均丢包率，单位：%）
	avgPacketLoss, err := queryVMInstant("avg(aria_probe_loss_ratio < 1) * 100")
	if err == nil && len(avgPacketLoss) > 0 {
		stats.PacketLoss = avgPacketLoss[0]
	}

	// 5. 平均 CPU
	avgCPU, err := queryVMInstant("avg(aria_cpu_usage_percent)")
	if err == nil && len(avgCPU) > 0 {
		stats.AvgCPU = avgCPU[0]
	}

	// 6. 平均内存（使用 sys 类型，转换为 MB）
	avgMem, err := queryVMInstant(`avg(aria_memory_bytes{type="sys"}) / 1024 / 1024`)
	if err == nil && len(avgMem) > 0 {
		stats.AvgMemory = avgMem[0]
	}

	// 7. 总 Goroutines
	totalGoroutines, err := queryVMInstant("sum(aria_go_goroutines)")
	if err == nil && len(totalGoroutines) > 0 {
		stats.TotalGoroutines = int(totalGoroutines[0])
	}

	return stats, nil
}

// fetchNodeDetail 获取节点详情
func fetchNodeDetail(hostID string) (*NodeDetail, error) {
	detail := &NodeDetail{
		HostID: hostID,
	}

	// 1. 获取节点基本信息
	versionQuery := fmt.Sprintf(`aria_build_info{host_id="%s"}`, hostID)
	versionResult, err := queryVMInstantWithLabels(versionQuery)
	if err == nil && len(versionResult) > 0 {
		detail.Version = versionResult[0]["version"]
		detail.Region = versionResult[0]["region"]
		detail.PublicIP = versionResult[0]["public_ip"]  // 公网 IP
		detail.LocalIP = versionResult[0]["local_ip"]    // 本地 WireGuard IP
		detail.Role = versionResult[0]["role"]           // 角色
		if v, ok := versionResult[0]["runtime_mode"]; ok && v != "" {
			detail.RuntimeMode = v // 运行模式
		} else {
			detail.RuntimeMode = "unknown"
		}
	}

	// 1.1 从 nodeStore 获取 Hostname
	if nodeStore != nil {
		allNodes, err := nodeStore.GetAllNodes()
		if err == nil {
			for _, node := range allNodes {
				// 匹配方式1: public_key 前20字符
				if len(node.PublicKey) >= 20 && node.PublicKey[:20] == hostID {
					detail.Hostname = node.Hostname
					break
				}
				// 匹配方式2: hostname
				if node.Hostname == hostID {
					detail.Hostname = node.Hostname
					break
				}
			}
		}
	}

	// 2. 获取运行时间（从进程启动时间计算）
	uptimeQuery := fmt.Sprintf(`time() - process_start_time_seconds{host_id="%s"}`, hostID)
	uptimeResult, err := queryVMInstant(uptimeQuery)
	if err == nil && len(uptimeResult) > 0 {
		detail.Uptime = int64(uptimeResult[0])
	}

	// 3. 获取总 CPU 使用率
	cpuUsageQuery := fmt.Sprintf(`aria_cpu_usage_percent{host_id="%s"}`, hostID)
	cpuUsageResult, err := queryVMInstant(cpuUsageQuery)
	if err == nil && len(cpuUsageResult) > 0 {
		detail.CPUUsage = cpuUsageResult[0]
	}

	// 4. 获取内存使用率
	memUsageQuery := fmt.Sprintf(`aria_memory_usage_percent{host_id="%s"}`, hostID)
	memUsageResult, err := queryVMInstant(memUsageQuery)
	if err == nil && len(memUsageResult) > 0 {
		detail.MemoryUsage = memUsageResult[0]
	}

	// 5. 获取 Goroutine 数量
	goroutinesQuery := fmt.Sprintf(`go_goroutines{host_id="%s"}`, hostID)
	goroutinesResult, err := queryVMInstant(goroutinesQuery)
	if err == nil && len(goroutinesResult) > 0 {
		detail.Goroutines = int(goroutinesResult[0])
	}

	// 6. 获取 Per-Core CPU 数据
	cpuQuery := fmt.Sprintf(`aria_cpu_core_usage_percent{host_id="%s"}`, hostID)
	cpuResult, err := queryVMInstantWithLabels(cpuQuery)
	if err == nil {
		cpuMap := make(map[string]*CPUCore)
		for _, item := range cpuResult {
			core := item["core"]
			mode := item["mode"]
			value := item["value"]

			if _, ok := cpuMap[core]; !ok {
				cpuMap[core] = &CPUCore{Core: core}
			}

			valueFloat := 0.0
			fmt.Sscanf(value, "%f", &valueFloat)

			switch mode {
			case "user":
				cpuMap[core].User = valueFloat
			case "system":
				cpuMap[core].System = valueFloat
			case "softirq":
				cpuMap[core].SoftIRQ = valueFloat
			case "idle":
				cpuMap[core].Idle = valueFloat
			}
			cpuMap[core].Total = 100 - cpuMap[core].Idle
			cpuMap[core].Usage = cpuMap[core].Total  // 新增：前端需要的 usage 字段
		}

		for _, cpu := range cpuMap {
			detail.CPUCores = append(detail.CPUCores, *cpu)
		}
	}

	// 3. 获取 CPU 均衡度
	balanceQuery := fmt.Sprintf(`aria_cpu_balance_score{host_id="%s"}`, hostID)
	balanceResult, err := queryVMInstant(balanceQuery)
	if err == nil && len(balanceResult) > 0 {
		detail.CPUBalance = balanceResult[0]
	}

	// 4. 获取隧道流量数据
	tunnelQuery := fmt.Sprintf(`aria_tunnel_tx_bytes{host_id="%s"} or aria_tunnel_rx_bytes{host_id="%s"}`, hostID, hostID)
	tunnelResult, err := queryVMInstantWithLabels(tunnelQuery)
	if err == nil {
		tunnelMap := make(map[string]*TunnelTraffic)
		for _, item := range tunnelResult {
			tunnel := item["tunnel"]
			metric := item["__name__"]
			value := item["value"]

			if _, ok := tunnelMap[tunnel]; !ok {
				tunnelMap[tunnel] = &TunnelTraffic{
					Tunnel: tunnel,
					Name:   tunnel, // 前端需要的 name 字段
				}
			}

			valueFloat := 0.0
			fmt.Sscanf(value, "%f", &valueFloat)

			if metric == "aria_tunnel_tx_bytes" {
				tunnelMap[tunnel].TxBytes = valueFloat
			} else if metric == "aria_tunnel_rx_bytes" {
				tunnelMap[tunnel].RxBytes = valueFloat
			}
			tunnelMap[tunnel].Total = tunnelMap[tunnel].TxBytes + tunnelMap[tunnel].RxBytes
		}

		// 获取隧道速率（使用 rate 函数计算）
		tunnelRateQuery := fmt.Sprintf(`rate(aria_tunnel_tx_bytes{host_id="%s"}[1m]) or rate(aria_tunnel_rx_bytes{host_id="%s"}[1m])`, hostID, hostID)
		tunnelRateResult, err := queryVMInstantWithLabels(tunnelRateQuery)
		if err == nil {
			for _, item := range tunnelRateResult {
				tunnel := item["tunnel"]
				metric := item["__name__"]
				value := item["value"]

				if t, ok := tunnelMap[tunnel]; ok {
					valueFloat := 0.0
					fmt.Sscanf(value, "%f", &valueFloat)
					// 转换为 Mbps
					valueMbps := valueFloat * 8 / 1000000

					if metric == "aria_tunnel_tx_bytes" {
						t.TxRate = valueMbps
					} else if metric == "aria_tunnel_rx_bytes" {
						t.RxRate = valueMbps
					}
				}
			}
		}

		// 计算总流量用于百分比计算
		var totalTraffic float64
		for _, t := range tunnelMap {
			totalTraffic += t.Total
		}

		// 计算百分比并添加到结果
		for _, t := range tunnelMap {
			if totalTraffic > 0 {
				t.Percentage = (t.Total / totalTraffic) * 100
			}
			detail.Tunnels = append(detail.Tunnels, *t)
		}
	}

	// 5. 获取隧道均衡度
	tunnelBalanceQuery := fmt.Sprintf(`aria_tunnel_balance_score{host_id="%s"}`, hostID)
	tunnelBalanceResult, err := queryVMInstant(tunnelBalanceQuery)
	if err == nil && len(tunnelBalanceResult) > 0 {
		detail.TunnelBalance = tunnelBalanceResult[0]
	}

	// 6. 获取 Peer 连接信息
	peerQuery := fmt.Sprintf(`wireguard_peer_last_handshake_seconds{host_id="%s"}`, hostID)
	peerResult, err := queryVMInstantWithLabels(peerQuery)
	if err == nil {
		peerMap := make(map[string]*PeerConnection)

		for _, item := range peerResult {
			handshakeAge := 0.0
			fmt.Sscanf(item["value"], "%f", &handshakeAge)

			status := "online"
			if handshakeAge > 180 {
				status = "offline"
			} else if handshakeAge > 60 {
				status = "warning"
			}

			publicKey := item["public_key"]
			endpoint := item["endpoint"]

			// 从 endpoint 提取 peer IP (格式: ip:port)
			peerIP := endpoint
			if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
				peerIP = endpoint[:idx]
			}

			peerMap[publicKey] = &PeerConnection{
				PublicKey:    publicKey,
				PeerIP:       peerIP,
				Endpoint:     endpoint,
				HandshakeAge: handshakeAge,
				Status:       status,
			}
		}

		// 获取 RTT 数据
		rttQuery := fmt.Sprintf(`aria_link_rtt_ms{host_id="%s"}`, hostID)
		rttResult, err := queryVMInstantWithLabels(rttQuery)
		if err == nil {
			for _, item := range rttResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					rtt := 0.0
					fmt.Sscanf(item["value"], "%f", &rtt)
					peer.RTT = rtt
				}
			}
		}

		// 获取丢包率数据
		lossQuery := fmt.Sprintf(`aria_link_loss_ratio{host_id="%s"}`, hostID)
		lossResult, err := queryVMInstantWithLabels(lossQuery)
		if err == nil {
			for _, item := range lossResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					loss := 0.0
					fmt.Sscanf(item["value"], "%f", &loss)
					peer.LossRatio = loss
				}
			}
		}

		// 获取接收字节数
		rxBytesQuery := fmt.Sprintf(`wireguard_peer_rx_bytes{host_id="%s"}`, hostID)
		rxBytesResult, err := queryVMInstantWithLabels(rxBytesQuery)
		if err == nil {
			for _, item := range rxBytesResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					rxBytes := 0.0
					fmt.Sscanf(item["value"], "%f", &rxBytes)
					peer.RxBytes = rxBytes
				}
			}
		}

		// 获取发送字节数
		txBytesQuery := fmt.Sprintf(`wireguard_peer_tx_bytes{host_id="%s"}`, hostID)
		txBytesResult, err := queryVMInstantWithLabels(txBytesQuery)
		if err == nil {
			for _, item := range txBytesResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					txBytes := 0.0
					fmt.Sscanf(item["value"], "%f", &txBytes)
					peer.TxBytes = txBytes
				}
			}
		}

		// 获取接收速率
		rxRateQuery := fmt.Sprintf(`rate(wireguard_peer_rx_bytes{host_id="%s"}[1m])`, hostID)
		rxRateResult, err := queryVMInstantWithLabels(rxRateQuery)
		if err == nil {
			for _, item := range rxRateResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					rxRate := 0.0
					fmt.Sscanf(item["value"], "%f", &rxRate)
					// 转换为 Mbps
					peer.RxRate = rxRate * 8 / 1000000
				}
			}
		}

		// 获取发送速率
		txRateQuery := fmt.Sprintf(`rate(wireguard_peer_tx_bytes{host_id="%s"}[1m])`, hostID)
		txRateResult, err := queryVMInstantWithLabels(txRateQuery)
		if err == nil {
			for _, item := range txRateResult {
				publicKey := item["public_key"]
				if peer, ok := peerMap[publicKey]; ok {
					txRate := 0.0
					fmt.Sscanf(item["value"], "%f", &txRate)
					// 转换为 Mbps
					peer.TxRate = txRate * 8 / 1000000
				}
			}
		}

		// 添加到结果
		for _, peer := range peerMap {
			detail.Peers = append(detail.Peers, *peer)
		}
	}

	// 7. 获取防火墙统计
	fwQuery := fmt.Sprintf(`aria_firewall_packets_total{host_id="%s"}`, hostID)
	fwResult, err := queryVMInstantWithLabels(fwQuery)
	if err == nil {
		fw := &FirewallStats{}
		for _, item := range fwResult {
			action := item["action"]
			reason := item["reason"]
			value := 0.0
			fmt.Sscanf(item["value"], "%f", &value)

			if action == "accept" {
				fw.AcceptPackets += value
				fw.ProcessedPackets += value // 前端需要的处理包数
			} else if action == "drop" {
				fw.DropPackets += value
				fw.DroppedPackets += value // 前端需要的丢弃包数
				fw.ProcessedPackets += value
				if reason == "invalid" {
					fw.InvalidPackets += value
				} else if reason == "tcp_flags" {
					fw.TCPFlagsPackets += value
				}
			}
		}

		// 获取 NOTRACK 规则数
		notrackQuery := fmt.Sprintf(`aria_firewall_notrack_rules{host_id="%s"}`, hostID)
		notrackResult, err := queryVMInstant(notrackQuery)
		if err == nil && len(notrackResult) > 0 {
			fw.NotrackRules = int(notrackResult[0])
		}

		detail.Firewall = fw
	}

	// 10. 从 nodeStore 获取广播路由
	if nodeStore != nil {
		allNodes, err := nodeStore.GetAllNodes()
		if err == nil {
			// hostID 可能是完整 public_key 的前 20 字符，也可能是 hostname
			// 需要同时匹配
			for _, node := range allNodes {
				// 匹配方式1: public_key 前20字符
				if len(node.PublicKey) >= 20 && node.PublicKey[:20] == hostID {
					detail.AdvertisedRoutes = node.AdvertisedRoutes
					break
				}
				// 匹配方式2: hostname
				if node.Hostname == hostID {
					detail.AdvertisedRoutes = node.AdvertisedRoutes
					break
				}
			}
		}
	}

	return detail, nil
}

// queryVMInstantWithLabels 查询 VictoriaMetrics 并返回带标签的结果
func queryVMInstantWithLabels(query string) ([]map[string]string, error) {
	vmURL := fmt.Sprintf("%s/api/v1/query?query=%s", vmBaseURL, url.QueryEscape(query))

	resp, err := httpClient.Get(vmURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query VM: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("VM query failed: %s", result.Status)
	}

	var results []map[string]string
	for _, item := range result.Data.Result {
		row := make(map[string]string)
		// 复制所有标签
		for k, v := range item.Metric {
			row[k] = v
		}
		// 添加值
		if len(item.Value) > 1 {
			row["value"] = fmt.Sprintf("%v", item.Value[1])
		}
		results = append(results, row)
	}

	return results, nil
}

// fetchGlobalStats 获取全局统计信息（前端监控中心需要）
func fetchGlobalStats(peerDetails []PeerDetail) (*GlobalStats, error) {
	stats := &GlobalStats{}

	// 1. 节点总数和在线节点数
	totalNodes, err := queryVMInstant("count(aria_link_health_score)")
	if err == nil && len(totalNodes) > 0 {
		stats.TotalNodes = int(totalNodes[0])
	}

	onlineNodes, err := queryVMInstant("count(aria_link_health_score == 1)")
	if err == nil && len(onlineNodes) > 0 {
		stats.OnlineNodes = int(onlineNodes[0])
	}

	// 2. Region 统计（从 peer details 中提取）
	regionMap := make(map[string]bool)
	for _, peer := range peerDetails {
		if peer.Region != "" {
			regionMap[peer.Region] = true
		}
	}
	stats.TotalRegions = len(regionMap)
	stats.RegionList = make([]string, 0, len(regionMap))
	for region := range regionMap {
		stats.RegionList = append(stats.RegionList, region)
	}

	// 3. 路由条数（TODO: 需要实现路由表监控指标）
	// 暂时使用模拟数据，后续需要添加 aria_routing_table_entries 指标
	stats.TotalRoutes = 0
	stats.DirectRoutes = 0
	stats.RelayRoutes = 0

	// 尝试查询路由指标（如果已实现）
	totalRoutes, err := queryVMInstant("aria_routing_table_entries")
	if err == nil && len(totalRoutes) > 0 {
		stats.TotalRoutes = int(totalRoutes[0])
	}

	directRoutes, err := queryVMInstant("aria_routing_table_entries{type=\"direct\"}")
	if err == nil && len(directRoutes) > 0 {
		stats.DirectRoutes = int(directRoutes[0])
	}

	relayRoutes, err := queryVMInstant("aria_routing_table_entries{type=\"relay\"}")
	if err == nil && len(relayRoutes) > 0 {
		stats.RelayRoutes = int(relayRoutes[0])
	}

	// 4. 总流量速率（Mbps）
	totalRxRate, err := queryVMInstant("sum(rate(wireguard_peer_rx_bytes[1m])) * 8 / 1000000")
	if err == nil && len(totalRxRate) > 0 {
		stats.TotalRxRate = totalRxRate[0]
	}

	totalTxRate, err := queryVMInstant("sum(rate(wireguard_peer_tx_bytes[1m])) * 8 / 1000000")
	if err == nil && len(totalTxRate) > 0 {
		stats.TotalTxRate = totalTxRate[0]
	}

	// 5. 累计流量（最近 5 分钟）
	totalRx, _ := queryVMInstant("sum(increase(wireguard_peer_rx_bytes[5m]))")
	totalTx, _ := queryVMInstant("sum(increase(wireguard_peer_tx_bytes[5m]))")
	var totalBytes float64
	if len(totalRx) > 0 {
		totalBytes += totalRx[0]
	}
	if len(totalTx) > 0 {
		totalBytes += totalTx[0]
	}
	stats.TotalTraffic = formatBytesForMonitor(totalBytes)

	return stats, nil
}
