package cli

import (
	"database/sql/driver"
	"regexp"
	"testing"

	"aria/pkg/logging"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type bcryptHashForPassword struct {
	password string
}

func (m bcryptHashForPassword) Match(value driver.Value) bool {
	hash, ok := value.(string)
	if !ok {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(m.password)) == nil
}

func TestEnsureSuperAdminSyncsConfiguredPasswordForExistingUser(t *testing.T) {
	t.Setenv("ARIA_SUPER_ADMIN", "sysadmin")
	t.Setenv("ARIA_SUPER_ADMIN_PASSWORD", "NewSecret@123")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userID := uuid.New().String()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE users SET password_hash = $1, must_change_password = TRUE WHERE id = $2`)).
		WithArgs(bcryptHashForPassword{password: "NewSecret@123"}, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	logger, err := logging.NewLogger(&logging.Config{LogDir: t.TempDir(), Component: "test"})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	if err := ensureSuperAdmin(db, logger); err != nil {
		t.Fatalf("ensureSuperAdmin returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureSuperAdminDoesNotRewriteExistingPasswordWithoutEnvOverride(t *testing.T) {
	t.Setenv("ARIA_SUPER_ADMIN", "sysadmin")
	t.Setenv("ARIA_SUPER_ADMIN_PASSWORD", "")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userID := uuid.New().String()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))

	logger, err := logging.NewLogger(&logging.Config{LogDir: t.TempDir(), Component: "test"})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	if err := ensureSuperAdmin(db, logger); err != nil {
		t.Fatalf("ensureSuperAdmin returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
