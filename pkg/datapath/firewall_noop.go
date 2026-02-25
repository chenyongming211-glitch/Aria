package datapath

// NoopFirewallManager is a placeholder implementation of FirewallManager.
// Used on platforms where nftables is not available (macOS, Windows).
type NoopFirewallManager struct{}

// NewNoopFirewallManager creates a new no-op firewall manager.
func NewNoopFirewallManager() *NoopFirewallManager {
	return &NoopFirewallManager{}
}

// Init initializes firewall tables, chains, and default safety rules.
// No-op implementation always succeeds.
func (m *NoopFirewallManager) Init() error {
	return nil
}

// Cleanup removes all managed firewall rules and tables.
// No-op implementation always succeeds.
func (m *NoopFirewallManager) Cleanup() error {
	return nil
}

// ApplyPolicy atomically applies a new set of ACL rules.
// No-op implementation always succeeds (rules are ignored).
func (m *NoopFirewallManager) ApplyPolicy(rules []ACLRule) error {
	return nil
}

// GetStats returns firewall statistics.
// No-op implementation returns empty stats with Enabled=false.
func (m *NoopFirewallManager) GetStats() (*FirewallStats, error) {
	return &FirewallStats{
		Enabled: false,
	}, nil
}

// IsEnabled returns true if the firewall is active.
// No-op implementation always returns false.
func (m *NoopFirewallManager) IsEnabled() bool {
	return false
}

// BlockIP blocks a specific IP address.
// No-op implementation always succeeds.
func (m *NoopFirewallManager) BlockIP(ip string) error {
	return nil
}

// LimitIP applies bandwidth limit to a specific IP.
// No-op implementation always succeeds.
func (m *NoopFirewallManager) LimitIP(ip string, mbps int) error {
	return nil
}

// LimitPeerPair applies bandwidth limit between two IPs.
// No-op implementation always succeeds.
func (m *NoopFirewallManager) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	return nil
}
