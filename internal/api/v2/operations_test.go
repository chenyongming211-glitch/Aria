package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aria/internal/api/apibase"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestBatchAgentCommandRejectsInvalidNodeID(t *testing.T) {
	tenantID := uuid.New()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tenants/"+tenantID.String()+"/agents/command",
		strings.NewReader(`{"node_ids":["not-a-uuid"],"command":{"command":"sync"}}`))
	rr := httptest.NewRecorder()

	router.handleTenantBatchAgentCommand(rr, req, tenantID)
	resp := decodeAPIResponse(t, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Code != apibase.CodeInvalidRequest {
		t.Fatalf("expected code %s, got %s", apibase.CodeInvalidRequest, resp.Code)
	}
}
