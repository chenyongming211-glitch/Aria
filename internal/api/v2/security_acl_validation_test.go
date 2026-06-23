package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreateTenantNodeACLRejectsPortFilterOnUnsupportedProtocol(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	node := &controllerstorage.Node{ID: uuid.New(), TenantID: tenantID, PublicKey: "node-key"}
	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"action":"allow","src_cidr":"any","dst_cidr":"any","protocol":1,"ports":"443","direction":"ingress","enabled":true}`))
	rr := httptest.NewRecorder()

	router.createTenantNodeACL(rr, req, tenantID, node)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateTenantNodeACLRejectsInvalidPortFilterSyntax(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	node := &controllerstorage.Node{ID: uuid.New(), TenantID: tenantID, PublicKey: "node-key"}
	router := &Router{store: controllerstorage.NewStorageWithDB(db)}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"action":"deny","src_cidr":"any","dst_cidr":"any","protocol":6,"ports":"abc","direction":"egress","enabled":true}`))
	rr := httptest.NewRecorder()

	router.createTenantNodeACL(rr, req, tenantID, node)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateTenantNodeBlacklistRejectsInvalidPortRange(t *testing.T) {
	for _, body := range []string{
		`{"port":-1,"description":"negative"}`,
		`{"port":70000,"description":"too high"}`,
	} {
		t.Run(body, func(t *testing.T) {
			db, _, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			tenantID := uuid.New()
			node := &controllerstorage.Node{ID: uuid.New(), TenantID: tenantID, PublicKey: "node-key"}
			router := &Router{store: controllerstorage.NewStorageWithDB(db)}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			rr := httptest.NewRecorder()

			router.createTenantNodeBlacklistRule(rr, req, tenantID, node, controllerstorage.BlacklistScopePorts)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
