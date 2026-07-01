package controllerstorage

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetAIAuditLogsReturnsRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rowErr := errors.New("audit row iteration failed")
	mock.ExpectQuery("SELECT id, session_id, user_message, ai_response, tool_name, tool_arguments, tool_result, tool_status, execution_time_ms, created_at").
		WithArgs("", 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "session_id", "user_message", "ai_response", "tool_name", "tool_arguments", "tool_result", "tool_status", "execution_time_ms", "created_at",
		}).
			AddRow(int64(1), "session-1", "hello", "ok", "", []byte(`{}`), "", "", nil, time.Now()).
			AddRow(int64(2), "session-2", "hello again", "ok", "", []byte(`{}`), "", "", nil, time.Now()).
			RowError(1, rowErr))

	_, err = NewStorageWithDB(db).GetAIAuditLogs("", 10)
	if !errors.Is(err, rowErr) {
		t.Fatalf("expected rows.Err %v, got %v", rowErr, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
