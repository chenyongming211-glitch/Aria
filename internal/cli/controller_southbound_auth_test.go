package cli

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"
	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestHandleUnregisterRequiresRuntimeToken(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/unregister", strings.NewReader(`{"public_key":"pub-key-1"}`))
	rr := httptest.NewRecorder()
	controller.HandleUnregister(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated unregister, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleUnregisterRejectsRuntimeTokenForDifferentNode(t *testing.T) {
	auth.SetRuntimeSecret("southbound-runtime-secret")

	tenantID := uuid.New()
	tokenNodeID := uuid.New()
	targetNodeID := uuid.New()
	targetPublicKey := "target-node-public-key"
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, targetPublicKey, tenantID, targetNodeID, "online", now)

	runtimeToken, _, err := auth.GenerateRuntimeToken(tokenNodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/unregister", strings.NewReader(`{"public_key":"`+targetPublicKey+`"}`))
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	rr := httptest.NewRecorder()
	controller.HandleUnregister(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for runtime token node mismatch, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleNetworkManageRequiresJWT(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/network", strings.NewReader(`{"hostname":"node-1","cidr":"10.10.0.0/24","action":"add"}`))
	rr := httptest.NewRecorder()
	controller.HandleNetworkManage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated network manage, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleNetworkManageScopesHostnameLookupToJWTTenant(t *testing.T) {
	auth.SetSecret("network-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, controllerstorage.SystemRoleAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow("{routes:write}"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND status != 'deleted' ORDER BY last_seen DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}))

	token, err := auth.GenerateToken("user-1", "admin", controllerstorage.SystemRoleAdmin, tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/network", strings.NewReader(`{"hostname":"shared-host","cidr":"10.10.0.0/24","action":"add"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	controller.HandleNetworkManage(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when hostname is absent in JWT tenant, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleNetworkManageSuperAdminRequiresTenantID(t *testing.T) {
	auth.SetSecret("network-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	token, err := auth.GenerateToken("user-1", "sysadmin", "super_admin", "")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/network", strings.NewReader(`{"hostname":"shared-host","cidr":"10.10.0.0/24","action":"add"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	controller.HandleNetworkManage(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without tenant_id, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
