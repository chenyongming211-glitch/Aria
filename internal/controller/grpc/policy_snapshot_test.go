package grpc

import (
	"testing"

	"aria/pkg/grpc/agentpb"
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
			rule.GetPorts() != "443" {
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
			rule.GetPorts() != "53" {
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
		},
		nil,
	)
	if err != nil {
		t.Fatalf("compileAgentPolicySnapshot failed: %v", err)
	}
	if len(snapshot.QoSRules) != 2 {
		t.Fatalf("expected 2 QoS rules, got %#v", snapshot.QoSRules)
	}

	egress := snapshot.QoSRules[0]
	if egress.GetDirection() != "egress" ||
		egress.GetDstIp() != "10.10.0.0/24" ||
		egress.GetSrcIp() != "" ||
		egress.GetRateBps() != 100_000_000 ||
		egress.GetBurstBytes() != 1_250_000 ||
		egress.GetPriority() != 7 ||
		egress.GetMode() != "policing" {
		t.Fatalf("unexpected egress QoS rule: %#v", egress)
	}

	ingress := snapshot.QoSRules[1]
	if ingress.GetDirection() != "ingress" ||
		ingress.GetSrcIp() != "172.16.0.0/16" ||
		ingress.GetDstIp() != "" ||
		ingress.GetRateBps() != 250_000_000 ||
		ingress.GetBurstBytes() != 4_000_000 ||
		ingress.GetMode() != "policing" {
		t.Fatalf("unexpected ingress QoS rule: %#v", ingress)
	}
}
