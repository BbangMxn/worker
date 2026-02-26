// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// MailDomainWrapper wraps MailAdapter to implement domain.EmailRepository.
// This provides compatibility between the out.EmailRepository (persistence layer)
// and domain.EmailRepository (used by agent tools).
type MailDomainWrapper struct {
	adapter  *MailAdapter
	bodyRepo out.EmailBodyRepository
}

// NewMailDomainWrapper creates a wrapper that implements domain.EmailRepository.
func NewMailDomainWrapper(adapter *MailAdapter, bodyRepo out.EmailBodyRepository) *MailDomainWrapper {
	return &MailDomainWrapper{
		adapter:  adapter,
		bodyRepo: bodyRepo,
	}
}

// entityToDomain converts out.MailEntity to domain.Email
func (w *MailDomainWrapper) entityToDomain(e *out.MailEntity) *domain.Email {
	if e == nil {
		return nil
	}

	email := &domain.Email{
		ID:           e.ID,
		UserID:       e.UserID,
		ConnectionID: e.ConnectionID,
		Provider:     domain.Provider(e.Provider),
		ProviderID:   e.ExternalID,
		ThreadID:     "",
		Subject:      e.Subject,
		FromEmail:    e.FromEmail,
		ToEmails:     e.ToEmails,
		CcEmails:     e.CcEmails,
		BccEmails:    e.BccEmails,
		Date:         e.ReceivedAt,
		Folder:       domain.LegacyFolder(e.Folder),
		Labels:       e.Labels,
		IsRead:       e.IsRead,
		IsStarred:    e.IsStarred,
		HasAttach:    e.HasAttachment,
		ReceivedAt:   e.ReceivedAt,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}

	if e.FromName != "" {
		email.FromName = &e.FromName
	}
	if e.Category != "" {
		cat := domain.EmailCategory(e.Category)
		email.AICategory = &cat
	}
	if e.Priority > 0 {
		pri := domain.Priority(e.Priority)
		email.AIPriority = &pri
	}
	if e.Summary != "" {
		email.AISummary = &e.Summary
	}
	if e.Tags != nil {
		email.AITags = e.Tags
	}

	return email
}

// domainToEntity converts domain.Email to out.MailEntity
func (w *MailDomainWrapper) domainToEntity(d *domain.Email) *out.MailEntity {
	if d == nil {
		return nil
	}

	entity := &out.MailEntity{
		ID:            d.ID,
		UserID:        d.UserID,
		ConnectionID:  d.ConnectionID,
		Provider:      string(d.Provider),
		ExternalID:    d.ProviderID,
		FromEmail:     d.FromEmail,
		ToEmails:      d.ToEmails,
		CcEmails:      d.CcEmails,
		BccEmails:     d.BccEmails,
		Subject:       d.Subject,
		Folder:        string(d.Folder),
		Labels:        d.Labels,
		IsRead:        d.IsRead,
		HasAttachment: d.HasAttach,
		ReceivedAt:    d.ReceivedAt,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}

	if d.FromName != nil {
		entity.FromName = *d.FromName
	}
	if d.AICategory != nil {
		entity.Category = string(*d.AICategory)
	}
	if d.AISubCategory != nil {
		entity.SubCategory = string(*d.AISubCategory)
	}
	if d.AIPriority != nil {
		entity.Priority = float64(*d.AIPriority)
	}
	if d.AISummary != nil {
		entity.Summary = *d.AISummary
	}
	if d.AITags != nil {
		entity.Tags = d.AITags
	}
	if d.AIScore != nil {
		entity.AIScore = *d.AIScore
	}
	if d.ClassificationSource != nil {
		entity.ClassificationSource = string(*d.ClassificationSource)
	}

	return entity
}

// GetByID implements domain.EmailRepository.
func (w *MailDomainWrapper) GetByID(id int64) (*domain.Email, error) {
	ctx := context.Background()
	entity, err := w.adapter.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return w.entityToDomain(entity), nil
}

// GetByProviderID implements domain.EmailRepository.
func (w *MailDomainWrapper) GetByProviderID(userID uuid.UUID, provider domain.Provider, providerID string) (*domain.Email, error) {
	// Not directly supported - would need connection lookup
	return nil, nil
}

// GetByThreadID implements domain.EmailRepository.
func (w *MailDomainWrapper) GetByThreadID(threadID string) ([]*domain.Email, error) {
	// MailAdapter doesn't have GetByThreadID, return empty for now
	return []*domain.Email{}, nil
}

// GetByDateRange implements domain.EmailRepository.
func (w *MailDomainWrapper) GetByDateRange(userID uuid.UUID, startDate, endDate time.Time) ([]*domain.Email, error) {
	ctx := context.Background()
	// Use basic list query - date filtering would need to be added to MailListQuery
	query := &out.MailListQuery{
		Limit: 1000,
	}

	entities, _, err := w.adapter.List(ctx, userID, query)
	if err != nil {
		return nil, err
	}

	// Filter by date in memory
	var emails []*domain.Email
	for _, e := range entities {
		if !e.ReceivedAt.Before(startDate) && !e.ReceivedAt.After(endDate) {
			emails = append(emails, w.entityToDomain(e))
		}
	}
	return emails, nil
}

// List implements domain.EmailRepository.
func (w *MailDomainWrapper) List(filter *domain.EmailFilter) ([]*domain.Email, int, error) {
	ctx := context.Background()

	query := &out.MailListQuery{
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	if filter.Folder != nil {
		query.Folder = string(*filter.Folder)
	}
	if filter.Category != nil {
		query.Category = string(*filter.Category)
	}
	if filter.Priority != nil {
		pri := float64(*filter.Priority)
		query.Priority = &pri
	}
	if filter.MinPriority != nil {
		pri := float64(*filter.MinPriority)
		query.Priority = &pri
	}
	if filter.IsRead != nil {
		query.IsRead = filter.IsRead
	}
	if filter.IsStarred != nil {
		query.IsStarred = filter.IsStarred
	}
	if filter.ConnectionID != nil {
		query.ConnectionID = filter.ConnectionID
	}
	if filter.FolderID != nil {
		query.FolderID = filter.FolderID
	}
	if filter.SubCategory != nil {
		query.SubCategory = string(*filter.SubCategory)
	}
	if filter.WorkflowStatus != nil {
		query.WorkflowStatus = string(*filter.WorkflowStatus)
	}
	if filter.FromEmail != nil {
		query.FromEmail = *filter.FromEmail
	}
	if filter.FromDomain != nil {
		query.FromDomain = *filter.FromDomain
	}
	if len(filter.LabelIDs) > 0 {
		query.LabelIDs = filter.LabelIDs
	}

	// === Inbox/Category View Filters ===
	if filter.ViewType != nil {
		query.ViewType = *filter.ViewType
	}
	if len(filter.Categories) > 0 {
		query.Categories = make([]string, len(filter.Categories))
		for i, c := range filter.Categories {
			query.Categories[i] = string(c)
		}
	}
	if len(filter.SubCategories) > 0 {
		query.SubCategories = make([]string, len(filter.SubCategories))
		for i, sc := range filter.SubCategories {
			query.SubCategories[i] = string(sc)
		}
	}
	if len(filter.ExcludeCategories) > 0 {
		query.ExcludeCategories = make([]string, len(filter.ExcludeCategories))
		for i, c := range filter.ExcludeCategories {
			query.ExcludeCategories[i] = string(c)
		}
	}

	// === Sorting ===
	// SortBy: "date" (default), "priority" (TODO view)
	if filter.SortBy == "priority" {
		query.OrderBy = "ai_priority"
		query.Order = "desc"
	}

	entities, total, err := w.adapter.List(ctx, filter.UserID, query)
	if err != nil {
		return nil, 0, err
	}

	// Apply additional filters in memory if needed
	var emails []*domain.Email
	for _, e := range entities {
		email := w.entityToDomain(e)

		// Search filter (if any)
		if filter.Search != nil && *filter.Search != "" {
			searchLower := stringToLower(*filter.Search)
			if !containsIgnoreCase(email.Subject, searchLower) &&
				!containsIgnoreCase(email.FromEmail, searchLower) {
				continue
			}
		}

		// FromEmail filter
		if filter.FromEmail != nil && *filter.FromEmail != "" {
			if !containsIgnoreCase(email.FromEmail, *filter.FromEmail) {
				continue
			}
		}

		// Date filters
		if filter.DateFrom != nil && email.Date.Before(*filter.DateFrom) {
			continue
		}
		if filter.DateTo != nil && email.Date.After(*filter.DateTo) {
			continue
		}

		emails = append(emails, email)
	}

	return emails, total, nil
}

// Create implements domain.EmailRepository.
func (w *MailDomainWrapper) Create(email *domain.Email) error {
	ctx := context.Background()
	entity := w.domainToEntity(email)
	err := w.adapter.Create(ctx, entity)
	if err != nil {
		return err
	}
	email.ID = entity.ID
	email.CreatedAt = entity.CreatedAt
	email.UpdatedAt = entity.UpdatedAt
	return nil
}

// CreateBatch implements domain.EmailRepository.
func (w *MailDomainWrapper) CreateBatch(emails []*domain.Email) error {
	// Create one by one since batch isn't available
	for _, email := range emails {
		if err := w.Create(email); err != nil {
			return err
		}
	}
	return nil
}

// Update implements domain.EmailRepository.
func (w *MailDomainWrapper) Update(email *domain.Email) error {
	ctx := context.Background()
	entity := w.domainToEntity(email)
	return w.adapter.Update(ctx, entity)
}

// Delete implements domain.EmailRepository.
func (w *MailDomainWrapper) Delete(id int64) error {
	ctx := context.Background()
	return w.adapter.Delete(ctx, id)
}

// GetBody implements domain.EmailRepository.
func (w *MailDomainWrapper) GetBody(emailID int64) (*domain.EmailBody, error) {
	if w.bodyRepo == nil {
		return nil, nil
	}

	ctx := context.Background()
	body, err := w.bodyRepo.GetBody(ctx, emailID)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	return &domain.EmailBody{
		EmailID:      body.EmailID,
		TextBody:     body.Text,
		HTMLBody:     body.HTML,
		IsCompressed: body.IsCompressed,
	}, nil
}

// SaveBody implements domain.EmailRepository.
func (w *MailDomainWrapper) SaveBody(body *domain.EmailBody) error {
	if w.bodyRepo == nil {
		return nil
	}

	ctx := context.Background()
	return w.bodyRepo.SaveBody(ctx, &out.MailBodyEntity{
		EmailID:      body.EmailID,
		Text:         body.TextBody,
		HTML:         body.HTMLBody,
		IsCompressed: body.IsCompressed,
	})
}

// Helper functions
func stringToLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func containsIgnoreCase(s, substr string) bool {
	sLower := stringToLower(s)
	substrLower := stringToLower(substr)
	return len(sLower) >= len(substrLower) && findSubstring(sLower, substrLower) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// Ensure MailDomainWrapper implements domain.EmailRepository
var _ domain.EmailRepository = (*MailDomainWrapper)(nil)
