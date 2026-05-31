package auth

import (
	"errors"
	"testing"
)

func TestGenerateTokenFailsWhenJWTSecretMissing(t *testing.T) {
	SetSecret("")

	_, err := GenerateToken("user-1", "admin", "super_admin", "")
	if err == nil {
		t.Fatalf("expected GenerateToken to fail when JWT secret is missing")
	}
	if !errors.Is(err, ErrJWTSecretNotConfigured) {
		t.Fatalf("expected ErrJWTSecretNotConfigured, got %v", err)
	}
}
