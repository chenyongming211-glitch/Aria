package v2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
