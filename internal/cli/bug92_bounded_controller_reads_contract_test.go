package cli

import (
	"os"
	"strings"
	"testing"
)

func TestBug92ControllerServerAPIsDoNotUseUnboundedTenantNodeReads(t *testing.T) {
	testCases := []struct {
		name      string
		signature string
		forbidden []string
		required  []string
	}{
		{
			name:      "network manage",
			signature: "func (c *Controller) HandleNetworkManage",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"ListTenantNodesByHostname(",
				"FindTenantAdvertisedRouteConflict(",
			},
		},
		{
			name:      "advertised route validation",
			signature: "func (c *Controller) validateAdvertisedRouteConflicts",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"FindTenantAdvertisedRouteConflict(",
			},
		},
	}

	sourceBytes, err := os.ReadFile("controller_serve.go")
	if err != nil {
		t.Fatalf("failed to read controller_serve.go: %v", err)
	}
	source := string(sourceBytes)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := goControllerFunctionBody(t, source, tc.signature)
			for _, needle := range tc.forbidden {
				if strings.Contains(body, needle) {
					t.Fatalf("%s must not call unbounded node read %q", tc.signature, needle)
				}
			}
			for _, needle := range tc.required {
				if !strings.Contains(body, needle) {
					t.Fatalf("%s must use bounded replacement %q", tc.signature, needle)
				}
			}
		})
	}
}

func goControllerFunctionBody(t *testing.T, source, signature string) string {
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
