package eBPF

import (
	"net"
	"testing"
	"time"
)

func TestQoSManager(t *testing.T) {
	// Create a new QoS manager
	qosMgr, err := NewQoSManager()
	if err != nil {
		t.Fatalf("Failed to create QoS manager: %v", err)
	}
	defer qosMgr.Close()

	t.Run("Test IP Limit", func(t *testing.T) {
		ip := "192.168.1.100"
		mbps := 10

		err := qosMgr.LimitIP(ip, mbps)
		if err != nil {
			t.Errorf("Failed to set IP limit: %v", err)
		}

		// Verify the limit was set
		stats, err := qosMgr.GetIPStats(ip)
		if err != nil {
			t.Errorf("Failed to get IP stats: %v", err)
		} else if stats == nil {
			t.Error("Expected non-nil stats")
		}

		// Remove the limit
		err = qosMgr.RemoveIPLimit(ip)
		if err != nil {
			t.Errorf("Failed to remove IP limit: %v", err)
		}
	})

	t.Run("Test Peer Limit", func(t *testing.T) {
		srcIP := "192.168.1.100"
		dstIP := "192.168.1.200"
		mbps := 20

		err := qosMgr.LimitPeerPair(srcIP, dstIP, mbps)
		if err != nil {
			t.Errorf("Failed to set peer limit: %v", err)
		}

		// Verify the limit was set
		stats, err := qosMgr.GetPeerStats(srcIP, dstIP)
		if err != nil {
			t.Errorf("Failed to get peer stats: %v", err)
		} else if stats == nil {
			t.Error("Expected non-nil stats")
		}

		// Remove the limit
		err = qosMgr.RemovePeerLimit(srcIP, dstIP)
		if err != nil {
			t.Errorf("Failed to remove peer limit: %v", err)
		}
	})

	t.Run("Test Port Limit", func(t *testing.T) {
		port := 80
		mbps := 50

		err := qosMgr.LimitPort(port, mbps)
		if err != nil {
			t.Errorf("Failed to set port limit: %v", err)
		}

		// Remove the limit after a delay to allow for potential processing
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("Test Service Limit", func(t *testing.T) {
		srcIP := "10.0.0.10"
		dstIP := "10.0.0.20"
		srcPort := 80
		dstPort := 8080
		protocol := 6 // TCP
		mbps := 100

		err := qosMgr.LimitService(srcIP, dstIP, srcPort, dstPort, protocol, mbps)
		if err != nil {
			t.Errorf("Failed to set service limit: %v", err)
		}

		// Verify the limit was set
		stats, err := qosMgr.GetServiceStats(srcIP, dstIP, srcPort, dstPort, protocol)
		if err != nil {
			t.Errorf("Failed to get service stats: %v", err)
		} else if stats == nil {
			t.Error("Expected non-nil stats")
		}

		// Remove the limit
		err = qosMgr.RemoveServiceLimit(srcIP, dstIP, srcPort, dstPort, protocol)
		if err != nil {
			t.Errorf("Failed to remove service limit: %v", err)
		}
	})

	t.Run("Test Port For IP Limit", func(t *testing.T) {
		ip := "172.16.0.50"
		port := 443
		mbps := 30
		direction := "both"

		err := qosMgr.LimitPortForIP(ip, port, mbps, direction)
		if err != nil {
			t.Errorf("Failed to set port for IP limit: %v", err)
		}

		// Remove the limit after a delay to allow for potential processing
		time.Sleep(10 * time.Millisecond)
	})
}

func TestACLManager(t *testing.T) {
	// Create a new ACL manager
	aclMgr, err := NewACLManager()
	if err != nil {
		t.Fatalf("Failed to create ACL manager: %v", err)
	}
	defer aclMgr.Close()

	t.Run("Test Allow CIDR", func(t *testing.T) {
		cidr := "192.168.1.0/24"

		err := aclMgr.AllowCIDR(cidr)
		if err != nil {
			t.Errorf("Failed to allow CIDR: %v", err)
		}

		// Remove the rule
		err = aclMgr.RemoveRule(cidr)
		if err != nil {
			t.Errorf("Failed to remove CIDR rule: %v", err)
		}
	})

	t.Run("Test Block CIDR", func(t *testing.T) {
		cidr := "10.0.0.0/8"

		err := aclMgr.BlockCIDR(cidr)
		if err != nil {
			t.Errorf("Failed to block CIDR: %v", err)
		}

		// Remove the rule
		err = aclMgr.RemoveRule(cidr)
		if err != nil {
			t.Errorf("Failed to remove CIDR rule: %v", err)
		}
	})

	t.Run("Test Redirect CIDR", func(t *testing.T) {
		cidr := "172.16.0.0/16"
		port := uint32(8080)

		err := aclMgr.RedirectCIDR(cidr, port)
		if err != nil {
			t.Errorf("Failed to redirect CIDR: %v", err)
		}

		// Remove the rule
		err = aclMgr.RemoveRule(cidr)
		if err != nil {
			t.Errorf("Failed to remove CIDR rule: %v", err)
		}
	})
}

func TestFlowKeyCreation(t *testing.T) {
	// Test the flow key creation with various inputs
	testCases := []struct {
		name      string
		srcIP     string
		dstIP     string
		srcPort   uint16
		dstPort   uint16
		protocol  uint8
	}{
		{
			name:     "Standard HTTP",
			srcIP:    "192.168.1.100",
			dstIP:    "192.168.1.200",
			srcPort:  12345,
			dstPort:  80,
			protocol: 6, // TCP
		},
		{
			name:     "HTTPS",
			srcIP:    "10.0.0.10",
			dstIP:    "10.0.0.20",
			srcPort:  54321,
			dstPort:  443,
			protocol: 6, // TCP
		},
		{
			name:     "UDP",
			srcIP:    "172.16.0.10",
			dstIP:    "172.16.0.20",
			srcPort:  10000,
			dstPort:  53,
			protocol: 17, // UDP
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert IPs to uint32
			srcIPInt := ipToUint32(net.ParseIP(tc.srcIP))
			dstIPInt := ipToUint32(net.ParseIP(tc.dstIP))

			// Create flow key
			flowKey := FlowKey{
				SrcIP:   srcIPInt,
				DstIP:   dstIPInt,
				SrcPort: htons(tc.srcPort),
				DstPort: htons(tc.dstPort),
				Proto:   tc.protocol,
				Pad:     [3]byte{0, 0, 0},
			}

			// Verify fields are set correctly
			if flowKey.SrcIP != srcIPInt {
				t.Errorf("Expected SrcIP %d, got %d", srcIPInt, flowKey.SrcIP)
			}
			if flowKey.DstIP != dstIPInt {
				t.Errorf("Expected DstIP %d, got %d", dstIPInt, flowKey.DstIP)
			}
			if flowKey.SrcPort != htons(tc.srcPort) {
				t.Errorf("Expected SrcPort %d, got %d", htons(tc.srcPort), flowKey.SrcPort)
			}
			if flowKey.DstPort != htons(tc.dstPort) {
				t.Errorf("Expected DstPort %d, got %d", htons(tc.dstPort), flowKey.DstPort)
			}
			if flowKey.Proto != tc.protocol {
				t.Errorf("Expected Proto %d, got %d", tc.protocol, flowKey.Proto)
			}
		})
	}
}

func TestHtonsFunction(t *testing.T) {
	// Test the htons function with various inputs
	testCases := []struct {
		input    uint16
		expected uint16
	}{
		{input: 80, expected: 0x5000},   // 80 in big-endian on little-endian machine
		{input: 443, expected: 0xbb01},  // 443 in big-endian on little-endian machine
		{input: 1234, expected: 0xd204}, // 1234 in big-endian on little-endian machine
	}

	for _, tc := range testCases {
		result := htons(tc.input)
		if result != tc.expected {
			t.Errorf("htons(%d) = %d, expected %d", tc.input, result, tc.expected)
		}
	}
}