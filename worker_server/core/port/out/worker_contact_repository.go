package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ContactRepository defines the outbound port for contact persistence.
type ContactRepository interface {
	// CRUD operations
	Create(ctx context.Context, contact *ContactEntity) error
	Update(ctx context.Context, contact *ContactEntity) error
	Delete(ctx context.Context, userID uuid.UUID, id int64) error
	GetByID(ctx context.Context, userID uuid.UUID, id int64) (*ContactEntity, error)
	GetByEmail(ctx context.Context, userID uuid.UUID, email string) (*ContactEntity, error)

	// Query operations
	List(ctx context.Context, userID uuid.UUID, query *ContactListQuery) ([]*ContactEntity, int, error)

	// Interaction updates
	UpdateInteraction(ctx context.Context, userID uuid.UUID, contactID int64) error
	UpdateRelationshipScore(ctx context.Context, userID uuid.UUID, contactID int64, score int16) error

	// Sync operations
	Upsert(ctx context.Context, contact *ContactEntity) error
}

// ContactEntity represents contact domain entity.
type ContactEntity struct {
	ID                   int64
	UserID               uuid.UUID
	Provider             string
	ProviderID           string
	Name                 string
	Email                string
	Phone                string
	PhotoURL             string
	Company              string
	JobTitle             string
	Department           string
	Notes                string
	Tags                 []string
	Groups               []string
	RelationshipScore    int16
	InteractionCount     int
	InteractionFrequency string
	LastContactDate      *time.Time
	LastInteractionAt    *time.Time
	IsFavorite           bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	SyncedAt             *time.Time
}

// ContactListQuery represents contact list query parameters.
type ContactListQuery struct {
	Search    string
	Company   string
	Tags      []string
	Favorites bool
	Provider  string
	Limit     int
	Offset    int
	OrderBy   string
	Order     string
}
