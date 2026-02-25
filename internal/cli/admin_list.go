package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"aria/pkg/controllerstorage"
)

var adminListCmd = &cobra.Command{
	Use:   "list-agents",
	Short: "List all registered agents",
	Long: `List all agents registered with the controller.

Displays:
  - Device ID (public key)
  - Hostname
  - VPN IP address
  - Public IP address
  - Last seen timestamp
  - Status

Examples:
  aria admin list-agents
  aria admin list-agents --verbose`,
	RunE: runAdminList,
}

var adminListVerbose bool

func init() {
	adminCmd.AddCommand(adminListCmd)
	adminListCmd.Flags().BoolVarP(&adminListVerbose, "verbose", "v", false, "Show detailed information")
}

func runAdminList(cmd *cobra.Command, args []string) error {
	// Connect to database
	db, err := connectToDatabase()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create storage (using empty base IP and CIDR since we only need read access)
	store, err := controllerstorage.NewStorage(&controllerstorage.Config{
		Host:     getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Port:     5432,
		User:     getEnvOrDefault("POSTGRES_USER", "postgres"),
		Password: getEnvOrDefault("POSTGRES_PASSWORD", ""),
		Database: getEnvOrDefault("POSTGRES_DATABASE", "aria"),
		SSLMode:  getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
	}, "100.64.0.0", "100.64.0.0/16")
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}
	defer store.Close()

	// Get all nodes
	nodes, err := store.GetAllNodes()
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	if len(nodes) == 0 {
		fmt.Println("No agents registered.")
		return nil
	}

	// Print header
	fmt.Println()
	if adminListVerbose {
		fmt.Printf("%-16s  %-20s  %-16s  %-16s  %-20s  %-10s\n",
			"DEVICE ID", "HOSTNAME", "VPN IP", "PUBLIC IP", "LAST SEEN", "STATUS")
	} else {
		fmt.Printf("%-16s  %-20s  %-16s  %-20s\n",
			"DEVICE ID", "HOSTNAME", "VPN IP", "LAST SEEN")
	}
	fmt.Println(strings.Repeat("-", 90))

	// Print nodes
	now := time.Now().Unix()
	for _, node := range nodes {
		deviceID := node.PublicKey
		if len(deviceID) > 16 {
			deviceID = deviceID[:12] + "..."
		}

		hostname := node.Hostname
		if len(hostname) > 20 {
			hostname = hostname[:17] + "..."
		}

		lastSeen := formatLastSeen(now, node.LastSeen)

		status := "online"
		if now-node.LastSeen > 120 {
			status = "offline"
		}

		if adminListVerbose {
			fmt.Printf("%-16s  %-20s  %-16s  %-16s  %-20s  %-10s\n",
				deviceID, hostname, node.AssignedIP, node.PublicIP, lastSeen, status)
		} else {
			fmt.Printf("%-16s  %-20s  %-16s  %-20s\n",
				deviceID, hostname, node.AssignedIP, lastSeen)
		}
	}

	fmt.Println()
	fmt.Printf("Total: %d agent(s)\n", len(nodes))
	fmt.Println()

	return nil
}

func formatLastSeen(now, lastSeen int64) string {
	diff := now - lastSeen
	if diff < 60 {
		return fmt.Sprintf("%ds ago", diff)
	} else if diff < 3600 {
		return fmt.Sprintf("%dm ago", diff/60)
	} else if diff < 86400 {
		return fmt.Sprintf("%dh ago", diff/3600)
	}
	return time.Unix(lastSeen, 0).Format("2006-01-02 15:04")
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
