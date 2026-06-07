package nodeidentity

import "testing"

func TestNormalizePublicIPv4RejectsLocalRanges(t *testing.T) {
	for _, value := range []string{
		"10.2.0.3",
		"172.16.0.10",
		"192.168.1.10",
		"100.64.0.2",
		"127.0.0.1",
		"169.254.1.1",
		"198.18.0.1",
		"203.0.113.10",
	} {
		if got := NormalizePublicIPv4(value); got != "" {
			t.Fatalf("expected %q to be rejected, got %q", value, got)
		}
	}
}

func TestNormalizePublicIPv4AcceptsGlobalAddress(t *testing.T) {
	if got := NormalizePublicIPv4("82.156.48.111"); got != "82.156.48.111" {
		t.Fatalf("expected true public ip, got %q", got)
	}
}

func TestNormalizePublicEndpointReplacesPrivateHostWithPublicIP(t *testing.T) {
	got := NormalizePublicEndpoint("10.2.0.3:51820", "82.156.48.111")
	if got != "82.156.48.111:51820" {
		t.Fatalf("expected endpoint to use public ip, got %q", got)
	}

	if got := NormalizePublicEndpoint("10.2.0.3", ""); got != "" {
		t.Fatalf("expected local endpoint without port to be rejected, got %q", got)
	}
}

func TestNormalizeReportedNetworkDerivesPublicIPFromEndpoint(t *testing.T) {
	publicIP, endpoint := NormalizeReportedNetwork("", "82.156.48.111:51820")
	if publicIP != "82.156.48.111" || endpoint != "82.156.48.111:51820" {
		t.Fatalf("unexpected network identity publicIP=%q endpoint=%q", publicIP, endpoint)
	}
}
