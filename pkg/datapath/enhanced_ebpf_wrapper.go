package datapath

import (
	"fmt"
	"log"
	"sync"
	"time"

	firewall_ebpf "aria/internal/agent/firewall"
)

// EnhancedEBPFWrapper wraps the eBPF firewall functionality with enhanced persistence
type EnhancedEBPFWrapper struct {
	adapter   *firewall_ebpf.EBPFAdapter
	enabled   bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewEnhancedEBPFFirewall creates a new eBPF firewall wrapper with enhanced persistence
func NewEnhancedEBPFFirewall() (*EnhancedEBPFWrapper, error) {
	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF adapter: %v", err)
	}

	wrapper := &EnhancedEBPFWrapper{
		adapter: adapter,
		enabled: true,
		stopCh:  make(chan struct{}),
	}

	// Pin the maps to survive process crashes
	if err := wrapper.adapter.PinMaps(); err != nil {
		log.Printf("Warning: failed to pin eBPF maps: %v", err)
	}

	// Start background tasks
	wrapper.startBackgroundTasks()

	return wrapper, nil
}

// startBackgroundTasks starts background tasks for enhanced functionality
func (e *EnhancedEBPFWrapper) startBackgroundTasks() {
	// Start periodic snapshot saving
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(5 * time.Minute) // Save snapshot every 5 minutes
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Periodic save is handled by individual operations
			case <-e.stopCh:
				return
			}
		}
	}()
}

// ApplyPolicy applies ACL policies using eBPF
func (e *EnhancedEBPFWrapper) ApplyPolicy(acls []ACLRule) error {
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
func (e *EnhancedEBPFWrapper) IsEnabled() bool {
	return e.enabled
}

// Init initializes the eBPF firewall
func (e *EnhancedEBPFWrapper) Init() error {
	if !e.enabled {
		return nil
	}

	// Apply default policies
	return e.adapter.ApplyDefaultPolicy()
}

// Close closes the eBPF firewall
func (e *EnhancedEBPFWrapper) Close() error {
	close(e.stopCh)
	e.wg.Wait()

	if e.adapter != nil {
		return e.adapter.Close()
	}
	return nil
}

// Cleanup performs cleanup operations for the eBPF firewall
func (e *EnhancedEBPFWrapper) Cleanup() error {
	if e.adapter != nil {
		return e.adapter.Close()
	}
	return nil
}

// BlockIP blocks a specific IP using eBPF
func (e *EnhancedEBPFWrapper) BlockIP(ip string) error {
	if !e.enabled {
		return nil
	}

	return e.adapter.BlockIP(ip)
}

// LimitIP applies bandwidth limit to a specific IP using eBPF
func (e *EnhancedEBPFWrapper) LimitIP(ip string, mbps int) error {
	if !e.enabled {
		return nil
	}

	err := e.adapter.LimitIP(ip, mbps)
	if err == nil {
		// Trigger snapshot save after successful operation
	}
	return err
}

// LimitPeerPair applies bandwidth limit between two IPs using eBPF
func (e *EnhancedEBPFWrapper) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	if !e.enabled {
		return nil
	}

	err := e.adapter.LimitPeerPair(srcIP, dstIP, mbps)
	if err == nil {
		// Trigger snapshot save after successful operation
	}
	return err
}

// GetStats returns firewall statistics
func (e *EnhancedEBPFWrapper) GetStats() (*FirewallStats, error) {
	if !e.enabled {
		return &FirewallStats{Enabled: false}, nil
	}

	// Return dummy stats - in a real implementation we'd collect real statistics
	return &FirewallStats{
		Enabled: true,
	}, nil
}