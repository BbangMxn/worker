package http

import (
	"strconv"

	"worker_server/core/domain"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SenderProfileHandler handles sender profile HTTP requests.
type SenderProfileHandler struct {
	senderProfileRepo domain.SenderProfileRepository
}

// NewSenderProfileHandler creates a new SenderProfileHandler.
func NewSenderProfileHandler(senderProfileRepo domain.SenderProfileRepository) *SenderProfileHandler {
	return &SenderProfileHandler{
		senderProfileRepo: senderProfileRepo,
	}
}

// RegisterRoutes registers sender profile routes.
func (h *SenderProfileHandler) RegisterRoutes(app fiber.Router) {
	senders := app.Group("/senders")
	senders.Get("/", h.ListSenderProfiles)
	senders.Get("/vip", h.ListVIPSenders)
	senders.Get("/muted", h.ListMutedSenders)
	senders.Get("/:id", h.GetSenderProfile)
	senders.Put("/:id", h.UpdateSenderProfile)
	senders.Put("/:id/vip", h.ToggleVIP)
	senders.Put("/:id/mute", h.ToggleMute)
	senders.Delete("/:id", h.DeleteSenderProfile)
}

// ListSenderProfiles returns all sender profiles for the current user.
func (h *SenderProfileHandler) ListSenderProfiles(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	profiles, err := h.senderProfileRepo.GetByUserID(userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"senders": profiles,
		"count":   len(profiles),
	})
}

// ListVIPSenders returns all VIP senders for the current user.
func (h *SenderProfileHandler) ListVIPSenders(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	profiles, err := h.senderProfileRepo.GetVIPSenders(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"senders": profiles,
		"count":   len(profiles),
	})
}

// ListMutedSenders returns all muted senders for the current user.
func (h *SenderProfileHandler) ListMutedSenders(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	profiles, err := h.senderProfileRepo.GetMutedSenders(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"senders": profiles,
		"count":   len(profiles),
	})
}

// GetSenderProfile returns a single sender profile by ID.
func (h *SenderProfileHandler) GetSenderProfile(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid sender ID"})
	}

	profile, err := h.senderProfileRepo.GetByID(id)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sender not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if profile.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(profile)
}

// UpdateSenderProfile updates a sender profile.
func (h *SenderProfileHandler) UpdateSenderProfile(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid sender ID"})
	}

	profile, err := h.senderProfileRepo.GetByID(id)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sender not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if profile.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req struct {
		LearnedCategory    *string `json:"learned_category,omitempty"`
		LearnedSubCategory *string `json:"learned_sub_category,omitempty"`
		IsVIP              *bool   `json:"is_vip,omitempty"`
		IsMuted            *bool   `json:"is_muted,omitempty"`
		DisplayName        *string `json:"display_name,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.LearnedCategory != nil {
		cat := domain.EmailCategory(*req.LearnedCategory)
		profile.LearnedCategory = &cat
	}
	if req.LearnedSubCategory != nil {
		subCat := domain.EmailSubCategory(*req.LearnedSubCategory)
		profile.LearnedSubCategory = &subCat
	}
	if req.IsVIP != nil {
		profile.IsVIP = *req.IsVIP
	}
	if req.IsMuted != nil {
		profile.IsMuted = *req.IsMuted
	}
	if req.DisplayName != nil {
		profile.DisplayName = req.DisplayName
	}

	if err := h.senderProfileRepo.Update(profile); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(profile)
}

// ToggleVIP toggles the VIP status of a sender.
func (h *SenderProfileHandler) ToggleVIP(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid sender ID"})
	}

	profile, err := h.senderProfileRepo.GetByID(id)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sender not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if profile.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	profile.IsVIP = !profile.IsVIP
	if profile.IsVIP {
		profile.IsMuted = false // VIP and muted are mutually exclusive
	}

	if err := h.senderProfileRepo.Update(profile); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"id":     profile.ID,
		"is_vip": profile.IsVIP,
	})
}

// ToggleMute toggles the mute status of a sender.
func (h *SenderProfileHandler) ToggleMute(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid sender ID"})
	}

	profile, err := h.senderProfileRepo.GetByID(id)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sender not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if profile.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	profile.IsMuted = !profile.IsMuted
	if profile.IsMuted {
		profile.IsVIP = false // VIP and muted are mutually exclusive
	}

	if err := h.senderProfileRepo.Update(profile); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"id":       profile.ID,
		"is_muted": profile.IsMuted,
	})
}

// DeleteSenderProfile deletes a sender profile.
func (h *SenderProfileHandler) DeleteSenderProfile(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid sender ID"})
	}

	profile, err := h.senderProfileRepo.GetByID(id)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "sender not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if profile.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	if err := h.senderProfileRepo.Delete(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
