package grpc

import (
	"os"
	"strings"
	"testing"
)

func TestAgentImmediateSyncFailurePersistsRuntimeState(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}
	source := string(sourceBytes)

	start := strings.Index(source, "_ = self.sync_now.notified()")
	if start < 0 {
		t.Fatalf("immediate sync branch not found")
	}
	end := strings.Index(source[start:], "_ = metrics_timer.tick()")
	if end < 0 {
		t.Fatalf("metrics timer branch not found after immediate sync branch")
	}
	immediateSyncBranch := source[start : start+end]
	if !strings.Contains(immediateSyncBranch, "self.persist_runtime_state()") {
		t.Fatalf("immediate sync failure branch must persist runtime state after recording sync error")
	}
}

func TestAgentCommandStreamHasReconnectResponseOutbox(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}
	source := string(sourceBytes)

	for _, required := range []string{
		"pending_command_responses",
		"flush_pending_command_responses",
		"store_pending_command_response",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("agent command stream must include reconnect outbox helper %q", required)
		}
	}
}
