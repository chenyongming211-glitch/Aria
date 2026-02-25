package eBPF

import (
	"fmt"
	"log"
)

// FirewallQoSManager 统一管理防火墙和 QoS 功能
type FirewallQoSManager struct {
	aclMgr *ACLManager
	qosMgr *QoSManager
}

// NewFirewallQoSManager 创建新的防火墙和 QoS 管理器
func NewFirewallQoSManager() (*FirewallQoSManager, error) {
	aclMgr, err := NewACLManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create ACL manager: %v", err)
	}

	qosMgr, err := NewQoSManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create QoS manager: %v", err)
	}

	return &FirewallQoSManager{
		aclMgr: aclMgr,
		qosMgr: qosMgr,
	}, nil
}

// NewFirewallQoSManagerFromPinned 从已固定的 eBPF maps 创建管理器
func NewFirewallQoSManagerFromPinned() (*FirewallQoSManager, error) {
	aclMgr, err := NewACLManagerFromPinned()
	if err != nil {
		return nil, fmt.Errorf("failed to create ACL manager from pinned maps: %v", err)
	}

	qosMgr, err := NewQoSManagerFromPinned()
	if err != nil {
		return nil, fmt.Errorf("failed to create QoS manager from pinned maps: %v", err)
	}

	return &FirewallQoSManager{
		aclMgr: aclMgr,
		qosMgr: qosMgr,
	}, nil
}

// ACL Methods
func (f *FirewallQoSManager) Apply5TupleACLRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8, action uint32) error {
	return f.aclMgr.Apply5TupleACLRule(srcIP, dstIP, srcPort, dstPort, protocol, action)
}

func (f *FirewallQoSManager) BlockPort(port int) error {
	return f.aclMgr.BlockPort(port)
}

func (f *FirewallQoSManager) BlockIP(ip string) error {
	return f.aclMgr.BlockIP(ip)
}

func (f *FirewallQoSManager) RemoveRule(srcIP, dstIP string, srcPort, dstPort int, protocol uint8) error {
	return f.aclMgr.RemoveRule(srcIP, dstIP, srcPort, dstPort, protocol)
}

// QoS Methods
func (f *FirewallQoSManager) LimitIP(ip string, mbps int) error {
	return f.qosMgr.LimitIP(ip, mbps)
}

func (f *FirewallQoSManager) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	return f.qosMgr.LimitPeerPair(srcIP, dstIP, mbps)
}

func (f *FirewallQoSManager) LimitPort(port int, mbps int) error {
	return f.qosMgr.LimitPort(port, mbps)
}

func (f *FirewallQoSManager) LimitService(srcIP, dstIP string, srcPort, dstPort, protocol int, mbps int) error {
	return f.qosMgr.LimitService(srcIP, dstIP, srcPort, dstPort, protocol, mbps)
}

func (f *FirewallQoSManager) LimitPortForIP(ip string, port int, mbps int, direction string) error {
	return f.qosMgr.LimitPortForIP(ip, port, mbps, direction)
}

// Remove Methods
func (f *FirewallQoSManager) RemoveIPLimit(ip string) error {
	return f.qosMgr.RemoveIPLimit(ip)
}

func (f *FirewallQoSManager) RemovePeerLimit(srcIP, dstIP string) error {
	return f.qosMgr.RemovePeerLimit(srcIP, dstIP)
}

func (f *FirewallQoSManager) RemoveServiceLimit(srcIP, dstIP string, srcPort, dstPort, protocol int) error {
	return f.qosMgr.RemoveServiceLimit(srcIP, dstIP, srcPort, dstPort, protocol)
}

// Stats Methods
func (f *FirewallQoSManager) GetIPStats(ip string) (*BucketState, error) {
	return f.qosMgr.GetIPStats(ip)
}

func (f *FirewallQoSManager) GetPeerStats(srcIP, dstIP string) (*BucketState, error) {
	return f.qosMgr.GetPeerStats(srcIP, dstIP)
}

func (f *FirewallQoSManager) GetServiceStats(srcIP, dstIP string, srcPort, dstPort, protocol int) (*BucketState, error) {
	return f.qosMgr.GetServiceStats(srcIP, dstIP, srcPort, dstPort, protocol)
}

// Update Interfaces
func (f *FirewallQoSManager) UpdateInterfaces(newIfaces []string) error {
	return f.qosMgr.UpdateInterfaces(newIfaces)
}

// Attach Programs
func (f *FirewallQoSManager) AttachXDP(interfaceName string) error {
	return f.aclMgr.AttachXDP(interfaceName)
}

func (f *FirewallQoSManager) AttachTC(interfaceName string) error {
	return f.qosMgr.AttachTC(interfaceName)
}

// Start Event Listeners
func (f *FirewallQoSManager) StartDropListeners() error {
	if err := f.aclMgr.StartDropListener(); err != nil {
		return fmt.Errorf("failed to start ACL drop listener: %v", err)
	}

	if err := f.qosMgr.StartDropListener(); err != nil {
		return fmt.Errorf("failed to start QoS drop listener: %v", err)
	}

	log.Println("Both ACL and QoS drop listeners started")
	return nil
}

// Close Resources
func (f *FirewallQoSManager) Close() error {
	err1 := f.aclMgr.Close()
	err2 := f.qosMgr.Close()

	if err1 != nil && err2 != nil {
		return fmt.Errorf("errors closing managers: ACL: %v, QoS: %v", err1, err2)
	} else if err1 != nil {
		return fmt.Errorf("error closing ACL manager: %v", err1)
	} else if err2 != nil {
		return fmt.Errorf("error closing QoS manager: %v", err2)
	}

	return nil
}

// Pin Maps
func (f *FirewallQoSManager) Pin() error {
	err1 := f.aclMgr.Pin()
	err2 := f.qosMgr.Pin()

	if err1 != nil && err2 != nil {
		return fmt.Errorf("errors pinning maps: ACL: %v, QoS: %v", err1, err2)
	} else if err1 != nil {
		return fmt.Errorf("error pinning ACL maps: %v", err1)
	} else if err2 != nil {
		return fmt.Errorf("error pinning QoS maps: %v", err2)
	}

	log.Println("All ACL and QoS maps pinned successfully")
	return nil
}

// Unpin Maps
func (f *FirewallQoSManager) Unpin() error {
	err1 := f.aclMgr.Unpin()
	err2 := f.qosMgr.Unpin()

	if err1 != nil && err2 != nil {
		return fmt.Errorf("errors unpinning maps: ACL: %v, QoS: %v", err1, err2)
	} else if err1 != nil {
		return fmt.Errorf("error unpinning ACL maps: %v", err1)
	} else if err2 != nil {
		return fmt.Errorf("error unpinning QoS maps: %v", err2)
	}

	log.Println("All ACL and QoS maps unpinned successfully")
	return nil
}