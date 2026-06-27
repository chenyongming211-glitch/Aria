package controllerstorage

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCountNodesByTenantAndStatusTreatsInactiveNodesAsNonOnline(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (
					WHERE COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')
					  AND last_seen >= EXTRACT(EPOCH FROM NOW()) - 60
				) AS online,
				COUNT(*) FILTER (
					WHERE COALESCE(status, 'online') IN ('suspended', 'banned')
					   OR last_seen < EXTRACT(EPOCH FROM NOW()) - 60
					   OR last_seen IS NULL
				) AS offline
			FROM nodes
			WHERE tenant_id = $1 AND COALESCE(status, 'online') != 'deleted'
		`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "online", "offline"}).AddRow(3, 1, 2))

	total, online, offline, err := NewStorageWithDB(db).CountNodesByTenantAndStatus(tenantID)
	if err != nil {
		t.Fatalf("CountNodesByTenantAndStatus returned error: %v", err)
	}
	if total != 3 || online != 1 || offline != 2 {
		t.Fatalf("unexpected counts: total=%d online=%d offline=%d", total, online, offline)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
