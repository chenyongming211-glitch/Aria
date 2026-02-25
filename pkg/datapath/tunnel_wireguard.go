package datapath

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	// DefaultInterfaceName is the default WireGuard interface name for Linux.
	DefaultInterfaceName = "aria0"

	// DefaultMacOSInterface is the interface name prefix for macOS.
	DefaultMacOSInterface = "utun"

	// DefaultListenPort is the default WireGuard UDP port.
	DefaultListenPort = 51820

	// DefaultMTU is the default MTU for WireGuard interfaces.
	// Set to 1360 for optimal performance in cloud environments
	DefaultMTU = 1360
)

// WireGuardTunnelManager implements TunnelManager using WireGuard.
type WireGuardTunnelManager struct {
	interfaceName       string
	actualInterfaceName string
	privateKey          string
	publicKey           string
	runtimeMode         RuntimeMode
}

// NewWireGuardTunnelManager creates a new WireGuard tunnel manager.
func NewWireGuardTunnelManager(interfaceName string, mode RuntimeMode) *WireGuardTunnelManager {
	if interfaceName == "" {
		interfaceName = DefaultInterfaceName
		if runtime.GOOS == "darwin" {
			interfaceName = DefaultMacOSInterface
		}
	}
	if mode == "" {
		mode = ModeKernel
	}
	return &WireGuardTunnelManager{
		interfaceName: interfaceName,
		runtimeMode:   mode,
	}
}

// EnsureInterface creates or updates the WireGuard interface.
func (m *WireGuardTunnelManager) EnsureInterface(cfg *InterfaceConfig) error {
	if cfg == nil {
		return fmt.Errorf("interface config is required")
	}

	// Set defaults
	if cfg.ListenPort == 0 {
		cfg.ListenPort = DefaultListenPort
	}
	if cfg.MTU == 0 {
		cfg.MTU = DefaultMTU
	}
	if cfg.RuntimeMode == "" {
		cfg.RuntimeMode = m.runtimeMode
	}

	m.privateKey = cfg.PrivateKey

	// Calculate public key
	if cfg.PrivateKey != "" {
		pubKey, err := derivePublicKey(cfg.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to derive public key: %w", err)
		}
		m.publicKey = pubKey
	}

	if cfg.RuntimeMode == ModeUserspace || runtime.GOOS == "darwin" {
		return m.ensureUserspaceInterface(cfg)
	}
	return m.ensureKernelInterface(cfg)
}

// ensureKernelInterface creates WireGuard interface using kernel module.
// Modified to be idempotent: only create if not exists, preserve if exists
func (m *WireGuardTunnelManager) ensureKernelInterface(cfg *InterfaceConfig) error {
	// Check if interface already exists (supports aria0, aria1, aria2, etc.)
	link, err := netlink.LinkByName(m.interfaceName)
	if err == nil {
		// Interface exists - check if it's a WireGuard interface
		if _, ok := link.(*netlink.Wireguard); ok {
			// Interface already exists as WireGuard - adopt it (no recreation!)
			fmt.Printf("WireGuard interface %s already exists, adopting it\n", m.interfaceName)

			// Update actual interface name
			m.actualInterfaceName = m.interfaceName

			// Configure WireGuard with new settings (key, port, etc.)
			// This updates the existing interface without recreating it
			client, err := wgctrl.New()
			if err != nil {
				return fmt.Errorf("failed to open wgctrl: %w", err)
			}
			defer client.Close()

			key, err := wgtypes.ParseKey(cfg.PrivateKey)
			if err != nil {
				return ErrInvalidKey
			}

			wgCfg := wgtypes.Config{
				PrivateKey: &key,
				ListenPort: &cfg.ListenPort,
			}

			if err := client.ConfigureDevice(m.interfaceName, wgCfg); err != nil {
				return fmt.Errorf("failed to configure WireGuard: %w", err)
			}

			// Ensure interface is up
			if err := netlink.LinkSetUp(link); err != nil {
				return fmt.Errorf("failed to set interface up: %w", err)
			}

			// Enable IP forwarding
			if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
				fmt.Printf("Warning: failed to enable IP forwarding: %v\n", err)
			}

			return nil
		}
		// Different type of interface with same name - delete it
		netlink.LinkDel(link)
	}

	// If we reach here, interface doesn't exist - check if another aria* interface already has our desired port
	// This handles the case where aria0 was created but we're trying to create aria1 with the same port
	existingPorts, err := getExistingAriaPorts()
	if err == nil && len(existingPorts) > 0 {
		for port, ifaceName := range existingPorts {
			if port == cfg.ListenPort {
				fmt.Printf("Warning: Port %d is already in use by interface %s\n", port, ifaceName)
			}
		}
	}

	// Create WireGuard interface
	wg := &netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: m.interfaceName,
		},
	}

	if err := netlink.LinkAdd(wg); err != nil {
		return fmt.Errorf("failed to create WireGuard interface: %w", err)
	}

	// Set MTU
	link, err = netlink.LinkByName(m.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface: %w", err)
	}

	if err := netlink.LinkSetMTU(link, cfg.MTU); err != nil {
		return fmt.Errorf("failed to set MTU: %w", err)
	}

	// Bring interface up
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set interface up: %w", err)
	}

	// Configure WireGuard
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %w", err)
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return ErrInvalidKey
	}

	wgCfg := wgtypes.Config{
		PrivateKey: &key,
		ListenPort: &cfg.ListenPort,
	}

	if err := client.ConfigureDevice(m.interfaceName, wgCfg); err != nil {
		return fmt.Errorf("failed to configure WireGuard: %w", err)
	}

	// Enable IP forwarding
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		// Non-fatal, just log
		fmt.Printf("Warning: failed to enable IP forwarding: %v\n", err)
	}

	m.actualInterfaceName = m.interfaceName
	return nil
}

// ensureUserspaceInterface creates WireGuard interface using wireguard-go.
func (m *WireGuardTunnelManager) ensureUserspaceInterface(cfg *InterfaceConfig) error {
	actualIfaceName := m.interfaceName

	// Get existing utun interfaces (for macOS)
	var existingUtuns []string
	if runtime.GOOS == "darwin" {
		output, _ := exec.Command("ifconfig", "-l").Output()
		for _, iface := range strings.Fields(string(output)) {
			if strings.HasPrefix(iface, "utun") {
				existingUtuns = append(existingUtuns, iface)
			}
		}
	}

	// Start wireguard-go
	cmd := exec.Command("wireguard-go", m.interfaceName)
	cmd.Env = append(os.Environ(), "WG_I_PREFER_BUGGY_USERSPACE_TO_POLISHED_KMOD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start wireguard-go: %w, output: %s", err, output)
	}

	// macOS: Find the newly created utun interface
	if runtime.GOOS == "darwin" {
		for i := 0; i < 10; i++ {
			output, _ := exec.Command("ifconfig", "-l").Output()
			for _, iface := range strings.Fields(string(output)) {
				if strings.HasPrefix(iface, "utun") {
					isNew := true
					for _, existing := range existingUtuns {
						if iface == existing {
							isNew = false
							break
						}
					}
					if isNew {
						actualIfaceName = iface
						break
					}
				}
			}
			if actualIfaceName != m.interfaceName {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Wait for interface
	if err := m.waitForInterface(actualIfaceName); err != nil {
		return err
	}

	// Set MTU
	if err := m.setInterfaceMTU(actualIfaceName, cfg.MTU); err != nil {
		return err
	}

	// Set private key and listen port
	cmd = exec.Command("wg", "set", actualIfaceName, "private-key", "/dev/stdin", "listen-port", fmt.Sprintf("%d", cfg.ListenPort))
	cmd.Stdin = strings.NewReader(cfg.PrivateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	// Bring interface up (Linux only, macOS utun is already up)
	if runtime.GOOS != "darwin" {
		cmd = exec.Command("ip", "link", "set", "up", "dev", actualIfaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
		}
	}

	m.actualInterfaceName = actualIfaceName
	return nil
}

// DeleteInterface removes the WireGuard interface.
func (m *WireGuardTunnelManager) DeleteInterface() error {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	if runtime.GOOS == "darwin" {
		// macOS: kill wireguard-go process
		exec.Command("pkill", "-f", "wireguard-go.*"+ifaceName).Run()
		return nil
	}

	// Linux: use netlink
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return nil // Interface doesn't exist, nothing to do
	}

	return netlink.LinkDel(link)
}

// SetIPAddress sets the IP address on the interface.
func (m *WireGuardTunnelManager) SetIPAddress(ip string) error {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	if runtime.GOOS == "darwin" {
		// macOS: ifconfig utun inet 100.64.0.5 100.64.0.5 netmask 255.255.0.0
		// Extract IP without CIDR
		ipOnly := strings.Split(ip, "/")[0]
		cmd := exec.Command("ifconfig", ifaceName, "inet", ipOnly, ipOnly, "netmask", "255.255.0.0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
		}
		return nil
	}

	// Linux: use netlink
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return ErrInterfaceNotFound
	}

	// Ensure CIDR notation
	if !strings.Contains(ip, "/") {
		ip = ip + "/16"
	}

	addr, err := netlink.ParseAddr(ip)
	if err != nil {
		return ErrInvalidCIDR
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		// Ignore if address already exists
		if !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("failed to add address: %w", err)
		}
	}

	return nil
}

// AddPeer adds a new peer to the WireGuard interface.
func (m *WireGuardTunnelManager) AddPeer(peer *PeerConfig) error {
	return m.configurePeer(peer, false)
}

// RemovePeer removes a peer by its public key.
func (m *WireGuardTunnelManager) RemovePeer(publicKey string) error {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %w", err)
	}
	defer client.Close()

	pubKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return ErrInvalidKey
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: pubKey,
				Remove:    true,
			},
		},
	}

	return client.ConfigureDevice(ifaceName, cfg)
}

// UpdatePeer updates an existing peer's configuration.
func (m *WireGuardTunnelManager) UpdatePeer(peer *PeerConfig) error {
	return m.configurePeer(peer, true)
}

// ListPeers returns all configured peers.
func (m *WireGuardTunnelManager) ListPeers() ([]*PeerInfo, error) {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to open wgctrl: %w", err)
	}
	defer client.Close()

	device, err := client.Device(ifaceName)
	if err != nil {
		return nil, ErrInterfaceNotFound
	}

	peers := make([]*PeerInfo, 0, len(device.Peers))
	for _, p := range device.Peers {
		allowedIPs := make([]string, 0, len(p.AllowedIPs))
		for _, ip := range p.AllowedIPs {
			allowedIPs = append(allowedIPs, ip.String())
		}

		endpoint := ""
		if p.Endpoint != nil {
			endpoint = p.Endpoint.String()
		}

		peers = append(peers, &PeerInfo{
			PublicKey:           p.PublicKey.String(),
			Endpoint:            endpoint,
			AllowedIPs:          allowedIPs,
			LastHandshake:       p.LastHandshakeTime,
			RxBytes:             uint64(p.ReceiveBytes),
			TxBytes:             uint64(p.TransmitBytes),
			PersistentKeepalive: int(p.PersistentKeepaliveInterval.Seconds()),
		})
	}

	return peers, nil
}

// GetStats returns interface statistics.
func (m *WireGuardTunnelManager) GetStats() (*TunnelStats, error) {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return nil, ErrInterfaceNotFound
	}

	stats := link.Attrs().Statistics
	if stats == nil {
		return &TunnelStats{InterfaceName: ifaceName}, nil
	}

	// Get peer count
	peers, _ := m.ListPeers()

	return &TunnelStats{
		InterfaceName: ifaceName,
		RxBytes:       stats.RxBytes,
		TxBytes:       stats.TxBytes,
		RxPackets:     stats.RxPackets,
		TxPackets:     stats.TxPackets,
		PeerCount:     len(peers),
	}, nil
}

// GetInterfaceName returns the actual interface name.
func (m *WireGuardTunnelManager) GetInterfaceName() string {
	if m.actualInterfaceName != "" {
		return m.actualInterfaceName
	}
	return m.interfaceName
}

// GetPublicKey returns the interface's public key.
func (m *WireGuardTunnelManager) GetPublicKey() string {
	return m.publicKey
}

// configurePeer configures a WireGuard peer.
func (m *WireGuardTunnelManager) configurePeer(peer *PeerConfig, update bool) error {
	ifaceName := m.actualInterfaceName
	if ifaceName == "" {
		ifaceName = m.interfaceName
	}

	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %w", err)
	}
	defer client.Close()

	pubKey, err := wgtypes.ParseKey(peer.PublicKey)
	if err != nil {
		return ErrInvalidKey
	}

	var allowedIPs []net.IPNet
	for _, ipStr := range peer.AllowedIPs {
		ip, ipNet, err := net.ParseCIDR(ipStr)
		if err != nil {
			continue
		}
		ipNet.IP = ip
		allowedIPs = append(allowedIPs, *ipNet)
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey:  pubKey,
		AllowedIPs: allowedIPs,
	}

	if peer.Endpoint != "" {
		endpoint, err := net.ResolveUDPAddr("udp", peer.Endpoint)
		if err == nil {
			peerCfg.Endpoint = endpoint
		}
	}

	// Set PersistentKeepalive - default to 25 seconds for cloud environments
	// This prevents NAT timeout issues (cloud NAT gateways typically timeout after 60-180s)
	keepaliveSeconds := peer.PersistentKeepalive
	if keepaliveSeconds == 0 {
		keepaliveSeconds = 25 // Default for cloud environments
	}
	if keepaliveSeconds > 0 {
		keepalive := time.Duration(keepaliveSeconds) * time.Second
		peerCfg.PersistentKeepaliveInterval = &keepalive
	}

	if update {
		peerCfg.UpdateOnly = true
		peerCfg.ReplaceAllowedIPs = true
	}

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}

	if err := client.ConfigureDevice(ifaceName, cfg); err != nil {
		return fmt.Errorf("failed to configure peer: %w", err)
	}

	// macOS: Add routes for peer's allowed IPs
	if runtime.GOOS == "darwin" {
		for _, ipStr := range peer.AllowedIPs {
			// For /32 routes, use -host flag
			if strings.HasSuffix(ipStr, "/32") {
				ip := strings.TrimSuffix(ipStr, "/32")
				exec.Command("route", "add", "-host", ip, "-interface", ifaceName).Run()
			} else {
				exec.Command("route", "add", "-net", ipStr, "-interface", ifaceName).Run()
			}
		}
	}

	return nil
}

// waitForInterface waits for the interface to be ready.
func (m *WireGuardTunnelManager) waitForInterface(ifaceName string) error {
	for i := 0; i < 10; i++ {
		var checkCmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			checkCmd = exec.Command("ifconfig", ifaceName)
		} else {
			checkCmd = exec.Command("ip", "link", "show", ifaceName)
		}
		if _, err := checkCmd.Output(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("interface %s not ready after 1 second", ifaceName)
}

// setInterfaceMTU sets the MTU on the interface.
func (m *WireGuardTunnelManager) setInterfaceMTU(ifaceName string, mtu int) error {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("ifconfig", ifaceName, "mtu", fmt.Sprintf("%d", mtu))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	} else {
		cmd := exec.Command("ip", "link", "set", ifaceName, "mtu", fmt.Sprintf("%d", mtu))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	}
	return nil
}

// derivePublicKey derives the public key from a private key.
func derivePublicKey(privateKey string) (string, error) {
	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return "", err
	}
	return key.PublicKey().String(), nil
}

// getExistingAriaPorts returns a map of existing aria* interfaces and their listen ports.
// This is used to detect port conflicts during interface creation.
func getExistingAriaPorts() (map[int]string, error) {
	ports := make(map[int]string)

	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to open wgctrl: %w", err)
	}
	defer client.Close()

	devices, err := client.Devices()
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	for _, dev := range devices {
		// Check if interface name starts with "aria"
		if strings.HasPrefix(dev.Name, "aria") {
			ports[dev.ListenPort] = dev.Name
		}
	}

	return ports, nil
}

// GetAllAriaInterfaces returns all existing aria* interface names.
// This is used by multi-tunnel manager to detect existing tunnels.
func GetAllAriaInterfaces() ([]string, error) {
	var interfaces []string

	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	for _, link := range links {
		if strings.HasPrefix(link.Attrs().Name, "aria") {
			interfaces = append(interfaces, link.Attrs().Name)
		}
	}

	return interfaces, nil
}
