package domain

import (
	"time"

	"github.com/google/uuid"
)

// Label represents a user-defined tag for organizing emails
// Labels are different from Folders (location) and Categories (AI-assigned type)
// An email can have multiple labels (N:N relationship)
type Label struct {
	ID           int64     `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	ConnectionID *int64    `json:"connection_id,omitempty"` // nil = user-created
	ProviderID   *string   `json:"provider_id,omitempty"`   // Gmail/Outlook label ID

	Name     string  `json:"name"`
	Color    *string `json:"color,omitempty"`
	Position int     `json:"position"` // Display order

	IsSystem  bool `json:"is_system"` // INBOX, SENT, etc.
	IsVisible bool `json:"is_visible"`

	// Stats
	EmailCount  int `json:"email_count"`
	UnreadCount int `json:"unread_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LabelProviderMapping maps workspace labels to provider-specific labels/categories
type LabelProviderMapping struct {
	ID           int64     `json:"id"`
	LabelID      int64     `json:"label_id"`
	ConnectionID int64     `json:"connection_id"`
	Provider     Provider  `json:"provider"`
	ExternalID   *string   `json:"external_id,omitempty"` // Provider's label ID (nil = not yet created)
	MappingType  string    `json:"mapping_type"`          // "label" (Gmail) or "category" (Outlook)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LabelRepository interface {
	GetByID(id int64) (*Label, error)
	GetByProviderID(connectionID int64, providerID string) (*Label, error)
	ListByUser(userID uuid.UUID) ([]*Label, error)
	ListByConnection(connectionID int64) ([]*Label, error)
	Create(label *Label) error
	Update(label *Label) error
	Delete(id int64) error
	UpdatePositions(userID uuid.UUID, labelIDs []int64) error

	// Email-Label associations
	AddEmailLabel(emailID, labelID int64) error
	RemoveEmailLabel(emailID, labelID int64) error
	GetEmailLabels(emailID int64) ([]*Label, error)
	GetEmailsByLabel(labelID int64, limit, offset int) ([]int64, error)

	// Provider mappings
	GetMapping(labelID, connectionID int64) (*LabelProviderMapping, error)
	GetMappingByExternalID(connectionID int64, externalID string) (*LabelProviderMapping, error)
	CreateMapping(mapping *LabelProviderMapping) error
	UpdateMapping(mapping *LabelProviderMapping) error
	DeleteMapping(id int64) error
}
