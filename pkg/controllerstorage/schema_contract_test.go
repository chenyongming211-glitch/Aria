package controllerstorage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrationSchemaMatchesTenantAndUserHandlers(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "postgres.go")
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read postgres.go: %v", err)
	}
	source := string(content)

	for _, required := range []string{
		"ALTER TABLE tenants ADD COLUMN IF NOT EXISTS email",
		"ALTER TABLE tenants ADD COLUMN IF NOT EXISTS phone",
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_unique ON users(username)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration is missing %q", required)
		}
	}
}
