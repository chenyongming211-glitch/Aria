package v2

import "testing"

func TestPromQLInstanceRegexEscapesIPDotsForPromQLString(t *testing.T) {
	got := promQLInstanceRegex("10.2.0.3")
	want := `10\\.2\\.0\\.3:.*`

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPromQLInstanceRegexEscapesQuotesAndBackslashes(t *testing.T) {
	got := promQLInstanceRegex(`host"name\part`)
	want := `host\"name\\\\part:.*`

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPromQLInstanceRegexIgnoresBlankHost(t *testing.T) {
	if got := promQLInstanceRegex("  "); got != "" {
		t.Fatalf("expected blank host to return empty matcher, got %q", got)
	}
}
