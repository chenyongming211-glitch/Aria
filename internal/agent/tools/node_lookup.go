package tools

import "aria/pkg/controllerstorage"

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

func findUniqueNodeByHostname(store *controllerstorage.Storage, hostname string) (*controllerstorage.Node, int, error) {
	nodes, err := store.GetAllNodes()
	if err != nil {
		return nil, 0, err
	}
	node, count := findUniqueNodeByHostnameInNodes(nodes, hostname)
	return node, count, nil
}
