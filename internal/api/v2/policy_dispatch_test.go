package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"aria/internal/api/apibase"
	controllerstorage "aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestWritePolicyMutationSuccessReturnsErrorWhenDispatchFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	nodeID := uuid.New()
	node := &controllerstorage.Node{
		ID:        nodeID,
		TenantID:  tenantID,
		PublicKey: "node-policy-key",
		Hostname:  "node-1",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WillReturnError(errors.New("desired state unavailable"))
	mock.ExpectRollback()

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.writePolicyMutationSuccess(rr, node, "acl", "create", map[string]interface{}{
		"id": "acl-1",
	}, "ACL created", map[string]interface{}{
		"policy_ref": "acl-1",
	})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when policy dispatch fails, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatalf("expected unsuccessful response: %#v", resp)
	}
	if resp.Error == nil {
		t.Fatalf("expected error payload: %#v", resp)
	}
	if resp.Error.Details["dispatch_error"] == "" {
		t.Fatalf("expected dispatch_error in error details: %#v", resp.Error.Details)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func nodeControlStateRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state",
		"observed_message", "observed_at", "last_sync_at", "last_sync_error",
		"created_at", "updated_at",
	}).AddRow(
		uuid.New(), uuid.New(), "dsv-test", []byte(`{}`), now,
		"", nil, "",
		"", nil, nil, "",
		now, now,
	)
}
