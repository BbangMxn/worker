package http

import (
	"strconv"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/service"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TemplateHandler handles HTTP requests for email templates
type TemplateHandler struct {
	service *service.TemplateService
}

// NewTemplateHandler creates a new TemplateHandler
func NewTemplateHandler(service *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{service: service}
}

// Register registers template routes
func (h *TemplateHandler) Register(router fiber.Router) {
	templates := router.Group("/templates")

	templates.Get("/", h.List)
	templates.Get("/categories", h.GetCategories)
	templates.Get("/variables", h.GetDefaultVariables)
	templates.Get("/category/:category", h.GetByCategory)
	templates.Get("/default/:category", h.GetDefault)
	templates.Get("/:id", h.GetByID)
	templates.Post("/", h.Create)
	templates.Put("/:id", h.Update)
	templates.Delete("/:id", h.Delete)
	templates.Post("/:id/use", h.UseTemplate)
	templates.Post("/:id/default", h.SetDefault)
	templates.Post("/:id/archive", h.Archive)
	templates.Post("/:id/restore", h.Restore)
	templates.Delete("/batch", h.DeleteBatch)
}

// CreateTemplateRequest represents the HTTP request to create a template
type CreateTemplateRequest struct {
	Name      string                    `json:"name"`
	Category  string                    `json:"category"`
	Subject   *string                   `json:"subject"`
	Body      string                    `json:"body"`
	HTMLBody  *string                   `json:"html_body"`
	Variables []domain.TemplateVariable `json:"variables"`
	Tags      []string                  `json:"tags"`
	IsDefault bool                      `json:"is_default"`
}

// UpdateTemplateRequest represents the HTTP request to update a template
type UpdateTemplateRequest struct {
	Name      *string                    `json:"name"`
	Category  *string                    `json:"category"`
	Subject   *string                    `json:"subject"`
	Body      *string                    `json:"body"`
	HTMLBody  *string                    `json:"html_body"`
	Variables *[]domain.TemplateVariable `json:"variables"`
	Tags      *[]string                  `json:"tags"`
	IsDefault *bool                      `json:"is_default"`
}

// UseTemplateRequest represents the HTTP request to use a template
type UseTemplateRequest struct {
	Variables map[string]string `json:"variables"`
}

// TemplateResponse represents the HTTP response for a template
type TemplateResponse struct {
	ID         int64                     `json:"id"`
	Name       string                    `json:"name"`
	Category   string                    `json:"category"`
	Subject    *string                   `json:"subject,omitempty"`
	Body       string                    `json:"body"`
	HTMLBody   *string                   `json:"html_body,omitempty"`
	Variables  []domain.TemplateVariable `json:"variables"`
	Tags       []string                  `json:"tags"`
	IsDefault  bool                      `json:"is_default"`
	IsArchived bool                      `json:"is_archived"`
	UsageCount int                       `json:"usage_count"`
	LastUsedAt *string                   `json:"last_used_at,omitempty"`
	CreatedAt  string                    `json:"created_at"`
	UpdatedAt  string                    `json:"updated_at"`
}

// TemplateListItemResponse represents a lightweight template response
type TemplateListItemResponse struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Subject    *string  `json:"subject,omitempty"`
	Preview    string   `json:"preview"`
	Tags       []string `json:"tags"`
	IsDefault  bool     `json:"is_default"`
	UsageCount int      `json:"usage_count"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	UpdatedAt  string   `json:"updated_at"`
}

// Create creates a new template
// @Summary Create a new email template
// @Tags Templates
// @Accept json
// @Produce json
// @Param request body CreateTemplateRequest true "Template data"
// @Success 201 {object} TemplateResponse
// @Router /api/v1/templates [post]
func (h *TemplateHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req CreateTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	template, err := h.service.Create(c.Context(), userID, &service.CreateTemplateRequest{
		Name:      req.Name,
		Category:  req.Category,
		Subject:   req.Subject,
		Body:      req.Body,
		HTMLBody:  req.HTMLBody,
		Variables: req.Variables,
		Tags:      req.Tags,
		IsDefault: req.IsDefault,
	})
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTemplateResponse(template))
}

// Update updates a template
// @Summary Update an email template
// @Tags Templates
// @Accept json
// @Produce json
// @Param id path int true "Template ID"
// @Param request body UpdateTemplateRequest true "Template data"
// @Success 200 {object} TemplateResponse
// @Router /api/v1/templates/{id} [put]
func (h *TemplateHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	var req UpdateTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	template, err := h.service.Update(c.Context(), userID, &service.UpdateTemplateRequest{
		ID:        id,
		Name:      req.Name,
		Category:  req.Category,
		Subject:   req.Subject,
		Body:      req.Body,
		HTMLBody:  req.HTMLBody,
		Variables: req.Variables,
		Tags:      req.Tags,
		IsDefault: req.IsDefault,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "template not found"})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toTemplateResponse(template))
}

// Delete deletes a template
// @Summary Delete an email template
// @Tags Templates
// @Param id path int true "Template ID"
// @Success 204
// @Router /api/v1/templates/{id} [delete]
func (h *TemplateHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	if err := h.service.Delete(c.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "template not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// GetByID retrieves a template by ID
// @Summary Get an email template by ID
// @Tags Templates
// @Produce json
// @Param id path int true "Template ID"
// @Success 200 {object} TemplateResponse
// @Router /api/v1/templates/{id} [get]
func (h *TemplateHandler) GetByID(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	template, err := h.service.GetByID(c.Context(), userID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "template not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toTemplateResponse(template))
}

// List lists templates with filters
// @Summary List email templates
// @Tags Templates
// @Produce json
// @Param category query string false "Filter by category"
// @Param search query string false "Search in name and body"
// @Param tags query string false "Filter by tags (comma-separated)"
// @Param is_default query bool false "Filter by default status"
// @Param is_archived query bool false "Include archived templates"
// @Param limit query int false "Limit (default 50)"
// @Param offset query int false "Offset"
// @Param order_by query string false "Order by (name, category, usage_count, updated_at)"
// @Param order query string false "Order direction (asc, desc)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/templates [get]
func (h *TemplateHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	req := &service.TemplateListRequest{
		Limit:   c.QueryInt("limit", 50),
		Offset:  c.QueryInt("offset", 0),
		OrderBy: c.Query("order_by", "updated_at"),
		Order:   c.Query("order", "desc"),
	}

	if category := c.Query("category"); category != "" {
		req.Category = &category
	}
	if search := c.Query("search"); search != "" {
		req.Search = &search
	}
	if tagsStr := c.Query("tags"); tagsStr != "" {
		req.Tags = strings.Split(tagsStr, ",")
	}
	if isDefault := c.Query("is_default"); isDefault != "" {
		val := isDefault == "true"
		req.IsDefault = &val
	}
	if isArchived := c.Query("is_archived"); isArchived != "" {
		val := isArchived == "true"
		req.IsArchived = &val
	}

	templates, total, err := h.service.List(c.Context(), userID, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]TemplateListItemResponse, len(templates))
	for i, t := range templates {
		items[i] = toTemplateListItemResponse(t)
	}

	return c.JSON(fiber.Map{
		"templates": items,
		"total":     total,
		"limit":     req.Limit,
		"offset":    req.Offset,
	})
}

// GetByCategory retrieves all templates for a category
// @Summary Get templates by category
// @Tags Templates
// @Produce json
// @Param category path string true "Category"
// @Success 200 {array} TemplateListItemResponse
// @Router /api/v1/templates/category/{category} [get]
func (h *TemplateHandler) GetByCategory(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)
	category := c.Params("category")

	templates, err := h.service.GetByCategory(c.Context(), userID, category)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]TemplateListItemResponse, len(templates))
	for i, t := range templates {
		items[i] = toTemplateListItemResponse(t)
	}

	return c.JSON(fiber.Map{"templates": items})
}

// GetDefault retrieves the default template for a category
// @Summary Get default template for a category
// @Tags Templates
// @Produce json
// @Param category path string true "Category"
// @Success 200 {object} TemplateResponse
// @Router /api/v1/templates/default/{category} [get]
func (h *TemplateHandler) GetDefault(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)
	category := c.Params("category")

	template, err := h.service.GetDefault(c.Context(), userID, category)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if template == nil {
		return c.Status(404).JSON(fiber.Map{"error": "no default template for this category"})
	}

	return c.JSON(toTemplateResponse(template))
}

// SetDefault sets a template as the default for its category
// @Summary Set template as default
// @Tags Templates
// @Param id path int true "Template ID"
// @Success 200
// @Router /api/v1/templates/{id}/default [post]
func (h *TemplateHandler) SetDefault(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	if err := h.service.SetDefault(c.Context(), userID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "template set as default"})
}

// Archive archives a template
// @Summary Archive a template
// @Tags Templates
// @Param id path int true "Template ID"
// @Success 200
// @Router /api/v1/templates/{id}/archive [post]
func (h *TemplateHandler) Archive(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	if err := h.service.Archive(c.Context(), userID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "template archived"})
}

// Restore restores an archived template
// @Summary Restore an archived template
// @Tags Templates
// @Param id path int true "Template ID"
// @Success 200
// @Router /api/v1/templates/{id}/restore [post]
func (h *TemplateHandler) Restore(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	if err := h.service.Restore(c.Context(), userID, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "template restored"})
}

// UseTemplate uses a template with variable substitution
// @Summary Use a template with variables
// @Tags Templates
// @Accept json
// @Produce json
// @Param id path int true "Template ID"
// @Param request body UseTemplateRequest true "Variables"
// @Success 200 {object} map[string]string
// @Router /api/v1/templates/{id}/use [post]
func (h *TemplateHandler) UseTemplate(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid template ID"})
	}

	var req UseTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		req.Variables = make(map[string]string)
	}

	rendered, err := h.service.UseTemplate(c.Context(), userID, id, req.Variables)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "template not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"subject":   rendered.Subject,
		"body":      rendered.Body,
		"html_body": rendered.HTMLBody,
	})
}

// DeleteBatch deletes multiple templates
// @Summary Delete multiple templates
// @Tags Templates
// @Accept json
// @Param request body map[string][]int64 true "Template IDs"
// @Success 204
// @Router /api/v1/templates/batch [delete]
func (h *TemplateHandler) DeleteBatch(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.service.DeleteBatch(c.Context(), userID, req.IDs); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// GetCategories returns all available template categories
// @Summary Get template categories
// @Tags Templates
// @Produce json
// @Success 200 {array} string
// @Router /api/v1/templates/categories [get]
func (h *TemplateHandler) GetCategories(c *fiber.Ctx) error {
	categories := domain.ValidTemplateCategories()
	result := make([]string, len(categories))
	for i, cat := range categories {
		result[i] = string(cat)
	}
	return c.JSON(fiber.Map{"categories": result})
}

// GetDefaultVariables returns default template variables
// @Summary Get default template variables
// @Tags Templates
// @Produce json
// @Success 200 {array} domain.TemplateVariable
// @Router /api/v1/templates/variables [get]
func (h *TemplateHandler) GetDefaultVariables(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"variables": domain.DefaultVariables()})
}

// Helper functions

func toTemplateResponse(t *domain.EmailTemplate) TemplateResponse {
	resp := TemplateResponse{
		ID:         t.ID,
		Name:       t.Name,
		Category:   string(t.Category),
		Subject:    t.Subject,
		Body:       t.Body,
		HTMLBody:   t.HTMLBody,
		Variables:  t.Variables,
		Tags:       t.Tags,
		IsDefault:  t.IsDefault,
		IsArchived: t.IsArchived,
		UsageCount: t.UsageCount,
		CreatedAt:  t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if t.LastUsedAt != nil {
		formatted := t.LastUsedAt.Format("2006-01-02T15:04:05Z")
		resp.LastUsedAt = &formatted
	}

	if resp.Variables == nil {
		resp.Variables = []domain.TemplateVariable{}
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}

	return resp
}

func toTemplateListItemResponse(t *domain.EmailTemplate) TemplateListItemResponse {
	listItem := t.ToListItem()

	resp := TemplateListItemResponse{
		ID:         listItem.ID,
		Name:       listItem.Name,
		Category:   string(listItem.Category),
		Subject:    listItem.Subject,
		Preview:    listItem.Preview,
		Tags:       listItem.Tags,
		IsDefault:  listItem.IsDefault,
		UsageCount: listItem.UsageCount,
		UpdatedAt:  listItem.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if listItem.LastUsedAt != nil {
		formatted := listItem.LastUsedAt.Format("2006-01-02T15:04:05Z")
		resp.LastUsedAt = &formatted
	}

	if resp.Tags == nil {
		resp.Tags = []string{}
	}

	return resp
}
