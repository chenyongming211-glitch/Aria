package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Node struct {
	ID           int64
	PublicKey    string
	Endpoint     string
	PrivateIP    string
	PublicIP     string
	Region       string
	VPCID        string
	Hostname     string
	AssignedIP   string
	IPOffset     int
	LastSeen     int64
	RegisteredAt int64
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

	dbPath := filepath.Join(dataDir, "storage.db")

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
	CREATE TABLE IF NOT EXISTS nodes (
		public_key TEXT PRIMARY KEY,
		endpoint TEXT,
		private_ip TEXT,
		public_ip TEXT,
		region TEXT,
		vpc_id TEXT,
		hostname TEXT,
		assigned_ip TEXT NOT NULL,
		ip_offset INTEGER NOT NULL,
		last_seen INTEGER NOT NULL,
		registered_at INTEGER NOT NULL
	);
	
	CREATE TABLE IF NOT EXISTS state (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL
	);
	`
	_, err = db.Exec(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %v", err)
	}

	s.db = db
	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) SaveNode(node *Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT OR REPLACE INTO nodes (public_key, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		node.PublicKey, node.Endpoint, node.PrivateIP, node.PublicIP,
		node.Region, node.VPCID, node.Hostname, node.AssignedIP,
		node.IPOffset, node.LastSeen, node.RegisteredAt,
	)

	return err
}

func (s *Storage) GetNode(publicKey string) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT * FROM nodes WHERE public_key = ?`
	row := s.db.QueryRow(query, publicKey)

	return s.scanNode(row)
}

func (s *Storage) GetAllNodes() ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT * FROM nodes ORDER BY last_seen DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		node, err := s.scanNodeRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *Storage) DeleteNode(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM nodes WHERE public_key = ?`
	_, err := s.db.Exec(query, publicKey)
	return err
}

func (s *Storage) CleanupStaleNodes(thresholdSeconds int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Unix() - thresholdSeconds
	query := `DELETE FROM nodes WHERE last_seen < ?`
	result, err := s.db.Exec(query, cutoff)
	if err != nil {
		return 0, err
	}

	count, _ := result.RowsAffected()
	return int(count), nil
}

func (s *Storage) GetNextOffset() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var offset int
	query := `SELECT value FROM state WHERE key = 'next_offset'`
	err := s.db.QueryRow(query).Scan(&offset)
	if err == sql.ErrNoRows {
		return 2, nil
	}
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func (s *Storage) SetNextOffset(offset int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT OR REPLACE INTO state (key, value) VALUES ('next_offset', ?)`
	_, err := s.db.Exec(query, offset)
	return err
}

func (s *Storage) IncrementNextOffset() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.GetNextOffset()
	if err != nil {
		return 0, err
	}

	newOffset := current + 1
	if newOffset > 254 {
		newOffset = 2
	}

	if err := s.SetNextOffset(newOffset); err != nil {
		return 0, err
	}

	return current, nil
}

func (s *Storage) IsOffsetUsed(offset int) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT COUNT(*) FROM nodes WHERE ip_offset = ?`
	var count int
	err := s.db.QueryRow(query, offset).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Storage) GetNextAvailableOffset() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.GetNextOffset()
	if err != nil {
		return 0, err
	}

	for i := 0; i < 253; i++ {
		offset := current + i
		if offset > 254 {
			offset = 2
		}

		var count int
		query := `SELECT COUNT(*) FROM nodes WHERE ip_offset = ?`
		err := s.db.QueryRow(query, offset).Scan(&count)
		if err != nil {
			return 0, err
		}

		if count == 0 {
			newOffset := offset + 1
			if newOffset > 254 {
				newOffset = 2
			}
			s.SetNextOffset(newOffset)
			return offset, nil
		}
	}

	return 2, nil
}

func (s *Storage) GetNodeCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	query := `SELECT COUNT(*) FROM nodes`
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

func (s *Storage) scanNode(row *sql.Row) (*Node, error) {
	node := &Node{}
	err := row.Scan(
		&node.PublicKey, &node.Endpoint, &node.PrivateIP,
		&node.PublicIP, &node.Region, &node.VPCID, &node.Hostname,
		&node.AssignedIP, &node.IPOffset, &node.LastSeen, &node.RegisteredAt,
	)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (s *Storage) scanNodeRows(rows *sql.Rows) (*Node, error) {
	node := &Node{}
	err := rows.Scan(
		&node.PublicKey, &node.Endpoint, &node.PrivateIP,
		&node.PublicIP, &node.Region, &node.VPCID, &node.Hostname,
		&node.AssignedIP, &node.IPOffset, &node.LastSeen, &node.RegisteredAt,
	)
	if err != nil {
		return nil, err
	}
	return node, nil
}
