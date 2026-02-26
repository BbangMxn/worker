package in

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type ContactService interface {
	// Contact operations
	GetContact(ctx context.Context, contactID int64) (*domain.Contact, error)
	GetContactByEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.Contact, error)
	ListContacts(ctx context.Context, filter *domain.ContactFilter) ([]*domain.Contact, int, error)
	CreateContact(ctx context.Context, userID uuid.UUID, req *CreateContactRequest) (*domain.Contact, error)
	UpdateContact(ctx context.Context, contactID int64, req *UpdateContactRequest) (*domain.Contact, error)
	DeleteContact(ctx context.Context, contactID int64) error

	// Company operations
	GetCompany(ctx context.Context, companyID int64) (*domain.Company, error)
	ListCompanies(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Company, int, error)
	CreateCompany(ctx context.Context, userID uuid.UUID, req *CreateCompanyRequest) (*domain.Company, error)
	UpdateCompany(ctx context.Context, companyID int64, req *UpdateCompanyRequest) (*domain.Company, error)
	DeleteCompany(ctx context.Context, companyID int64) error

	// Auto-extract from emails
	ExtractContactsFromEmails(ctx context.Context, userID uuid.UUID) (int, error)
}

type CreateContactRequest struct {
	Email   string   `json:"email"`
	Name    *string  `json:"name,omitempty"`
	Company *string  `json:"company,omitempty"`
	Title   *string  `json:"title,omitempty"`
	Phone   *string  `json:"phone,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type UpdateContactRequest struct {
	Name    *string  `json:"name,omitempty"`
	Company *string  `json:"company,omitempty"`
	Title   *string  `json:"title,omitempty"`
	Phone   *string  `json:"phone,omitempty"`
	Notes   *string  `json:"notes,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type CreateCompanyRequest struct {
	Name        string  `json:"name"`
	Domain      *string `json:"domain,omitempty"`
	Industry    *string `json:"industry,omitempty"`
	Website     *string `json:"website,omitempty"`
	Description *string `json:"description,omitempty"`
}

type UpdateCompanyRequest struct {
	Name        *string `json:"name,omitempty"`
	Industry    *string `json:"industry,omitempty"`
	Website     *string `json:"website,omitempty"`
	Description *string `json:"description,omitempty"`
}
