package http

import (
	"strconv"

	"worker_server/core/domain"

	"github.com/gofiber/fiber/v2"
)

// LabelHandler handles label-related HTTP requests.
type LabelHandler struct {
	labelRepo domain.LabelRepository
}

// NewLabelHandler creates a new label handler.
func NewLabelHandler(labelRepo domain.LabelRepository) *LabelHandler {
	return &LabelHandler{
		labelRepo: labelRepo,
	}
}

// Register registers label routes.
func (h *LabelHandler) Register(router fiber.Router) {
	labels := router.Group("/labels")

	// Label CRUD
	labels.Get("/", h.ListLabels)
	labels.Get("/:id", h.GetLabel)
	labels.Post("/", h.CreateLabel)
	labels.Put("/:id", h.UpdateLabel)
	labels.Delete("/:id", h.DeleteLabel)

	// Email-Label operations
	labels.Post("/:id/emails/:emailId", h.AddLabelToEmail)
	labels.Delete("/:id/emails/:emailId", h.RemoveLabelFromEmail)
	labels.Get("/emails/:emailId", h.GetEmailLabels)
}

// =============================================================================
// Label CRUD
// =============================================================================

// ListLabels returns all labels for the current user.
// GET /api/labels
func (h *LabelHandler) ListLabels(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labels, err := h.labelRepo.ListByUser(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"labels": labels,
		"total":  len(labels),
	})
}

// GetLabel returns a specific label by ID.
// GET /api/labels/:id
func (h *LabelHandler) GetLabel(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labelID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid label ID")
	}

	label, err := h.labelRepo.GetByID(labelID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Label not found")
	}

	// Verify ownership
	if label.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "Access denied")
	}

	return c.JSON(label)
}

// CreateLabelRequest represents the request body for creating a label.
type CreateLabelRequest struct {
	Name  string  `json:"name" validate:"required"`
	Color *string `json:"color,omitempty"`
}

// CreateLabel creates a new label.
// POST /api/labels
func (h *LabelHandler) CreateLabel(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	var req CreateLabelRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Label name is required")
	}

	label := &domain.Label{
		UserID:    userID,
		Name:      req.Name,
		Color:     req.Color,
		IsSystem:  false,
		IsVisible: true,
	}

	if err := h.labelRepo.Create(label); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(label)
}

// UpdateLabelRequest represents the request body for updating a label.
type UpdateLabelRequest struct {
	Name      *string `json:"name,omitempty"`
	Color     *string `json:"color,omitempty"`
	IsVisible *bool   `json:"is_visible,omitempty"`
}

// UpdateLabel updates an existing label.
// PUT /api/labels/:id
func (h *LabelHandler) UpdateLabel(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labelID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid label ID")
	}

	// Get existing label
	label, err := h.labelRepo.GetByID(labelID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Label not found")
	}

	// Verify ownership
	if label.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "Access denied")
	}

	// Cannot modify system labels
	if label.IsSystem {
		return fiber.NewError(fiber.StatusForbidden, "Cannot modify system labels")
	}

	var req UpdateLabelRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Apply updates
	if req.Name != nil {
		label.Name = *req.Name
	}
	if req.Color != nil {
		label.Color = req.Color
	}
	if req.IsVisible != nil {
		label.IsVisible = *req.IsVisible
	}

	if err := h.labelRepo.Update(label); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(label)
}

// DeleteLabel deletes a label.
// DELETE /api/labels/:id
func (h *LabelHandler) DeleteLabel(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labelID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid label ID")
	}

	// Get existing label to verify ownership
	label, err := h.labelRepo.GetByID(labelID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Label not found")
	}

	// Verify ownership
	if label.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "Access denied")
	}

	// Cannot delete system labels
	if label.IsSystem {
		return fiber.NewError(fiber.StatusForbidden, "Cannot delete system labels")
	}

	if err := h.labelRepo.Delete(labelID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// =============================================================================
// Email-Label Operations
// =============================================================================

// AddLabelToEmail adds a label to an email.
// POST /api/labels/:id/emails/:emailId
func (h *LabelHandler) AddLabelToEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labelID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid label ID")
	}

	emailID, err := strconv.ParseInt(c.Params("emailId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid email ID")
	}

	// Verify label ownership
	label, err := h.labelRepo.GetByID(labelID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Label not found")
	}

	if label.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "Access denied")
	}

	// Add label to email
	if err := h.labelRepo.AddEmailLabel(emailID, labelID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"message":  "Label added to email",
		"email_id": emailID,
		"label_id": labelID,
	})
}

// RemoveLabelFromEmail removes a label from an email.
// DELETE /api/labels/:id/emails/:emailId
func (h *LabelHandler) RemoveLabelFromEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	labelID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid label ID")
	}

	emailID, err := strconv.ParseInt(c.Params("emailId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid email ID")
	}

	// Verify label ownership
	label, err := h.labelRepo.GetByID(labelID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Label not found")
	}

	if label.UserID != userID {
		return fiber.NewError(fiber.StatusForbidden, "Access denied")
	}

	// Remove label from email
	if err := h.labelRepo.RemoveEmailLabel(emailID, labelID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"message":  "Label removed from email",
		"email_id": emailID,
		"label_id": labelID,
	})
}

// GetEmailLabels returns all labels for a specific email.
// GET /api/labels/emails/:emailId
func (h *LabelHandler) GetEmailLabels(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.labelRepo == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Label service not available")
	}

	emailID, err := strconv.ParseInt(c.Params("emailId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid email ID")
	}

	labels, err := h.labelRepo.GetEmailLabels(emailID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"labels":   labels,
		"email_id": emailID,
		"total":    len(labels),
	})
}
