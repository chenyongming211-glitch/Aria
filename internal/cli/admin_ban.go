package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"aria/pkg/controllerstorage"
)

var adminBanCmd = &cobra.Command{
	Use:   "ban <device-id>",
	Short: "Ban a device from the network",
	Long: `Ban a device from the Aria network.

This command removes the device from the network and prevents it from
re-registering. The device will need a new token to rejoin.

You can specify the device by:
  - Public key (full or partial)
  - Hostname

Examples:
  aria admin ban abc123...xyz
  aria admin ban --hostname=server1`,
	RunE: runAdminBan,
}

var banHostname string

func init() {
	adminCmd.AddCommand(adminBanCmd)
	adminBanCmd.Flags().StringVar(&banHostname, "hostname", "", "Ban by hostname instead of device ID")
}

func runAdminBan(cmd *cobra.Command, args []string) error {
	if banHostname == "" && len(args) == 0 {
		return fmt.Errorf("device ID or --hostname is required")
	}

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

	var publicKey string
	var hostname string

	if banHostname != "" {
		// Find by hostname. Hostnames are not globally unique across tenants, so
		// refuse ambiguous matches instead of banning an arbitrary first row.
		nodes, err := store.GetAllNodes()
		if err != nil {
			return fmt.Errorf("failed to get nodes: %w", err)
		}
		var matchedNode *controllerstorage.Node
		matchCount := 0
		for _, node := range nodes {
			if node.Hostname == banHostname {
				matchCount++
				if matchedNode == nil {
					matchedNode = node
				}
			}
		}
		if matchCount > 1 {
			return fmt.Errorf("multiple devices use hostname %q, please ban by public key prefix", banHostname)
		}
		if matchedNode == nil {
			return fmt.Errorf("device not found with hostname: %s", banHostname)
		}
		publicKey = matchedNode.PublicKey
		hostname = matchedNode.Hostname
	} else {
		// Find by public key (partial match)
		deviceID := args[0]
		nodes, err := store.GetAllNodes()
		if err != nil {
			return fmt.Errorf("failed to get nodes: %w", err)
		}

		var matchedNode *controllerstorage.Node
		for _, node := range nodes {
			if strings.HasPrefix(node.PublicKey, deviceID) {
				if matchedNode != nil {
					return fmt.Errorf("multiple devices match prefix '%s', please provide more characters", deviceID)
				}
				matchedNode = node
			}
		}

		if matchedNode == nil {
			return fmt.Errorf("device not found with ID prefix: %s", deviceID)
		}
		publicKey = matchedNode.PublicKey
		hostname = matchedNode.Hostname
	}

	if _, err := store.ApplyNodeLifecycleTransition(publicKey, controllerstorage.NodeLifecycleTransition{
		TargetStatus:   "banned",
		RevokeReason:   "node banned by admin",
		AuditEventType: "node_banned",
		AuditActor:     "admin",
		AuditSummary:   "Node banned by admin",
	}); err != nil {
		return fmt.Errorf("failed to ban device: %w", err)
	}

	fmt.Printf("\nDevice banned successfully.\n")
	fmt.Printf("  Hostname: %s\n", hostname)
	fmt.Printf("  Device ID: %s...\n", publicKey[:16])
	fmt.Println()
	fmt.Println("The device will need a new token to rejoin the network.")
	fmt.Println()

	return nil
}
