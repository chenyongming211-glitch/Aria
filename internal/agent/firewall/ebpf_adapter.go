package firewall

import (
	"fmt"
	"log"
	"sync"
	"time"

	"aria/internal/eBPF"
	"aria/pkg/config"
	controllerstorage "aria/pkg/controllerstorage"
)

// EBPFAdapter adapts the eBPF functionality to Aria's firewall interface
type EBPFAdapter struct {
	aclMgr        *eBPF.ACLManager
	qosMgr        *eBPF.QoSManager
	mu            sync.RWMutex
	snapshotMgr   *config.EbpfSnapshotManager
}

// NewEBPFAdapter creates a new eBPF adapter
func NewEBPFAdapter() (*EBPFAdapter, error) {
	return NewEBPFAdapterWithSnapshots("/var/lib/aria/ebpf_snapshot.json")
}

// NewEBPFAdapterWithSnapshots creates a new eBPF adapter with snapshot capability
func NewEBPFAdapterWithSnapshots(snapshotPath string) (*EBPFAdapter, error) {
	// Initialize snapshot manager
	snapshotMgr := config.NewEbpfSnapshotManager(snapshotPath)

	// Try recovery methods in order:
	// 1. Try to recover from pinned maps (survived process crash)
	adapter, err := NewEBPFAdapterFromPinned()
	if err == nil {
		log.Println("✅ Successfully recovered eBPF adapter from pinned maps")
		adapter.snapshotMgr = snapshotMgr
		return adapter, nil
	}

	log.Printf("⚠️ Pinned maps not available, creating fresh adapter: %v", err)

	// 2. Create fresh managers
	aclMgr, err := eBPF.NewACLManager()
	if err != nil {
		return nil, fmt.Errorf("creating ACL manager: %v", err)
	}

	qosMgr, err := eBPF.NewQoSManager()
	if err != nil {
		aclMgr.Close() // Clean up ACL manager if QoS fails
		return nil, fmt.Errorf("creating QoS manager: %v", err)
	}

	adapter = &EBPFAdapter{
		aclMgr:      aclMgr,
		qosMgr:      qosMgr,
		snapshotMgr: snapshotMgr,
	}

	// 3. Try to load from snapshot
	if snapshotMgr.Exists() {
		log.Println("🔧 Loading eBPF config from snapshot...")
		snapshot, err := snapshotMgr.Load()
		if err == nil {
			if err := applySnapshotToAdapter(adapter, snapshot); err != nil {
				log.Printf("Warning: failed to apply snapshot: %v", err)
			} else {
				log.Println("✅ Applied eBPF config from snapshot")
			}
		} else {
			log.Printf("Warning: failed to load snapshot: %v", err)
		}
	}

	return adapter, nil
}

// NewEBPFAdapterFromPinned creates an eBPF adapter from pinned maps
func NewEBPFAdapterFromPinned() (*EBPFAdapter, error) {
	aclMgr, err := eBPF.NewACLManagerFromPinned()
	if err != nil {
		return nil, fmt.Errorf("creating ACL manager from pinned: %v", err)
	}

	qosMgr, err := eBPF.NewQoSManagerFromPinned()
	if err != nil {
		aclMgr.Close() // Clean up ACL manager if QoS fails
		return nil, fmt.Errorf("creating QoS manager from pinned: %v", err)
	}

	return &EBPFAdapter{
		aclMgr: aclMgr,
		qosMgr: qosMgr,
	}, nil
}

// ApplyACLRule applies an access control rule
func (e *EBPFAdapter) ApplyACLRule(cidr string, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Parse CIDR to get IP (simplified - in production you'd parse properly)
	// For CIDR rules, we use BlockIP as the underlying implementation
	switch action {
	case "block":
		return e.aclMgr.BlockIP(cidr)
	case "allow":
		// Allow is the default behavior in eBPF, no explicit rule needed
		return nil
	default:
		return fmt.Errorf("unknown ACL action: %s", action)
	}
}

// Apply5TupleACLRule applies a 5-tuple access control rule
func (e *EBPFAdapter) Apply5TupleACLRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Convert action to the numeric value expected by the ACL manager
	var actionValue uint32
	if action == "allow" || action == "pass" {
		actionValue = 1
	} else if action == "block" || action == "drop" {
		actionValue = 0
	} else {
		return fmt.Errorf("unknown ACL action: %s", action)
	}

	// Apply the 5-tuple rule to the ACL manager
	return e.aclMgr.Apply5TupleACLRule(srcIP, dstIP, srcPort, dstPort, protocol, actionValue)
}

// RemoveACLRule removes an access control rule
func (e *EBPFAdapter) RemoveACLRule(cidr string) error {
	// Note: ACLManager's RemoveRule requires 5-tuple parameters
	// CIDR-based rules are removed by re-attaching with default policy
	// This is a simplified implementation
	return fmt.Errorf("CIDR rule removal not yet implemented")
}

// Remove5TupleACLRule removes a 5-tuple access control rule
func (e *EBPFAdapter) Remove5TupleACLRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove the 5-tuple rule from the ACL manager
	return e.aclMgr.RemoveRule(srcIP, dstIP, srcPort, dstPort, protocol)
}

// ApplyQoSRule applies a quality of service rule
func (e *EBPFAdapter) ApplyQoSRule(ruleType string, params map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch ruleType {
	case "ip_limit":
		ip, ok := params["ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid IP in params")
		}

		mbps, ok := params["mbps"].(float64) // JSON unmarshals numbers as float64
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		return e.qosMgr.LimitIP(ip, int(mbps))

	case "peer_limit":
		srcIP, ok := params["src_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid source IP in params")
		}

		dstIP, ok := params["dst_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid destination IP in params")
		}

		mbps, ok := params["mbps"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		return e.qosMgr.LimitPeerPair(srcIP, dstIP, int(mbps))

	case "port_limit":
		port, ok := params["port"].(float64) // JSON unmarshals numbers as float64
		if !ok {
			return fmt.Errorf("missing or invalid port in params")
		}

		mbps, ok := params["mbps"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		return e.qosMgr.LimitPort(int(port), int(mbps))

	case "service_limit":
		srcIP, ok := params["src_ip"].(string)
		if !ok {
			srcIP = "" // Optional parameter
		}

		dstIP, ok := params["dst_ip"].(string)
		if !ok {
			dstIP = "" // Optional parameter
		}

		srcPort, ok := params["src_port"].(float64)
		if !ok {
			srcPort = 0 // Optional parameter
		}

		dstPort, ok := params["dst_port"].(float64)
		if !ok {
			dstPort = 0 // Optional parameter
		}

		protocol, ok := params["protocol"].(float64)
		if !ok {
			protocol = 6 // Default to TCP
		}

		mbps, ok := params["mbps"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		return e.qosMgr.LimitService(srcIP, dstIP, int(srcPort), int(dstPort), int(protocol), int(mbps))

	default:
		return fmt.Errorf("unknown QoS rule type: %s", ruleType)
	}
}

// RemoveQoSRule removes a quality of service rule
func (e *EBPFAdapter) RemoveQoSRule(ruleType string, params map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch ruleType {
	case "ip_limit":
		ip, ok := params["ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid IP in params")
		}
		return e.qosMgr.RemoveIPLimit(ip)

	case "peer_limit":
		srcIP, ok := params["src_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid source IP in params")
		}

		dstIP, ok := params["dst_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid destination IP in params")
		}

		return e.qosMgr.RemovePeerLimit(srcIP, dstIP)

	case "service_limit":
		srcIP, ok := params["src_ip"].(string)
		if !ok {
			srcIP = "" // Optional parameter
		}

		dstIP, ok := params["dst_ip"].(string)
		if !ok {
			dstIP = "" // Optional parameter
		}

		srcPort, ok := params["src_port"].(float64)
		if !ok {
			srcPort = 0 // Optional parameter
		}

		dstPort, ok := params["dst_port"].(float64)
		if !ok {
			dstPort = 0 // Optional parameter
		}

		protocol, ok := params["protocol"].(float64)
		if !ok {
			protocol = 6 // Default to TCP
		}

		return e.qosMgr.RemoveServiceLimit(srcIP, dstIP, int(srcPort), int(dstPort), int(protocol))

	default:
		return fmt.Errorf("unknown QoS rule type: %s", ruleType)
	}
}

// UpdateNetworkInterfaces updates the interfaces that the eBPF programs are attached to
func (e *EBPFAdapter) UpdateNetworkInterfaces(interfaces []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update QoS interfaces
	if err := e.qosMgr.UpdateInterfaces(interfaces); err != nil {
		return fmt.Errorf("updating QoS interfaces: %v", err)
	}

	// TODO: Add XDP ACL interface updates when the XDP attachment logic is implemented

	return nil
}

// BlockIP implements the interface to block a specific IP
func (e *EBPFAdapter) BlockIP(ip string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.aclMgr.BlockIP(ip)
}

// LimitIP implements the interface to limit bandwidth for a specific IP
func (e *EBPFAdapter) LimitIP(ip string, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.qosMgr.LimitIP(ip, mbps)
}

// LimitPeerPair implements the interface to limit bandwidth between two IPs
func (e *EBPFAdapter) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.qosMgr.LimitPeerPair(srcIP, dstIP, mbps)
}

// LimitPort implements the interface to limit bandwidth for a specific port
func (e *EBPFAdapter) LimitPort(port int, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.qosMgr.LimitPort(port, mbps)
}

// LimitService implements the interface to limit bandwidth for a five-tuple (srcIP, dstIP, srcPort, dstPort, protocol)
func (e *EBPFAdapter) LimitService(srcIP, dstIP string, srcPort, dstPort, protocol int, mbps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.qosMgr.LimitService(srcIP, dstIP, srcPort, dstPort, protocol, mbps)
}

// LimitPortForIP implements the interface to limit bandwidth for a specific port from/to an IP
func (e *EBPFAdapter) LimitPortForIP(ip string, port int, mbps int, direction string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.qosMgr.LimitPortForIP(ip, port, mbps, direction)
}

// GetIPStats retrieves statistics for a specific IP
func (e *EBPFAdapter) GetIPStats(ip string) (map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats, err := e.qosMgr.GetIPStats(ip)
	if err != nil {
		return nil, fmt.Errorf("getting IP stats: %v", err)
	}

	return map[string]interface{}{
		"rate_bytes_per_sec": stats.RateBytesPerSec,
		"burst_bytes":        stats.BurstBytes,
		"current_tokens":     stats.Tokens,
		"last_update_ns":     stats.LastUpdateNS,
		"pass_bytes":         stats.PassBytes,
		"drop_bytes":         stats.DropBytes,
		"rule_id":            stats.RuleID,
	}, nil
}

// GetPeerStats retrieves statistics for a peer pair
func (e *EBPFAdapter) GetPeerStats(srcIP, dstIP string) (map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats, err := e.qosMgr.GetPeerStats(srcIP, dstIP)
	if err != nil {
		return nil, fmt.Errorf("getting peer stats: %v", err)
	}

	return map[string]interface{}{
		"rate_bytes_per_sec": stats.RateBytesPerSec,
		"burst_bytes":        stats.BurstBytes,
		"current_tokens":     stats.Tokens,
		"last_update_ns":     stats.LastUpdateNS,
		"pass_bytes":         stats.PassBytes,
		"drop_bytes":         stats.DropBytes,
		"rule_id":            stats.RuleID,
	}, nil
}

// GetServiceStats retrieves statistics for a service (five-tuple)
func (e *EBPFAdapter) GetServiceStats(srcIP, dstIP string, srcPort, dstPort, protocol int) (map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats, err := e.qosMgr.GetServiceStats(srcIP, dstIP, srcPort, dstPort, protocol)
	if err != nil {
		return nil, fmt.Errorf("getting service stats: %v", err)
	}

	return map[string]interface{}{
		"rate_bytes_per_sec": stats.RateBytesPerSec,
		"burst_bytes":        stats.BurstBytes,
		"current_tokens":     stats.Tokens,
		"last_update_ns":     stats.LastUpdateNS,
		"pass_bytes":         stats.PassBytes,
		"drop_bytes":         stats.DropBytes,
		"rule_id":            stats.RuleID,
	}, nil
}

// Close cleans up the eBPF adapter resources
func (e *EBPFAdapter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Save current state before closing
	if e.snapshotMgr != nil {
		e.saveCurrentState()
	}

	// Unpin maps to clean up filesystem references
	if e.aclMgr != nil {
		e.aclMgr.Unpin()
	}
	if e.qosMgr != nil {
		e.qosMgr.Unpin()
	}

	errs := make([]error, 0)

	if e.aclMgr != nil {
		if err := e.aclMgr.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if e.qosMgr != nil {
		if err := e.qosMgr.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing eBPF adapter: %v", errs)
	}

	return nil
}

// saveCurrentState saves the current eBPF configuration to snapshot
func (e *EBPFAdapter) saveCurrentState() {
	// Create a snapshot of the current state
	snapshot := &config.EbpfSnapshot{
		Version: time.Now().Unix(),
		// Note: In a real implementation, we would iterate through the eBPF maps
		// to reconstruct the rules. For now, we're using empty slices as placeholders.
		ACLRules:   []config.ACLRuleSnapshot{},
		QoSRules:   []config.QoSRuleSnapshot{},
		Interfaces: []string{}, // Would come from stored interface list
	}

	if e.snapshotMgr != nil {
		if err := e.snapshotMgr.Save(snapshot); err != nil {
			log.Printf("Warning: failed to save eBPF snapshot: %v", err)
		} else {
			log.Println("✅ eBPF configuration saved to snapshot")
		}
	}
}

// ApplyDefaultPolicy applies the default security policy
func (e *EBPFAdapter) ApplyDefaultPolicy() error {
	log.Println("⚠️ Applying default security policy...")

	e.mu.Lock()
	defer e.mu.Unlock()

	// Apply default security policy
	// 1. Allow essential connections
	defaultACLs := []struct {
		cidr   string
		action string
	}{
		{"127.0.0.1/32", "allow"},    // Localhost
		{"10.0.0.0/8", "allow"},      // Private networks
		{"172.16.0.0/12", "allow"},
		{"192.168.0.0/16", "allow"},
		{"100.64.0.0/10", "allow"},   // RFC 6598 CGNAT
	}

	for _, rule := range defaultACLs {
		// Try to apply rule, ignore errors for default policies
		if err := e.ApplyACLRule(rule.cidr, rule.action); err != nil {
			log.Printf("Warning: Failed to apply default policy %s: %v", rule.cidr, err)
		}
	}

	log.Println("✅ Default security policy applied")
	return nil
}

// SyncACLRules performs a declarative synchronization of ACL rules
// It compares the desired state with the current state in eBPF maps and makes minimal updates
func (e *EBPFAdapter) SyncACLRules(desiredRules []*controllerstorage.ACLRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.aclMgr.SyncACLRules(desiredRules)
}

// PinMaps pins the eBPF maps to filesystem to survive process crashes
func (e *EBPFAdapter) PinMaps() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.aclMgr != nil {
		if err := e.aclMgr.Pin(); err != nil {
			log.Printf("Warning: Failed to pin ACL map: %v", err)
		} else {
			log.Println("✅ ACL map pinned successfully")
		}
	}

	if e.qosMgr != nil {
		if err := e.qosMgr.Pin(); err != nil {
			log.Printf("Warning: Failed to pin QoS maps: %v", err)
		} else {
			log.Println("✅ QoS maps pinned successfully")
		}
	}

	return nil
}

// applySnapshotToAdapter applies the configuration from a snapshot to the adapter
func applySnapshotToAdapter(adapter *EBPFAdapter, snapshot *config.EbpfSnapshot) error {
	// Apply ACL rules
	for _, rule := range snapshot.ACLRules {
		if err := adapter.ApplyACLRule(rule.CIDR, rule.Action); err != nil {
			log.Printf("Warning: failed to apply ACL rule %s: %v", rule.CIDR, err)
		}
	}

	// Apply QoS rules
	for _, rule := range snapshot.QoSRules {
		params := map[string]interface{}{
			"mbps": float64(rule.Bandwidth),
		}

		switch rule.Type {
		case "ip":
			params["ip"] = rule.SourceIP
			if err := adapter.ApplyQoSRule("ip_limit", params); err != nil {
				log.Printf("Warning: failed to apply IP limit rule: %v", err)
			}
		case "peer":
			params["src_ip"] = rule.SourceIP
			params["dst_ip"] = rule.DestIP
			if err := adapter.ApplyQoSRule("peer_limit", params); err != nil {
				log.Printf("Warning: failed to apply peer limit rule: %v", err)
			}
		case "service":
			params["src_ip"] = rule.SourceIP
			params["dst_ip"] = rule.DestIP
			params["src_port"] = float64(rule.SourcePort)
			params["dst_port"] = float64(rule.DestPort)
			params["protocol"] = float64(rule.Protocol)
			if err := adapter.ApplyQoSRule("service_limit", params); err != nil {
				log.Printf("Warning: failed to apply service limit rule: %v", err)
			}
		case "port":
			params["port"] = float64(rule.DestPort)
			if err := adapter.ApplyQoSRule("port_limit", params); err != nil {
				log.Printf("Warning: failed to apply port limit rule: %v", err)
			}
		}
	}

	// Apply network interfaces
	if len(snapshot.Interfaces) > 0 {
		if err := adapter.UpdateNetworkInterfaces(snapshot.Interfaces); err != nil {
			log.Printf("Warning: failed to update network interfaces: %v", err)
		}
	}

	return nil
}