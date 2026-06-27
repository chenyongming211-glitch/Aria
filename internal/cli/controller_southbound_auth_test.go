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

func TestHandleUnregisterRejectsInactiveRuntimeNode(t *testing.T) {
	auth.SetRuntimeSecret("southbound-runtime-secret")
	t.Cleanup(func() { auth.SetRuntimeSecret("") })

	tenantID := uuid.New()
	nodeID := uuid.New()
	publicKey := "inactive-node-public-key"
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, nodeID, "suspended", now)

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/unregister", strings.NewReader(`{"public_key":"`+publicKey+`"}`))
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	rr := httptest.NewRecorder()
	controller.HandleUnregister(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for inactive runtime node, got %d body=%s", rr.Code, rr.Body.String())
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

func TestHandleNetworkManageRejectsInactiveTargetNode(t *testing.T) {
	auth.SetSecret("network-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
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
		}).AddRow(
			nodeID, "suspended-key", "machine-1", tenantID, "1.1.1.1:51820", "", "1.1.1.1", "sh", "vpc-1", "suspended-host", "100.64.0.2", 2,
			now.Unix(), now.Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "suspended", int64(0), "{}", "", now, now,
		))

	token, err := auth.GenerateToken("user-1", "admin", controllerstorage.SystemRoleAdmin, tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/network", strings.NewReader(`{"hostname":"suspended-host","cidr":"10.10.0.0/24","action":"add"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	controller.HandleNetworkManage(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for inactive target node, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleNetworkManagePreservesOfflineNodeStatus(t *testing.T) {
	auth.SetSecret("network-jwt-secret")
	t.Cleanup(func() { auth.SetSecret("") })

	tenantID := uuid.New()
	nodeID := uuid.New()
	commandID := uuid.New().String()
	deliveryID := uuid.New()
	now := time.Now()
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
		}).AddRow(
			nodeID, "offline-key", "machine-1", tenantID, "1.1.1.1:51820", "", "1.1.1.1", "sh", "vpc-1", "offline-host", "100.64.0.2", 2,
			now.Add(-time.Hour).Unix(), now.Add(-24*time.Hour).Unix(), "member", "kernel", "6.0", true, "offline", int64(now.Add(-time.Hour).Unix()), "{}", "", now, now,
		))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes SET advertised_routes = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`)).
		WithArgs(sqlmock.AnyArg(), nodeID, tenantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO node_control_states")).
		WithArgs(tenantID, nodeID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "node_id", "desired_state_version", "desired_state_metadata", "desired_state_updated_at",
			"applied_state_version", "applied_state_updated_at", "observed_state",
			"observed_message", "observed_at", "last_sync_at", "last_sync_error",
			"created_at", "updated_at",
		}).AddRow(
			tenantID, nodeID, "dsv-network", []byte(`{}`), now,
			"", nil, "",
			"", nil, nil, "",
			now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands ac")).
		WithArgs("offline-key", controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged, tenantID, nodeID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(tenantID, nodeID, controllerstorage.AgentCommandStatusStale, sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO agent_commands")).
		WithArgs("offline-key", "sync", sqlmock.AnyArg(), controllerstorage.AgentCommandStatusPending, 1, 60).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(commandID, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO policy_deliveries")).
		WithArgs(tenantID, nodeID, "route", "10.10.0.0/24", "10.10.0.0/24", "create", commandID, controllerstorage.AgentCommandStatusPending, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "policy_domain", "policy_ref", "policy_name",
			"action", "command_id", "command_status", "last_error", "metadata", "created_at", "updated_at", "completed_at",
		}).AddRow(
			deliveryID, tenantID, nodeID, "route", "10.10.0.0/24", "10.10.0.0/24",
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

	token, err := auth.GenerateToken("user-1", "admin", controllerstorage.SystemRoleAdmin, tenantID.String())
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/network", strings.NewReader(`{"hostname":"offline-host","cidr":"10.10.0.0/24","action":"add"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	controller.HandleNetworkManage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 while preserving offline status, got %d body=%s", rr.Code, rr.Body.String())
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
