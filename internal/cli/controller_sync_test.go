package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestSyncNodeFiltersACLRegionWithTenantNodesOnly(t *testing.T) {
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
		now.Unix(), now.Add(-time.Hour).Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{10.10.0.0/24}", "", now, now,
	))
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT id, COALESCE(name, ''), COALESCE(src_node, ''), COALESCE(src_net::text, src_cidr::text, '0.0.0.0/0'), COALESCE(dst_node, ''), COALESCE(dst_net::text, dst_cidr::text, '0.0.0.0/0'), protocol, min_port, max_port,
			       COALESCE(action, 'allow'), COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
			       enabled, priority, COALESCE(description, ''), created_at, updated_at
			FROM acl_rules
			WHERE tenant_id = $1 AND enabled = true
			ORDER BY priority ASC, id ASC
		`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "src_node", "src_net", "dst_node", "dst_net", "protocol", "min_port", "max_port", "action", "direction", "ports", "enabled", "priority", "description", "created_at", "updated_at",
		}).AddRow(ruleID, "allow-tenant-route", "", "10.10.0.0/24", "", "0.0.0.0/0", uint8(6), uint16(443), uint16(443), "allow", "ingress", "443", true, 100, "allow tenant route", now, now))
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
		t.Fatalf("expected one ACL rule from tenant node region lookup, got %#v", resp.ACLRules)
	}
	if !resp.SnapshotComplete {
		t.Fatalf("expected REST sync snapshot_complete=true")
	}
	if resp.ACLRules[0].Direction != "ingress" || resp.ACLRules[0].Ports != "443" {
		t.Fatalf("expected REST sync ACL runtime fields, got %#v", resp.ACLRules[0])
	}
	if resp.DomainVersions["acl"] != "dsv-rest-phase1" {
		t.Fatalf("expected REST sync acl domain version, got %#v", resp.DomainVersions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func expectSyncNodePeerQuery(mock sqlmock.Sqlmock, tenantID uuid.UUID) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')`)).
		WithArgs(tenantID)
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
