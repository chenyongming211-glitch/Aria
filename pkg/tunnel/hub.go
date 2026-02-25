package tunnel

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type HubMode struct {
	manager *Manager
	isHub   bool
	hubIP   string
}

func NewHubMode(mgr *Manager, isHub bool, hubIP string) *HubMode {
	return &HubMode{
		manager: mgr,
		isHub:   isHub,
		hubIP:   hubIP,
	}
}

func (h *HubMode) Configure() error {
	if !h.isHub {
		log.Printf("HubMode: running in Spoke mode, Hub=%s", h.hubIP)
		return nil
	}

	log.Printf("HubMode: configuring as Hub node")

	link, err := netlink.LinkByName(h.manager.interfaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface: %v", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set interface up: %v", err)
	}

	if err := enableMasquerade(); err != nil {
		log.Printf("HubMode: warning - failed to enable masquerade: %v", err)
	}

	return nil
}

func (h *HubMode) ConfigureSpoke(hubPeer PeerConfig) error {
	if h.isHub {
		return nil
	}

	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to open wgctrl: %v", err)
	}
	defer client.Close()

	pubKey, err := wgtypes.ParseKey(hubPeer.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %v", err)
	}

	var allowedIPs []net.IPNet
	for _, ipStr := range hubPeer.AllowedIPs {
		ip, ipNet, err := net.ParseCIDR(ipStr)
		if err != nil {
			continue
		}
		ipNet.IP = ip
		allowedIPs = append(allowedIPs, *ipNet)
	}

	keepalive := 25 * time.Second
	peerCfg := wgtypes.PeerConfig{
		PublicKey:                   pubKey,
		AllowedIPs:                  allowedIPs,
		Endpoint:                    parseEndpoint(hubPeer.Endpoint),
		PersistentKeepaliveInterval: &keepalive,
	}

	return client.ConfigureDevice(h.manager.interfaceName, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	})
}

func enableMasquerade() error {
	log.Printf("HubMode: IP Masquerade should be enabled via iptables")
	log.Printf("HubMode: Run: iptables -t nat -A POSTROUTING -s 100.64.0.0/16 -o eth0 -j MASQUERADE")
	return nil
}

type HubConfig struct {
	IsHub        bool
	HubIP        string
	HubPublicKey string
}
