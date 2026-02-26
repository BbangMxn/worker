package domain

import (
	"time"

	"github.com/google/uuid"
)

type Contact struct {
	ID         int64     `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Provider   string    `json:"provider,omitempty"`
	ProviderID string    `json:"provider_id,omitempty"`

	// Basic info
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone,omitempty"`
	PhotoURL string `json:"photo_url,omitempty"`

	// Work info
	Company    string `json:"company,omitempty"`
	JobTitle   string `json:"job_title,omitempty"`
	Department string `json:"department,omitempty"`

	// Additional
	Notes  string   `json:"notes,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Groups []string `json:"groups,omitempty"`

	// Relationship
	RelationshipScore    int16      `json:"relationship_score"`
	InteractionCount     int        `json:"interaction_count"`
	InteractionFrequency string     `json:"interaction_frequency,omitempty"`
	LastContactDate      *time.Time `json:"last_contact_date,omitempty"`
	LastInteractionAt    *time.Time `json:"last_interaction_at,omitempty"`
	IsFavorite           bool       `json:"is_favorite"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	SyncedAt  *time.Time `json:"synced_at,omitempty"`
}

type Company struct {
	ID          int64     `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Domain      *string   `json:"domain,omitempty"`
	Industry    *string   `json:"industry,omitempty"`
	Size        *string   `json:"size,omitempty"`
	Website     *string   `json:"website,omitempty"`
	Description *string   `json:"description,omitempty"`
	LogoURL     *string   `json:"logo_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ContactFilter struct {
	UserID  uuid.UUID
	Search  *string
	Company *string
	Group   *string
	Tags    []string
	Limit   int
	Offset  int
}

type ContactRepository interface {
	GetByID(id int64) (*Contact, error)
	GetByEmail(userID uuid.UUID, email string) (*Contact, error)
	List(filter *ContactFilter) ([]*Contact, int, error)
	Create(contact *Contact) error
	Update(contact *Contact) error
	Delete(id int64) error

	// Company
	GetCompanyByID(id int64) (*Company, error)
	GetCompanyByDomain(userID uuid.UUID, domain string) (*Company, error)
	ListCompanies(userID uuid.UUID, limit, offset int) ([]*Company, int, error)
	CreateCompany(company *Company) error
	UpdateCompany(company *Company) error
	DeleteCompany(id int64) error
}
