//go:build !linux

package datapath

// NftablesOption is a no-op on non-Linux platforms.
type NftablesOption func(*NoopFirewallManager)

// WithWireGuardPort is a no-op on non-Linux platforms.
func WithWireGuardPort(port int) NftablesOption {
	return func(*NoopFirewallManager) {}
}

// WithWireGuardPortRange is a no-op on non-Linux platforms.
func WithWireGuardPortRange(minPort, maxPort int) NftablesOption {
	return func(*NoopFirewallManager) {}
}

// NewNftablesFirewallManager returns a no-op firewall manager on non-Linux platforms.
func NewNftablesFirewallManager(opts ...NftablesOption) FirewallManager {
	return NewNoopFirewallManager()
}
