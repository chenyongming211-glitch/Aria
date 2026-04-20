package grpc

import "testing"

func TestDefaultACLAction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "empty defaults to allow", input: "", expect: "allow"},
		{name: "allow normalized", input: " ALLOW ", expect: "allow"},
		{name: "deny normalized", input: "Deny", expect: "deny"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultACLAction(tt.input); got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}
