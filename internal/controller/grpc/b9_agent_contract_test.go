package grpc

import (
	"os"
	"strings"
	"testing"
)

func TestAgentCleanupRemovesAllPinnedAclQosMaps(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/agent_runtime.rs")
	if err != nil {
		t.Fatalf("failed to read agent runtime source: %v", err)
	}
	source := string(sourceBytes)

	cleanupBody := rustFunctionBody(t, source, "async fn cleanup(&self)")
	if !strings.Contains(cleanupBody, "Self::cleanup_pinned_acl_qos_maps();") {
		t.Fatalf("agent shutdown cleanup must reuse the full startup pinned ACL/QoS map cleanup helper")
	}
}

func TestAgentConfigYamlWritesAreAtomic(t *testing.T) {
	sourceBytes, err := os.ReadFile("../../../agent-rust/agent/src/config.rs")
	if err != nil {
		t.Fatalf("failed to read agent config source: %v", err)
	}
	source := string(sourceBytes)

	writeBody := rustFunctionBody(t, source, "fn write_yaml_file<T>")
	for _, required := range []string{
		"tmp_path",
		"OpenOptions::new()",
		"file.sync_all()",
		"std::fs::rename(&tmp_path, path)",
	} {
		if !strings.Contains(writeBody, required) {
			t.Fatalf("write_yaml_file must write YAML atomically with temp file fsync and rename; missing %q", required)
		}
	}
	if strings.Contains(writeBody, "std::fs::write(path") {
		t.Fatalf("write_yaml_file must not write directly to the target path")
	}
}

func rustFunctionBody(t *testing.T, source, signature string) string {
	t.Helper()

	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("Rust function %q not found", signature)
	}

	open := strings.Index(source[start:], "{")
	if open < 0 {
		t.Fatalf("Rust function %q has no body", signature)
	}
	bodyStart := start + open

	depth := 0
	for idx := bodyStart; idx < len(source); idx++ {
		switch source[idx] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[bodyStart : idx+1]
			}
		}
	}

	t.Fatalf("Rust function %q body is not balanced", signature)
	return ""
}
