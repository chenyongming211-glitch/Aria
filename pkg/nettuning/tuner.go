package nettuning

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
)

// Tuner orchestrates all network performance optimizations
type Tuner struct {
	PhysicalInterface string // eth0, ens5, etc.
	TunnelInterface   string // aria0, wg0, etc.
	WireGuardPort     int    // WireGuard UDP port (for NOTRACK)
	Verbose           bool
	ExcludeCPU0       bool // Exclude CPU 0 from RPS for control plane separation

	networkInfo *NetworkInfo
}

// TuneResult contains the results of the tuning operation
type TuneResult struct {
	BBREnabled      bool
	RPSEnabled      bool
	MSSClamped      bool
	SysctlTuned     bool
	OffloadEnabled  bool
	QdiscConfigured bool
	NotrackEnabled  bool
	RingBufferOpt   bool
	MultiQueueOpt   bool
	Warnings        []string
	Errors          []string
}

// NewTuner creates a new network tuner
func NewTuner(physicalIface, tunnelIface string) *Tuner {
	return &Tuner{
		PhysicalInterface: physicalIface,
		TunnelInterface:   tunnelIface,
		WireGuardPort:     51820, // Default WireGuard port
		ExcludeCPU0:       false, // Default: use all CPUs
	}
}

// Tune applies all network optimizations
func (t *Tuner) Tune() (*TuneResult, error) {
	result := &TuneResult{}

	// Check if running as root
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("network tuning requires root privileges")
	}

	// Step 1: Detect network capabilities
	t.log("Detecting network capabilities...")
	info, err := Detect(t.PhysicalInterface)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("detection failed: %v", err))
		info = &NetworkInfo{InterfaceName: t.PhysicalInterface}
	}
	t.networkInfo = info

	t.log("  Interface: %s (driver: %s)", info.InterfaceName, info.DriverName)
	t.log("  Virtual NIC: %v", info.IsVirtual)
	t.log("  CPU cores: %d, NIC queues: %d", info.CPUCount, info.QueueCount)
	t.log("  Current congestion control: %s", info.CurrentCongCtl)

	// Step 2: Apply sysctl optimizations (BBR + buffers)
	t.log("Applying sysctl optimizations...")
	sysctlCfg := DefaultSysctlConfig()

	if !info.BBRAvailable {
		result.Warnings = append(result.Warnings, "BBR not available, using cubic")
		sysctlCfg.TCPCongestionControl = "cubic"
	}

	if err := sysctlCfg.Apply(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("sysctl: %v", err))
	} else {
		result.SysctlTuned = true
		if info.BBRAvailable {
			result.BBREnabled = true
			t.log("  ✓ BBR congestion control enabled")
		}
		t.log("  ✓ TCP buffer sizes optimized")
	}

	// Persist sysctl settings
	if err := sysctlCfg.SaveToFile("/etc/sysctl.d/99-aria.conf"); err != nil {
		result.Warnings = append(result.Warnings, "could not persist sysctl settings")
	}

	// Step 3: Optimize hardware multi-queue (RSS) - do this BEFORE RPS
	t.log("Optimizing hardware multi-queue (RSS)...")
	mqOptimized, queueCount, err := CheckAndOptimizeMultiQueue(t.PhysicalInterface)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("multi-queue: %v", err))
	} else if mqOptimized {
		result.MultiQueueOpt = true
		t.log("  ✓ Hardware queues increased to %d (matching CPU count)", queueCount)
		// Re-detect after queue optimization
		info, _ = Detect(t.PhysicalInterface)
		t.networkInfo = info
	} else {
		t.log("  ✓ Hardware queues already optimal (%d queues)", queueCount)
	}

	// Step 4: Configure RPS if still needed (queue count < CPU count)
	// After multi-queue optimization, this should rarely be needed
	if info.NeedsRPS {
		t.log("Configuring RPS/RFS (queues %d < CPUs %d)...", info.QueueCount, info.CPUCount)
		rpsCfg, err := NewRPSConfig(t.PhysicalInterface)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("RPS config: %v", err))
		} else {
			rpsCfg.ExcludeCPU0 = t.ExcludeCPU0
			if err := rpsCfg.Apply(); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("RPS apply: %v", err))
			} else {
				result.RPSEnabled = true
				if t.ExcludeCPU0 {
					t.log("  ✓ RPS/RFS enabled across %d CPUs (excluding CPU 0)", info.CPUCount-1)
				} else {
					t.log("  ✓ RPS/RFS enabled across %d CPUs", info.CPUCount)
				}
				t.log("  ✓ rps_sock_flow_entries = %d", info.CPUCount*rpsCfg.FlowCount)
			}
		}

		// Also configure XPS for transmit
		xpsCfg, _ := NewXPSConfig(t.PhysicalInterface)
		if xpsCfg != nil {
			xpsCfg.Apply() // Best effort
		}
	} else {
		t.log("RPS not needed (hardware has %d queues for %d CPUs)", info.QueueCount, info.CPUCount)
	}

	// Also apply RPS to tunnel interface if it exists
	if t.TunnelInterface != "" {
		tunnelRPS, err := NewRPSConfig(t.TunnelInterface)
		if err == nil {
			tunnelRPS.ExcludeCPU0 = t.ExcludeCPU0
			tunnelRPS.Apply() // Best effort, tunnel might not exist yet
		}
	}

	// Step 5: Configure MSS clamping for tunnel interface
	if t.TunnelInterface != "" {
		t.log("Configuring MSS clamping for %s...", t.TunnelInterface)
		mssCfg := DefaultMSSClampConfig(t.TunnelInterface)
		if err := mssCfg.Apply(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("MSS clamp: %v", err))
		} else {
			result.MSSClamped = true
			t.log("  ✓ MSS clamping enabled (PMTU discovery)")
		}
	}

	// Step 5: Enable UDP offload (GRO/GSO) for physical interface
	t.log("Enabling UDP offload features...")
	offloadCfg := DefaultOffloadConfig(t.PhysicalInterface)
	if err := offloadCfg.Apply(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("offload: %v", err))
	} else {
		result.OffloadEnabled = true
		t.log("  ✓ UDP GRO/GSO enabled (20-50%% throughput boost)")
	}

	// Step 6: Configure optimal qdisc
	t.log("Configuring queuing discipline...")
	// For physical interface, try CAKE with auto-detected bandwidth
	if err := ApplyOptimalQdisc(t.PhysicalInterface, true, "1Gbit"); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("qdisc physical: %v", err))
	} else {
		result.QdiscConfigured = true
		qdisc, _ := GetCurrentQdisc(t.PhysicalInterface)
		t.log("  ✓ Physical interface qdisc: %s", qdisc)
	}

	// For tunnel interface, use fq_codel
	if t.TunnelInterface != "" {
		if err := ApplyOptimalQdisc(t.TunnelInterface, false, ""); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("qdisc tunnel: %v", err))
		} else {
			qdisc, _ := GetCurrentQdisc(t.TunnelInterface)
			t.log("  ✓ Tunnel interface qdisc: %s", qdisc)
		}
	}

	// Step 7: Configure NOTRACK for WireGuard port
	if t.WireGuardPort > 0 {
		t.log("Configuring NOTRACK for WireGuard port %d...", t.WireGuardPort)
		notrackCfg := DefaultNotrackConfig(t.WireGuardPort)
		if err := notrackCfg.Apply(); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("notrack: %v", err))
		} else {
			result.NotrackEnabled = true
			t.log("  ✓ NOTRACK enabled (bypassing conntrack for WireGuard)")
		}
	}

	// Step 9: Optimize ring buffer
	t.log("Optimizing ring buffer...")
	ringOptimized, err := CheckAndOptimizeRingBuffer(t.PhysicalInterface)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("ring buffer: %v", err))
	} else if ringOptimized {
		result.RingBufferOpt = true
		t.log("  ✓ Ring buffer optimized to maximum size")
	} else {
		t.log("  ✓ Ring buffer already at optimal size")
	}

	// Note: IP forwarding is already enabled via sysctl settings
	t.log("IP forwarding enabled via sysctl")

	return result, nil
}

// GetStatus returns the current tuning status
func (t *Tuner) GetStatus() string {
	var sb strings.Builder

	sb.WriteString("=== Aria Network Tuning Status ===\n\n")

	// BBR status
	currentCC := getCurrentCongestionControl()
	if currentCC == "bbr" {
		sb.WriteString("✓ BBR: enabled\n")
	} else {
		sb.WriteString(fmt.Sprintf("✗ BBR: disabled (using %s)\n", currentCC))
	}

	// Multi-queue status
	if t.PhysicalInterface != "" {
		queueInfo, err := GetQueueInfo(t.PhysicalInterface)
		if err == nil {
			sb.WriteString(fmt.Sprintf("✓ Multi-Queue: %d/%d queues (CPUs: %d)\n",
				queueInfo.CurrentCombined, queueInfo.MaxCombined, runtime.NumCPU()))
		}
	}

	// RPS status
	if t.PhysicalInterface != "" {
		enabled, mask := CheckRPSStatus(t.PhysicalInterface)
		if enabled {
			sb.WriteString(fmt.Sprintf("✓ RPS: enabled (mask: %s)\n", mask))
		} else {
			sb.WriteString("✗ RPS: disabled\n")
		}
	}

	// MSS clamping status
	if t.TunnelInterface != "" {
		enabled, rule := CheckMSSRules(t.TunnelInterface)
		if enabled {
			sb.WriteString("✓ MSS Clamping: enabled\n")
		} else {
			sb.WriteString("✗ MSS Clamping: disabled\n")
		}
		_ = rule
	}

	// UDP Offload status
	if t.PhysicalInterface != "" {
		gro, gso, err := GetUDPOffloadSupport(t.PhysicalInterface)
		if err == nil {
			if gro || gso {
				sb.WriteString(fmt.Sprintf("✓ UDP Offload: GRO=%v, GSO=%v\n", gro, gso))
			} else {
				sb.WriteString("✗ UDP Offload: disabled\n")
			}
		}
	}

	// Qdisc status
	if t.PhysicalInterface != "" {
		qdisc, _ := GetCurrentQdisc(t.PhysicalInterface)
		sb.WriteString(fmt.Sprintf("✓ Qdisc: %s\n", qdisc))
	}

	// NOTRACK status
	if t.WireGuardPort > 0 {
		enabled, _ := CheckNotrackRules(t.WireGuardPort)
		if enabled {
			sb.WriteString(fmt.Sprintf("✓ NOTRACK: enabled (port %d)\n", t.WireGuardPort))
		} else {
			sb.WriteString(fmt.Sprintf("✗ NOTRACK: disabled (port %d)\n", t.WireGuardPort))
		}
	}

	// Ring Buffer status
	if t.PhysicalInterface != "" {
		info, err := GetRingBufferInfo(t.PhysicalInterface)
		if err == nil {
			sb.WriteString(fmt.Sprintf("✓ Ring Buffer: RX=%d/%d, TX=%d/%d\n",
				info.CurrentRx, info.MaxRx, info.CurrentTx, info.MaxTx))
		}
	}

	// Network info
	if t.networkInfo != nil {
		sb.WriteString(fmt.Sprintf("\nInterface: %s\n", t.networkInfo.InterfaceName))
		sb.WriteString(fmt.Sprintf("  Driver: %s\n", t.networkInfo.DriverName))
		sb.WriteString(fmt.Sprintf("  Virtual: %v\n", t.networkInfo.IsVirtual))
		sb.WriteString(fmt.Sprintf("  Queues: %d (CPUs: %d)\n", t.networkInfo.QueueCount, t.networkInfo.CPUCount))
		sb.WriteString(fmt.Sprintf("  MTU: %d\n", t.networkInfo.CurrentMTU))
	}

	return sb.String()
}

// log prints a message if verbose mode is enabled
func (t *Tuner) log(format string, args ...interface{}) {
	if t.Verbose {
		log.Printf(format, args...)
	}
}

// AutoDetectPhysicalInterface finds the default network interface
func AutoDetectPhysicalInterface() string {
	// Common interface names in order of preference
	candidates := []string{
		"eth0", "ens5", "ens3", "enp0s3", // Cloud VMs
		"eno1", "eno2",                   // Physical servers
		"enp1s0", "enp2s0",               // Modern Linux
	}

	for _, name := range candidates {
		path := fmt.Sprintf("/sys/class/net/%s", name)
		if _, err := os.Stat(path); err == nil {
			return name
		}
	}

	// Fallback: find first non-loopback interface
	entries, _ := os.ReadDir("/sys/class/net")
	for _, entry := range entries {
		name := entry.Name()
		if name != "lo" && !strings.HasPrefix(name, "docker") &&
			!strings.HasPrefix(name, "veth") && !strings.HasPrefix(name, "br-") &&
			!strings.HasPrefix(name, "aria") && !strings.HasPrefix(name, "wg") {
			return name
		}
	}

	return "eth0" // Default fallback
}
