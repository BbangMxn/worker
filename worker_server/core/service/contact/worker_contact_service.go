package contact

import (
	"context"

	"worker_server/core/domain"
	"worker_server/core/port/in"

	"github.com/google/uuid"
)

type Service struct {
	contactRepo domain.ContactRepository
}

func NewService(contactRepo domain.ContactRepository) *Service {
	return &Service{
		contactRepo: contactRepo,
	}
}

func (s *Service) GetContact(ctx context.Context, contactID int64) (*domain.Contact, error) {
	return s.contactRepo.GetByID(contactID)
}

func (s *Service) GetContactByEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.Contact, error) {
	return s.contactRepo.GetByEmail(userID, email)
}

func (s *Service) ListContacts(ctx context.Context, filter *domain.ContactFilter) ([]*domain.Contact, int, error) {
	return s.contactRepo.List(filter)
}

func (s *Service) CreateContact(ctx context.Context, userID uuid.UUID, req *in.CreateContactRequest) (*domain.Contact, error) {
	contact := &domain.Contact{
		UserID: userID,
		Email:  req.Email,
	}

	if req.Name != nil {
		contact.Name = *req.Name
	}
	if req.Company != nil {
		contact.Company = *req.Company
	}
	if req.Title != nil {
		contact.JobTitle = *req.Title
	}
	if req.Phone != nil {
		contact.Phone = *req.Phone
	}
	if req.Tags != nil {
		contact.Tags = req.Tags
	}

	if err := s.contactRepo.Create(contact); err != nil {
		return nil, err
	}

	return contact, nil
}

func (s *Service) UpdateContact(ctx context.Context, contactID int64, req *in.UpdateContactRequest) (*domain.Contact, error) {
	contact, err := s.contactRepo.GetByID(contactID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		contact.Name = *req.Name
	}
	if req.Company != nil {
		contact.Company = *req.Company
	}
	if req.Title != nil {
		contact.JobTitle = *req.Title
	}
	if req.Phone != nil {
		contact.Phone = *req.Phone
	}
	if req.Notes != nil {
		contact.Notes = *req.Notes
	}
	if req.Tags != nil {
		contact.Tags = req.Tags
	}

	if err := s.contactRepo.Update(contact); err != nil {
		return nil, err
	}

	return contact, nil
}

func (s *Service) DeleteContact(ctx context.Context, contactID int64) error {
	return s.contactRepo.Delete(contactID)
}

func (s *Service) GetCompany(ctx context.Context, companyID int64) (*domain.Company, error) {
	return s.contactRepo.GetCompanyByID(companyID)
}

func (s *Service) ListCompanies(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Company, int, error) {
	return s.contactRepo.ListCompanies(userID, limit, offset)
}

func (s *Service) CreateCompany(ctx context.Context, userID uuid.UUID, req *in.CreateCompanyRequest) (*domain.Company, error) {
	company := &domain.Company{
		UserID: userID,
		Name:   req.Name,
	}

	if req.Domain != nil {
		company.Domain = req.Domain
	}
	if req.Industry != nil {
		company.Industry = req.Industry
	}
	if req.Website != nil {
		company.Website = req.Website
	}
	if req.Description != nil {
		company.Description = req.Description
	}

	if err := s.contactRepo.CreateCompany(company); err != nil {
		return nil, err
	}

	return company, nil
}

func (s *Service) UpdateCompany(ctx context.Context, companyID int64, req *in.UpdateCompanyRequest) (*domain.Company, error) {
	company, err := s.contactRepo.GetCompanyByID(companyID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		company.Name = *req.Name
	}
	if req.Industry != nil {
		company.Industry = req.Industry
	}
	if req.Website != nil {
		company.Website = req.Website
	}
	if req.Description != nil {
		company.Description = req.Description
	}

	if err := s.contactRepo.UpdateCompany(company); err != nil {
		return nil, err
	}

	return company, nil
}

func (s *Service) DeleteCompany(ctx context.Context, companyID int64) error {
	return s.contactRepo.DeleteCompany(companyID)
}

func (s *Service) ExtractContactsFromEmails(ctx context.Context, userID uuid.UUID) (int, error) {
	// TODO: Implement contact extraction from emails
	return 0, nil
}
