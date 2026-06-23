package grpc

import (
	"testing"
	"time"

	controllerstorage "aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"

	"github.com/google/uuid"
)

func TestCompilePolicySnapshotExpandsPortAclAndBlacklist(t *testing.T) {
	legacyACL := []map[string]interface{}{
		{
			"src_net":   "",
			"dst_net":   "10.0.0.0/24",
			"protocol":  0,
			"min_port":  443,
			"max_port":  443,
			"action":    "drop",
			"direction": "all",
			"priority":  float64(200),
		},
	}

	snapshot, err := compileAgentPolicySnapshot(
		legacyACL,
		nil,
		[]*agentpb.BlacklistRule{{Scope: "ports", Port: 53}},
	)
	if err != nil {
		t.Fatalf("compileAgentPolicySnapshot failed: %v", err)
	}

	if len(snapshot.BlacklistRules) != 0 {
		t.Fatalf("expected blacklist rules folded into ACLs, got %#v", snapshot.BlacklistRules)
	}
	if len(snapshot.ACLRules) != 4 {
		t.Fatalf("expected 4 compiled ACL rules, got %#v", snapshot.ACLRules)
	}

	for i, proto := range []uint32{6, 17} {
		rule := snapshot.ACLRules[i]
		if rule.GetSrcNet() != "any" ||
			rule.GetDstNet() != "10.0.0.0/24" ||
			rule.GetProtocol() != proto ||
			rule.GetAction() != "deny" ||
			rule.GetDirection() != "both" ||
			rule.GetPorts() != "443" ||
			rule.GetPriority() != 200 {
			t.Fatalf("unexpected expanded ACL rule %d: %#v", i, rule)
		}
	}

	for i, proto := range []uint32{6, 17} {
		rule := snapshot.ACLRules[i+2]
		if rule.GetSrcNet() != "any" ||
			rule.GetDstNet() != "any" ||
			rule.GetProtocol() != proto ||
			rule.GetAction() != "deny" ||
			rule.GetDirection() != "ingress" ||
			rule.GetPorts() != "53" ||
			rule.GetPriority() != 0 {
			t.Fatalf("unexpected blacklist-derived ACL rule %d: %#v", i, rule)
		}
	}
}

func TestCompilePolicySnapshotNormalizesQoSForAgentGroups(t *testing.T) {
	snapshot, err := compileAgentPolicySnapshot(
		nil,
		[]*agentpb.QoSRule{
			{
				DstIp:         "10.10.0.0/24",
				BandwidthMbps: 100,
				Priority:      7,
			},
			{
				SrcIp:      "172.16.0.0/16",
				Direction:  "in",
				RateBps:    250_000_000,
				BurstBytes: 4_000_000,
				Mode:       "shaping",
			},
			{
				DstIp:         "100.64.0.2/32",
				BandwidthMbps: 10,
				Mode:          "auto",
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("compileAgentPolicySnapshot failed: %v", err)
	}
	if len(snapshot.QoSRules) != 3 {
		t.Fatalf("expected 3 QoS rules, got %#v", snapshot.QoSRules)
	}

	egress := snapshot.QoSRules[0]
	if egress.GetDirection() != "egress" ||
		egress.GetDstIp() != "10.10.0.0/24" ||
		egress.GetSrcIp() != "" ||
		egress.GetRateBps() != 100_000_000 ||
		egress.GetBurstBytes() != 1_250_000 ||
		egress.GetPriority() != 7 ||
		egress.GetMode() != "auto" {
		t.Fatalf("unexpected egress QoS rule: %#v", egress)
	}

	ingress := snapshot.QoSRules[1]
	if ingress.GetDirection() != "ingress" ||
		ingress.GetSrcIp() != "172.16.0.0/16" ||
		ingress.GetDstIp() != "" ||
		ingress.GetRateBps() != 250_000_000 ||
		ingress.GetBurstBytes() != 4_000_000 ||
		ingress.GetMode() != "shaping" {
		t.Fatalf("unexpected ingress QoS rule: %#v", ingress)
	}

	auto := snapshot.QoSRules[2]
	if auto.GetDirection() != "egress" ||
		auto.GetDstIp() != "100.64.0.2/32" ||
		auto.GetMode() != "auto" {
		t.Fatalf("unexpected auto QoS rule: %#v", auto)
	}
}

func TestCompilePolicySnapshotIncludesIPGroups(t *testing.T) {
	officeID := "11111111-1111-1111-1111-111111111111"
	prodID := "22222222-2222-2222-2222-222222222222"
	now := time.Now()

	snapshot, err := compileAgentPolicySnapshotWithGroups(
		[]*controllerstorage.IPGroupRecord{
			{
				ID:   uuid.MustParse(officeID),
				Name: "office",
				Kind: controllerstorage.IPGroupKindCustom,
				Members: []controllerstorage.IPGroupMemberRecord{
					{CIDR: "10.10.0.0/16"},
					{CIDR: "2001:db8:10::/48"},
				},
			},
			{
				ID:      uuid.MustParse(prodID),
				Name:    "prod",
				Kind:    controllerstorage.IPGroupKindCustom,
				Members: []controllerstorage.IPGroupMemberRecord{{CIDR: "172.16.0.0/16"}},
			},
		},
		[]*controllerstorage.ACLRuleRecord{
			{
				ID:         uuid.New(),
				SrcGroupID: uuid.NullUUID{UUID: uuid.MustParse(officeID), Valid: true},
				DstGroupID: uuid.NullUUID{UUID: uuid.MustParse(prodID), Valid: true},
				Protocol:   6,
				Action:     "deny",
				Direction:  "egress",
				Priority:   100,
				Enabled:    true,
				CreatedAt:  now,
			},
		},
		[]*controllerstorage.QoSRuleRecord{
			{
				ID:            uuid.New(),
				GroupID:       uuid.NullUUID{UUID: uuid.MustParse(officeID), Valid: true},
				Direction:     "egress",
				BandwidthMbps: 10,
				RateBps:       10_000_000,
				BurstBytes:    1500,
				Priority:      100,
				Mode:          "policing",
				Enabled:       true,
				CreatedAt:     now,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("compileAgentPolicySnapshotWithGroups failed: %v", err)
	}
	if len(snapshot.IPGroups) != 2 {
		t.Fatalf("expected 2 IP groups, got %#v", snapshot.IPGroups)
	}
	if snapshot.IPGroups[0].GetId() != officeID || len(snapshot.IPGroups[0].GetCidrs()) != 2 {
		t.Fatalf("office group was not compiled with members: %#v", snapshot.IPGroups[0])
	}
	if snapshot.ACLRules[0].GetSrcGroupId() != officeID ||
		snapshot.ACLRules[0].GetDstGroupId() != prodID ||
		snapshot.ACLRules[0].GetPriority() != 100 {
		t.Fatalf("ACL group ids not preserved: %#v", snapshot.ACLRules[0])
	}
	if snapshot.QoSRules[0].GetGroupId() != officeID {
		t.Fatalf("QoS group id not preserved: %#v", snapshot.QoSRules[0])
	}
}
