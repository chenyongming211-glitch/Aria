package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"
	"aria/pkg/victoriametrics"

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
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs("pub-key-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", time.Now(), time.Now(),
		))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs("pub-key-1", "deleted").
		WillReturnError(errors.New("mark deleted failed"))
	mock.ExpectRollback()

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

func TestNodesAPI_DeleteReturnsInternalErrorWhenCertificateRevokeFails(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs("pub-key-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", time.Now(), time.Now(),
		))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs("pub-key-1", "deleted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE node_certificates
			SET status = $2,
			    revoked_at = NOW(),
			    revoke_reason = $3,
			    updated_at = NOW()
			WHERE node_id = $1
	`)).
		WithArgs(nodeID, controllerstorage.CertStatusRevoked, "node deleted via API").
		WillReturnError(errors.New("revoke failed"))
	mock.ExpectRollback()

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
	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{"id"}))
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
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectNodeLifecycleTransitionByPublicKey(mock, "pub-key-1", tenantID, nodeID, "online", "deleted", "node deleted via API", "node_deleted", "user", "Node deleted via API")

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

func TestNodesAPI_GetByIDSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// One lookup in handleTenantNodes and one lookup in getTenantNodeByID.
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	// buildTenantNodeResponse best-effort operations summary.
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM agent_commands
		WHERE node_public_key = $1
		  AND status IN ($2, $3, $4)
	`)).
		WithArgs("pub-key-1", "pending", "sent", "acknowledged").
		WillReturnError(errors.New("summary unavailable"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(), nil), tenantID)
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
	if data["public_ip"] != "1.1.1.1" {
		t.Fatalf("expected public_ip=1.1.1.1, got %#v", data["public_ip"])
	}
	if data["private_ip"] != "10.0.0.1" {
		t.Fatalf("expected private_ip=10.0.0.1, got %#v", data["private_ip"])
	}
	if data["endpoint"] != "1.1.1.1:51820" {
		t.Fatalf("expected endpoint=1.1.1.1:51820, got %#v", data["endpoint"])
	}
	if _, ok := data["status"]; !ok {
		t.Fatalf("expected status field in node payload")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_GetByIDNotFoundReturnsNodeNotFound(t *testing.T) {
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
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(), nil), tenantID)
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

func TestNodesAPI_GetByIDCrossTenantReturnsNodeNotFound(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Router-level lookup returns a node from another tenant.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-x", "machine-x", otherTenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-x", "node-x", "10.0.0.20", 20,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(), nil), tenantID)
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

func TestNodesAPI_SingleNodePatchReturnsMethodNotAllowed(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{}")

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPatch, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String(), nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeMethodNotAllowed {
		t.Fatalf("expected code %s, got %s", apibase.CodeMethodNotAllowed, resp.Code)
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

func TestMonitoringAPI_NodeDetailSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, nodeID, "{10.10.0.0/16}")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT tenant_id, node_id, COALESCE(desired_state_version, ''), desired_state_metadata, desired_state_updated_at,
		       COALESCE(applied_state_version, ''), applied_state_updated_at, COALESCE(observed_state, ''),
		       COALESCE(observed_message, ''), observed_at, last_sync_at, COALESCE(last_sync_error, ''),
		       created_at, updated_at
		FROM node_control_states
		WHERE tenant_id = $1 AND node_id = $2
	`)).
		WithArgs(tenantID, nodeID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, serial_number, cert_pem, ca_pem,
		       not_before, not_after, status, issued_at, revoked_at,
		       COALESCE(revoke_reason, ''), renewed_from, updated_at
		FROM node_certificates
		WHERE node_id = $1
	`)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "serial_number", "cert_pem", "ca_pem",
			"not_before", "not_after", "status", "issued_at", "revoked_at",
			"revoke_reason", "renewed_from", "updated_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID,
			"serial-1",
			"cert-pem",
			"ca-pem",
			now.Add(-24*time.Hour),
			now.Add(24*time.Hour),
			controllerstorage.CertStatusIssued,
			now.Add(-24*time.Hour),
			nil,
			"",
			nil,
			now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM audit_events WHERE tenant_id = $1 AND node_id = $2 AND event_type = $3")).
		WithArgs(tenantID, nodeID, "certificate_renewed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, event_type, actor, summary, detail, created_at
		FROM audit_events
		WHERE tenant_id = $1 AND node_id = $2 AND event_type = $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`)).
		WithArgs(tenantID, nodeID, "certificate_renewed", 1, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID.String(),
			"certificate_renewed",
			"system",
			"certificate renewed",
			[]byte(`{"serial_number":"serial-2","renewed_from":"serial-1","not_after":"2026-04-25T10:00:00Z"}`),
			now.Add(-2*time.Hour),
		))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM audit_events WHERE tenant_id = $1 AND node_id = $2 AND event_type = $3")).
		WithArgs(tenantID, nodeID, "certificate_renew_failed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, event_type, actor, summary, detail, created_at
		FROM audit_events
		WHERE tenant_id = $1 AND node_id = $2 AND event_type = $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`)).
		WithArgs(tenantID, nodeID, "certificate_renew_failed", 1, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID.String(),
			"certificate_renew_failed",
			"system",
			"certificate renew failed",
			[]byte(`{"error":"runtime token expired"}`),
			now.Add(-time.Hour),
		))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, node_public_key, command, params, status, COALESCE(message, ''), priority, timeout_seconds,
		       created_at, updated_at, sent_at, acknowledged_at, completed_at, result
		FROM agent_commands
		WHERE node_public_key = $1
		ORDER BY created_at DESC
		LIMIT $2
	`)).
		WithArgs("pub-key-1", 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "node_public_key", "command", "params", "status", "message",
			"priority", "timeout_seconds", "created_at", "updated_at", "sent_at",
			"acknowledged_at", "completed_at", "result",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, policy_domain, policy_ref, COALESCE(policy_name, ''), action, command_id, command_status,
		       COALESCE(last_error, ''), metadata, created_at, updated_at, completed_at
		FROM policy_deliveries
		WHERE tenant_id = $1 AND node_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`)).
		WithArgs(tenantID, nodeID, 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata",
			"created_at", "updated_at", "completed_at",
		}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = $2 AND node_id = $3")).
		WithArgs(tenantID, "active", nodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND status = $2 AND node_id = $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`)).
		WithArgs(tenantID, "active", nodeID, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID.String(),
			"sync_failed",
			"warning",
			"Sync failed",
			"last sync reported an error",
			[]byte(`{"phase":"apply"}`),
			"active",
			now,
			nil,
		))
	expectTenantNodesQuery(mock, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/"+nodeID.String(), nil), tenantID)
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
	if data["node_id"] != nodeID.String() {
		t.Fatalf("expected node_id=%s, got %#v", nodeID.String(), data["node_id"])
	}
	if _, ok := data["recent_commands"]; !ok {
		t.Fatalf("expected recent_commands field")
	}
	certificate, ok := data["certificate"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected certificate field, got %#v", data["certificate"])
	}
	if certificate["status"] != controllerstorage.CertStatusIssued {
		t.Fatalf("expected certificate status %q, got %#v", controllerstorage.CertStatusIssued, certificate["status"])
	}
	certificateActivity, ok := data["certificate_activity"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected certificate_activity field, got %#v", data["certificate_activity"])
	}
	if certificateActivity["last_renew_failure"] != "runtime token expired" {
		t.Fatalf("expected last_renew_failure to be populated, got %#v", certificateActivity["last_renew_failure"])
	}
	if certificateActivity["last_renewed_serial_number"] != "serial-2" {
		t.Fatalf("expected last_renewed_serial_number to be populated, got %#v", certificateActivity["last_renewed_serial_number"])
	}
	if _, ok := data["recent_policy_deliveries"]; !ok {
		t.Fatalf("expected recent_policy_deliveries field")
	}
	if alerts, ok := data["active_alerts"].([]interface{}); !ok || len(alerts) != 1 {
		t.Fatalf("expected active_alerts field with 1 item, got %#v", data["active_alerts"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_NodeDetailNotFoundReturnsNodeNotFound(t *testing.T) {
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
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/"+nodeID.String(), nil), tenantID)
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

func TestMonitoringAPI_NodeDetailCrossTenantReturnsNodeNotFound(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-x", "machine-x", otherTenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-x", "node-x", "10.0.0.20", 20,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/"+nodeID.String(), nil), tenantID)
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

func TestMonitoringAPI_NodeDetailInvalidNodeIDReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/not-a-uuid", nil), tenantID)
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

func TestMonitoringAPI_HealthNodeCountFailureReturnsInternalError(t *testing.T) {
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
		WillReturnError(errors.New("count unavailable"))
	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/health", nil), tenantID)
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

func TestMonitoringAPI_EventsSuccessReturnsContractFields(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()
	eventNodeID := uuid.New()
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
		}).AddRow(uuid.New().String(), "alert", "high_latency", "warning", eventNodeID.String(), "Latency high", []byte(`{"k":"v"}`), now))

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
	items := responseDataSlice(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("expected one event item, got %d", len(items))
	}
	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first event item as map, got %#v", items[0])
	}
	if item["source"] != "alert" || item["event_type"] != "high_latency" || item["severity"] != "warning" {
		t.Fatalf("unexpected event item identity: %#v", item)
	}
	if item["node_id"] != eventNodeID.String() {
		t.Fatalf("unexpected event node_id: %#v", item["node_id"])
	}
	detail, ok := item["detail"].(map[string]interface{})
	if !ok || detail["k"] != "v" {
		t.Fatalf("unexpected event detail payload: %#v", item["detail"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_EventsLimitAndOffsetBoundary(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
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
		WithArgs(tenantID, 200, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source", "event_type", "severity", "node_id", "title", "detail", "created_at",
		}))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/events?limit=999&offset=-7", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if data["limit"] != float64(200) || data["offset"] != float64(0) {
		t.Fatalf("expected clamped paging fields, got limit=%#v offset=%#v", data["limit"], data["offset"])
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
	context, ok := item["context"].(map[string]interface{})
	if !ok || context["threshold_ms"] != float64(200) {
		t.Fatalf("unexpected alert context payload: %#v", item["context"])
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

	vmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[0,"1.5"]}]}}`))
	}))
	t.Cleanup(vmServer.Close)

	router := &Router{store: controllerstorage.NewStorageWithDB(db), vmClient: victoriametrics.NewClient(vmServer.URL)}
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

func TestMonitoringAPI_NodeMetricsVMUnavailableReturnsServiceUnavailable(t *testing.T) {
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

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeServiceUnavailable {
		t.Fatalf("expected code %s, got %s", apibase.CodeServiceUnavailable, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_NodeMetricsNotFoundReturnsNodeNotFound(t *testing.T) {
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
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/nodes/"+nodeID.String()+"/metrics", nil), tenantID)
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

func TestMonitoringAPI_TrafficInvalidRangeReturnsBadRequest(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/traffic?range=2h", nil), tenantID)
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

func TestMonitoringAPI_StatsMethodBoundaryReturnsInvalidPath(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/stats", nil), tenantID)
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

func TestNodesAPI_RoutesMethodBoundaryReturnsMethodNotAllowed(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Lookup in handleTenantNodes.
	expectNodeLookup(mock, tenantID, nodeID, "{}")
	// Lookup in handleTenantNodeRoutes.
	expectNodeLookup(mock, tenantID, nodeID, "{}")

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPatch, "/api/v2/tenants/"+tenantID.String()+"/nodes/"+nodeID.String()+"/routes", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeMethodNotAllowed {
		t.Fatalf("expected code %s, got %s", apibase.CodeMethodNotAllowed, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestNodesAPI_AddRouteRejectsCrossRegionConflict(t *testing.T) {
	tenantID := uuid.New()
	targetNodeID := uuid.New()
	conflictNodeID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookup(mock, tenantID, targetNodeID, "{}")
	expectNodeLookup(mock, tenantID, targetNodeID, "{}")
	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		conflictNodeID, "conflict-key", "machine-b", tenantID, "2.2.2.2:51820", "10.0.0.2", "2.2.2.2", "bj", "vpc-2", "node-b", "10.0.0.11", 11,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{10.10.0.0/16}", "", now, now,
	))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(
		http.MethodPost,
		"/api/v2/tenants/"+tenantID.String()+"/nodes/"+targetNodeID.String()+"/routes",
		strings.NewReader(`{"cidr":"10.10.1.0/24"}`),
	), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeBadRequest, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_TopologyBoundaryNoNodesReturnsEmptyCollections(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/topology", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)
	nodes := responseDataSlice(t, data["nodes"])
	links := responseDataSlice(t, data["links"])

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.Code != apibase.CodeOK {
		t.Fatalf("expected code %s, got %s", apibase.CodeOK, resp.Code)
	}
	if len(nodes) != 0 || len(links) != 0 {
		t.Fatalf("expected empty topology, got nodes=%d links=%d", len(nodes), len(links))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_TopologyReturnsInternalErrorWhenNodeQueryFails(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnError(errors.New("query failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/topology", nil), tenantID)
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

func TestMonitoringAPI_TopologyOneNodeReturnsNodeWithoutLinks(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{10.10.0.0/16}", "", now, now,
	))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/topology", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)
	nodes := responseDataSlice(t, data["nodes"])
	links := responseDataSlice(t, data["links"])

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(nodes) != 1 || len(links) != 0 {
		t.Fatalf("expected one node and zero links, got nodes=%d links=%d", len(nodes), len(links))
	}
	firstNode, ok := nodes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first topology node as map, got %#v", nodes[0])
	}
	if firstNode["id"] != nodeID.String() || firstNode["status"] != "online" {
		t.Fatalf("unexpected topology node payload: %#v", firstNode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_TopologyTwoNodesReturnsActiveLink(t *testing.T) {
	tenantID := uuid.New()
	nodeAID := uuid.New()
	nodeBID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		nodeAID, "pub-key-a", "machine-a", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "10.0.0.10", 10,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	).AddRow(
		nodeBID, "pub-key-b", "machine-b", tenantID, "2.2.2.2:51820", "10.0.0.2", "2.2.2.2", "bj", "vpc-2", "node-b", "10.0.0.11", 11,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/topology", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)
	data := responseDataMap(t, resp)
	links := responseDataSlice(t, data["links"])

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(links) != 1 {
		t.Fatalf("expected one link, got %d", len(links))
	}
	link, ok := links[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first link as map, got %#v", links[0])
	}
	if link["status"] != "active" {
		t.Fatalf("expected active link status, got %#v", link["status"])
	}
	if link["source"] != nodeAID.String() || link["target"] != nodeBID.String() {
		t.Fatalf("unexpected link endpoints: %#v", link)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_TopologyScopesVMQueryToTenantNodeInstances(t *testing.T) {
	tenantID := uuid.New()
	nodeAID := uuid.New()
	nodeBID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		nodeAID, "pub-key-a", "machine-a", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-a", "10.0.0.10", 10,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	).AddRow(
		nodeBID, "pub-key-b", "machine-b", tenantID, "2.2.2.2:51820", "10.0.0.2", "2.2.2.2", "bj", "vpc-2", "node-b", "10.0.0.11", 11,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))

	var capturedQuery string
	vmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	t.Cleanup(vmServer.Close)

	router := &Router{store: controllerstorage.NewStorageWithDB(db), vmClient: victoriametrics.NewClient(vmServer.URL)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/topology", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(capturedQuery, `wireguard_peer_tx_bytes{instance=~"`) {
		t.Fatalf("expected tenant-scoped instance filter, got query %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, `1\\.1\\.1\\.1`) || !strings.Contains(capturedQuery, `2\\.2\\.2\\.2`) {
		t.Fatalf("expected query to include tenant node public IPs, got %q", capturedQuery)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertResolveAlreadyResolvedReturns400(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, tenantID, nodeID.String(), "high_latency", "warning",
			"Latency high", "", []byte(`{}`), "resolved", now, now,
		))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/"+alertID.String()+"/resolve", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if resp.Code != "ALERT_ALREADY_RESOLVED" {
		t.Fatalf("expected code ALERT_ALREADY_RESOLVED, got %s", resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertResolveCrossTenantReturns404(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	alertID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, otherTenantID, nodeID.String(), "high_latency", "warning",
			"Latency high", "", []byte(`{}`), "active", now, nil,
		))

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

func TestMonitoringAPI_AlertResolveSuccessReturnsResolvedAlert(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, tenantID, nodeID.String(), "high_latency", "warning",
			"Latency high", "", []byte(`{}`), "active", now, nil,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE alerts
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`)).
		WithArgs(alertID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, tenantID, nodeID.String(), "high_latency", "warning",
			"Latency high", "", []byte(`{}`), "resolved", now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, "alert_resolved", "user", "Alert resolved: Latency high", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(uuid.New(), tenantID, nodeID.String(), "alert_resolved", "user", "Alert resolved: Latency high", []byte(`{}`), now))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/"+alertID.String()+"/resolve", nil), tenantID)
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
	if data["id"] != alertID.String() || data["status"] != "resolved" {
		t.Fatalf("unexpected resolved alert payload: %#v", data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_AlertResolveGetAlertFailureReturnsInternalError(t *testing.T) {
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
		WillReturnError(errors.New("db down"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/"+alertID.String()+"/resolve", nil), tenantID)
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

func TestMonitoringAPI_AlertResolveResolveFailureReturnsInternalError(t *testing.T) {
	tenantID := uuid.New()
	alertID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title",
			"message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			alertID, tenantID, nodeID.String(), "high_latency", "warning",
			"Latency high", "", []byte(`{}`), "active", now, nil,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE alerts
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`)).
		WithArgs(alertID).
		WillReturnError(errors.New("resolve failed"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/monitoring/alerts/"+alertID.String()+"/resolve", nil), tenantID)
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

func TestMonitoringAPI_TrafficVMUnavailableReturnsServiceUnavailable(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/traffic?range=24h", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeServiceUnavailable {
		t.Fatalf("expected code %s, got %s", apibase.CodeServiceUnavailable, resp.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMonitoringAPI_TrafficNodeQueryFailureReturnsInternalError(t *testing.T) {
	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnError(errors.New("db timeout"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/traffic?range=24h", nil), tenantID)
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

func TestMonitoringAPI_TrafficVMFailureReturnsInternalError(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectTenantNodesQuery(mock, tenantID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
		"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
		"created_at", "updated_at",
	}).AddRow(
		nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
		time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
	))
	vmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad query", http.StatusUnprocessableEntity)
	}))
	t.Cleanup(vmServer.Close)

	router := &Router{store: controllerstorage.NewStorageWithDB(db), vmClient: victoriametrics.NewClient(vmServer.URL)}
	req := withSuperAdmin(httptest.NewRequest(http.MethodGet, "/api/v2/tenants/"+tenantID.String()+"/monitoring/traffic?range=24h", nil), tenantID)
	rr := httptest.NewRecorder()

	router.HandleTenantScoped(rr, req)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeInternalServerError {
		t.Fatalf("expected code %s, got %s", apibase.CodeInternalServerError, resp.Code)
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
