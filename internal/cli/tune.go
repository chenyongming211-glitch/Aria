package cli

import (
	"fmt"
	"os"

	"aria/pkg/nettuning"

	"github.com/spf13/cobra"
)

var tuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Optimize network performance for cloud VMs",
	Long: `Automatically detect and apply network optimizations for cloud VMs.

Optimizations include:
  - Sysctl tuning: TCP/UDP buffers (26MB), IP forwarding, conntrack (1M)
  - BBR congestion control: 2-10x better performance on lossy networks
  - Hardware multi-queue (RSS): Increase NIC queues to match CPU count
  - RPS/RFS: Software multi-queue for multi-core packet distribution
  - MSS clamping: Prevent PMTU black holes in tunnels
  - UDP offload (GRO/GSO): 20-50% throughput boost for WireGuard
  - QDisc (fq_codel/CAKE): Reduce latency jitter, prevent bufferbloat
  - NOTRACK: Bypass conntrack for WireGuard port (reduce CPU overhead)
  - Ring buffer: Maximize NIC buffer size to prevent packet drops

This command requires root privileges.`,
	RunE: runTune,
}

var (
	tuneInterface   string
	tuneVerbose     bool
	tuneStatus      bool
	tuneWireGuardPort int
	tuneExcludeCPU0 bool
)

func init() {
	tuneCmd.Flags().StringVarP(&tuneInterface, "interface", "i", "", "Physical network interface (auto-detect if not specified)")
	tuneCmd.Flags().BoolVarP(&tuneVerbose, "verbose", "v", false, "Show detailed output")
	tuneCmd.Flags().BoolVar(&tuneStatus, "status", false, "Show current tuning status")
	tuneCmd.Flags().IntVar(&tuneWireGuardPort, "wg-port", 51820, "WireGuard UDP port for NOTRACK optimization")
	tuneCmd.Flags().BoolVar(&tuneExcludeCPU0, "exclude-cpu0", false, "Exclude CPU 0 from RPS (for control plane separation on 8+ core systems)")

	rootCmd.AddCommand(tuneCmd)
}

func runTune(cmd *cobra.Command, args []string) error {
	// Auto-detect interface if not specified
	if tuneInterface == "" {
		tuneInterface = nettuning.AutoDetectPhysicalInterface()
	}

	tuner := nettuning.NewTuner(tuneInterface, "aria0")
	tuner.Verbose = tuneVerbose
	tuner.WireGuardPort = tuneWireGuardPort
	tuner.ExcludeCPU0 = tuneExcludeCPU0

	// If --status flag, just show status
	if tuneStatus {
		fmt.Println(tuner.GetStatus())
		return nil
	}

	// Check root
	if os.Geteuid() != 0 {
		return fmt.Errorf("network tuning requires root privileges (try: sudo aria tune)")
	}

	fmt.Println("Aria Network Performance Tuning")
	fmt.Println("================================")
	fmt.Printf("Physical interface: %s\n", tuneInterface)
	fmt.Printf("Tunnel interface:   aria0\n")
	fmt.Println()

	result, err := tuner.Tune()
	if err != nil {
		return err
	}

	// Print results
	fmt.Println("\nResults:")
	fmt.Println("--------")
	printResult("Sysctl tuning (buffers, forwarding)", result.SysctlTuned)
	printResult("BBR congestion control", result.BBREnabled)
	printResult("Hardware multi-queue (RSS)", result.MultiQueueOpt)
	printResult("RPS/RFS (softirq distribution)", result.RPSEnabled)
	printResult("MSS clamping (PMTU)", result.MSSClamped)
	printResult("UDP offload (GRO/GSO)", result.OffloadEnabled)
	printResult("QDisc (fq_codel/CAKE)", result.QdiscConfigured)
	printResult("NOTRACK (bypass conntrack)", result.NotrackEnabled)
	printResult("Ring buffer optimization", result.RingBufferOpt)

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
	}

	fmt.Println("\nSettings persisted to /etc/sysctl.d/99-aria.conf")
	fmt.Println("Run 'aria tune --status' to check current status")

	return nil
}

func printResult(name string, enabled bool) {
	if enabled {
		fmt.Printf("  ✓ %s: enabled\n", name)
	} else {
		fmt.Printf("  ✗ %s: not applied\n", name)
	}
}
