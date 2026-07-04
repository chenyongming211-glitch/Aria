package delivery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot(t), path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestBuildWorkflowRequiresArtifactsAndFullPublishGate(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/build.yml")

	if strings.Contains(workflow, "continue-on-error: true") {
		t.Fatal("required artifact uploads must not continue on error")
	}

	if !strings.Contains(workflow, "needs: [go-build, rust-agent-build, frontend-build]") {
		t.Fatal("Docker publish must wait for Go, Rust Agent, and frontend jobs")
	}

	expectedGate := "github.event_name == 'workflow_dispatch' && (github.ref == 'refs/heads/master' || startsWith(github.ref, 'refs/tags/v'))"
	if !strings.Contains(workflow, expectedGate) {
		t.Fatal("Docker publish must only run for manual master or release-tag workflows")
	}
}

func TestControllerAnsibleDoesNotPinStaleImageVersion(t *testing.T) {
	playbook := readRepoFile(t, "deployments/ansible/playbooks/deploy-controller.yml")
	groupVars := readRepoFile(t, "deployments/ansible/group_vars/all.yml")
	combined := playbook + "\n" + groupVars

	if strings.Contains(combined, "0.2.35-test") {
		t.Fatal("Controller Ansible defaults must not pin the stale 0.2.35-test image")
	}

	if !strings.Contains(combined, "aria_controller_version") {
		t.Fatal("Controller Ansible must expose an explicit controller version variable")
	}

	if !strings.Contains(combined, "../../../VERSION") {
		t.Fatal("Controller Ansible must read the repository VERSION when no version override is provided")
	}
}

func TestAgentAnsibleUsesCurrentBinaryAndServiceNames(t *testing.T) {
	playbook := readRepoFile(t, "deployments/ansible/playbooks/deploy-agent.yml")

	for _, stale := range []string{
		"/Users/chen/Aria/agent-rust",
		"agent_binary_path: /usr/local/bin/aria\n",
		"name: aria\n",
	} {
		if strings.Contains(playbook, stale) {
			t.Fatalf("Agent Ansible still contains stale deployment target %q", stale)
		}
	}

	for _, expected := range []string{
		"aria_agent_binary_src",
		"aria_agent_binary_path: /usr/local/bin/aria-agent",
		"aria_agent_service_name: aria-agent",
	} {
		if !strings.Contains(playbook, expected) {
			t.Fatalf("Agent Ansible must contain %q", expected)
		}
	}
}
