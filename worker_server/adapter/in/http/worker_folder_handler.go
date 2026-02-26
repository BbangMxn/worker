package http

import (
	"strconv"

	"worker_server/core/domain"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// FolderHandler handles folder-related HTTP requests.
type FolderHandler struct {
	folderRepo      domain.FolderRepository
	smartFolderRepo domain.SmartFolderRepository
}

// NewFolderHandler creates a new FolderHandler.
func NewFolderHandler(folderRepo domain.FolderRepository, smartFolderRepo domain.SmartFolderRepository) *FolderHandler {
	return &FolderHandler{
		folderRepo:      folderRepo,
		smartFolderRepo: smartFolderRepo,
	}
}

// RegisterRoutes registers folder routes.
func (h *FolderHandler) RegisterRoutes(app fiber.Router) {
	folders := app.Group("/folders")
	folders.Get("/", h.ListFolders)
	folders.Post("/", h.CreateFolder)
	folders.Get("/:id", h.GetFolder)
	folders.Put("/:id", h.UpdateFolder)
	folders.Delete("/:id", h.DeleteFolder)

	smartFolders := app.Group("/smart-folders")
	smartFolders.Get("/", h.ListSmartFolders)
	smartFolders.Post("/", h.CreateSmartFolder)
	smartFolders.Get("/:id", h.GetSmartFolder)
	smartFolders.Put("/:id", h.UpdateSmartFolder)
	smartFolders.Delete("/:id", h.DeleteSmartFolder)
	smartFolders.Get("/:id/count", h.GetSmartFolderCount)
}

// ListFolders returns all folders for the current user.
func (h *FolderHandler) ListFolders(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	folders, err := h.folderRepo.GetByUserID(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"folders": folders,
		"count":   len(folders),
	})
}

// CreateFolder creates a new folder.
func (h *FolderHandler) CreateFolder(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Name     string  `json:"name"`
		Color    *string `json:"color,omitempty"`
		Icon     *string `json:"icon,omitempty"`
		Position int     `json:"position"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	folder := &domain.EmailFolder{
		UserID:   userID,
		Name:     req.Name,
		Type:     domain.FolderTypeUser,
		Color:    req.Color,
		Icon:     req.Icon,
		Position: req.Position,
	}

	if err := h.folderRepo.Create(folder); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(folder)
}

// GetFolder returns a single folder by ID.
func (h *FolderHandler) GetFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid folder ID"})
	}

	folder, err := h.folderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(folder)
}

// UpdateFolder updates a folder.
func (h *FolderHandler) UpdateFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid folder ID"})
	}

	folder, err := h.folderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req struct {
		Name     *string `json:"name,omitempty"`
		Color    *string `json:"color,omitempty"`
		Icon     *string `json:"icon,omitempty"`
		Position *int    `json:"position,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name != nil {
		folder.Name = *req.Name
	}
	if req.Color != nil {
		folder.Color = req.Color
	}
	if req.Icon != nil {
		folder.Icon = req.Icon
	}
	if req.Position != nil {
		folder.Position = *req.Position
	}

	if err := h.folderRepo.Update(folder); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(folder)
}

// DeleteFolder deletes a folder.
func (h *FolderHandler) DeleteFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid folder ID"})
	}

	folder, err := h.folderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	// Cannot delete system folders
	if folder.IsSystem() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot delete system folder"})
	}

	if err := h.folderRepo.Delete(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ListSmartFolders returns all smart folders for the current user.
func (h *FolderHandler) ListSmartFolders(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	folders, err := h.smartFolderRepo.GetByUserID(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"smart_folders": folders,
		"count":         len(folders),
	})
}

// CreateSmartFolder creates a new smart folder.
func (h *FolderHandler) CreateSmartFolder(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Name     string                  `json:"name"`
		Icon     *string                 `json:"icon,omitempty"`
		Color    *string                 `json:"color,omitempty"`
		Query    domain.SmartFolderQuery `json:"query"`
		Position int                     `json:"position"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	folder := &domain.SmartFolder{
		UserID:   userID,
		Name:     req.Name,
		Icon:     req.Icon,
		Color:    req.Color,
		Query:    req.Query,
		IsSystem: false,
		Position: req.Position,
	}

	if err := h.smartFolderRepo.Create(folder); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(folder)
}

// GetSmartFolder returns a single smart folder by ID.
func (h *FolderHandler) GetSmartFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid smart folder ID"})
	}

	folder, err := h.smartFolderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "smart folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	return c.JSON(folder)
}

// UpdateSmartFolder updates a smart folder.
func (h *FolderHandler) UpdateSmartFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid smart folder ID"})
	}

	folder, err := h.smartFolderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "smart folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	var req struct {
		Name     *string                  `json:"name,omitempty"`
		Icon     *string                  `json:"icon,omitempty"`
		Color    *string                  `json:"color,omitempty"`
		Query    *domain.SmartFolderQuery `json:"query,omitempty"`
		Position *int                     `json:"position,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name != nil {
		folder.Name = *req.Name
	}
	if req.Icon != nil {
		folder.Icon = req.Icon
	}
	if req.Color != nil {
		folder.Color = req.Color
	}
	if req.Query != nil {
		folder.Query = *req.Query
	}
	if req.Position != nil {
		folder.Position = *req.Position
	}

	if err := h.smartFolderRepo.Update(folder); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(folder)
}

// DeleteSmartFolder deletes a smart folder.
func (h *FolderHandler) DeleteSmartFolder(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid smart folder ID"})
	}

	folder, err := h.smartFolderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "smart folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	// Cannot delete system smart folders
	if folder.IsSystem {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot delete system smart folder"})
	}

	if err := h.smartFolderRepo.Delete(id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// GetSmartFolderCount returns the email count for a smart folder.
func (h *FolderHandler) GetSmartFolderCount(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid smart folder ID"})
	}

	folder, err := h.smartFolderRepo.GetByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "smart folder not found"})
	}

	// Check ownership
	userID, _ := c.Locals("user_id").(uuid.UUID)
	if folder.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	total, unread, err := h.smartFolderRepo.CountEmails(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"total":  total,
		"unread": unread,
	})
}
