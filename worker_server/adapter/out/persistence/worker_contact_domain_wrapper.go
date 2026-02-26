// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"worker_server/core/domain"

	"github.com/google/uuid"
)

// ContactDomainWrapper wraps ContactAdapter to implement domain.ContactRepository.
// This provides a clean separation between the out.ContactRepository (used by other adapters)
// and domain.ContactRepository (used by services).
type ContactDomainWrapper struct {
	adapter *ContactAdapter
}

// NewContactDomainWrapper creates a wrapper that implements domain.ContactRepository.
func NewContactDomainWrapper(adapter *ContactAdapter) *ContactDomainWrapper {
	return &ContactDomainWrapper{adapter: adapter}
}

// GetByID implements domain.ContactRepository.
func (w *ContactDomainWrapper) GetByID(id int64) (*domain.Contact, error) {
	return w.adapter.DomainGetByID(id)
}

// GetByEmail implements domain.ContactRepository.
func (w *ContactDomainWrapper) GetByEmail(userID uuid.UUID, email string) (*domain.Contact, error) {
	return w.adapter.DomainGetByEmail(userID, email)
}

// List implements domain.ContactRepository.
func (w *ContactDomainWrapper) List(filter *domain.ContactFilter) ([]*domain.Contact, int, error) {
	return w.adapter.DomainList(filter)
}

// Create implements domain.ContactRepository.
func (w *ContactDomainWrapper) Create(contact *domain.Contact) error {
	return w.adapter.DomainCreate(contact)
}

// Update implements domain.ContactRepository.
func (w *ContactDomainWrapper) Update(contact *domain.Contact) error {
	return w.adapter.DomainUpdate(contact)
}

// Delete implements domain.ContactRepository.
func (w *ContactDomainWrapper) Delete(id int64) error {
	return w.adapter.DomainDelete(id)
}

// GetCompanyByID implements domain.ContactRepository.
func (w *ContactDomainWrapper) GetCompanyByID(id int64) (*domain.Company, error) {
	return w.adapter.DomainGetCompanyByID(id)
}

// GetCompanyByDomain implements domain.ContactRepository.
func (w *ContactDomainWrapper) GetCompanyByDomain(userID uuid.UUID, domainName string) (*domain.Company, error) {
	return w.adapter.DomainGetCompanyByDomain(userID, domainName)
}

// ListCompanies implements domain.ContactRepository.
func (w *ContactDomainWrapper) ListCompanies(userID uuid.UUID, limit, offset int) ([]*domain.Company, int, error) {
	return w.adapter.DomainListCompanies(userID, limit, offset)
}

// CreateCompany implements domain.ContactRepository.
func (w *ContactDomainWrapper) CreateCompany(company *domain.Company) error {
	return w.adapter.DomainCreateCompany(company)
}

// UpdateCompany implements domain.ContactRepository.
func (w *ContactDomainWrapper) UpdateCompany(company *domain.Company) error {
	return w.adapter.DomainUpdateCompany(company)
}

// DeleteCompany implements domain.ContactRepository.
func (w *ContactDomainWrapper) DeleteCompany(id int64) error {
	return w.adapter.DomainDeleteCompany(id)
}

// Ensure ContactDomainWrapper implements domain.ContactRepository
var _ domain.ContactRepository = (*ContactDomainWrapper)(nil)
