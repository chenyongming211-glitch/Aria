package nettuning

import (
	"fmt"
	"os/exec"
	"strings"
)

// OffloadConfig configures network offload features
type OffloadConfig struct {
	InterfaceName string
	EnableUDPGRO  bool // UDP Generic Receive Offload
	EnableUDPGSO  bool // UDP Generic Segmentation Offload
	EnableTSO     bool // TCP Segmentation Offload
	EnableGSO     bool // Generic Segmentation Offload
}

// DefaultOffloadConfig returns default offload configuration
func DefaultOffloadConfig(ifaceName string) *OffloadConfig {
	return &OffloadConfig{
		InterfaceName: ifaceName,
		EnableUDPGRO:  true,
		EnableUDPGSO:  true,
		EnableTSO:     true,
		EnableGSO:     true,
	}
}

// Apply enables network offload features
// This is critical for WireGuard performance - can improve throughput by 20-50%
func (c *OffloadConfig) Apply() error {
	var errors []string

	// UDP tunnel offload features (critical for WireGuard)
	if c.EnableUDPGSO {
		if err := setOffload(c.InterfaceName, "tx-udp_tnl-segmentation", "on"); err != nil {
			errors = append(errors, fmt.Sprintf("UDP GSO: %v", err))
		}
		if err := setOffload(c.InterfaceName, "tx-udp_tnl-csum-segmentation", "on"); err != nil {
			errors = append(errors, fmt.Sprintf("UDP GSO checksum: %v", err))
		}
	}

	if c.EnableUDPGRO {
		if err := setOffload(c.InterfaceName, "rx-udp_tunnel-port-offload", "on"); err != nil {
			errors = append(errors, fmt.Sprintf("UDP GRO: %v", err))
		}
		// Also enable generic UDP GRO if available
		setOffload(c.InterfaceName, "rx-udp-gro-forwarding", "on") // Non-fatal
	}

	// Generic offload features
	if c.EnableTSO {
		if err := setOffload(c.InterfaceName, "tso", "on"); err != nil {
			errors = append(errors, fmt.Sprintf("TSO: %v", err))
		}
	}

	if c.EnableGSO {
		if err := setOffload(c.InterfaceName, "gso", "on"); err != nil {
			errors = append(errors, fmt.Sprintf("GSO: %v", err))
		}
	}

	// Additional helpful offloads
	setOffload(c.InterfaceName, "tx-checksumming", "on")
	setOffload(c.InterfaceName, "rx-checksumming", "on")
	setOffload(c.InterfaceName, "scatter-gather", "on")

	if len(errors) > 0 {
		return fmt.Errorf("some offload features failed (may not be supported): %s", strings.Join(errors, "; "))
	}

	return nil
}

// setOffload sets a single offload feature using ethtool
func setOffload(ifaceName, feature, value string) error {
	cmd := exec.Command("ethtool", "-K", ifaceName, feature, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set %s=%s: %w", feature, value, err)
	}
	return nil
}

// CheckOffloadStatus returns the current offload configuration
func CheckOffloadStatus(ifaceName string) (map[string]bool, error) {
	cmd := exec.Command("ethtool", "-k", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get offload status: %w", err)
	}

	status := make(map[string]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				feature := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				status[feature] = strings.Contains(value, "on")
			}
		}
	}

	return status, nil
}

// GetUDPOffloadSupport checks if the interface supports UDP offload
func GetUDPOffloadSupport(ifaceName string) (gro, gso bool, err error) {
	status, err := CheckOffloadStatus(ifaceName)
	if err != nil {
		return false, false, err
	}

	// Check for UDP-specific offload features
	for feature, enabled := range status {
		if strings.Contains(feature, "udp") && strings.Contains(feature, "gro") {
			gro = gro || enabled
		}
		if strings.Contains(feature, "udp") && strings.Contains(feature, "segmentation") {
			gso = gso || enabled
		}
	}

	return gro, gso, nil
}
