package cli

import (
	"net/http"
	"net/http/httptest"
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
