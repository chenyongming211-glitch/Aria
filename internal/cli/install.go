package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const agentServiceTemplate = `[Unit]
Description=Aria SD-WAN Agent
Documentation=https://github.com/example/aria
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s up --daemon --config=/etc/aria/agent.yaml
ExecStop=%s down
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aria

# Security hardening
NoNewPrivileges=false
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/aria /var/log/aria /etc/wireguard /etc/aria
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install aria as a systemd service",
	Long: `Install aria as a systemd service for automatic startup.

This command:
  1. Creates /etc/systemd/system/aria.service
  2. Enables the service for auto-start on boot
  3. Does NOT start the service (you need to run 'aria up' first to configure)

Prerequisites:
  - Must be run as root
  - Config must exist at /etc/aria/agent.yaml (run 'aria up' first)

Examples:
  # First, connect to the network
  aria up --server=https://controller:8080 --token=tk_xxx

  # Then install as service
  aria install

  # Start the service
  systemctl start aria`,
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// Get binary path
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Check if config exists
	if _, err := os.Stat("/etc/aria/agent.yaml"); os.IsNotExist(err) {
		fmt.Println("Warning: /etc/aria/agent.yaml not found")
		fmt.Println("Run 'aria up --server=... --token=...' first to create configuration")
		fmt.Println()
	}

	// Create service file
	serviceContent := fmt.Sprintf(agentServiceTemplate, binaryPath, binaryPath)
	serviceFilePath := "/etc/systemd/system/aria.service"

	if err := os.WriteFile(serviceFilePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}
	fmt.Printf("Created: %s\n", serviceFilePath)

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}
	fmt.Println("Systemd daemon reloaded")

	// Enable service
	if err := exec.Command("systemctl", "enable", "aria.service").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}
	fmt.Println("Service enabled")

	fmt.Println()
	fmt.Println("Aria service installed successfully!")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  systemctl start aria    # Start the service")
	fmt.Println("  systemctl stop aria     # Stop the service")
	fmt.Println("  systemctl status aria   # Check status")
	fmt.Println("  journalctl -u aria -f   # View logs")

	return nil
}
