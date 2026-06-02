package cli

import (
	"database/sql/driver"
	"regexp"
	"strings"
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
	oldHash, err := bcrypt.GenerateFromPassword([]byte("OldSecret@123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}).AddRow(userID, string(oldHash)))
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
	oldHash, err := bcrypt.GenerateFromPassword([]byte("ExistingSecret@123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}).AddRow(userID, string(oldHash)))

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

func TestEnsureSuperAdminDoesNotRewriteMatchingConfiguredPassword(t *testing.T) {
	t.Setenv("ARIA_SUPER_ADMIN", "sysadmin")
	t.Setenv("ARIA_SUPER_ADMIN_PASSWORD", "CurrentSecret@123")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userID := uuid.New().String()
	currentHash, err := bcrypt.GenerateFromPassword([]byte("CurrentSecret@123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}).AddRow(userID, string(currentHash)))

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

func TestEnsureSuperAdminRequiresConfiguredPasswordForFreshInstall(t *testing.T) {
	t.Setenv("ARIA_SUPER_ADMIN", "sysadmin")
	t.Setenv("ARIA_SUPER_ADMIN_PASSWORD", "")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("sysadmin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users WHERE role = 'super_admin'")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	logger, err := logging.NewLogger(&logging.Config{LogDir: t.TempDir(), Component: "test"})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	err = ensureSuperAdmin(db, logger)
	if err == nil {
		t.Fatal("expected missing ARIA_SUPER_ADMIN_PASSWORD to fail fresh super admin creation")
	}
	if !strings.Contains(err.Error(), "ARIA_SUPER_ADMIN_PASSWORD") {
		t.Fatalf("expected ARIA_SUPER_ADMIN_PASSWORD error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestEnsureSuperAdminCreatesConfiguredUsernameWhenAnotherSuperAdminExists(t *testing.T) {
	t.Setenv("ARIA_SUPER_ADMIN", "ops-admin")
	t.Setenv("ARIA_SUPER_ADMIN_PASSWORD", "OpsSecret@123")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`)).
		WithArgs("ops-admin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM users WHERE role = 'super_admin'")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO users (username, password_hash, role, tenant_id, must_change_password) VALUES ($1, $2, 'super_admin', NULL, TRUE)`)).
		WithArgs("ops-admin", bcryptHashForPassword{password: "OpsSecret@123"}).
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
