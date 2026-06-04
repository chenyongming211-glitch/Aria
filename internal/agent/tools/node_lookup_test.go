package tools

import (
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func TestFindUniqueNodeByHostnameInNodesForTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	nodes := []*controllerstorage.Node{
		{Hostname: "edge-1", TenantID: tenantA, PublicKey: "node-a"},
		{Hostname: "edge-1", TenantID: tenantB, PublicKey: "node-b"},
	}

	_, globalCount := findUniqueNodeByHostnameInNodes(nodes, "edge-1")
	if globalCount != 2 {
		t.Fatalf("expected global lookup to remain ambiguous, got count=%d", globalCount)
	}

	matched, tenantCount := findUniqueNodeByHostnameInNodesForTenant(nodes, "edge-1", tenantB)
	if tenantCount != 1 {
		t.Fatalf("expected tenant-scoped lookup to return one match, got count=%d", tenantCount)
	}
	if matched == nil || matched.PublicKey != "node-b" {
		t.Fatalf("expected tenant B node, got %#v", matched)
	}
}

func TestParseOptionalTenantID(t *testing.T) {
	tenantID := uuid.New()

	parsed, ok, err := parseOptionalTenantID(map[string]interface{}{"tenant_id": tenantID.String()})
	if err != nil {
		t.Fatalf("unexpected error parsing tenant_id: %v", err)
	}
	if !ok || parsed != tenantID {
		t.Fatalf("expected parsed tenant_id %s, got %s ok=%v", tenantID, parsed, ok)
	}

	if _, ok, err := parseOptionalTenantID(map[string]interface{}{}); err != nil || ok {
		t.Fatalf("expected missing tenant_id to be optional, ok=%v err=%v", ok, err)
	}

	if _, _, err := parseOptionalTenantID(map[string]interface{}{"tenant_id": "not-a-uuid"}); err == nil {
		t.Fatal("expected invalid tenant_id to fail")
	}
}
