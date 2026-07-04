package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRouteBodyRejectsEmptyPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))

	if route, err := decodeRouteBody(req); err == nil {
		t.Fatalf("expected empty route payload to be rejected, got route %q", route)
	}
}

func TestNormalizeRoutesUsesIPv6HostPrefix(t *testing.T) {
	routes, err := normalizeRoutes([]string{"2001:db8::10"})
	if err != nil {
		t.Fatalf("normalizeRoutes failed: %v", err)
	}
	if len(routes) != 1 || routes[0] != "2001:db8::10/128" {
		t.Fatalf("expected bare IPv6 host route to normalize to /128, got %#v", routes)
	}
}
