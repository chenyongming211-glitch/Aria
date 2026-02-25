package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// EnrollmentToken represents an enrollment token for agent registration
type EnrollmentToken struct {
	ID              string    `db:"id"`
	Token           string    `db:"token"`
	Description     string    `db:"description"`
	CreatedAt       time.Time `db:"created_at"`
	ExpiresAt       time.Time `db:"expires_at"`
	MaxUses         int       `db:"max_uses"`
	UsedCount       int       `db:"used_count"`
	Status          string    `db:"status"` // active, expired, exhausted, revoked
	CreatedBy       string    `db:"created_by"`
	LastUsedAt      time.Time `db:"last_used_at"`
	LastUsedBy      string    `db:"last_used_by"`
}

// GenerateToken creates a new enrollment token
func GenerateToken(description string, validDays int, maxUses int, createdBy string) (*EnrollmentToken, error) {
	// Generate random token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	tokenStr := hex.EncodeToString(tokenBytes)
	now := time.Now()
	expiresAt := now.AddDate(0, 0, validDays)

	token := &EnrollmentToken{
		ID:          generateID(),
		Token:       tokenStr,
		Description: description,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		MaxUses:     maxUses,
		UsedCount:   0,
		Status:      "active",
		CreatedBy:   createdBy,
	}

	return token, nil
}

// ValidateToken checks if a token is valid for registration
func (t *EnrollmentToken) ValidateToken() error {
	// Check status
	if t.Status == "revoked" {
		return fmt.Errorf("token has been revoked")
	}

	if t.Status == "exhausted" {
		return fmt.Errorf("token has reached maximum uses")
	}

	// Check expiration
	if time.Now().After(t.ExpiresAt) {
		return fmt.Errorf("token has expired")
	}

	// Check usage limit
	if t.MaxUses > 0 && t.UsedCount >= t.MaxUses {
		return fmt.Errorf("token has reached maximum uses (%d)", t.MaxUses)
	}

	return nil
}

// UseToken increments the usage count and updates last used info
func (t *EnrollmentToken) UseToken(usedBy string) error {
	if err := t.ValidateToken(); err != nil {
		return err
	}

	t.UsedCount++
	t.LastUsedAt = time.Now()
	t.LastUsedBy = usedBy

	// Update status if exhausted
	if t.MaxUses > 0 && t.UsedCount >= t.MaxUses {
		t.Status = "exhausted"
	}

	return nil
}

// RevokeToken marks a token as revoked
func (t *EnrollmentToken) RevokeToken() {
	t.Status = "revoked"
}

// GetStatus returns human-readable status
func (t *EnrollmentToken) GetStatus() string {
	if t.Status == "revoked" {
		return "已撤销"
	}
	if t.Status == "exhausted" {
		return "已用尽"
	}
	if time.Now().After(t.ExpiresAt) {
		return "已过期"
	}
	if t.Status == "active" {
		remaining := t.MaxUses - t.UsedCount
		if t.MaxUses > 0 {
			return fmt.Sprintf("活跃 (剩余: %d/%d)", remaining, t.MaxUses)
		}
		return "活跃 (无限制)"
	}
	return t.Status
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
