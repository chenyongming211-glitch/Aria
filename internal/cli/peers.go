package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"aria/pkg/rpc"
)

var peersCmd = &cobra.Command{
	Use:   "peers",
	Short: "Show all connected peers",
	Long: `Display information about all WireGuard peers currently connected to this node.

This command shows:
  - Hostname and region information
  - Connection status (handshake time)
  - Traffic statistics (RX/TX)
  - Network latency (if available)

Examples:
  aria peers              # Show all peers
  aria peers --json       # Output in JSON format`,
	RunE: runPeers,
}

var peersJSON bool

func init() {
	rootCmd.AddCommand(peersCmd)
	peersCmd.Flags().BoolVar(&peersJSON, "json", false, "Output in JSON format")
}

func runPeers(cmd *cobra.Command, args []string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// Create RPC client
	client := rpc.NewClient("")

	// Check if daemon is running
	if !client.IsServerRunning() {
		return fmt.Errorf("aria daemon is not running. Start it with: sudo systemctl start aria")
	}

	// Get peers
	resp, err := client.GetPeers()
	if err != nil {
		return fmt.Errorf("failed to get peers: %w", err)
	}

	// Output in JSON if requested
	if peersJSON {
		return outputJSON(resp)
	}

	// Format and print table
	printPeersTable(resp)

	return nil
}

func outputJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func printPeersTable(resp *rpc.GetPeersResponse) {
	if len(resp.Peers) == 0 {
		fmt.Println("No peers connected")
		return
	}

	// Create tabwriter for aligned columns
	// minwidth=0, tabwidth=4, padding=3 for better spacing
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)

	// Print header
	fmt.Fprintln(w, "NAME\tREGION\tIP\tENDPOINT\tHANDSHAKE\tRX/TX\tSTATUS")

	// Print separator matching columns
	fmt.Fprintln(w, "────────────────\t────────────\t───────────────\t───────────────────────\t────────────\t───────────────\t────────")

	// Print each peer
	for _, peer := range resp.Peers {
		status := peer.Status
		if status == "online" {
			status = "● online"
		} else if status == "offline" {
			status = "○ offline"
		} else {
			status = "○ " + status
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			padRight(truncate(peer.Hostname, 16), 16),
			padRight(truncate(peer.Region, 12), 12),
			padRight(peer.AssignedIP, 15),
			padRight(truncate(peer.Endpoint, 23), 23),
			padRight(formatHandshake(peer.LastHandshake, peer.Status), 12),
			padRight(formatTraffic(peer.RxBytes, peer.TxBytes), 15),
			status,
		)
	}

	w.Flush()

	// Print summary
	fmt.Println()
	fmt.Printf("%d peers total, %d active\n", resp.TotalCount, resp.ActiveCount)
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(s))
}

func formatHandshake(lastHandshake time.Time, status string) string {
	if status == "never" || lastHandshake.IsZero() {
		return "never"
	}

	elapsed := time.Since(lastHandshake)

	// Show warning if handshake is too old
	if elapsed > 3*time.Minute {
		return fmt.Sprintf("%s ⚠️", formatDuration(elapsed))
	}

	return formatDuration(elapsed)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func formatTraffic(rx, tx uint64) string {
	return fmt.Sprintf("%s/%s", formatBytes(rx), formatBytes(tx))
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatLatency(latency float64) string {
	if latency == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1fms", latency)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}
