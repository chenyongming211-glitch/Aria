package controllerstorage

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCleanupDeletedNodesDetachesHistoryAndPurgesChildRowsBeforeDeletingNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	threshold := int64(1780739000)
	nodeID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id
		FROM nodes
		WHERE status = 'deleted' AND updated_at < to_timestamp($1)
		FOR UPDATE`)).
		WithArgs(threshold).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(nodeID))

	for _, query := range []string{
		"UPDATE audit_events",
		"UPDATE alerts",
		"DELETE FROM policy_deliveries",
		"DELETE FROM acl_rules",
		"DELETE FROM qos_rules",
		"DELETE FROM blacklist_rules",
		"DELETE FROM device_configs",
		"DELETE FROM tunnel_links",
		"DELETE FROM ip_allocations",
	} {
		mock.ExpectExec(regexp.QuoteMeta(query)).
			WithArgs(sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM nodes
		WHERE id = ANY($1)`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	count, err := NewStorageWithDB(db).CleanupDeletedNodes(threshold)
	if err != nil {
		t.Fatalf("CleanupDeletedNodes returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deleted node, got %d", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCleanupDeletedNodesCommitsWhenNoNodesMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	threshold := int64(1780739000)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id
		FROM nodes
		WHERE status = 'deleted' AND updated_at < to_timestamp($1)
		FOR UPDATE`)).
		WithArgs(threshold).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	count, err := NewStorageWithDB(db).CleanupDeletedNodes(threshold)
	if err != nil {
		t.Fatalf("CleanupDeletedNodes returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no deleted nodes, got %d", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
