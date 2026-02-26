package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TemplateVariableEntity represents a variable in template
type TemplateVariableEntity struct {
	Name        string `json:"name"`
	Placeholder string `json:"placeholder"`
	DefaultVal  string `json:"default_value"`
	Description string `json:"description"`
}

// TemplateEntity represents the database entity for email templates
type TemplateEntity struct {
	ID         int64
	UserID     uuid.UUID
	Name       string
	Category   string
	Subject    string
	Body       string
	HTMLBody   string
	Variables  []TemplateVariableEntity
	Tags       []string
	IsDefault  bool
	IsArchived bool
	UsageCount int
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TemplateListQuery represents query parameters for listing templates
type TemplateListQuery struct {
	Category   *string
	Search     *string
	Tags       []string
	IsDefault  *bool
	IsArchived *bool
	Limit      int
	Offset     int
	OrderBy    string
	Order      string
}

// TemplateRepository defines the interface for template data operations
type TemplateRepository interface {
	// CRUD operations
	Create(ctx context.Context, template *TemplateEntity) error
	Update(ctx context.Context, template *TemplateEntity) error
	Delete(ctx context.Context, userID uuid.UUID, id int64) error
	GetByID(ctx context.Context, userID uuid.UUID, id int64) (*TemplateEntity, error)

	// Query operations
	List(ctx context.Context, userID uuid.UUID, query *TemplateListQuery) ([]*TemplateEntity, int, error)
	GetDefault(ctx context.Context, userID uuid.UUID, category string) (*TemplateEntity, error)
	GetByCategory(ctx context.Context, userID uuid.UUID, category string) ([]*TemplateEntity, error)

	// Usage tracking
	IncrementUsage(ctx context.Context, id int64) error

	// Default management
	SetDefault(ctx context.Context, userID uuid.UUID, id int64) error
	ClearDefault(ctx context.Context, userID uuid.UUID, category string) error

	// Archive operations
	Archive(ctx context.Context, userID uuid.UUID, id int64) error
	Restore(ctx context.Context, userID uuid.UUID, id int64) error

	// Batch operations
	DeleteBatch(ctx context.Context, userID uuid.UUID, ids []int64) error
}
