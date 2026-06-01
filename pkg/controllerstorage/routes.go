package controllerstorage

import (
	"fmt"
	"net"
	"strings"
)

// RouteConflictError describes an advertised-route overlap with another node.
type RouteConflictError struct {
	CIDR            string
	ExistingCIDR    string
	NodeHostname    string
	NodeRegion      string
	NodePublicKey   string
	TargetRegion    string
	TargetPublicKey string
}

func (e *RouteConflictError) Error() string {
	return fmt.Sprintf("route %s conflicts with route %s advertised by node %s in region %s",
		e.CIDR, e.ExistingCIDR, e.NodeHostname, e.NodeRegion)
}

// FindAdvertisedRouteConflict returns the first cross-region route overlap.
// Same-region overlap is allowed for active-active redundancy.
func FindAdvertisedRouteConflict(nodes []*Node, targetPublicKey, targetRegion string, candidateRoutes []string) error {
	targetPublicKey = strings.TrimSpace(targetPublicKey)
	targetRegion = strings.TrimSpace(targetRegion)

	parsedCandidates := make([]struct {
		raw string
		net *net.IPNet
	}, 0, len(candidateRoutes))
	for _, route := range candidateRoutes {
		trimmed := strings.TrimSpace(route)
		if trimmed == "" {
			continue
		}
		_, network, err := net.ParseCIDR(trimmed)
		if err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", trimmed, err)
		}
		parsedCandidates = append(parsedCandidates, struct {
			raw string
			net *net.IPNet
		}{raw: trimmed, net: network})
	}

	for _, node := range nodes {
		if node == nil {
			continue
		}
		if strings.TrimSpace(node.PublicKey) == targetPublicKey {
			continue
		}
		if strings.TrimSpace(node.Region) == targetRegion {
			continue
		}
		if !routeConflictNodeActive(node) {
			continue
		}

		for _, existingRoute := range node.AdvertisedRoutes {
			existingRoute = strings.TrimSpace(existingRoute)
			if existingRoute == "" {
				continue
			}
			_, existingNetwork, err := net.ParseCIDR(existingRoute)
			if err != nil {
				continue
			}
			for _, candidate := range parsedCandidates {
				if cidrNetworksOverlap(candidate.net, existingNetwork) {
					return &RouteConflictError{
						CIDR:            candidate.raw,
						ExistingCIDR:    existingRoute,
						NodeHostname:    firstRouteNodeName(node),
						NodeRegion:      node.Region,
						NodePublicKey:   node.PublicKey,
						TargetRegion:    targetRegion,
						TargetPublicKey: targetPublicKey,
					}
				}
			}
		}
	}

	return nil
}

func routeConflictNodeActive(node *Node) bool {
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "deleted", "suspended", "banned":
		return false
	default:
		return true
	}
}

func cidrNetworksOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func firstRouteNodeName(node *Node) string {
	for _, candidate := range []string{node.Hostname, node.PublicKey, node.ID.String()} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return "unknown"
}
