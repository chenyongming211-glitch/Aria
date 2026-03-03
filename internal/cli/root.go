package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	Version = "0.2.26-test-7" // 导出的版本号，通过 ldflags 注入
	commit  = "unknown"       // Will be set by ldflags during build
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "aria-controller",
	Short: "Aria Controller - Decentralized SD-WAN Control Plane",
	Long: `Aria Controller is the control plane for the Aria SD-WAN system.

It manages network topology, distributes routing information, and provides
a web interface for monitoring and configuration.

Components:
  Controller  - Central brain (this binary)
  Agent       - Edge node (separate Rust binary: aria-agent)

Quick Start:
  aria-controller serve --config /etc/aria/controller.yaml   # Run controller
  aria-controller token create --uses=10 --ttl=24h          # Create enrollment token
  aria-controller admin list                                 # View all agents`,
	Version: Version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/aria/controller.yaml)")
}
