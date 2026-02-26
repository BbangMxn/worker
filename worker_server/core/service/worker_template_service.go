package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// TemplateService handles email template business logic
type TemplateService struct {
	repo out.TemplateRepository
}

// NewTemplateService creates a new TemplateService
func NewTemplateService(repo out.TemplateRepository) *TemplateService {
	return &TemplateService{repo: repo}
}

// CreateTemplateRequest represents the request to create a template
type CreateTemplateRequest struct {
	Name      string
	Category  string
	Subject   *string
	Body      string
	HTMLBody  *string
	Variables []domain.TemplateVariable
	Tags      []string
	IsDefault bool
}

// UpdateTemplateRequest represents the request to update a template
type UpdateTemplateRequest struct {
	ID        int64
	Name      *string
	Category  *string
	Subject   *string
	Body      *string
	HTMLBody  *string
	Variables *[]domain.TemplateVariable
	Tags      *[]string
	IsDefault *bool
}

// TemplateListRequest represents the request to list templates
type TemplateListRequest struct {
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

// Create creates a new template
func (s *TemplateService) Create(ctx context.Context, userID uuid.UUID, req *CreateTemplateRequest) (*domain.EmailTemplate, error) {
	// Validate category
	if !domain.IsValidTemplateCategory(req.Category) {
		req.Category = string(domain.TemplateCategoryCustom)
	}

	// Validate name
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("template name is required")
	}

	// Validate body
	if strings.TrimSpace(req.Body) == "" {
		return nil, fmt.Errorf("template body is required")
	}

	// Extract variables from body if not provided
	variables := req.Variables
	if len(variables) == 0 {
		variables = extractVariables(req.Body)
	}

	// If setting as default, clear existing default
	if req.IsDefault {
		if err := s.repo.ClearDefault(ctx, userID, req.Category); err != nil {
			return nil, err
		}
	}

	entity := &out.TemplateEntity{
		UserID:    userID,
		Name:      req.Name,
		Category:  req.Category,
		Body:      req.Body,
		Tags:      req.Tags,
		IsDefault: req.IsDefault,
	}

	if req.Subject != nil {
		entity.Subject = *req.Subject
	}
	if req.HTMLBody != nil {
		entity.HTMLBody = *req.HTMLBody
	}

	// Convert variables
	entity.Variables = make([]out.TemplateVariableEntity, len(variables))
	for i, v := range variables {
		entity.Variables[i] = out.TemplateVariableEntity{
			Name:        v.Name,
			Placeholder: v.Placeholder,
			DefaultVal:  v.DefaultVal,
			Description: v.Description,
		}
	}

	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entityToDomain(entity), nil
}

// Update updates a template
func (s *TemplateService) Update(ctx context.Context, userID uuid.UUID, req *UpdateTemplateRequest) (*domain.EmailTemplate, error) {
	// Get existing template
	existing, err := s.repo.GetByID(ctx, userID, req.ID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Category != nil {
		if !domain.IsValidTemplateCategory(*req.Category) {
			return nil, fmt.Errorf("invalid category")
		}
		existing.Category = *req.Category
	}
	if req.Subject != nil {
		existing.Subject = *req.Subject
	}
	if req.Body != nil {
		existing.Body = *req.Body
	}
	if req.HTMLBody != nil {
		existing.HTMLBody = *req.HTMLBody
	}
	if req.Tags != nil {
		existing.Tags = *req.Tags
	}
	if req.Variables != nil {
		existing.Variables = make([]out.TemplateVariableEntity, len(*req.Variables))
		for i, v := range *req.Variables {
			existing.Variables[i] = out.TemplateVariableEntity{
				Name:        v.Name,
				Placeholder: v.Placeholder,
				DefaultVal:  v.DefaultVal,
				Description: v.Description,
			}
		}
	}
	if req.IsDefault != nil {
		if *req.IsDefault && !existing.IsDefault {
			// Setting as new default
			if err := s.repo.ClearDefault(ctx, userID, existing.Category); err != nil {
				return nil, err
			}
		}
		existing.IsDefault = *req.IsDefault
	}

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	return entityToDomain(existing), nil
}

// Delete deletes a template
func (s *TemplateService) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}

// GetByID retrieves a template by ID
func (s *TemplateService) GetByID(ctx context.Context, userID uuid.UUID, id int64) (*domain.EmailTemplate, error) {
	entity, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return entityToDomain(entity), nil
}

// List lists templates with filters
func (s *TemplateService) List(ctx context.Context, userID uuid.UUID, req *TemplateListRequest) ([]*domain.EmailTemplate, int, error) {
	query := &out.TemplateListQuery{
		Category:   req.Category,
		Search:     req.Search,
		Tags:       req.Tags,
		IsDefault:  req.IsDefault,
		IsArchived: req.IsArchived,
		Limit:      req.Limit,
		Offset:     req.Offset,
		OrderBy:    req.OrderBy,
		Order:      req.Order,
	}

	entities, total, err := s.repo.List(ctx, userID, query)
	if err != nil {
		return nil, 0, err
	}

	templates := make([]*domain.EmailTemplate, len(entities))
	for i, e := range entities {
		templates[i] = entityToDomain(e)
	}

	return templates, total, nil
}

// GetDefault retrieves the default template for a category
func (s *TemplateService) GetDefault(ctx context.Context, userID uuid.UUID, category string) (*domain.EmailTemplate, error) {
	entity, err := s.repo.GetDefault(ctx, userID, category)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	return entityToDomain(entity), nil
}

// GetByCategory retrieves all templates for a category
func (s *TemplateService) GetByCategory(ctx context.Context, userID uuid.UUID, category string) ([]*domain.EmailTemplate, error) {
	entities, err := s.repo.GetByCategory(ctx, userID, category)
	if err != nil {
		return nil, err
	}

	templates := make([]*domain.EmailTemplate, len(entities))
	for i, e := range entities {
		templates[i] = entityToDomain(e)
	}

	return templates, nil
}

// SetDefault sets a template as the default for its category
func (s *TemplateService) SetDefault(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.SetDefault(ctx, userID, id)
}

// Archive archives a template
func (s *TemplateService) Archive(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.Archive(ctx, userID, id)
}

// Restore restores an archived template
func (s *TemplateService) Restore(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.Restore(ctx, userID, id)
}

// UseTemplate increments usage count and returns the rendered template
func (s *TemplateService) UseTemplate(ctx context.Context, userID uuid.UUID, id int64, variables map[string]string) (*RenderedTemplate, error) {
	entity, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	// Increment usage
	_ = s.repo.IncrementUsage(ctx, id)

	// Render template with variables
	rendered := &RenderedTemplate{
		Subject:  renderText(entity.Subject, variables),
		Body:     renderText(entity.Body, variables),
		HTMLBody: renderText(entity.HTMLBody, variables),
	}

	return rendered, nil
}

// RenderedTemplate represents a template with variables applied
type RenderedTemplate struct {
	Subject  string
	Body     string
	HTMLBody string
}

// DeleteBatch deletes multiple templates
func (s *TemplateService) DeleteBatch(ctx context.Context, userID uuid.UUID, ids []int64) error {
	return s.repo.DeleteBatch(ctx, userID, ids)
}

// Helper functions

// entityToDomain converts entity to domain model
func entityToDomain(e *out.TemplateEntity) *domain.EmailTemplate {
	template := &domain.EmailTemplate{
		ID:         e.ID,
		UserID:     e.UserID,
		Name:       e.Name,
		Category:   domain.TemplateCategory(e.Category),
		Body:       e.Body,
		Tags:       e.Tags,
		IsDefault:  e.IsDefault,
		IsArchived: e.IsArchived,
		UsageCount: e.UsageCount,
		LastUsedAt: e.LastUsedAt,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}

	if e.Subject != "" {
		template.Subject = &e.Subject
	}
	if e.HTMLBody != "" {
		template.HTMLBody = &e.HTMLBody
	}

	// Convert variables
	template.Variables = make([]domain.TemplateVariable, len(e.Variables))
	for i, v := range e.Variables {
		template.Variables[i] = domain.TemplateVariable{
			Name:        v.Name,
			Placeholder: v.Placeholder,
			DefaultVal:  v.DefaultVal,
			Description: v.Description,
		}
	}

	return template
}

// extractVariables extracts variable placeholders from text
func extractVariables(text string) []domain.TemplateVariable {
	// Match ${variableName} pattern
	re := regexp.MustCompile(`\$\{(\w+)\}`)
	matches := re.FindAllStringSubmatch(text, -1)

	// Use map to deduplicate
	seen := make(map[string]bool)
	var variables []domain.TemplateVariable

	for _, match := range matches {
		if len(match) >= 2 && !seen[match[1]] {
			seen[match[1]] = true
			variables = append(variables, domain.TemplateVariable{
				Name:        match[1],
				Placeholder: match[0],
			})
		}
	}

	return variables
}

// renderText replaces variables in text
func renderText(text string, variables map[string]string) string {
	if text == "" || len(variables) == 0 {
		return text
	}

	result := text
	for name, value := range variables {
		placeholder := "${" + name + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Handle date/time variables
	now := time.Now()
	result = strings.ReplaceAll(result, "${date}", now.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "${time}", now.Format("15:04"))
	result = strings.ReplaceAll(result, "${datetime}", now.Format("2006-01-02 15:04"))

	return result
}
