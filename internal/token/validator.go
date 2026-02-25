package token

import (
	"fmt"
	"log"
	"time"
)

type Validator struct {
	store *Store
}

func NewValidator(store *Store) *Validator {
	return &Validator{store: store}
}

func (v *Validator) Validate(tokenStr string) (*Token, error) {
	if tokenStr == "" {
		return nil, ErrTokenInvalid
	}

	token, err := v.store.GetByToken(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %w", err)
	}
	if token == nil {
		return nil, ErrTokenNotFound
	}

	switch token.Status {
	case StatusRevoked:
		return nil, ErrTokenRevoked
	case StatusExpired:
		return nil, ErrTokenExpired
	case StatusExhausted:
		return nil, ErrTokenExhausted
	}

	if time.Now().UTC().After(token.ExpiresAt) {
		// ✅ 记录状态同步时的潜在数据库错误
		if err := v.store.UpdateStatus(token.ID.String(), StatusExpired); err != nil {
			log.Printf("⚠️ ERROR: failed to update token %s status to expired: %v", token.ID, err)
		}
		return nil, ErrTokenExpired
	}

	if token.UsedCount >= token.MaxUses {
		// ✅ 记录状态同步时的潜在数据库错误
		if err := v.store.UpdateStatus(token.ID.String(), StatusExhausted); err != nil {
			log.Printf("⚠️ ERROR: failed to update token %s status to exhausted: %v", token.ID, err)
		}
		return nil, ErrTokenExhausted
	}

	return token, nil
}

func (v *Validator) ConsumeToken(tokenStr, deviceID string) error {
	token, err := v.Validate(tokenStr)
	if err != nil {
		return err
	}

	return v.store.IncrementUsage(token.Token, deviceID)
}

func (v *Validator) ValidateOnly(tokenStr string) error {
	_, err := v.Validate(tokenStr)
	return err
}