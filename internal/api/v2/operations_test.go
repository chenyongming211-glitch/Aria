package v2

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func operationNodeColumns() []string {
	return []string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}
}

func addOperationNodeRow(rows *sqlmock.Rows, nodeID, tenantID uuid.UUID, publicKey, hostname, status string) *sqlmock.Rows {
	now := time.Now()
	return rows.AddRow(
		nodeID, publicKey, "machine-"+hostname, tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", hostname, "100.64.0.2", 2,
		now.Unix(), now.Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, status, int64(0), "{}", "", now, now,
	)
}

func TestBatchAgentCommandRejectsInvalidNodeID(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/agents/command",
		strings.NewReader(`{"node_ids":["not-a-uuid"],"command":{"command":"sync"}}`))
	rr := httptest.NewRecorder()

	router.handleTenantBatchAgentCommand(rr, req, tenantID)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeInvalidRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeInvalidRequest, resp.Code)
	}
}

func TestNodeAgentCommandRejectsInactiveNode(t *testing.T) {
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/tid/nodes/"+nodeID.String()+"/agent/command",
		strings.NewReader(`{"command":"sync"}`))
	rr := httptest.NewRecorder()

	router.handleTenantNodeAgentCommand(rr, req, &controllerstorage.Node{
		ID:        nodeID,
		PublicKey: "suspended-node-key",
		Status:    "suspended",
	})
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeConflict {
		t.Fatalf("expected code %s, got %s", apibase.CodeConflict, resp.Code)
	}
	if !strings.Contains(resp.Message, "suspended") {
		t.Fatalf("expected suspended status message, got %q", resp.Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodeAgentCommandWritesQueuedAuditWithOperationContext(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	alertID := uuid.New()
	commandID := uuid.New().String()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO agent_commands (node_public_key, command, params, status, priority, timeout_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`)).
		WithArgs("node-key-1", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 0, 30).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditCommandQueued, "user", "Command queued: sync", jsonDetailContains{
			"command_id":    commandID,
			"command":       "sync",
			"source":        "monitoring",
			"alert_id":      alertID.String(),
			"event_type":    "sync_failed",
			"policy_ref":    "acl-1",
			"policy_domain": "acl",
		}).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID.String(), controllerstorage.AuditCommandQueued, "user", "Command queued: sync", []byte(`{}`), now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String()+"/agent/command",
		strings.NewReader(`{"command":"sync","params":{"source":"monitoring","alert_id":"`+alertID.String()+`","event_type":"sync_failed","policy_ref":"acl-1","policy_domain":"acl"}}`))
	rr := httptest.NewRecorder()

	router.handleTenantNodeAgentCommand(rr, req, &controllerstorage.Node{
		ID:        nodeID,
		TenantID:  tenantID,
		PublicKey: "node-key-1",
		Status:    "online",
	})
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if data["command_id"] != commandID {
		t.Fatalf("expected command id %s, got %#v", commandID, data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBatchAgentCommandSkipsInactiveNodes(t *testing.T) {
	tenantID := uuid.New()
	activeNodeID := uuid.New()
	suspendedNodeID := uuid.New()
	commandID := uuid.New().String()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows(operationNodeColumns())
	addOperationNodeRow(rows, activeNodeID, tenantID, "active-node-key", "node-active", "online")
	addOperationNodeRow(rows, suspendedNodeID, tenantID, "suspended-node-key", "node-suspended", "suspended")
	expectTenantNodesQuery(mock, tenantID).WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO agent_commands (node_public_key, command, params, status, priority, timeout_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`)).
		WithArgs("active-node-key", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 0, 30).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, activeNodeID, controllerstorage.AuditCommandQueued, "user", "Command queued: sync", jsonDetailContains{
			"command_id": commandID,
			"command":    "sync",
			"source":     "api.v2",
		}).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, activeNodeID.String(), controllerstorage.AuditCommandQueued, "user", "Command queued: sync", []byte(`{}`), now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/agents/command",
		strings.NewReader(`{"command":{"command":"sync"}}`))
	rr := httptest.NewRecorder()

	router.handleTenantBatchAgentCommand(rr, req, tenantID)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if data["success_count"] != float64(1) || data["failed_count"] != float64(1) {
		t.Fatalf("expected one success and one failure, got %#v", data)
	}
	results := responseDataSlice(t, data["results"])
	if len(results) != 2 {
		t.Fatalf("expected two batch results, got %#v", results)
	}
	failed := results[1].(map[string]interface{})
	if failed["node_id"] != suspendedNodeID.String() || failed["status"] != controllerstorage.AgentCommandStatusFailed {
		t.Fatalf("unexpected inactive node result: %#v", failed)
	}
	if !strings.Contains(failed["message"].(string), "suspended") {
		t.Fatalf("expected suspended failure message, got %#v", failed["message"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
