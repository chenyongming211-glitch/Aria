package controllerstorage

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
)

type recordingRoleExec struct {
	calls [][]interface{}
}

func (e *recordingRoleExec) Exec(_ string, args ...interface{}) (sql.Result, error) {
	e.calls = append(e.calls, args)
	return roleExecResult{}, nil
}

type roleExecResult struct{}

func (roleExecResult) LastInsertId() (int64, error) { return 0, nil }
func (roleExecResult) RowsAffected() (int64, error) { return 1, nil }

func TestCreateSystemRolesUsesProvidedExecutor(t *testing.T) {
	tenantID := uuid.New()
	exec := &recordingRoleExec{}
	store := &Storage{}

	if err := store.createSystemRoles(exec, tenantID); err != nil {
		t.Fatalf("createSystemRoles returned error: %v", err)
	}

	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 role inserts through provided executor, got %d", len(exec.calls))
	}
	for _, call := range exec.calls {
		if got := call[0]; got != tenantID {
			t.Fatalf("expected tenant id %s, got %v", tenantID, got)
		}
	}
}
