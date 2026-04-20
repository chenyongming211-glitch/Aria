package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func withSuperAdmin(req *http.Request, tenantID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserRoleKey, "super_admin")
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

func decodeAPIResponse(t *testing.T, rr *httptest.ResponseRecorder) apibase.APIResponse {
	t.Helper()
	var resp apibase.APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v, body=%s", err, rr.Body.String())
	}
	return resp
}

func responseDataMap(t *testing.T, resp apibase.APIResponse) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("failed to marshal response data: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("failed to unmarshal response data map: %v", err)
	}
	return out
}

func responseDataSlice(t *testing.T, v interface{}) []interface{} {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal response data slice: %v", err)
	}

	var out []interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("failed to unmarshal response data slice: %v", err)
	}
	return out
}

func expectTenantNodesQuery(mock sqlmock.Sqlmock, tenantID uuid.UUID) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND status != 'deleted' ORDER BY last_seen DESC`,
	)).WithArgs(tenantID)
}

func TestNodesAPI_ListSuccessWithEmptyData(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_ListReturnsGetNodesFailedOnDBError(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnError(errors.New("db unavailable"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeGetNodesFailed {
		t.Fatalf("expected code %s, got %s", apibase.CodeGetNodesFailed, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_InvalidNodeIDReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes/not-a-uuid", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInvalidRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeInvalidRequest, resp.Code)
	}
}

func TestNodesAPI_MethodBoundaryReturnsMethodNotAllowed(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/nodes", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeMethodNotAllowed {
		t.Fatalf("expected code %s, got %s", apibase.CodeMethodNotAllowed, resp.Code)
	}
}

func TestNodesAPI_UpdateInvalidBodyReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectNodeLookup(mock, tenantID, nodeID, "{}")

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		strings.NewReader("{"),
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInvalidRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeInvalidRequest, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_UpdateReturnsInternalErrorWhenExecFails(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes SET 
		hostname = COALESCE(NULLIF($1, ''), hostname),
		endpoint = COALESCE(NULLIF($2, ''), endpoint),
		private_ip = COALESCE(NULLIF($3, ''), private_ip),
		public_ip = COALESCE(NULLIF($4, ''), public_ip),
		region = COALESCE(NULLIF($5, ''), region),
		vpc_id = COALESCE(NULLIF($6, ''), vpc_id),
		role = $7,
		advertised_routes = $8,
		updated_at = NOW()
		WHERE id = $9 AND tenant_id = $10`)).
		WillReturnError(errors.New("update failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		strings.NewReader(`{"hostname":"new-host"}`),
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeUpdateNodeFailed {
		t.Fatalf("expected code %s, got %s", apibase.CodeUpdateNodeFailed, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_DeleteNotFoundReturnsNodeNotFound(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnError(sql.ErrNoRows)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodDelete,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		nil,
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeNodeNotFound {
		t.Fatalf("expected code %s, got %s", apibase.CodeNodeNotFound, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_DeleteReturnsInternalErrorWhenMarkDeletedFails(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT public_key FROM nodes WHERE id = $1 AND tenant_id = $2`)).
		WithArgs(nodeID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow("pub-key-1"))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = 'deleted', updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs("pub-key-1").
		WillReturnError(errors.New("mark deleted failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodDelete,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		nil,
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeUpdateNodeFailed {
		t.Fatalf("expected code %s, got %s", apibase.CodeUpdateNodeFailed, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_UpdateSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{10.1.0.0/24}")
	expectNodeLookup(mock, tenantID, nodeID, "{10.1.0.0/24}")
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes SET 
		hostname = COALESCE(NULLIF($1, ''), hostname),
		endpoint = COALESCE(NULLIF($2, ''), endpoint),
		private_ip = COALESCE(NULLIF($3, ''), private_ip),
		public_ip = COALESCE(NULLIF($4, ''), public_ip),
		region = COALESCE(NULLIF($5, ''), region),
		vpc_id = COALESCE(NULLIF($6, ''), vpc_id),
		role = $7,
		advertised_routes = $8,
		updated_at = NOW()
		WHERE id = $9 AND tenant_id = $10`)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodPut,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		strings.NewReader(`{"hostname":"edge-1","role":"gateway","advertised_routes":["10.10.0.0/16"]}`),
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["id"] != nodeID.String() {
		t.Fatalf("expected id=%s, got %#v", nodeID.String(), data["id"])
	}
	if data["tenant_id"] != tenantID.String() {
		t.Fatalf("expected tenant_id=%s, got %#v", tenantID.String(), data["tenant_id"])
	}
	if data["role"] != "gateway" {
		t.Fatalf("expected role=gateway, got %#v", data["role"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_DeleteSuccessReturnsDeletedStatus(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT public_key FROM nodes WHERE id = $1 AND tenant_id = $2`)).
		WithArgs(nodeID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"public_key"}).AddRow("pub-key-1"))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = 'deleted', updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs("pub-key-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodDelete,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(),
		nil,
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["id"] != nodeID.String() {
		t.Fatalf("expected id=%s, got %#v", nodeID.String(), data["id"])
	}
	if data["status"] != "deleted" {
		t.Fatalf("expected status=deleted, got %#v", data["status"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_InvalidNodeIDParamReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/events?node_id=bad-node-id", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeBadRequest, resp.Code)
	}
}

func TestMonitoringAPI_StatsSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE last_seen >= EXTRACT(EPOCH FROM NOW()) - 60) AS online,
			COUNT(*) FILTER (WHERE last_seen < EXTRACT(EPOCH FROM NOW()) - 60 OR last_seen IS NULL) AS offline
		FROM nodes
		WHERE tenant_id = $1 AND COALESCE(status, 'online') != 'deleted'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "online", "offline"}).AddRow(10, 7, 3))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '' AND ncs.desired_state_version = ncs.applied_state_version) AS synced,
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '') AS total
		FROM node_control_states ncs
		JOIN nodes n ON n.id = ncs.node_id
		WHERE n.tenant_id = $1
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"synced", "total"}).AddRow(9, 10))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM acl_rules WHERE tenant_id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM qos_rules WHERE tenant_id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM agent_commands ac
		JOIN nodes n ON n.public_key = ac.node_public_key
		WHERE n.tenant_id = $1 AND ac.status = 'failed'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = 'active'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/stats", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["total_nodes"] != float64(10) || data["online_nodes"] != float64(7) || data["offline_nodes"] != float64(3) {
		t.Fatalf("unexpected node counts: %#v", data)
	}
	if data["sync_success_rate"] != float64(90) {
		t.Fatalf("expected sync_success_rate=90, got %#v", data["sync_success_rate"])
	}
	if data["active_alerts_count"] != float64(4) || data["failed_commands_count"] != float64(2) {
		t.Fatalf("unexpected monitoring counters: %#v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_HealthSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE last_seen >= EXTRACT(EPOCH FROM NOW()) - 60) AS online,
			COUNT(*) FILTER (WHERE last_seen < EXTRACT(EPOCH FROM NOW()) - 60 OR last_seen IS NULL) AS offline
		FROM nodes
		WHERE tenant_id = $1 AND COALESCE(status, 'online') != 'deleted'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "online", "offline"}).AddRow(20, 15, 5))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '' AND ncs.desired_state_version = ncs.applied_state_version) AS synced,
			COUNT(*) FILTER (WHERE ncs.desired_state_version != '') AS total
		FROM node_control_states ncs
		JOIN nodes n ON n.id = ncs.node_id
		WHERE n.tenant_id = $1
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"synced", "total"}).AddRow(18, 20))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = 'active'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM agent_commands ac
		JOIN nodes n ON n.public_key = ac.node_public_key
		WHERE n.tenant_id = $1 AND ac.status = 'failed'
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/health", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["node_online_rate"] != float64(75) {
		t.Fatalf("expected node_online_rate=75, got %#v", data["node_online_rate"])
	}
	if data["sync_success_rate"] != float64(90) {
		t.Fatalf("expected sync_success_rate=90, got %#v", data["sync_success_rate"])
	}
	if data["active_alerts_count"] != float64(3) || data["failed_commands_count"] != float64(1) {
		t.Fatalf("unexpected health counters: %#v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_EventsSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT (
			SELECT COUNT(*) FROM alerts WHERE tenant_id = $1
		) + (
			SELECT COUNT(*) FROM audit_events WHERE tenant_id = $1
		)
	`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, source, event_type, severity, node_id, title, detail, created_at FROM (
			SELECT id::text, 'alert' AS source, alert_type AS event_type, severity,
			       COALESCE(node_id::text, '') AS node_id, title, context AS detail, created_at
			FROM alerts WHERE tenant_id = $1
			UNION ALL
			SELECT id::text, 'audit' AS source, event_type, '' AS severity,
			       COALESCE(node_id::text, '') AS node_id, summary AS title, detail, created_at
			FROM audit_events WHERE tenant_id = $1
		) combined
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`)).
		WithArgs(tenantID, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source", "event_type", "severity", "node_id", "title", "detail", "created_at",
		}).AddRow(uuid.New().String(), "alert", "high_latency", "warning", uuid.New().String(), "Latency high", []byte(`{"k":"v"}`), now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/events", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["total"] != float64(1) || data["limit"] != float64(50) || data["offset"] != float64(0) {
		t.Fatalf("unexpected paging fields: %#v", data)
	}
	if _, ok := data["items"]; !ok {
		t.Fatalf("expected items field in response data")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertsSuccessReturnsItemContract(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = $2")).
		WithArgs(tenantID, "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`)).
		WithArgs(tenantID, "active", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID,
			tenantID,
			nodeID.String(),
			"high_latency",
			"warning",
			"Latency high",
			"latency over threshold",
			[]byte(`{"threshold_ms":200}`),
			"active",
			now,
			nil,
		))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)
	items := responseDataSlice(t, data["items"])

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if data["total"] != float64(1) || data["limit"] != float64(50) || data["offset"] != float64(0) {
		t.Fatalf("unexpected paging fields: %#v", data)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 alert item, got %d", len(items))
	}

	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first item as map, got %#v", items[0])
	}
	if item["id"] != alertID.String() || item["tenant_id"] != tenantID.String() {
		t.Fatalf("unexpected item identity fields: %#v", item)
	}
	if item["alert_type"] != "high_latency" || item["severity"] != "warning" || item["status"] != "active" {
		t.Fatalf("unexpected item contract fields: %#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_NodeMetricsSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/"+nodeID.String()+"/metrics", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if _, ok := data["upload_mbps"]; !ok {
		t.Fatalf("expected upload_mbps field")
	}
	if _, ok := data["download_mbps"]; !ok {
		t.Fatalf("expected download_mbps field")
	}
	if _, ok := data["latency_ms"]; !ok {
		t.Fatalf("expected latency_ms field")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_EventsInvalidSinceReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/events?since=not-rfc3339", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeBadRequest, resp.Code)
	}
}

func TestMonitoringAPI_AlertsLimitAndOffsetBoundary(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = $2")).
		WithArgs(tenantID, "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`)).
		WithArgs(tenantID, "active", 200, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts?limit=999&offset=-5", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if data["limit"] != float64(200) {
		t.Fatalf("expected limit=200, got %#v", data["limit"])
	}
	if data["offset"] != float64(0) {
		t.Fatalf("expected offset=0, got %#v", data["offset"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertsInvalidNodeIDReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts?node_id=bad-node-id", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeBadRequest, resp.Code)
	}
}

func TestMonitoringAPI_AlertResolveInvalidIDReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/not-a-uuid/resolve", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeBadRequest, resp.Code)
	}
}

func TestMonitoringAPI_AlertResolveNotFoundReturns404(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE id = $1
	`)).
		WithArgs(alertID).
		WillReturnError(sql.ErrNoRows)

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/"+alertID.String()+"/resolve", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	if resp.Code != "ALERT_NOT_FOUND" {
		t.Fatalf("expected code ALERT_NOT_FOUND, got %s", resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_EventsReturnsInternalErrorWhenQueryFails(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT (
			SELECT COUNT(*) FROM alerts WHERE tenant_id = $1
		) + (
			SELECT COUNT(*) FROM audit_events WHERE tenant_id = $1
		)
	`)).
		WithArgs(tenantID).
		WillReturnError(errors.New("event feed count failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/events", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInternalServerError {
		t.Fatalf("expected code %s, got %s", apibase.CodeInternalServerError, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertsReturnsInternalErrorWhenQueryFails(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = $2")).
		WithArgs(tenantID, "active").
		WillReturnError(errors.New("alerts count failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInternalServerError {
		t.Fatalf("expected code %s, got %s", apibase.CodeInternalServerError, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_InvalidPathReturnsInvalidPath(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/not-supported", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInvalidPath {
		t.Fatalf("expected code %s, got %s", apibase.CodeInvalidPath, resp.Code)
	}
}

func TestMonitoringAPI_TrafficBoundaryNoNodesReturnsEmptySeries(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/traffic?range=24h", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_StatsReturnsInternalErrorWhenNodeCountFails(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE last_seen >= EXTRACT(EPOCH FROM NOW()) - 60) AS online,
			COUNT(*) FILTER (WHERE last_seen < EXTRACT(EPOCH FROM NOW()) - 60 OR last_seen IS NULL) AS offline
		FROM nodes
		WHERE tenant_id = $1 AND COALESCE(status, 'online') != 'deleted'
	`)).
		WithArgs(tenantID).
		WillReturnError(errors.New("count failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/stats", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeInternalServerError {
		t.Fatalf("expected code %s, got %s", apibase.CodeInternalServerError, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

