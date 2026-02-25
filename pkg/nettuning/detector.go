// Package nettuning provides network performance optimization for cloud VMs
package nettuning

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// NetworkInfo contains detected network capabilities
type NetworkInfo struct {
	InterfaceName   string
	QueueCount      int // Hardware queue count
	CPUCount        int
	NeedsRPS        bool   // True if queue < CPU
	DriverName      string // e.g., virtio_net, ena, vmxnet3
	IsVirtual       bool
	SupportsTSO     bool
	SupportsGSO     bool
	CurrentMTU      int
	BBRAvailable    bool
	CurrentCongCtl  string
}

// Detect analyzes the network interface and system capabilities
func Detect(ifaceName string) (*NetworkInfo, error) {
	info := &NetworkInfo{
		InterfaceName: ifaceName,
		CPUCount:      runtime.NumCPU(),
	}

	// Detect queue count
	info.QueueCount = detectQueueCount(ifaceName)

	// Detect driver
	info.DriverName = detectDriver(ifaceName)
	info.IsVirtual = isVirtualNIC(info.DriverName)

	// Check if RPS is needed
	info.NeedsRPS = info.QueueCount < info.CPUCount

	// Detect offload capabilities
	info.SupportsTSO, info.SupportsGSO = detectOffloadCapabilities(ifaceName)

	// Detect MTU
	info.CurrentMTU = detectMTU(ifaceName)

	// Check BBR availability
	info.BBRAvailable = isBBRAvailable()
	info.CurrentCongCtl = getCurrentCongestionControl()

	return info, nil
}

// detectQueueCount returns the number of hardware queues
func detectQueueCount(ifaceName string) int {
	// Try ethtool first
	out, err := exec.Command("ethtool", "-l", ifaceName).Output()
	if err == nil {
		// Parse "Combined: N" from output
		lines := strings.Split(string(out), "\n")
		inCurrent := false
		for _, line := range lines {
			if strings.Contains(line, "Current hardware settings:") {
				inCurrent = true
				continue
			}
			if inCurrent && strings.Contains(line, "Combined:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if n, err := strconv.Atoi(parts[1]); err == nil {
						return n
					}
				}
			}
		}
	}

	// Fallback: count queue directories
	pattern := fmt.Sprintf("/sys/class/net/%s/queues/rx-*", ifaceName)
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return len(matches)
	}

	return 1 // Default to 1 if detection fails
}

// detectDriver returns the network driver name
func detectDriver(ifaceName string) string {
	linkPath := fmt.Sprintf("/sys/class/net/%s/device/driver", ifaceName)
	realPath, err := os.Readlink(linkPath)
	if err != nil {
		return "unknown"
	}
	return filepath.Base(realPath)
}

// isVirtualNIC checks if the driver is for a virtual NIC
func isVirtualNIC(driver string) bool {
	virtualDrivers := map[string]bool{
		"virtio_net": true, // KVM/QEMU
		"vmxnet3":    true, // VMware
		"hv_netvsc":  true, // Hyper-V
		"ena":        true, // AWS ENA
		"ixgbevf":    true, // Intel VF
		"vif":        true, // Xen
	}
	return virtualDrivers[driver]
}

// detectOffloadCapabilities checks TSO/GSO support
func detectOffloadCapabilities(ifaceName string) (tso, gso bool) {
	out, err := exec.Command("ethtool", "-k", ifaceName).Output()
	if err != nil {
		return false, false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tcp-segmentation-offload:") {
			tso = strings.Contains(line, "on")
		}
		if strings.HasPrefix(line, "generic-segmentation-offload:") {
			gso = strings.Contains(line, "on")
		}
	}
	return
}

// detectMTU returns the current MTU
func detectMTU(ifaceName string) int {
	data, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/mtu", ifaceName))
	if err != nil {
		return 1500
	}
	mtu, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if mtu == 0 {
		return 1500
	}
	return mtu
}

// isBBRAvailable checks if BBR congestion control is available
func isBBRAvailable() bool {
	data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "bbr")
}

// getCurrentCongestionControl returns the current TCP congestion control
func getCurrentCongestionControl() string {
	data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_congestion_control")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}
