package controllerstorage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readStorageSource(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(filename), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}

func TestB7SchemaHasScaleIndexesAndDeadlineColumn(t *testing.T) {
	source := readStorageSource(t, "postgres.go")

	for _, required := range []string{
		"CREATE INDEX IF NOT EXISTS idx_acl_rules_tenant_node ON acl_rules(tenant_id, node_id)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_hostname ON nodes(hostname)",
		"deadline_at TIMESTAMPTZ",
		"CREATE INDEX IF NOT EXISTS idx_agent_commands_node_status_deadline ON agent_commands(node_public_key, status, deadline_at)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration is missing scale guardrail %q", required)
		}
	}
}

func TestB7NodeQueriesUseTypedPaginationOptions(t *testing.T) {
	source := readStorageSource(t, "postgres.go")

	if strings.Contains(source, "func (s *Storage) getNodes(extraWhere string") {
		t.Fatal("node list queries must not accept raw SQL where/order fragments")
	}
	for _, required := range []string{
		"type NodeListOptions struct",
		"func (s *Storage) GetNodesByTenantPage",
		"LIMIT $",
		"OFFSET $",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("node list implementation is missing %q", required)
		}
	}
}

func TestB7AgentCommandTimeoutQueriesUseIndexedDeadline(t *testing.T) {
	source := readStorageSource(t, "agent_commands.go")

	if strings.Contains(source, "timeout_seconds * interval") {
		t.Fatal("agent command timeout sweeps must not compute timeout expressions in WHERE clauses")
	}
	if strings.Contains(source, "COALESCE(acknowledged_at, sent_at, created_at) +") {
		t.Fatal("agent command timeout sweeps must use deadline_at instead of timestamp expressions")
	}
	if !strings.Contains(source, "deadline_at") {
		t.Fatal("agent command timeout code must maintain deadline_at")
	}
}
