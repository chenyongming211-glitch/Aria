package cli

import (
	"github.com/spf13/cobra"
)

var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Controller management commands",
	Long: `Commands for managing the Aria Controller.

The Controller is the central brain of the Aria network:
  - Manages device registration and authentication
  - Distributes routing information to agents
  - Maintains the network topology

Commands:
  init   - Initialize controller configuration
  serve  - Start the controller service

Examples:
  aria controller init --mode=local
  aria controller serve --config=/etc/aria/controller.yaml`,
}

func init() {
	rootCmd.AddCommand(controllerCmd)
}
