package v2

import (
	"os"
	"strings"
	"testing"
)

func TestBug92MonitoringAndPolicyAPIsDoNotUseUnboundedTenantNodeReads(t *testing.T) {
	testCases := []struct {
		name      string
		file      string
		signature string
		forbidden []string
		required  []string
	}{
		{
			name:      "monitoring node detail learned routes",
			file:      "monitoring.go",
			signature: "func (r *Router) handleMonitoringNodeDetail",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"ListTenantLearnedRoutes(",
			},
		},
		{
			name:      "policy center list",
			file:      "setup.go",
			signature: "func (r *Router) listTenantPolicies",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"GetNodesByTenantPage(",
				"parseNodeListPagination(",
			},
		},
		{
			name:      "route conflict validation",
			file:      "setup.go",
			signature: "func (r *Router) validateTenantAdvertisedRouteConflicts",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"FindTenantAdvertisedRouteConflict(",
			},
		},
		{
			name:      "monitoring traffic",
			file:      "monitoring.go",
			signature: "func (r *Router) handleMonitoringTraffic",
			forbidden: []string{
				"GetNodesByTenant(",
			},
			required: []string{
				"GetNodesByTenantPage(",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("failed to read %s: %v", tc.file, err)
			}
			body := goFunctionBody(t, string(sourceBytes), tc.signature)
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
