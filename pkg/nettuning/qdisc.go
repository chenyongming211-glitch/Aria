package nettuning

import (
	"fmt"
	"os/exec"
	"strings"
)

// QdiscType represents the queuing discipline algorithm
type QdiscType string

const (
	QdiscFQ      QdiscType = "fq"        // Fair Queue (required for BBR)
	QdiscFQCodel QdiscType = "fq_codel"  // Fair Queue CoDel (best for most cases)
	QdiscCAKE    QdiscType = "cake"      // Common Applications Kept Enhanced (best for SD-WAN)
	QdiscPFIFO   QdiscType = "pfifo_fast" // Default Linux qdisc
)

// QdiscConfig configures queuing discipline
type QdiscConfig struct {
	InterfaceName string
	Type          QdiscType
	Bandwidth     string // e.g., "1Gbit", "100Mbit" (for CAKE)
	Options       []string
}

// DefaultQdiscConfig returns default qdisc configuration
// For WireGuard interfaces, use fq_codel to reduce latency jitter
// For physical interfaces with known bandwidth, use CAKE
func DefaultQdiscConfig(ifaceName string, isPhysical bool) *QdiscConfig {
	if isPhysical {
		// For physical interfaces, use CAKE if available, otherwise fq_codel
		return &QdiscConfig{
			InterfaceName: ifaceName,
			Type:          QdiscCAKE,
			Bandwidth:     "1Gbit", // Adjust based on actual link speed
			Options:       []string{"besteffort"},
		}
	}

	// For virtual interfaces (WireGuard), use fq_codel
	return &QdiscConfig{
		InterfaceName: ifaceName,
		Type:          QdiscFQCodel,
		Options:       []string{},
	}
}

// Apply configures the queuing discipline
func (c *QdiscConfig) Apply() error {
	// Remove existing qdisc first
	c.Remove()

	var args []string
	args = append(args, "qdisc", "add", "dev", c.InterfaceName, "root")

	switch c.Type {
	case QdiscCAKE:
		args = append(args, "cake")
		if c.Bandwidth != "" {
			args = append(args, "bandwidth", c.Bandwidth)
		}
		args = append(args, c.Options...)

	case QdiscFQCodel:
		args = append(args, "fq_codel")
		args = append(args, c.Options...)

	case QdiscFQ:
		args = append(args, "fq")
		args = append(args, c.Options...)

	default:
		return fmt.Errorf("unsupported qdisc type: %s", c.Type)
	}

	cmd := exec.Command("tc", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply qdisc: %w, output: %s", err, output)
	}

	return nil
}

// Remove removes the current qdisc configuration
func (c *QdiscConfig) Remove() error {
	cmd := exec.Command("tc", "qdisc", "del", "dev", c.InterfaceName, "root")
	cmd.Run() // Ignore errors (qdisc might not exist)
	return nil
}

// GetCurrentQdisc returns the current qdisc configuration
func GetCurrentQdisc(ifaceName string) (string, error) {
	cmd := exec.Command("tc", "qdisc", "show", "dev", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get qdisc: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "qdisc") && strings.Contains(line, "root") {
			return strings.TrimSpace(line), nil
		}
	}

	return "unknown", nil
}

// IsCAKEAvailable checks if CAKE qdisc module is available
func IsCAKEAvailable() bool {
	cmd := exec.Command("tc", "qdisc", "add", "dev", "lo", "root", "cake")
	err := cmd.Run()
	if err == nil {
		// Clean up test qdisc
		exec.Command("tc", "qdisc", "del", "dev", "lo", "root").Run()
		return true
	}
	return false
}

// IsFQCodelAvailable checks if fq_codel is available
func IsFQCodelAvailable() bool {
	cmd := exec.Command("tc", "qdisc", "add", "dev", "lo", "root", "fq_codel")
	err := cmd.Run()
	if err == nil {
		// Clean up test qdisc
		exec.Command("tc", "qdisc", "del", "dev", "lo", "root").Run()
		return true
	}
	return false
}

// ApplyOptimalQdisc applies the best available qdisc for the interface
func ApplyOptimalQdisc(ifaceName string, isPhysical bool, bandwidth string) error {
	var cfg *QdiscConfig

	if isPhysical && IsCAKEAvailable() {
		// CAKE is best for physical interfaces with known bandwidth
		cfg = &QdiscConfig{
			InterfaceName: ifaceName,
			Type:          QdiscCAKE,
			Bandwidth:     bandwidth,
			Options:       []string{"besteffort"},
		}
	} else if IsFQCodelAvailable() {
		// fq_codel is widely available and works well
		cfg = &QdiscConfig{
			InterfaceName: ifaceName,
			Type:          QdiscFQCodel,
		}
	} else {
		// Fallback to fq (required for BBR anyway)
		cfg = &QdiscConfig{
			InterfaceName: ifaceName,
			Type:          QdiscFQ,
		}
	}

	return cfg.Apply()
}
