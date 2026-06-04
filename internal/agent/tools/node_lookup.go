package tools

import (
	"fmt"
	"strings"

	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func findUniqueNodeByHostnameInNodes(nodes []*controllerstorage.Node, hostname string) (*controllerstorage.Node, int) {
	var matched *controllerstorage.Node
	count := 0
	for _, node := range nodes {
		if node != nil && node.Hostname == hostname {
			count++
			if matched == nil {
				matched = node
			}
		}
	}
	return matched, count
}

func findUniqueNodeByHostnameInNodesForTenant(nodes []*controllerstorage.Node, hostname string, tenantID uuid.UUID) (*controllerstorage.Node, int) {
	var matched *controllerstorage.Node
	count := 0
	for _, node := range nodes {
		if node != nil && node.Hostname == hostname && node.TenantID == tenantID {
			count++
			if matched == nil {
				matched = node
			}
		}
	}
	return matched, count
}

func findUniqueNodeByHostnameInNodesForScope(nodes []*controllerstorage.Node, hostname string, tenantID uuid.UUID, tenantScoped bool) (*controllerstorage.Node, int) {
	if tenantScoped {
		return findUniqueNodeByHostnameInNodesForTenant(nodes, hostname, tenantID)
	}
	return findUniqueNodeByHostnameInNodes(nodes, hostname)
}

func filterNodesByTenant(nodes []*controllerstorage.Node, tenantID uuid.UUID, tenantScoped bool) []*controllerstorage.Node {
	if !tenantScoped {
		return nodes
	}

	filtered := make([]*controllerstorage.Node, 0, len(nodes))
	for _, node := range nodes {
		if node != nil && node.TenantID == tenantID {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func parseOptionalTenantID(req map[string]interface{}) (uuid.UUID, bool, error) {
	raw, ok := req["tenant_id"]
	if !ok || raw == nil {
		return uuid.Nil, false, nil
	}

	text, ok := raw.(string)
	if !ok {
		return uuid.Nil, false, fmt.Errorf("tenant_id must be a string")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return uuid.Nil, false, nil
	}

	tenantID, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("invalid tenant_id: %w", err)
	}
	return tenantID, true, nil
}

func findUniqueNodeByHostnameForScope(store *controllerstorage.Storage, hostname string, tenantID uuid.UUID, tenantScoped bool) (*controllerstorage.Node, int, error) {
	nodes, err := store.GetAllNodes()
	if err != nil {
		return nil, 0, err
	}
	node, count := findUniqueNodeByHostnameInNodesForScope(nodes, hostname, tenantID, tenantScoped)
	return node, count, nil
}
