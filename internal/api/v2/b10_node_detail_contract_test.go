package v2

import (
	"os"
	"strings"
	"testing"
)

func TestMonitoringNodeDetailUsesBundledStateLookup(t *testing.T) {
	sourceBytes, err := os.ReadFile("monitoring.go")
	if err != nil {
		t.Fatalf("failed to read monitoring source: %v", err)
	}

	body := goFunctionBody(t, string(sourceBytes), "func (r *Router) handleMonitoringNodeDetail")
	if !strings.Contains(body, "GetNodeMonitoringDetailState(") {
		t.Fatalf("node monitoring detail must load control state, policy stats, and certificate through one bundled state query")
	}
	if !strings.Contains(body, "ListRecentNodeAlerts(") {
		t.Fatalf("node monitoring detail must fetch recent node alerts without the paginated alert count query")
	}

	for _, fragmentedLookup := range []string{
		"GetNodeControlState(",
		"GetNodePolicyStats(",
		"GetNodeCertificate(",
		"ListAlerts(",
	} {
		if strings.Contains(body, fragmentedLookup) {
			t.Fatalf("node monitoring detail must not use fragmented state lookup %q", fragmentedLookup)
		}
	}
}

func goFunctionBody(t *testing.T, source, signature string) string {
	t.Helper()

	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("Go function %q not found", signature)
	}

	open := strings.Index(source[start:], "{")
	if open < 0 {
		t.Fatalf("Go function %q has no body", signature)
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

	t.Fatalf("Go function %q body is not balanced", signature)
	return ""
}
