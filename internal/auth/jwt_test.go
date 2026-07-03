package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func TestValidateTokenRejectsWrongIssuer(t *testing.T) {
	SetSecret("test-jwt-secret")
	t.Cleanup(func() { SetSecret("") })

	claims := &Claims{
		UserID:   "user-1",
		Username: "alice",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "unexpected-issuer",
			Subject:   "user-1",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret, err := getSecret()
	if err != nil {
		t.Fatalf("getSecret failed: %v", err)
	}
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString failed: %v", err)
	}

	_, err = ValidateToken(tokenString)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for wrong issuer, got %v", err)
	}
}

func TestGenerateTokenWithVersionIncludesTokenVersion(t *testing.T) {
	SetSecret("test-jwt-secret")
	t.Cleanup(func() { SetSecret("") })

	token, err := GenerateTokenWithVersion("user-1", "alice", "admin", "tenant-1", 7)
	if err != nil {
		t.Fatalf("GenerateTokenWithVersion failed: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.TokenVersion != 7 {
		t.Fatalf("expected token version 7, got %d", claims.TokenVersion)
	}
}
