package models

import "time"

type Upstream struct {
	ID        uint      `gorm:"primarykey;autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string `gorm:"uniqueIndex;not null"`
	BaseURL   string `gorm:"column:base_url;not null"`
	APIKey    string `gorm:"column:api_key"`
	Enabled   bool   `gorm:"default:true"`
}

type AccessKey struct {
	ID        string    `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Token     string `gorm:"uniqueIndex;not null"`
	Enabled   bool   `gorm:"default:true"`
}

type UsageRecord struct {
	ID           uint      `gorm:"primarykey;autoIncrement"`
	CreatedAt    time.Time
	KeyID        string `gorm:"index;not null"`
	UpstreamID   uint   `gorm:"index;not null"`
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
	Success      bool
}
