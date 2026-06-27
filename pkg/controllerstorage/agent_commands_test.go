package controllerstorage

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestQueueAgentCommandRejectsUnsupportedCommand(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	if _, err := store.QueueAgentCommand("node-key", "rm -rf /", nil, 0, 30); err == nil {
		t.Fatalf("expected unsupported command to be rejected")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetNextPendingAgentCommandRequeuesTimedOutSentCommands(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(nodePublicKey, AgentCommandStatusPending, AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1 AND status = $2
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`)).
		WithArgs(nodePublicKey, AgentCommandStatusPending).
		WillReturnError(errors.New("query cancelled"))
	mock.ExpectRollback()

	if _, err := store.GetNextPendingAgentCommand(nodePublicKey); err == nil {
		t.Fatalf("expected query error after requeue")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRequeueSentAgentCommandRestoresPendingStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"

	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(commandID, nodePublicKey, AgentCommandStatusPending, "stream send failed", AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.RequeueSentAgentCommand(commandID, nodePublicKey, "stream send failed"); err != nil {
		t.Fatalf("RequeueSentAgentCommand returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestFailIncompleteAgentCommandsForNodeMarksCommandsAndDeliveriesFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	nodePublicKey := "stream-node-key"
	message := "node status changed to suspended"

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(nodePublicKey, AgentCommandStatusFailed, message, AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(nodePublicKey, AgentCommandStatusFailed, message, AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := store.FailIncompleteAgentCommandsForNode(nodePublicKey, message); err != nil {
		t.Fatalf("FailIncompleteAgentCommandsForNode returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestFailTimedOutAgentCommandsForNodeMarksCommandsAndDeliveriesFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(
			nodePublicKey,
			AgentCommandStatusFailed,
			"command timed out waiting for agent result",
			AgentCommandStatusPending,
			AgentCommandStatusSent,
			AgentCommandStatusAcknowledged,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(
			nodePublicKey,
			AgentCommandStatusFailed,
			"command timed out waiting for agent result",
			AgentCommandStatusPending,
			AgentCommandStatusSent,
			AgentCommandStatusAcknowledged,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	affected, err := store.FailTimedOutAgentCommandsForNode(nodePublicKey)
	if err != nil {
		t.Fatalf("FailTimedOutAgentCommandsForNode returned error: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 affected command, got %d", affected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM agent_commands WHERE id = $1 AND node_public_key = $2 FOR UPDATE`)).
		WithArgs(commandID, nodePublicKey).
		WillReturnError(errors.New("not found"))
	mock.ExpectRollback()

	err = store.UpdateAgentCommandStatusForNode(commandID, nodePublicKey, AgentCommandStatusCompleted, "done", map[string]string{"ok": "true"})
	if err == nil {
		t.Fatalf("expected command status update to reject mismatched node")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateAgentCommandStatusForNodeRejectsUnknownStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	err = store.UpdateAgentCommandStatusForNode("cmd-1", "stream-node-key", "ready", "agent ready", nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported agent command status") {
		t.Fatalf("expected unsupported status error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateAgentCommandStatusForNodeRejectsTerminalDowngrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM agent_commands WHERE id = $1 AND node_public_key = $2 FOR UPDATE`)).
		WithArgs(commandID, nodePublicKey).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(AgentCommandStatusCompleted))
	mock.ExpectRollback()

	err = store.UpdateAgentCommandStatusForNode(commandID, nodePublicKey, AgentCommandStatusAcknowledged, "late ack", nil)
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal transition rejection, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateAgentCommandStatusForNodeAllowsSentToAcknowledged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM agent_commands WHERE id = $1 AND node_public_key = $2 FOR UPDATE`)).
		WithArgs(commandID, nodePublicKey).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(AgentCommandStatusSent))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(commandID, AgentCommandStatusAcknowledged, "ack", []byte(`{"phase":"apply"}`), nodePublicKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(commandID, AgentCommandStatusAcknowledged, "ack").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT n.tenant_id, n.id, ac.command, ac.params`)).
		WithArgs(commandID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	err = store.UpdateAgentCommandStatusForNode(commandID, nodePublicKey, AgentCommandStatusAcknowledged, "ack", map[string]string{"phase": "apply"})
	if err != nil {
		t.Fatalf("expected acknowledged status update to succeed, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateAgentCommandStatusForNodeIgnoresStaleCommand(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM agent_commands WHERE id = $1 AND node_public_key = $2 FOR UPDATE`)).
		WithArgs(commandID, nodePublicKey).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(AgentCommandStatusStale))
	mock.ExpectRollback()

	err = store.UpdateAgentCommandStatusForNode(commandID, nodePublicKey, AgentCommandStatusCompleted, "done", map[string]string{"desired_state_version": "dsv-old"})
	if err != nil {
		t.Fatalf("expected stale command result to be ignored, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetNextPendingAgentCommandReturnsCommandAfterRequeueSweep(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStorageWithDB(db)
	commandID := "a74e0068-4fd9-4d85-bb56-4d37400eb8cc"
	nodePublicKey := "stream-node-key"
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(nodePublicKey, AgentCommandStatusPending, AgentCommandStatusSent).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1 AND status = $2
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`)).
		WithArgs(nodePublicKey, AgentCommandStatusPending).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "node_public_key", "command", "params", "status", "message", "priority", "timeout_seconds",
			"created_at", "updated_at", "sent_at", "acknowledged_at", "completed_at", "result",
		}).AddRow(commandID, nodePublicKey, "sync", []byte(`{}`), AgentCommandStatusPending, "", 1, 30, now, now, nil, nil, nil, []byte(`{}`)))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(commandID, AgentCommandStatusSent, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	cmd, err := store.GetNextPendingAgentCommand(nodePublicKey)
	if err != nil {
		t.Fatalf("GetNextPendingAgentCommand returned error: %v", err)
	}
	if cmd == nil || cmd.ID != commandID || cmd.Status != AgentCommandStatusSent {
		t.Fatalf("unexpected command: %#v", cmd)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
