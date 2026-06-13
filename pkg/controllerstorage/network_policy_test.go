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

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-web", "allow", nil, nil, "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", "10.0.0.0/24", "0.0.0.0/0", 443, 443).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-web", "allow", nil, nil, "10.0.0.0/24", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
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

func TestDetectACLPolicyConflictRejectsAmbiguousSamePriorityOverlap(t *testing.T) {
	existing := []*ACLRuleRecord{{
		ID:        uuid.New(),
		SrcCIDR:   "10.10.0.0/16",
		DstCIDR:   "0.0.0.0/0",
		Protocol:  1,
		Direction: "ingress",
		Priority:  100,
		Enabled:   true,
	}}
	candidate := &ACLRuleRecord{
		SrcCIDR:   "10.10.0.0/16",
		DstCIDR:   "0.0.0.0/0",
		Protocol:  1,
		Direction: "ingress",
		Priority:  100,
		Enabled:   true,
	}

	err := DetectACLPolicyConflict(existing, candidate, uuid.Nil)
	if !errors.Is(err, ErrAmbiguousPolicyConflict) {
		t.Fatalf("expected ambiguous policy conflict, got %v", err)
	}
}

func TestDetectACLPolicyConflictAllowsMoreSpecificSamePriorityRule(t *testing.T) {
	existing := []*ACLRuleRecord{{
		ID:        uuid.New(),
		SrcCIDR:   "10.10.0.0/16",
		DstCIDR:   "0.0.0.0/0",
		Protocol:  1,
		Direction: "ingress",
		Priority:  100,
		Enabled:   true,
	}}
	candidate := &ACLRuleRecord{
		SrcCIDR:   "10.10.1.10/32",
		DstCIDR:   "0.0.0.0/0",
		Protocol:  1,
		Direction: "ingress",
		Priority:  100,
		Enabled:   true,
	}

	if err := DetectACLPolicyConflict(existing, candidate, uuid.Nil); err != nil {
		t.Fatalf("expected more specific ACL rule to be accepted, got %v", err)
	}
}

func TestDetectQoSPolicyConflictRejectsAmbiguousSamePriorityOverlap(t *testing.T) {
	existing := []*QoSRuleRecord{{
		DstCIDR:   "100.64.0.0/24",
		Direction: "egress",
		Priority:  100,
		Enabled:   true,
	}}
	candidate := &QoSRuleRecord{
		DstCIDR:   "100.64.0.0/24",
		Direction: "egress",
		Priority:  100,
		Enabled:   true,
	}

	err := DetectQoSPolicyConflict(existing, candidate, uuid.Nil)
	if !errors.Is(err, ErrAmbiguousPolicyConflict) {
		t.Fatalf("expected ambiguous QoS conflict, got %v", err)
	}
}

func TestDetectQoSPolicyConflictAllowsMoreSpecificSamePriorityRule(t *testing.T) {
	existing := []*QoSRuleRecord{{
		DstCIDR:   "100.64.0.0/24",
		Direction: "egress",
		Priority:  100,
		Enabled:   true,
	}}
	candidate := &QoSRuleRecord{
		DstCIDR:   "100.64.0.3/32",
		Direction: "egress",
		Priority:  100,
		Enabled:   true,
	}

	if err := DetectQoSPolicyConflict(existing, candidate, uuid.Nil); err != nil {
		t.Fatalf("expected more specific QoS rule to be accepted, got %v", err)
	}
}

func TestCreateTenantNodeACLRuleWritesGroupReferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	srcGroupID := uuid.New()
	dstGroupID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-web", "allow", srcGroupID, dstGroupID, "", "", 443, 6, "ingress", "443", 100, true, "allow web", "0.0.0.0/0", "0.0.0.0/0", 443, 443).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-web", "allow", srcGroupID, dstGroupID, "0.0.0.0/0", "0.0.0.0/0", 443, 6, "ingress", "443", 100, true, "allow web", now, now))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)

	store := NewStorageWithDB(db)
	created, err := store.CreateTenantNodeACLRule(&ACLRuleRecord{
		TenantID:    tenantID,
		NodeID:      nodeID,
		Name:        "allow-web",
		Action:      "allow",
		SrcGroupID:  uuid.NullUUID{UUID: srcGroupID, Valid: true},
		DstGroupID:  uuid.NullUUID{UUID: dstGroupID, Valid: true},
		DstPort:     443,
		Protocol:    6,
		Direction:   "ingress",
		Ports:       "443",
		Priority:    100,
		Enabled:     true,
		Description: "allow web",
	})
	if err != nil {
		t.Fatalf("CreateTenantNodeACLRule failed: %v", err)
	}
	if created.SrcGroupID.UUID != srcGroupID || created.DstGroupID.UUID != dstGroupID {
		t.Fatalf("expected group ids to round-trip, got %#v", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantNodeQoSRuleWritesGroupReference(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	groupID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO qos_rules (tenant_id, node_id, group_id, src_cidr, dst_cidr, src_port, dst_port, protocol, bandwidth_mbps, direction, rate_bps, burst_bytes, priority, mode, enabled, description)
		 VALUES ($1, $2, $3, NULLIF($4, '')::cidr, NULLIF($5, '')::cidr, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, tenant_id, node_id, group_id, COALESCE(src_cidr::text, ''), COALESCE(dst_cidr::text, ''),
		           COALESCE(src_port, 0), COALESCE(dst_port, 0), COALESCE(protocol, 0),
		           bandwidth_mbps, COALESCE(direction, 'egress'),
		           COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000),
		           COALESCE(burst_bytes, GREATEST((COALESCE(rate_bps, bandwidth_mbps::bigint * 1000000) / 8 / 10), 1500)),
		           COALESCE(priority, 0), COALESCE(mode, 'policing'),
		           enabled, COALESCE(description, ''), created_at, updated_at`)).
		WithArgs(tenantID, nodeID, groupID, "", "", nil, nil, nil, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "group_id", "src_cidr", "dst_cidr", "src_port", "dst_port", "protocol", "bandwidth_mbps", "direction", "rate_bps", "burst_bytes", "priority", "mode", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, groupID, "", "", 0, 0, 0, 200, "egress", uint64(200000000), uint64(2500000), 100, "policing", true, "qos web", now, now))
	expectNetworkPolicyBumpDesiredState(mock, tenantID, nodeID)

	store := NewStorageWithDB(db)
	created, err := store.CreateTenantNodeQoSRule(&QoSRuleRecord{
		TenantID:      tenantID,
		NodeID:        nodeID,
		GroupID:       uuid.NullUUID{UUID: groupID, Valid: true},
		BandwidthMbps: 200,
		Direction:     "egress",
		RateBps:       200000000,
		BurstBytes:    2500000,
		Priority:      100,
		Mode:          "policing",
		Enabled:       true,
		Description:   "qos web",
	})
	if err != nil {
		t.Fatalf("CreateTenantNodeQoSRule failed: %v", err)
	}
	if created.GroupID.UUID != groupID {
		t.Fatalf("expected group id to round-trip, got %#v", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolvePolicyGroupRefCreatesInlineGroupForDirectCIDR(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	now := time.Now()
	inlineName := inlineIPGroupName([]string{"10.10.0.0/16"})

	mock.ExpectQuery(regexp.QuoteMeta(upsertInlineIPGroupSQL)).
		WithArgs(tenantID, inlineName, "inline policy group", IPGroupKindInline).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "description", "kind", "created_by", "created_at", "updated_at"}).
			AddRow(groupID, tenantID, inlineName, "inline policy group", IPGroupKindInline, nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta(upsertIPGroupMemberSQL)).
		WithArgs(tenantID, groupID, "10.10.0.0/16", "inline").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewStorageWithDB(db)
	ref, err := store.ResolvePolicyGroupRef(tenantID, uuid.NullUUID{}, "10.10.0.0/16")
	if err != nil {
		t.Fatalf("ResolvePolicyGroupRef failed: %v", err)
	}
	if !ref.Valid || ref.UUID != groupID {
		t.Fatalf("unexpected group ref: %#v", ref)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolvePolicyGroupRefVerifiesExplicitGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	groupID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM ip_groups WHERE tenant_id = $1 AND id = $2`)).
		WithArgs(tenantID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(groupID))

	store := NewStorageWithDB(db)
	ref, err := store.ResolvePolicyGroupRef(tenantID, uuid.NullUUID{UUID: groupID, Valid: true}, "")
	if err != nil {
		t.Fatalf("ResolvePolicyGroupRef failed: %v", err)
	}
	if !ref.Valid || ref.UUID != groupID {
		t.Fatalf("unexpected group ref: %#v", ref)
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

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "allow-any", "allow", nil, nil, "", "", nil, 0, "ingress", "", 10, true, "", "0.0.0.0/0", "0.0.0.0/0", 0, 65535).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "allow-any", "allow", nil, nil, "0.0.0.0/0", "0.0.0.0/0", 0, 0, "ingress", "", 10, true, "", now, now))
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

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO acl_rules (tenant_id, node_id, name, action, src_group_id, dst_group_id, src_cidr, dst_cidr, dst_port, protocol, direction, ports, priority, enabled, description, src_net, dst_net, min_port, max_port)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::cidr, NULLIF($8, '')::cidr, $9, $10, $11, $12, $13, $14, $15, $16::cidr, $17::cidr, $18, $19)
		 RETURNING id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		           COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		           COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		           priority, enabled, COALESCE(description, ''),
		           created_at, updated_at`)).
		WithArgs(tenantID, nodeID, "deny-any", "deny", nil, nil, "", "", nil, 0, "ingress", "", 50, true, "", "0.0.0.0/0", "0.0.0.0/0", 0, 65535).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(uuid.New(), tenantID, nodeID, "deny-any", "deny", nil, nil, "0.0.0.0/0", "0.0.0.0/0", 0, 0, "ingress", "", 50, true, "", now, now))
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
