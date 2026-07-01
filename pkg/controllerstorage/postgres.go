package controllerstorage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// TenantInfo 定义租户信息结构体
type TenantInfo struct {
	ID            uuid.UUID
	Name          string
	Code          string
	Status        string
	ResourceQuota string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const (
	// Optimized connection pool settings for high concurrency
	defaultMaxOpenConns    = 200              // Increased from 100 for better concurrency
	defaultMaxIdleConns    = 50               // Increased from 10 to maintain warm connections
	defaultConnMaxLifetime = 30 * time.Minute // Reduced from 1 hour to prevent stale connections
	defaultConnMaxIdleTime = 5 * time.Minute  // Close idle connections after 5 minutes
)

type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

type Storage struct {
	db        *sql.DB
	heartbeat *HeartbeatStore
	baseIP    string
	cidr      string
}

func NewStorage(cfg *Config, baseIP, cidr string) (*Storage, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Configure connection pool
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(defaultMaxOpenConns)
	}

	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(defaultMaxIdleConns)
	}

	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(defaultConnMaxLifetime)
	}

	// Set max idle time to close stale connections
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	} else {
		db.SetConnMaxIdleTime(defaultConnMaxIdleTime)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	log.Printf("Database connection pool configured: maxOpen=%d, maxIdle=%d, maxLifetime=%v, maxIdleTime=%v",
		db.Stats().MaxOpenConnections, defaultMaxIdleConns, defaultConnMaxLifetime, defaultConnMaxIdleTime)

	s := &Storage{db: db, baseIP: baseIP, cidr: cidr}

	if err := s.Migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %v", err)
	}

	if err := s.EnsureAllTenantRoles(); err != nil {
		log.Printf("WARN: failed to ensure tenant roles: %v", err)
	}

	return s, nil
}

func (s *Storage) SetHeartbeatStore(hb *HeartbeatStore) {
	s.heartbeat = hb
}

func (s *Storage) HasHeartbeat() bool {
	return s.heartbeat != nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection
func (s *Storage) DB() *sql.DB {
	return s.db
}

// NewStorageWithDB creates a storage wrapper from an existing DB handle.
// Primarily used in unit tests where the DB is mocked.
func NewStorageWithDB(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func ipToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		return uint32(ip[12])<<24 | uint32(ip[13])<<16 | uint32(ip[14])<<8 | uint32(ip[15])
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func intToIp(nn uint32) net.IP {
	return net.IP{byte(nn >> 24), byte(nn >> 16), byte(nn >> 8), byte(nn)}
}

func (s *Storage) CalculateIP(offset int) (string, error) {
	ip := net.ParseIP(s.baseIP)
	if ip == nil {
		return "", fmt.Errorf("invalid base IP: %s", s.baseIP)
	}

	baseInt := ipToInt(ip)
	newIP := intToIp(baseInt + uint32(offset))
	return newIP.String(), nil
}

func (s *Storage) Migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(100) NOT NULL,
			code VARCHAR(50) UNIQUE,
			email VARCHAR(100),
			phone VARCHAR(32),
			status VARCHAR(20) DEFAULT 'active',
			resource_quota JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS email VARCHAR(100)`,
		`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS phone VARCHAR(32)`,

		`CREATE TABLE IF NOT EXISTS nodes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			public_key VARCHAR(44) NOT NULL UNIQUE,
			machine_id VARCHAR(100) NOT NULL,
			tenant_id UUID REFERENCES tenants(id),
			endpoint VARCHAR(100),
			private_ip VARCHAR(45),
			public_ip VARCHAR(45),
			region VARCHAR(50),
			vpc_id VARCHAR(50),
			hostname VARCHAR(100),
			assigned_ip INET,
			ip_offset INT NOT NULL DEFAULT 0,
			last_seen BIGINT NOT NULL,
			registered_at BIGINT NOT NULL,
			role VARCHAR(20) DEFAULT 'spoke',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS state (
			key TEXT PRIMARY KEY,
			value INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS ip_pools (
			id SERIAL PRIMARY KEY,
			tenant_id UUID REFERENCES tenants(id),
			cidr CIDR NOT NULL,
			current_offset INT DEFAULT 1,
			is_active BOOLEAN DEFAULT true,
			UNIQUE(tenant_id, cidr)
		)`,

		`CREATE TABLE IF NOT EXISTS ip_allocations (
			id SERIAL PRIMARY KEY,
			pool_id INT REFERENCES ip_pools(id),
			node_id UUID REFERENCES nodes(id),
			ip_address INET NOT NULL,
			ip_offset INT NOT NULL,
			UNIQUE(pool_id, ip_offset),
			UNIQUE(node_id),
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS tunnel_links (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			src_node_id UUID REFERENCES nodes(id),
			dst_node_id UUID REFERENCES nodes(id),
			link_type VARCHAR(20),
			status VARCHAR(20) DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(src_node_id, dst_node_id)
		)`,

		`CREATE TABLE IF NOT EXISTS device_configs (
			id SERIAL PRIMARY KEY,
			node_id UUID REFERENCES nodes(id),
			version INT NOT NULL,
			config_body JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(node_id, version)
		)`,

		`CREATE TABLE IF NOT EXISTS tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			token VARCHAR(64) UNIQUE NOT NULL,
			tag VARCHAR(128),
			tenant_id UUID REFERENCES tenants(id),
			max_uses INTEGER NOT NULL DEFAULT 1,
			used_count INTEGER NOT NULL DEFAULT 0,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT NOW(),
			created_by VARCHAR(64),
			status VARCHAR(16) DEFAULT 'active',
			last_used_at TIMESTAMP,
			last_used_by VARCHAR(64)
		)`,

		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username VARCHAR(64) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			tenant_id UUID REFERENCES tenants(id),
			role VARCHAR(20) DEFAULT 'viewer',
			email VARCHAR(100),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			last_login TIMESTAMPTZ
		)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW()`,
		`ALTER TABLE users ALTER COLUMN tenant_id DROP NOT NULL`,

		`CREATE TABLE IF NOT EXISTS ip_groups (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			name VARCHAR(128) NOT NULL,
			description TEXT DEFAULT '',
			kind VARCHAR(16) NOT NULL DEFAULT 'custom',
			created_by UUID REFERENCES users(id) ON DELETE SET NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			CHECK (kind IN ('custom', 'inline', 'system')),
			UNIQUE (tenant_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS ip_group_members (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			group_id UUID NOT NULL REFERENCES ip_groups(id) ON DELETE CASCADE,
			cidr CIDR NOT NULL,
			note TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE (group_id, cidr)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_groups_tenant_kind ON ip_groups(tenant_id, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_group_members_tenant_group ON ip_group_members(tenant_id, group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_group_members_cidr ON ip_group_members USING gist(cidr inet_ops)`,

		`CREATE TABLE IF NOT EXISTS acl_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(100),
			tenant_id UUID REFERENCES tenants(id),
			src_net CIDR NOT NULL,
			dst_net CIDR NOT NULL,
			protocol SMALLINT NOT NULL DEFAULT 0,
			min_port INTEGER NOT NULL DEFAULT 0,
			max_port INTEGER NOT NULL DEFAULT 65535,
			direction VARCHAR(16) DEFAULT 'ingress',
			ports TEXT,
			enabled BOOLEAN DEFAULT true,
			priority INTEGER DEFAULT 100,
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS qos_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			node_id UUID NOT NULL REFERENCES nodes(id),
			src_cidr CIDR,
			dst_cidr CIDR,
			src_port INTEGER,
			dst_port INTEGER,
			protocol SMALLINT,
			bandwidth_mbps INTEGER NOT NULL,
			enabled BOOLEAN DEFAULT true,
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS blacklist_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			node_id UUID NOT NULL REFERENCES nodes(id),
			scope VARCHAR(16) NOT NULL,
			cidr CIDR,
			port INTEGER,
			enabled BOOLEAN DEFAULT true,
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			CHECK (scope IN ('src', 'dst', 'ports'))
		)`,

		`CREATE INDEX IF NOT EXISTS idx_nodes_public_key ON nodes(public_key)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_tenant_id ON nodes(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_ip_allocations_node_id ON ip_allocations(node_id)`,

		// Add runtime mode fields for capability detection
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS runtime_mode VARCHAR(16) DEFAULT 'kernel'`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS kernel_version VARCHAR(32)`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS has_aesni BOOLEAN DEFAULT false`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_runtime_mode ON nodes(runtime_mode)`,

		// Add status management fields for node lifecycle (阶段2)
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'online'`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS offline_since BIGINT`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status)`,

		// Add enrolled_with_token to track which token was used for registration
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS enrolled_with_token VARCHAR(64)`,
		`ALTER TABLE nodes ADD COLUMN IF NOT EXISTS advertised_routes TEXT[] DEFAULT '{}'::text[]`,

		// Add node references and action for policy management
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES nodes(id)`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS src_cidr CIDR`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS dst_cidr CIDR`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS dst_port INTEGER`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS src_node VARCHAR(100)`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS dst_node VARCHAR(100)`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS action VARCHAR(10) DEFAULT 'allow'`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS direction VARCHAR(16) DEFAULT 'ingress'`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS ports TEXT`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS src_group_id UUID REFERENCES ip_groups(id)`,
		`ALTER TABLE acl_rules ADD COLUMN IF NOT EXISTS dst_group_id UUID REFERENCES ip_groups(id)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_enabled ON acl_rules(enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_priority ON acl_rules(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_src_node ON acl_rules(src_node)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_dst_node ON acl_rules(dst_node)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_node_id ON acl_rules(node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_src_group ON acl_rules(tenant_id, src_group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_dst_group ON acl_rules(tenant_id, dst_group_id)`,

		// Add indexes for tenant isolation
		`CREATE INDEX IF NOT EXISTS idx_tokens_tenant_id ON tokens(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_acl_rules_tenant_id ON acl_rules(tenant_id)`,
		`DROP INDEX IF EXISTS idx_qos_rules_tenant_node_category`,
		`ALTER TABLE qos_rules DROP CONSTRAINT IF EXISTS qos_rules_category_check`,
		`ALTER TABLE qos_rules DROP COLUMN IF EXISTS category`,
		`CREATE INDEX IF NOT EXISTS idx_qos_rules_tenant_node ON qos_rules(tenant_id, node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qos_rules_enabled ON qos_rules(enabled)`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS direction VARCHAR(16) DEFAULT 'egress'`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS rate_bps BIGINT`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS burst_bytes BIGINT`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 100`,
		`ALTER TABLE qos_rules ALTER COLUMN priority SET DEFAULT 100`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS mode VARCHAR(16) DEFAULT 'auto'`,
		`ALTER TABLE qos_rules ALTER COLUMN mode SET DEFAULT 'auto'`,
		`ALTER TABLE qos_rules ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES ip_groups(id)`,
		`CREATE INDEX IF NOT EXISTS idx_qos_rules_group ON qos_rules(tenant_id, group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_blacklist_rules_tenant_node_scope ON blacklist_rules(tenant_id, node_id, scope)`,
		`CREATE INDEX IF NOT EXISTS idx_blacklist_rules_enabled ON blacklist_rules(enabled)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_unique ON users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id)`,

		// AI Audit Log - 记录 AI 对话和工具调用
		`CREATE TABLE IF NOT EXISTS ai_audit_logs (
			id SERIAL PRIMARY KEY,
			session_id VARCHAR(64) NOT NULL,
			user_message TEXT NOT NULL,
			ai_response TEXT,
			tool_name VARCHAR(100),
			tool_arguments JSONB,
			tool_result TEXT,
			tool_status VARCHAR(20) DEFAULT 'pending',
			execution_time_ms INTEGER,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_audit_session ON ai_audit_logs(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_audit_tool ON ai_audit_logs(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_audit_created ON ai_audit_logs(created_at)`,

		`CREATE TABLE IF NOT EXISTS agent_commands (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			node_public_key VARCHAR(44) NOT NULL,
			command VARCHAR(64) NOT NULL,
			params JSONB NOT NULL DEFAULT '{}'::jsonb,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			message TEXT,
			priority INTEGER NOT NULL DEFAULT 0,
			timeout_seconds INTEGER NOT NULL DEFAULT 30,
			result JSONB NOT NULL DEFAULT '{}'::jsonb,
			sent_at TIMESTAMPTZ,
			acknowledged_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS policy_deliveries (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			node_id UUID NOT NULL REFERENCES nodes(id),
			policy_domain VARCHAR(16) NOT NULL,
			policy_ref VARCHAR(255) NOT NULL,
			policy_name VARCHAR(255),
			action VARCHAR(32) NOT NULL,
			command_id UUID NOT NULL REFERENCES agent_commands(id) ON DELETE CASCADE,
			command_status VARCHAR(20) NOT NULL DEFAULT 'pending',
			last_error TEXT,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS node_control_states (
			node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			desired_state_version VARCHAR(64),
			desired_state_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			desired_state_updated_at TIMESTAMPTZ,
			applied_state_version VARCHAR(64),
			applied_state_updated_at TIMESTAMPTZ,
			observed_state VARCHAR(32) NOT NULL DEFAULT 'idle',
			observed_message TEXT,
			observed_at TIMESTAMPTZ,
			last_sync_at TIMESTAMPTZ,
			last_sync_error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS node_policy_stats (
			node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			stats JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_control_states_unique_node ON node_control_states(node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_node_policy_stats_tenant_node ON node_policy_stats(tenant_id, node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_commands_node_status ON agent_commands(node_public_key, status)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_commands_created_at ON agent_commands(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_policy_deliveries_tenant_node_domain_ref ON policy_deliveries(tenant_id, node_id, policy_domain, policy_ref)`,
		`CREATE INDEX IF NOT EXISTS idx_policy_deliveries_command_id ON policy_deliveries(command_id)`,
		`CREATE INDEX IF NOT EXISTS idx_policy_deliveries_created_at ON policy_deliveries(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_node_control_states_tenant_node ON node_control_states(tenant_id, node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_node_control_states_desired_version ON node_control_states(desired_state_version)`,

		// Alerts table for monitoring closure
		`CREATE TABLE IF NOT EXISTS alerts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			node_id UUID REFERENCES nodes(id),
			alert_type VARCHAR(32) NOT NULL,
			severity VARCHAR(16) NOT NULL,
			title VARCHAR(255) NOT NULL,
			message TEXT,
			context JSONB NOT NULL DEFAULT '{}'::jsonb,
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			resolved_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_tenant_status ON alerts(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_tenant_node_type ON alerts(tenant_id, node_id, alert_type)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at)`,

		// Audit events table for monitoring closure
		`CREATE TABLE IF NOT EXISTS audit_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id),
			node_id UUID REFERENCES nodes(id),
			event_type VARCHAR(32) NOT NULL,
			actor VARCHAR(128) NOT NULL DEFAULT 'system',
			summary VARCHAR(512) NOT NULL,
			detail JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_tenant ON audit_events(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_node ON audit_events(tenant_id, node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_type ON audit_events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at)`,
		`CREATE TABLE IF NOT EXISTS roles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			name VARCHAR(64) NOT NULL,
			description TEXT DEFAULT '',
			is_system BOOLEAN DEFAULT FALSE,
			permissions TEXT[] NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(tenant_id, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_roles_tenant ON roles(tenant_id)`,
		`CREATE TABLE IF NOT EXISTS node_certificates (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			node_id UUID NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
			serial_number VARCHAR(128) NOT NULL,
			cert_pem TEXT NOT NULL,
			ca_pem TEXT NOT NULL,
			not_before TIMESTAMPTZ NOT NULL,
			not_after TIMESTAMPTZ NOT NULL,
			status VARCHAR(16) NOT NULL DEFAULT 'issued',
			issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			revoked_at TIMESTAMPTZ,
			revoke_reason TEXT,
			renewed_from UUID REFERENCES node_certificates(id),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_node_certificates_tenant ON node_certificates(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_node_certificates_status ON node_certificates(status)`,
		`CREATE INDEX IF NOT EXISTS idx_node_certificates_not_after ON node_certificates(not_after)`,
	}

	for i, migration := range migrations {
		_, err := s.db.Exec(migration)
		if err != nil {
			return fmt.Errorf("migration %d failed: %v\nSQL: %s", i+1, err, migration)
		}
	}

	log.Println("Database migrations completed successfully")
	return nil
}

type Node struct {
	ID                uuid.UUID
	PublicKey         string
	MachineID         string
	TenantID          uuid.UUID
	Endpoint          string
	PrivateIP         string
	PublicIP          string
	Region            string
	VPCID             string
	Hostname          string
	AssignedIP        string
	IPOffset          int
	LastSeen          int64
	RegisteredAt      int64
	Role              string
	RuntimeMode       string
	KernelVersion     string
	HasAESNI          bool
	Status            string   // online, offline, stale, deleted
	OfflineSince      int64    // timestamp when node went offline
	AdvertisedRoutes  []string // Site-to-Site VPN: 宣告的网段列表
	EnrolledWithToken string   // token used for registration
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (s *Storage) GetOrCreateTenant(name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(
		`INSERT INTO tenants (code, name) VALUES ($1, $2) ON CONFLICT (code) DO UPDATE SET name = $2 RETURNING id`,
		name, name,
	).Scan(&id)
	return id, err
}

// GetTenantIDByToken retrieves the tenant ID associated with a token
func (s *Storage) GetTenantIDByToken(tokenStr string) (uuid.UUID, error) {
	var tenantID uuid.UUID
	err := s.db.QueryRow(
		`SELECT tenant_id FROM tokens WHERE token = $1`,
		tokenStr,
	).Scan(&tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	return tenantID, nil
}

// GetTenantInfo retrieves tenant information by ID
func (s *Storage) GetTenantInfo(tenantID uuid.UUID) (*TenantInfo, error) {
	var info TenantInfo
	var code sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, code, status, resource_quota, created_at, updated_at FROM tenants WHERE id = $1`,
		tenantID,
	).Scan(&info.ID, &info.Name, &code, &info.Status, &info.ResourceQuota, &info.CreatedAt, &info.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if code.Valid {
		info.Code = code.String
	}
	return &info, nil
}

func (s *Storage) GetTenantStatus(tenantID uuid.UUID) (string, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM tenants WHERE id = $1`, tenantID).Scan(&status)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(status), nil
}

// GetNodesByTenant retrieves all nodes for a specific tenant
func (s *Storage) GetNodesByTenant(tenantID uuid.UUID) ([]*Node, error) {
	return s.getNodes("WHERE tenant_id = $1 AND status != 'deleted'", []interface{}{tenantID}, "")
}

// GetNodeByTenant retrieves a specific node for a specific tenant (ensures tenant isolation)
func (s *Storage) GetNodeByTenant(publicKey string, tenantID uuid.UUID) (*Node, error) {
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 AND tenant_id = $2`
	row := s.db.QueryRow(query, publicKey, tenantID)
	return s.scanNode(row)
}

// GetOrCreateTenantByCode retrieves or creates a tenant by code
func (s *Storage) GetOrCreateTenantByCode(code, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(
		`INSERT INTO tenants (code, name) VALUES ($1, $2) ON CONFLICT (code) DO UPDATE SET name = $2 RETURNING id`,
		code, name,
	).Scan(&id)
	return id, err
}

func (s *Storage) SaveNode(node *Node) error {
	if node == nil {
		return fmt.Errorf("node cannot be nil")
	}

	query := `
		INSERT INTO nodes (public_key, machine_id, tenant_id, endpoint, private_ip, public_ip,
			region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role,
			runtime_mode, kernel_version, has_aesni, advertised_routes, enrolled_with_token, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, 'online', NOW())
		ON CONFLICT (public_key) DO UPDATE SET
			machine_id = $2,
			endpoint = $4,
			private_ip = $5,
			public_ip = $6,
			region = $7,
			vpc_id = $8,
			hostname = $9,
			assigned_ip = $10,
			ip_offset = $11,
			last_seen = $12,
			registered_at = $13,
			role = COALESCE(NULLIF($14, ''), nodes.role),
			runtime_mode = $15,
			kernel_version = $16,
			has_aesni = $17,
			advertised_routes = $18,
			enrolled_with_token = COALESCE(nodes.enrolled_with_token, $19),
			status = 'online',
			offline_since = NULL,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRow(query,
		node.PublicKey, node.MachineID, node.TenantID, node.Endpoint,
		node.PrivateIP, node.PublicIP, node.Region, node.VPCID, node.Hostname,
		node.AssignedIP, node.IPOffset, node.LastSeen, node.RegisteredAt, node.Role,
		node.RuntimeMode, node.KernelVersion, node.HasAESNI, pq.Array(node.AdvertisedRoutes),
		node.EnrolledWithToken,
	).Scan(&node.ID, &node.CreatedAt, &node.UpdatedAt)
	if err != nil {
		return err
	}
	node.Status = "online"
	node.OfflineSince = 0
	return nil
}

func (s *Storage) UpdateNodeHeartbeat(nodeID uuid.UUID, lastSeen int64) error {
	result, err := s.db.Exec(`
		UPDATE nodes
		SET last_seen = $2,
		    status = 'online',
		    offline_since = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND status NOT IN ('deleted', 'suspended', 'banned')
	`, nodeID, lastSeen)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Storage) UpdateNodePublicIdentity(nodeID uuid.UUID, publicIP, endpoint string) error {
	publicIP = strings.TrimSpace(publicIP)
	endpoint = strings.TrimSpace(endpoint)
	if publicIP == "" && endpoint == "" {
		return nil
	}

	_, err := s.db.Exec(`
		UPDATE nodes
		SET public_ip = COALESCE(NULLIF($2, ''), public_ip),
		    endpoint = COALESCE(NULLIF($3, ''), endpoint),
		    private_ip = '',
		    updated_at = NOW()
		WHERE id = $1
		  AND status NOT IN ('deleted', 'suspended', 'banned')
		  AND (
		      ($2 <> '' AND COALESCE(public_ip, '') <> $2)
		   OR ($3 <> '' AND COALESCE(endpoint, '') <> $3)
		   OR COALESCE(private_ip, '') <> ''
		  )
	`, nodeID, publicIP, endpoint)
	return err
}

func (s *Storage) GetNode(publicKey string) (*Node, error) {
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`
	row := s.db.QueryRow(query, publicKey)
	return s.scanNode(row)
}

func (s *Storage) GetNodeByID(nodeID uuid.UUID) (*Node, error) {
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`
	row := s.db.QueryRow(query, nodeID)
	return s.scanNode(row)
}

func (s *Storage) GetNodeByHostname(hostname string) (*Node, error) {
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE hostname = $1`
	row := s.db.QueryRow(query, hostname)
	return s.scanNode(row)
}

func (s *Storage) GetNodeByHostnameForTenant(hostname string, tenantID uuid.UUID) (*Node, error) {
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE hostname = $1 AND tenant_id = $2 AND status != 'deleted'`
	row := s.db.QueryRow(query, hostname, tenantID)
	return s.scanNode(row)
}

const nodeSelectColumns = `id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at`

// getNodes is a helper that queries nodes with optional WHERE clause and args.
// extraWhere should include the "WHERE" keyword if non-empty, e.g. "WHERE status != 'deleted'".
func (s *Storage) getNodes(extraWhere string, args []interface{}, orderBy string) ([]*Node, error) {
	query := `SELECT ` + nodeSelectColumns + ` FROM nodes`
	if extraWhere != "" {
		query += " " + extraWhere
	}
	if orderBy != "" {
		query += " " + orderBy
	} else {
		query += " ORDER BY last_seen DESC"
	}

	rows, err := s.db.Query(query, args...)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Storage) GetAllNodes() ([]*Node, error) {
	return s.getNodes("", nil, "WHERE status != 'deleted'")
}

// GetAllNodesIncludeDeleted returns all nodes including deleted ones (for audit/admin purposes)
func (s *Storage) GetAllNodesIncludeDeleted() ([]*Node, error) {
	return s.getNodes("", nil, "")
}

// ReuseHostnameIP atomically finds a node by hostname within a tenant, marks it deleted,
// and returns its assigned_ip and ip_offset for reuse. Uses SELECT FOR UPDATE
// to prevent concurrent hostname reuse races. Returns sql.ErrNoRows if not found.
func (s *Storage) ReuseHostnameIP(hostname string, tenantID uuid.UUID) (assignedIP string, ipOffset int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", 0, err
	}
	defer rollbackTx(tx, "ReuseHostnameIP")

	var pubKey string
	var nodeID uuid.UUID
	var status string
	err = tx.QueryRow(
		`SELECT id, public_key, assigned_ip, ip_offset, COALESCE(status, 'online') FROM nodes WHERE hostname = $1 AND tenant_id = $2 AND status != 'deleted' FOR UPDATE LIMIT 1`,
		hostname, tenantID,
	).Scan(&nodeID, &pubKey, &assignedIP, &ipOffset, &status)
	if err != nil {
		return "", 0, err
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "suspended", "banned":
		return "", 0, fmt.Errorf("inactive hostname %q cannot be reused: node status %s", hostname, status)
	}

	_, err = tx.Exec(`UPDATE nodes SET status = 'deleted', updated_at = NOW() WHERE public_key = $1`, pubKey)
	if err != nil {
		return "", 0, err
	}

	if _, err := revokeNodeCertificatesTx(tx, nodeID, "hostname_reused"); err != nil {
		return "", 0, err
	}
	if err := failIncompleteAgentCommandsForNodeTx(tx, pubKey, "node deleted for hostname reuse"); err != nil {
		return "", 0, err
	}

	return assignedIP, ipOffset, tx.Commit()
}

func (s *Storage) DeleteNode(publicKey string) error {
	query := `DELETE FROM nodes WHERE public_key = $1`
	_, err := s.db.Exec(query, publicKey)
	return err
}

// ============= 节点状态管理（阶段2）=============

// MarkOfflineNodes 标记超过阈值时间无心跳的节点为 offline
func (s *Storage) MarkOfflineNodes(thresholdTimestamp int64) (int, error) {
	query := `
		UPDATE nodes
		SET status = 'offline',
		    offline_since = CASE WHEN offline_since IS NULL THEN $1 ELSE offline_since END
		WHERE last_seen < $1 AND status = 'online'
	`
	result, err := s.db.Exec(query, thresholdTimestamp)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

// MarkStaleNodes 标记长期离线的节点为 stale
func (s *Storage) MarkStaleNodes(thresholdTimestamp int64) (int, error) {
	query := `
		UPDATE nodes
		SET status = 'stale'
		WHERE offline_since < $1 AND status = 'offline'
	`
	result, err := s.db.Exec(query, thresholdTimestamp)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

// MarkNodeDeleted 手动删除节点（标记为deleted状态）
func (s *Storage) MarkNodeDeleted(publicKey string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer rollbackTx(tx, "MarkNodeDeleted")

	var nodeID uuid.UUID
	err = tx.QueryRow(`SELECT id FROM nodes WHERE public_key = $1 FOR UPDATE`, publicKey).Scan(&nodeID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE nodes
		SET status = 'deleted', updated_at = NOW()
		WHERE public_key = $1
	`, publicKey); err != nil {
		return err
	}
	if _, err := revokeNodeCertificatesTx(tx, nodeID, "node_deleted"); err != nil {
		return err
	}
	if err := failIncompleteAgentCommandsForNodeTx(tx, publicKey, "node deleted"); err != nil {
		return err
	}

	return tx.Commit()
}

// CleanupDeletedNodes 清理已标记为 deleted 超过指定时间的节点
func (s *Storage) CleanupDeletedNodes(thresholdTimestamp int64) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(tx, "CleanupDeletedNodes")
		}
	}()

	rows, err := tx.Query(`
		SELECT id
		FROM nodes
		WHERE status = 'deleted' AND updated_at < to_timestamp($1)
		FOR UPDATE
	`, thresholdTimestamp)
	if err != nil {
		return 0, err
	}

	nodeIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var nodeID uuid.UUID
		if err := rows.Scan(&nodeID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	if len(nodeIDs) == 0 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		committed = true
		return 0, nil
	}

	if err := detachDeletedNodeHistoryTx(tx, nodeIDs); err != nil {
		return 0, err
	}
	if err := purgeDeletedNodeDependentsTx(tx, nodeIDs); err != nil {
		return 0, err
	}

	result, err := tx.Exec(`
		DELETE FROM nodes
		WHERE id = ANY($1)
	`, pq.Array(nodeIDs))
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return int(count), nil
}

func detachDeletedNodeHistoryTx(tx roleExec, nodeIDs []uuid.UUID) error {
	for _, query := range []string{
		`UPDATE audit_events SET node_id = NULL WHERE node_id = ANY($1)`,
		`UPDATE alerts SET node_id = NULL WHERE node_id = ANY($1)`,
	} {
		if _, err := tx.Exec(query, pq.Array(nodeIDs)); err != nil {
			return err
		}
	}
	return nil
}

func purgeDeletedNodeDependentsTx(tx roleExec, nodeIDs []uuid.UUID) error {
	for _, query := range []string{
		`DELETE FROM policy_deliveries WHERE node_id = ANY($1)`,
		`DELETE FROM acl_rules WHERE node_id = ANY($1)`,
		`DELETE FROM qos_rules WHERE node_id = ANY($1)`,
		`DELETE FROM blacklist_rules WHERE node_id = ANY($1)`,
		`DELETE FROM device_configs WHERE node_id = ANY($1)`,
		`DELETE FROM tunnel_links WHERE src_node_id = ANY($1) OR dst_node_id = ANY($1)`,
		`DELETE FROM ip_allocations WHERE node_id = ANY($1)`,
	} {
		if _, err := tx.Exec(query, pq.Array(nodeIDs)); err != nil {
			return err
		}
	}
	return nil
}

// GetNodesGoingOffline returns nodes that are about to be marked offline:
// last_seen older than thresholdSeconds ago AND status is not already 'offline'/'stale'/'deleted'.
func (s *Storage) GetNodesGoingOffline(thresholdSeconds int) ([]*Node, error) {
	threshold := time.Now().Unix() - int64(thresholdSeconds)
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE last_seen < $1 AND status = 'online'`
	rows, err := s.db.Query(query, threshold)
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
	return nodes, rows.Err()
}

// GetNodesRecovering returns nodes that have recovered: last_seen within thresholdSeconds AND status is 'offline'.
func (s *Storage) GetNodesRecovering(thresholdSeconds int) ([]*Node, error) {
	threshold := time.Now().Unix() - int64(thresholdSeconds)
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE last_seen >= $1 AND status = 'offline'`
	rows, err := s.db.Query(query, threshold)
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
	return nodes, rows.Err()
}

// MarkNodeOnline updates a single node's status to 'online' and clears offline_since.
func (s *Storage) MarkNodeOnline(nodeID uuid.UUID) error {
	query := `UPDATE nodes SET status = 'online', offline_since = NULL, updated_at = NOW() WHERE id = $1`
	_, err := s.db.Exec(query, nodeID)
	return err
}

// CleanupStaleNodes 执行完整的节点状态维护流程
// thresholdSeconds 参数已废弃，改用固定的时间策略
func (s *Storage) CleanupStaleNodes(thresholdSeconds int64) (int, error) {
	now := time.Now().Unix()

	// 1. 标记离线节点 (>30秒无心跳)
	offlineCount, err := s.MarkOfflineNodes(now - 30)
	if err != nil {
		return 0, fmt.Errorf("failed to mark offline nodes: %v", err)
	}

	// 2. 标记长期离线节点 (>24小时)
	staleCount, err := s.MarkStaleNodes(now - 86400)
	if err != nil {
		return 0, fmt.Errorf("failed to mark stale nodes: %v", err)
	}

	// 3. 清理已删除节点 (>7天)
	deletedCount, err := s.CleanupDeletedNodes(now - 604800)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup deleted nodes: %v", err)
	}

	totalAffected := offlineCount + staleCount + deletedCount
	if totalAffected > 0 {
		log.Printf("Node status cleanup: %d offline, %d stale, %d deleted",
			offlineCount, staleCount, deletedCount)
	}

	return totalAffected, nil
}

func (s *Storage) GetNextOffset() (int, error) {
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
	query := `INSERT INTO state (key, value) VALUES ('next_offset', $1)
	          ON CONFLICT (key) DO UPDATE SET value = $1`
	_, err := s.db.Exec(query, offset)
	return err
}

func (s *Storage) IncrementNextOffset() (int, error) {
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
	query := `SELECT COUNT(*) FROM nodes WHERE ip_offset = $1`
	var count int
	err := s.db.QueryRow(query, offset).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Storage) GetNextAvailableOffset() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer rollbackTx(tx, "GetNextAvailableOffset")

	// Lock the state row with SELECT FOR UPDATE to prevent concurrent reads
	var current int
	err = tx.QueryRow(
		`SELECT value FROM state WHERE key = 'next_offset' FOR UPDATE`,
	).Scan(&current)
	if err == sql.ErrNoRows {
		// Initialize the state row if it doesn't exist
		_, err = tx.Exec(`INSERT INTO state (key, value) VALUES ('next_offset', 1) ON CONFLICT (key) DO NOTHING`)
		if err != nil {
			return 0, fmt.Errorf("failed to initialize next_offset: %v", err)
		}
		current = 1
		// Re-acquire the lock
		err = tx.QueryRow(
			`SELECT value FROM state WHERE key = 'next_offset' FOR UPDATE`,
		).Scan(&current)
		if err != nil {
			return 0, fmt.Errorf("failed to lock next_offset: %v", err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("failed to read next_offset: %v", err)
	}

	// Find the next available offset using a single SQL query
	// For /16 network: offset range is 1 to 65534 (100.64.0.1 ~ 100.64.255.254)
	const maxOffset = 65534
	var foundOffset int = -1

	err = tx.QueryRow(`
		SELECT o FROM generate_series(1, $2) AS o
		WHERE NOT EXISTS (SELECT 1 FROM nodes WHERE ip_offset = o AND status != 'deleted')
		ORDER BY CASE WHEN o >= $1 THEN o - $1 ELSE o - $1 + $2 END, o
		LIMIT 1`,
		current, maxOffset,
	).Scan(&foundOffset)
	if err != nil {
		return 0, fmt.Errorf("no available IP offset (all %d offsets in use): %v", maxOffset, err)
	}

	if foundOffset == -1 {
		return 0, fmt.Errorf("no available IP offset (all %d offsets in use)", maxOffset)
	}

	// Update next_offset to the one after the found offset
	newOffset := foundOffset + 1
	if newOffset > maxOffset {
		newOffset = 1
	}
	_, err = tx.Exec(
		`INSERT INTO state (key, value) VALUES ('next_offset', $1) ON CONFLICT (key) DO UPDATE SET value = $1`,
		newOffset,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update next_offset: %v", err)
	}

	// Commit the transaction - this releases the row lock
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %v", err)
	}

	return foundOffset, nil
}

func (s *Storage) GetNodeCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM nodes`
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// ScanNode is a public wrapper for the private scanNode method
func (s *Storage) ScanNode(row *sql.Row) (*Node, error) {
	return s.scanNode(row)
}

// ScanNodeRows is a public wrapper for the private scanNodeRows method
func (s *Storage) ScanNodeRows(rows *sql.Rows) (*Node, error) {
	return s.scanNodeRows(rows)
}

func (s *Storage) scanNode(row *sql.Row) (*Node, error) {
	node := &Node{}
	var advertisedRoutes pq.StringArray
	err := row.Scan(
		&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID,
		&node.Endpoint, &node.PrivateIP, &node.PublicIP, &node.Region,
		&node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset,
		&node.LastSeen, &node.RegisteredAt, &node.Role,
		&node.RuntimeMode, &node.KernelVersion, &node.HasAESNI,
		&node.Status, &node.OfflineSince,
		&advertisedRoutes, &node.EnrolledWithToken,
		&node.CreatedAt, &node.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	node.AdvertisedRoutes = []string(advertisedRoutes)
	return node, nil
}

func (s *Storage) scanNodeRows(rows *sql.Rows) (*Node, error) {
	node := &Node{}
	var advertisedRoutes pq.StringArray
	err := rows.Scan(
		&node.ID, &node.PublicKey, &node.MachineID, &node.TenantID,
		&node.Endpoint, &node.PrivateIP, &node.PublicIP, &node.Region,
		&node.VPCID, &node.Hostname, &node.AssignedIP, &node.IPOffset,
		&node.LastSeen, &node.RegisteredAt, &node.Role,
		&node.RuntimeMode, &node.KernelVersion, &node.HasAESNI,
		&node.Status, &node.OfflineSince,
		&advertisedRoutes, &node.EnrolledWithToken,
		&node.CreatedAt, &node.UpdatedAt,
	)
	node.AdvertisedRoutes = []string(advertisedRoutes)
	return node, err
}

type RegisterDeviceRequest struct {
	TenantID  uuid.UUID
	PublicKey string
	MachineID string
	Provider  string
	Region    string
	VPCID     string
	PublicIP  string
	Hostname  string
}

type IPPool struct {
	ID            int
	TenantID      uuid.UUID
	CIDR          string
	CurrentOffset int
	IsActive      bool
}

func (s *Storage) CreateIPPool(tenantID uuid.UUID, cidr string) (*IPPool, error) {
	var pool IPPool
	err := s.db.QueryRow(
		`INSERT INTO ip_pools (tenant_id, cidr) VALUES ($1, $2) ON CONFLICT (tenant_id, cidr) DO UPDATE SET is_active = true RETURNING *`,
		tenantID, cidr,
	).Scan(&pool.ID, &pool.TenantID, &pool.CIDR, &pool.CurrentOffset, &pool.IsActive)
	return &pool, err
}

func (s *Storage) AllocateIP(tenantID uuid.UUID) (string, int, error) {
	var ip string
	var offset int

	if s.heartbeat != nil {
		lock, err := s.heartbeat.AcquireIPAMLock(tenantID.String())
		if err != nil {
			return "", 0, fmt.Errorf("failed to acquire IPAM lock: %v", err)
		}
		defer lock.Release()

		ip, offset, err = s.allocateIPInternal(tenantID)
		if err != nil {
			return "", 0, err
		}

		return ip, offset, nil
	}

	return s.allocateIPInternal(tenantID)
}

func (s *Storage) allocateIPInternal(tenantID uuid.UUID) (string, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", 0, err
	}
	defer rollbackTx(tx, "allocateIPInternal")

	row := tx.QueryRow(
		`SELECT id, cidr, current_offset FROM ip_pools 
		 WHERE tenant_id = $1 AND is_active = true 
		 FOR UPDATE`,
		tenantID,
	)

	var poolID int
	var poolCIDR string
	var currentOffset int
	if err := row.Scan(&poolID, &poolCIDR, &currentOffset); err != nil {
		return "", 0, fmt.Errorf("no active IP pool for tenant: %v", err)
	}

	offset := currentOffset + 1
	ip := s.calculateIP(poolCIDR, offset)

	_, err = tx.Exec(
		`UPDATE ip_pools SET current_offset = $1 WHERE id = $2`,
		offset, poolID,
	)
	if err != nil {
		return "", 0, err
	}

	if err := tx.Commit(); err != nil {
		return "", 0, err
	}

	return ip, offset, nil
}

func (s *Storage) calculateIP(cidr string, offset int) string {
	return fmt.Sprintf("100.64.0.%d", offset)
}

func (s *Storage) AssignIPToDevice(deviceID uuid.UUID, ip string, offset int) error {
	_, err := s.db.Exec(
		`UPDATE nodes SET assigned_ip = $1, ip_offset = $2 WHERE id = $3`,
		ip, offset, deviceID,
	)
	return err
}

func (s *Storage) SaveDeviceConfig(deviceID uuid.UUID, version int, configBody map[string]interface{}) error {
	configJSON, err := json.Marshal(configBody)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO device_configs (node_id, version, config_body) VALUES ($1, $2, $3)
		 ON CONFLICT (node_id, version) DO UPDATE SET config_body = $3`,
		deviceID, version, configJSON,
	)
	return err
}

func (s *Storage) GetDeviceConfig(deviceID uuid.UUID, version int) (map[string]interface{}, error) {
	var configBody []byte
	err := s.db.QueryRow(
		`SELECT config_body FROM device_configs WHERE node_id = $1 AND version = $2`,
		deviceID, version,
	).Scan(&configBody)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configBody, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (s *Storage) GetLatestConfigVersion(deviceID uuid.UUID) (int, error) {
	var version int
	err := s.db.QueryRow(
		`SELECT MAX(version) FROM device_configs WHERE node_id = $1`,
		deviceID,
	).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return version, err
}

func (s *Storage) SaveTunnelLink(srcDeviceID, dstDeviceID uuid.UUID, linkType string) error {
	_, err := s.db.Exec(
		`INSERT INTO tunnel_links (src_node_id, dst_node_id, link_type) VALUES ($1, $2, $3)
		 ON CONFLICT (src_node_id, dst_node_id) DO UPDATE SET link_type = $3`,
		srcDeviceID, dstDeviceID, linkType,
	)
	return err
}

func (s *Storage) GetTunnelLinks(deviceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.Query(
		`SELECT CASE 
			WHEN src_node_id = $1 THEN dst_node_id 
			ELSE src_node_id 
		 END as peer_id
		 FROM tunnel_links 
		 WHERE (src_node_id = $1 OR dst_node_id = $1) AND status = 'active'`,
		deviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peerIDs []uuid.UUID
	for rows.Next() {
		var peerID uuid.UUID
		if err := rows.Scan(&peerID); err != nil {
			return nil, err
		}
		peerIDs = append(peerIDs, peerID)
	}
	return peerIDs, nil
}

func (s *Storage) SetBaseIP(baseIP string) {
	s.baseIP = baseIP
}

func (s *Storage) SetCIDR(cidr string) {
	s.cidr = cidr
}

func (s *Storage) GetBaseIP() string {
	return s.baseIP
}

func (s *Storage) GetCIDR() string {
	return s.cidr
}

// AIAuditLog represents an AI audit log entry
type AIAuditLog struct {
	ID              int64
	SessionID       string
	UserMessage     string
	AIResponse      string
	ToolName        string
	ToolArguments   map[string]interface{}
	ToolResult      string
	ToolStatus      string
	ExecutionTimeMs int
	CreatedAt       time.Time
}

// SaveAIAuditLog saves an AI audit log entry
func (s *Storage) SaveAIAuditLog(log *AIAuditLog) error {
	// Serialize tool arguments as JSON
	var toolArgsJSON []byte
	if log.ToolArguments != nil {
		var err error
		toolArgsJSON, err = json.Marshal(log.ToolArguments)
		if err != nil {
			toolArgsJSON = []byte("{}")
		}
	} else {
		toolArgsJSON = []byte("{}")
	}

	query := `
		INSERT INTO ai_audit_logs (session_id, user_message, ai_response, tool_name, tool_arguments, tool_result, tool_status, execution_time_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.Exec(query,
		log.SessionID,
		log.UserMessage,
		log.AIResponse,
		log.ToolName,
		toolArgsJSON,
		log.ToolResult,
		log.ToolStatus,
		log.ExecutionTimeMs,
	)
	return err
}

// GetAIAuditLogs retrieves AI audit logs with optional filters
func (s *Storage) GetAIAuditLogs(sessionID string, limit int) ([]AIAuditLog, error) {
	query := `
		SELECT id, session_id, user_message, ai_response, tool_name, tool_arguments, tool_result, tool_status, execution_time_ms, created_at
		FROM ai_audit_logs
		WHERE ($1 = '' OR session_id = $1)
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AIAuditLog
	for rows.Next() {
		var log AIAuditLog
		var toolArgsJSON []byte
		var toolName, toolResult, toolStatus, aiResponse sql.NullString
		var execTimeMs sql.NullInt64

		err := rows.Scan(&log.ID, &log.SessionID, &log.UserMessage, &aiResponse, &toolName, &toolArgsJSON, &toolResult, &toolStatus, &execTimeMs, &log.CreatedAt)
		if err != nil {
			continue
		}

		log.AIResponse = aiResponse.String
		log.ToolName = toolName.String
		log.ToolResult = toolResult.String
		log.ToolStatus = toolStatus.String
		if execTimeMs.Valid {
			log.ExecutionTimeMs = int(execTimeMs.Int64)
		}

		if toolArgsJSON != nil {
			json.Unmarshal(toolArgsJSON, &log.ToolArguments)
		}

		logs = append(logs, log)
	}

	return logs, rows.Err()
}
