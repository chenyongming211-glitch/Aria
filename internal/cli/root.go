package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	Version = "0.2.26-test-7" // 导出的版本号，通过 ldflags 注入
	commit  = "unknown" // Will be set by ldflags during build
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "aria",
	Short: "Aria - Decentralized SD-WAN System",
	Long: `Aria is a decentralized SD-WAN system that creates secure mesh networks
using WireGuard. It consists of a Controller (control plane) and Agents (edge nodes).

Roles:
  Controller  - Central brain, distributes routing and manages devices
  Gateway     - Edge node, tunnels traffic between sites
  Admin Tool  - Management commands for tokens and devices

Quick Start:
  aria up --server=https://controller:8080 --token=xxx   # Connect as gateway
  aria controller serve                                   # Run controller
  aria token create --uses=10 --ttl=24h                  # Create enrollment token`,
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/aria/agent.yaml or /etc/aria/controller.yaml)")
}
