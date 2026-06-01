package controllerstorage

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestFindAdvertisedRouteConflictRejectsCrossRegionOverlap(t *testing.T) {
	targetPublicKey := "target-key"
	nodes := []*Node{
		{
			ID:               uuid.New(),
			PublicKey:        "other-key",
			Hostname:         "edge-b",
			Region:           "bj",
			Status:           "online",
			AdvertisedRoutes: []string{"10.10.0.0/16"},
		},
	}

	err := FindAdvertisedRouteConflict(nodes, targetPublicKey, "sh", []string{"10.10.1.0/24"})
	if err == nil {
		t.Fatal("expected route conflict")
	}
	var conflict *RouteConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected RouteConflictError, got %T: %v", err, err)
	}
	if conflict.NodeHostname != "edge-b" || conflict.ExistingCIDR != "10.10.0.0/16" {
		t.Fatalf("unexpected conflict details: %#v", conflict)
	}
}

func TestFindAdvertisedRouteConflictAllowsSameRegionOverlap(t *testing.T) {
	nodes := []*Node{
		{
			ID:               uuid.New(),
			PublicKey:        "other-key",
			Hostname:         "edge-b",
			Region:           "sh",
			Status:           "online",
			AdvertisedRoutes: []string{"10.10.0.0/16"},
		},
	}

	if err := FindAdvertisedRouteConflict(nodes, "target-key", "sh", []string{"10.10.1.0/24"}); err != nil {
		t.Fatalf("expected same-region overlap to be allowed, got %v", err)
	}
}

func TestFindAdvertisedRouteConflictSkipsInactiveNodes(t *testing.T) {
	nodes := []*Node{
		{
			ID:               uuid.New(),
			PublicKey:        "deleted-key",
			Hostname:         "deleted-node",
			Region:           "bj",
			Status:           "deleted",
			AdvertisedRoutes: []string{"10.10.0.0/16"},
		},
	}

	if err := FindAdvertisedRouteConflict(nodes, "target-key", "sh", []string{"10.10.1.0/24"}); err != nil {
		t.Fatalf("expected inactive node to be ignored, got %v", err)
	}
}
