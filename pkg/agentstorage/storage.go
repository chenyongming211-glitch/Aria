package agentstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DBFileName = "agent.db"
)

type PeerConfig struct {
	PublicKey           string
	Endpoint            string
	AllowedIPs          []string
	PersistentKeepalive int
	AddedAt             int64
	// 新增字段 - 控制面和数据面分离所需
	AssignedIP string // VPN IP (如 100.64.0.1)
	Hostname   string // 主机名
	Region     string // 区域标识
	VPCID      string // VPC ID
	Status     string // online, offline, deleted
	LastSeen   int64  // 最后在线时间
}

// ACLRule represents a cached ACL rule (for Fail-Static support).
// When Controller is unreachable, Agent applies cached rules.
type ACLRule struct {
	SrcNet   string `json:"src_net"`
	DstNet   string `json:"dst_net"`
	Protocol uint8  `json:"protocol"`
	MinPort  uint16 `json:"min_port"`
	MaxPort  uint16 `json:"max_port"`
}

type AgentConfig struct {
	PrivateKey    string
	ControllerURL string
	InterfaceName string
	AssignedIP    string
	Hostname      string
	Version       string
	UpdatedAt     int64
}

type Metric struct {
	ID         int64
	Timestamp  int64
	LinkID     string
	Latency    int64
	PacketLoss float64
	BytesRx    uint64
	BytesTx    uint64
	IsUploaded int
}

type Storage struct {
	mu      sync.RWMutex
	db      *sql.DB
	dataDir string
}

func NewStorage(dataDir string) (*Storage, error) {
	s := &Storage{
		dataDir: dataDir,
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, DBFileName)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// 启用 WAL 模式，提高并发性能
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %v", err)
	}

	// 设置 busy timeout
	_, err = db.Exec("PRAGMA busy_timeout = 5000")
	if err != nil {
		return nil, fmt.Errorf("failed to set busy_timeout: %v", err)
	}

	// 初始化表
	schema := `
	-- 配置表 (KV 结构)
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	);
	
	-- 对等体表 (WireGuard Peers)
	CREATE TABLE IF NOT EXISTS peers (
		public_key TEXT PRIMARY KEY,
		endpoint TEXT,
		allowed_ips TEXT,
		persistent_keepalive INTEGER,
		added_at INTEGER NOT NULL,
		assigned_ip TEXT,
		hostname TEXT,
		region TEXT,
		status TEXT DEFAULT 'online',
		last_seen INTEGER,
		UNIQUE(public_key)
	);
	
	-- 监控数据缓冲表 (时序结构)
	CREATE TABLE IF NOT EXISTS metrics_buffer (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		link_id TEXT NOT NULL,
		latency INTEGER,
		packet_loss REAL,
		bytes_rx INTEGER,
		bytes_tx INTEGER,
		is_uploaded INTEGER DEFAULT 0
	);
	
	-- 索引
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics_buffer(timestamp);
	CREATE INDEX IF NOT EXISTS idx_metrics_uploaded ON metrics_buffer(is_uploaded);
	CREATE INDEX IF NOT EXISTS idx_peers_public_key ON peers(public_key);

	-- ACL 规则缓存表 (用于 Fail-Static 策略)
	-- 当 Controller 不可达时，Agent 使用本地缓存的 ACL 规则
	CREATE TABLE IF NOT EXISTS acl_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		src_net TEXT NOT NULL,
		dst_net TEXT NOT NULL,
		protocol INTEGER NOT NULL,
		min_port INTEGER NOT NULL,
		max_port INTEGER NOT NULL,
		cached_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_acl_cached_at ON acl_rules(cached_at);
	`
	_, err = db.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %v", err)
	}

	// Migration: Add region column if not exists (for existing databases)
	_, _ = db.Exec("ALTER TABLE peers ADD COLUMN region TEXT")
	// Migration: Add vpc_id column if not exists
	_, _ = db.Exec("ALTER TABLE peers ADD COLUMN vpc_id TEXT")

	s.db = db
	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// ============= Config 操作 =============

func (s *Storage) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT OR REPLACE INTO config (key, value, updated_at) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, key, value, time.Now().Unix())
	return err
}

func (s *Storage) GetConfig(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	query := `SELECT value FROM config WHERE key = ?`
	err := s.db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Storage) DeleteConfig(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM config WHERE key = ?`
	_, err := s.db.Exec(query, key)
	return err
}

// ============= Agent 配置原子操作 =============

func (s *Storage) SaveAgentConfig(cfg *AgentConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	queries := []struct {
		key   string
		value string
	}{
		{"private_key", cfg.PrivateKey},
		{"controller_url", cfg.ControllerURL},
		{"interface_name", cfg.InterfaceName},
		{"assigned_ip", cfg.AssignedIP},
		{"hostname", cfg.Hostname},
		{"version", cfg.Version},
	}

	for _, q := range queries {
		query := `INSERT OR REPLACE INTO config (key, value, updated_at) VALUES (?, ?, ?)`
		if _, err := tx.Exec(query, q.key, q.value, now); err != nil {
			return fmt.Errorf("failed to save config %s: %v", q.key, err)
		}
	}

	return tx.Commit()
}

func (s *Storage) LoadAgentConfig() (*AgentConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := &AgentConfig{}
	keys := []string{"private_key", "controller_url", "interface_name", "assigned_ip", "hostname", "version"}

	for _, key := range keys {
		var value string
		query := `SELECT value FROM config WHERE key = ?`
		err := s.db.QueryRow(query, key).Scan(&value)
		if err == sql.ErrNoRows {
			return nil, nil // 配置不存在
		}
		if err != nil {
			return nil, err
		}

		switch key {
		case "private_key":
			cfg.PrivateKey = value
		case "controller_url":
			cfg.ControllerURL = value
		case "interface_name":
			cfg.InterfaceName = value
		case "assigned_ip":
			cfg.AssignedIP = value
		case "hostname":
			cfg.Hostname = value
		case "version":
			cfg.Version = value
		}
	}

	// 检查必需字段
	if cfg.PrivateKey == "" || cfg.ControllerURL == "" || cfg.InterfaceName == "" {
		return nil, nil
	}

	return cfg, nil
}

// ============= Peers 操作 =============

func (s *Storage) SavePeers(peers []PeerConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// 删除所有旧 Peer
	if _, err := tx.Exec("DELETE FROM peers"); err != nil {
		return fmt.Errorf("failed to delete old peers: %v", err)
	}

	// 插入新 Peer
	for _, peer := range peers {
		allowedIPsJSON, _ := json.Marshal(peer.AllowedIPs)
		query := `INSERT INTO peers (public_key, endpoint, allowed_ips, persistent_keepalive, added_at, assigned_ip, hostname, region, vpc_id, status, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := tx.Exec(query,
			peer.PublicKey,
			peer.Endpoint,
			string(allowedIPsJSON),
			peer.PersistentKeepalive,
			peer.AddedAt,
			peer.AssignedIP,
			peer.Hostname,
			peer.Region,
			peer.VPCID,
			peer.Status,
			peer.LastSeen,
		)
		if err != nil {
			return fmt.Errorf("failed to save peer %s: %v", peer.PublicKey[:8], err)
		}
	}

	return tx.Commit()
}

func (s *Storage) LoadPeers() ([]PeerConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT public_key, endpoint, allowed_ips, persistent_keepalive, added_at,
		       COALESCE(assigned_ip, ''), COALESCE(hostname, ''),
		       COALESCE(region, ''), COALESCE(vpc_id, ''), COALESCE(status, 'online'), COALESCE(last_seen, 0)
		FROM peers
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []PeerConfig
	for rows.Next() {
		var peer PeerConfig
		var allowedIPsJSON string
		err := rows.Scan(
			&peer.PublicKey,
			&peer.Endpoint,
			&allowedIPsJSON,
			&peer.PersistentKeepalive,
			&peer.AddedAt,
			&peer.AssignedIP,
			&peer.Hostname,
			&peer.Region,
			&peer.VPCID,
			&peer.Status,
			&peer.LastSeen,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(allowedIPsJSON), &peer.AllowedIPs); err != nil {
			peer.AllowedIPs = []string{}
		}

		peers = append(peers, peer)
	}

	return peers, nil
}

// ============= Peer 辅助操作（HA支持）=============

// GetPeerByPublicKey 根据公钥获取单个peer配置
func (s *Storage) GetPeerByPublicKey(publicKey string) (*PeerConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var peer PeerConfig
	var allowedIPsJSON string

	query := `
		SELECT public_key, endpoint, allowed_ips, persistent_keepalive, added_at,
		       COALESCE(assigned_ip, ''), COALESCE(hostname, ''),
		       COALESCE(region, ''), COALESCE(vpc_id, ''), COALESCE(status, 'online'), COALESCE(last_seen, 0)
		FROM peers
		WHERE public_key = ?
	`
	err := s.db.QueryRow(query, publicKey).Scan(
		&peer.PublicKey,
		&peer.Endpoint,
		&allowedIPsJSON,
		&peer.PersistentKeepalive,
		&peer.AddedAt,
		&peer.AssignedIP,
		&peer.Hostname,
		&peer.Region,
		&peer.VPCID,
		&peer.Status,
		&peer.LastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(allowedIPsJSON), &peer.AllowedIPs); err != nil {
		peer.AllowedIPs = []string{}
	}

	return &peer, nil
}

// DeletePeer 从本地缓存删除peer（用于手动删除或收到删除通知）
func (s *Storage) DeletePeer(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM peers WHERE public_key = ?`
	_, err := s.db.Exec(query, publicKey)
	return err
}

// MarkPeerOffline 标记peer为离线状态（Controller不可达时使用）
func (s *Storage) MarkPeerOffline(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE peers SET status = 'offline', last_seen = ? WHERE public_key = ?`
	_, err := s.db.Exec(query, time.Now().Unix(), publicKey)
	return err
}

// UpdatePeerStatus 更新peer状态（online/offline/deleted）
func (s *Storage) UpdatePeerStatus(publicKey, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE peers SET status = ?, last_seen = ? WHERE public_key = ?`
	_, err := s.db.Exec(query, status, time.Now().Unix(), publicKey)
	return err
}

// ============= Metrics 操作 =============

func (s *Storage) AddMetric(metric *Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO metrics_buffer (timestamp, link_id, latency, packet_loss, bytes_rx, bytes_tx, is_uploaded) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, metric.Timestamp, metric.LinkID, metric.Latency, metric.PacketLoss, metric.BytesRx, metric.BytesTx, metric.IsUploaded)
	return err
}

func (s *Storage) GetUnuploadedMetrics(limit int) ([]*Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT id, timestamp, link_id, latency, packet_loss, bytes_rx, bytes_tx, is_uploaded FROM metrics_buffer WHERE is_uploaded = 0 ORDER BY timestamp ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*Metric
	for rows.Next() {
		m := &Metric{}
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.LinkID, &m.Latency, &m.PacketLoss, &m.BytesRx, &m.BytesTx, &m.IsUploaded); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

func (s *Storage) MarkMetricsUploaded(ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		return nil
	}

	query := `UPDATE metrics_buffer SET is_uploaded = 1 WHERE id IN (`
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += fmt.Sprintf("%d", id)
	}
	query += ")"

	_, err := s.db.Exec(query)
	return err
}

func (s *Storage) CleanupOldMetrics(days int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Unix() - int64(days*24*60*60)
	query := `DELETE FROM metrics_buffer WHERE timestamp < ?`
	result, err := s.db.Exec(query, cutoff)
	if err != nil {
		return 0, err
	}

	count, _ := result.RowsAffected()
	return int(count), nil
}

func (s *Storage) GetMetricsCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	query := `SELECT COUNT(*) FROM metrics_buffer`
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// ============= 状态检查 =============

func (s *Storage) HasValidConfig() bool {
	cfg, err := s.LoadAgentConfig()
	return err == nil && cfg != nil
}

func (s *Storage) GetLastSeen() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var timestamp int64
	query := `SELECT MAX(updated_at) FROM config`
	err := s.db.QueryRow(query).Scan(&timestamp)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return timestamp, err
}

// ============= ACL Rules 操作 (Fail-Static 支持) =============

// SaveACLRules atomically replaces all cached ACL rules.
// This is called after successfully syncing with Controller.
func (s *Storage) SaveACLRules(rules []ACLRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Delete all old rules
	if _, err := tx.Exec("DELETE FROM acl_rules"); err != nil {
		return fmt.Errorf("failed to delete old ACL rules: %v", err)
	}

	// Insert new rules
	now := time.Now().Unix()
	for _, rule := range rules {
		query := `INSERT INTO acl_rules (src_net, dst_net, protocol, min_port, max_port, cached_at) VALUES (?, ?, ?, ?, ?, ?)`
		_, err := tx.Exec(query, rule.SrcNet, rule.DstNet, rule.Protocol, rule.MinPort, rule.MaxPort, now)
		if err != nil {
			return fmt.Errorf("failed to save ACL rule %s->%s: %v", rule.SrcNet, rule.DstNet, err)
		}
	}

	return tx.Commit()
}

// LoadACLRules loads cached ACL rules from local storage.
// Returns empty slice if no cached rules exist.
func (s *Storage) LoadACLRules() ([]ACLRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT src_net, dst_net, protocol, min_port, max_port
		FROM acl_rules
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ACLRule
	for rows.Next() {
		var rule ACLRule
		err := rows.Scan(&rule.SrcNet, &rule.DstNet, &rule.Protocol, &rule.MinPort, &rule.MaxPort)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// GetACLCacheTime returns the timestamp when ACL rules were last cached.
// Returns 0 if no cached rules exist.
func (s *Storage) GetACLCacheTime() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var timestamp int64
	query := `SELECT MAX(cached_at) FROM acl_rules`
	err := s.db.QueryRow(query).Scan(&timestamp)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return timestamp, err
}

// GetPeerUpdateTime returns the timestamp when peers were last updated.
// Returns the MAX(last_seen) from peers table.
func (s *Storage) GetPeerUpdateTime() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var timestamp int64
	query := `SELECT MAX(last_seen) FROM peers`
	err := s.db.QueryRow(query).Scan(&timestamp)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return timestamp, err
}

// GetACLUpdateTime is an alias for GetACLCacheTime for interface compatibility.
func (s *Storage) GetACLUpdateTime() (int64, error) {
	return s.GetACLCacheTime()
}
