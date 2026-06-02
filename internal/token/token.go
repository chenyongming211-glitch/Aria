package token

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ✅ 统一的业务哨兵错误 (Sentinel Errors)
var (
	ErrTokenNotFound  = errors.New("token not found")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenExhausted = errors.New("token exhausted (max uses reached)")
	ErrTokenRevoked   = errors.New("token revoked")
	ErrTokenInvalid   = errors.New("invalid token")
)

// Status represents the current state of a token
type Status string

const (
	StatusActive    Status = "active"
	StatusExhausted Status = "exhausted"
	StatusExpired   Status = "expired"
	StatusRevoked   Status = "revoked"
)

// Token represents an enrollment token for agent registration
type Token struct {
	ID         uuid.UUID `json:"id"`
	Token      string    `json:"token"`        // tk_xxxxxxxx format
	Tag        string    `json:"tag"`          // description tag
	TenantID   string    `json:"tenant_id"`    // associated tenant ID
	MaxUses    int       `json:"max_uses"`     // maximum usage count
	UsedCount  int       `json:"used_count"`   // current usage count
	ExpiresAt  time.Time `json:"expires_at"`   // expiration time
	CreatedAt  time.Time `json:"created_at"`   // creation time
	CreatedBy  string    `json:"created_by"`   // creator identifier
	Status     Status    `json:"status"`       // token status
	LastUsedAt time.Time `json:"last_used_at"` // last usage time
	LastUsedBy string    `json:"last_used_by"` // last user device ID
}

// IsValid checks if the token can still be used
func (t *Token) IsValid() bool {
	if t.Status != StatusActive {
		return false
	}
	// ✅ 修正：必须用 UTC 时间对比，防止跨时区导致失效时间漂移
	if time.Now().UTC().After(t.ExpiresAt) {
		return false
	}
	if t.MaxUses > 0 && t.UsedCount >= t.MaxUses {
		return false
	}
	return true
}

// RemainingUses returns how many times the token can still be used
func (t *Token) RemainingUses() int {
	if t.MaxUses == 0 {
		return -1
	}
	remaining := t.MaxUses - t.UsedCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TimeUntilExpiry returns the duration until the token expires
func (t *Token) TimeUntilExpiry() time.Duration {
	return time.Until(t.ExpiresAt)
}

// IsExpired checks if the token has passed its expiration time
func (t *Token) IsExpired() bool {
	return time.Now().UTC().After(t.ExpiresAt)
}
