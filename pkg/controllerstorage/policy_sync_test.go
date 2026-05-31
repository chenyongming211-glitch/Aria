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
