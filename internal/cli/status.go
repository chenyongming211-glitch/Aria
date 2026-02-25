package cli

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"aria/pkg/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Aria connection status",
	Long: `Show the current Aria connection status including:
  - WireGuard interface status
  - Controller connectivity
  - Peer information
  - Traffic statistics

Examples:
  aria status
  aria status --verbose`,
	RunE: runStatus,
}

var statusVerbose bool

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "Show detailed information")
}

func runStatus(cmd *cobra.Command, args []string) error {
	configPath := "/etc/aria/agent.yaml"
	if cfgFile != "" {
		configPath = cfgFile
	}

	configMgr := config.NewManager(configPath)

	// Check configuration
	fmt.Println()
	fmt.Println("Aria Status")
	fmt.Println(strings.Repeat("=", 50))

	if !configMgr.Exists() {
		fmt.Println("Status: NOT CONFIGURED")
		fmt.Println()
		fmt.Println("Run 'aria up --server=... --token=...' to connect.")
		fmt.Println()
		return nil
	}

	cfg, err := configMgr.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Device ID:   %s...\n", cfg.DeviceID[:16])
	fmt.Printf("VPN IP:      %s\n", cfg.AssignedIP)
	if cfg.Region != "" {
		fmt.Printf("Region:      %s\n", cfg.Region)
	}
	fmt.Printf("Controller:  %s\n", cfg.ControllerURL)
	fmt.Printf("Interface:   %s\n", cfg.Interface)
	fmt.Println()

	// Check WireGuard interface
	fmt.Println("WireGuard Interface")
	fmt.Println(strings.Repeat("-", 50))

	wgStatus, err := getWireGuardStatus(cfg.Interface)
	if err != nil {
		fmt.Printf("Status: DOWN (%v)\n", err)
	} else {
		fmt.Printf("Status: UP\n")
		if statusVerbose {
			fmt.Printf("Listen Port: %s\n", wgStatus.ListenPort)
			fmt.Printf("Public Key: %s...\n", wgStatus.PublicKey[:16])
		}
		fmt.Printf("Peers: %d\n", len(wgStatus.Peers))
	}
	fmt.Println()

	// Check controller connectivity
	fmt.Println("Controller Connectivity")
	fmt.Println(strings.Repeat("-", 50))

	ctrlStatus, latency, err := checkControllerStatus(cfg.ControllerURL, cfg.PublicKey)
	if err != nil {
		fmt.Printf("Status: UNREACHABLE (%v)\n", err)
	} else {
		fmt.Printf("Status: CONNECTED\n")
		fmt.Printf("Latency: %s\n", latency.Round(time.Millisecond))
		if statusVerbose {
			fmt.Printf("Peers Available: %d\n", ctrlStatus.PeerCount)
		}
	}
	fmt.Println()

	// Show peer details in verbose mode
	if statusVerbose && wgStatus != nil && len(wgStatus.Peers) > 0 {
		fmt.Println("Peer Details")
		fmt.Println(strings.Repeat("-", 50))
		for i, peer := range wgStatus.Peers {
			fmt.Printf("\n[Peer %d]\n", i+1)
			fmt.Printf("  Public Key: %s...\n", peer.PublicKey[:16])
			fmt.Printf("  Endpoint: %s\n", peer.Endpoint)
			fmt.Printf("  Allowed IPs: %s\n", peer.AllowedIPs)
			if peer.LastHandshake != "" {
				fmt.Printf("  Last Handshake: %s\n", peer.LastHandshake)
			}
			if peer.Transfer != "" {
				fmt.Printf("  Transfer: %s\n", peer.Transfer)
			}
		}
		fmt.Println()
	}

	return nil
}

type wgStatusInfo struct {
	PublicKey  string
	ListenPort string
	Peers      []wgPeerInfo
}

type wgPeerInfo struct {
	PublicKey     string
	Endpoint      string
	AllowedIPs    string
	LastHandshake string
	Transfer      string
}

func getWireGuardStatus(ifaceName string) (*wgStatusInfo, error) {
	cmd := exec.Command("wg", "show", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("interface not found or not running")
	}

	status := &wgStatusInfo{}
	lines := strings.Split(string(output), "\n")

	var currentPeer *wgPeerInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "public key:") {
			if currentPeer == nil {
				status.PublicKey = strings.TrimPrefix(line, "public key: ")
			}
		} else if strings.HasPrefix(line, "listening port:") {
			status.ListenPort = strings.TrimPrefix(line, "listening port: ")
		} else if strings.HasPrefix(line, "peer:") {
			if currentPeer != nil {
				status.Peers = append(status.Peers, *currentPeer)
			}
			currentPeer = &wgPeerInfo{
				PublicKey: strings.TrimPrefix(line, "peer: "),
			}
		} else if currentPeer != nil {
			if strings.HasPrefix(line, "endpoint:") {
				currentPeer.Endpoint = strings.TrimPrefix(line, "endpoint: ")
			} else if strings.HasPrefix(line, "allowed ips:") {
				currentPeer.AllowedIPs = strings.TrimPrefix(line, "allowed ips: ")
			} else if strings.HasPrefix(line, "latest handshake:") {
				currentPeer.LastHandshake = strings.TrimPrefix(line, "latest handshake: ")
			} else if strings.HasPrefix(line, "transfer:") {
				currentPeer.Transfer = strings.TrimPrefix(line, "transfer: ")
			}
		}
	}

	if currentPeer != nil {
		status.Peers = append(status.Peers, *currentPeer)
	}

	return status, nil
}

type ctrlStatusInfo struct {
	PeerCount int
}

func checkControllerStatus(controllerURL, publicKey string) (*ctrlStatusInfo, time.Duration, error) {
	syncURL := fmt.Sprintf("%s/sync?pubkey=%s", controllerURL, url.QueryEscape(publicKey))

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}

	start := time.Now()
	resp, err := client.Get(syncURL)
	latency := time.Since(start)

	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, latency, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var syncResp upSyncResponseWithPeers
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return nil, latency, err
	}

	return &ctrlStatusInfo{
		PeerCount: len(syncResp.Peers),
	}, latency, nil
}

// Helper function to check if WireGuard interface exists
func interfaceExists(name string) bool {
	_, err := os.Stat(fmt.Sprintf("/sys/class/net/%s", name))
	return err == nil
}
