package controllerstorage

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateAgentCommandStatusForNodeRejectsMismatchedNode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(commandID, AgentCommandStatusCompleted, "done", []byte(`{"ok":"true"}`), nodePublicKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = store.UpdateAgentCommandStatusForNode(commandID, nodePublicKey, AgentCommandStatusCompleted, "done", map[string]string{"ok": "true"})
	if err == nil {
		t.Fatalf("expected command status update to reject mismatched node")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
