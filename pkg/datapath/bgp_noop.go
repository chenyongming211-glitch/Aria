package datapath

// NoopBGPManager is a placeholder implementation of BGPManager.
// It returns ErrNotImplemented for all operations.
type NoopBGPManager struct{}

// NewNoopBGPManager creates a new no-op BGP manager.
func NewNoopBGPManager() *NoopBGPManager {
	return &NoopBGPManager{}
}

// Start initializes and starts the BGP daemon.
func (m *NoopBGPManager) Start(cfg *BGPConfig) error {
	return ErrNotImplemented
}

// Stop gracefully stops the BGP daemon.
func (m *NoopBGPManager) Stop() error {
	return nil // No-op, always succeeds
}

// AddNeighbor adds a BGP neighbor/peer.
func (m *NoopBGPManager) AddNeighbor(neighbor *BGPNeighbor) error {
	return ErrNotImplemented
}

// RemoveNeighbor removes a BGP neighbor by peer IP.
func (m *NoopBGPManager) RemoveNeighbor(peerIP string) error {
	return ErrNotImplemented
}

// AdvertisePrefix announces a prefix to BGP neighbors.
func (m *NoopBGPManager) AdvertisePrefix(prefix string) error {
	return ErrNotImplemented
}

// WithdrawPrefix withdraws a previously announced prefix.
func (m *NoopBGPManager) WithdrawPrefix(prefix string) error {
	return ErrNotImplemented
}

// GetNeighbors returns all BGP neighbors and their status.
func (m *NoopBGPManager) GetNeighbors() ([]*BGPNeighbor, error) {
	return nil, ErrNotImplemented
}
