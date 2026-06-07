package controllerstorage

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateTenantNodeACLRuleWritesLegacySyncColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12, $13, $14::cidr, $15::cidr, $16, $17)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", "10.0.0.0/24", "0.0.0.0/0", 443, 443).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-web", "allow", "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)

	store := NewStorageWithDB(db)
	created, err := store.CreateTenantNodeACLRule(&ACLRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Name:        "allow-web",
		Action:      "allow",
		SrcCIDR:     "10.0.0.0/24",
		DstCIDR:     "0.0.0.0/0",
		DstPort:     443,
		Protocol:    6,
		Priority:    100,
		Enabled:     true,
		Description: "allow web",
	})
	if err != nil {
		t.Fatalf("CreateTenantNodeACLRule failed: %v", err)
	}
	if created.ID != ruleID || created.SrcCIDR != "10.0.0.0/24" || created.DstPort != 443 {
		t.Fatalf("unexpected created ACL rule: %#v", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantNodeACLRuleDefaultsLegacySyncRangeForAnyPort(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12, $13, $14::cidr, $15::cidr, $16, $17)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-any", "allow", "", "", nil, 0, "ingress", "", 10, true, "", "0.0.0.0/0", "0.0.0.0/0", 0, 65535).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-any", "allow", "0.0.0.0/0", "0.0.0.0/0", 0, 0, "ingress", "", 10, true, "", now, now))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)

	store := NewStorageWithDB(db)
	if _, err := store.CreateTenantNodeACLRule(&ACLRuleRecord{
		TenantID: tenantID,
		NodeID:   nodeID,
		Name:     "allow-any",
		Action:   "allow",
		Priority: 10,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("CreateTenantNodeACLRule failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantNodeACLRuleReturnsDesiredStateBumpError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	bumpErr := errors.New("desired state unavailable")

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::cidr, NULLIF($6, '')::cidr, $7, $8, $9, $10, $11, $12, $13, $14::cidr, $15::cidr, $16, $17)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "deny-any", "deny", "", "", nil, 0, "ingress", "", 50, true, "", "0.0.0.0/0", "0.0.0.0/0", 0, 65535).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "deny-any", "deny", "0.0.0.0/0", "0.0.0.0/0", 0, 0, "ingress", "", 50, true, "", now, now))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO node_control_states (tenant_id, node_id, desired_state_version, desired_state_updated_at, updated_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (node_id) DO UPDATE SET
			    desired_state_version = EXCLUDED.desired_state_version,
			    desired_state_updated_at = NOW(),
			    updated_at = NOW()`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg()).
		WillReturnError(bumpErr)

	store := NewStorageWithDB(db)
	if _, err := store.CreateTenantNodeACLRule(&ACLRuleRecord{
		TenantID: tenantID,
		NodeID:   nodeID,
		Name:     "deny-any",
		Action:   "deny",
		Priority: 50,
		Enabled:  true,
	}); !errors.Is(err, bumpErr) {
		t.Fatalf("expected desired state bump error %v, got %v", bumpErr, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBlacklistMutationsBumpDesiredStateVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO blacklist_rules (tenant_id, node_id, scope, cidr, port, enabled, description)
			 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, $5, $6, $7)
			 RETURNING id, tenant_id, node_id, scope, COALESCE(cidr::text, ''), COALESCE(port, 0), enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, BlacklistScopeSrc, "192.0.2.0/24", nil, true, "blocked").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "scope", "cidr", "port", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, BlacklistScopeSrc, "192.0.2.0/24", 0, true, "blocked", now, now))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM blacklist_rules WHERE id = $1 AND tenant_id = $2 AND node_id = $3 AND scope = $4`)).
		WithArgs(ruleID, tenantID, nodeID, BlacklistScopeSrc).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)

	store := NewStorageWithDB(db)
	if _, err := store.CreateTenantNodeBlacklistRule(&BlacklistRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Scope:       BlacklistScopeSrc,
		CIDR:        "192.0.2.0/24",
		Enabled:     true,
		Description: "blocked",
	}); err != nil {
		t.Fatalf("CreateTenantNodeBlacklistRule failed: %v", err)
	}
	if err := store.DeleteTenantNodeBlacklistRuleByID(tenantID, nodeID, BlacklistScopeSrc, ruleID); err != nil {
		t.Fatalf("DeleteTenantNodeBlacklistRuleByID failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectNetworkPolicyBumpDesiredState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO node_control_states (tenant_id, node_id, desired_state_version, desired_state_updated_at, updated_at)
			 VALUES ($1, $2, $3, NOW(), NOW())
			 ON CONFLICT (node_id) DO UPDATE SET
			    desired_state_version = EXCLUDED.desired_state_version,
			    desired_state_updated_at = NOW(),
			    updated_at = NOW()`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}
