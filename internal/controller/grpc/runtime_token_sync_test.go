package grpc

import (
	"errors"
	"testing"

	"aria/internal/auth"
	"aria/pkg/controllerstorage"

	"github.com/google/uuid"
)

func TestGenerateRuntimeTokenForNode(t *testing.T) {
	t.Run("returns token when runtime secret is configured", func(t *testing.T) {
		auth.SetRuntimeSecret("grpc-runtime-secret")
		t.Cleanup(func() { auth.SetRuntimeSecret("") })

		node := &controllerstorage.Node{
			ID:       uuid.New(),
			TenantID: uuid.New(),
		}

		token, expiresAt, err := generateRuntimeTokenForNode(node)
		if err != nil {
			t.Fatalf("generateRuntimeTokenForNode failed: %v", err)
		}
		if token == "" {
			t.Fatalf("expected non-empty runtime token")
		}
		if expiresAt <= 0 {
			t.Fatalf("expected positive runtime token expiry, got %d", expiresAt)
		}
	})

	t.Run("returns error when runtime secret is missing", func(t *testing.T) {
		auth.SetRuntimeSecret("")

		node := &controllerstorage.Node{
			ID:       uuid.New(),
			TenantID: uuid.New(),
		}

		_, _, err := generateRuntimeTokenForNode(node)
		if err == nil {
			t.Fatalf("expected error when runtime secret is missing")
		}
		if !errors.Is(err, auth.ErrRuntimeTokenSecretNotConfigured) {
			t.Fatalf("expected ErrRuntimeTokenSecretNotConfigured, got %v", err)
		}
	})
}
