package cli

import (
	"regexp"
	"testing"

	"aria/pkg/controllerstorage"
	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestEnsureDefaultTenantCreatesSystemRolesForNewTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tenants")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO tenants (code, name) VALUES ($1, $2) ON CONFLICT (code) DO UPDATE SET name = $2 RETURNING id`)).
		WithArgs("default", "Aria Default").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tenantID))
	for _, role := range []string{"admin", "operator", "viewer"} {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO roles (tenant_id, name, description, is_system, permissions)
				VALUES ($1, $2, $3, true, $4)
				ON CONFLICT (tenant_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				is_system = true,
				permissions = EXCLUDED.permissions,
				updated_at = NOW()`)).
			WithArgs(tenantID, role, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	logger, err := logging.NewLogger(&logging.Config{LogDir: t.TempDir(), Component: "test"})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	if err := ensureDefaultTenant(controllerstorage.NewStorageWithDB(db), logger); err != nil {
		t.Fatalf("ensureDefaultTenant failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
