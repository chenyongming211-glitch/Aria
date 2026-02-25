package nettuning

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RPSConfig configures Receive Packet Steering
type RPSConfig struct {
	InterfaceName string
	CPUCount      int
	RxQueues      []string // e.g., ["rx-0", "rx-1"]
	FlowCount     int      // rps_flow_cnt per queue (default 4096)
	ExcludeCPU0   bool     // Exclude CPU 0 for control plane separation
}

// NewRPSConfig creates a new RPS configuration
func NewRPSConfig(ifaceName string) (*RPSConfig, error) {
	cfg := &RPSConfig{
		InterfaceName: ifaceName,
		CPUCount:      runtime.NumCPU(),
		FlowCount:     4096, // Per-queue flow count
		ExcludeCPU0:   false, // Default: use all CPUs
	}

	// Find all RX queues
	pattern := fmt.Sprintf("/sys/class/net/%s/queues/rx-*", ifaceName)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find rx queues: %w", err)
	}

	for _, match := range matches {
		cfg.RxQueues = append(cfg.RxQueues, filepath.Base(match))
	}

	if len(cfg.RxQueues) == 0 {
		cfg.RxQueues = []string{"rx-0"} // Default if not found
	}

	return cfg, nil
}

// Apply enables RPS and RFS on all RX queues
func (c *RPSConfig) Apply() error {
	// Calculate CPU mask (all CPUs or exclude CPU 0)
	// e.g., 4 CPUs -> 0xf (binary: 1111) or 0xe (binary: 1110, excluding CPU 0)
	// e.g., 8 CPUs -> 0xff (binary: 11111111) or 0xfe (binary: 11111110, excluding CPU 0)
	cpuMask := calculateCPUMask(c.CPUCount, c.ExcludeCPU0)

	// Set global RFS flow table size: CPUs * FlowCount
	// This enables Receive Flow Steering (RFS) which is socket-aware
	globalFlowEntries := c.CPUCount * c.FlowCount
	if err := os.WriteFile("/proc/sys/net/core/rps_sock_flow_entries",
		[]byte(fmt.Sprintf("%d", globalFlowEntries)), 0644); err != nil {
		// Non-fatal, log and continue
	}

	for _, queue := range c.RxQueues {
		// Set RPS CPU mask - distribute packets across all CPUs
		rpsPath := fmt.Sprintf("/sys/class/net/%s/queues/%s/rps_cpus", c.InterfaceName, queue)
		if err := os.WriteFile(rpsPath, []byte(cpuMask), 0644); err != nil {
			return fmt.Errorf("failed to set RPS for %s: %w", queue, err)
		}

		// Set RPS flow count for better distribution (enables RFS per-queue)
		flowPath := fmt.Sprintf("/sys/class/net/%s/queues/%s/rps_flow_cnt", c.InterfaceName, queue)
		if err := os.WriteFile(flowPath, []byte(fmt.Sprintf("%d", c.FlowCount)), 0644); err != nil {
			// Non-fatal, some systems don't support this
		}
	}

	return nil
}

// calculateCPUMask generates hex mask for CPUs
// e.g., 4 CPUs = "f", 8 CPUs = "ff", 16 CPUs = "ffff"
// If excludeCPU0 is true, CPU 0 is excluded (e.g., 4 CPUs = "e", 8 CPUs = "fe")
func calculateCPUMask(cpuCount int, excludeCPU0 bool) string {
	if cpuCount <= 0 {
		return "0"
	}

	// For large CPU counts, we need to handle this differently
	// Each hex digit represents 4 CPUs
	// CPU 0-3   -> f     (1111)
	// CPU 4-7   -> f     (1111)
	// CPU 8-11  -> f     (1111)
	// etc.

	if cpuCount <= 4 {
		mask := (1 << cpuCount) - 1
		if excludeCPU0 {
			mask &= ^1 // Clear bit 0
		}
		return fmt.Sprintf("%x", mask)
	}

	// For more than 4 CPUs, generate the full hex string
	fullGroups := cpuCount / 4
	remaining := cpuCount % 4

	var sb strings.Builder
	if remaining > 0 {
		mask := (1 << remaining) - 1
		sb.WriteString(fmt.Sprintf("%x", mask))
	}
	for i := 0; i < fullGroups; i++ {
		sb.WriteString("f")
	}

	result := sb.String()

	// If excluding CPU 0, modify the last character
	if excludeCPU0 && len(result) > 0 {
		lastChar := result[len(result)-1]
		if lastChar >= '0' && lastChar <= '9' {
			// Clear bit 0 of the last digit
			newVal := (lastChar - '0') & 0xe
			result = result[:len(result)-1] + fmt.Sprintf("%x", newVal)
		} else if lastChar >= 'a' && lastChar <= 'f' {
			// Clear bit 0 of the last digit
			newVal := (lastChar - 'a' + 10) & 0xe
			result = result[:len(result)-1] + fmt.Sprintf("%x", newVal)
		}
	}

	return result
}

// XPSConfig configures Transmit Packet Steering
type XPSConfig struct {
	InterfaceName string
	CPUCount      int
	TxQueues      []string
}

// NewXPSConfig creates a new XPS configuration
func NewXPSConfig(ifaceName string) (*XPSConfig, error) {
	cfg := &XPSConfig{
		InterfaceName: ifaceName,
		CPUCount:      runtime.NumCPU(),
	}

	pattern := fmt.Sprintf("/sys/class/net/%s/queues/tx-*", ifaceName)
	matches, _ := filepath.Glob(pattern)
	for _, match := range matches {
		cfg.TxQueues = append(cfg.TxQueues, filepath.Base(match))
	}

	return cfg, nil
}

// Apply enables XPS (distributes TX across CPUs)
func (c *XPSConfig) Apply() error {
	if len(c.TxQueues) == 0 {
		return nil
	}

	// Distribute TX queues evenly across CPUs
	for i, queue := range c.TxQueues {
		cpuIndex := i % c.CPUCount
		mask := 1 << cpuIndex

		xpsPath := fmt.Sprintf("/sys/class/net/%s/queues/%s/xps_cpus", c.InterfaceName, queue)
		if err := os.WriteFile(xpsPath, []byte(fmt.Sprintf("%x", mask)), 0644); err != nil {
			// Non-fatal, not all systems support XPS
		}
	}

	return nil
}

// CheckRPSStatus returns the current RPS configuration status
func CheckRPSStatus(ifaceName string) (bool, string) {
	rpsPath := fmt.Sprintf("/sys/class/net/%s/queues/rx-0/rps_cpus", ifaceName)
	data, err := os.ReadFile(rpsPath)
	if err != nil {
		return false, "unknown"
	}

	mask := strings.TrimSpace(string(data))
	// If mask is all zeros, RPS is disabled
	if mask == "" || mask == "0" || strings.Trim(mask, "0,") == "" {
		return false, mask
	}

	return true, mask
}
