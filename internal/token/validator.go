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
	// 直接尝试原子化增加使用次数，数据库会处理 used_count < max_uses 逻辑
	// 这样可以避免 Validate() 后 IncrementUsage() 之前的竞态窗口
	err := v.store.IncrementUsage(tokenStr, deviceID)
	if err != nil {
		// 如果原子更新失败，再通过 Validate 获取具体原因反馈给用户
		_, validateErr := v.Validate(tokenStr)
		if validateErr != nil {
			return validateErr
		}
		return err
	}
	return nil
}

func (v *Validator) ValidateOnly(tokenStr string) error {
	_, err := v.Validate(tokenStr)
	return err
}