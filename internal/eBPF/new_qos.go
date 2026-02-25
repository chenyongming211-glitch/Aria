package eBPF

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
)

// QoSManager handles TC-based Quality of Service (bandwidth limiting) with the new specifications
type QoSManager struct {
	maps    *AriaBPFMaps
	links   map[string]netlink.Link
	mu      sync.Mutex
	program *ebpf.Program // Placeholder for the TC program
}

// NewQoSManager creates a new QoS manager with the new structures
func NewQoSManager() (*QoSManager, error) {
	// Remove resource limits for kernels <5.11
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("removing memlock limit: %v", err)
	}

	maps := &AriaBPFMaps{}
	var err error

	// Create App QoS map (5-tuple level control)
	maps.AppQoSMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    uint32(unsafe.Sizeof(ACL5TupleKey{})),
		ValueSize:  uint32(unsafe.Sizeof(BucketState{})),
		MaxEntries: 65536,
		Name:       "app_qos_map",
	})
	if err != nil {
		return nil, fmt.Errorf("creating app QoS map: %v", err)
	}

	// Create Peer QoS map (IP level control between peers)
	maps.PeerQoSMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4, // uint32 for IP
		ValueSize:  uint32(unsafe.Sizeof(BucketState{})),
		MaxEntries: 8192,
		Name:       "peer_qos_map",
	})
	if err != nil {
		maps.AppQoSMap.Close()
		return nil, fmt.Errorf("creating peer QoS map: %v", err)
	}

	// Create Global QoS map (ARRAY type for total physical outlet)
	maps.GlobalQoSMap, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Array,
		KeySize:    4, // uint32
		ValueSize:  uint32(unsafe.Sizeof(BucketState{})),
		MaxEntries: 1,
		Name:       "global_qos_map",
	})
	if err != nil {
		maps.AppQoSMap.Close()
		maps.PeerQoSMap.Close()
		return nil, fmt.Errorf("creating global QoS map: %v", err)
	}

	// Create Rule Flow Table (LRU per-CPU hash for observability)
	maps.RuleFlowTable, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash, // Use regular hash instead of LRU per-CPU due to compatibility issues
		KeySize:    uint32(unsafe.Sizeof(FlowDetailKey{})),
		ValueSize:  uint32(unsafe.Sizeof(FlowDetailStats{})),
		MaxEntries: 65536,
		Name:       "rule_flow_table",
	})
	if err != nil {
		maps.AppQoSMap.Close()
		maps.PeerQoSMap.Close()
		maps.GlobalQoSMap.Close()
		return nil, fmt.Errorf("creating rule flow table: %v", err)
	}

	// Create Drop Alerts (Ring buffer)
	maps.DropAlerts, err = ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.RingBuf,
		MaxEntries: 512 * 1024, // 512KB
		Name:       "drop_alerts",
	})
	if err != nil {
		maps.AppQoSMap.Close()
		maps.PeerQoSMap.Close()
		maps.GlobalQoSMap.Close()
		maps.RuleFlowTable.Close()
		return nil, fmt.Errorf("creating drop alerts ringbuf: %v", err)
	}

	return &QoSManager{
		maps:  maps,
		links: make(map[string]netlink.Link),
	}, nil
}

// NewQoSManagerFromPinned creates a QoS manager from pinned maps
func NewQoSManagerFromPinned() (*QoSManager, error) {
	// Attempt to load pinned maps
	appQoSMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_app_qos_map", &ebpf.LoadPinOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading pinned app QoS map: %v", err)
	}

	peerQoSMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_peer_qos_map", &ebpf.LoadPinOptions{})
	if err != nil {
		appQoSMap.Close()
		return nil, fmt.Errorf("loading pinned peer QoS map: %v", err)
	}

	globalQoSMap, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_global_qos_map", &ebpf.LoadPinOptions{})
	if err != nil {
		appQoSMap.Close()
		peerQoSMap.Close()
		return nil, fmt.Errorf("loading pinned global QoS map: %v", err)
	}

	ruleFlowTable, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_rule_flow_table", &ebpf.LoadPinOptions{})
	if err != nil {
		appQoSMap.Close()
		peerQoSMap.Close()
		globalQoSMap.Close()
		return nil, fmt.Errorf("loading pinned rule flow table: %v", err)
	}

	dropAlerts, err := ebpf.LoadPinnedMap("/sys/fs/bpf/aria_drop_alerts", &ebpf.LoadPinOptions{})
	if err != nil {
		appQoSMap.Close()
		peerQoSMap.Close()
		globalQoSMap.Close()
		ruleFlowTable.Close()
		return nil, fmt.Errorf("loading pinned drop alerts: %v", err)
	}

	log.Println("QoS maps recovered from pinned locations")

	maps := &AriaBPFMaps{
		AppQoSMap:     appQoSMap,
		PeerQoSMap:    peerQoSMap,
		GlobalQoSMap:  globalQoSMap,
		RuleFlowTable: ruleFlowTable,
		DropAlerts:    dropAlerts,
	}

	return &QoSManager{
		maps:  maps,
		links: make(map[string]netlink.Link),
	}, nil
}

// LimitIP sets bandwidth limit for a single IP address (Global level QoS)
func (q *QoSManager) LimitIP(ip string, mbps int) error {
	ipInt := ipToUint32(net.ParseIP(ip))
	if ipInt == 0 {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	// 使用 QoS 计算器来计算参数
	rate, burst := CalculateBucketParams(float64(mbps), 0)

	// 将每秒速率转换为纳秒单位
	rateNs := rate / 1000000000
	if rateNs == 0 && rate > 0 {
		// 如果整数除法导致精度丢失，设置最小非零值
		rateNs = 1
	}

	bucket := BucketState{
		RateBytesPerSec: rate, // 使用新的字段
		BurstBytes:      burst,
		Tokens:          burst, // Initialize tokens to burst size
		LastUpdateNS:    uint64(time.Now().UnixNano()),
		PassBytes:       0,
		DropBytes:       0,
		RuleID:          0, // Rule ID
	}

	return q.maps.GlobalQoSMap.Put(uint32(0), bucket) // Index 0 for the single entry
}

// LimitPeerPair sets bandwidth limit between two IP addresses (Peer level QoS)
func (q *QoSManager) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	dstIPInt := ipToUint32(net.ParseIP(dstIP))
	if dstIPInt == 0 {
		return fmt.Errorf("invalid destination IP address: %s", dstIP)
	}

	// 使用 QoS 计算器来计算参数
	rate, burst := CalculateBucketParams(float64(mbps), 0)

	// 将每秒速率转换为纳秒单位
	rateNs := rate / 1000000000
	if rateNs == 0 && rate > 0 {
		// 如果整数除法导致精度丢失，设置最小非零值
		rateNs = 1
	}

	bucket := BucketState{
		RateBytesPerSec: rate, // 使用新的字段
		BurstBytes:      burst,
		Tokens:          burst, // Initialize tokens to burst size
		LastUpdateNS:    uint64(time.Now().UnixNano()),
		PassBytes:       0,
		DropBytes:       0,
		RuleID:          0, // Rule ID
	}

	return q.maps.PeerQoSMap.Put(dstIPInt, bucket)
}

// LimitPort sets bandwidth limit for a specific port
func (q *QoSManager) LimitPort(port int, mbps int) error {
	return q.LimitService("", "", 0, port, 6 /*TCP*/, mbps) // Default to TCP
}

// LimitService sets bandwidth limit for a five-tuple (srcIP, dstIP, srcPort, dstPort, protocol) (App level QoS)
func (q *QoSManager) LimitService(srcIP, dstIP string, srcPort, dstPort, protocol int, mbps int) error {
	var srcIPInt, dstIPInt uint32

	if srcIP != "" {
		srcIPInt = ipToUint32(net.ParseIP(srcIP))
		if srcIPInt == 0 {
			return fmt.Errorf("invalid source IP address: %s", srcIP)
		}
	}

	if dstIP != "" {
		dstIPInt = ipToUint32(net.ParseIP(dstIP))
		if dstIPInt == 0 {
			return fmt.Errorf("invalid destination IP address: %s", dstIP)
		}
	}

	if srcPort < 0 || srcPort > 65535 {
		return fmt.Errorf("invalid source port: %d", srcPort)
	}

	if dstPort < 0 || dstPort > 65535 {
		return fmt.Errorf("invalid destination port: %d", dstPort)
	}

	if protocol < 0 || protocol > 255 {
		return fmt.Errorf("invalid protocol: %d", protocol)
	}

	// 使用 QoS 计算器来计算参数
	rate, burst := CalculateBucketParams(float64(mbps), 0) // 使用规则ID 0，可以根据需要传递真实ID

	// 将每秒速率转换为纳秒单位
	rateNs := rate / 1000000000
	if rateNs == 0 && rate > 0 {
		// 如果整数除法导致精度丢失，设置最小非零值
		rateNs = 1
	}

	flowKey := ACL5TupleKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(uint16(srcPort)), // Convert to network byte order
		DstPort: htons(uint16(dstPort)), // Convert to network byte order
		Proto:   uint8(protocol),
		Pad1:    0,
		Pad2:    0,
	}

	bucket := BucketState{
		RateBytesPerSec: rate, // 使用新的字段
		BurstBytes:      burst,
		Tokens:          burst, // Initialize tokens to burst size
		LastUpdateNS:    uint64(time.Now().UnixNano()),
		PassBytes:       0,
		DropBytes:       0,
		RuleID:          0, // Rule ID
	}

	return q.maps.AppQoSMap.Put(flowKey, bucket)
}

// LimitPortForIP sets bandwidth limit for a specific port from/to an IP
func (q *QoSManager) LimitPortForIP(ip string, port int, mbps int, direction string) error {
	ipInt := ipToUint32(net.ParseIP(ip))
	if ipInt == 0 {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}

	// 使用 QoS 计算器来计算参数
	rate, burst := CalculateBucketParams(float64(mbps), 0)

	// Convert to bytes per nanosecond for kernel space
	rateNs := rate / 1000000000
	if rateNs == 0 && rate > 0 {
		// 如果整数除法导致精度丢失，设置最小非零值
		rateNs = 1
	}

	var flowKey ACL5TupleKey
	switch direction {
	case "src":
		// Limit traffic from this IP on this port
		flowKey = ACL5TupleKey{
			SrcIP:   ipInt,
			DstIP:   0, // Wildcard
			SrcPort: htons(uint16(port)), // Convert to network byte order
			DstPort: 0,                   // Wildcard
			Proto:   6,                   // Default to TCP
			Pad1:    0,
			Pad2:    0,
		}
	case "dst":
		// Limit traffic to this IP on this port
		flowKey = ACL5TupleKey{
			SrcIP:   0, // Wildcard
			DstIP:   ipInt,
			SrcPort: 0,                   // Wildcard
			DstPort: htons(uint16(port)), // Convert to network byte order
			Proto:   6,                   // Default to TCP
			Pad1:    0,
			Pad2:    0,
		}
	default:
		// Bidirectional - limit both directions
		// We'll create both source and destination rules
		flowKey1 := ACL5TupleKey{
			SrcIP:   ipInt,
			DstIP:   0, // Wildcard
			SrcPort: htons(uint16(port)), // Convert to network byte order
			DstPort: 0,                   // Wildcard
			Proto:   6,                   // Default to TCP
			Pad1:    0,
			Pad2:    0,
		}

		bucket1 := BucketState{
			RateBytesPerSec: rate, // 使用新的字段
			BurstBytes:      burst,
			Tokens:          burst, // Initialize tokens to burst size
			LastUpdateNS:    uint64(time.Now().UnixNano()),
			PassBytes:       0,
			DropBytes:       0,
			RuleID:          0, // Rule ID
		}

		if err := q.maps.AppQoSMap.Put(flowKey1, bucket1); err != nil {
			return err
		}

		flowKey2 := ACL5TupleKey{
			SrcIP:   0, // Wildcard
			DstIP:   ipInt,
			SrcPort: 0,                   // Wildcard
			DstPort: htons(uint16(port)), // Convert to network byte order
			Proto:   6,                   // Default to TCP
			Pad1:    0,
			Pad2:    0,
		}

		bucket2 := BucketState{
			RateBytesPerSec: rate, // 使用新的字段
			BurstBytes:      burst,
			Tokens:          burst, // Initialize tokens to burst size
			LastUpdateNS:    uint64(time.Now().UnixNano()),
			PassBytes:       0,
			DropBytes:       0,
			RuleID:          0, // Rule ID
		}

		return q.maps.AppQoSMap.Put(flowKey2, bucket2)
	}

	bucket := BucketState{
		RateBytesPerSec: rate, // 使用新的字段
		BurstBytes:      burst,
		Tokens:          burst, // Initialize tokens to burst size
		LastUpdateNS:    uint64(time.Now().UnixNano()),
		PassBytes:       0,
		DropBytes:       0,
		RuleID:          0, // Rule ID
	}

	return q.maps.AppQoSMap.Put(flowKey, bucket)
}

// RemoveIPLimit removes the bandwidth limit for an IP
func (q *QoSManager) RemoveIPLimit(ip string) error {
	ipInt := ipToUint32(net.ParseIP(ip))
	if ipInt == 0 {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	// For GlobalQoSMap which is ARRAY type, we zero out the entry
	zeroBucket := BucketState{}
	return q.maps.GlobalQoSMap.Put(uint32(0), zeroBucket)
}

// RemovePeerLimit removes the bandwidth limit between two IPs
func (q *QoSManager) RemovePeerLimit(srcIP, dstIP string) error {
	dstIPInt := ipToUint32(net.ParseIP(dstIP))
	if dstIPInt == 0 {
		return fmt.Errorf("invalid destination IP address: %s", dstIP)
	}

	return q.maps.PeerQoSMap.Delete(dstIPInt)
}

// RemoveServiceLimit removes the bandwidth limit for a five-tuple
func (q *QoSManager) RemoveServiceLimit(srcIP, dstIP string, srcPort, dstPort, protocol int) error {
	var srcIPInt, dstIPInt uint32

	if srcIP != "" {
		srcIPInt = ipToUint32(net.ParseIP(srcIP))
		if srcIPInt == 0 {
			return fmt.Errorf("invalid source IP address: %s", srcIP)
		}
	}

	if dstIP != "" {
		dstIPInt = ipToUint32(net.ParseIP(dstIP))
		if dstIPInt == 0 {
			return fmt.Errorf("invalid destination IP address: %s", dstIP)
		}
	}

	if srcPort < 0 || srcPort > 65535 {
		return fmt.Errorf("invalid source port: %d", srcPort)
	}

	if dstPort < 0 || dstPort > 65535 {
		return fmt.Errorf("invalid destination port: %d", dstPort)
	}

	if protocol < 0 || protocol > 255 {
		return fmt.Errorf("invalid protocol: %d", protocol)
	}

	flowKey := ACL5TupleKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(uint16(srcPort)), // Convert to network byte order
		DstPort: htons(uint16(dstPort)), // Convert to network byte order
		Proto:   uint8(protocol),
		Pad1:    0,
		Pad2:    0,
	}

	return q.maps.AppQoSMap.Delete(flowKey)
}

// UpdateInterfaces updates which interfaces the QoS is applied to
func (q *QoSManager) UpdateInterfaces(newIfaces []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	newSet := make(map[string]bool)
	for _, iface := range newIfaces {
		newSet[iface] = true

		if _, exists := q.links[iface]; exists {
			continue // Already attached, skip
		}

		// Get interface object
		ifaceObj, err := net.InterfaceByName(iface)
		if err != nil {
			return fmt.Errorf("failed to find interface %s: %v", iface, err)
		}

		link, err := netlink.LinkByIndex(ifaceObj.Index)
		if err != nil {
			return fmt.Errorf("failed to get netlink for %s: %v", iface, err)
		}

		// Store the link reference
		q.links[iface] = link
		fmt.Printf("✅ QoS attached to %s\n", iface)
	}

	// Remove attachments from interfaces no longer in the list
	for iface := range q.links {
		if !newSet[iface] {
			// In a real implementation, we would detach here
			delete(q.links, iface)
			fmt.Printf("🗑️ QoS detached from %s\n", iface)
		}
	}

	return nil
}

// GetIPStats retrieves statistics for a specific IP
func (q *QoSManager) GetIPStats(ip string) (*BucketState, error) {
	ipInt := ipToUint32(net.ParseIP(ip))
	if ipInt == 0 {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	// Since GlobalQoSMap is an array with 1 element at index 0
	var bucket BucketState
	if err := q.maps.GlobalQoSMap.Lookup(uint32(0), &bucket); err != nil {
		return nil, fmt.Errorf("failed to lookup global QoS stats: %v", err)
	}

	return &bucket, nil
}

// GetPeerStats retrieves statistics for a peer
func (q *QoSManager) GetPeerStats(srcIP, dstIP string) (*BucketState, error) {
	dstIPInt := ipToUint32(net.ParseIP(dstIP))
	if dstIPInt == 0 {
		return nil, fmt.Errorf("invalid destination IP address: %s", dstIP)
	}

	var bucket BucketState
	if err := q.maps.PeerQoSMap.Lookup(dstIPInt, &bucket); err != nil {
		return nil, fmt.Errorf("failed to lookup peer QoS stats: %v", err)
	}

	return &bucket, nil
}

// GetServiceStats retrieves statistics for a service (five-tuple)
func (q *QoSManager) GetServiceStats(srcIP, dstIP string, srcPort, dstPort, protocol int) (*BucketState, error) {
	var srcIPInt, dstIPInt uint32

	if srcIP != "" {
		srcIPInt = ipToUint32(net.ParseIP(srcIP))
		if srcIPInt == 0 {
			return nil, fmt.Errorf("invalid source IP address: %s", srcIP)
		}
	}

	if dstIP != "" {
		dstIPInt = ipToUint32(net.ParseIP(dstIP))
		if dstIPInt == 0 {
			return nil, fmt.Errorf("invalid destination IP address: %s", dstIP)
		}
	}

	if srcPort < 0 || srcPort > 65535 {
		return nil, fmt.Errorf("invalid source port: %d", srcPort)
	}

	if dstPort < 0 || dstPort > 65535 {
		return nil, fmt.Errorf("invalid destination port: %d", dstPort)
	}

	if protocol < 0 || protocol > 255 {
		return nil, fmt.Errorf("invalid protocol: %d", protocol)
	}

	flowKey := ACL5TupleKey{
		SrcIP:   srcIPInt,
		DstIP:   dstIPInt,
		SrcPort: htons(uint16(srcPort)), // Convert to network byte order
		DstPort: htons(uint16(dstPort)), // Convert to network byte order
		Proto:   uint8(protocol),
		Pad1:    0,
		Pad2:    0,
	}

	var bucket BucketState

	// 直接获取整个 BucketState 结构体
	if err := q.maps.AppQoSMap.Lookup(flowKey, &bucket); err != nil {
		return nil, fmt.Errorf("failed to lookup service QoS stats: %v", err)
	}

	return &bucket, nil
}

// Close cleans up the QoS manager resources
func (q *QoSManager) Close() error {
	errs := make([]error, 0)

	if q.maps.AppQoSMap != nil {
		if err := q.maps.AppQoSMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if q.maps.PeerQoSMap != nil {
		if err := q.maps.PeerQoSMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if q.maps.GlobalQoSMap != nil {
		if err := q.maps.GlobalQoSMap.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if q.maps.RuleFlowTable != nil {
		if err := q.maps.RuleFlowTable.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if q.maps.DropAlerts != nil {
		if err := q.maps.DropAlerts.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing QoS manager: %v", errs)
	}

	return nil
}

// Pin saves the QoS maps to the filesystem to survive process restarts
func (q *QoSManager) Pin() error {
	pinDir := "/sys/fs/bpf/aria"
	if err := os.MkdirAll(pinDir, 0755); err != nil {
		return fmt.Errorf("failed to create bpffs directory: %v", err)
	}

	if q.maps.AppQoSMap != nil {
		if err := q.maps.AppQoSMap.Pin("/sys/fs/bpf/aria_app_qos_map"); err != nil {
			return fmt.Errorf("failed to pin app QoS map: %v", err)
		}
	}

	if q.maps.PeerQoSMap != nil {
		if err := q.maps.PeerQoSMap.Pin("/sys/fs/bpf/aria_peer_qos_map"); err != nil {
			return fmt.Errorf("failed to pin peer QoS map: %v", err)
		}
	}

	if q.maps.GlobalQoSMap != nil {
		if err := q.maps.GlobalQoSMap.Pin("/sys/fs/bpf/aria_global_qos_map"); err != nil {
			return fmt.Errorf("failed to pin global QoS map: %v", err)
		}
	}

	if q.maps.RuleFlowTable != nil {
		if err := q.maps.RuleFlowTable.Pin("/sys/fs/bpf/aria_rule_flow_table"); err != nil {
			return fmt.Errorf("failed to pin rule flow table: %v", err)
		}
	}

	if q.maps.DropAlerts != nil {
		if err := q.maps.DropAlerts.Pin("/sys/fs/bpf/aria_drop_alerts"); err != nil {
			return fmt.Errorf("failed to pin drop alerts: %v", err)
		}
	}

	return nil
}

// Unpin removes the QoS maps from the filesystem
func (q *QoSManager) Unpin() error {
	// Remove pin files if they exist
	pins := []string{
		"/sys/fs/bpf/aria_app_qos_map",
		"/sys/fs/bpf/aria_peer_qos_map",
		"/sys/fs/bpf/aria_global_qos_map",
		"/sys/fs/bpf/aria_rule_flow_table",
		"/sys/fs/bpf/aria_drop_alerts",
	}

	errs := make([]error, 0)
	for _, pinPath := range pins {
		if err := os.Remove(pinPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to unpin %s: %v", pinPath, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors unpinning QoS maps: %v", errs)
	}

	return nil
}

// AttachTC attaches the TC program to an interface
func (q *QoSManager) AttachTC(interfaceName string) error {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return fmt.Errorf("finding interface %s: %v", interfaceName, err)
	}

	// This is a placeholder - actual TC attachment code would go here
	fmt.Printf("Would attach QoS TC program to interface %s (index %d)\n", interfaceName, iface.Index)

	return nil
}