package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"aria/pkg/rpc"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Display current monitoring metrics",
	Long: `Show real-time metrics including peer status, link quality, and system resources.

Examples:
  aria metrics              # Show summary
  aria metrics --verbose    # Show detailed peer metrics`,
	RunE: runMetrics,
}

var metricsVerbose bool

func init() {
	rootCmd.AddCommand(metricsCmd)
	metricsCmd.Flags().BoolVarP(&metricsVerbose, "verbose", "v", false, "Show verbose output with peer details")
}

func runMetrics(cmd *cobra.Command, args []string) error {
	client := rpc.NewClient("")

	if !client.IsServerRunning() {
		return fmt.Errorf("aria daemon is not running. Start it with 'aria up --daemon'")
	}

	resp, err := client.GetMetrics()
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	// 显示标题
	fmt.Printf("=== Aria Metrics (as of %s) ===\n\n", resp.Timestamp.Format("15:04:05"))

	// 显示摘要
	fmt.Printf("Peers:  %d total, %d healthy\n",
		resp.Summary.PeersTotal, resp.Summary.PeersHealthy)

	if resp.Summary.AvgRTT > 0 {
		fmt.Printf("RTT:    %.2f ms (avg)\n", resp.Summary.AvgRTT)
	}
	if resp.Summary.AvgLossRate > 0 {
		fmt.Printf("Loss:   %.2f%% (avg)\n", resp.Summary.AvgLossRate*100)
	}

	// 显示系统信息
	fmt.Printf("\nSystem:\n")
	if resp.System.CPUUsage > 0 {
		fmt.Printf("  CPU:        %.1f%%\n", resp.System.CPUUsage)
	}
	fmt.Printf("  Memory:     %.0f MB\n", resp.System.MemoryMB)
	fmt.Printf("  Goroutines: %d\n", resp.System.Goroutines)

	// 详细模式：显示每个 peer
	if metricsVerbose && len(resp.Peers) > 0 {
		fmt.Printf("\n=== Peer Details ===\n")
		fmt.Printf("%-20s %-22s %-12s %-10s %-12s %-12s\n",
			"Public Key", "Endpoint", "Status", "RTT", "Rx", "Tx")
		fmt.Println("----------------------------------------------------------------------------------------------------")

		for _, peer := range resp.Peers {
			status := "✓ healthy"
			if !peer.IsHealthy {
				status = "✗ stale"
			}

			rttStr := "-"
			if peer.RTT > 0 {
				rttStr = fmt.Sprintf("%.1f ms", peer.RTT)
			}

			rxStr := formatBytes(peer.RxBytes)
			txStr := formatBytes(peer.TxBytes)

			fmt.Printf("%-20s %-22s %-12s %-10s %-12s %-12s\n",
				peer.PublicKey, peer.Endpoint, status, rttStr, rxStr, txStr)
		}

		fmt.Printf("\nLast handshake times:\n")
		for _, peer := range resp.Peers {
			if peer.LastHandshake.IsZero() {
				fmt.Printf("  %s: never\n", peer.PublicKey)
			} else {
				fmt.Printf("  %s: %s\n", peer.PublicKey, formatDuration(time.Since(peer.LastHandshake)))
			}
		}
	}

	fmt.Println()
	return nil
}
