package controllerstorage

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestQueueAgentCommandsUsesSingleBulkInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WithArgs(
			"node-key-a", "sync", sqlmock.AnyArg(), AgentCommandStatusPending, 5, 45,
			"node-key-b", "sync", sqlmock.AnyArg(), AgentCommandStatusPending, 5, 45,
		).
		WillReturnRows(sqlmock.NewRows([]string{"node_public_key", "id", "created_at", "updated_at"}).
			AddRow("node-key-a", "cmd-a", now, now).
			AddRow("node-key-b", "cmd-b", now, now))

	commands, err := store.QueueAgentCommands([]AgentCommandTarget{
		{NodePublicKey: "node-key-a"},
		{NodePublicKey: "node-key-b"},
	}, "sync", map[string]interface{}{"source": "batch"}, 5, 45)
	if err != nil {
		t.Fatalf("QueueAgentCommands returned error: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected two commands, got %d", len(commands))
	}
	if commands[0].NodePublicKey != "node-key-a" || commands[0].ID != "cmd-a" {
		t.Fatalf("first command was not returned in target order: %#v", commands[0])
	}
	if commands[1].NodePublicKey != "node-key-b" || commands[1].ID != "cmd-b" {
		t.Fatalf("second command was not returned in target order: %#v", commands[1])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
