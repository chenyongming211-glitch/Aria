package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"aria/internal/api/apibase"
	"aria/internal/token"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestListTenantTokensRedactsTokenSecrets(t *testing.T) {
	tenantID := uuid.New()
	tokenID := uuid.New()
	now := time.Now().UTC()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token", "tag", "max_uses", "used_count", "expires_at", "created_at", "status"}).
			AddRow(tokenID, "tk_1234567890abcdef", "edge", 1, 0, now.Add(time.Hour), now, "active"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantTokens(rr, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	items := resp.Data.([]interface{})
	item := items[0].(map[string]interface{})
	if _, ok := item["token"]; ok {
		t.Fatalf("list response leaked raw token: %#v", item)
	}
	if item["token_preview"] != "tk_123...cdef" {
		t.Fatalf("unexpected token preview: %#v", item["token_preview"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListTenantTokensAllowsNullTag(t *testing.T) {
	tenantID := uuid.New()
	tokenID := uuid.New()
	now := time.Now().UTC()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token", "tag", "max_uses", "used_count", "expires_at", "created_at", "status"}).
			AddRow(tokenID, "tk_1234567890abcdef", nil, 1, 0, now.Add(time.Hour), now, "active"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantTokens(rr, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	items := resp.Data.([]interface{})
	item := items[0].(map[string]interface{})
	if item["tag"] != "" {
		t.Fatalf("expected empty tag for NULL token tag, got %#v", item["tag"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestControllerInfoIsPublicAndStable(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/controller-info", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected public controller-info 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response: %#v", resp)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object data, got %#v", resp.Data)
	}
	if data["name"] != "aria-controller" {
		t.Fatalf("unexpected controller name: %#v", data["name"])
	}
	if strings.TrimSpace(data["version"].(string)) == "" {
		t.Fatalf("expected non-empty version: %#v", data)
	}
	features := data["supported_features"].([]interface{})
	if !containsStringValue(features, "grpc_sync") || !containsStringValue(features, "runtime_token_refresh") || !containsStringValue(features, "cert_renew") {
		t.Fatalf("missing supported features: %#v", features)
	}
	auth := data["auth"].(map[string]interface{})
	if auth["enrollment"] != true || auth["challenge_auth"] != false {
		t.Fatalf("unexpected auth contract: %#v", auth)
	}
	grpcTLS := data["grpc_tls"].(map[string]interface{})
	if grpcTLS["mode"] != "server" {
		t.Fatalf("expected default server TLS mode, got %#v", grpcTLS)
	}
	if grpcTLS["ca_cert_path"] != "/etc/aria/certs/ca.crt" {
		t.Fatalf("expected default CA cert path, got %#v", grpcTLS)
	}
	if grpcTLS["server_name"] != "example.com" {
		t.Fatalf("expected default TLS server name, got %#v", grpcTLS)
	}
	if data["controller_api_url"] == "" {
		t.Fatalf("expected controller_api_url in controller-info: %#v", data)
	}
	grpc := data["grpc"].(map[string]interface{})
	if grpc["server"] == "" {
		t.Fatalf("expected grpc.server in controller-info: %#v", grpc)
	}
	agent := data["agent"].(map[string]interface{})
	if agent["default_interface"] != "aria0" {
		t.Fatalf("expected default agent interface aria0, got %#v", agent)
	}
	if grpcTLS["ca_cert_url"] == "" {
		t.Fatalf("expected grpc_tls.ca_cert_url in controller-info: %#v", grpcTLS)
	}
	if _, ok := grpcTLS["ca_cert_sha256"].(string); !ok {
		t.Fatalf("expected grpc_tls.ca_cert_sha256 string in controller-info: %#v", grpcTLS)
	}
}

func containsStringValue(values []interface{}, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestControllerInfoIncludesBootstrapURLsAndCAChecksum(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----\n")
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write ca fixture failed: %v", err)
	}
	sum := sha256.Sum256(caPEM)
	expectedChecksum := hex.EncodeToString(sum[:])
	t.Setenv("ARIA_GRPC_CA_CERT", caPath)
	t.Setenv("ARIA_GRPC_TLS_SERVER_NAME", "controller.example.com")

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "https://controller.example.com/api/v2/controller-info", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected controller-info 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	if data["controller_api_url"] != "https://controller.example.com" {
		t.Fatalf("unexpected controller api url: %#v", data["controller_api_url"])
	}
	grpc := data["grpc"].(map[string]interface{})
	if grpc["server"] != "https://controller.example.com:50051" {
		t.Fatalf("unexpected grpc server: %#v", grpc)
	}
	grpcTLS := data["grpc_tls"].(map[string]interface{})
	if grpcTLS["ca_cert_url"] != "https://controller.example.com/api/v2/controller-info/grpc-ca.crt" {
		t.Fatalf("unexpected ca url: %#v", grpcTLS["ca_cert_url"])
	}
	if grpcTLS["ca_cert_sha256"] != expectedChecksum {
		t.Fatalf("unexpected ca checksum: got %#v want %s", grpcTLS["ca_cert_sha256"], expectedChecksum)
	}
	agent := data["agent"].(map[string]interface{})
	if agent["download_url"] != "https://controller.example.com/api/v2/downloads/aria-agent/linux/amd64" {
		t.Fatalf("unexpected agent download url: %#v", agent["download_url"])
	}
}

func TestControllerGRPCCAReturnsPEM(t *testing.T) {
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----\n")
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatalf("write ca fixture failed: %v", err)
	}
	t.Setenv("ARIA_GRPC_CA_CERT", caPath)

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/controller-info/grpc-ca.crt", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected ca endpoint 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := strings.TrimSpace(rr.Body.String()); got != strings.TrimSpace(string(caPEM)) {
		t.Fatalf("unexpected ca response: %q", got)
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/x-pem-file") {
		t.Fatalf("expected pem content type, got %q", contentType)
	}
}

func TestControllerGRPCCARejectsPrivateKeyMaterial(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, []byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write private key fixture failed: %v", err)
	}
	t.Setenv("ARIA_GRPC_CA_CERT", caPath)

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/controller-info/grpc-ca.crt", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected private key material to be rejected, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentInstallerScriptIncludesInstallInitAndSystemd(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/install/agent.sh", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected installer script 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, expected := range []string{
		"aria-agent init",
		"systemctl enable --now aria-agent",
		"/etc/aria/certs/ca.crt",
		"ExecStart=/usr/local/bin/aria-agent up --interface",
		"ip link del aria\\$i",
		"ensure_runtime_commands",
		"wireguard-tools",
		"need ip",
		"need wg",
		"--controller-api-url",
		"--tls-server-name",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected installer script to contain %q\nscript:\n%s", expected, body)
		}
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "text/x-shellscript") {
		t.Fatalf("expected shellscript content type, got %q", contentType)
	}
}

func TestAgentBinaryDownloadReturns404WhenMissing(t *testing.T) {
	t.Setenv("ARIA_AGENT_ARTIFACT_PATH", filepath.Join(t.TempDir(), "missing-agent"))

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/downloads/aria-agent/linux/amd64", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected missing artifact 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentBinaryDownloadReturnsArtifactAndChecksum(t *testing.T) {
	artifact := []byte("fake-agent-binary")
	artifactPath := filepath.Join(t.TempDir(), "aria-agent-linux-amd64")
	if err := os.WriteFile(artifactPath, artifact, 0o755); err != nil {
		t.Fatalf("write artifact fixture failed: %v", err)
	}
	sum := sha256.Sum256(artifact)
	expectedChecksum := hex.EncodeToString(sum[:])
	t.Setenv("ARIA_AGENT_ARTIFACT_PATH", artifactPath)

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	SetupRoutes(mux, controllerstorage.NewStorageWithDB(db), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/downloads/aria-agent/linux/amd64", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected artifact 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.Bytes(); string(got) != string(artifact) {
		t.Fatalf("unexpected artifact body: %q", string(got))
	}
	if rr.Header().Get("X-Aria-Artifact-SHA256") != expectedChecksum {
		t.Fatalf("unexpected checksum header: %q", rr.Header().Get("X-Aria-Artifact-SHA256"))
	}
	if contentType := rr.Header().Get("Content-Type"); !strings.Contains(contentType, "application/octet-stream") {
		t.Fatalf("expected octet-stream content type, got %q", contentType)
	}
}

func TestListTenantTokensReturnsScanError(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now().UTC()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, max_uses, used_count, expires_at, created_at, status 
		 FROM tokens WHERE tenant_id = $1 ORDER BY created_at DESC`)).
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token", "tag", "max_uses", "used_count", "expires_at", "created_at", "status"}).
			AddRow("not-a-uuid", "tk_secret", "edge", 1, 0, now.Add(time.Hour), now, "active"))

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	rr := httptest.NewRecorder()
	router.listTenantTokens(rr, tenantID)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantTokenDefaultsMaxUsesToOneWhenOmitted(t *testing.T) {
	tenantID := uuid.New()
	tokenID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO tokens (token, tag, tenant_id, max_uses, used_count, expires_at, created_by, status)
		         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		         RETURNING id`)).
		WithArgs(sqlmock.AnyArg(), "edge", tenantID.String(), 1, 0, sqlmock.AnyArg(), "", token.StatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tokenID))

	router := &Router{store: controllerstorage.NewStorageWithDB(db), tokenStore: token.NewStore(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/tokens", strings.NewReader(`{"tag":"edge"}`))
	rr := httptest.NewRecorder()
	router.createTenantToken(rr, req, tenantID)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantTokenHonorsRequestedTTL(t *testing.T) {
	tenantID := uuid.New()
	tokenID := uuid.New()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO tokens (token, tag, tenant_id, max_uses, used_count, expires_at, created_by, status)
		         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		         RETURNING id`)).
		WithArgs(sqlmock.AnyArg(), "edge", tenantID.String(), 1, 0, sqlmock.AnyArg(), "", token.StatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tokenID))

	router := &Router{store: controllerstorage.NewStorageWithDB(db), tokenStore: token.NewStore(db)}
	before := time.Now().UTC()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/tokens", strings.NewReader(`{"tag":"edge","ttl":"24h"}`))
	rr := httptest.NewRecorder()
	router.createTenantToken(rr, req, tenantID)
	after := time.Now().UTC()

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp apibase.APIResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	expiresAt, err := time.Parse(time.RFC3339Nano, data["expires_at"].(string))
	if err != nil {
		t.Fatalf("parse expires_at failed: %v", err)
	}
	if expiresAt.Before(before.Add(23*time.Hour)) || expiresAt.After(after.Add(25*time.Hour)) {
		t.Fatalf("expected token to expire near requested 24h TTL, got %s", expiresAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTenantTokenRejectsInvalidTTL(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db), tokenStore: token.NewStore(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/tokens", strings.NewReader(`{"tag":"edge","ttl":"not-a-duration"}`))
	rr := httptest.NewRecorder()
	router.createTenantToken(rr, req, tenantID)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
