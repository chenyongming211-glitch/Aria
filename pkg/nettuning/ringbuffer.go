package nettuning

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// RingBufferConfig configures network interface ring buffer sizes
type RingBufferConfig struct {
	InterfaceName string
	RxSize        int // Receive ring buffer size
	TxSize        int // Transmit ring buffer size
	AutoDetect    bool // Automatically use maximum supported size
}

// RingBufferInfo contains current and maximum ring buffer sizes
type RingBufferInfo struct {
	CurrentRx int
	CurrentTx int
	MaxRx     int
	MaxTx     int
}

// DefaultRingBufferConfig returns default ring buffer configuration
func DefaultRingBufferConfig(ifaceName string) *RingBufferConfig {
	return &RingBufferConfig{
		InterfaceName: ifaceName,
		AutoDetect:    true,
	}
}

// GetRingBufferInfo retrieves current and maximum ring buffer sizes
func GetRingBufferInfo(ifaceName string) (*RingBufferInfo, error) {
	cmd := exec.Command("ethtool", "-g", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ring buffer info: %w", err)
	}

	info := &RingBufferInfo{}
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

// Apply configures ring buffer sizes
func (c *RingBufferConfig) Apply() error {
	if c.AutoDetect {
		// Get maximum supported sizes
		info, err := GetRingBufferInfo(c.InterfaceName)
		if err != nil {
			return fmt.Errorf("failed to detect ring buffer sizes: %w", err)
		}

		// Use maximum sizes if they're larger than current
		if info.MaxRx > info.CurrentRx {
			c.RxSize = info.MaxRx
		}
		if info.MaxTx > info.CurrentTx {
			c.TxSize = info.MaxTx
		}
	}

	// Apply new sizes if specified
	if c.RxSize > 0 || c.TxSize > 0 {
		args := []string{"-G", c.InterfaceName}

		if c.RxSize > 0 {
			args = append(args, "rx", fmt.Sprintf("%d", c.RxSize))
		}
		if c.TxSize > 0 {
			args = append(args, "tx", fmt.Sprintf("%d", c.TxSize))
		}

		cmd := exec.Command("ethtool", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set ring buffer: %w, output: %s", err, output)
		}
	}

	return nil
}

// CheckAndOptimizeRingBuffer checks if ring buffer needs optimization and applies it
func CheckAndOptimizeRingBuffer(ifaceName string) (bool, error) {
	info, err := GetRingBufferInfo(ifaceName)
	if err != nil {
		return false, err
	}

	// Check if current sizes are significantly smaller than maximum
	needsOptimization := false
	if info.MaxRx > 0 && info.CurrentRx < info.MaxRx/2 {
		needsOptimization = true
	}
	if info.MaxTx > 0 && info.CurrentTx < info.MaxTx/2 {
		needsOptimization = true
	}

	if needsOptimization {
		cfg := &RingBufferConfig{
			InterfaceName: ifaceName,
			AutoDetect:    true,
		}
		if err := cfg.Apply(); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}
