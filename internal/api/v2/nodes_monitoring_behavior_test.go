package v2

import (
	"context"
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

