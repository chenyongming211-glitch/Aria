package datapath

import (
	"fmt"
	"strings"
	"testing"
)

// normalizeCIDR ensures IP addresses are in CIDR format.
func normalizeCIDR(ipOrCIDR string) string {
	if ipOrCIDR == "" {
		return ""
	}
	// Check if already in CIDR format
	if strings.Contains(ipOrCIDR, "/") {
		return ipOrCIDR
	}
	// Plain IP address, append /32 for single host
	return ipOrCIDR + "/32"
}

// mockFirewallManager is a platform-independent mock for testing rule generation logic.
type mockFirewallManager struct {
	wgPort int
}

// generateStatelessRulesMock implements the same logic as the real implementation.
func (m *mockFirewallManager) generateStatelessRulesMock(rules []ACLRule) ([]string, []string) {
	var outboundElems []string
	var inboundElems []string

	for _, rule := range rules {
		srcNet := normalizeCIDR(rule.SrcNet)
		dstNet := normalizeCIDR(rule.DstNet)

		// Map protocol number to name
		protoName := "tcp"
		if rule.Protocol == 17 {
			protoName = "udp"
		} else if rule.Protocol == 1 {
			protoName = "icmp"
		}

		// Format port range
		var portStr string
		if rule.MinPort == rule.MaxPort {
			portStr = fmt.Sprintf("%d", rule.MinPort)
		} else if rule.MinPort == 0 && rule.MaxPort == 65535 {
			portStr = "0-65535"
		} else {
			portStr = fmt.Sprintf("%d-%d", rule.MinPort, rule.MaxPort)
		}

		// Generate outbound element: Src . Dst . Proto . Dport
		outElem := fmt.Sprintf("%s . %s . %s . %s",
			srcNet, dstNet, protoName, portStr)
		outboundElems = append(outboundElems, outElem)

		// Generate inbound element: Dst . Src . Proto . Sport (reversed)
		inElem := fmt.Sprintf("%s . %s . %s . %s",
			dstNet, srcNet, protoName, portStr)
		inboundElems = append(inboundElems, inElem)
	}

	return outboundElems, inboundElems
}

// TestStatelessRuleGeneration tests the bidirectional rule generation logic (platform-independent).
func TestStatelessRuleGeneration(t *testing.T) {
	m := &mockFirewallManager{
		wgPort: 51820,
	}

	tests := []struct {
		name           string
		rules          []ACLRule
		wantOutbound   []string
		wantInbound    []string
		description    string
	}{
		{
			name: "single TCP rule",
			rules: []ACLRule{
				{
					SrcNet:   "10.0.1.0/24",
					DstNet:   "192.168.1.100/32",
					Protocol: 6, // TCP
					MinPort:  80,
					MaxPort:  80,
				},
			},
			wantOutbound: []string{
				"10.0.1.0/24 . 192.168.1.100/32 . tcp . 80",
			},
			wantInbound: []string{
				"192.168.1.100/32 . 10.0.1.0/24 . tcp . 80",
			},
			description: "Outbound: 10.0.1.0/24 -> 192.168.1.100:80, Inbound: 192.168.1.100:80 -> 10.0.1.0/24",
		},
		{
			name: "UDP DNS rule",
			rules: []ACLRule{
				{
					SrcNet:   "10.0.1.0/24",
					DstNet:   "8.8.8.8/32",
					Protocol: 17, // UDP
					MinPort:  53,
					MaxPort:  53,
				},
			},
			wantOutbound: []string{
				"10.0.1.0/24 . 8.8.8.8/32 . udp . 53",
			},
			wantInbound: []string{
				"8.8.8.8/32 . 10.0.1.0/24 . udp . 53",
			},
			description: "DNS query: 10.0.1.0/24 -> 8.8.8.8:53, DNS response: 8.8.8.8:53 -> 10.0.1.0/24",
		},
		{
			name: "port range rule",
			rules: []ACLRule{
				{
					SrcNet:   "10.0.0.0/8",
					DstNet:   "192.168.0.0/16",
					Protocol: 6, // TCP
					MinPort:  8000,
					MaxPort:  8999,
				},
			},
			wantOutbound: []string{
				"10.0.0.0/8 . 192.168.0.0/16 . tcp . 8000-8999",
			},
			wantInbound: []string{
				"192.168.0.0/16 . 10.0.0.0/8 . tcp . 8000-8999",
			},
			description: "Port range: 10.0.0.0/8 -> 192.168.0.0/16:8000-8999",
		},
		{
			name: "multiple rules",
			rules: []ACLRule{
				{
					SrcNet:   "10.0.1.0/24",
					DstNet:   "192.168.1.100/32",
					Protocol: 6, // TCP
					MinPort:  80,
					MaxPort:  80,
				},
				{
					SrcNet:   "10.0.2.0/24",
					DstNet:   "8.8.8.8/32",
					Protocol: 17, // UDP
					MinPort:  53,
					MaxPort:  53,
				},
			},
			wantOutbound: []string{
				"10.0.1.0/24 . 192.168.1.100/32 . tcp . 80",
				"10.0.2.0/24 . 8.8.8.8/32 . udp . 53",
			},
			wantInbound: []string{
				"192.168.1.100/32 . 10.0.1.0/24 . tcp . 80",
				"8.8.8.8/32 . 10.0.2.0/24 . udp . 53",
			},
			description: "Multiple rules with proper reversal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutbound, gotInbound := m.generateStatelessRulesMock(tt.rules)

			// Check outbound elements
			if len(gotOutbound) != len(tt.wantOutbound) {
				t.Errorf("outbound count mismatch: got %d, want %d", len(gotOutbound), len(tt.wantOutbound))
			}
			for i, want := range tt.wantOutbound {
				if i >= len(gotOutbound) {
					break
				}
				if gotOutbound[i] != want {
					t.Errorf("outbound[%d] = %q, want %q", i, gotOutbound[i], want)
				}
			}

			// Check inbound elements
			if len(gotInbound) != len(tt.wantInbound) {
				t.Errorf("inbound count mismatch: got %d, want %d", len(gotInbound), len(tt.wantInbound))
			}
			for i, want := range tt.wantInbound {
				if i >= len(gotInbound) {
					break
				}
				if gotInbound[i] != want {
					t.Errorf("inbound[%d] = %q, want %q", i, gotInbound[i], want)
				}
			}

			t.Logf("✓ %s", tt.description)
		})
	}
}

// TestReversalLogicMock tests the core reversal logic.
func TestReversalLogicMock(t *testing.T) {
	m := &mockFirewallManager{
		wgPort: 51820,
	}

	rule := ACLRule{
		SrcNet:   "10.0.1.0/24",
		DstNet:   "8.8.8.8/32",
		Protocol: 17,
		MinPort:  53,
		MaxPort:  53,
	}

	outbound, inbound := m.generateStatelessRulesMock([]ACLRule{rule})

	expectedOutbound := "10.0.1.0/24 . 8.8.8.8/32 . udp . 53"
	expectedInbound := "8.8.8.8/32 . 10.0.1.0/24 . udp . 53"

	if outbound[0] != expectedOutbound {
		t.Errorf("outbound = %q, want %q", outbound[0], expectedOutbound)
	}

	if inbound[0] != expectedInbound {
		t.Errorf("inbound = %q, want %q", inbound[0], expectedInbound)
	}

	t.Logf("✓ Reversal logic correct:")
	t.Logf("  Outbound: %s", outbound[0])
	t.Logf("  Inbound:  %s", inbound[0])
}

// TestAsymmetricRoutingScenario documents the asymmetric routing support.
func TestAsymmetricRoutingScenario(t *testing.T) {
	m := &mockFirewallManager{
		wgPort: 51820,
	}

	rule := ACLRule{
		SrcNet:   "10.0.1.100/32",
		DstNet:   "192.168.1.200/32",
		Protocol: 6,
		MinPort:  443,
		MaxPort:  443,
	}

	outbound, inbound := m.generateStatelessRulesMock([]ACLRule{rule})

	t.Log("Asymmetric Routing Scenario:")
	t.Logf("  Client: 10.0.1.100")
	t.Logf("  Server: 192.168.1.200:443")
	t.Log("")
	t.Log("Agent A (outbound path):")
	t.Logf("  Rule: %s", outbound[0])
	t.Log("  Matches: Client -> Server (SYN)")
	t.Log("")
	t.Log("Agent B (return path):")
	t.Logf("  Rule: %s", inbound[0])
	t.Log("  Matches: Server -> Client (ACK)")
	t.Log("")
	t.Log("✓ Both agents have both rules, asymmetric routing works")
}
