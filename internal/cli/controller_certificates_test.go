package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/auth"
	"aria/internal/security/certissuance"
	"aria/internal/token"
	"aria/pkg/controllerstorage"
	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

const upsertNodeCertificateQuery = `
		INSERT INTO node_certificates (
			tenant_id, node_id, serial_number, cert_pem, ca_pem,
			not_before, not_after, status, issued_at, renewed_from, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW())
		ON CONFLICT (node_id) DO UPDATE SET
			serial_number = EXCLUDED.serial_number,
			cert_pem = EXCLUDED.cert_pem,
			ca_pem = EXCLUDED.ca_pem,
			not_before = EXCLUDED.not_before,
			not_after = EXCLUDED.not_after,
			status = EXCLUDED.status,
			issued_at = NOW(),
			revoked_at = NULL,
			revoke_reason = '',
			renewed_from = EXCLUDED.renewed_from,
			updated_at = NOW()
	`

const getNodeCertificateByNodeIDQuery = `
		SELECT id, tenant_id, node_id, serial_number, cert_pem, ca_pem,
		       not_before, not_after, status, issued_at, revoked_at,
		       COALESCE(revoke_reason, ''), renewed_from, updated_at
		FROM node_certificates
		WHERE node_id = $1
	`

func init() {
	auth.SetRuntimeSecret("test-runtime-secret")
}

func TestHandleIssueCertificate_SuccessWithRuntimeToken(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	certService := newTestCertService(t)
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	csrPEM := generateCSRPEM(t, "node-"+nodeID.String())

	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateUpsert(mock, tenantID, nodeID, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, nodeID, "abc123", "cert", "ca", now, now.Add(24*time.Hour),
			controllerstorage.CertStatusIssued, now, nil, "", nil, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: certService,
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(csrPEM) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["status"] != "success" {
		t.Fatalf("expected status=success, got %#v", payload["status"])
	}
	if payload["node_id"] != nodeID.String() {
		t.Fatalf("expected node_id=%s, got %#v", nodeID.String(), payload["node_id"])
	}
	if _, ok := payload["cert_pem"]; !ok {
		t.Fatalf("expected cert_pem in response")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_SuccessWithBearerRuntimeToken(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	certService := newTestCertService(t)
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	csrPEM := generateCSRPEM(t, "node-bearer-"+nodeID.String())

	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateUpsert(mock, tenantID, nodeID, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, nodeID, "bearer-serial", "bearer-cert", "bearer-ca", now, now.Add(24*time.Hour),
			controllerstorage.CertStatusIssued, now, nil, "", nil, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: certService,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(`{"csr_pem":"`+jsonEscape(csrPEM)+`"}`))
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_InvalidCSRReturns500(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	certService := newTestCertService(t)
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	expectNodeLookupByID(mock, tenantID, nodeID, now)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: certService,
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"not-a-pem"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_RuntimeTokenDeniedForDeletedNode(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	certService := newTestCertService(t)
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-deleted", "machine-deleted", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-deleted", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "deleted", int64(0), "{}", "", now, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: certService,
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-deleted")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_MissingAuthReturns401(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(`{"csr_pem":"x"}`))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandleIssueCertificate_MethodNotAllowedReturns405(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/agents/certificates/issue", nil)
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleIssueCertificate_ServiceDisabledReturns503(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store: controllerstorage.NewStorageWithDB(db),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(`{"runtime_token":"x","csr_pem":"y"}`))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

func TestHandleIssueCertificate_InvalidRuntimeTokenReturns401(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	body := `{"runtime_token":"invalid-token","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-invalid-token")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleIssueCertificate_RuntimeTokenTenantMismatchReturns401(t *testing.T) {
	tenantFromToken := uuid.New()
	nodeTenant := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantFromToken.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	expectNodeLookupByID(mock, nodeTenant, nodeID, now)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-tenant-mismatch")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_StorageUpsertFailureReturns500(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateUpsert(mock, tenantID, nodeID, nil).
		WillReturnError(sql.ErrConnDone)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-upsert-fail")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_EnrollmentTokenRequiresRuntimeToken(t *testing.T) {
	nodeID := uuid.New()
	enrollToken := "tk_enroll_nodeid"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"token":"` + enrollToken + `","node_id":"` + nodeID.String() + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-enroll-nodeid")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_EnrollmentTokenTenantMismatchReturns401(t *testing.T) {
	nodePublicKey := "pub-key-enroll-mismatch"
	enrollToken := "tk_enroll_mismatch"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"token":"` + enrollToken + `","public_key":"` + nodePublicKey + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-enroll-mismatch")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_EnrollmentTokenWithoutNodeSelectorReturns401(t *testing.T) {
	enrollToken := "tk_enroll_missing_node_selector"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"token":"` + enrollToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-enroll-no-selector")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegister_ReRegistrationRequiresNodeProof(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-proof-required-1234567890"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	body := `{"public_key":"` + publicKey + `","hostname":"node-proof"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestValidateEnrollmentTokenTenantRejectsInactiveTenant(t *testing.T) {
	for _, status := range []string{"suspended", "deleted"} {
		t.Run(status, func(t *testing.T) {
			tenantID := uuid.New()
			enrollToken := "tk_register_inactive_" + status
			now := time.Now()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
			expectTenantIDByTokenLookup(mock, enrollToken, tenantID)
			expectTenantStatusLookup(mock, tenantID, status)

			controller := &Controller{
				store:          controllerstorage.NewStorageWithDB(db),
				tokenValidator: token.NewValidator(token.NewStore(db)),
				logger:         logging.GetLogger(),
			}

			_, authErr := controller.validateEnrollmentTokenTenant(enrollToken)
			if authErr == nil {
				t.Fatalf("expected inactive tenant %q to reject enrollment token", status)
			}
			if authErr.status != http.StatusForbidden {
				t.Fatalf("expected 403 for inactive tenant %q, got %d", status, authErr.status)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func TestHandleRegister_ReRegistrationRejectsEnrollmentMachineMismatch(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-machine-mismatch-1234567890"
	enrollToken := "tk_register_machine_mismatch"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusMachineAndID(mock, publicKey, tenantID, persistedNodeID, "online", "machine-existing", now)
	expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tenantID)
	expectTenantStatusLookup(mock, tenantID, "active")

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"public_key":"` + publicKey + `","hostname":"node-proof","machine_id":"machine-attacker","token":"` + enrollToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegister_ReRegistrationRejectsCrossRegionRouteConflict(t *testing.T) {
	auth.SetRuntimeSecret("route-conflict-runtime-secret")

	tenantID := uuid.New()
	nodeID := uuid.New()
	conflictNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-route-conflict-1234567890"
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, nodeID, "online", now)
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, nodeID, "online", now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND status != 'deleted' ORDER BY last_seen DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			conflictNodeID, "conflict-key", "machine-b", tenantID, "2.2.2.2:51820", "10.0.0.2", "2.2.2.2", "bj", "vpc-2", "node-b", "10.0.0.11", 11,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{10.10.0.0/16}", "", now, now,
		))

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	body := `{"public_key":"` + publicKey + `","runtime_token":"` + runtimeToken + `","hostname":"node-a","region":"sh","advertised_routes":["10.10.1.0/24"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegister_CSRUsesPersistedNodeIDForCertUpsert(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-csr-existing-1234567890"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)
	expectNodeCertificateUpsert(mock, tenantID, persistedNodeID, nil).
		WillReturnError(sql.ErrConnDone)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}

	runtimeToken, _, err := auth.GenerateRuntimeToken(persistedNodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	body := `{"public_key":"` + publicKey + `","runtime_token":"` + runtimeToken + `","hostname":"node-a","csr_pem":"` + jsonEscape(generateCSRPEM(t, "register-existing")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegister_CSRFailureRollsBackFreshEnrollment(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-csr-deleted-1234567890"
	enrollToken := "tk_register_deleted_reenroll"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "deleted", now)
	expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tenantID)
	expectTenantStatusLookup(mock, tenantID, "active")
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "deleted", now)
	expectSaveNodeSuccess(mock, publicKey, tenantID, persistedNodeID)
	expectEnrollmentTokenConsume(mock, enrollToken, publicKey)
	expectNodeCertificateUpsert(mock, tenantID, persistedNodeID, nil).
		WillReturnError(sql.ErrConnDone)
	expectMarkNodeDeleted(mock, publicKey)

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"public_key":"` + publicKey + `","hostname":"node-b","token":"` + enrollToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "register-deleted")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegisterDoesNotConsumeEnrollmentTokenWhenSaveFails(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-save-fails-before-consume-1234567890"
	enrollToken := "tk_register_save_fails_before_consume"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "deleted", now)
	expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tenantID)
	expectTenantStatusLookup(mock, tenantID, "active")
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "deleted", now)
	mock.ExpectQuery(`INSERT INTO nodes \(`).
		WithArgs(
			publicKey,
			sqlmock.AnyArg(),
			tenantID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnError(sql.ErrConnDone)

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		tokenValidator: token.NewValidator(token.NewStore(db)),
		logger:         logging.GetLogger(),
	}

	body := `{"public_key":"` + publicKey + `","hostname":"node-b","token":"` + enrollToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRegister_CSRSuccessIncludesCertificateInSyncResponse(t *testing.T) {
	tenantID := uuid.New()
	persistedNodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-register-csr-success-1234567890"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// HandleRegister pre-check + save + issue
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)
	expectNodeCertificateUpsert(mock, tenantID, persistedNodeID, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectNodeCertificateGetByNodeID(mock, persistedNodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, persistedNodeID, "register-serial", "register-cert", "register-ca",
			now, now.Add(48*time.Hour), controllerstorage.CertStatusIssued, now, nil, "", nil, now,
		))
	expectSaveNodeSuccess(mock, publicKey, tenantID, persistedNodeID)

	// syncNode fallback path for empty token
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs("").
		WillReturnError(sql.ErrNoRows)
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)

	// peers query by tenant returns the requesting node so sync can load node-scoped ACLs
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')`,
	)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			persistedNodeID, publicKey, "machine-cert", tenantID, "1.1.1.1:51820", "", "1.1.1.1", "test-region", "", "node-c", "100.64.0.2", 2,
			now.Unix(), now.Unix(), "spoke", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))

	// node-scoped ACL query returns empty rule list
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, COALESCE(name, ''), action, src_group_id, dst_group_id, COALESCE(src_cidr::text, src_net::text, ''), COALESCE(dst_cidr::text, dst_net::text, ''),
		        COALESCE(dst_port, CASE WHEN min_port = max_port THEN max_port ELSE 0 END, 0), COALESCE(protocol, 0),
		        COALESCE(direction, 'ingress'), COALESCE(ports, CASE WHEN min_port > 0 AND max_port > 0 AND min_port <> max_port THEN min_port::text || '-' || max_port::text WHEN min_port > 0 THEN min_port::text ELSE '' END),
		        priority, enabled, COALESCE(description, ''),
		        created_at, updated_at
		   FROM acl_rules
		  WHERE tenant_id = $1 AND node_id = $2 AND enabled = true
		  ORDER BY priority ASC, created_at ASC
	`)).
		WithArgs(tenantID, persistedNodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "name", "action", "src_group_id", "dst_group_id", "src_cidr", "dst_cidr", "dst_port", "protocol", "direction", "ports", "priority", "enabled", "description", "created_at", "updated_at",
		}))

	expectSyncNodeControlState(mock, tenantID, persistedNodeID, "dsv-register-cert", now)

	// attachNodeCertificateToSyncResponse
	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, persistedNodeID, "online", now)
	expectNodeCertificateGetByNodeID(mock, persistedNodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, persistedNodeID, "register-serial", "register-cert", "register-ca",
			now, now.Add(48*time.Hour), controllerstorage.CertStatusIssued, now, nil, "", nil, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}

	runtimeToken, _, err := auth.GenerateRuntimeToken(persistedNodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	body := `{"public_key":"` + publicKey + `","runtime_token":"` + runtimeToken + `","hostname":"node-c","csr_pem":"` + jsonEscape(generateCSRPEM(t, "register-success")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRegister(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["certificate_pem"] != "register-cert" {
		t.Fatalf("expected certificate_pem=register-cert, got %#v", payload["certificate_pem"])
	}
	if payload["certificate_ca"] != "register-ca" {
		t.Fatalf("expected certificate_ca=register-ca, got %#v", payload["certificate_ca"])
	}
	if _, ok := payload["certificate_not_after"]; !ok {
		t.Fatalf("expected certificate_not_after in response")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestAttachNodeCertificateToSyncResponseSkipsExpiredCertificate(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-expired-cert-sync-1234567890"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, nodeID, "online", now)
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, nodeID, "expired-serial", "expired-cert", "expired-ca",
			now.Add(-48*time.Hour), now.Add(-time.Hour), controllerstorage.CertStatusIssued, now.Add(-48*time.Hour), nil, "", nil, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}
	resp := &SyncResponse{}
	controller.attachNodeCertificateToSyncResponse(publicKey, resp)

	if resp.CertificatePEM != "" || resp.CertificateCA != "" || resp.CertificateNotAfter != 0 {
		t.Fatalf("expected expired certificate to be skipped, got %#v", resp)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleUnregister_RevokesCertificateAndCreatesAuditEvent(t *testing.T) {
	auth.SetRuntimeSecret("unregister-runtime-secret")

	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	publicKey := "pubkey-unregister-lifecycle-1234567890"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectNodeLookupByPublicKeyWithStatusAndID(mock, publicKey, tenantID, nodeID, "online", now)
	expectNodeLifecycleTransition(mock, publicKey, tenantID, nodeID, "online", "deleted", "node unregistered", "node_unregistered", "agent", "Node unregistered", now)

	controller := &Controller{
		store:  controllerstorage.NewStorageWithDB(db),
		logger: logging.GetLogger(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/unregister", strings.NewReader(`{"public_key":"`+publicKey+`"}`))
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	rr := httptest.NewRecorder()
	controller.HandleUnregister(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_NoExistingCertReturns404(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnError(sql.ErrNoRows)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-renew")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_LoadExistingCertFailureReturns500(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnError(sql.ErrConnDone)
	expectNoActiveAlertByNodeAndType(mock, tenantID, nodeID, "certificate_renew_failed")
	expectAlertCreate(mock, tenantID, nodeID, "certificate_renew_failed", "warning", "节点证书续签失败")
	expectAuditEventCreate(mock, tenantID, nodeID, "certificate_renew_failed", "system", "节点 node-1 证书续签失败")

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
		logger:      logging.GetLogger(),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-renew-load-fail")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_RuntimeTokenNodeNotFoundReturns401(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnError(sql.ErrNoRows)

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-renew-not-found")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_SuccessPersistsRenewedFrom(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	existingCertID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	certService := newTestCertService(t)
	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}
	csrPEM := generateCSRPEM(t, "node-renew-success")

	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			existingCertID, tenantID, nodeID, "old-serial", "old-cert", "old-ca", now.Add(-24*time.Hour), now.Add(24*time.Hour),
			controllerstorage.CertStatusIssued, now.Add(-24*time.Hour), nil, "", nil, now.Add(-time.Hour),
		))
	expectNodeCertificateUpsert(mock, tenantID, nodeID, existingCertID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectNoActiveAlertByNodeAndType(mock, tenantID, nodeID, "certificate_expiring")
	expectNoActiveAlertByNodeAndType(mock, tenantID, nodeID, "certificate_renew_failed")
	expectAuditEventCreate(mock, tenantID, nodeID, "certificate_renewed", "system", "节点 node-1 证书已续签")
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, nodeID, "new-serial", "new-cert", "new-ca", now, now.Add(48*time.Hour),
			controllerstorage.CertStatusIssued, now, nil, "", existingCertID.String(), now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: certService,
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(csrPEM) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload["status"] != "success" {
		t.Fatalf("expected status=success, got %#v", payload["status"])
	}
	if payload["node_id"] != nodeID.String() {
		t.Fatalf("expected node_id=%s, got %#v", nodeID.String(), payload["node_id"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_RevokedCertReturns409(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	existingCertID := uuid.New()
	now := time.Now()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeToken, _, err := auth.GenerateRuntimeToken(nodeID.String(), tenantID.String())
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			existingCertID, tenantID, nodeID, "old-serial", "old-cert", "old-ca", now.Add(-24*time.Hour), now.Add(24*time.Hour),
			controllerstorage.CertStatusRevoked, now.Add(-24*time.Hour), now, "node deleted", nil, now,
		))

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	body := `{"runtime_token":"` + runtimeToken + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-renew-revoked")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleRenewCertificate_CSRRequiredReturns400(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/renew", strings.NewReader(`{"runtime_token":"x"}`))
	rr := httptest.NewRecorder()
	controller.HandleRenewCertificate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func newTestCertService(t *testing.T) *certissuance.Service {
	t.Helper()
	caCertPEM, caKeyPEM := generateTestCA(t)
	svc, err := certissuance.NewServiceFromPEM(caCertPEM, caKeyPEM, 24*time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("NewServiceFromPEM failed: %v", err)
	}
	return svc
}

func expectNodeLookupByID(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE id = $1`,
	)).
		WithArgs(nodeID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, "pub-key-1", "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))
}

func expectNodeLookupByPublicKey(mock sqlmock.Sqlmock, publicKey string, tenantID uuid.UUID, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`,
	)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			uuid.New(), publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, "online", int64(0), "{}", "", now, now,
		))
}

func expectNodeLookupByPublicKeyWithStatusAndID(
	mock sqlmock.Sqlmock,
	publicKey string,
	tenantID uuid.UUID,
	nodeID uuid.UUID,
	status string,
	now time.Time,
) {
	expectNodeLookupByPublicKeyWithStatusMachineAndID(mock, publicKey, tenantID, nodeID, status, "machine-1", now)
}

func expectNodeLookupByPublicKeyWithStatusMachineAndID(
	mock sqlmock.Sqlmock,
	publicKey string,
	tenantID uuid.UUID,
	nodeID uuid.UUID,
	status string,
	machineID string,
	now time.Time,
) {
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1`,
	)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, publicKey, machineID, tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, status, int64(0), "{}", "", now, now,
		))
}

func expectEnrollmentTokenValidate(mock sqlmock.Sqlmock, tokenValue string, tenantID uuid.UUID, now time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
		       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
		FROM tokens
		WHERE token = $1
	`)).
		WithArgs(tokenValue).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "token", "tag", "tenant_id", "max_uses", "used_count", "expires_at", "created_at",
			"created_by", "status", "last_used_at", "last_used_by",
		}).AddRow(
			uuid.New(), tokenValue, "enroll", tenantID.String(), 10, 0, now.Add(2*time.Hour), now.Add(-time.Hour),
			"tester", string(token.StatusActive), nil, "",
		))
}

func expectTenantIDByTokenLookup(mock sqlmock.Sqlmock, tokenValue string, tenantID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id FROM tokens WHERE token = $1`)).
		WithArgs(tokenValue).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(tenantID))
}

func expectTenantStatusLookup(mock sqlmock.Sqlmock, tenantID uuid.UUID, status string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM tenants WHERE id = $1`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(status))
}

func expectEnrollmentTokenConsume(mock sqlmock.Sqlmock, tokenValue, publicKey string) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE tokens
		SET used_count = used_count + 1,
		    last_used_at = NOW(),
		    last_used_by = $2,
		    status = CASE
		        WHEN max_uses > 0 AND used_count + 1 >= max_uses THEN 'exhausted'
		        ELSE status
		    END
		WHERE token = $1 AND status = 'active' AND (max_uses = 0 OR used_count < max_uses)`)).
		WithArgs(tokenValue, publicKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectNodeCertificateUpsert(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, renewedFrom interface{}) *sqlmock.ExpectedExec {
	return mock.ExpectExec(regexp.QuoteMeta(upsertNodeCertificateQuery)).
		WithArgs(
			tenantID,
			nodeID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			controllerstorage.CertStatusIssued,
			renewedFrom,
		)
}

func expectNodeCertificateGetByNodeID(mock sqlmock.Sqlmock, nodeID uuid.UUID) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(getNodeCertificateByNodeIDQuery)).
		WithArgs(nodeID)
}

func expectNoActiveAlertByNodeAndType(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, alertType string) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		       context, status, created_at, resolved_at
		FROM alerts
		WHERE tenant_id = $1 AND node_id = $2 AND alert_type = $3 AND status = 'active'
		LIMIT 1
	`)).
		WithArgs(tenantID, nodeID, alertType).
		WillReturnError(sql.ErrNoRows)
}

func expectAlertCreate(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, alertType, severity, title string) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO alerts (tenant_id, node_id, alert_type, severity, title, message, context, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, node_id, alert_type, severity, title, COALESCE(message, ''),
		          context, status, created_at, resolved_at
	`)).
		WithArgs(tenantID, nodeID, alertType, severity, title, sqlmock.AnyArg(), sqlmock.AnyArg(), "active").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "alert_type", "severity", "title", "message", "context", "status", "created_at", "resolved_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID,
			alertType,
			severity,
			title,
			"",
			[]byte(`{}`),
			"active",
			time.Now(),
			nil,
		))
}

func expectAuditEventCreate(mock sqlmock.Sqlmock, tenantID, nodeID uuid.UUID, eventType, actor, summary string) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, node_id, event_type, actor, summary, detail, created_at
	`)).
		WithArgs(tenantID, nodeID, eventType, actor, summary, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "node_id", "event_type", "actor", "summary", "detail", "created_at",
		}).AddRow(
			uuid.New(),
			tenantID,
			nodeID,
			eventType,
			actor,
			summary,
			[]byte(`{}`),
			time.Now(),
		))
}

func expectSaveNodeSuccess(mock sqlmock.Sqlmock, publicKey string, tenantID, persistedNodeID uuid.UUID) {
	mock.ExpectQuery(`INSERT INTO nodes \(`).
		WithArgs(
			publicKey,
			sqlmock.AnyArg(),
			tenantID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(persistedNodeID, time.Now(), time.Now()))
}

func expectMarkNodeDeleted(mock sqlmock.Sqlmock, publicKey string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM nodes WHERE public_key = $1 FOR UPDATE`)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE nodes
		SET status = 'deleted', updated_at = NOW()
		WHERE public_key = $1`)).
		WithArgs(publicKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE node_certificates
		SET status = $2,
		    revoked_at = NOW(),
		    revoke_reason = $3,
		    updated_at = NOW()
		WHERE node_id = $1 AND status = $4`)).
		WithArgs(sqlmock.AnyArg(), controllerstorage.CertStatusRevoked, "node_deleted", controllerstorage.CertStatusIssued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE policy_deliveries")).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node deleted", controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_commands")).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node deleted", controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectNodeLifecycleTransition(
	mock sqlmock.Sqlmock,
	publicKey string,
	tenantID, nodeID uuid.UUID,
	fromStatus, targetStatus, revokeReason, eventType, actor, summary string,
	now time.Time,
) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE public_key = $1 FOR UPDATE`,
	)).
		WithArgs(publicKey).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "public_key", "machine_id", "tenant_id", "endpoint", "private_ip", "public_ip", "region", "vpc_id", "hostname", "assigned_ip", "ip_offset",
			"last_seen", "registered_at", "role", "runtime_mode", "kernel_version", "has_aesni", "status", "offline_since", "advertised_routes", "enrolled_with_token",
			"created_at", "updated_at",
		}).AddRow(
			nodeID, publicKey, "machine-1", tenantID, "1.1.1.1:51820", "10.0.0.1", "1.1.1.1", "sh", "vpc-1", "node-1", "10.0.0.10", 10,
			time.Now().Unix(), time.Now().Add(-time.Hour).Unix(), "member", "kernel", "6.0", true, fromStatus, int64(0), "{}", "", now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE nodes
		SET status = $2, updated_at = NOW()
		WHERE public_key = $1
	`)).
		WithArgs(publicKey, targetStatus).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
			UPDATE node_certificates
			SET status = $2,
			    revoked_at = NOW(),
			    revoke_reason = $3,
			    updated_at = NOW()
			WHERE node_id = $1 AND status = $4
		`)).
		WithArgs(nodeID, controllerstorage.CertStatusRevoked, revokeReason, controllerstorage.CertStatusIssued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)).
		WithArgs(tenantID, nodeID, controllerstorage.AuditCertRevoked, actor, "Node certificate revoked due to node lifecycle change", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE policy_deliveries
		SET command_status = $2,
		    last_error = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE command_id IN (
			SELECT id
			FROM agent_commands
			WHERE node_public_key = $1 AND status IN ($4, $5, $6)
		)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node status changed to "+targetStatus, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE agent_commands
		SET status = $2,
		    message = $3,
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE node_public_key = $1 AND status IN ($4, $5, $6)
	`)).
		WithArgs(publicKey, controllerstorage.AgentCommandStatusFailed, "node status changed to "+targetStatus, controllerstorage.AgentCommandStatusPending, controllerstorage.AgentCommandStatusSent, controllerstorage.AgentCommandStatusAcknowledged).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
			INSERT INTO audit_events (tenant_id, node_id, event_type, actor, summary, detail)
			VALUES ($1, $2, $3, $4, $5, $6)
		`)).
		WithArgs(tenantID, nodeID, eventType, actor, summary, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func newNodeCertificateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "tenant_id", "node_id", "serial_number", "cert_pem", "ca_pem",
		"not_before", "not_after", "status", "issued_at", "revoked_at",
		"revoke_reason", "renewed_from", "updated_at",
	})
}

func generateCSRPEM(t *testing.T, cn string) string {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: cn},
	}, leafKey)
	if err != nil {
		t.Fatalf("CreateCertificateRequest failed: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
}

func generateTestCA(t *testing.T) ([]byte, []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aria-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(48 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, caKey.Public(), caKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	caKeyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey failed: %v", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caKeyDER})
	return caCertPEM, caKeyPEM
}

func jsonEscape(raw string) string {
	b, _ := json.Marshal(raw)
	if len(b) < 2 {
		return raw
	}
	return string(b[1 : len(b)-1])
}
