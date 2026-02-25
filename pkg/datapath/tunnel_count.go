package datapath

import (
	"runtime"
)

const (
	// DefaultTunnelCount is the default number of tunnels for all nodes.
	// This ensures consistent tunnel count across all agents regardless of CPU cores.
	DefaultTunnelCount = 4

	// MinTunnelCount is the minimum number of tunnels.
	MinTunnelCount = 1

	// MaxTunnelCount is the maximum number of tunnels.
	MaxTunnelCount = 8
)

// DetermineTunnelCount returns the tunnel count based on configuration.
// If count is 0, returns the default value (4).
// If count is specified, validates and returns it.
func DetermineTunnelCount(count int) int {
	// If count is 0, use default
	if count == 0 {
		return DefaultTunnelCount
	}

	// Validate range
	if count < MinTunnelCount {
		return MinTunnelCount
	}
	if count > MaxTunnelCount {
		return MaxTunnelCount
	}

	return count
}

// DetectCPUCount returns the number of CPU cores
func DetectCPUCount() int {
	return runtime.NumCPU()
}
