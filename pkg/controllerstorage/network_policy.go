package controllerstorage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	BlacklistScopeSrc   = "src"
	BlacklistScopeDst   = "dst"
	BlacklistScopePorts = "ports"
)

type QoSRuleRecord struct {
	ID                uuid.UUID                `json:"id"`
	TenantID          uuid.UUID                `json:"tenant_id"`
	NodeID            uuid.UUID                `json:"node_id"`
	SrcCIDR           string                   `json:"src_cidr"`
	DstCIDR           string                   `json:"dst_cidr"`
	SrcPort           int                      `json:"src_port"`
	DstPort           int                      `json:"dst_port"`
	Protocol          int                      `json:"protocol"`
	BandwidthMbps     int                      `json:"bandwidth_mbps"`
	Direction         string                   `json:"direction"`
	RateBps           uint64                   `json:"rate_bps"`
	BurstBytes        uint64                   `json:"burst_bytes"`
	Priority          int                      `json:"priority"`
	Mode              string                   `json:"mode"`
	Enabled           bool                     `json:"enabled"`
	Description       string                   `json:"description"`
	Stats             map[string]interface{}   `json:"stats,omitempty"`
	PolicyStatus      string                   `json:"policy_status,omitempty"`
	PendingCmds       int                      `json:"pending_cmds"`
	LastDelivery      map[string]interface{}   `json:"last_delivery,omitempty"`
	DeliveryHistory   []map[string]interface{} `json:"delivery_history,omitempty"`
	LastDeliveryError string                   `json:"last_delivery_error,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type ACLRuleRecord struct {
	ID                uuid.UUID                `json:"id"`
	TenantID          uuid.UUID                `json:"tenant_id"`
	NodeID            uuid.UUID                `json:"node_id"`
	Name              string                   `json:"name"`
	Action            string                   `json:"action"` // "allow", "deny"
	SrcCIDR           string                   `json:"src_cidr"`
	DstCIDR           string                   `json:"dst_cidr"`
	DstPort           int                      `json:"dst_port"`
	Protocol          int                      `json:"protocol"`
	Direction         string                   `json:"direction"`
	Ports             string                   `json:"ports"`
	Priority          int                      `json:"priority"`
	Enabled           bool                     `json:"enabled"`
	Description       string                   `json:"description"`
	Stats             map[string]interface{}   `json:"stats,omitempty"`
	PolicyStatus      string                   `json:"policy_status,omitempty"`
	PendingCmds       int                      `json:"pending_cmds"`
	LastDelivery      map[string]interface{}   `json:"last_delivery,omitempty"`
	DeliveryHistory   []map[string]interface{} `json:"delivery_history,omitempty"`
	LastDeliveryError string                   `json:"last_delivery_error,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
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

type policyMutationExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

func (s *Storage) ListTenantNodeQoSRules(tenantID, nodeID uuid.UUID) ([]*QoSRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at DESC`,
		tenantID, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]*QoSRuleRecord, 0)
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
	return createTenantNodeQoSRule(s.db, rule, true)
}

func createTenantNodeQoSRule(q policyMutationExecutor, rule *QoSRuleRecord, bumpDesiredVersion bool) (*QoSRuleRecord, error) {
	created := &QoSRuleRecord{}
	normalizeQoSRuntimeFields(rule)
	err := q.QueryRow(
		`INSERT INTO qos_rules (tenant_id, node_id, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, direction, rate_bps, burst_bytes, priority, mode, enabled, description)
		 VALUES ($1, $2, NULLIF($3, '')::cidr, NULLIF($4, '')::cidr, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id, tenant_id, node_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, COALESCE(direction, 'egress'),
		           COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		           COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		           COALESCE(priority, 0), COALESCE(mode, 'policing'),
		           enabled, COALESCE(description, ''), created_at, updated_at`,
		rule.TenantID,
		rule.NodeID,
		strings.TrimSpace(rule.SrcCIDR),
		strings.TrimSpace(rule.DstCIDR),
		nullableInt(rule.SrcPort),
		nullableInt(rule.DstPort),
		nullableInt(rule.Protocol),
		rule.BandwidthMbps,
		rule.Direction,
		rule.RateBps,
		rule.BurstBytes,
		rule.Priority,
		rule.Mode,
		rule.Enabled,
		strings.TrimSpace(rule.Description),
	).Scan(
		&created.ID,
		&created.TenantID,
		&created.NodeID,
		&created.SrcCIDR,
		&created.DstCIDR,
		&created.SrcPort,
		&created.DstPort,
		&created.Protocol,
		&created.BandwidthMbps,
		&created.Direction,
		&created.RateBps,
		&created.BurstBytes,
		&created.Priority,
		&created.Mode,
		&created.Enabled,
		&created.Description,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, rule.TenantID, rule.NodeID); err != nil {
			return nil, err
		}
	}

	return created, nil
}

func (s *Storage) DeleteTenantNodeQoSRule(tenantID, nodeID uuid.UUID, ruleID uuid.UUID) error {
	return deleteTenantNodeQoSRule(s.db, tenantID, nodeID, ruleID, true)
}

func deleteTenantNodeQoSRule(q policyMutationExecutor, tenantID, nodeID uuid.UUID, ruleID uuid.UUID, bumpDesiredVersion bool) error {
	result, err := q.Exec(
		`DELETE FROM qos_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
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

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, tenantID, nodeID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) GetTenantNodeQoSRule(tenantID, nodeID, ruleID uuid.UUID) (*QoSRuleRecord, error) {
	rule, err := scanQoSRule(s.db.QueryRow(
		`SELECT id, tenant_id, node_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
	))
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Storage) UpdateTenantNodeQoSRule(tenantID, nodeID, ruleID uuid.UUID, rule *QoSRuleRecord) (*QoSRuleRecord, error) {
	return updateTenantNodeQoSRule(s.db, tenantID, nodeID, ruleID, rule, true)
}

func updateTenantNodeQoSRule(q policyMutationExecutor, tenantID, nodeID, ruleID uuid.UUID, rule *QoSRuleRecord, bumpDesiredVersion bool) (*QoSRuleRecord, error) {
	normalizeQoSRuntimeFields(rule)
	result, err := q.Exec(
		`UPDATE qos_rules SET
			src_cidr = NULLIF($4, '')::cidr, dst_cidr = NULLIF($5, '')::cidr,
			src_port = $6, dst_port = $7, protocol = $8,
			bandwidth_mbps = $9, direction = $10, rate_bps = $11, burst_bytes = $12,
			priority = $13, mode = $14, description = $15,
			enabled = $16, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
		rule.SrcCIDR, rule.DstCIDR,
		rule.SrcPort, rule.DstPort, rule.Protocol,
		rule.BandwidthMbps, rule.Direction, rule.RateBps, rule.BurstBytes,
		rule.Priority, rule.Mode, rule.Description, rule.Enabled,
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

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, tenantID, nodeID); err != nil {
			return nil, err
		}
	}

	rule.ID = ruleID
	rule.TenantID = tenantID
	rule.NodeID = nodeID
	rule.UpdatedAt = time.Now()
	return rule, nil
}

// ACL Methods

func (s *Storage) ListTenantNodeACLRules(tenantID, nodeID uuid.UUID) ([]*ACLRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND node_id = $2
		  ORDER BY priority ASC, created_at ASC`,
		tenantID, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]*ACLRuleRecord, 0)
	for rows.Next() {
		rule := &ACLRuleRecord{}
		err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.NodeID, &rule.Name, &rule.Action, &rule.SrcCIDR, &rule.DstCIDR,
			&rule.DstPort, &rule.Protocol, &rule.Direction, &rule.Ports,
			&rule.Priority, &rule.Enabled, &rule.Description,
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
	return createTenantNodeACLRule(s.db, rule, true)
}

func createTenantNodeACLRule(q policyMutationExecutor, rule *ACLRuleRecord, bumpDesiredVersion bool) (*ACLRuleRecord, error) {
	created := &ACLRuleRecord{}
	srcNet := aclNetworkForSync(rule.SrcCIDR)
	dstNet := aclNetworkForSync(rule.DstCIDR)
	minPort, maxPort := aclPortRangeForSync(rule.DstPort)
	normalizeACLRuntimeFields(rule)

	err := q.QueryRow(
		`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12, $13, $14::cidr, $15::cidr, $16, $17)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`,
		rule.TenantID, rule.NodeID, strings.TrimSpace(rule.Name), rule.Action,
		strings.TrimSpace(rule.SrcCIDR), strings.TrimSpace(rule.DstCIDR),
		nullableInt(rule.DstPort), rule.Protocol, rule.Direction, rule.Ports,
		rule.Priority, rule.Enabled, strings.TrimSpace(rule.Description), srcNet, dstNet, minPort, maxPort,
	).Scan(
		&created.ID, &created.TenantID, &created.NodeID, &created.Name, &created.Action, &created.SrcCIDR, &created.DstCIDR,
		&created.DstPort, &created.Protocol, &created.Direction, &created.Ports,
		&created.Priority, &created.Enabled, &created.Description,
		&created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, rule.TenantID, rule.NodeID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s *Storage) DeleteTenantNodeACLRuleByID(tenantID, nodeID uuid.UUID, ruleID uuid.UUID) error {
	return deleteTenantNodeACLRuleByID(s.db, tenantID, nodeID, ruleID, true)
}

func deleteTenantNodeACLRuleByID(q policyMutationExecutor, tenantID, nodeID uuid.UUID, ruleID uuid.UUID, bumpDesiredVersion bool) error {
	result, err := q.Exec(
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

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, tenantID, nodeID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) GetTenantNodeACLRule(tenantID, nodeID, ruleID uuid.UUID) (*ACLRuleRecord, error) {
	rule, err := scanACLRuleRecord(s.db.QueryRow(
		`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
	))
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Storage) UpdateTenantNodeACLRule(tenantID, nodeID, ruleID uuid.UUID, rule *ACLRuleRecord) (*ACLRuleRecord, error) {
	return updateTenantNodeACLRule(s.db, tenantID, nodeID, ruleID, rule, true)
}

func updateTenantNodeACLRule(q policyMutationExecutor, tenantID, nodeID, ruleID uuid.UUID, rule *ACLRuleRecord, bumpDesiredVersion bool) (*ACLRuleRecord, error) {
	srcNet := aclNetworkForSync(rule.SrcCIDR)
	dstNet := aclNetworkForSync(rule.DstCIDR)
	minPort, maxPort := aclPortRangeForSync(rule.DstPort)
	normalizeACLRuntimeFields(rule)

	result, err := q.Exec(
		`UPDATE acl_rules SET
			name = $4, action = $5, src_cidr = NULLIF($6, '')::cidr, dst_cidr = NULLIF($7, '')::cidr,
			dst_port = $8, protocol = $9, direction = $10, ports = $11, priority = $12, description = $13,
			enabled = $14, src_net = $15::cidr, dst_net = $16::cidr, min_port = $17, max_port = $18, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND node_id = $3`,
		ruleID, tenantID, nodeID,
		strings.TrimSpace(rule.Name), rule.Action, rule.SrcCIDR, rule.DstCIDR,
		rule.DstPort, rule.Protocol, rule.Direction, rule.Ports, rule.Priority,
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

	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, tenantID, nodeID); err != nil {
			return nil, err
		}
	}

	rule.ID = ruleID
	rule.TenantID = tenantID
	rule.NodeID = nodeID
	rule.Name = strings.TrimSpace(rule.Name)
	rule.UpdatedAt = time.Now()
	return rule, nil
}

func scanACLRuleRecord(scanner interface {
	Scan(dest ...interface{}) error
}) (*ACLRuleRecord, error) {
	rule := &ACLRuleRecord{}
	err := scanner.Scan(
		&rule.ID, &rule.TenantID, &rule.NodeID, &rule.Name, &rule.Action, &rule.SrcCIDR, &rule.DstCIDR,
		&rule.DstPort, &rule.Protocol, &rule.Direction, &rule.Ports,
		&rule.Priority, &rule.Enabled, &rule.Description,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
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
	return createTenantNodeBlacklistRule(s.db, rule, true)
}

func createTenantNodeBlacklistRule(q policyMutationExecutor, rule *BlacklistRuleRecord, bumpDesiredVersion bool) (*BlacklistRuleRecord, error) {
	created := &BlacklistRuleRecord{}
	err := q.QueryRow(
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
	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, rule.TenantID, rule.NodeID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s *Storage) DeleteTenantNodeBlacklistRuleByID(tenantID, nodeID uuid.UUID, scope string, ruleID uuid.UUID) error {
	return deleteTenantNodeBlacklistRuleByID(s.db, tenantID, nodeID, scope, ruleID, true)
}

func deleteTenantNodeBlacklistRuleByID(q policyMutationExecutor, tenantID, nodeID uuid.UUID, scope string, ruleID uuid.UUID, bumpDesiredVersion bool) error {
	result, err := q.Exec(
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
	if bumpDesiredVersion {
		if err := bumpNodeDesiredVersionWith(q, tenantID, nodeID); err != nil {
			return err
		}
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
		`SELECT id, tenant_id, node_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		        COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		        bandwidth_mbps, COALESCE(direction, 'egress'),
		        COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		        COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		        COALESCE(priority, 0), COALESCE(mode, 'policing'),
		        enabled, COALESCE(description, ''), created_at, updated_at
		   FROM qos_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
		  ORDER BY priority ASC, created_at ASC`,
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

func (s *Storage) GetNodeACLRulesByPublicKey(publicKey string) ([]*ACLRuleRecord, error) {
	node, err := s.GetNode(publicKey)
	if err != nil {
		return nil, err
	}
	return s.GetEnabledTenantNodeACLRules(node.TenantID, node.ID)
}

func (s *Storage) GetEnabledTenantNodeACLRules(tenantID, nodeID uuid.UUID) ([]*ACLRuleRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
		  ORDER BY priority ASC, created_at ASC`,
		tenantID, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]*ACLRuleRecord, 0)
	for rows.Next() {
		rule, err := scanACLRuleRecord(rows)
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
		&record.SrcCIDR,
		&record.DstCIDR,
		&record.SrcPort,
		&record.DstPort,
		&record.Protocol,
		&record.BandwidthMbps,
		&record.Direction,
		&record.RateBps,
		&record.BurstBytes,
		&record.Priority,
		&record.Mode,
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

func normalizeACLRuntimeFields(rule *ACLRuleRecord) {
	if strings.TrimSpace(rule.Direction) == "" {
		rule.Direction = "ingress"
	}
	if strings.TrimSpace(rule.Ports) == "" && rule.DstPort > 0 {
		rule.Ports = fmt.Sprintf("%d", rule.DstPort)
	}
}

func normalizeQoSRuntimeFields(rule *QoSRuleRecord) {
	if strings.TrimSpace(rule.Direction) == "" {
		rule.Direction = defaultQoSDirection(rule)
	}
	if rule.RateBps == 0 && rule.BandwidthMbps > 0 {
		rule.RateBps = uint64(rule.BandwidthMbps) * 1000000
	}
	if rule.BurstBytes == 0 {
		rule.BurstBytes = defaultQoSBurst(rule.RateBps)
	}
	if strings.TrimSpace(rule.Mode) == "" {
		rule.Mode = "policing"
	}
}

func defaultQoSDirection(rule *QoSRuleRecord) string {
	if strings.TrimSpace(rule.SrcCIDR) != "" && strings.TrimSpace(rule.DstCIDR) == "" {
		return "ingress"
	}
	return "egress"
}

func defaultQoSBurst(rateBps uint64) uint64 {
	burst := rateBps / 8 / 10
	if burst < 1500 {
		return 1500
	}
	return burst
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
	return bumpNodeDesiredVersionWith(s.db, tenantID, nodeID)
}

func bumpNodeDesiredVersionWith(q interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, tenantID, nodeID uuid.UUID) error {
	newVersion := uuid.New().String()
	_, err := q.Exec(
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
