package cli

import (
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administrative commands",
	Long: `Administrative commands for managing the Aria network.

These commands should be run on the controller host (or via docker exec).

Commands:
  list-agents  - List all registered agents
  ban          - Ban a device from the network
  routes       - Show routing information

Examples:
  aria admin list-agents
  aria admin ban <device-id>
  aria admin routes`,
}

func init() {
	rootCmd.AddCommand(adminCmd)
}
