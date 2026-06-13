package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/pkg/controllerstorage"
	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestSyncNodeReturnsInternalServerErrorWhenPeerQueryFails(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs("enroll-token").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))
	expectSyncNodePeerQuery(mock, tenantID).WillReturnError(errors.New("database unavailable"))

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}
	rr := httptest.NewRecorder()
	controller.syncNode(&RegisterRequest{
		Token:     "enroll-token",
		PublicKey: "node-public-key",
		Region:    "sh",
	}, "100.64.0.2", rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSyncNodeReturnsInternalServerErrorWhenACLQueryFails(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs("enroll-token").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))
	expectSyncNodePeerQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows(syncNodeColumns()).AddRow(
		nodeID, "node-public-key", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "100.64.0.2", 2,
		now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))
	expectEnabledACLRulesQuery(mock, tenantID, nodeID).WillReturnError(errors.New("acl query unavailable"))

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}
	rr := httptest.NewRecorder()
	controller.syncNode(&RegisterRequest{
		Token:     "enroll-token",
		PublicKey: "node-public-key",
		Region:    "sh",
	}, "100.64.0.2", rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Failed to load ACL rules") {
		t.Fatalf("expected ACL error response, got body=%s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSyncNodeReturnsInternalServerErrorWhenRegisteredNodeMissingFromPeerSet(t *testing.T) {
	tenantID := uuid.New()
	otherNodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs("enroll-token").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))
	expectSyncNodePeerQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows(syncNodeColumns()).AddRow(
		otherNodeID, "other-node-public-key", "machine-2", tenantID, "1.1.1.2:51820", "10.0.0.2", "1.1.1.2", "sh", "vpc-1", "node-b", "100.64.0.3", 3,
		now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}
	rr := httptest.NewRecorder()
	controller.syncNode(&RegisterRequest{
		Token:     "enroll-token",
		PublicKey: "node-public-key",
		Region:    "sh",
	}, "100.64.0.2", rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Registered node not found in active peer set") {
		t.Fatalf("expected missing registered node response, got body=%s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestProcessSyncReturnsErrorWhenACLQueryFails(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	publicKey := "node-public-key"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows(syncNodeColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "100.64.0.2", 2,
			now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO nodes`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(nodeID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND status != 'deleted' ORDER BY last_seen DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows(syncNodeColumns()).AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "100.64.0.2", 2,
			now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))
	expectEnabledACLRulesQuery(mock, tenantID, nodeID).WillReturnError(errors.New("acl query unavailable"))

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}
	_, _, _, _, err = controller.processSync(publicKey)
	if err == nil || !strings.Contains(err.Error(), "failed to get node ACL rules") {
		t.Fatalf("expected ACL query error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSyncNodeReturnsNodeScopedACLWithoutRegionFiltering(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs("enroll-token").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))
	expectSyncNodePeerQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows(syncNodeColumns()).AddRow(
		nodeID, "node-public-key", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "100.64.0.2", 2,
		now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))
	expectEnabledACLRulesQuery(mock, tenantID, nodeID).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "deny-node-port", "deny", "198.51.100.10/32", "203.0.113.10/32", 65530, 6, "ingress", "65530", 201, true, "node scoped acl", now, now))
	expectSyncNodeControlState(mock, tenantID, nodeID, "dsv-rest-phase1", now)

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}
	rr := httptest.NewRecorder()
	controller.syncNode(&RegisterRequest{
		Token:     "enroll-token",
		PublicKey: "node-public-key",
		Region:    "sh",
	}, "100.64.0.2", rr)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}

	var resp SyncResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode sync response: %v", err)
	}
	if len(resp.ACLRules) != 1 {
		t.Fatalf("expected one node-scoped ACL rule, got %#v", resp.ACLRules)
	}
	if !resp.SnapshotComplete {
		t.Fatalf("expected REST sync snapshot_complete=true")
	}
	if resp.ACLRules[0].SrcNet != "198.51.100.10/32" ||
		resp.ACLRules[0].DstNet != "203.0.113.10/32" ||
		resp.ACLRules[0].Direction != "ingress" ||
		resp.ACLRules[0].Ports != "65530" ||
		resp.ACLRules[0].Action != "deny" {
		t.Fatalf("expected REST sync ACL runtime fields, got %#v", resp.ACLRules[0])
	}
	if resp.DomainVersions["acl"] != "dsv-rest-phase1" {
		t.Fatalf("expected REST sync acl domain version, got %#v", resp.DomainVersions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestACLRuleRecordsForSyncExpandsAnyProtocolPortRules(t *testing.T) {
	ruleID := uuid.New()
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	rules := aclRuleRecordsForSync([]*controllerstorage.ACLRuleRecord{
		{
			ID:        ruleID,
			TenantID:  tenantID,
			NodeID:    nodeID,
			Action:    "deny",
			SrcCIDR:   "198.51.100.10/32",
			DstCIDR:   "203.0.113.10/32",
			Protocol:  0,
			Direction: "egress",
			Ports:     "443",
			Priority:  100,
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
	})

	if len(rules) != 2 {
		t.Fatalf("expected Any+ports ACL to expand to TCP and UDP, got %#v", rules)
	}
	for i, protocol := range []uint8{6, 17} {
		rule := rules[i]
		if rule.ID != ruleID.String() ||
			rule.SrcNet != "198.51.100.10/32" ||
			rule.DstNet != "203.0.113.10/32" ||
			rule.Protocol != protocol ||
			rule.Direction != "egress" ||
			rule.Ports != "443" ||
			rule.Action != "deny" {
			t.Fatalf("unexpected expanded ACL rule %d: %#v", i, rule)
		}
	}
}

func expectSyncNodePeerQuery(mock sqlmock.Sqlmock, tenantID uuid.UUID) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')`)).
		WithArgs(tenantID)
}

func expectEnabledACLRulesQuery(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT id, tenant_id, node_id, COALESCE(name, ''), action, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
			        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
			        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			        priority, enabled, COALESCE(description, ''),
			        created_at, updated_at
			   FROM acl_rules
			  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
			  ORDER BY priority ASC, created_at ASC
		`))
}

func syncNodeColumns() []string {
	return []string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}
}

func expectSyncNodeControlState(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, desiredVersion string, now time.Time) {
	mock.ExpectQuery(`(?s)SELECT tenant_id, node_id, COALESCE\(desired_state_version, ''\).*FROM node_control_states.*WHERE tenant_id = \$1 AND node_id = \$2`).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
			"applied_state_version", "applied_state_updated_at", "observed_state", "observed_message", "observed_at",
			"last_sync_at", "last_sync_error", "created_at", "updated_at",
		}).AddRow(
			tenantID, nodeID, desiredVersion, []byte(`{"source":"test"}`), now,
			"", nil, "", "", nil, nil, "", now, now,
		))
}
