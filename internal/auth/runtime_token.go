package auth

import (
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
	runtimeSecret   = []byte("aria-runtime-secret-key-change-in-prod!")
	runtimeSecretMu sync.RWMutex
)

func getRuntimeSecret() []byte {
	runtimeSecretMu.RLock()
	defer runtimeSecretMu.RUnlock()
	return runtimeSecret
}

func SetRuntimeSecret(secret string) {
	runtimeSecretMu.Lock()
	defer runtimeSecretMu.Unlock()
	runtimeSecret = []byte(secret)
}

func init() {
	if secret := os.Getenv("ARIA_RUNTIME_TOKEN_SECRET"); secret != "" {
		SetRuntimeSecret(secret)
	}
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(getRuntimeSecret())
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

	claims := &RuntimeClaims{}
	token, err := jwt.ParseWithClaims(jwtPart, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getRuntimeSecret(), nil
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
