package eBPF

import (
	"github.com/cilium/ebpf"
)

// CombinedBPFMaps contains all eBPF maps for both ACL and QoS with the new specifications
type CombinedBPFMaps struct {
	*AriaBPFMaps  // Embed the new maps structure
	*OldBPFMaps   // Keep backward compatibility
}

// OldBPFMaps contains the original maps for backward compatibility
type OldBPFMaps struct {
	// ACL Maps (XDP Layer)
	ACLMap *ebpf.Map // LPM Trie for CIDR-based access control

	// QoS Maps (TC Layer)
	IPRateMap      *ebpf.Map // Per-IP rate limiting
	PeerRateMap    *ebpf.Map // Peer-to-peer rate limiting
	ServiceRateMap *ebpf.Map // Service-level (port-based) rate limiting
}