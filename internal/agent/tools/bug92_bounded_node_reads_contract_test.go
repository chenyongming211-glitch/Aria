package tools

import (
	"os"
	"strings"
	"testing"
)

func TestBug92AIToolsDoNotUseUnboundedAllNodeReads(t *testing.T) {
	testCases := []struct {
		name      string
		file      string
		signature string
		required  []string
	}{
		{
			name:      "list nodes",
			file:      "tools.go",
			signature: "func NewListNodesToolWithStore",
			required:  []string{"listNodesForToolScope("},
		},
		{
			name:      "node detail",
			file:      "tools.go",
			signature: "func NewGetNodeDetailTool",
			required:  []string{"findUniqueNodeByHostnameForScope("},
		},
		{
			name:      "token detail used-by nodes",
			file:      "token_management.go",
			signature: "func NewGetTokenDetailTool",
			required:  []string{"listNodesForEnrollmentToken("},
		},
		{
			name:      "get node routes",
			file:      "route_management.go",
			signature: "func NewGetNodeRoutesTool",
			required:  []string{"findUniqueNodeByHostnameForScope("},
		},
		{
			name:      "list all routes",
			file:      "route_management.go",
			signature: "func NewListAllRoutesTool",
			required:  []string{"listNodesForToolScope("},
		},
		{
			name:      "add route",
			file:      "route_management.go",
			signature: "func NewAddRouteTool",
			required: []string{
				"findUniqueNodeByHostnameForScope(",
				"FindTenantAdvertisedRouteConflict(",
			},
		},
		{
			name:      "remove route",
			file:      "route_management.go",
			signature: "func NewRemoveRouteTool",
			required:  []string{"findUniqueNodeByHostnameForScope("},
		},
		{
			name:      "monitor stats",
			file:      "monitoring.go",
			signature: "func NewGetMonitorStatsTool",
			required:  []string{"listNodesForToolScope("},
		},
		{
			name:      "diagnose connectivity",
			file:      "diagnostics.go",
			signature: "func NewDiagnoseConnectivityTool",
			required:  []string{"listNodesForToolScope("},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("failed to read %s: %v", tc.file, err)
			}
			body := goToolFunctionBody(t, string(sourceBytes), tc.signature)
			if strings.Contains(body, "GetAllNodes()") {
				t.Fatalf("%s must not call unbounded GetAllNodes", tc.signature)
			}
			for _, needle := range tc.required {
				if !strings.Contains(body, needle) {
					t.Fatalf("%s must use bounded helper %q", tc.signature, needle)
				}
			}
		})
	}
}

func goToolFunctionBody(t *testing.T, source, signature string) string {
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
