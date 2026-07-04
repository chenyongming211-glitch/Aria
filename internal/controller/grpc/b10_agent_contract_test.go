package grpc

import (
	"os"
	"strings"
	"testing"
)

func TestAgentSyncPeersBuildsPlansBeforeBlockingApply(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}

	body := rustFunctionBody(t, string(sourceBytes), "async fn sync_peers(&mut self")
	if strings.Contains(body, "let new_peers = new_peers.to_vec();") {
		t.Fatalf("sync_peers must not clone the full desired peer slice just to cross the blocking boundary")
	}
	if !strings.Contains(body, "Self::build_peer_sync_plans(&snapshots, new_peers") {
		t.Fatalf("sync_peers must build peer plans while borrowing the desired peer slice")
	}
	if strings.Count(body, "tokio::task::spawn_blocking") < 2 {
		t.Fatalf("sync_peers must keep blocking WireGuard snapshot/apply operations off the async worker without moving desired peers into a single blocking closure")
	}
}
