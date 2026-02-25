package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"aria/pkg/capability"
	"aria/pkg/config"
	"aria/pkg/wgmanager"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize and register with the Aria Controller",
	Long: `Initialize the Aria Agent and register with the Controller.

This command only performs registration and saves configuration.
It does NOT start the tunnel or connect to the network.

After initialization, use 'aria up' to start the tunnel.

Examples:
  # First-time initialization
  aria init --server=https://controller:8443 --token=tk_xxxxxxxx --region=sh

  # Re-initialize with new settings
  aria init --server=https://new-controller:8443 --token=tk_xxxxxx --region=bj --force`,
	RunE: runInit,
}

var (
	initServer          string
	initToken           string
	initRegion          string
	initCustomerID     string
	initAdvertiseRoutes string
	initForce          bool
	initCACert         string
)

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initServer, "server", "", "Controller URL (required)")
	initCmd.Flags().StringVar(&initToken, "token", "", "Enrollment token (required for first-time)")
	initCmd.Flags().StringVar(&initRegion, "region", "", "Region identifier (e.g., cn-shanghai, us-west)")
	initCmd.Flags().StringVar(&initCustomerID, "customer-id", "", "Customer identifier")
	initCmd.Flags().StringVar(&initAdvertiseRoutes, "advertise-routes", "", "Routes to advertise (comma-separated)")
	initCmd.Flags().StringVar(&initCACert, "ca-cert", "", "CA certificate path for TLS verification")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Force re-initialization even if config exists")
}

func runInit(cmd *cobra.Command, args []string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// Validate required parameters
	if initServer == "" {
		return fmt.Errorf("--server is required")
	}
	if initToken == "" {
		return fmt.Errorf("--token is required")
	}

	configPath := "/etc/aria/agent.yaml"
	configMgr := config.NewManager(configPath)

	// Check existing config
	if configMgr.Exists() && !initForce {
		return fmt.Errorf("configuration already exists. Use --force to re-initialize")
	}

	if initForce && configMgr.Exists() {
		fmt.Println("Force mode: removing existing configuration...")
		configMgr.Delete()
	}

	// Ensure directories exist
	os.MkdirAll("/etc/aria", 0755)
	os.MkdirAll("/var/lib/aria", 0755)
	os.MkdirAll("/var/log/aria", 0755)

	// Detect system capabilities
	fmt.Print("Detecting system capabilities... ")
	detector := capability.NewDetector()
	sysInfo, err := detector.Detect()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
		sysInfo = &capability.SystemInfo{RuntimeMode: capability.ModeUserspace}
	}
	fmt.Println("done")
	fmt.Printf("  Runtime mode: %s\n", sysInfo.RuntimeMode)
	fmt.Printf("  Kernel version: %s\n", sysInfo.KernelVersion)
	fmt.Printf("  AES-NI: %v\n", sysInfo.HasAESNI)

	// Generate WireGuard keys
	fmt.Print("Generating WireGuard keys... ")
	privateKey, publicKey, err := generateKeys()
	if err != nil {
		fmt.Printf("failed: %v\n", err)
		return fmt.Errorf("failed to generate keys: %w", err)
	}
	fmt.Println("done")

	// Get environment info
	fmt.Print("Detecting environment... ")
	privateIP := getLocalIPAddr()
	publicIP := getPublicIPAddr()
	hostname, _ := os.Hostname()
	fmt.Println("done")
	fmt.Printf("  Hostname: %s\n", hostname)
	fmt.Printf("  Private IP: %s\n", privateIP)
	fmt.Printf("  Public IP: %s\n", publicIP)

	// Parse advertised routes
	var advertisedRoutesArray []string
	if initAdvertiseRoutes != "" {
		for _, route := range splitAndTrim(initAdvertiseRoutes, ",") {
			if route != "" {
				advertisedRoutesArray = append(advertisedRoutesArray, route)
			}
		}
	}

	// Register with controller
	fmt.Print("Registering with controller... ")
	assignedIP, metricsGateway, err := registerWithCtrl(initServer, initToken, publicKey, privateIP, publicIP, hostname, advertisedRoutesArray, initRegion, initCustomerID, sysInfo, initCACert)
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("registration failed: %w", err)
	}
	fmt.Println("done")
	fmt.Printf("  Assigned VPN IP: %s\n", assignedIP)

	// Determine interface name
	interfaceName := wgmanager.DefaultInterfaceName
	if runtime.GOOS == "darwin" {
		interfaceName = wgmanager.DefaultMacOSInterface
	}

	// Fix metrics push gateway
	metricsPushGateway := fixMetricsGateway(metricsGateway, initServer)

	// Save configuration
	agentConfig := &config.AgentConfig{
		ControllerURL:    initServer,
		DeviceID:        publicKey,
		PrivateKey:      privateKey,
		PublicKey:       publicKey,
		AssignedIP:      assignedIP,
		LogLevel:        "info",
		Interface:       interfaceName,
		StorageDir:      "/var/lib/aria",
		RuntimeMode:     string(sysInfo.RuntimeMode),
		Region:          initRegion,
		CustomerID:      initCustomerID,
		AdvertisedRoutes: advertisedRoutesArray,
		CACert:          initCACert,
		Metrics: config.MetricsConfig{
			Enabled:      true,
			ListenAddr:  ":9090",
			PushGateway: metricsPushGateway,
		},
	}

	if err := configMgr.Save(agentConfig); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("\n✓ Initialization complete!")
	fmt.Println("  Use 'aria up' to start the tunnel")
	fmt.Println("  Use 'systemctl enable --now aria' to start automatically on boot")

	return nil
}

// splitAndTrim splits a string by separator and trims whitespace
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}
