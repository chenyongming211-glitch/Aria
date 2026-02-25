package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/spf13/cobra"
	"aria/internal/eBPF" // Adjust import path according to your project structure
)

// Global variables for eBPF objects
var objs *eBPF.AriaIngressObjects

// IPv4ToUint32 converts an IPv4 address string to uint32 using network byte order (BigEndian)
func IPv4ToUint32(ipStr string) uint32 {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Fatalf("Invalid IP address: %s", ipStr)
		return 0
	}

	// Use To4() to ensure we have a 4-byte IPv4 address
	ip4 := ip.To4()
	if ip4 == nil {
		log.Fatalf("Not a valid IPv4 address: %s", ipStr)
		return 0
	}

	// Convert to uint32 using BigEndian
	return binary.BigEndian.Uint32(ip4)
}

// CalculateParams calculates rate and burst from mbps using the specified formulas
func CalculateParams(mbps float64) (uint64, uint64) {
	rate := uint64((mbps * 1000000) / 8) // Convert Mbps to Bytes per second
	burst := uint64((mbps * 1000000 * 20) / (8 * 1000)) // 20ms burst: (Mbps * 10^6 * 20ms) / (8 * 1000ms)
	return rate, burst
}

// Initialize eBPF objects
func initEbpf() error {
	objs = &eBPF.AriaIngressObjects{}
	err := eBPF.LoadAriaIngressObjects(objs, &ebpf.CollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to load eBPF objects: %v", err)
	}
	return nil
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "aria-cli",
	Short: "Aria CLI tool for managing eBPF firewall and QoS",
	Long:  "Command-line interface for managing Aria v3.0 eBPF Firewall & QoS System",
}

// QoS command group
var qosCmd = &cobra.Command{
	Use:   "qos",
	Short: "QoS management commands",
	Long:  "Commands for managing Quality of Service rules",
}

// QoS add command
var qosAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a QoS rule",
	Long:  "Add a new QoS rule for bandwidth limiting",
	Run: func(cmd *cobra.Command, args []string) {
		ip, _ := cmd.Flags().GetString("ip")
		rate, _ := cmd.Flags().GetInt("rate")
		id, _ := cmd.Flags().GetInt("id")

		if ip == "" {
			fmt.Println("Error: --ip is required")
			os.Exit(1)
		}

		if rate <= 0 {
			fmt.Println("Error: --rate must be greater than 0")
			os.Exit(1)
		}

		// Initialize eBPF objects
		if err := initEbpf(); err != nil {
			log.Fatalf("Failed to initialize eBPF: %v", err)
		}
		defer objs.Close()

		// Build ACL5TupleKey with only destination IP filled, others set to 0
		key := eBPF.ACL5TupleKey{
			SrcIP:   0,
			DstIP:   IPv4ToUint32(ip),
			SrcPort: 0,
			DstPort: 0,
			Proto:   0,
			Pad1:    0,
			Pad2:    0,
		}

		// Calculate rate and burst from mbps
		rateBytes, burstBytes := CalculateParams(float64(rate))

		// Build BucketState structure with 4-byte placeholder for bpf_spin_lock
		bucket := eBPF.BucketState{
			RateBytesPerSec: rateBytes,
			BurstBytes:      burstBytes,
			Tokens:          burstBytes, // Initialize tokens to burst size
			LastUpdateNS:    uint64(time.Now().UnixNano()),
			PassBytes:       0,
			DropBytes:       0,
			RuleID:          uint32(id), // Rule ID at the end of the structure
		}

		// Update the AppQoSMap with the new rule
		err := objs.AppQosMap.Update(key, bucket, ebpf.UpdateAny)
		if err != nil {
			log.Fatalf("Failed to update AppQoSMap: %v", err)
		}

		fmt.Printf("Successfully added QoS rule: IP=%s, Rate=%d Mbps, ID=%d\n", ip, rate, id)
	},
}

// Monitor command
var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor real-time statistics",
	Long:  "Monitor real-time throughput statistics from eBPF maps",
}

// Live monitor command
var monitorLiveCmd = &cobra.Command{
	Use:   "live",
	Short: "Live monitoring of real-time throughput",
	Long:  "Display live throughput statistics by aggregating PERCPU map values",
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize eBPF objects
		if err := initEbpf(); err != nil {
			log.Fatalf("Failed to initialize eBPF: %v", err)
		}
		defer objs.Close()

		// Prepare for storing last values for calculating throughput
		lastValues := make(map[eBPF.FlowDetailKey]uint64)

		fmt.Println("Starting live monitoring...")
		fmt.Printf("%-30s %-15s %-15s %-15s\n", "Flow", "Last Bytes", "Current Bytes", "Throughput (BPS)")
		fmt.Println(strings.Repeat("-", 90))

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Get all entries from RuleFlowTable
			iter := objs.RuleFlowTable.Iterate()
			var key eBPF.FlowDetailKey
			var values []eBPF.FlowDetailStats

			for iter.Next(&key, &values) {
				// Aggregate values from all CPUs for this key
				totalBytes := uint64(0)
				for _, value := range values {
					totalBytes += value.Bytes
				}

				// Calculate throughput
				throughput := uint64(0)
				lastBytes, exists := lastValues[key]

				if exists {
					// Calculate difference over 1 second
					if totalBytes >= lastBytes {
						throughput = totalBytes - lastBytes
					}
				}

				// Store current value for next iteration
				lastValues[key] = totalBytes

				// Format flow info
				flowInfo := fmt.Sprintf("%s:%d->%s:%d",
					intToIP(key.SrcIP), ntohs(uint16(key.SrcPort)),
					intToIP(key.DstIP), ntohs(uint16(key.DstPort)))

				fmt.Printf("%-30s %-15d %-15d %-15d\n",
					flowInfo, lastBytes, totalBytes, throughput)
			}

			// Add a separator after each iteration
			fmt.Println(strings.Repeat("-", 90))
		}
	},
}

// Helper function to convert uint32 IP to dotted decimal string
func intToIP(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(ip>>24),
		byte(ip>>16),
		byte(ip>>8),
		byte(ip))
}

// Helper function to convert network byte order to host byte order for ports
func ntohs(port uint16) uint16 {
	return (port<<8)&0xff00 | port>>8
}

func main() {
	// Add flags to qosAddCmd
	qosAddCmd.Flags().StringP("ip", "i", "", "IP address for the QoS rule")
	qosAddCmd.Flags().IntP("rate", "r", 0, "Rate in Mbps")
	qosAddCmd.Flags().IntP("id", "d", 0, "Rule ID")

	// Add subcommands to qosCmd
	qosCmd.AddCommand(qosAddCmd)

	// Add subcommands to monitorCmd
	monitorCmd.AddCommand(monitorLiveCmd)

	// Add subcommands to rootCmd
	rootCmd.AddCommand(qosCmd)
	rootCmd.AddCommand(monitorCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}