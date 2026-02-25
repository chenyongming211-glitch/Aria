package nettuning

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SysctlConfig contains network sysctl parameters
type SysctlConfig struct {
	// TCP/IP Core
	TCPCongestionControl string // bbr
	DefaultQdisc         string // fq (required for BBR)
	TCPNoMetricsSave     int    // 1 = don't cache metrics
	TCPMTUProbing        int    // 1 = enable PMTU probing

	// Buffer sizes (bytes)
	NetCoreRmemMax     int // Max receive buffer
	NetCoreWmemMax     int // Max send buffer
	NetCoreRmemDefault int
	NetCoreWmemDefault int
	TCPRmem            [3]int // min, default, max
	TCPWmem            [3]int // min, default, max

	// Connection handling
	NetCoreNetdevMaxBacklog int // Packet queue length
	NetCoreSomaxconn        int // Listen backlog
	TCPMaxSynBacklog        int // SYN queue length

	// Conntrack (critical for SD-WAN with many connections)
	ConntrackMax int // nf_conntrack_max

	// Keepalive (for long-lived WireGuard connections)
	TCPKeepaliveTime   int // seconds before first probe
	TCPKeepaliveIntvl  int // seconds between probes
	TCPKeepaliveProbes int // number of probes

	// Memory pressure
	TCPMemPressureThreshold int
	TCPModerateRcvbuf       int // 1 = enable auto-tuning
}

// DefaultSysctlConfig returns optimized settings for cloud VMs
func DefaultSysctlConfig() *SysctlConfig {
	return &SysctlConfig{
		// Enable BBR - critical for cloud/public network
		// IMPORTANT: BBR requires fq qdisc to work properly
		TCPCongestionControl: "bbr",
		DefaultQdisc:         "fq",
		TCPNoMetricsSave:     1,
		TCPMTUProbing:        1,

		// Large buffers for high bandwidth-delay product
		// 25MB for 10Gbps links with 20ms RTT
		NetCoreRmemMax:     26214400,
		NetCoreWmemMax:     26214400,
		NetCoreRmemDefault: 1048576,
		NetCoreWmemDefault: 1048576,
		TCPRmem:            [3]int{4096, 87380, 26214400},
		TCPWmem:            [3]int{4096, 16384, 26214400},

		// High-traffic queue settings
		NetCoreNetdevMaxBacklog: 50000,
		NetCoreSomaxconn:        65535,
		TCPMaxSynBacklog:        65535,

		// Conntrack for SD-WAN (many concurrent connections)
		ConntrackMax: 1048576, // 1M connections

		// Keepalive for tunnel connections
		TCPKeepaliveTime:   60,
		TCPKeepaliveIntvl:  10,
		TCPKeepaliveProbes: 6,

		// Memory management
		TCPModerateRcvbuf: 1,
	}
}

// Apply applies sysctl settings to the system
func (c *SysctlConfig) Apply() error {
	settings := map[string]string{
		// Core TCP settings - BBR requires fq qdisc
		"net.core.default_qdisc":           c.DefaultQdisc,
		"net.ipv4.tcp_congestion_control":  c.TCPCongestionControl,
		"net.ipv4.tcp_no_metrics_save":     fmt.Sprintf("%d", c.TCPNoMetricsSave),
		"net.ipv4.tcp_mtu_probing":         fmt.Sprintf("%d", c.TCPMTUProbing),

		// Buffer sizes
		"net.core.rmem_max":     fmt.Sprintf("%d", c.NetCoreRmemMax),
		"net.core.wmem_max":     fmt.Sprintf("%d", c.NetCoreWmemMax),
		"net.core.rmem_default": fmt.Sprintf("%d", c.NetCoreRmemDefault),
		"net.core.wmem_default": fmt.Sprintf("%d", c.NetCoreWmemDefault),
		"net.ipv4.tcp_rmem":     fmt.Sprintf("%d %d %d", c.TCPRmem[0], c.TCPRmem[1], c.TCPRmem[2]),
		"net.ipv4.tcp_wmem":     fmt.Sprintf("%d %d %d", c.TCPWmem[0], c.TCPWmem[1], c.TCPWmem[2]),

		// Queue settings
		"net.core.netdev_max_backlog":  fmt.Sprintf("%d", c.NetCoreNetdevMaxBacklog),
		"net.core.somaxconn":           fmt.Sprintf("%d", c.NetCoreSomaxconn),
		"net.ipv4.tcp_max_syn_backlog": fmt.Sprintf("%d", c.TCPMaxSynBacklog),

		// Conntrack - critical for SD-WAN
		"net.netfilter.nf_conntrack_max": fmt.Sprintf("%d", c.ConntrackMax),

		// Keepalive
		"net.ipv4.tcp_keepalive_time":   fmt.Sprintf("%d", c.TCPKeepaliveTime),
		"net.ipv4.tcp_keepalive_intvl":  fmt.Sprintf("%d", c.TCPKeepaliveIntvl),
		"net.ipv4.tcp_keepalive_probes": fmt.Sprintf("%d", c.TCPKeepaliveProbes),

		// Memory management
		"net.ipv4.tcp_moderate_rcvbuf": fmt.Sprintf("%d", c.TCPModerateRcvbuf),

		// Additional optimizations
		"net.ipv4.tcp_fastopen":             "3", // Enable TFO for client and server
		"net.ipv4.tcp_slow_start_after_idle": "0", // Don't reset cwnd after idle
		"net.ipv4.tcp_tw_reuse":             "1", // Reuse TIME_WAIT sockets
		"net.ipv4.ip_local_port_range":      "1024 65535", // More ephemeral ports

		// IP forwarding (required for routing)
		"net.ipv4.ip_forward":          "1",
		"net.ipv6.conf.all.forwarding": "1",
	}

	var errors []string
	for key, value := range settings {
		if err := setSysctl(key, value); err != nil {
			// Some settings might not exist on all systems (e.g., conntrack)
			// Only report as error for critical settings
			if !strings.Contains(key, "conntrack") {
				errors = append(errors, fmt.Sprintf("%s: %v", key, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some sysctl settings failed: %s", strings.Join(errors, "; "))
	}
	return nil
}

// setSysctl sets a single sysctl value
func setSysctl(key, value string) error {
	// Try sysctl command first
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	if err := cmd.Run(); err != nil {
		// Fallback to /proc filesystem
		path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return err
		}
	}
	return nil
}

// GenerateSysctlConf generates /etc/sysctl.d/99-aria.conf content
func (c *SysctlConfig) GenerateSysctlConf() string {
	var sb strings.Builder
	sb.WriteString("# Aria Network Performance Tuning\n")
	sb.WriteString("# Generated by aria agent\n")
	sb.WriteString("# https://github.com/anthropics/aria\n\n")

	sb.WriteString("# === TCP Congestion Control (BBR) ===\n")
	sb.WriteString("# BBR provides 2-10x better performance on lossy networks\n")
	sb.WriteString("# IMPORTANT: fq qdisc is required for BBR to work properly\n")
	sb.WriteString(fmt.Sprintf("net.core.default_qdisc = %s\n", c.DefaultQdisc))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_congestion_control = %s\n", c.TCPCongestionControl))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_no_metrics_save = %d\n", c.TCPNoMetricsSave))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_mtu_probing = %d\n", c.TCPMTUProbing))
	sb.WriteString("\n")

	sb.WriteString("# === Buffer Sizes ===\n")
	sb.WriteString("# Large buffers for high bandwidth-delay product (BDP)\n")
	sb.WriteString("# Formula: BDP = Bandwidth * RTT, e.g., 10Gbps * 20ms = 25MB\n")
	sb.WriteString(fmt.Sprintf("net.core.rmem_max = %d\n", c.NetCoreRmemMax))
	sb.WriteString(fmt.Sprintf("net.core.wmem_max = %d\n", c.NetCoreWmemMax))
	sb.WriteString(fmt.Sprintf("net.core.rmem_default = %d\n", c.NetCoreRmemDefault))
	sb.WriteString(fmt.Sprintf("net.core.wmem_default = %d\n", c.NetCoreWmemDefault))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_rmem = %d %d %d\n", c.TCPRmem[0], c.TCPRmem[1], c.TCPRmem[2]))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_wmem = %d %d %d\n", c.TCPWmem[0], c.TCPWmem[1], c.TCPWmem[2]))
	sb.WriteString("\n")

	sb.WriteString("# === Queue Settings ===\n")
	sb.WriteString("# Prevent packet drops during traffic bursts\n")
	sb.WriteString(fmt.Sprintf("net.core.netdev_max_backlog = %d\n", c.NetCoreNetdevMaxBacklog))
	sb.WriteString(fmt.Sprintf("net.core.somaxconn = %d\n", c.NetCoreSomaxconn))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_max_syn_backlog = %d\n", c.TCPMaxSynBacklog))
	sb.WriteString("\n")

	sb.WriteString("# === Connection Tracking (Conntrack) ===\n")
	sb.WriteString("# SD-WAN handles many concurrent connections, default table is too small\n")
	sb.WriteString(fmt.Sprintf("net.netfilter.nf_conntrack_max = %d\n", c.ConntrackMax))
	sb.WriteString("\n")

	sb.WriteString("# === Keepalive ===\n")
	sb.WriteString("# Faster detection of dead connections\n")
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_keepalive_time = %d\n", c.TCPKeepaliveTime))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_keepalive_intvl = %d\n", c.TCPKeepaliveIntvl))
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_keepalive_probes = %d\n", c.TCPKeepaliveProbes))
	sb.WriteString("\n")

	sb.WriteString("# === Additional Optimizations ===\n")
	sb.WriteString("net.ipv4.tcp_fastopen = 3\n")
	sb.WriteString("net.ipv4.tcp_slow_start_after_idle = 0\n")
	sb.WriteString("net.ipv4.tcp_tw_reuse = 1\n")
	sb.WriteString("net.ipv4.ip_local_port_range = 1024 65535\n")
	sb.WriteString(fmt.Sprintf("net.ipv4.tcp_moderate_rcvbuf = %d\n", c.TCPModerateRcvbuf))
	sb.WriteString("\n")

	sb.WriteString("# === IP Forwarding (Required for Routing) ===\n")
	sb.WriteString("net.ipv4.ip_forward = 1\n")
	sb.WriteString("net.ipv6.conf.all.forwarding = 1\n")

	return sb.String()
}

// SaveToFile saves sysctl configuration to a persistent file
func (c *SysctlConfig) SaveToFile(path string) error {
	content := c.GenerateSysctlConf()
	return os.WriteFile(path, []byte(content), 0644)
}
