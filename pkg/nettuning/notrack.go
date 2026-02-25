package nettuning

// DEPRECATED: This file is deprecated and kept for reference only.
//
// NOTRACK functionality has been migrated to nftables raw table.
// See: pkg/datapath/nftables-init.nft (aria_raw table)
//
// Migration Notes:
//   - Old: iptables raw table NOTRACK rules (this file)
//   - New: nftables raw table with notrack action
//   - Reason: Unified firewall management, better performance
//
// The new implementation provides:
//   1. Unified nftables management (no iptables/nftables mixing)
//   2. Atomic rule application
//   3. Better integration with ACL rules
//   4. Stateless bidirectional ACL support
//
// DO NOT USE THIS FILE IN NEW CODE.

import (
	"fmt"
	"os/exec"
	"strings"
)

// NotrackConfig configures iptables NOTRACK rules for WireGuard
// This bypasses connection tracking for WireGuard UDP traffic,
// significantly reducing CPU overhead in high-concurrency scenarios
type NotrackConfig struct {
	Port      int    // WireGuard UDP port (typically 51820)
	Interface string // Optional: specific interface to apply rules
}

// DefaultNotrackConfig returns default NOTRACK configuration
func DefaultNotrackConfig(port int) *NotrackConfig {
	return &NotrackConfig{
		Port: port,
	}
}

// Apply configures iptables NOTRACK rules
// This is critical for high-performance WireGuard deployments
func (c *NotrackConfig) Apply() error {
	// Remove existing rules first
	c.Remove()

	portStr := fmt.Sprintf("%d", c.Port)

	// Rules to apply (in order)
	rules := [][]string{
		// 1. NOTRACK in raw table PREROUTING (incoming packets)
		{"-t", "raw", "-A", "PREROUTING", "-p", "udp", "--dport", portStr, "-j", "NOTRACK"},
		// 2. NOTRACK in raw table OUTPUT (outgoing packets)
		{"-t", "raw", "-A", "OUTPUT", "-p", "udp", "--sport", portStr, "-j", "NOTRACK"},
		// 3. Accept in filter table INPUT (since conntrack is bypassed)
		{"-A", "INPUT", "-p", "udp", "--dport", portStr, "-j", "ACCEPT"},
		// 4. Accept in filter table OUTPUT
		{"-A", "OUTPUT", "-p", "udp", "--sport", portStr, "-j", "ACCEPT"},
	}

	// Apply IPv4 rules
	var errors []string
	for _, rule := range rules {
		cmd := exec.Command("iptables", rule...)
		if err := cmd.Run(); err != nil {
			errors = append(errors, fmt.Sprintf("iptables %s: %v", strings.Join(rule, " "), err))
		}
	}

	// Apply IPv6 rules (non-fatal if fails)
	for _, rule := range rules {
		cmd := exec.Command("ip6tables", rule...)
		cmd.Run() // Ignore errors for IPv6
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to apply some NOTRACK rules: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Remove removes NOTRACK rules
func (c *NotrackConfig) Remove() error {
	portStr := fmt.Sprintf("%d", c.Port)

	// Find and delete matching rules
	chains := []struct {
		table string
		chain string
	}{
		{"raw", "PREROUTING"},
		{"raw", "OUTPUT"},
		{"filter", "INPUT"},
		{"filter", "OUTPUT"},
	}

	for _, ch := range chains {
		// List rules
		cmd := exec.Command("iptables", "-t", ch.table, "-S", ch.chain)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		// Find rules matching our port
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, portStr) && (strings.Contains(line, "NOTRACK") || strings.Contains(line, "ACCEPT")) {
				// Convert -A to -D for deletion
				deleteRule := strings.Replace(line, "-A "+ch.chain, "-D "+ch.chain, 1)
				args := strings.Fields(deleteRule)
				if len(args) > 1 {
					delCmd := exec.Command("iptables", append([]string{"-t", ch.table}, args[1:]...)...)
					delCmd.Run() // Ignore errors
				}
			}
		}
	}

	// Also try IPv6
	for _, ch := range chains {
		cmd := exec.Command("ip6tables", "-t", ch.table, "-S", ch.chain)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, portStr) && (strings.Contains(line, "NOTRACK") || strings.Contains(line, "ACCEPT")) {
				deleteRule := strings.Replace(line, "-A "+ch.chain, "-D "+ch.chain, 1)
				args := strings.Fields(deleteRule)
				if len(args) > 1 {
					delCmd := exec.Command("ip6tables", append([]string{"-t", ch.table}, args[1:]...)...)
					delCmd.Run()
				}
			}
		}
	}

	return nil
}

// CheckNotrackRules checks if NOTRACK rules are configured
func CheckNotrackRules(port int) (bool, string) {
	portStr := fmt.Sprintf("%d", port)

	// Check raw table PREROUTING
	cmd := exec.Command("iptables", "-t", "raw", "-S", "PREROUTING")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, portStr) && strings.Contains(line, "NOTRACK") {
			return true, line
		}
	}

	return false, ""
}

// GetConntrackCount returns the current number of tracked connections
func GetConntrackCount() (int, error) {
	cmd := exec.Command("conntrack", "-C")
	output, err := cmd.Output()
	if err != nil {
		// Try alternative method
		cmd = exec.Command("cat", "/proc/sys/net/netfilter/nf_conntrack_count")
		output, err = cmd.Output()
		if err != nil {
			return 0, fmt.Errorf("failed to get conntrack count: %w", err)
		}
	}

	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("failed to parse conntrack count: %w", err)
	}

	return count, nil
}
