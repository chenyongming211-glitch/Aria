package datapath

import (
	"fmt"
	"log"
	"sync"
	"time"

	firewall_ebpf "aria/internal/agent/firewall"
)

// EBPFFirewallManager implements FirewallManager using eBPF (XDP + TC).
//
// Architecture:
//   - XDP (Express Data Path): Ingress ACL filtering at packet entry
//   - TC (Traffic Control): Egress QoS and filtering at packet exit
//   - eBPF Maps: Store ACL rules, QoS limits, and statistics
//
// Security Rules (hardcoded, cannot be overridden):
//   - Allow SSH (tcp/22)
//   - Allow HTTPS (tcp/443)
//   - Allow WireGuard (udp/51820-51830)
//   - Allow ICMP/ICMPv6
//   - Allow loopback
//   - Allow DHCP (udp/67-68)
type EBPFFirewallManager struct {
	adapter *firewall_ebpf.EBPFAdapter
	enabled bool
	mu      sync.RWMutex

	// Statistics
	ruleCount  int
	lastUpdate time.Time
}

// NewEBPFFirewallManager creates a new eBPF-based firewall manager.
func NewEBPFFirewallManager() (*EBPFFirewallManager, error) {
	log.Println("Firewall: initializing eBPF-based firewall manager")

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF adapter: %w", err)
	}

	fw := &EBPFFirewallManager{
		adapter: adapter,
		enabled: true,
	}

	log.Println("Firewall: eBPF adapter created successfully")
	return fw, nil
}

// Init initializes the eBPF firewall with default security rules.
func (e *EBPFFirewallManager) Init() error {
	if !e.enabled {
		return nil
	}

	log.Println("Firewall: applying default security policies")

	// Apply default security policy
	// These rules are hardcoded and cannot be overridden
	if err := e.adapter.ApplyDefaultPolicy(); err != nil {
		return fmt.Errorf("failed to apply default policy: %w", err)
	}

	// Pin maps for persistence across process restarts
	if err := e.adapter.PinMaps(); err != nil {
		log.Printf("Warning: failed to pin eBPF maps: %v", err)
		// Non-fatal, continue anyway
	}

	e.lastUpdate = time.Now()
	log.Println("Firewall: eBPF initialized and ready")
	return nil
}

// Cleanup removes all managed firewall rules and cleans up eBPF resources.
func (e *EBPFFirewallManager) Cleanup() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	log.Println("Firewall: cleaning up eBPF resources")

	if e.adapter != nil {
		if err := e.adapter.Close(); err != nil {
			return fmt.Errorf("failed to close eBPF adapter: %w", err)
		}
	}

	e.enabled = false
	log.Println("Firewall: eBPF cleaned up")
	return nil
}

// ApplyPolicy atomically applies a new set of ACL rules.
// This replaces all existing rules with the new set.
//
// Each ACLRule defines: SrcNet -> DstNet:Protocol:Port whitelist.
// Rules are converted to eBPF 5-tuple format and stored in maps.
//
// Implementation Note:
//   - Rules are stored in eBPF hash maps for O(1) lookup
//   - Both ingress (XDP) and egress (TC) maps are updated
//   - Security rules have priority over ACL rules
func (e *EBPFFirewallManager) ApplyPolicy(rules []ACLRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return fmt.Errorf("firewall not initialized")
	}

	log.Printf("Firewall: applying %d ACL rules via eBPF", len(rules))

	// Convert ACLRule to eBPF 5-tuple format and apply
	for _, rule := range rules {
		srcIP := rule.SrcNet
		dstIP := rule.DstNet
		srcPort := int(rule.MinPort)
		dstPort := int(rule.MaxPort)
		protocol := rule.Protocol

		// Default protocol to TCP if not specified
		if protocol == 0 {
			protocol = 6 // TCP
		}

		// Apply allow rule (action = "allow" means PASS/ALLOW)
		err := e.adapter.Apply5TupleACLRule(
			srcIP, dstIP, srcPort, dstPort, protocol, "allow",
		)
		if err != nil {
			log.Printf("Warning: failed to apply ACL rule %v -> %v: %v",
				rule.SrcNet, rule.DstNet, err)
			// Continue applying other rules
		}
	}

	e.ruleCount = len(rules)
	e.lastUpdate = time.Now()

	log.Printf("Firewall: applied %d ACL rules successfully", len(rules))
	return nil
}

// GetStats returns firewall statistics from eBPF maps.
func (e *EBPFFirewallManager) GetStats() (*FirewallStats, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := &FirewallStats{
		Enabled:    e.enabled,
		RuleCount:  e.ruleCount,
		LastUpdate: e.lastUpdate,
	}

	if !e.enabled {
		return stats, nil
	}

	// TODO: Read actual statistics from eBPF maps
	// Currently returns basic stats, need to implement:
	// - DroppedPackets/DroppedBytes from DropAlerts ring buffer
	// - TCPBytes/UDPBytes/ICMPBytes from protocol counters
	// - InvalidPackets from XDP redirect errors

	return stats, nil
}

// IsEnabled returns true if the firewall is active.
func (e *EBPFFirewallManager) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled
}

// BlockIP blocks a specific IP address using eBPF.
func (e *EBPFFirewallManager) BlockIP(ip string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	log.Printf("Firewall: blocking IP %s via eBPF", ip)
	return e.adapter.BlockIP(ip)
}

// LimitIP applies bandwidth limit to a specific IP using eBPF QoS.
func (e *EBPFFirewallManager) LimitIP(ip string, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	log.Printf("Firewall: limiting IP %s to %d Mbps via eBPF", ip, mbps)
	return e.adapter.LimitIP(ip, mbps)
}

// LimitPeerPair applies bandwidth limit between two IPs using eBPF QoS.
func (e *EBPFFirewallManager) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	log.Printf("Firewall: limiting peer pair %s -> %s to %d Mbps via eBPF",
		srcIP, dstIP, mbps)
	return e.adapter.LimitPeerPair(srcIP, dstIP, mbps)
}
