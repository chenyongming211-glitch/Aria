package tunnel

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"aria/pkg/capability"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type PeerConfig struct {
	PublicKey           string
	AllowedIPs          []string
	Endpoint            string
	PersistentKeepalive int
}

type Manager struct {
	interfaceName string
	privateKey    string
	publicKey     string
	runtimeMode   capability.RuntimeMode
}

// NewManager creates a new tunnel manager (defaults to kernel mode)
func NewManager(interfaceName string) (*Manager, error) {
	return &Manager{
		interfaceName: interfaceName,
		runtimeMode:   capability.ModeKernel,
	}, nil
}

// NewManagerWithMode creates a new tunnel manager with specified runtime mode
func NewManagerWithMode(interfaceName string, mode capability.RuntimeMode) (*Manager, error) {
	return &Manager{
		interfaceName: interfaceName,
		runtimeMode:   mode,
	}, nil
}

// GetRuntimeMode returns the current runtime mode
func (m *Manager) GetRuntimeMode() capability.RuntimeMode {
	return m.runtimeMode
}

// SetRuntimeMode sets the runtime mode
func (m *Manager) SetRuntimeMode(mode capability.RuntimeMode) {
	m.runtimeMode = mode
}

// EnsureInterface creates and configures the WireGuard interface
func (m *Manager) EnsureInterface(privateKey string) error {
	if m.runtimeMode == capability.ModeUserspace {
		return m.ensureUserspaceInterface(privateKey)
	}
	return m.ensureKernelInterface(privateKey)
}

// ensureKernelInterface creates WireGuard interface using kernel module
func (m *Manager) ensureKernelInterface(privateKey string) error {
	// 1. 删除已存在的同名接口（如果存在）
	link, err := netlink.LinkByName(m.interfaceName)
	if err == nil {
		netlink.LinkDel(link)
	}

	// 2. 创建 WireGuard 接口
	wg := &netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: m.interfaceName,
		},
	}

	err = netlink.LinkAdd(wg)
	if err != nil {
		return fmt.Errorf("failed to create WireGuard interface: %v", err)
	}

	// 3. 设置 MTU 为 1360（优化后的 MTU 值）
	if err := netlink.LinkSetMTU(wg, 1360); err != nil {
		return fmt.Errorf("failed to set MTU: %v", err)
	}

	// 4. UP 接口（先UP，后续再配置IP）
	if err := netlink.LinkSetUp(wg); err != nil {
		return fmt.Errorf("failed to set interface up: %v", err)
	}

	// 5. 配置 WireGuard 私钥
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %v", err)
	}
	defer client.Close()

	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return fmt.Errorf("invalid private key: %v", err)
	}

	err = client.ConfigureDevice(m.interfaceName, wgtypes.Config{
		PrivateKey: &key,
		ListenPort: func() *int { p := 51820; return &p }(),
	})
	if err != nil {
		return fmt.Errorf("failed to configure device: %v", err)
	}

	// 6. 启用 IP Forwarding
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %v", err)
	}

	return nil
}

// SetIPAddress 设置接口的 IP 地址
func (m *Manager) SetIPAddress(ip string) error {
	link, err := netlink.LinkByName(m.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface: %v", err)
	}

	addr, err := netlink.ParseAddr(ip)
	if err != nil {
		return fmt.Errorf("failed to parse address: %v", err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add address: %v", err)
	}

	return nil
}

// UpdatePeers 动态添加/更新 Peer 配置
func (m *Manager) UpdatePeers(peers []PeerConfig) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %v", err)
	}
	defer client.Close()

	for _, peer := range peers {
		err := configurePeer(client, m.interfaceName, peer)
		if err != nil {
			return fmt.Errorf("failed to configure peer %s: %v", peer.PublicKey[:8], err)
		}
		log.Printf("Successfully configured peer: %s...", peer.PublicKey[:8])
	}

	return nil
}

func configurePeer(client *wgctrl.Client, iface string, peer PeerConfig) error {
	pubKey, err := wgtypes.ParseKey(peer.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %v", err)
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

	keepalive := time.Duration(peer.PersistentKeepalive) * time.Second
	peerCfg := wgtypes.PeerConfig{
		PublicKey:                   pubKey,
		AllowedIPs:                  allowedIPs,
		Endpoint:                    parseEndpoint(peer.Endpoint),
		PersistentKeepaliveInterval: &keepalive,
	}

	return client.ConfigureDevice(iface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	})
}

func parseEndpoint(endpoint string) *net.UDPAddr {
	if endpoint == "" {
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return nil
	}
	return addr
}

// GetPublicKey 返回公钥
func (m *Manager) GetPublicKey() string {
	return m.publicKey
}

// SetKeys 设置公私钥
func (m *Manager) SetKeys(privateKey, publicKey string) {
	m.privateKey = privateKey
	m.publicKey = publicKey
}

// GetInterface 返回接口名称
func (m *Manager) GetInterface() string {
	return m.interfaceName
}

// GetStats 获取 WireGuard 接口统计信息
func (m *Manager) GetStats() (rx, tx uint64, err error) {
	link, err := netlink.LinkByName(m.interfaceName)
	if err != nil {
		return 0, 0, err
	}

	stats := link.Attrs().Statistics
	return stats.RxBytes, stats.TxBytes, nil
}

// Cleanup 清理 WireGuard 接口
func (m *Manager) Cleanup() error {
	link, err := netlink.LinkByName(m.interfaceName)
	if err != nil {
		return nil
	}

	return netlink.LinkDel(link)
}

func init() {
	unix.Umask(0077)
}
