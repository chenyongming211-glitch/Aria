package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// EbpfSnapshot represents the persistent eBPF configuration state
type EbpfSnapshot struct {
	Version    int64               `json:"version"`     // Timestamp of last update
	ACLRules   []ACLRuleSnapshot   `json:"acl_rules"`   // Access control rules
	QoSRules   []QoSRuleSnapshot   `json:"qos_rules"`   // Quality of service rules
	Interfaces []string            `json:"interfaces"`  // Network interfaces
}

type ACLRuleSnapshot struct {
	CIDR   string `json:"cidr"`
	Action string `json:"action"` // "allow", "block", "redirect"
	Port   int    `json:"port,omitempty"`
}

type QoSRuleSnapshot struct {
	Type       string `json:"type"`                  // "ip", "peer", "service", "port"
	SourceIP   string `json:"source_ip,omitempty"`
	DestIP     string `json:"dest_ip,omitempty"`
	SourcePort int    `json:"source_port,omitempty"`
	DestPort   int    `json:"dest_port,omitempty"`
	Protocol   int    `json:"protocol,omitempty"`
	Bandwidth  int    `json:"bandwidth"`   // Mbps
	Direction  string `json:"direction,omitempty"` // "src", "dst", "both"
}

type EbpfSnapshotManager struct {
	snapshotPath string
	mu           sync.RWMutex
}

func NewEbpfSnapshotManager(snapshotPath string) *EbpfSnapshotManager {
	// Ensure directory exists
	dir := filepath.Dir(snapshotPath)
	os.MkdirAll(dir, 0755)

	return &EbpfSnapshotManager{
		snapshotPath: snapshotPath,
	}
}

func (esm *EbpfSnapshotManager) Save(snapshot *EbpfSnapshot) error {
	esm.mu.Lock()
	defer esm.mu.Unlock()

	// Create temporary file
	tempPath := esm.snapshotPath + ".tmp"

	// Write to temporary file
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %v", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %v", err)
	}

	// Atomic rename
	return os.Rename(tempPath, esm.snapshotPath)
}

func (esm *EbpfSnapshotManager) Load() (*EbpfSnapshot, error) {
	esm.mu.RLock()
	defer esm.mu.RUnlock()

	data, err := os.ReadFile(esm.snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %v", err)
	}

	var snapshot EbpfSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %v", err)
	}

	return &snapshot, nil
}

func (esm *EbpfSnapshotManager) Exists() bool {
	_, err := os.Stat(esm.snapshotPath)
	return err == nil
}