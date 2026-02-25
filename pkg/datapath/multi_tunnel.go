package datapath

import (
	"fmt"
)

// TunnelInstance represents a single WireGuard tunnel instance
type TunnelInstance struct {
	Name       string        // aria0, aria1, aria2...
	Port       int           // 51820, 51821, 51822...
	Manager    TunnelManager // WireGuard manager
	PrivateKey string
	PublicKey  string
}

// MultiTunnelManager manages multiple WireGuard tunnels
type MultiTunnelManager struct {
	tunnels      []*TunnelInstance
	basePort     int    // 51820
	baseName     string // "aria"
	tunnelCount  int
	routeManager RouteManager
	runtimeMode  RuntimeMode
}

// NewMultiTunnelManager creates a new multi-tunnel manager
func NewMultiTunnelManager(baseName string, basePort int, count int, runtimeMode RuntimeMode, routeManager RouteManager) *MultiTunnelManager {
	return &MultiTunnelManager{
		tunnels:      make([]*TunnelInstance, 0, count),
		basePort:     basePort,
		baseName:     baseName,
		tunnelCount:  count,
		routeManager: routeManager,
		runtimeMode:  runtimeMode,
	}
}

// EnsureAllTunnels creates all tunnel instances
func (m *MultiTunnelManager) EnsureAllTunnels(cfg *InterfaceConfig) error {
	for i := 0; i < m.tunnelCount; i++ {
		instance := &TunnelInstance{
			Name: fmt.Sprintf("%s%d", m.baseName, i),
			Port: m.basePort + i,
		}

		// Create tunnel configuration for this instance
		tunnelCfg := *cfg
		tunnelCfg.Name = instance.Name
		tunnelCfg.ListenPort = instance.Port

		// Create tunnel manager
		manager := NewWireGuardTunnelManager(instance.Name, m.runtimeMode)

		// Ensure interface exists
		if err := manager.EnsureInterface(&tunnelCfg); err != nil {
			return fmt.Errorf("failed to ensure interface %s: %w", instance.Name, err)
		}

		// Set IP address (only for the first tunnel)
		if i == 0 {
			if err := manager.SetIPAddress(cfg.Address); err != nil {
				return fmt.Errorf("failed to set IP address on %s: %w", instance.Name, err)
			}
		}

		instance.Manager = manager
		instance.PrivateKey = cfg.PrivateKey
		instance.PublicKey = manager.GetPublicKey()

		m.tunnels = append(m.tunnels, instance)
	}

	return nil
}

// ConfigureAllPeers configures the same peer on all tunnels
func (m *MultiTunnelManager) ConfigureAllPeers(peers []*PeerConfig) error {
	for _, peer := range peers {
		for i, tunnel := range m.tunnels {
			// Create peer config for this tunnel
			peerCfg := *peer

			// Update endpoint port to match tunnel port
			if peerCfg.Endpoint != "" {
				// Parse endpoint and replace port
				host, _, err := parseEndpoint(peerCfg.Endpoint)
				if err != nil {
					return fmt.Errorf("failed to parse endpoint %s: %w", peerCfg.Endpoint, err)
				}
				peerCfg.Endpoint = fmt.Sprintf("%s:%d", host, m.basePort+i)
			}

			// Add peer to tunnel
			if err := tunnel.Manager.AddPeer(&peerCfg); err != nil {
				return fmt.Errorf("failed to add peer to %s: %w", tunnel.Name, err)
			}
		}
	}

	return nil
}

// SetupECMPRoutes configures ECMP routes for all tunnels
func (m *MultiTunnelManager) SetupECMPRoutes(destinations []string) error {
	// Collect all interface names
	interfaces := make([]string, len(m.tunnels))
	for i, tunnel := range m.tunnels {
		interfaces[i] = tunnel.Name
	}

	// Add ECMP route for each destination
	for _, dest := range destinations {
		if err := m.routeManager.AddECMPRoute(dest, interfaces); err != nil {
			return fmt.Errorf("failed to add ECMP route for %s: %w", dest, err)
		}
	}

	return nil
}

// Cleanup removes all tunnels
func (m *MultiTunnelManager) Cleanup() error {
	var lastErr error
	for _, tunnel := range m.tunnels {
		if err := tunnel.Manager.DeleteInterface(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// GetTunnels returns all tunnel instances
func (m *MultiTunnelManager) GetTunnels() []*TunnelInstance {
	return m.tunnels
}

// GetPrimaryTunnel returns the first tunnel (primary)
func (m *MultiTunnelManager) GetPrimaryTunnel() *TunnelInstance {
	if len(m.tunnels) > 0 {
		return m.tunnels[0]
	}
	return nil
}

// parseEndpoint parses an endpoint string into host and port
func parseEndpoint(endpoint string) (host string, port string, err error) {
	// Simple parser for "host:port" format
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			return endpoint[:i], endpoint[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint format: %s", endpoint)
}
