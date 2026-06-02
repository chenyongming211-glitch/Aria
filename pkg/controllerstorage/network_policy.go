package controllerstorage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	QoSCategoryService = "service"
	QoSCategoryPeers   = "peers"
	QoSCategoryIP      = "ip"

	BlacklistScopeSrc   = "src"
	BlacklistScopeDst   = "dst"
	BlacklistScopePorts = "ports"
)

type QoSRuleRecord struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	NodeID        uuid.UUID `json:"node_id"`
	Category      string    `json:"category"`
	SrcCIDR       string    `json:"src_cidr"`
	DstCIDR       string    `json:"dst_cidr"`
	SrcPort       int       `json:"src_port"`
	DstPort       int       `json:"dst_port"`
	Protocol      int       `json:"protocol"`
	BandwidthMbps int       `json:"bandwidth_mbps"`
	Enabled       bool      `json:"enabled"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ACLRuleRecord struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	NodeID      uuid.UUID `json:"node_id"`
	Name        string    `json:"name"`
	Action      string    `json:"action"` // "allow", "deny"
	SrcCIDR     string    `json:"src_cidr"`
	DstCIDR     string    `json:"dst_cidr"`
	DstPort     int       `json:"dst_port"`
	Protocol    int       `json:"protocol"`
	Priority    int       `json:"priority"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BlacklistRuleRecord struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	NodeID      uuid.UUID
	Scope       string
	CIDR        string
	Port        int
	Enabled     bool
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s *Storage) ListTenantNodeQoSRules(tenantID, nodeID uuid.UUID, category string) ([]*QoSRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND category = $3
		  ORDER BY created_at DESC`,
		tenantID, nodeID, category,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*QoSRuleRecord
	for rows.Next() {
		rule, err := scanQoSRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Storage) CreateTenantNodeQoSRule(rule *QoSRuleRecord) (*QoSRuleRecord, error) {
	created := &QoSRuleRecord{}
	err := s.db.QueryRow(
		`INSERT INTO qos_rules (tenant_id, node_id, category, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11)
		 RETURNING id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at`,
		rule.TenantID,
		rule.NodeID,
		rule.Category,
		strings.TrimSpace(rule.SrcCIDR),
		strings.TrimSpace(rule.DstCIDR),
		nullableInt(rule.SrcPort),
		nullableInt(rule.DstPort),
		nullableInt(rule.Protocol),
		rule.BandwidthMbps,
		rule.Enabled,
		strings.TrimSpace(rule.Description),
	).Scan(
		&created.ID,
		&created.TenantID,
		&created.NodeID,
		&created.Category,
		&created.SrcCIDR,
		&created.DstCIDR,
		&created.SrcPort,
		&created.DstPort,
		&created.Protocol,
		&created.BandwidthMbps,
		&created.Enabled,
		&created.Description,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// 自动提升版本
	if err := s.bumpNodeDesiredVersion(rule.TenantID, rule.NodeID); err != nil {
		return nil, err
	}

	return created, nil
}

func (s *Storage) DeleteTenantNodeQoSRule(tenantID, nodeID uuid.UUID, category string, ruleID uuid.UUID) error {
	result, err := s.db.Exec(
		`DELETE FROM qos_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND category = $4`,
		ruleID, tenantID, nodeID, category,
	)
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

	// 自动提升版本
	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return err
	}

	return nil
}

func (s *Storage) UpdateTenantNodeQoSRule(tenantID, nodeID, ruleID uuid.UUID, category string, rule *QoSRuleRecord) (*QoSRuleRecord, error) {
	result, err := s.db.Exec(
		`UPDATE qos_rules SET
			src_cidr = NULLIF($5, ''), dst_cidr = NULLIF($6, ''),
			src_port = $7, dst_port = $8, protocol = $9,
			bandwidth_mbps = $10, description = $11,
			enabled = $12, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND category = $4`,
		ruleID, tenantID, nodeID, category,
		rule.SrcCIDR, rule.DstCIDR,
		rule.SrcPort, rule.DstPort, rule.Protocol,
		rule.BandwidthMbps, rule.Description, rule.Enabled,
	)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, sql.ErrNoRows
	}

	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return nil, err
	}

	rule.ID = ruleID
	rule.TenantID = tenantID
	rule.NodeID = nodeID
	rule.Category = category
	rule.UpdatedAt = time.Now()
	return rule, nil
}

// ACL Methods

func (s *Storage) ListTenantNodeACLRules(tenantID, nodeID uuid.UUID) ([]*ACLRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0), priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority DESC, created_at DESC`,
		tenantID, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*ACLRuleRecord
	for rows.Next() {
		rule := &ACLRuleRecord{}
		err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.NodeID, &rule.Name, &rule.Action, &rule.SrcCIDR, &rule.DstCIDR,
			&rule.DstPort, &rule.Protocol, &rule.Priority, &rule.Enabled, &rule.Description,
			&rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Storage) CreateTenantNodeACLRule(rule *ACLRuleRecord) (*ACLRuleRecord, error) {
	created := &ACLRuleRecord{}
	srcNet := aclNetworkForSync(rule.SrcCIDR)
	dstNet := aclNetworkForSync(rule.DstCIDR)
	minPort, maxPort := aclPortRangeForSync(rule.DstPort)

	err := s.db.QueryRow(
		`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12::cidr, $13::cidr, $14, $15)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0), priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`,
		rule.TenantID, rule.NodeID, strings.TrimSpace(rule.Name), rule.Action,
		strings.TrimSpace(rule.SrcCIDR), strings.TrimSpace(rule.DstCIDR),
		nullableInt(rule.DstPort), rule.Protocol, rule.Priority,
		rule.Enabled, strings.TrimSpace(rule.Description), srcNet, dstNet, minPort, maxPort,
	).Scan(
		&created.ID, &created.TenantID, &created.NodeID, &created.Name, &created.Action, &created.SrcCIDR, &created.DstCIDR,
		&created.DstPort, &created.Protocol, &created.Priority, &created.Enabled, &created.Description,
		&created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := s.bumpNodeDesiredVersion(rule.TenantID, rule.NodeID); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Storage) DeleteTenantNodeACLRuleByID(tenantID, nodeID uuid.UUID, ruleID uuid.UUID) error {
	result, err := s.db.Exec(
		`DELETE FROM acl_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
	)
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

	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return err
	}
	return nil
}

func (s *Storage) UpdateTenantNodeACLRule(tenantID, nodeID, ruleID uuid.UUID, rule *ACLRuleRecord) (*ACLRuleRecord, error) {
	srcNet := aclNetworkForSync(rule.SrcCIDR)
	dstNet := aclNetworkForSync(rule.DstCIDR)
	minPort, maxPort := aclPortRangeForSync(rule.DstPort)

	result, err := s.db.Exec(
		`UPDATE acl_rules SET
			name = $4, action = $5, src_cidr = NULLIF($6, '')::cidr, dst_cidr = NULLIF($7, '')::cidr,
			dst_port = $8, protocol = $9, priority = $10, description = $11,
			enabled = $12, src_net = $13::cidr, dst_net = $14::cidr, min_port = $15, max_port = $16, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
		strings.TrimSpace(rule.Name), rule.Action, rule.SrcCIDR, rule.DstCIDR,
		rule.DstPort, rule.Protocol, rule.Priority,
		rule.Description, rule.Enabled, srcNet, dstNet, minPort, maxPort,
	)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, sql.ErrNoRows
	}

	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return nil, err
	}

	rule.ID = ruleID
	rule.TenantID = tenantID
	rule.NodeID = nodeID
	rule.Name = strings.TrimSpace(rule.Name)
	rule.UpdatedAt = time.Now()
	return rule, nil
}

func (s *Storage) ListTenantNodeBlacklistRules(tenantID, nodeID uuid.UUID, scope string) ([]*BlacklistRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, scope, COALESCE(cidr::text, ''), COALESCE(port, 0), enabled, COALESCE(description, ''), created_at, updated_at
		   FROM blacklist_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND scope = $3
		  ORDER BY created_at DESC`,
		tenantID, nodeID, scope,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*BlacklistRuleRecord
	for rows.Next() {
		rule, err := scanBlacklistRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Storage) CreateTenantNodeBlacklistRule(rule *BlacklistRuleRecord) (*BlacklistRuleRecord, error) {
	created := &BlacklistRuleRecord{}
	err := s.db.QueryRow(
		`INSERT INTO blacklist_rules (tenant_id, node_id, scope, cidr, port, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, $5, $6, $7)
		 RETURNING id, tenant_id, node_id, scope, COALESCE(cidr::text, ''), COALESCE(port, 0), enabled, COALESCE(description, ''), created_at, updated_at`,
		rule.TenantID,
		rule.NodeID,
		rule.Scope,
		strings.TrimSpace(rule.CIDR),
		nullableInt(rule.Port),
		rule.Enabled,
		strings.TrimSpace(rule.Description),
	).Scan(
		&created.ID,
		&created.TenantID,
		&created.NodeID,
		&created.Scope,
		&created.CIDR,
		&created.Port,
		&created.Enabled,
		&created.Description,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := s.bumpNodeDesiredVersion(rule.TenantID, rule.NodeID); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Storage) DeleteTenantNodeBlacklistRuleByID(tenantID, nodeID uuid.UUID, scope string, ruleID uuid.UUID) error {
	result, err := s.db.Exec(
		`DELETE FROM blacklist_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND scope = $4`,
		ruleID, tenantID, nodeID, scope,
	)
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
	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return err
	}
	return nil
}

func (s *Storage) DeleteTenantNodePortBlacklistRule(tenantID, nodeID uuid.UUID, port int) error {
	result, err := s.db.Exec(
		`DELETE FROM blacklist_rules WHERE tenant_id = $1 AND node_id = $2 AND scope = $3 AND port = $4`,
		tenantID, nodeID, BlacklistScopePorts, port,
	)
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
	if err := s.bumpNodeDesiredVersion(tenantID, nodeID); err != nil {
		return err
	}
	return nil
}

func (s *Storage) GetNodeQoSRulesByPublicKey(publicKey string) ([]*QoSRuleRecord, error) {
	node, err := s.GetNode(publicKey)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, category, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
		  ORDER BY category ASC, created_at ASC`,
		node.TenantID, node.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*QoSRuleRecord
	for rows.Next() {
		rule, err := scanQoSRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Storage) GetNodeBlacklistRulesByPublicKey(publicKey string) ([]*BlacklistRuleRecord, error) {
	node, err := s.GetNode(publicKey)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, scope, COALESCE(cidr::text, ''), COALESCE(port, 0), enabled, COALESCE(description, ''), created_at, updated_at
		   FROM blacklist_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
		  ORDER BY scope ASC, created_at ASC`,
		node.TenantID, node.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*BlacklistRuleRecord
	for rows.Next() {
		rule, err := scanBlacklistRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func scanQoSRule(scanner interface {
	Scan(dest ...interface{}) error
}) (*QoSRuleRecord, error) {
	record := &QoSRuleRecord{}
	err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&record.NodeID,
		&record.Category,
		&record.SrcCIDR,
		&record.DstCIDR,
		&record.SrcPort,
		&record.DstPort,
		&record.Protocol,
		&record.BandwidthMbps,
		&record.Enabled,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func scanBlacklistRule(scanner interface {
	Scan(dest ...interface{}) error
}) (*BlacklistRuleRecord, error) {
	record := &BlacklistRuleRecord{}
	err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&record.NodeID,
		&record.Scope,
		&record.CIDR,
		&record.Port,
		&record.Enabled,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func ValidateQoSCategory(category string) error {
	switch category {
	case QoSCategoryService, QoSCategoryPeers, QoSCategoryIP:
		return nil
	default:
		return fmt.Errorf("unsupported QoS category: %s", category)
	}
}

func ValidateBlacklistScope(scope string) error {
	switch scope {
	case BlacklistScopeSrc, BlacklistScopeDst, BlacklistScopePorts:
		return nil
	default:
		return fmt.Errorf("unsupported blacklist scope: %s", scope)
	}
}

func nullableInt(value int) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

func aclNetworkForSync(cidr string) string {
	trimmed := strings.TrimSpace(cidr)
	if trimmed == "" {
		return "0.0.0.0/0"
	}
	return trimmed
}

func aclPortRangeForSync(dstPort int) (int, int) {
	if dstPort <= 0 {
		return 0, 65535
	}
	return dstPort, dstPort
}

func (s *Storage) bumpNodeDesiredVersion(tenantID, nodeID uuid.UUID) error {
	newVersion := uuid.New().String()
	_, err := s.db.Exec(
		`INSERT INTO node_control_states (tenant_id, node_id, desired_state_version, desired_state_updated_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (node_id) DO UPDATE SET
		    desired_state_version = EXCLUDED.desired_state_version,
		    desired_state_updated_at = NOW(),
		    updated_at = NOW()`,
		tenantID, nodeID, newVersion,
	)
	if err != nil {
		fmt.Printf("[storage] failed to bump version for node %s: %v\n", nodeID, err)
	}
	return err
}
