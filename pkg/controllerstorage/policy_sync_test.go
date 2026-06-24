package controllerstorage

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestQueuePolicySyncRollsBackWhenPolicyDeliveryFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
			"applied_state_version", "applied_state_updated_at", "observed_state",
			"observed_message", "observed_at", "last_sync_at", "last_sync_error",
			"created_at", "updated_at",
		}).AddRow(
			tenantID, nodeID, "dsv-test", []byte(`{"domain":"acl"}`), now,
			"", nil, "",
			"", nil, nil, "",
			now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs("node-policy-key", AgentCommandStatusStale, "superseded by desired state dsv-test", AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, AgentCommandStatusStale, "superseded by desired state dsv-test", AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("cmd-1", now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO policy_deliveries")).
		WillReturnError(errors.New("delivery insert failed"))
	mock.ExpectRollback()

	store := NewStorageWithDB(db)
	_, err = store.QueuePolicySync(PolicySyncRequest{
		TenantID:            tenantID,
		NodeID:              nodeID,
		NodePublicKey:       "node-policy-key",
		Domain:              "acl",
		Action:              "create",
		PolicyRef:           "acl-1",
		PolicyName:          "allow-web",
		DesiredStateVersion: "dsv-test",
		DesiredMetadata:     map[string]interface{}{"domain": "acl"},
		CommandParams:       map[string]interface{}{"domain": "acl"},
		DeliveryMetadata:    map[string]interface{}{"domain": "acl"},
		Priority:            1,
		TimeoutSeconds:      60,
	})
	if err == nil {
		t.Fatalf("expected delivery failure")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestQueuePolicySyncMarksOlderActiveDeliveriesAndCommandsStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	deliveryID := uuid.New()
	commandID := uuid.New().String()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WithArgs(tenantID, nodeID, "dsv-new", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
			"applied_state_version", "applied_state_updated_at", "observed_state",
			"observed_message", "observed_at", "last_sync_at", "last_sync_error",
			"created_at", "updated_at",
		}).AddRow(
			tenantID, nodeID, "dsv-new", []byte(`{"domain":"acl"}`), now,
			"", nil, "",
			"", nil, nil, "",
			now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs("node-policy-key", AgentCommandStatusStale, "superseded by desired state dsv-new", AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, AgentCommandStatusStale, "superseded by desired state dsv-new", AgentCommandStatusPending, AgentCommandStatusSent, AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WithArgs("node-policy-key", "sync", sqlmock.AnyArg(), AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO policy_deliveries")).
		WithArgs(tenantID, nodeID, "acl", "acl-1", "allow-web", "update", commandID, AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(
			deliveryID, tenantID, nodeID, "acl", "acl-1", "allow-web",
			"update", commandID, AgentCommandStatusPending, "", []byte(`{}`), now, now, nil,
		))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, AuditCommandQueued, "controller", "Command queued: sync", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID, AuditCommandQueued, "controller", "Command queued: sync", []byte(`{}`), now))

	store := NewStorageWithDB(db)
	result, err := store.QueuePolicySync(PolicySyncRequest{
		TenantID:            tenantID,
		NodeID:              nodeID,
		NodePublicKey:       "node-policy-key",
		Domain:              "acl",
		Action:              "update",
		PolicyRef:           "acl-1",
		PolicyName:          "allow-web",
		DesiredStateVersion: "dsv-new",
		DesiredMetadata:     map[string]interface{}{"domain": "acl"},
		CommandParams:       map[string]interface{}{"domain": "acl"},
		DeliveryMetadata:    map[string]interface{}{"domain": "acl"},
		Priority:            1,
		TimeoutSeconds:      60,
	})
	if err != nil {
		t.Fatalf("QueuePolicySync returned error: %v", err)
	}
	if result.Command == nil || result.Command.ID != commandID {
		t.Fatalf("unexpected command result: %#v", result.Command)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
