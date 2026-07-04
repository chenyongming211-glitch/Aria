package controllerstorage

import (
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetNodeMonitoringDetailStateReturnsBundledSections(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	certID := uuid.New()
	renewedFrom := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`WITH control AS`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows(nodeMonitoringDetailStateTestColumns()).AddRow(
			tenantID.String(), nodeID.String(), "desired-v1", []byte(`{"source":"test"}`), now.Add(-4*time.Minute),
			"applied-v1", now.Add(-3*time.Minute), "healthy", "ok", now.Add(-2*time.Minute),
			now.Add(-time.Minute), "", now.Add(-5*time.Minute), now,
			tenantID.String(), nodeID.String(), []byte(`{"acl_rules":{"r1":{"hits":7}}}`), now,
			certID.String(), tenantID.String(), nodeID.String(), "serial-1", "cert-pem", "ca-pem",
			now.Add(-time.Hour), now.Add(time.Hour), CertStatusIssued, now.Add(-time.Hour), nil, "", renewedFrom.String(), now,
		))

	state, err := NewStorageWithDB(db).GetNodeMonitoringDetailState(tenantID, nodeID)
	if err != nil {
		t.Fatalf("GetNodeMonitoringDetailState failed: %v", err)
	}
	if state.ControlState == nil || state.PolicyStats == nil || state.Certificate == nil {
		t.Fatalf("expected all bundled sections, got %#v", state)
	}
	if state.ControlState.DesiredStateVersion != "desired-v1" {
		t.Fatalf("expected desired state version to be scanned, got %q", state.ControlState.DesiredStateVersion)
	}
	if state.PolicyStats.Stats["acl_rules"] == nil {
		t.Fatalf("expected policy stats to be decoded, got %#v", state.PolicyStats.Stats)
	}
	if state.Certificate.ID != certID || state.Certificate.RenewedFrom == nil || *state.Certificate.RenewedFrom != renewedFrom {
		t.Fatalf("expected certificate metadata to be scanned, got %#v", state.Certificate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetNodeMonitoringDetailStateReturnsEmptyBundleWhenSectionsAreMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	row := make([]driver.Value, len(nodeMonitoringDetailStateTestColumns()))

	mock.ExpectQuery(regexp.QuoteMeta(`WITH control AS`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows(nodeMonitoringDetailStateTestColumns()).AddRow(row...))

	state, err := NewStorageWithDB(db).GetNodeMonitoringDetailState(tenantID, nodeID)
	if err != nil {
		t.Fatalf("GetNodeMonitoringDetailState failed: %v", err)
	}
	if state == nil {
		t.Fatalf("expected empty state bundle, got nil")
	}
	if state.ControlState != nil || state.PolicyStats != nil || state.Certificate != nil {
		t.Fatalf("expected no bundled sections, got %#v", state)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func nodeMonitoringDetailStateTestColumns() []string {
	return []string{
		"control_tenant_id", "control_node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state", "observed_message", "observed_at",
		"last_sync_at", "last_sync_error", "control_created_at", "control_updated_at",
		"stats_tenant_id", "stats_node_id", "policy_stats", "policy_stats_updated_at",
		"cert_id", "cert_tenant_id", "cert_node_id", "serial_number", "cert_pem", "ca_pem",
		"not_before", "not_after", "cert_status", "issued_at", "revoked_at", "revoke_reason", "renewed_from", "cert_updated_at",
	}
}
