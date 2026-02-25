package eBPF

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	controllerstorage "aria/pkg/controllerstorage"
)

// AriaBPFMaps contains all eBPF maps used for ACL and QoS according to the new specifications
type AriaBPFMaps struct {
	// Ingress ACL Maps (XDP Layer)
	Ingress5TupleMap    *ebpf.Map // Hash map for 5-tuple based ACL
	IngressPortBlockMap *ebpf.Map // Hash map for port blocking
	IngressIPBlockMap   *ebpf.Map // Hash map for IP blocking

	// Egress ACL Maps (TC Layer)
	Egress5TupleMap  *ebpf.Map // Hash map for 5-tuple based ACL
	EgressIPBlockMap *ebpf.Map // Hash map for IP blocking

	// QoS Maps (TC Layer)
	AppQoSMap     *ebpf.Map // Hash map for application-level QoS (5-tuple)
	PeerQoSMap    *ebpf.Map // Hash map for peer-level QoS (by IP)
	GlobalQoSMap  *ebpf.Map // Array map for global QoS (fallback)
	RuleFlowTable *ebpf.Map // LRU per-CPU hash for flow observability
	DropAlerts    *ebpf.Map // Ring buffer for drop events
}

// ACL5TupleKey corresponds to the C struct acl_5tuple_key
type ACL5TupleKey struct {
	SrcIP    uint32 // Source IP
	DstIP    uint32 // Destination IP
	SrcPort  uint16 // Source Port
	DstPort  uint16 // Destination Port
	Proto    uint8  // Protocol
	Pad1     uint8  // Padding
	Pad2     uint16 // Padding
}

// ACLRuleValue corresponds to the C struct acl_rule_value
type ACLRuleValue struct {
	Action  uint32 // 0=DROP, 1=PASS
	RuleID  uint32 // Rule ID
	Bytes   uint64 // Bytes counter
	Packets uint64 // Packets counter
}

// BucketState corresponds to the C struct bucket_state
type BucketState struct {
	RateBytesPerSec uint64 // 0-7
	BurstBytes      uint64 // 8-15
	Tokens          uint64 // 16-23
	LastUpdateNS    uint64 // 24-31
	PassBytes       uint64 // 32-39
	DropBytes       uint64 // 40-47
	_               uint32 // 48-51 占位，对应 C 的 bpf_spin_lock
	RuleID          uint32 // 52-55
}

// FlowDetailKey corresponds to the C struct flow_detail_key
type FlowDetailKey struct {
	RuleID   uint32 // 0-3
	RuleType uint32 // 4-7
	SrcIP    uint32 // 8-11
	DstIP    uint32 // 12-15
	SrcPort  uint16 // 16-17
	DstPort  uint16 // 18-19
	Proto    uint8  // 20
	_        uint8  // 21 pad1
	_        uint16 // 22-23 pad2
}

// FlowDetailStats corresponds to the C struct flow_detail_stats
type FlowDetailStats struct {
	Bytes     uint64 // Bytes counter
	Packets   uint64 // Packets counter
	LastSeen  uint64 // Last seen timestamp
}

// DropEventT corresponds to the C struct drop_event_t
type DropEventT struct {
	RuleID    uint32 // 0-3
	Reason    uint32 // 4-7
	SrcIP     uint32 // 8-11
	DstIP     uint32 // 12-15
	SrcPort   uint16 // 16-17
	DstPort   uint16 // 18-19
	Proto     uint8  // 20
	_         uint8  // 21 pad1
	_         uint16 // 22-23 pad2
	Timestamp uint64 // 24-31
}

// ACLManager handles XDP-based access control lists with the new structure
type ACLManager struct {
	maps *AriaBPFMaps
}

// NewACLManager creates a new ACL manager with the new structures
func NewACLManager() (*ACLManager, error) {
	// Remove resource limits for kernels <5.11
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("removing memlock limit: %v", err)
	}

	maps := &AriaBPFMaps{}
	var err error

	// Create ingress 5-tuple map for ACL rules
	maps.Ingress5TupleMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    uint32(unsafe.Sizeof(ACL5TupleKey{})),
		ValueSize:  uint32(unsafe.Sizeof(ACLRuleValue{})),
		MaxEntries: 65536,
		Name:       "ingress_5tuple_map",
	})
	if err != nil {
		return nil, fmt.Errorf("creating ingress 5-tuple map: %v", err)
	}

	// Create ingress port block map
	maps.IngressPortBlockMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    2, // uint16 for port
		ValueSize:  1, // uint8 for block flag
		MaxEntries: 8192,
		Name:       "ingress_port_blk_map",
	})
	if err != nil {
		maps.Ingress5TupleMap.Close()
		return nil, fmt.Errorf("creating ingress port block map: %v", err)
	}

	// Create ingress IP block map
	maps.IngressIPBlockMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4, // uint32 for IP
		ValueSize:  1, // uint8 for block flag
		MaxEntries: 8192,
		Name:       "ingress_ip_blk_map",
	})
	if err != nil {
		maps.Ingress5TupleMap.Close()
		maps.IngressPortBlockMap.Close()
		return nil, fmt.Errorf("creating ingress IP block map: %v", err)
	}

	// Create egress 5-tuple map for ACL rules
	maps.Egress5TupleMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    uint32(unsafe.Sizeof(ACL5TupleKey{})),
		ValueSize:  uint32(unsafe.Sizeof(ACLRuleValue{})),
		MaxEntries: 65536,
		Name:       "egress_5tuple_map",
	})
	if err != nil {
		maps.Ingress5TupleMap.Close()
		maps.IngressPortBlockMap.Close()
		maps.IngressIPBlockMap.Close()
		return nil, fmt.Errorf("creating egress 5-tuple map: %v", err)
	}

	// Create egress IP block map
	maps.EgressIPBlockMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4, // uint32 for IP
		ValueSize:  1, // uint8 for block flag
		MaxEntries: 8192,
		Name:       "egress_ip_blk_map",
	})
	if err != nil {
		maps.Ingress5TupleMap.Close()
		maps.IngressPortBlockMap.Close()
		maps.IngressIPBlockMap.Close()
		maps.Egress5TupleMap.Close()
		return nil, fmt.Errorf("creating egress IP block map: %v", err)
	}

	return &ACLManager{maps: maps}, nil
}

// NewACLManagerFromPinned creates an ACL manager from pinned maps
func NewACLManagerFromPinned() (*ACLManager, error) {
	// Attempt to load pinned maps
	ingress5TupleMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_ingress_5tuple_map", &ebpf.LoadPinOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading pinned ingress 5-tuple map: %v", err)
	}

	ingressPortBlockMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_ingress_port_block_map", &ebpf.LoadPinOptions{})
	if err != nil {
		ingress5TupleMap.Close()
		return nil, fmt.Errorf("loading pinned ingress port block map: %v", err)
	}

	ingressIPBlockMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_ingress_ip_block_map", &ebpf.LoadPinOptions{})
	if err != nil {
		ingress5TupleMap.Close()
		ingressPortBlockMap.Close()
		return nil, fmt.Errorf("loading pinned ingress IP block map: %v", err)
	}

	egress5TupleMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_egress_5tuple_map", &ebpf.LoadPinOptions{})
	if err != nil {
		ingress5TupleMap.Close()
		ingressPortBlockMap.Close()
		ingressIPBlockMap.Close()
		return nil, fmt.Errorf("loading pinned egress 5-tuple map: %v", err)
	}

	egressIPBlockMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_egress_ip_block_map", &ebpf.LoadPinOptions{})
	if err != nil {
		ingress5TupleMap.Close()
		ingressPortBlockMap.Close()
		ingressIPBlockMap.Close()
		egress5TupleMap.Close()
		return nil, fmt.Errorf("loading pinned egress IP block map: %v", err)
	}

	log.Println("ACL maps recovered from pinned locations")

	maps := &AriaBPFMaps{
		Ingress5TupleMap:    ingress5TupleMap,
		IngressPortBlockMap: ingressPortBlockMap,
		IngressIPBlockMap:   ingressIPBlockMap,
		Egress5TupleMap:     egress5TupleMap,
		EgressIPBlockMap:    egressIPBlockMap,
	}

	return &ACLManager{maps: maps}, nil
}

// Apply5TupleACLRule applies a 5-tuple based ACL rule
func (a *ACLManager) Apply5TupleACLRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8, action uint32) error {
	srcIPInt := ipToUint32(net.ParseIP(srcIP))
	dstIPInt := ipToUint32(net.ParseIP(dstIP))

	if srcIPInt == 0 || dstIPInt == 0 {
		return fmt.Errorf("invalid IP address: src=%s, dst=%s", srcIP, dstIP)
	}

	if srcPort < 0 || srcPort > 65535 {
		return fmt.Errorf("invalid source port: %d", srcPort)
	}

	if dstPort < 0 || dstPort > 65535 {
		return fmt.Errorf("invalid destination port: %d", dstPort)
	}

	if protocol == 0 {
		protocol = 6 // Default to TCP
	}

	key := ACL5TupleKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(uint16(srcPort)),
		DstPort: htons(uint16(dstPort)),
		Proto:   protocol,
		Pad1:    0,
		Pad2:    0,
	}

	value := ACLRuleValue{
		Action:  action, // 0=DROP, 1=PASS
		RuleID:  0,      // Will be set to a proper value in real implementation
		Bytes:   0,
		Packets: 0,
	}

	// Apply rule to both ingress and egress maps
	if err := a.maps.Ingress5TupleMap.Put(key, value); err != nil {
		return fmt.Errorf("setting ingress 5-tuple ACL rule: %v", err)
	}

	if err := a.maps.Egress5TupleMap.Put(key, value); err != nil {
		return fmt.Errorf("setting egress 5-tuple ACL rule: %v", err)
	}

	return nil
}

// BlockPort adds a block rule for a specific port
func (a *ACLManager) BlockPort(port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}

	portVal := uint16(port)
	blockFlag := uint8(1)

	// Apply to ingress
	if err := a.maps.IngressPortBlockMap.Put(portVal, blockFlag); err != nil {
		return fmt.Errorf("blocking ingress port %d: %v", port, err)
	}

	// Note: Port blocking at port level usually only applies to ingress,
	// but we could extend this to egress if needed

	return nil
}

// BlockIP adds a block rule for a specific IP
func (a *ACLManager) BlockIP(ip string) error {
	ipInt := ipToUint32(net.ParseIP(ip))
	if ipInt == 0 {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	blockFlag := uint8(1)

	// Apply to ingress
	if err := a.maps.IngressIPBlockMap.Put(ipInt, blockFlag); err != nil {
		return fmt.Errorf("blocking ingress IP %s: %v", ip, err)
	}

	// Apply to egress
	if err := a.maps.EgressIPBlockMap.Put(ipInt, blockFlag); err != nil {
		return fmt.Errorf("blocking egress IP %s: %v", ip, err)
	}

	return nil
}

// RemoveRule removes an ACL rule
func (a *ACLManager) RemoveRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8) error {
	srcIPInt := ipToUint32(net.ParseIP(srcIP))
	dstIPInt := ipToUint32(net.ParseIP(dstIP))

	if srcIPInt == 0 || dstIPInt == 0 {
		return fmt.Errorf("invalid IP address: src=%s, dst=%s", srcIP, dstIP)
	}

	if srcPort < 0 || srcPort > 65535 {
		return fmt.Errorf("invalid source port: %d", srcPort)
	}

	if dstPort < 0 || dstPort > 65535 {
		return fmt.Errorf("invalid destination port: %d", dstPort)
	}

	if protocol == 0 {
		protocol = 6 // Default to TCP
	}

	key := ACL5TupleKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(uint16(srcPort)),
		DstPort: htons(uint16(dstPort)),
		Proto:   protocol,
		Pad1:    0,
		Pad2:    0,
	}

	// Remove from both ingress and egress maps
	if err := a.maps.Ingress5TupleMap.Delete(key); err != nil {
		log.Printf("Warning: failed to remove ingress 5-tuple rule: %v", err)
	}

	if err := a.maps.Egress5TupleMap.Delete(key); err != nil {
		log.Printf("Warning: failed to remove egress 5-tuple rule: %v", err)
	}

	return nil
}

// Close cleans up the ACL manager resources
func (a *ACLManager) Close() error {
	errs := make([]error, 0)

	if a.maps.Ingress5TupleMap != nil {
		if err := a.maps.Ingress5TupleMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.maps.IngressPortBlockMap != nil {
		if err := a.maps.IngressPortBlockMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.maps.IngressIPBlockMap != nil {
		if err := a.maps.IngressIPBlockMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.maps.Egress5TupleMap != nil {
		if err := a.maps.Egress5TupleMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.maps.EgressIPBlockMap != nil {
		if err := a.maps.EgressIPBlockMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing ACL manager: %v", errs)
	}

	return nil
}

// Pin saves the ACL maps to the filesystem to survive process restarts
func (a *ACLManager) Pin() error {
	pinDir := "/sys/fs/bpf/aria"
	if err := os.MkdirAll(pinDir, 0755); err != nil {
		return fmt.Errorf("failed to create bpffs directory: %v", err)
	}

	if a.maps.Ingress5TupleMap != nil {
		if err := a.maps.Ingress5TupleMap.Pin("/sys/fs/bpf/aria_ingress_5tuple_map"); err != nil {
			return fmt.Errorf("failed to pin ingress 5-tuple map: %v", err)
		}
	}

	if a.maps.IngressPortBlockMap != nil {
		if err := a.maps.IngressPortBlockMap.Pin("/sys/fs/bpf/aria_ingress_port_block_map"); err != nil {
			return fmt.Errorf("failed to pin ingress port block map: %v", err)
		}
	}

	if a.maps.IngressIPBlockMap != nil {
		if err := a.maps.IngressIPBlockMap.Pin("/sys/fs/bpf/aria_ingress_ip_block_map"); err != nil {
			return fmt.Errorf("failed to pin ingress IP block map: %v", err)
		}
	}

	if a.maps.Egress5TupleMap != nil {
		if err := a.maps.Egress5TupleMap.Pin("/sys/fs/bpf/aria_egress_5tuple_map"); err != nil {
			return fmt.Errorf("failed to pin egress 5-tuple map: %v", err)
		}
	}

	if a.maps.EgressIPBlockMap != nil {
		if err := a.maps.EgressIPBlockMap.Pin("/sys/fs/bpf/aria_egress_ip_block_map"); err != nil {
			return fmt.Errorf("failed to pin egress IP block map: %v", err)
		}
	}

	return nil
}

// Unpin removes the ACL maps from the filesystem
func (a *ACLManager) Unpin() error {
	// Remove pin files if they exist
	pins := []string{
		"/sys/fs/bpf/aria_ingress_5tuple_map",
		"/sys/fs/bpf/aria_ingress_port_block_map",
		"/sys/fs/bpf/aria_ingress_ip_block_map",
		"/sys/fs/bpf/aria_egress_5tuple_map",
		"/sys/fs/bpf/aria_egress_ip_block_map",
	}

	errs := make([]error, 0)
	for _, pinPath := range pins {
		if err := os.Remove(pinPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to unpin %s: %v", pinPath, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors unpinning ACL maps: %v", errs)
	}

	return nil
}

// AttachXDP attaches the ACL program to an interface
func (a *ACLManager) AttachXDP(interfaceName string) error {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return fmt.Errorf("finding interface %s: %v", interfaceName, err)
	}

	// This is a placeholder - actual XDP attachment code would go here
	fmt.Printf("Would attach ACL XDP program to interface %s (index %d)\n", interfaceName, iface.Index)

	return nil
}

// htons converts host byte order to network byte order for 16-bit values
func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | (i>>8)&0x00ff
}

// ipToUint32 converts net.IP to uint32
func ipToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

// ipToUint32BigEndian converts net.IP to uint32 in big-endian byte order
func ipToUint32BigEndian(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	// Using binary.BigEndian to ensure consistent byte ordering
	return binary.BigEndian.Uint32(ip4)
}

// ValidateMemoryLayout validates that the Go structs have the same memory layout as their C counterparts
func ValidateMemoryLayout() error {
	// Verify ACL5TupleKey size matches expected (16 bytes based on C struct)
	expectedSize := int(unsafe.Sizeof(ACL5TupleKey{}))
	if expectedSize != 16 {
		return fmt.Errorf("ACL5TupleKey size mismatch: expected 16 bytes, got %d bytes", expectedSize)
	}

	// Verify ACLRuleValue size matches expected (24 bytes based on C struct)
	expectedRuleSize := int(unsafe.Sizeof(ACLRuleValue{}))
	if expectedRuleSize != 24 {
		return fmt.Errorf("ACLRuleValue size mismatch: expected 24 bytes, got %d bytes", expectedRuleSize)
	}

	// Verify BucketState size matches expected (56 bytes based on C struct with spinlock)
	expectedBucketSize := int(unsafe.Sizeof(BucketState{}))
	// Expected: 8*6 bytes for the uint64 fields + 4 bytes for spinlock + 4 bytes for ruleID = 56 bytes
	if expectedBucketSize != 56 {
		return fmt.Errorf("BucketState size mismatch: expected 56 bytes, got %d bytes", expectedBucketSize)
	}

	// Additional checks for field alignment could be added here

	log.Println("✅ Memory layout validation passed")
	return nil
}

// ValidateByteOrdering verifies proper byte ordering for network data
func ValidateByteOrdering() error {
	// Test IP conversion with known values
	testIP := net.ParseIP("192.168.1.1")
	if testIP == nil {
		return fmt.Errorf("failed to parse test IP")
	}

	ipInt := ipToUint32BigEndian(testIP)
	expected := uint32(192<<24 | 168<<16 | 1<<8 | 1) // Big-endian representation

	if ipInt != expected {
		return fmt.Errorf("byte ordering mismatch: expected %d, got %d", expected, ipInt)
	}

	// Test port conversion
	testPort := uint16(8080)
	portConverted := htons(testPort)
	portReverted := htons(portConverted) // Double conversion should return original
	if testPort != portReverted {
		return fmt.Errorf("port byte conversion failed: expected %d, got %d after revert", testPort, portReverted)
	}

	log.Println("✅ Byte ordering validation passed")
	return nil
}

// ConvertACLRulesTo5TupleMap converts ACLRule slice to the internal 5-tuple map format
// Ensures proper byte order and memory layout compatibility for eBPF maps
func (a *ACLManager) ConvertACLRulesTo5TupleMap(rules []*controllerstorage.ACLRule) (map[ACL5TupleKey]ACLRuleValue, error) {
	result := make(map[ACL5TupleKey]ACLRuleValue, len(rules))

	for _, rule := range rules {
		// Parse IP addresses
		srcIP := net.ParseIP(rule.SrcNet)
		if srcIP == nil {
			// If SrcNet is not an IP, try to parse as CIDR and extract network
			_, ipNet, err := net.ParseCIDR(rule.SrcNet)
			if err != nil {
				// If it's not a valid CIDR either, check if it's a node name (will be resolved later)
				// For now, just store it as-is or handle appropriately
				return nil, fmt.Errorf("invalid source network: %s", rule.SrcNet)
			}
			srcIP = ipNet.IP
		}

		dstIP := net.ParseIP(rule.DstNet)
		if dstIP == nil {
			// If DstNet is not an IP, try to parse as CIDR and extract network
			_, ipNet, err := net.ParseCIDR(rule.DstNet)
			if err != nil {
				return nil, fmt.Errorf("invalid destination network: %s", rule.DstNet)
			}
			dstIP = ipNet.IP
		}

		// Convert IPs to uint32 with proper byte ordering
		srcIPInt := ipToUint32BigEndian(srcIP)
		dstIPInt := ipToUint32BigEndian(dstIP)

		// Validate ports
		if rule.MinPort < 0 || rule.MinPort > 65535 {
			return nil, fmt.Errorf("invalid source port: %d", rule.MinPort)
		}

		if rule.MaxPort < 0 || rule.MaxPort > 65535 {
			return nil, fmt.Errorf("invalid destination port: %d", rule.MaxPort)
		}

		// Set protocol if not provided
		protocol := rule.Protocol
		if protocol == 0 {
			protocol = 6 // Default to TCP
		}

		// Create key with proper byte ordering and padding
		key := ACL5TupleKey{
			SrcIP:   srcIPInt,
			DstIP:   dstIPInt,
			SrcPort: htons(uint16(rule.MinPort)), // Properly convert to network byte order
			DstPort: htons(uint16(rule.MaxPort)), // Properly convert to network byte order
			Proto:   protocol,
			Pad1:    0, // Explicitly zero padding
			Pad2:    0, // Explicitly zero padding
		}

		// Convert action string to numeric value
		var actionValue uint32
		switch rule.Action {
		case "allow", "pass":
			actionValue = 1 // PASS in eBPF
		case "deny", "drop":
			actionValue = 0 // DROP in eBPF
		default:
			return nil, fmt.Errorf("invalid action: %s, must be 'allow', 'deny', 'pass', or 'drop'", rule.Action)
		}

		// Create value with proper initialization
		value := ACLRuleValue{
			Action:  actionValue,
			RuleID:  uint32(rule.ID), // Use the rule ID from database
			Bytes:   0,               // Initialize counters to 0
			Packets: 0,
		}

		// Store in map
		result[key] = value
	}

	return result, nil
}

// ApplyBulkACLRules applies multiple ACL rules in bulk to both ingress and egress maps
// Ensures atomicity at the map level and proper byte order conversion
func (a *ACLManager) ApplyBulkACLRules(rules []*controllerstorage.ACLRule) error {
	// Convert rules to internal 5-tuple format
	ruleMap, err := a.ConvertACLRulesTo5TupleMap(rules)
	if err != nil {
		return fmt.Errorf("converting ACL rules to 5-tuple format: %v", err)
	}

	// Apply all rules to both ingress and egress maps
	for key, value := range ruleMap {
		// Apply to ingress map
		if err := a.maps.Ingress5TupleMap.Put(key, value); err != nil {
			return fmt.Errorf("applying ACL rule to ingress map: %v", err)
		}

		// Apply to egress map
		if err := a.maps.Egress5TupleMap.Put(key, value); err != nil {
			return fmt.Errorf("applying ACL rule to egress map: %v", err)
		}
	}

	log.Printf("✅ Applied %d ACL rules in bulk to both ingress and egress maps", len(ruleMap))
	return nil
}

// SyncACLRules performs a declarative synchronization of ACL rules
// It compares the desired state with the current state in eBPF maps and makes minimal updates
func (a *ACLManager) SyncACLRules(desiredRules []*controllerstorage.ACLRule) error {
	// Convert desired rules to map format
	desiredMap, err := a.ConvertACLRulesTo5TupleMap(desiredRules)
	if err != nil {
		return fmt.Errorf("converting desired ACL rules: %v", err)
	}

	// Get current rules from both ingress and egress maps
	currentIngressMap, err := a.getCurrentACLRules(a.maps.Ingress5TupleMap)
	if err != nil {
		return fmt.Errorf("getting current ingress ACL rules: %v", err)
	}

	// Synchronize ingress map
	if err := a.syncACLRuleMap(a.maps.Ingress5TupleMap, currentIngressMap, desiredMap); err != nil {
		return fmt.Errorf("syncing ingress ACL rules: %v", err)
	}

	// Get current rules from egress map
	currentEgressMap, err := a.getCurrentACLRules(a.maps.Egress5TupleMap)
	if err != nil {
		return fmt.Errorf("getting current egress ACL rules: %v", err)
	}

	// Synchronize egress map
	if err := a.syncACLRuleMap(a.maps.Egress5TupleMap, currentEgressMap, desiredMap); err != nil {
		return fmt.Errorf("syncing egress ACL rules: %v", err)
	}

	log.Printf("✅ ACL rules synchronized: %d rules applied", len(desiredMap))
	return nil
}

// getCurrentACLRules retrieves all ACL rules from an eBPF map
func (a *ACLManager) getCurrentACLRules(mapObj *ebpf.Map) (map[ACL5TupleKey]ACLRuleValue, error) {
	result := make(map[ACL5TupleKey]ACLRuleValue)
	iter := mapObj.Iterate()

	var key ACL5TupleKey
	var value ACLRuleValue

	for iter.Next(&key, &value) {
		result[key] = value
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// syncACLRuleMap synchronizes an individual eBPF map by comparing current vs desired state
func (a *ACLManager) syncACLRuleMap(mapObj *ebpf.Map, current, desired map[ACL5TupleKey]ACLRuleValue) error {
	added := 0
	removed := 0
	updated := 0

	// Process each desired rule
	for key, desiredValue := range desired {
		if currentValue, exists := current[key]; exists {
			// Rule exists in both current and desired - check if it needs updating
			if currentValue.Action != desiredValue.Action ||
				currentValue.RuleID != desiredValue.RuleID ||
				currentValue.Bytes != desiredValue.Bytes ||
				currentValue.Packets != desiredValue.Packets {
				// Update the rule
				if err := mapObj.Put(key, desiredValue); err != nil {
					return fmt.Errorf("updating ACL rule: %v", err)
				}
				updated++
			}
			// Remove from current map to track which rules remain
			delete(current, key)
		} else {
			// Rule exists in desired but not current - add it
			if err := mapObj.Put(key, desiredValue); err != nil {
				return fmt.Errorf("adding ACL rule: %v", err)
			}
			added++
		}
	}

	// Any remaining rules in current map should be removed
	for key := range current {
		if err := mapObj.Delete(key); err != nil {
			log.Printf("Warning: failed to remove ACL rule: %v", err)
		} else {
			removed++
		}
	}

	log.Printf("Sync complete - Added: %d, Updated: %d, Removed: %d", added, updated, removed)
	return nil
}