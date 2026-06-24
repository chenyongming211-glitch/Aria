package v2

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
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

func TestWritePolicyMutationSuccessWritesPolicyChangedAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	commandID := uuid.New().String()
	deliveryID := uuid.New()
	node := &controllerstorage.Node{
		ID:        nodeID,
		TenantID:  tenantID,
		PublicKey: "node-policy-key",
		Hostname:  "node-1",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(nodeControlStateRowsFor(tenantID, nodeID, "dsv-test", now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs(node.PublicKey, controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WithArgs(node.PublicKey, "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO policy_deliveries")).
		WithArgs(tenantID, nodeID, "acl", "acl-1", "Allow SSH", "create", commandID, controllerstorage.AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(
			deliveryID, tenantID, nodeID, "acl", "acl-1", "Allow SSH",
			"create", commandID, controllerstorage.AgentCommandStatusPending, "", []byte(`{}`), now, now, nil,
		))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID, controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", []byte(`{}`), now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditPolicyChanged, "user", "Policy changed", jsonDetailContains{
			"policy_domain":         "acl",
			"policy_ref":            "acl-1",
			"policy_name":           "Allow SSH",
			"action":                "create",
			"source":                "api.v2",
			"command_id":            commandID,
			"desired_state_version": "*",
		}).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID, controllerstorage.AuditPolicyChanged, "user", "Policy changed", []byte(`{}`), now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM agent_commands
		WHERE node_public_key = $1
		  AND status IN ($2, $3, $4)
	`)).
		WithArgs(node.PublicKey, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT 1
	`)).
		WithArgs(node.PublicKey).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		       COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		       COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		       created_at, updated_at
		FROM node_control_states
		WHERE tenant_id = $1 AND node_id = $2
	`)).
		WithArgs(tenantID, nodeID).
		WillReturnRows(nodeControlStateRowsFor(tenantID, nodeID, "dsv-test", now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.writePolicyMutationSuccess(rr, node, "acl", "create", map[string]interface{}{
		"id":   "acl-1",
		"name": "Allow SSH",
	}, "ACL created", map[string]interface{}{
		"policy_ref":  "acl-1",
		"policy_name": "Allow SSH",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRetryPolicySyncQueuesFreshDelivery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	tenantID := uuid.New()
	nodeID := uuid.New()
	ruleID := uuid.New()
	commandID := uuid.New().String()
	deliveryID := uuid.New()

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE id = $1 AND tenant_id = $2 AND node_id = $3`)).
		WithArgs(ruleID, tenantID, nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr",
			"dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}).AddRow(ruleID, tenantID, nodeID, "allow-icmp", "allow", nil, nil, "", "", 0, 1, "egress", "", 10, true, "Allow ICMP", now, now))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(nodeControlStateRowsFor(tenantID, nodeID, "dsv-retry", now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs("pub-key-1", controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WithArgs("pub-key-1", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO policy_deliveries")).
		WithArgs(tenantID, nodeID, "acl", ruleID.String(), "allow-icmp", "retry", commandID, controllerstorage.AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(
			deliveryID, tenantID, nodeID, "acl", ruleID.String(), "allow-icmp",
			"retry", commandID, controllerstorage.AgentCommandStatusPending, "", []byte(`{}`), now, now, nil,
		))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID, controllerstorage.AuditCommandQueued, "controller", "Command queued: sync", []byte(`{}`), now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditPolicyChanged, "user", "Policy changed", jsonDetailContains{
			"policy_domain": "acl",
			"policy_ref":    ruleID.String(),
			"policy_name":   "allow-icmp",
			"action":        "retry",
			"source":        "api.v2",
			"command_id":    commandID,
		}).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID, controllerstorage.AuditPolicyChanged, "user", "Policy changed", []byte(`{}`), now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	body := fmt.Sprintf(`{"node_id":"%s","policy_domain":"acl","policy_ref":"%s"}`, nodeID, ruleID)
	req := withAuthContext(
		httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/policies/retry", strings.NewReader(body)),
		"super_admin",
		tenantID,
	)
	rr := httptest.NewRecorder()
	router.handleTenantPolicies(rr, req, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response: %#v", resp)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %#v", resp.Data)
	}
	dispatch, ok := data["dispatch"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dispatch in response, got %#v", data)
	}
	if dispatch["command_id"] != commandID {
		t.Fatalf("expected command id %s, got %#v", commandID, dispatch["command_id"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func nodeControlStateRows() *sqlmock.Rows {
	now := time.Now()
	return nodeControlStateRowsFor(uuid.New(), uuid.New(), "dsv-test", now)
}

func nodeControlStateRowsFor(tenantID, nodeID uuid.UUID, desiredVersion string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
		"applied_state_version", "applied_state_updated_at", "observed_state",
		"observed_message", "observed_at", "last_sync_at", "last_sync_error",
		"created_at", "updated_at",
	}).AddRow(
		tenantID, nodeID, desiredVersion, []byte(`{}`), now,
		"", nil, "",
		"", nil, nil, "",
		now, now,
	)
}

type jsonDetailContains map[string]string

func (m jsonDetailContains) Match(value driver.Value) bool {
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return false
	}

	var detail map[string]interface{}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return false
	}

	for key, expected := range m {
		actual := strings.TrimSpace(fmt.Sprint(detail[key]))
		if expected == "*" {
			if actual == "" {
				return false
			}
			continue
		}
		if actual != expected {
			return false
		}
	}

	return true
}
