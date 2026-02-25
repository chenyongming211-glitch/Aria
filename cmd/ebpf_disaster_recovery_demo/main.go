package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	firewall "aria/internal/agent/firewall"
)

func main() {
	fmt.Println("🚀 Starting Aria Agent with 3-Level Disaster Recovery System...")

	// Initialize eBPF adapter with snapshot capability
	// This will attempt recovery in the following order:
	// 1. Try to recover from pinned maps (Level 1: Process crash survival)
	// 2. Try to load from snapshot (Level 2: Server restart survival)
	// 3. Fall back to default policy (Level 3: Data corruption protection)
	adapter, err := firewall.NewEBPFAdapterWithSnapshots("/var/lib/aria/ebpf_snapshot.json")
	if err != nil {
		log.Fatalf("❌ Failed to create eBPF adapter: %v", err)
	}
	defer func() {
		fmt.Println("🛑 Shutting down eBPF adapter...")
		if err := adapter.Close(); err != nil {
			log.Printf("Warning: failed to close adapter: %v", err)
		}
	}()

	// Pin the maps to survive process crashes (Level 1: Map Pinning)
	fmt.Println("📍 Pinning eBPF maps for process crash recovery...")
	if err := adapter.PinMaps(); err != nil {
		log.Printf("⚠️ Warning: Failed to pin maps: %v", err)
	} else {
		fmt.Println("✅ Maps pinned successfully - will survive process crashes")
	}

	// Apply some example configurations
	fmt.Println("\n📝 Applying example configurations...")

	// Example: Block a specific IP
	blockIP := "192.168.1.100"
	if err := adapter.BlockIP(blockIP); err != nil {
		log.Printf("Failed to block IP %s: %v", blockIP, err)
	} else {
		fmt.Printf("✅ Successfully blocked IP: %s\n", blockIP)
	}

	// Example: Limit bandwidth for an IP to 10 Mbps
	limitIP := "10.0.0.50"
	mbps := 10
	if err := adapter.LimitIP(limitIP, mbps); err != nil {
		log.Printf("Failed to limit IP %s to %d Mbps: %v", limitIP, mbps, err)
	} else {
		fmt.Printf("✅ Successfully limited IP %s to %d Mbps\n", limitIP, mbps)
	}

	// Example: Limit bandwidth between two IPs to 5 Mbps
	srcIP := "10.0.0.10"
	dstIP := "10.0.0.20"
	mbps = 5
	if err := adapter.LimitPeerPair(srcIP, dstIP, mbps); err != nil {
		log.Printf("Failed to limit peer pair %s->%s to %d Mbps: %v", srcIP, dstIP, mbps, err)
	} else {
		fmt.Printf("✅ Successfully limited peer pair %s->%s to %d Mbps\n", srcIP, dstIP, mbps)
	}

	// Example: Apply port-based QoS rule
	port := 80
	mbps = 100
	if err := adapter.LimitPort(port, mbps); err != nil {
		log.Printf("Failed to limit port %d to %d Mbps: %v", port, mbps, err)
	} else {
		fmt.Printf("✅ Successfully limited port %d to %d Mbps\n", port, mbps)
	}

	// Update network interfaces for eBPF programs
	interfaces := []string{"wg0", "aria0", "eth0"} // Common Aria interface names
	if err := adapter.UpdateNetworkInterfaces(interfaces); err != nil {
		log.Printf("Failed to update network interfaces: %v", err)
	} else {
		fmt.Printf("✅ Successfully updated network interfaces: %v\n", interfaces)
	}

	fmt.Println("\n📊 Current Configuration Status:")
	fmt.Println("   • Map Pinning: ACTIVE (Survives process crashes)")
	fmt.Println("   • Local Snapshot: ACTIVE (Survives server restarts)")
	fmt.Println("   • Default Policy: AVAILABLE (Survives data corruption)")

	// Show how to retrieve stats
	fmt.Println("\n📈 Sample Statistics:")
	if stats, err := adapter.GetIPStats(limitIP); err != nil {
		log.Printf("Failed to get stats for IP %s: %v", limitIP, err)
	} else {
		fmt.Printf("   Stats for IP %s: %+v\n", limitIP, stats)
	}

	if stats, err := adapter.GetPeerStats(srcIP, dstIP); err != nil {
		log.Printf("Failed to get stats for peer pair %s->%s: %v", srcIP, dstIP, err)
	} else {
		fmt.Printf("   Stats for peer pair %s->%s: %+v\n", srcIP, dstIP, stats)
	}

	// Wait for interrupt signal to gracefully shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	fmt.Printf("\n✅ Aria eBPF firewall is running with 3-level disaster recovery.\n")
	fmt.Printf("   • Level 1: Map Pinning - Survives process crashes\n")
	fmt.Printf("   • Level 2: Local Snapshot - Survives server restarts\n")
	fmt.Printf("   • Level 3: Default Policy - Survives data corruption\n")
	fmt.Printf("\n⏳ Waiting for signal to exit (Ctrl+C to stop)...\n")
	<-sigChan

	fmt.Println("\n👋 Gracefully shutting down...")
}