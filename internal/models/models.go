package models

import (
	"time"

	"one-codingplan/internal/crypto"
)

type Upstream struct {
	ID        uint      `gorm:"primarykey;autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string `gorm:"uniqueIndex;not null"`
	BaseURL   string `gorm:"column:base_url;not null"`
	APIKeyEnc []byte `gorm:"column:api_key_enc"`
	Enabled       bool   `gorm:"default:true"`
	ModelOverride string `gorm:"column:model_override;default:''"`
}


// DecryptAPIKey returns the plaintext API key using the provided AES key.
func (u *Upstream) DecryptAPIKey(encKey []byte) (string, error) {
	if len(u.APIKeyEnc) == 0 {
		return "", nil
	}
	return crypto.Decrypt(encKey, u.APIKeyEnc)
}

type AccessKey struct {
	ID        string    `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Token     string `gorm:"uniqueIndex;not null"`
	Enabled   bool   `gorm:"default:true"`

	Name               string     `gorm:"not null;default:''"`
	TokenBudget        int64      `gorm:"default:0"`
	AllowedUpstreams   string     `gorm:"type:text;default:''"` // JSON-encoded []string; "" = unrestricted
	ExpiresAt          *time.Time
	RateLimitPerMinute int `gorm:"default:0"`
	RateLimitPerDay    int `gorm:"default:0"`
}

type UsageRecord struct {
	ID           uint      `gorm:"primarykey;autoIncrement"`
	CreatedAt    time.Time `gorm:"index"`
	KeyID        string    `gorm:"index;not null"`
	UpstreamID   uint      `gorm:"index;not null"`
	UpstreamName string    `gorm:"not null;default:''"`
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
	Success      bool
}
