package controllerstorage

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestListRecentNodeAlertsDoesNotCountRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	alertID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND status = $2 AND node_id = $3
		ORDER BY created_at DESC
		LIMIT $4
	`)).
		WithArgs(tenantID, "active", nodeID, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID,
			tenantID,
			nodeID.String(),
			"sync_failed",
			"warning",
			"Sync failed",
			"last sync reported an error",
			[]byte(`{"phase":"apply"}`),
			"active",
			now,
			nil,
		))

	alerts, err := NewStorageWithDB(db).ListRecentNodeAlerts(tenantID, nodeID, "active", 10)
	if err != nil {
		t.Fatalf("ListRecentNodeAlerts failed: %v", err)
	}
	if len(alerts) != 1 || alerts[0].ID != alertID {
		t.Fatalf("expected one recent alert, got %#v", alerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
