package datapath

import "time"

// InterfaceConfig contains WireGuard interface configuration.
type InterfaceConfig struct {
	// Name is the desired interface name (e.g., "aria0").
	// On macOS with userspace mode, the actual name may differ (e.g., "utun5").
	Name string

	// PrivateKey is the WireGuard private key (base64 encoded).
	PrivateKey string

	// ListenPort is the UDP port for WireGuard (default: 51820).
	ListenPort int

	// MTU is the interface MTU (default: 1280 for compatibility).
	MTU int

	// Address is the IP address to assign to the interface (e.g., "100.64.0.1/16").
	Address string

	// RuntimeMode specifies kernel or userspace mode.
	RuntimeMode RuntimeMode
}

// RuntimeMode specifies the WireGuard operation mode.
type RuntimeMode string

const (
	// ModeKernel uses the Linux kernel WireGuard module.
	ModeKernel RuntimeMode = "kernel"

	// ModeUserspace uses wireguard-go in userspace.
	ModeUserspace RuntimeMode = "userspace"
)

// PeerConfig contains WireGuard peer configuration.
type PeerConfig struct {
	// PublicKey is the peer's public key (base64 encoded).
	PublicKey string

	// Endpoint is the peer's public endpoint (e.g., "1.2.3.4:51820").
	// Can be empty for peers behind NAT.
	Endpoint string

	// AllowedIPs is the list of IP ranges this peer can send from.
	AllowedIPs []string

	// PersistentKeepalive is the keepalive interval in seconds (0 to disable).
	PersistentKeepalive int
}

// PeerInfo contains peer information and status.
type PeerInfo struct {
	// PublicKey is the peer's public key.
	PublicKey string

	// Endpoint is the last known endpoint.
	Endpoint string

	// AllowedIPs is the configured allowed IPs.
	AllowedIPs []string

	// LastHandshake is the time of the last successful handshake.
	LastHandshake time.Time

	// RxBytes is the number of bytes received from this peer.
	RxBytes uint64

	// TxBytes is the number of bytes sent to this peer.
	TxBytes uint64

	// PersistentKeepalive is the keepalive interval in seconds.
	PersistentKeepalive int
}

// TunnelStats contains interface-level statistics.
type TunnelStats struct {
	// InterfaceName is the actual interface name.
	InterfaceName string

	// RxBytes is total bytes received.
	RxBytes uint64

	// TxBytes is total bytes transmitted.
	TxBytes uint64

	// RxPackets is total packets received.
	RxPackets uint64

	// TxPackets is total packets transmitted.
	TxPackets uint64

	// PeerCount is the number of configured peers.
	PeerCount int
}

// RouteEntry represents a routing table entry.
type RouteEntry struct {
	// Destination is the destination CIDR (e.g., "10.0.0.0/8").
	Destination string

	// Gateway is the next-hop gateway IP (empty for direct routes).
	Gateway string

	// Interface is the outgoing interface name.
	Interface string

	// Metric is the route metric/priority.
	Metric int

	// Table is the routing table ID (0 for main table).
	Table int
}

// BGPConfig contains BGP daemon configuration.
type BGPConfig struct {
	// ASN is the local Autonomous System Number.
	ASN uint32

	// RouterID is the BGP router ID (typically an IPv4 address).
	RouterID string

	// ListenPort is the BGP listen port (default: 179).
	ListenPort int
}

// BGPNeighbor represents a BGP neighbor/peer.
type BGPNeighbor struct {
	// PeerIP is the neighbor's IP address.
	PeerIP string

	// PeerASN is the neighbor's AS number.
	PeerASN uint32

	// State is the BGP session state (e.g., "Established", "Idle").
	State string

	// PrefixesReceived is the number of prefixes received from this neighbor.
	PrefixesReceived int

	// PrefixesSent is the number of prefixes sent to this neighbor.
	PrefixesSent int
}

// FirewallPolicy represents a firewall rule/policy.
type FirewallPolicy struct {
	// ID is the unique policy identifier.
	ID string

	// Name is a human-readable policy name.
	Name string

	// Action is the policy action ("accept", "drop", "reject").
	Action string

	// SourceIPs is the list of source IP/CIDRs.
	SourceIPs []string

	// DestinationIPs is the list of destination IP/CIDRs.
	DestinationIPs []string

	// Ports is the list of destination ports.
	Ports []int

	// Protocol is the protocol ("tcp", "udp", "icmp", "any").
	Protocol string

	// Priority is the rule priority (lower = higher priority).
	Priority int
}

// SyncStatus represents HA synchronization status.
type SyncStatus struct {
	// Role is the current HA role ("active", "standby").
	Role string

	// PeerIP is the HA peer's IP address.
	PeerIP string

	// PeerState is the peer's state ("online", "offline", "unknown").
	PeerState string

	// LastSyncTime is the time of the last successful sync.
	LastSyncTime time.Time

	// SyncLag is the replication lag in milliseconds.
	SyncLag int
}

// ACLRule represents a firewall access control rule.
// Rules define: SrcNet -> DstNet:Protocol:Port whitelist.
// Uses nftables concatenated sets with interval flag for O(1) lookup.
type ACLRule struct {
	// SrcNet is the source network CIDR (e.g., "10.0.0.0/8").
	SrcNet string

	// DstNet is the destination network CIDR (e.g., "192.168.0.0/16").
	DstNet string

	// Protocol is the IP protocol number (6=TCP, 17=UDP, 0=any).
	Protocol uint8

	// MinPort is the minimum port in the range (inclusive).
	// Use 0 for "any port" or when protocol doesn't use ports.
	MinPort uint16

	// MaxPort is the maximum port in the range (inclusive).
	// Use 65535 for "any port" or single port (MinPort=MaxPort).
	MaxPort uint16
}

// FirewallStats contains firewall statistics.
type FirewallStats struct {
	// Enabled indicates if the firewall is currently active.
	Enabled bool

	// PacketsAccepted is the total number of packets accepted.
	PacketsAccepted uint64

	// PacketsDropped is the total number of packets dropped.
	PacketsDropped uint64

	// BytesAccepted is the total bytes accepted.
	BytesAccepted uint64

	// BytesDropped is the total bytes dropped.
	BytesDropped uint64

	// RuleCount is the number of active ACL rules.
	RuleCount int

	// LastUpdate is the time of the last policy update.
	LastUpdate time.Time

	// DroppedPackets is the total number of packets dropped (from drop_counter).
	DroppedPackets uint64

	// DroppedBytes is the total bytes dropped (from drop_counter).
	DroppedBytes uint64

	// InvalidPackets is the number of invalid packets (ct state invalid).
	InvalidPackets uint64

	// TCPBytes is the total TCP traffic bytes (from tcp_counter).
	TCPBytes uint64

	// UDPBytes is the total UDP traffic bytes (from udp_counter).
	UDPBytes uint64

	// ICMPBytes is the total ICMP traffic bytes (from icmp_counter).
	ICMPBytes uint64
}
