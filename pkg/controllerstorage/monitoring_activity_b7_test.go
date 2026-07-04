package controllerstorage

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGetLatestNodeCertificateActivityQueriesAllEventTypesOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT DISTINCT ON (event_type) id, tenant_id, node_id, event_type, actor, summary, detail, created_at
		FROM audit_events
		WHERE tenant_id = $1 AND node_id = $2 AND event_type = ANY($3)
		ORDER BY event_type, created_at DESC
	`)).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(
			uuid.New(), tenantID, nodeID.String(), "certificate_renew_failed", "system", "renew failed", []byte(`{"error":"timeout"}`), now,
		).AddRow(
			uuid.New(), tenantID, nodeID.String(), AuditCertRevoked, "system", "revoked", []byte(`{"reason":"suspended"}`), now.Add(-time.Minute),
		))

	events, err := store.GetLatestNodeCertificateActivity(tenantID, nodeID)
	if err != nil {
		t.Fatalf("GetLatestNodeCertificateActivity returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two activity events, got %d", len(events))
	}
	if events["certificate_renew_failed"].Detail["error"] != "timeout" {
		t.Fatalf("unexpected renew failure detail: %#v", events["certificate_renew_failed"].Detail)
	}
	if events[AuditCertRevoked].Detail["reason"] != "suspended" {
		t.Fatalf("unexpected revoke detail: %#v", events[AuditCertRevoked].Detail)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
