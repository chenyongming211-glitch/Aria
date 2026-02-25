package datapath

import "time"

// NoopHAManager is a placeholder implementation of HAManager.
// It returns ErrNotImplemented for all operations.
type NoopHAManager struct{}

// NewNoopHAManager creates a new no-op HA manager.
func NewNoopHAManager() *NoopHAManager {
	return &NoopHAManager{}
}

// SetSyncPeer sets the peer for state synchronization.
func (m *NoopHAManager) SetSyncPeer(peerIP string, port int) error {
	return ErrNotImplemented
}

// GetSyncStatus returns the current sync status.
func (m *NoopHAManager) GetSyncStatus() (*SyncStatus, error) {
	return &SyncStatus{
		Role:         "standalone",
		PeerIP:       "",
		PeerState:    "unknown",
		LastSyncTime: time.Time{},
		SyncLag:      0,
	}, nil
}

// IsActive returns true if this node is the active node.
// In standalone mode, always returns true.
func (m *NoopHAManager) IsActive() bool {
	return true
}

// TriggerFailover initiates a failover to the standby node.
func (m *NoopHAManager) TriggerFailover() error {
	return ErrNotImplemented
}
