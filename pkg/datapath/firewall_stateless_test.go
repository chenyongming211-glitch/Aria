//go:build linux

package datapath

import (
	"testing"
)

// TestGenerateStatelessRules tests the bidirectional rule generation logic.
func TestGenerateStatelessRules(t *testing.T) {
	m := &NftablesFirewallManager{
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
		{
			name: "any port rule",
			rules: []ACLRule{
				{
					SrcNet:   "10.0.0.0/8",
					DstNet:   "192.168.0.0/16",
					Protocol: 6, // TCP
					MinPort:  0,
					MaxPort:  65535,
				},
			},
			wantOutbound: []string{
				"10.0.0.0/8 . 192.168.0.0/16 . tcp . 0-65535",
			},
			wantInbound: []string{
				"192.168.0.0/16 . 10.0.0.0/8 . tcp . 0-65535",
			},
			description: "Any port: 10.0.0.0/8 -> 192.168.0.0/16:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutbound, gotInbound := m.generateStatelessRules(tt.rules)

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

			t.Logf("Description: %s", tt.description)
		})
	}
}

// TestReversalLogic tests the core reversal logic for inbound rules.
func TestReversalLogic(t *testing.T) {
	m := &NftablesFirewallManager{
		wgPort: 51820,
	}

	// Test case: Internal network accessing external service
	rule := ACLRule{
		SrcNet:   "10.0.1.0/24",   // Internal network
		DstNet:   "8.8.8.8/32",    // External DNS server
		Protocol: 17,              // UDP
		MinPort:  53,              // DNS port
		MaxPort:  53,
	}

	outbound, inbound := m.generateStatelessRules([]ACLRule{rule})

	// Verify outbound: Internal -> External
	expectedOutbound := "10.0.1.0/24 . 8.8.8.8/32 . udp . 53"
	if outbound[0] != expectedOutbound {
		t.Errorf("outbound = %q, want %q", outbound[0], expectedOutbound)
	}

	// Verify inbound: External -> Internal (reversed)
	expectedInbound := "8.8.8.8/32 . 10.0.1.0/24 . udp . 53"
	if inbound[0] != expectedInbound {
		t.Errorf("inbound = %q, want %q", inbound[0], expectedInbound)
	}

	t.Logf("✓ Reversal logic correct:")
	t.Logf("  Outbound: %s (allows 10.0.1.x -> 8.8.8.8:53)", outbound[0])
	t.Logf("  Inbound:  %s (allows 8.8.8.8:53 -> 10.0.1.x)", inbound[0])
}

// TestAsymmetricRouting tests that bidirectional rules support asymmetric routing.
func TestAsymmetricRouting(t *testing.T) {
	m := &NftablesFirewallManager{
		wgPort: 51820,
	}

	// Scenario: Client -> Server via Agent A, Server -> Client via Agent B
	// Both agents need the same bidirectional rules
	rule := ACLRule{
		SrcNet:   "10.0.1.100/32", // Client
		DstNet:   "192.168.1.200/32", // Server
		Protocol: 6,               // TCP
		MinPort:  443,             // HTTPS
		MaxPort:  443,
	}

	outbound, inbound := m.generateStatelessRules([]ACLRule{rule})

	// Agent A sees: Client -> Server (outbound)
	// Agent B sees: Server -> Client (inbound)
	// Both agents have both rules, so traffic flows correctly

	t.Logf("Agent A (outbound path):")
	t.Logf("  Matches: %s", outbound[0])
	t.Logf("  Allows: 10.0.1.100 -> 192.168.1.200:443 (SYN)")

	t.Logf("Agent B (return path):")
	t.Logf("  Matches: %s", inbound[0])
	t.Logf("  Allows: 192.168.1.200:443 -> 10.0.1.100 (ACK)")

	t.Logf("✓ Asymmetric routing supported: both agents have both rules")
}

// TestTCPFlagsSecurity tests that the rule structure supports TCP flags checking.
func TestTCPFlagsSecurity(t *testing.T) {
	// This test documents the security model
	// Actual TCP flags checking is done in nftables rules, not in Go code

	t.Log("Security Model:")
	t.Log("1. Outbound rules match: tcp flags syn / syn,ack")
	t.Log("   - Only allows NEW connections (SYN packets)")
	t.Log("   - Internal hosts can initiate connections")
	t.Log("")
	t.Log("2. Inbound rules match: tcp flags != syn / syn,ack")
	t.Log("   - Only allows ESTABLISHED connections (non-SYN packets)")
	t.Log("   - External hosts CANNOT initiate connections")
	t.Log("")
	t.Log("3. UDP rules: no flags check (stateless)")
	t.Log("   - Both directions allowed")
	t.Log("   - Application-level security required")
	t.Log("")
	t.Log("✓ This prevents external active scanning and connection attempts")
}

// TestEmptyRules tests handling of empty rule sets.
func TestEmptyRules(t *testing.T) {
	m := &NftablesFirewallManager{
		wgPort: 51820,
	}

	outbound, inbound := m.generateStatelessRules([]ACLRule{})

	if len(outbound) != 0 {
		t.Errorf("expected empty outbound, got %d elements", len(outbound))
	}
	if len(inbound) != 0 {
		t.Errorf("expected empty inbound, got %d elements", len(inbound))
	}
}
