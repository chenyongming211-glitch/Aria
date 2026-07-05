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

func TestAgentInitialSyncFailurePersistsRuntimeState(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}
	source := string(sourceBytes)

	initialSyncBranch := sourceBetween(t, source, "// Step 4:", "self.start_unix_socket_server()?")
	if !strings.Contains(initialSyncBranch, "self.sync().await") {
		t.Fatalf("initial sync branch must run sync before starting runtime services")
	}
	if !strings.Contains(initialSyncBranch, "self.persist_runtime_state()") {
		t.Fatalf("initial sync failure branch must persist runtime state after sync records a rotated runtime token")
	}
}

func TestAgentRemoteCommandSyncFailurePersistsRuntimeState(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}
	source := string(sourceBytes)

	commandBody := rustFunctionBody(t, source, "async fn execute_remote_command")
	syncCommandBranch := sourceBetween(t, commandBody, `"sync" => match self.sync().await`, `"config_reload" =>`)
	syncFailureBranch := sourceFrom(t, syncCommandBranch, "Err(e) =>")
	if !strings.Contains(syncFailureBranch, "self.persist_runtime_state()") {
		t.Fatalf("remote command sync failure branch must persist runtime state after sync records a rotated runtime token")
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

func sourceBetween(t *testing.T, source, startMarker, endMarker string) string {
	t.Helper()

	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("source start marker %q not found", startMarker)
	}
	end := strings.Index(source[start:], endMarker)
	if end < 0 {
		t.Fatalf("source end marker %q not found after %q", endMarker, startMarker)
	}
	return source[start : start+end]
}

func sourceFrom(t *testing.T, source, startMarker string) string {
	t.Helper()

	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("source start marker %q not found", startMarker)
	}
	return source[start:]
}
