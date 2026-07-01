package controllerstorage

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestNormalizeRoleNameMapsLegacyTenantRoles(t *testing.T) {
	cases := map[string]string{
		"member":      SystemRoleOperator,
		" owner ":     SystemRoleAdmin,
		"ADMIN":       SystemRoleAdmin,
		"SUPER_ADMIN": "super_admin",
		"custom":      "custom",
	}

	for input, want := range cases {
		if got := NormalizeRoleName(input); got != want {
			t.Fatalf("NormalizeRoleName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestListRolesReturnsRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tenantID := uuid.New()
	roleID := uuid.New()
	rowErr := errors.New("role row iteration failed")
	now := time.Now()
	mock.ExpectQuery("SELECT id, tenant_id, name, description, is_system, permissions, created_at, updated_at").
		WithArgs(tenantID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "name", "description", "is_system", "permissions", "created_at", "updated_at",
		}).
			AddRow(roleID, tenantID, "admin", "admin role", true, "{nodes:read}", now, now).
			AddRow(uuid.New(), tenantID, "viewer", "viewer role", true, "{nodes:read}", now, now).
			RowError(1, rowErr))

	_, err = NewStorageWithDB(db).ListRoles(tenantID)
	if !errors.Is(err, rowErr) {
		t.Fatalf("expected rows.Err %v, got %v", rowErr, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureAllTenantRolesReturnsRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rowErr := errors.New("tenant row iteration failed")
	mock.ExpectQuery("SELECT id FROM tenants").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(uuid.New()).
			RowError(0, rowErr))

	err = NewStorageWithDB(db).EnsureAllTenantRoles()
	if err == nil || !strings.Contains(err.Error(), rowErr.Error()) {
		t.Fatalf("expected rows.Err %v, got %v", rowErr, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
