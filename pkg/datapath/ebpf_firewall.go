package datapath

import (
	"fmt"

	firewall_ebpf "aria/internal/agent/firewall"
)

// EBPFWrapper wraps the eBPF firewall functionality to implement the Firewall interface
type EBPFWrapper struct {
	adapter *firewall_ebpf.EBPFAdapter
	enabled bool
}

// NewEBPFFirewall creates a new eBPF firewall wrapper
func NewEBPFFirewall() (*EBPFWrapper, error) {
	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF adapter: %v", err)
	}

	return &EBPFWrapper{
		adapter: adapter,
		enabled: true,
	}, nil
}

// ApplyPolicy applies ACL policies using eBPF
func (e *EBPFWrapper) ApplyPolicy(acls []ACLRule) error {
	if !e.enabled {
		return nil
	}

	// Convert ACLRule to eBPF compatible format and apply
	for _, acl := range acls {
		// Apply using eBPF ACL manager
		cidr := acl.SrcNet
		if cidr == "" {
			cidr = acl.DstNet
		}
		if cidr != "" {
			// Apply allow rule for now - in a real implementation we'd have more sophisticated mapping
			e.adapter.ApplyACLRule(cidr, "allow")
		}
	}

	return nil
}

// IsEnabled returns whether the firewall is enabled
func (e *EBPFWrapper) IsEnabled() bool {
	return e.enabled
}

// Init initializes the eBPF firewall
func (e *EBPFWrapper) Init() error {
	if !e.enabled {
		return nil
	}

	// Apply default policies
	return e.adapter.ApplyDefaultPolicy()
}

// Close closes the eBPF firewall
func (e *EBPFWrapper) Close() error {
	if e.adapter != nil {
		return e.adapter.Close()
	}
	return nil
}

// Cleanup performs cleanup operations for the eBPF firewall
func (e *EBPFWrapper) Cleanup() error {
	if e.adapter != nil {
		return e.adapter.Close()
	}
	return nil
}

// BlockIP blocks a specific IP using eBPF
func (e *EBPFWrapper) BlockIP(ip string) error {
	if !e.enabled {
		return nil
	}

	return e.adapter.BlockIP(ip)
}

// LimitIP applies bandwidth limit to a specific IP using eBPF
func (e *EBPFWrapper) LimitIP(ip string, mbps int) error {
	if !e.enabled {
		return nil
	}

	return e.adapter.LimitIP(ip, mbps)
}

// LimitPeerPair applies bandwidth limit between two IPs using eBPF
func (e *EBPFWrapper) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	if !e.enabled {
		return nil
	}

	return e.adapter.LimitPeerPair(srcIP, dstIP, mbps)
}

// GetStats returns firewall statistics
func (e *EBPFWrapper) GetStats() (*FirewallStats, error) {
	if !e.enabled {
		return &FirewallStats{Enabled: false}, nil
	}

	// Return dummy stats - in a real implementation we'd collect real statistics
	return &FirewallStats{
		Enabled: true,
	}, nil
}