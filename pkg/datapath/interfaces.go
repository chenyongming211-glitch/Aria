// Package datapath provides a unified interface layer for network data plane operations.
// It abstracts WireGuard tunnel management, system routing, BGP, firewall, and HA operations.
package datapath

// TunnelManager manages WireGuard tunnel interfaces and peers.
type TunnelManager interface {
	// EnsureInterface creates or updates the WireGuard interface.
	// This operation is idempotent - safe to call multiple times.
	EnsureInterface(cfg *InterfaceConfig) error

	// DeleteInterface removes the WireGuard interface.
	DeleteInterface() error

	// SetIPAddress sets the IP address on the interface.
	SetIPAddress(ip string) error

	// AddPeer adds a new peer to the WireGuard interface.
	AddPeer(peer *PeerConfig) error

	// RemovePeer removes a peer by its public key.
	RemovePeer(publicKey string) error

	// UpdatePeer updates an existing peer's configuration.
	UpdatePeer(peer *PeerConfig) error

	// ListPeers returns all configured peers.
	ListPeers() ([]*PeerInfo, error)

	// GetStats returns interface statistics.
	GetStats() (*TunnelStats, error)

	// GetInterfaceName returns the actual interface name (may differ from requested on macOS).
	GetInterfaceName() string

	// GetPublicKey returns the interface's public key.
	GetPublicKey() string
}

// RouteManager manages system routing tables.
type RouteManager interface {
	// Init initializes the routing engine (creates tables, rules).
	Init() error

	// Cleanup removes all managed routes and rules.
	Cleanup() error

	// AddRoute adds a route to the routing table.
	AddRoute(route *RouteEntry) error

	// RemoveRoute removes a route by destination.
	RemoveRoute(destination string) error

	// ListRoutes returns all routes on the managed interface.
	ListRoutes() ([]*RouteEntry, error)

	// AddVPNRoute adds a route to the VPN routing table.
	AddVPNRoute(destination string) error

	// RemoveVPNRoute removes a route from the VPN routing table.
	RemoveVPNRoute(destination string) error

	// AddECMPRoute adds an ECMP (Equal-Cost Multi-Path) route.
	AddECMPRoute(destination string, interfaces []string) error

	// RemoveECMPRoute removes an ECMP route.
	RemoveECMPRoute(destination string) error
}

// BGPManager manages BGP dynamic routing (reserved for future implementation).
type BGPManager interface {
	// Start initializes and starts the BGP daemon.
	Start(cfg *BGPConfig) error

	// Stop gracefully stops the BGP daemon.
	Stop() error

	// AddNeighbor adds a BGP neighbor/peer.
	AddNeighbor(neighbor *BGPNeighbor) error

	// RemoveNeighbor removes a BGP neighbor by peer IP.
	RemoveNeighbor(peerIP string) error

	// AdvertisePrefix announces a prefix to BGP neighbors.
	AdvertisePrefix(prefix string) error

	// WithdrawPrefix withdraws a previously announced prefix.
	WithdrawPrefix(prefix string) error

	// GetNeighbors returns all BGP neighbors and their status.
	GetNeighbors() ([]*BGPNeighbor, error)
}

// FirewallManager manages firewall rules and ACL policies using eBPF.
// Implementation: XDP for ingress filtering, TC for egress QoS.
type FirewallManager interface {
	// Init initializes firewall resources and applies default security rules.
	// Hardcodes: SSH, WireGuard, ICMP, loopback, DHCP allow rules (failsafe).
	Init() error

	// Cleanup removes all managed firewall rules and resources.
	Cleanup() error

	// ApplyPolicy atomically applies a new set of ACL rules.
	// This replaces all existing rules with the new set.
	// Rules define: SrcNet -> DstNet:Protocol:Port whitelist.
	ApplyPolicy(rules []ACLRule) error

	// GetStats returns firewall statistics (packets accepted/dropped).
	GetStats() (*FirewallStats, error)

	// IsEnabled returns true if the firewall is active.
	IsEnabled() bool

	// BlockIP blocks traffic from/to a specific IP address.
	BlockIP(ip string) error

	// LimitIP applies bandwidth limit to a specific IP (eBPF QoS).
	LimitIP(ip string, mbps int) error

	// LimitPeerPair applies bandwidth limit between two IPs (eBPF QoS).
	LimitPeerPair(srcIP, dstIP string, mbps int) error
}

// HAManager manages high availability state synchronization (reserved for future implementation).
type HAManager interface {
	// SetSyncPeer sets the peer for state synchronization.
	SetSyncPeer(peerIP string, port int) error

	// GetSyncStatus returns the current sync status.
	GetSyncStatus() (*SyncStatus, error)

	// IsActive returns true if this node is the active node.
	IsActive() bool

	// TriggerFailover initiates a failover to the standby node.
	TriggerFailover() error
}
