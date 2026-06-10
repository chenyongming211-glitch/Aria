package controllerstorage

import (
	"testing"

	"github.com/google/uuid"
)

func TestNodePolicyStatsReturnsNestedRuleStats(t *testing.T) {
	aclID := uuid.New()
	qosID := uuid.New()
	stats := &NodePolicyStats{
		Stats: map[string]interface{}{
			"acl_rules": map[string]interface{}{
				aclID.String(): map[string]interface{}{
					"packets": float64(42),
				},
			},
			"qos_rules": map[string]interface{}{
				qosID.String(): map[string]interface{}{
					"passed_bytes": float64(4096),
				},
			},
		},
	}

	if got := stats.ACLRuleStats(aclID); got["packets"] != float64(42) {
		t.Fatalf("unexpected ACL stats: %#v", got)
	}
	if got := stats.QoSRuleStats(qosID); got["passed_bytes"] != float64(4096) {
		t.Fatalf("unexpected QoS stats: %#v", got)
	}
	if got := stats.ACLRuleStats(uuid.New()); got != nil {
		t.Fatalf("expected missing ACL stats to be nil, got %#v", got)
	}
}
