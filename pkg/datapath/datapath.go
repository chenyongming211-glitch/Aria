package datapath

// DataPath provides a unified interface to all data plane managers.
type DataPath struct {
	// Tunnel manages WireGuard tunnels and peers.
	Tunnel TunnelManager

	// Route manages system routing tables.
	Route RouteManager

	// BGP manages BGP dynamic routing (optional).
	BGP BGPManager

	// Firewall manages firewall rules (optional).
	Firewall FirewallManager

	// HA manages high availability (optional).
	HA HAManager
}

// Option is a function that configures a DataPath.
type Option func(*DataPath)

// NewDataPath creates a new DataPath with the specified options.
func NewDataPath(interfaceName string, mode RuntimeMode, opts ...Option) (*DataPath, error) {
	dp := &DataPath{
		Tunnel:   NewWireGuardTunnelManager(interfaceName, mode),
		Route:    NewLinuxRouteManager(interfaceName),
		BGP:      NewNoopBGPManager(),
		Firewall: NewNoopFirewallManager(),
		HA:       NewNoopHAManager(),
	}

	for _, opt := range opts {
		opt(dp)
	}

	return dp, nil
}

// WithTunnelManager sets a custom tunnel manager.
func WithTunnelManager(tm TunnelManager) Option {
	return func(dp *DataPath) {
		dp.Tunnel = tm
	}
}

// WithRouteManager sets a custom route manager.
func WithRouteManager(rm RouteManager) Option {
	return func(dp *DataPath) {
		dp.Route = rm
	}
}

// WithBGPManager sets a custom BGP manager.
func WithBGPManager(bgp BGPManager) Option {
	return func(dp *DataPath) {
		dp.BGP = bgp
	}
}

// WithFirewallManager sets a custom firewall manager.
func WithFirewallManager(fw FirewallManager) Option {
	return func(dp *DataPath) {
		dp.Firewall = fw
	}
}

// WithHAManager sets a custom HA manager.
func WithHAManager(ha HAManager) Option {
	return func(dp *DataPath) {
		dp.HA = ha
	}
}

// Init initializes all data path components.
func (dp *DataPath) Init() error {
	// Initialize route manager if available
	if dp.Route != nil {
		if err := dp.Route.Init(); err != nil {
			return err
		}
	}

	// Initialize firewall if available
	if dp.Firewall != nil {
		if err := dp.Firewall.Init(); err != nil {
			return err
		}
	}

	return nil
}

// Cleanup cleans up all data path components.
func (dp *DataPath) Cleanup() error {
	var lastErr error

	// Cleanup tunnel
	if dp.Tunnel != nil {
		if err := dp.Tunnel.DeleteInterface(); err != nil {
			lastErr = err
		}
	}

	// Cleanup routes
	if dp.Route != nil {
		if err := dp.Route.Cleanup(); err != nil {
			lastErr = err
		}
	}

	// Cleanup firewall
	if dp.Firewall != nil {
		if err := dp.Firewall.Cleanup(); err != nil {
			lastErr = err
		}
	}

	// Stop BGP
	if dp.BGP != nil {
		if err := dp.BGP.Stop(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
