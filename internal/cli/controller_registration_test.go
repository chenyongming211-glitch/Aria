package cli

import (
	"testing"

	"aria/pkg/controllerstorage"
)

func TestNodeRequiresFreshEnrollment(t *testing.T) {
	if nodeRequiresFreshEnrollment(nil) {
		t.Fatal("nil node should not require fresh enrollment")
	}
	if !nodeRequiresFreshEnrollment(&controllerstorage.Node{Status: "deleted"}) {
		t.Fatal("deleted node should require fresh enrollment")
	}
	if !nodeRequiresFreshEnrollment(&controllerstorage.Node{Status: " DELETED "}) {
		t.Fatal("deleted status should be normalized")
	}
	if nodeRequiresFreshEnrollment(&controllerstorage.Node{Status: "offline"}) {
		t.Fatal("offline node should not require fresh enrollment")
	}
}

func TestNodeRegistrationForbidden(t *testing.T) {
	if nodeRegistrationForbidden(nil) {
		t.Fatal("nil node should not be forbidden")
	}
	for _, status := range []string{"suspended", "banned", " SUSPENDED "} {
		if !nodeRegistrationForbidden(&controllerstorage.Node{Status: status}) {
			t.Fatalf("status %q should be forbidden", status)
		}
	}
	if nodeRegistrationForbidden(&controllerstorage.Node{Status: "deleted"}) {
		t.Fatal("deleted node should be re-enrollable with a fresh token, not permanently forbidden")
	}
}
