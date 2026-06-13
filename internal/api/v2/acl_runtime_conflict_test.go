package v2

import (
	"testing"

	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func TestACLRuntimeKeyConflictRejectsPortOnlyVariants(t *testing.T) {
	existingID := uuid.New()
	existing := []*controllerstorage.ACLRuleRecord{{
		ID:        existingID,
		SrcCIDR:   "100.64.0.2/32",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  6,
		Direction: "egress",
		Ports:     "80:1",
		Enabled:   true,
	}}
	candidate := &controllerstorage.ACLRuleRecord{
		ID:        uuid.New(),
		SrcCIDR:   "100.64.0.2/32",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  6,
		Direction: "egress",
		Ports:     "443:1",
		Enabled:   true,
	}

	if err := validateACLRuntimeKeyAvailable(existing, candidate, uuid.Nil); err == nil {
		t.Fatal("expected ACL runtime key conflict")
	}
}

func TestACLRuntimeKeyConflictAllowsDifferentDirections(t *testing.T) {
	existing := []*controllerstorage.ACLRuleRecord{{
		ID:        uuid.New(),
		SrcCIDR:   "100.64.0.2/32",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  1,
		Direction: "ingress",
		Enabled:   true,
	}}
	candidate := &controllerstorage.ACLRuleRecord{
		ID:        uuid.New(),
		SrcCIDR:   "100.64.0.2/32",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  1,
		Direction: "egress",
		Enabled:   true,
	}

	if err := validateACLRuntimeKeyAvailable(existing, candidate, uuid.Nil); err != nil {
		t.Fatalf("expected different directions to be allowed, got %v", err)
	}
}

func TestACLRuntimeKeyConflictExpandsBothDirections(t *testing.T) {
	existing := []*controllerstorage.ACLRuleRecord{{
		ID:        uuid.New(),
		SrcCIDR:   "any",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  1,
		Direction: "egress",
		Enabled:   true,
	}}
	candidate := &controllerstorage.ACLRuleRecord{
		ID:        uuid.New(),
		SrcCIDR:   "",
		DstCIDR:   "100.64.0.3/32",
		Protocol:  1,
		Direction: "both",
		Enabled:   true,
	}

	if err := validateACLRuntimeKeyAvailable(existing, candidate, uuid.Nil); err == nil {
		t.Fatal("expected both direction candidate to conflict with egress rule")
	}
}

func TestACLRuntimeKeyTreatsAllPortsAsNoPortFilter(t *testing.T) {
	rule := &controllerstorage.ACLRuleRecord{
		SrcCIDR:   "any",
		DstCIDR:   "any",
		Protocol:  0,
		Direction: "ingress",
		Ports:     "all",
		Enabled:   true,
	}

	keys := aclRuntimeKeysForRule(rule)
	if len(keys) != 1 {
		t.Fatalf("expected one any-protocol key for ports=all, got %#v", keys)
	}
	if keys[0].proto != 0 {
		t.Fatalf("expected ports=all to keep protocol 0, got %#v", keys[0])
	}
}
