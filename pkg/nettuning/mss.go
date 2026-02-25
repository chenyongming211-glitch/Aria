package nettuning

import (
	"fmt"
	"os/exec"
	"strings"
)

// MSSClampConfig configures TCP MSS clamping
type MSSClampConfig struct {
	InterfaceName string
	ClampToMTU    bool   // Use --clamp-mss-to-pmtu
	FixedMSS      int    // Fixed MSS value (0 = use PMTU)
	WireGuardMTU  int    // WireGuard interface MTU (typically 1360)
}

// DefaultMSSClampConfig returns default MSS clamping configuration
func DefaultMSSClampConfig(ifaceName string) *MSSClampConfig {
	return &MSSClampConfig{
		InterfaceName: ifaceName,
		ClampToMTU:    true,
		FixedMSS:      0,
		WireGuardMTU:  1360,
	}
}

// Apply configures iptables rules for MSS clamping
func (c *MSSClampConfig) Apply() error {
	// Remove any existing MSS rules for this interface first
	c.Remove()

	var rules [][]string

	if c.ClampToMTU {
		// Automatic PMTU discovery - recommended for most cases
		// This clamps MSS based on the path MTU
		rules = [][]string{
			// For traffic going through the tunnel (FORWARD chain)
			{"-t", "mangle", "-A", "FORWARD", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			// For traffic originating from the local host (OUTPUT chain)
			{"-t", "mangle", "-A", "OUTPUT", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			// For traffic entering the tunnel (POSTROUTING)
			{"-t", "mangle", "-A", "POSTROUTING", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		}
	} else if c.FixedMSS > 0 {
		// Fixed MSS value - use when PMTU discovery doesn't work
		mssStr := fmt.Sprintf("%d", c.FixedMSS)
		rules = [][]string{
			{"-t", "mangle", "-A", "FORWARD", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--set-mss", mssStr},
			{"-t", "mangle", "-A", "OUTPUT", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--set-mss", mssStr},
			{"-t", "mangle", "-A", "POSTROUTING", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--set-mss", mssStr},
		}
	} else {
		// Calculate MSS from WireGuard MTU
		// MSS = MTU - IP header (20) - TCP header (20) - WireGuard overhead
		// WireGuard overhead: 60 bytes for IPv4, 80 for IPv6
		mss := c.WireGuardMTU - 20 - 20
		mssStr := fmt.Sprintf("%d", mss)
		rules = [][]string{
			{"-t", "mangle", "-A", "FORWARD", "-o", c.InterfaceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--set-mss", mssStr},
		}
	}

	// Apply all rules
	var errors []string
	for _, rule := range rules {
		cmd := exec.Command("iptables", rule...)
		if err := cmd.Run(); err != nil {
			errors = append(errors, strings.Join(rule, " "))
		}
	}

	// Also apply for IPv6
	for _, rule := range rules {
		rule6 := make([]string, len(rule))
		copy(rule6, rule)
		cmd := exec.Command("ip6tables", rule6...)
		cmd.Run() // Ignore errors for IPv6
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to apply some MSS rules: %v", errors)
	}

	return nil
}

// Remove removes MSS clamping rules
func (c *MSSClampConfig) Remove() error {
	chains := []string{"FORWARD", "OUTPUT", "POSTROUTING"}

	for _, chain := range chains {
		// List rules and find ones matching our interface
		cmd := exec.Command("iptables", "-t", "mangle", "-S", chain)
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, c.InterfaceName) && strings.Contains(line, "TCPMSS") {
				// Convert -A to -D for deletion
				deleteRule := strings.Replace(line, "-A "+chain, "-D "+chain, 1)
				args := strings.Fields(deleteRule)
				if len(args) > 1 {
					exec.Command("iptables", append([]string{"-t", "mangle"}, args[1:]...)...).Run()
				}
			}
		}
	}

	return nil
}

// CalculateOptimalMSS calculates the optimal MSS for a given MTU
func CalculateOptimalMSS(mtu int, isIPv6 bool) int {
	ipHeaderSize := 20
	if isIPv6 {
		ipHeaderSize = 40
	}
	tcpHeaderSize := 20

	// WireGuard adds ~60 bytes for IPv4, ~80 for IPv6
	wgOverhead := 60
	if isIPv6 {
		wgOverhead = 80
	}

	return mtu - ipHeaderSize - tcpHeaderSize - wgOverhead
}

// CheckMSSRules returns whether MSS clamping is configured
func CheckMSSRules(ifaceName string) (bool, string) {
	cmd := exec.Command("iptables", "-t", "mangle", "-S")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ifaceName) && strings.Contains(line, "TCPMSS") {
			return true, line
		}
	}

	return false, ""
}
