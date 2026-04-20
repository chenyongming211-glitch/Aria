package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RuntimeTokenTTL    = 24 * time.Hour
	RuntimeIssuer      = "aria-runtime"
	RuntimeTokenPrefix = "rt_"
)

var (
	runtimeSecret   []byte
	runtimeSecretMu sync.RWMutex
)

var ErrRuntimeTokenSecretNotConfigured = errors.New("runtime token secret is not configured")

func getRuntimeSecret() ([]byte, error) {
	runtimeSecretMu.RLock()
	defer runtimeSecretMu.RUnlock()
	if len(runtimeSecret) == 0 {
		return nil, ErrRuntimeTokenSecretNotConfigured
	}
	return runtimeSecret, nil
}

func SetRuntimeSecret(secret string) {
	trimmed := strings.TrimSpace(secret)
	runtimeSecretMu.Lock()
	defer runtimeSecretMu.Unlock()
	if trimmed == "" {
		runtimeSecret = nil
		return
	}
	runtimeSecret = []byte(trimmed)
}

func LoadRuntimeSecretFromEnv() error {
	secret := strings.TrimSpace(os.Getenv("ARIA_RUNTIME_TOKEN_SECRET"))
	if secret == "" {
		SetRuntimeSecret("")
		return ErrRuntimeTokenSecretNotConfigured
	}
	SetRuntimeSecret(secret)
	return nil
}

func init() {
	_ = LoadRuntimeSecretFromEnv()
}

type RuntimeClaims struct {
	NodeID   string `json:"nid"`
	TenantID string `json:"tid"`
	jwt.RegisteredClaims
}

func GenerateRuntimeToken(nodeID, tenantID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(RuntimeTokenTTL)
	claims := &RuntimeClaims{
		NodeID:   nodeID,
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    RuntimeIssuer,
			Subject:   nodeID,
		},
	}

	secret, err := getRuntimeSecret()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate runtime token: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign runtime token: %w", err)
	}

	return RuntimeTokenPrefix + tokenString, expiresAt, nil
}

func ValidateRuntimeToken(tokenString string) (*RuntimeClaims, error) {
	// 前缀校验：必须以 rt_ 开头
	if !strings.HasPrefix(tokenString, RuntimeTokenPrefix) {
		return nil, ErrInvalidToken
	}
	jwtPart := strings.TrimPrefix(tokenString, RuntimeTokenPrefix)

	secret, err := getRuntimeSecret()
	if err != nil {
		return nil, err
	}

	claims := &RuntimeClaims{}
	token, err := jwt.ParseWithClaims(jwtPart, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	}, jwt.WithIssuer(RuntimeIssuer))
	if err != nil {
		if err == jwt.ErrTokenExpired {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
