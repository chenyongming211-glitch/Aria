package auth

import (
	"errors"
	"testing"
)

func TestLoadRuntimeSecretFromEnv_RequiresConfig(t *testing.T) {
	t.Setenv("ARIA_RUNTIME_TOKEN_SECRET", "")

	err := LoadRuntimeSecretFromEnv()
	if !errors.Is(err, ErrRuntimeTokenSecretNotConfigured) {
		t.Fatalf("expected ErrRuntimeTokenSecretNotConfigured, got %v", err)
	}
}

func TestGenerateAndValidateRuntimeToken_WithConfiguredSecret(t *testing.T) {
	SetRuntimeSecret("test-runtime-secret")
	t.Cleanup(func() { SetRuntimeSecret("") })

	token, _, err := GenerateRuntimeToken("node-1", "tenant-1")
	if err != nil {
		t.Fatalf("GenerateRuntimeToken failed: %v", err)
	}

	claims, err := ValidateRuntimeToken(token)
	if err != nil {
		t.Fatalf("ValidateRuntimeToken failed: %v", err)
	}
	if claims.NodeID != "node-1" {
		t.Fatalf("expected node id node-1, got %s", claims.NodeID)
	}
	if claims.TenantID != "tenant-1" {
		t.Fatalf("expected tenant id tenant-1, got %s", claims.TenantID)
	}
}

func TestGenerateRuntimeTokenWithVersionIncludesTokenVersion(t *testing.T) {
	SetRuntimeSecret("test-runtime-secret")
	t.Cleanup(func() { SetRuntimeSecret("") })

	token, _, err := GenerateRuntimeTokenWithVersion("node-1", "tenant-1", 3)
	if err != nil {
		t.Fatalf("GenerateRuntimeTokenWithVersion failed: %v", err)
	}

	claims, err := ValidateRuntimeToken(token)
	if err != nil {
		t.Fatalf("ValidateRuntimeToken failed: %v", err)
	}
	if claims.TokenVersion != 3 {
		t.Fatalf("expected runtime token version 3, got %d", claims.TokenVersion)
	}
}

func TestGenerateRuntimeToken_FailsWhenSecretMissing(t *testing.T) {
	SetRuntimeSecret("")

	_, _, err := GenerateRuntimeToken("node-1", "tenant-1")
	if err == nil {
		t.Fatalf("expected error when runtime secret is missing")
	}
	if !errors.Is(err, ErrRuntimeTokenSecretNotConfigured) {
		t.Fatalf("expected ErrRuntimeTokenSecretNotConfigured, got %v", err)
	}
}
