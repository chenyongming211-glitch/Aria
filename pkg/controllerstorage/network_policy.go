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
	ID            uuid.UUID
	TenantID      uuid.UUID
	NodeID        uuid.UUID
	Category      string
	SrcCIDR       string
	DstCIDR       string
	SrcPort       int
	DstPort       int
	Protocol      int
	BandwidthMbps int
	Enabled       bool
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
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
	return nil
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
