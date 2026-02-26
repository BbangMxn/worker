// Package out defines outbound ports (driven ports) for the application.
package out

import (
	"context"
	"time"
)

// =============================================================================
// EmailBodyRepository (MongoDB - 30일 캐시)
// =============================================================================

// EmailBodyRepository defines the outbound port for mail body storage.
// 본문은 MongoDB에 30일간 캐시되며, 캐시 miss 시 Provider API에서 가져옴.
type EmailBodyRepository interface {
	// Single operations
	SaveBody(ctx context.Context, body *MailBodyEntity) error
	GetBody(ctx context.Context, emailID int64) (*MailBodyEntity, error)
	DeleteBody(ctx context.Context, emailID int64) error
	ExistsBody(ctx context.Context, emailID int64) (bool, error)

	// Check if body is cached (without fetching)
	IsCached(ctx context.Context, emailID int64) (bool, error)

	// Bulk operations
	BulkSaveBody(ctx context.Context, bodies []*MailBodyEntity) error
	BulkDeleteBody(ctx context.Context, emailIDs []int64) error
	BulkGetBody(ctx context.Context, emailIDs []int64) (map[int64]*MailBodyEntity, error)

	// Cleanup operations
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByConnectionID(ctx context.Context, connectionID int64) (int64, error)
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)

	// Stats
	GetStorageStats(ctx context.Context) (*BodyStorageStats, error)
	GetCompressionStats(ctx context.Context) (*CompressionStats, error)
}

// MailBodyEntity represents mail body domain entity.
type MailBodyEntity struct {
	EmailID      int64
	ConnectionID int64
	ExternalID   string

	// Content
	HTML string
	Text string

	// Attachments metadata (actual files stored separately)
	Attachments []AttachmentEntity

	// Size info
	OriginalSize   int64
	CompressedSize int64
	IsCompressed   bool

	// Cache metadata
	CachedAt  time.Time
	ExpiresAt time.Time // 30일 후
	TTLDays   int       // 기본 30일
}

// AttachmentEntity represents attachment metadata.
type AttachmentEntity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	ContentID string `json:"content_id,omitempty"` // inline attachment
	IsInline  bool   `json:"is_inline"`
	URL       string `json:"url,omitempty"` // download URL (Provider에서)
}

// BodyStorageStats represents body storage statistics.
type BodyStorageStats struct {
	TotalCount     int64      `json:"total_count"`
	TotalSize      int64      `json:"total_size"`      // 원본 크기
	CompressedSize int64      `json:"compressed_size"` // 압축 후 크기
	AvgCompression float64    `json:"avg_compression"` // 평균 압축률
	ExpiredCount   int64      `json:"expired_count"`   // 만료된 항목 수
	OldestEntry    *time.Time `json:"oldest_entry,omitempty"`
	NewestEntry    *time.Time `json:"newest_entry,omitempty"`
}

// CompressionStats represents compression statistics.
type CompressionStats struct {
	TotalBodies         int64   `json:"total_bodies"`
	CompressedBodies    int64   `json:"compressed_bodies"`
	UncompressedBodies  int64   `json:"uncompressed_bodies"`
	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
	TotalSaved          int64   `json:"total_saved"` // 절약된 바이트
}

// =============================================================================
// Cache Helper
// =============================================================================

// DefaultTTLDays is the default TTL for body cache.
// 동기화 기간(3개월)과 일치시킴 - 분기 단위
const DefaultBodyTTLDays = 90

// NewMailBodyEntity creates a new body entity with default TTL.
func NewMailBodyEntity(emailID, connectionID int64, externalID string) *MailBodyEntity {
	now := time.Now()
	return &MailBodyEntity{
		EmailID:      emailID,
		ConnectionID: connectionID,
		ExternalID:   externalID,
		CachedAt:     now,
		ExpiresAt:    now.AddDate(0, 0, DefaultBodyTTLDays),
		TTLDays:      DefaultBodyTTLDays,
	}
}

// IsExpired returns true if the body cache has expired.
func (b *MailBodyEntity) IsExpired() bool {
	return time.Now().After(b.ExpiresAt)
}

// Refresh extends the cache TTL.
func (b *MailBodyEntity) Refresh() {
	b.ExpiresAt = time.Now().AddDate(0, 0, b.TTLDays)
}
