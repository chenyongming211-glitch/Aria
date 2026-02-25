package nettuning

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// MultiQueueConfig configures network interface multi-queue (RSS)
type MultiQueueConfig struct {
	InterfaceName string
	TargetQueues  int  // Desired number of queues (0 = auto-detect to match CPU count)
	AutoOptimize  bool // Automatically set queues to CPU count
}

// QueueInfo contains current and maximum queue information
type QueueInfo struct {
	CurrentCombined int
	CurrentRx       int
	CurrentTx       int
	MaxCombined     int
	MaxRx           int
	MaxTx           int
}

// DefaultMultiQueueConfig returns default multi-queue configuration
func DefaultMultiQueueConfig(ifaceName string) *MultiQueueConfig {
	return &MultiQueueConfig{
		InterfaceName: ifaceName,
		TargetQueues:  0, // Auto-detect
		AutoOptimize:  true,
	}
}

// GetQueueInfo retrieves current and maximum queue counts
func GetQueueInfo(ifaceName string) (*QueueInfo, error) {
	cmd := exec.Command("ethtool", "-l", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue info: %w", err)
	}

	info := &QueueInfo{}
	lines := strings.Split(string(output), "\n")

	inPreset := false
	inCurrent := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Pre-set maximums:") {
			inPreset = true
			inCurrent = false
			continue
		}
		if strings.Contains(line, "Current hardware settings:") {
			inPreset = false
			inCurrent = true
			continue
		}

		// Parse queue counts
		if strings.HasPrefix(line, "Combined:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.Atoi(parts[1])
				if inPreset {
					info.MaxCombined = val
				} else if inCurrent {
					info.CurrentCombined = val
				}
			}
		}

		if strings.HasPrefix(line, "RX:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.Atoi(parts[1])
				if inPreset {
					info.MaxRx = val
				} else if inCurrent {
					info.CurrentRx = val
				}
			}
		}

		if strings.HasPrefix(line, "TX:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.Atoi(parts[1])
				if inPreset {
					info.MaxTx = val
				} else if inCurrent {
					info.CurrentTx = val
				}
			}
		}
	}

	return info, nil
}

// Apply configures multi-queue settings
func (c *MultiQueueConfig) Apply() error {
	info, err := GetQueueInfo(c.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to get queue info: %w", err)
	}

	// Determine target queue count
	targetQueues := c.TargetQueues
	if c.AutoOptimize || targetQueues == 0 {
		cpuCount := runtime.NumCPU()
		// Use the minimum of CPU count and max supported queues
		if info.MaxCombined > 0 {
			targetQueues = min(cpuCount, info.MaxCombined)
		} else if info.MaxRx > 0 && info.MaxTx > 0 {
			targetQueues = min(cpuCount, min(info.MaxRx, info.MaxTx))
		} else {
			return fmt.Errorf("interface does not support multi-queue")
		}
	}

	// Check if already optimal
	if info.CurrentCombined >= targetQueues {
		return nil // Already optimal
	}

	// Apply new queue count
	// Try combined queues first (most common for modern NICs)
	if info.MaxCombined > 0 {
		cmd := exec.Command("ethtool", "-L", c.InterfaceName, "combined", fmt.Sprintf("%d", targetQueues))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set combined queues: %w, output: %s", err, output)
		}
		return nil
	}

	// Fallback: set RX and TX separately
	if info.MaxRx > 0 && info.MaxTx > 0 {
		cmd := exec.Command("ethtool", "-L", c.InterfaceName,
			"rx", fmt.Sprintf("%d", targetQueues),
			"tx", fmt.Sprintf("%d", targetQueues))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set rx/tx queues: %w, output: %s", err, output)
		}
		return nil
	}

	return fmt.Errorf("interface does not support queue configuration")
}

// CheckAndOptimizeMultiQueue checks if multi-queue needs optimization and applies it
func CheckAndOptimizeMultiQueue(ifaceName string) (bool, int, error) {
	info, err := GetQueueInfo(ifaceName)
	if err != nil {
		return false, 0, err
	}

	cpuCount := runtime.NumCPU()
	currentQueues := info.CurrentCombined
	if currentQueues == 0 {
		currentQueues = info.CurrentRx
	}

	// Check if optimization is needed
	maxQueues := info.MaxCombined
	if maxQueues == 0 {
		maxQueues = info.MaxRx
	}

	if maxQueues == 0 {
		return false, 0, fmt.Errorf("interface does not support multi-queue")
	}

	targetQueues := min(cpuCount, maxQueues)

	if currentQueues >= targetQueues {
		return false, currentQueues, nil // Already optimal
	}

	// Apply optimization
	cfg := &MultiQueueConfig{
		InterfaceName: ifaceName,
		TargetQueues:  targetQueues,
	}

	if err := cfg.Apply(); err != nil {
		return false, currentQueues, err
	}

	return true, targetQueues, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
