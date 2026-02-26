package http

import (
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

// ShortcutHandler handles keyboard shortcut API requests
type ShortcutHandler struct {
	repo out.ShortcutRepository
}

// NewShortcutHandler creates a new ShortcutHandler
func NewShortcutHandler(repo out.ShortcutRepository) *ShortcutHandler {
	return &ShortcutHandler{repo: repo}
}

// GetShortcuts returns user's keyboard shortcut settings
// GET /api/shortcuts
func (h *ShortcutHandler) GetShortcuts(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	shortcuts, err := h.repo.Get(c.Context(), userID)
	if err != nil {
		logger.Error("[ShortcutHandler] Failed to get shortcuts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get shortcuts",
		})
	}

	// Create default shortcuts if not found (lazy initialization)
	if shortcuts == nil {
		shortcuts = &domain.KeyboardShortcuts{
			UserID:    userID,
			Preset:    domain.PresetSuperhuman,
			Enabled:   true,
			ShowHints: true,
			Shortcuts: make(map[string]string), // Empty - will use defaults from preset
		}
		// Save default settings to DB
		if err := h.repo.Upsert(c.Context(), shortcuts); err != nil {
			logger.Warn("[ShortcutHandler] Failed to save default shortcuts: %v", err)
			// Continue anyway - return defaults
		}
	}

	// Always merge with defaults (user overrides take precedence)
	{
		// Merge with defaults (user overrides take precedence)
		defaults := domain.GetDefaultShortcuts(shortcuts.Preset)
		merged := make(map[string]string)
		for k, v := range defaults {
			merged[k] = v
		}
		for k, v := range shortcuts.Shortcuts {
			merged[k] = v
		}
		shortcuts.Shortcuts = merged
	}

	return c.JSON(fiber.Map{
		"preset":     shortcuts.Preset,
		"enabled":    shortcuts.Enabled,
		"show_hints": shortcuts.ShowHints,
		"shortcuts":  shortcuts.Shortcuts,
	})
}

// UpdateShortcutsRequest represents the update request body
type UpdateShortcutsRequest struct {
	Preset    *string           `json:"preset"`
	Enabled   *bool             `json:"enabled"`
	ShowHints *bool             `json:"show_hints"`
	Shortcuts map[string]string `json:"shortcuts"`
}

// UpdateShortcuts updates user's keyboard shortcut settings
// PUT /api/shortcuts
func (h *ShortcutHandler) UpdateShortcuts(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	var req UpdateShortcutsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Get existing or create new
	shortcuts, err := h.repo.Get(c.Context(), userID)
	if err != nil {
		logger.Error("[ShortcutHandler] Failed to get shortcuts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get shortcuts",
		})
	}

	if shortcuts == nil {
		shortcuts = &domain.KeyboardShortcuts{
			UserID:    userID,
			Preset:    domain.PresetSuperhuman,
			Enabled:   true,
			ShowHints: true,
			Shortcuts: make(map[string]string),
		}
	}

	// Update fields if provided
	if req.Preset != nil {
		preset := domain.ShortcutPreset(*req.Preset)
		if preset != domain.PresetSuperhuman && preset != domain.PresetGmail && preset != domain.PresetCustom {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid preset: must be 'superhuman', 'gmail', or 'custom'",
			})
		}
		shortcuts.Preset = preset

		// If changing preset (not custom), reset shortcuts to new defaults
		if preset != domain.PresetCustom && req.Shortcuts == nil {
			shortcuts.Shortcuts = make(map[string]string) // Clear overrides
		}
	}
	if req.Enabled != nil {
		shortcuts.Enabled = *req.Enabled
	}
	if req.ShowHints != nil {
		shortcuts.ShowHints = *req.ShowHints
	}
	if req.Shortcuts != nil {
		// Validate shortcuts
		if conflicts := validateShortcuts(req.Shortcuts); len(conflicts) > 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":     "shortcut conflicts detected",
				"conflicts": conflicts,
			})
		}

		// Only store overrides (different from defaults)
		defaults := domain.GetDefaultShortcuts(shortcuts.Preset)
		overrides := make(map[string]string)
		for action, key := range req.Shortcuts {
			if defaultKey, exists := defaults[action]; !exists || defaultKey != key {
				overrides[action] = key
			}
		}
		shortcuts.Shortcuts = overrides

		// If user is customizing, set preset to custom
		if len(overrides) > 0 && shortcuts.Preset != domain.PresetCustom {
			shortcuts.Preset = domain.PresetCustom
		}
	}

	// Save
	if err := h.repo.Upsert(c.Context(), shortcuts); err != nil {
		logger.Error("[ShortcutHandler] Failed to save shortcuts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to save shortcuts",
		})
	}

	// Return merged shortcuts
	defaults := domain.GetDefaultShortcuts(shortcuts.Preset)
	merged := make(map[string]string)
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range shortcuts.Shortcuts {
		merged[k] = v
	}

	return c.JSON(fiber.Map{
		"preset":     shortcuts.Preset,
		"enabled":    shortcuts.Enabled,
		"show_hints": shortcuts.ShowHints,
		"shortcuts":  merged,
	})
}

// ResetShortcuts resets user's shortcuts to default
// POST /api/shortcuts/reset
func (h *ShortcutHandler) ResetShortcuts(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	// Delete existing settings
	if err := h.repo.Delete(c.Context(), userID); err != nil {
		logger.Error("[ShortcutHandler] Failed to reset shortcuts: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to reset shortcuts",
		})
	}

	// Return defaults
	return c.JSON(fiber.Map{
		"preset":     domain.PresetSuperhuman,
		"enabled":    true,
		"show_hints": true,
		"shortcuts":  domain.DefaultSuperhumanShortcuts(),
	})
}

// GetPresets returns available shortcut presets
// GET /api/shortcuts/presets
func (h *ShortcutHandler) GetPresets(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"presets": []fiber.Map{
			{
				"id":          "superhuman",
				"name":        "Superhuman",
				"description": "Fast, keyboard-first shortcuts inspired by Superhuman",
			},
			{
				"id":          "gmail",
				"name":        "Gmail",
				"description": "Familiar shortcuts for Gmail users",
			},
			{
				"id":          "custom",
				"name":        "Custom",
				"description": "Your personalized shortcut configuration",
			},
		},
	})
}

// GetDefinitions returns all shortcut definitions with descriptions
// GET /api/shortcuts/definitions
func (h *ShortcutHandler) GetDefinitions(c *fiber.Ctx) error {
	definitions := domain.GetShortcutDefinitions()

	// Group by category
	categories := make(map[string][]domain.ShortcutDefinition)
	for _, def := range definitions {
		categories[def.Category] = append(categories[def.Category], def)
	}

	return c.JSON(fiber.Map{
		"definitions": definitions,
		"categories":  categories,
	})
}

// ShortcutConflict represents a shortcut conflict
type ShortcutConflict struct {
	Key     string   `json:"key"`
	Actions []string `json:"actions"`
}

// Register registers shortcut routes (authenticated)
func (h *ShortcutHandler) Register(router fiber.Router) {
	shortcuts := router.Group("/shortcuts")
	shortcuts.Get("/", h.GetShortcuts)
	shortcuts.Put("/", h.UpdateShortcuts)
	shortcuts.Post("/reset", h.ResetShortcuts)
	// Note: presets and definitions are registered as public in RegisterPublic
}

// RegisterPublic registers public shortcut routes (no auth required)
func (h *ShortcutHandler) RegisterPublic(router fiber.Router) {
	shortcuts := router.Group("/shortcuts")
	shortcuts.Get("/presets", h.GetPresets)
	shortcuts.Get("/definitions", h.GetDefinitions)
	shortcuts.Get("/defaults", h.GetDefaults)
}

// GetDefaults returns default shortcuts for each preset (no auth required)
// GET /api/v1/shortcuts/defaults
func (h *ShortcutHandler) GetDefaults(c *fiber.Ctx) error {
	preset := c.Query("preset", "superhuman")

	var shortcuts map[string]string
	switch preset {
	case "gmail":
		shortcuts = domain.DefaultGmailShortcuts()
	default:
		shortcuts = domain.DefaultSuperhumanShortcuts()
	}

	return c.JSON(fiber.Map{
		"preset":     preset,
		"enabled":    true,
		"show_hints": true,
		"shortcuts":  shortcuts,
	})
}

// validateShortcuts checks for duplicate key bindings
func validateShortcuts(shortcuts map[string]string) []ShortcutConflict {
	keyToActions := make(map[string][]string)
	for action, key := range shortcuts {
		keyToActions[key] = append(keyToActions[key], action)
	}

	var conflicts []ShortcutConflict
	for key, actions := range keyToActions {
		if len(actions) > 1 {
			conflicts = append(conflicts, ShortcutConflict{
				Key:     key,
				Actions: actions,
			})
		}
	}
	return conflicts
}
