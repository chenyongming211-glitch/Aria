package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"aria/pkg/rpc"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Force immediate configuration sync",
	Long: `Trigger an immediate synchronization with the controller, bypassing the normal 30-second interval.

This command is useful when:
  - You just made changes on the controller
  - You want to test connectivity immediately
  - You suspect peer information is out of date

Examples:
  aria sync              # Sync configuration now
  aria sync --verbose    # Show detailed sync progress`,
	RunE: runSync,
}

var syncVerbose bool

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&syncVerbose, "verbose", false, "Show verbose output")
}

func runSync(cmd *cobra.Command, args []string) error {
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

	fmt.Print("Syncing configuration from controller... ")

	// Trigger sync
	resp, err := client.SyncConfig()
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Println("done")

	// Display results
	if syncVerbose || resp.PeersUpdated > 0 || resp.RoutesUpdated > 0 {
		if resp.PeersUpdated > 0 {
			fmt.Printf("✓ Updated %d peers\n", resp.PeersUpdated)
		}
		if resp.RoutesUpdated > 0 {
			fmt.Printf("✓ Updated %d routes\n", resp.RoutesUpdated)
		}
		fmt.Printf("Completed in %s\n", resp.Duration)
	}

	return nil
}
