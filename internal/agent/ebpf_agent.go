package agent

import (
	"context"
	"fmt"
	"log"

	firewall "aria/internal/agent/firewall"
)

// EBPFAgent wraps the eBPF functionality for use in Aria's agent
type EBPFAgent struct {
	adapter *firewall.EBPFAdapter
}

// NewEBPFAgent creates a new eBPF-enabled agent
func NewEBPFAgent() (*EBPFAgent, error) {
	adapter, err := firewall.NewEBPFAdapter()
	if err != nil {
		return nil, fmt.Errorf("creating eBPF adapter: %v", err)
	}

	return &EBPFAgent{
		adapter: adapter,
	}, nil
}

// Start initializes the eBPF programs
func (e *EBPFAgent) Start(ctx context.Context) error {
	log.Println("Starting eBPF firewall and QoS engine...")

	// Load default policies
	if err := e.adapter.ApplyDefaultPolicy(); err != nil {
		log.Printf("Warning: Failed to load default policies: %v", err)
	}

	// Update network interfaces - in a real implementation, this would come from agent configuration
	interfaces := []string{"aria0"} // Default Aria interface
	if err := e.adapter.UpdateNetworkInterfaces(interfaces); err != nil {
		return fmt.Errorf("updating network interfaces: %v", err)
	}

	log.Println("eBPF firewall and QoS engine started successfully")
	return nil
}

// Stop cleans up the eBPF programs
func (e *EBPFAgent) Stop() error {
	log.Println("Stopping eBPF firewall and QoS engine...")
	return e.adapter.Close()
}

// ApplyACLRules applies access control rules received from the controller
func (e *EBPFAgent) ApplyACLRules(rules []ACLRule) error {
	for _, rule := range rules {
		if err := e.adapter.ApplyACLRule(rule.CIDR, rule.Action); err != nil {
			log.Printf("Failed to apply ACL rule for %s: %v", rule.CIDR, err)
			// Continue processing other rules even if one fails
		} else {
			log.Printf("Applied ACL rule: %s -> %s", rule.CIDR, rule.Action)
		}
	}
	return nil
}

// ApplyQoSRules applies quality of service rules received from the controller
func (e *EBPFAgent) ApplyQoSRules(rules []QoSRule) error {
	for _, rule := range rules {
		params := map[string]interface{}{
			"mbps": float64(rule.Mbps),
		}

		var ruleType string
		switch rule.Type {
		case "ip":
			ruleType = "ip_limit"
			params["ip"] = rule.Target
		case "peer":
			ruleType = "peer_limit"
			peers := rule.Target.(map[string]string) // Expecting map with "src_ip" and "dst_ip"
			params["src_ip"] = peers["src_ip"]
			params["dst_ip"] = peers["dst_ip"]
		default:
			log.Printf("Unknown QoS rule type: %s", rule.Type)
			continue
		}

		if err := e.adapter.ApplyQoSRule(ruleType, params); err != nil {
			log.Printf("Failed to apply QoS rule: %v", err)
		} else {
			log.Printf("Applied QoS rule: %s for %s at %d Mbps", ruleType, rule.Target, rule.Mbps)
		}
	}
	return nil
}

// ACLRule represents an access control rule
type ACLRule struct {
	CIDR   string // CIDR notation, e.g., "192.168.1.0/24"
	Action string // "allow", "block", etc.
}

// QoSRule represents a quality of service rule
type QoSRule struct {
	Type   string      // "ip" or "peer"
	Target interface{} // IP string for "ip" type, map for "peer" type
	Mbps   int         // Bandwidth limit in Mbps
}

// BlockIP blocks a specific IP address
func (e *EBPFAgent) BlockIP(ip string) error {
	return e.adapter.BlockIP(ip)
}

// LimitIP limits bandwidth for a specific IP address
func (e *EBPFAgent) LimitIP(ip string, mbps int) error {
	return e.adapter.LimitIP(ip, mbps)
}

// LimitPeerPair limits bandwidth between two IP addresses
func (e *EBPFAgent) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	return e.adapter.LimitPeerPair(srcIP, dstIP, mbps)
}

// GetIPStats retrieves statistics for a specific IP
func (e *EBPFAgent) GetIPStats(ip string) (map[string]interface{}, error) {
	return e.adapter.GetIPStats(ip)
}

// GetPeerStats retrieves statistics for a peer pair
func (e *EBPFAgent) GetPeerStats(srcIP, dstIP string) (map[string]interface{}, error) {
	return e.adapter.GetPeerStats(srcIP, dstIP)
}
