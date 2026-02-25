package rpc

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"aria/pkg/agentstorage"
	"aria/pkg/config"
)

// Daemon implements the DaemonInterface
type Daemon struct {
	config    *config.AgentConfig
	storage   *agentstorage.Storage
	version   string
	startTime time.Time
}

// NewDaemon creates a new daemon instance
func NewDaemon(cfg *config.AgentConfig, storage *agentstorage.Storage, version string) *Daemon {
	return &Daemon{
		config:    cfg,
		storage:   storage,
		version:   version,
		startTime: time.Now(),
	}
}

// GetPeers implements DaemonInterface
func (d *Daemon) GetPeers() (*GetPeersResponse, error) {
	// Execute wg show command
	cmd := exec.Command("wg", "show", d.config.Interface, "dump")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute wg show: %w", err)
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var peers []PeerInfo
	activeCount := 0

	// Load cached peer info from storage
	cachedPeers, _ := d.storage.LoadPeers()
	peerMap := make(map[string]agentstorage.PeerConfig)
	for _, p := range cachedPeers {
		peerMap[p.PublicKey] = p
	}

	// Skip first line (interface info)
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}

		publicKey := fields[0]
		endpoint := fields[2]
		latestHandshake, _ := strconv.ParseInt(fields[4], 10, 64)
		rxBytes, _ := strconv.ParseUint(fields[5], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[6], 10, 64)

		// Get additional info from cache
		cached, exists := peerMap[publicKey]

		hostname := publicKey[:8] + "..."
		region := ""
		customerID := ""
		assignedIP := ""
		status := "never"

		if exists {
			if cached.Hostname != "" {
				hostname = cached.Hostname
			}
			assignedIP = cached.AssignedIP
			region = cached.Region
		}

		// Parse handshake time
		var lastHandshake time.Time
		if latestHandshake > 0 {
			lastHandshake = time.Unix(latestHandshake, 0)
			elapsed := time.Since(lastHandshake)
			if elapsed < 3*time.Minute {
				status = "online"
				activeCount++
			} else {
				status = "offline"
			}
		}

		peers = append(peers, PeerInfo{
			Hostname:      hostname,
			Region:        region,
			CustomerID:    customerID,
			PublicKey:     publicKey,
			AssignedIP:    assignedIP,
			Endpoint:      endpoint,
			LastHandshake: lastHandshake,
			RxBytes:       rxBytes,
			TxBytes:       txBytes,
			Latency:       0, // TODO: implement latency measurement
			Status:        status,
		})
	}

	return &GetPeersResponse{
		Peers:       peers,
		TotalCount:  len(peers),
		ActiveCount: activeCount,
	}, nil
}

// GetRoutes implements DaemonInterface
func (d *Daemon) GetRoutes() (*GetRoutesResponse, error) {
	var routes []RouteInfo

	// 1. Add local VPN route
	if d.config.AssignedIP != "" {
		routes = append(routes, RouteInfo{
			Target:   d.config.AssignedIP + "/32",
			Via:      d.config.Interface,
			Metric:   0,
			Status:   "active",
			Type:     "local",
			Hostname: "(self)",
			Region:   d.config.Region,
		})
	}

	// 2. Get peer routes from WireGuard
	cmd := exec.Command("wg", "show", d.config.Interface, "dump")
	output, err := cmd.Output()
	if err != nil {
		return &GetRoutesResponse{Routes: routes}, nil // Return what we have
	}

	lines := strings.Split(string(output), "\n")
	// Skip header line (first line is interface info)
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}

		// Parse peer line
		// Format: public_key preshared_key endpoint allowed_ips latest_handshake rx tx persistent_keepalive
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}

		allowedIPsStr := fields[3]
		if allowedIPsStr == "(none)" {
			continue
		}

		// Get peer hostname and region from storage
		publicKey := fields[0]
		peerData, _ := d.storage.GetPeerByPublicKey(publicKey)
		hostname := "unknown"
		region := ""
		if peerData != nil {
			hostname = peerData.Hostname
			region = peerData.Region
		}

		// Parse allowed IPs
		allowedIPs := strings.Split(allowedIPsStr, ",")
		for _, allowedIP := range allowedIPs {
			allowedIP = strings.TrimSpace(allowedIP)
			if allowedIP == "" {
				continue
			}

			// Determine route type
			routeType := "advertised"
			if strings.HasSuffix(allowedIP, "/32") {
				// Single IP is peer's VPN address
				routeType = "peer"
			}

			routes = append(routes, RouteInfo{
				Target:   allowedIP,
				Via:      d.config.Interface,
				Metric:   0,
				Status:   "active",
				Type:     routeType,
				Hostname: hostname,
				Region:   region,
			})
		}
	}

	return &GetRoutesResponse{
		Routes: routes,
	}, nil
}

// SyncConfig implements DaemonInterface
func (d *Daemon) SyncConfig() (*SyncConfigResponse, error) {
	startTime := time.Now()

	// Create a signal file to trigger re-registration in the main loop
	// The main daemon loop will check this file and reload config
	signalFile := "/var/run/aria.reload"
	file, err := os.Create(signalFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create reload signal: %w", err)
	}
	file.Close()

	duration := time.Since(startTime)

	return &SyncConfigResponse{
		PeersUpdated:  0,
		RoutesUpdated: 0,
		Duration:      duration.String(),
	}, nil
}

// GetStatus implements DaemonInterface
func (d *Daemon) GetStatus() (*GetStatusResponse, error) {
	// Get peer count
	peersResp, _ := d.GetPeers()
	peerCount := 0
	if peersResp != nil {
		peerCount = peersResp.TotalCount
	}

	uptime := time.Since(d.startTime)

	return &GetStatusResponse{
		Status: StatusInfo{
			Version:       d.version,
			DeviceID:      d.config.DeviceID,
			AssignedIP:    d.config.AssignedIP,
			Region:        d.config.Region,
			CustomerID:    d.config.CustomerID,
			Interface:     d.config.Interface,
			ControllerURL: d.config.ControllerURL,
			Uptime:        uptime.String(),
			StartTime:     d.startTime,
			PeerCount:     peerCount,
			Status:        "running",
		},
	}, nil
}

// FlushConfig implements DaemonInterface
func (d *Daemon) FlushConfig() (*FlushConfigResponse, error) {
	// Get current peers count
	peersResp, _ := d.GetPeers()
	peersRemoved := 0
	if peersResp != nil {
		peersRemoved = peersResp.TotalCount
	}

	// Remove all peers
	for _, peer := range peersResp.Peers {
		cmd := exec.Command("wg", "set", d.config.Interface, "peer", peer.PublicKey, "remove")
		cmd.Run()
	}

	// Clear routes
	cmd := exec.Command("ip", "route", "flush", "dev", d.config.Interface)
	output, _ := cmd.CombinedOutput()

	// Count cleared routes
	routesCleared := len(strings.Split(string(output), "\n"))

	return &FlushConfigResponse{
		PeersRemoved:  peersRemoved,
		RoutesCleared: routesCleared,
	}, nil
}

// Ping implements DaemonInterface
func (d *Daemon) Ping(target string, count int, interval float64) (*PingResponse, error) {
	// Resolve hostname to IP if needed
	ip := target
	if !isIPAddress(target) {
		// Try to find IP from peers
		peersResp, _ := d.GetPeers()
		found := false
		for _, peer := range peersResp.Peers {
			if peer.Hostname == target {
				ip = peer.AssignedIP
				found = true
				break
			}
		}
		if !found {
			return &PingResponse{
				Output:  fmt.Sprintf("Unknown hostname: %s\n", target),
				Success: false,
			}, nil
		}
	}

	// Build ping command
	args := []string{"-I", d.config.Interface, "-c", strconv.Itoa(count)}
	if interval > 0 {
		args = append(args, "-i", fmt.Sprintf("%.1f", interval))
	}
	args = append(args, ip)

	cmd := exec.Command("ping", args...)
	output, err := cmd.CombinedOutput()
	success := err == nil

	return &PingResponse{
		Output:  string(output),
		Success: success,
	}, nil
}

// Helper function to check if string is an IP address
func isIPAddress(s string) bool {
	matched, _ := regexp.MatchString(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`, s)
	return matched
}

// GetMetrics implements DaemonInterface - retrieves current monitoring metrics
func (d *Daemon) GetMetrics() (*GetMetricsResponse, error) {
	var peers []PeerMetrics
	var summary MetricsSummary
	var totalRTT float64
	var totalLoss float64
	var healthyCount int

	// Get WireGuard peer data
	cmd := exec.Command("wg", "show", d.config.Interface, "dump")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute wg show: %w", err)
	}

	// Parse WireGuard output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}

		publicKey := fields[0]
		endpoint := fields[2]
		latestHandshake, _ := strconv.ParseInt(fields[4], 10, 64)
		rxBytes, _ := strconv.ParseUint(fields[5], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[6], 10, 64)

		lastHandshake := time.Unix(latestHandshake, 0)
		isHealthy := time.Since(lastHandshake) < 3*time.Minute

		if isHealthy {
			healthyCount++
		}

		// 尝试从 HTTP metrics 端点获取 RTT 和丢包率
		// 但为了简化，这里先不实现完整的 Prometheus 解析
		// 仅显示基本信息
		peers = append(peers, PeerMetrics{
			PublicKey:     publicKey[:16] + "...",
			Endpoint:      endpoint,
			LastHandshake: lastHandshake,
			RTT:           0, // TODO: 从 Prober 获取
			LossRate:      0, // TODO: 从 Prober 获取
			IsHealthy:     isHealthy,
			RxBytes:       rxBytes,
			TxBytes:       txBytes,
		})

		summary.PeersTotal++
	}

	summary.PeersHealthy = healthyCount
	if summary.PeersTotal > 0 {
		summary.AvgRTT = totalRTT / float64(summary.PeersTotal)
		summary.AvgLossRate = totalLoss / float64(summary.PeersTotal)
	}

	// Get system metrics
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	system := SystemMetrics{
		CPUUsage:   0, // TODO: 实现 CPU 读取
		MemoryMB:   float64(mem.Alloc) / 1024 / 1024,
		Goroutines: runtime.NumGoroutine(),
	}

	return &GetMetricsResponse{
		Summary:   summary,
		Peers:     peers,
		System:    system,
		Timestamp: time.Now(),
	}, nil
}
