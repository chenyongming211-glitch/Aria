package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	TokenPrefix      = "tk_"
	TokenRandomBytes = 8
)

type GenerateOptions struct {
	Tag      string
	TenantID string
	MaxUses  int
	TTL      time.Duration
	Creator  string
}

func DefaultOptions() GenerateOptions {
	return GenerateOptions{
		Tag:      "",
		TenantID: "",
		MaxUses:  1,
		TTL:      24 * time.Hour,
		Creator:  "",
	}
}

func Generate(opts GenerateOptions) (*Token, error) {
	// ✅ 新增：拦截非法的 TenantID 字符串，防止脏数据流向数据库
	if opts.TenantID != "" {
		if _, err := uuid.Parse(opts.TenantID); err != nil {
			return nil, fmt.Errorf("invalid tenant_id format: %w", err)
		}
	}

	bytes := make([]byte, TokenRandomBytes)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}

	tokenStr := TokenPrefix + hex.EncodeToString(bytes)
	now := time.Now().UTC()

	return &Token{
		Token:     tokenStr,
		Tag:       opts.Tag,
		TenantID:  opts.TenantID, // ✅ 修正：直接原样赋值，保证未传时可以写入数据库 NULL
		MaxUses:   opts.MaxUses,
		UsedCount: 0,
		ExpiresAt: now.Add(opts.TTL),
		CreatedAt: now,
		CreatedBy: opts.Creator,
		Status:    StatusActive,
	}, nil
}

func GenerateSimple(tag string, maxUses int, ttl time.Duration) (*Token, error) {
	return Generate(GenerateOptions{
		Tag:     tag,
		MaxUses: maxUses,
		TTL:     ttl,
	})
}