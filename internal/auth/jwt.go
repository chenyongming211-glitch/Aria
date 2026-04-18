package auth

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

var (
	jwtSecret   = []byte("aria-jwt-secret-key-32-bytes-long!")
	jwtSecretMu sync.RWMutex
)

// getSecret 获取当前的 JWT 密钥（线程安全）
func getSecret() []byte {
	jwtSecretMu.RLock()
	defer jwtSecretMu.RUnlock()
	return jwtSecret
}

// Claims 定义JWT声明
type Claims struct {
	UserID             string `json:"uid"`
	Username           string `json:"unm"`
	Role               string `json:"rol"`
	TenantID           string `json:"tid,omitempty"`
	MustChangePassword bool   `json:"mcp,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT令牌
func GenerateToken(userID, username, role, tenantID string, mustChangePassword ...bool) (string, error) {
	// 设置令牌过期时间（2小时）
	expirationTime := time.Now().Add(2 * time.Hour)

	mcp := false
	if len(mustChangePassword) > 0 {
		mcp = mustChangePassword[0]
	}

	claims := &Claims{
		UserID:             userID,
		Username:           username,
		Role:               role,
		TenantID:           tenantID,
		MustChangePassword: mcp,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "aria-controller",
			Subject:   userID,
		},
	}

	// 创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名令牌
	tokenString, err := token.SignedString(getSecret())
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken 验证JWT令牌并返回声明
func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getSecret(), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ExtractUserInfo 从令牌中提取用户信息（不验证，仅用于调试）
func ExtractUserInfo(tokenString string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := &Claims{}

	_, _, err := parser.ParseUnverified(tokenString, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	return claims, nil
}

// SetSecret 设置JWT密钥（应在初始化时调用）
func SetSecret(secret string) {
	jwtSecretMu.Lock()
	defer jwtSecretMu.Unlock()
	jwtSecret = []byte(secret)
}
