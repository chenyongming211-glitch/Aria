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

func TestHandleIssueCertificate_MissingAuthReturns401(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
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

func TestHandleIssueCertificate_SuccessWithEnrollmentTokenAndNodeID(t *testing.T) {
	tenantID := uuid.New()
	nodeID := uuid.New()
	now := time.Now()
	enrollToken := "tk_enroll_nodeid"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tenantID)
	expectNodeLookupByID(mock, tenantID, nodeID, now)
	expectNodeCertificateUpsert(mock, tenantID, nodeID, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectNodeCertificateGetByNodeID(mock, nodeID).
		WillReturnRows(newNodeCertificateRows().AddRow(
			uuid.New(), tenantID, nodeID, "enroll-serial", "enroll-cert", "enroll-ca", now, now.Add(24*time.Hour),
			controllerstorage.CertStatusIssued, now, nil, "", nil, now,
		))

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
	}

	body := `{"token":"` + enrollToken + `","node_id":"` + nodeID.String() + `","csr_pem":"` + jsonEscape(generateCSRPEM(t, "node-enroll-nodeid")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/agents/certificates/issue", strings.NewReader(body))
	rr := httptest.NewRecorder()
	controller.HandleIssueCertificate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHandleIssueCertificate_EnrollmentTokenTenantMismatchReturns401(t *testing.T) {
	tokenTenantID := uuid.New()
	nodeTenantID := uuid.New()
	nodePublicKey := "pub-key-enroll-mismatch"
	now := time.Now()
	enrollToken := "tk_enroll_mismatch"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectEnrollmentTokenValidate(mock, enrollToken, tokenTenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tokenTenantID)
	expectNodeLookupByPublicKey(mock, nodePublicKey, nodeTenantID, now)

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
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
	tenantID := uuid.New()
	now := time.Now()
	enrollToken := "tk_enroll_missing_node_selector"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	expectEnrollmentTokenValidate(mock, enrollToken, tenantID, now)
	expectTenantIDByTokenLookup(mock, enrollToken, tenantID)

	controller := &Controller{
		store:          controllerstorage.NewStorageWithDB(db),
		certService:    newTestCertService(t),
		tokenValidator: token.NewValidator(token.NewStore(db)),
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

	controller := &Controller{
		store:       controllerstorage.NewStorageWithDB(db),
		certService: newTestCertService(t),
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
