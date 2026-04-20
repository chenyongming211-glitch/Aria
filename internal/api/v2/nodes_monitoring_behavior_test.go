package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

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

