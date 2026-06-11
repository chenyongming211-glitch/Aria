package service

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"aria/internal/agent/tools"
	"aria/internal/api/middleware"
	"aria/pkg/controllerstorage"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func withAIToolContext(role string, tenantID uuid.UUID) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.UserRoleKey, role)
	ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	return ctx
}

func expectRolePermissions(mock sqlmock.Sqlmock, tenantID uuid.UUID, role string, permissions string) {
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT permissions FROM roles
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
		ORDER BY CASE WHEN name = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`)).
		WithArgs(tenantID, role).
		WillReturnRows(sqlmock.NewRows([]string{"permissions"}).AddRow(permissions))
}

func TestScopedAIToolDeniesMissingPermission(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	tenantID := uuid.New()
	expectRolePermissions(mock, tenantID, controllerstorage.SystemRoleViewer, "{ai:use}")
	service := &aiServiceImpl{store: controllerstorage.NewStorageWithDB(db)}
	called := false
	tool := tools.Tool{
		Name:               "create_token",
		RequiredPermission: "tokens:write",
		TenantScoped:       true,
		Run: func(args string) (string, error) {
			called = true
			return "should not run", nil
		},
	}

	_, err = service.runScopedTool(withAIToolContext(controllerstorage.SystemRoleViewer, tenantID), tool, tool.Run, `{}`)
	if err == nil || !strings.Contains(err.Error(), "tokens:write") {
		t.Fatalf("expected tokens:write denial, got %v", err)
	}
	if called {
		t.Fatal("tool ran despite missing permission")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestScopedAIToolOverridesCallerTenantID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	tenantID := uuid.New()
	otherTenantID := uuid.New()
	expectRolePermissions(mock, tenantID, controllerstorage.SystemRoleAdmin, "{routes:write}")
	service := &aiServiceImpl{store: controllerstorage.NewStorageWithDB(db)}
	var capturedArgs string
	tool := tools.Tool{
		Name:               "add_route",
		RequiredPermission: "routes:write",
		TenantScoped:       true,
		Run: func(args string) (string, error) {
			capturedArgs = args
			return "ok", nil
		},
	}

	result, err := service.runScopedTool(
		withAIToolContext(controllerstorage.SystemRoleAdmin, tenantID),
		tool,
		tool.Run,
		`{"hostname":"node-a","tenant_id":"`+otherTenantID.String()+`"}`,
	)
	if err != nil {
		t.Fatalf("runScopedTool failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %s", result)
	}
	if !strings.Contains(capturedArgs, tenantID.String()) {
		t.Fatalf("expected scoped tenant %s in args %s", tenantID, capturedArgs)
	}
	if strings.Contains(capturedArgs, otherTenantID.String()) {
		t.Fatalf("caller tenant was not overridden: %s", capturedArgs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
