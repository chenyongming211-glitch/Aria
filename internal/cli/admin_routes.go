package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"aria/pkg/controllerstorage"
)

var adminRoutesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Show routing information",
	Long: `Show routing information for the Aria network.

Displays:
  - Network configuration (base IP, CIDR)
  - IP allocation summary
  - Per-agent route advertisements

Examples:
  aria admin routes`,
	RunE: runAdminRoutes,
}

func init() {
	adminCmd.AddCommand(adminRoutesCmd)
}

func runAdminRoutes(cmd *cobra.Command, args []string) error {
	// Create storage
	store, err := controllerstorage.NewStorage(&controllerstorage.Config{
		Host:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnvOrDefault("POSTGRES_USER", "postgres"),
		Password: getEnvOrDefault("POSTGRES_PASSWORD", ""),
		Database: getEnvOrDefault("POSTGRES_DATABASE", "aria"),
		SSLMode:  getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
	}, "100.64.0.0", "100.64.0.0/16")
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer store.Close()

	// Get network configuration
	fmt.Println()
	fmt.Println("Network Configuration")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Base IP: %s\n", store.GetBaseIP())
	fmt.Printf("CIDR:    %s\n", store.GetCIDR())
	fmt.Println()

	// Get all nodes
	nodes, err := store.GetAllNodes()
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// IP Allocation Summary
	fmt.Println("IP Allocations")
	fmt.Println(strings.Repeat("-", 50))

	if len(nodes) == 0 {
		fmt.Println("No IP allocations.")
	} else {
		fmt.Printf("%-16s  %-20s  %-10s\n", "VPN IP", "HOSTNAME", "OFFSET")
		fmt.Println(strings.Repeat("-", 50))
		for _, node := range nodes {
			hostname := node.Hostname
			if len(hostname) > 20 {
				hostname = hostname[:17] + "..."
			}
			fmt.Printf("%-16s  %-20s  %-10d\n", node.AssignedIP, hostname, node.IPOffset)
		}
	}

	fmt.Println()
	fmt.Printf("Total allocations: %d\n", len(nodes))
	fmt.Println()

	// Route advertisements (if any)
	// Note: This is a placeholder - actual route advertisements would be
	// stored in a separate table or field
	fmt.Println("Advertised Routes")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println("(Route advertisements not yet configured)")
	fmt.Println()

	return nil
}
