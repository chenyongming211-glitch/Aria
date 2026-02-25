package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"aria/pkg/rpc"
)

var pingCmd = &cobra.Command{
	Use:   "ping <target>",
	Short: "Ping through the VPN tunnel",
	Long: `Send ICMP packets through the Aria VPN tunnel to test connectivity.

This command forces ping to use the Aria interface, ensuring packets
go through the VPN tunnel rather than the default route.

The target can be either:
  - A peer hostname (e.g., 'node-bj-01')
  - A VPN IP address (e.g., '100.64.0.2')

Examples:
  aria ping node-bj-01            # Ping by hostname
  aria ping 100.64.0.2            # Ping by IP
  aria ping node-sh-02 -c 10      # Send 10 packets
  aria ping 100.64.0.3 -i 0.5     # 0.5 second interval`,
	Args: cobra.ExactArgs(1),
	RunE: runPing,
}

var (
	pingCount    int
	pingInterval float64
)

func init() {
	rootCmd.AddCommand(pingCmd)
	pingCmd.Flags().IntVarP(&pingCount, "count", "c", 4, "Number of packets to send")
	pingCmd.Flags().Float64VarP(&pingInterval, "interval", "i", 1.0, "Interval between packets (seconds)")
}

func runPing(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// Create RPC client
	client := rpc.NewClient("")

	// Set longer timeout for ping
	client.SetTimeout(60 * 1000000000) // 60 seconds

	// Check if daemon is running
	if !client.IsServerRunning() {
		return fmt.Errorf("aria daemon is not running. Start it with: sudo systemctl start aria")
	}

	// Execute ping
	resp, err := client.Ping(target, pingCount, pingInterval)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Print output (even if empty, to help debug)
	if resp.Output != "" {
		fmt.Print(resp.Output)
	} else {
		fmt.Println("(no output from ping command)")
	}

	if !resp.Success {
		return fmt.Errorf("ping command failed")
	}

	return nil
}
