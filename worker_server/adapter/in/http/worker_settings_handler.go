package http

import (
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/pkg/logger"
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	settingsCacheTTL = 5 * time.Minute
	settingsCacheKey = "settings:%s"
	allSettingsKey   = "settings:all:%s"
)

// SettingsHandler handles user settings requests.
type SettingsHandler struct {
	settingsService *auth.SettingsService
	shortcutRepo    out.ShortcutRepository
	redis           *redis.Client
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(settingsService *auth.SettingsService) *SettingsHandler {
	return &SettingsHandler{
		settingsService: settingsService,
	}
}

// NewSettingsHandlerFull creates a settings handler with all dependencies.
func NewSettingsHandlerFull(settingsService *auth.SettingsService, shortcutRepo out.ShortcutRepository, redisClient *redis.Client) *SettingsHandler {
	return &SettingsHandler{
		settingsService: settingsService,
		shortcutRepo:    shortcutRepo,
		redis:           redisClient,
	}
}

// Register registers settings routes.
func (h *SettingsHandler) Register(router fiber.Router) {
	settings := router.Group("/settings")

	// Batch endpoint - get all settings in one request
	settings.Get("/all", h.GetAllSettings)

	// User settings
	settings.Get("/", h.GetSettings)
	settings.Put("/", h.UpdateSettings)
	settings.Patch("/", h.UpdateSettings)

	// AI preferences
	settings.Get("/ai", h.GetAISettings)
	settings.Put("/ai", h.UpdateAISettings)

	// Classification rules
	settings.Get("/classification-rules", h.GetClassificationRules)
	settings.Put("/classification-rules", h.UpdateClassificationRules)
}

// =============================================================================
// Batch Settings (All in one)
// =============================================================================

// AllSettingsResponse combines all settings for a single request.
type AllSettingsResponse struct {
	Settings            *domain.UserSettings        `json:"settings"`
	Shortcuts           *ShortcutSettingsResponse   `json:"shortcuts"`
	ClassificationRules *domain.ClassificationRules `json:"classification_rules"`
}

// ShortcutSettingsResponse for shortcuts in batch response.
type ShortcutSettingsResponse struct {
	Preset    string            `json:"preset"`
	Enabled   bool              `json:"enabled"`
	ShowHints bool              `json:"show_hints"`
	Shortcuts map[string]string `json:"shortcuts"`
}

// GetAllSettings returns all settings in a single response (optimized for settings page).
// GET /api/v1/settings/all
func (h *SettingsHandler) GetAllSettings(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Try cache first
	if h.redis != nil {
		cacheKey := fmt.Sprintf(allSettingsKey, userID.String())
		cached, err := h.redis.Get(c.Context(), cacheKey).Result()
		if err == nil && cached != "" {
			var response AllSettingsResponse
			if json.Unmarshal([]byte(cached), &response) == nil {
				logger.Debug("[Settings] Cache hit for user %s", userID)
				return c.JSON(response)
			}
		}
	}

	// Fetch all settings concurrently
	response := &AllSettingsResponse{}

	// 1. User settings
	if h.settingsService != nil {
		settings, err := h.settingsService.GetSettings(c.Context(), userID)
		if err == nil {
			response.Settings = settings
		}
	}

	// 2. Shortcuts
	if h.shortcutRepo != nil {
		shortcuts, _ := h.shortcutRepo.Get(c.Context(), userID)
		if shortcuts == nil {
			response.Shortcuts = &ShortcutSettingsResponse{
				Preset:    string(domain.PresetSuperhuman),
				Enabled:   true,
				ShowHints: true,
				Shortcuts: domain.DefaultSuperhumanShortcuts(),
			}
		} else {
			// Merge with defaults
			defaults := domain.GetDefaultShortcuts(shortcuts.Preset)
			merged := make(map[string]string)
			for k, v := range defaults {
				merged[k] = v
			}
			for k, v := range shortcuts.Shortcuts {
				merged[k] = v
			}
			response.Shortcuts = &ShortcutSettingsResponse{
				Preset:    string(shortcuts.Preset),
				Enabled:   shortcuts.Enabled,
				ShowHints: shortcuts.ShowHints,
				Shortcuts: merged,
			}
		}
	}

	// 3. Classification rules
	if h.settingsService != nil {
		rules, _ := h.settingsService.GetClassificationRules(c.Context(), userID)
		response.ClassificationRules = rules
	}

	// Cache the response
	if h.redis != nil {
		h.cacheAllSettings(c.Context(), userID, response)
	}

	return c.JSON(response)
}

// cacheAllSettings caches the all settings response.
func (h *SettingsHandler) cacheAllSettings(ctx context.Context, userID uuid.UUID, response *AllSettingsResponse) {
	if h.redis == nil {
		return
	}

	data, err := json.Marshal(response)
	if err != nil {
		return
	}

	cacheKey := fmt.Sprintf(allSettingsKey, userID.String())
	if err := h.redis.Set(ctx, cacheKey, data, settingsCacheTTL).Err(); err != nil {
		logger.Debug("[Settings] Failed to cache settings: %v", err)
	}
}

// invalidateSettingsCache invalidates all settings caches for a user.
func (h *SettingsHandler) invalidateSettingsCache(ctx context.Context, userID uuid.UUID) {
	if h.redis == nil {
		return
	}

	keys := []string{
		fmt.Sprintf(settingsCacheKey, userID.String()),
		fmt.Sprintf(allSettingsKey, userID.String()),
	}

	for _, key := range keys {
		h.redis.Del(ctx, key)
	}
}

// =============================================================================
// User Settings
// =============================================================================

// GetSettings returns user settings.
func (h *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	settings, err := h.settingsService.GetSettings(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(settings)
}

// UpdateSettingsRequest represents settings update request.
type UpdateSettingsRequest struct {
	// Email settings
	DefaultSignature *string `json:"default_signature,omitempty"`
	AutoReplyEnabled *bool   `json:"auto_reply_enabled,omitempty"`
	AutoReplyMessage *string `json:"auto_reply_message,omitempty"`

	// AI settings
	AIEnabled      *bool   `json:"ai_enabled,omitempty"`
	AIAutoClassify *bool   `json:"ai_auto_classify,omitempty"`
	AITone         *string `json:"ai_tone,omitempty"`

	// UI preferences
	Theme    *string `json:"theme,omitempty"`
	Language *string `json:"language,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
}

// UpdateSettings updates user settings.
func (h *SettingsHandler) UpdateSettings(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	var req UpdateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Convert to map for partial updates
	updates := make(map[string]any)
	if req.DefaultSignature != nil {
		updates["default_signature"] = *req.DefaultSignature
	}
	if req.AutoReplyEnabled != nil {
		updates["auto_reply_enabled"] = *req.AutoReplyEnabled
	}
	if req.AutoReplyMessage != nil {
		updates["auto_reply_message"] = *req.AutoReplyMessage
	}
	if req.AIEnabled != nil {
		updates["ai_enabled"] = *req.AIEnabled
	}
	if req.AIAutoClassify != nil {
		updates["ai_auto_classify"] = *req.AIAutoClassify
	}
	if req.AITone != nil {
		updates["ai_tone"] = *req.AITone
	}
	if req.Theme != nil {
		updates["theme"] = *req.Theme
	}
	if req.Language != nil {
		updates["language"] = *req.Language
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}

	settings, err := h.settingsService.UpdateSettings(c.Context(), userID, updates)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	// Invalidate cache
	h.invalidateSettingsCache(c.Context(), userID)

	return c.JSON(settings)
}

// =============================================================================
// AI Settings
// =============================================================================

// AISettingsResponse represents AI settings response.
type AISettingsResponse struct {
	Enabled      bool   `json:"enabled"`
	AutoClassify bool   `json:"auto_classify"`
	Tone         string `json:"tone"`
}

// GetAISettings returns AI settings.
func (h *SettingsHandler) GetAISettings(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	settings, err := h.settingsService.GetSettings(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(AISettingsResponse{
		Enabled:      settings.AIEnabled,
		AutoClassify: settings.AIAutoClassify,
		Tone:         settings.AITone,
	})
}

// UpdateAISettingsRequest represents AI settings update.
type UpdateAISettingsRequest struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	AutoClassify *bool   `json:"auto_classify,omitempty"`
	Tone         *string `json:"tone,omitempty"`
}

// UpdateAISettings updates AI settings.
func (h *SettingsHandler) UpdateAISettings(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	var req UpdateAISettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	updates := make(map[string]any)
	if req.Enabled != nil {
		updates["ai_enabled"] = *req.Enabled
	}
	if req.AutoClassify != nil {
		updates["ai_auto_classify"] = *req.AutoClassify
	}
	if req.Tone != nil {
		updates["ai_tone"] = *req.Tone
	}

	settings, err := h.settingsService.UpdateSettings(c.Context(), userID, updates)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	// Invalidate cache
	h.invalidateSettingsCache(c.Context(), userID)

	return c.JSON(AISettingsResponse{
		Enabled:      settings.AIEnabled,
		AutoClassify: settings.AIAutoClassify,
		Tone:         settings.AITone,
	})
}

// =============================================================================
// Classification Rules
// =============================================================================

// GetClassificationRules returns user's classification rules.
func (h *SettingsHandler) GetClassificationRules(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	rules, err := h.settingsService.GetClassificationRules(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(rules)
}

// UpdateClassificationRulesRequest represents classification rules update.
type UpdateClassificationRulesRequest struct {
	ImportantDomains  []string `json:"important_domains,omitempty"`
	ImportantKeywords []string `json:"important_keywords,omitempty"`
	IgnoreSenders     []string `json:"ignore_senders,omitempty"`
	IgnoreKeywords    []string `json:"ignore_keywords,omitempty"`
	HighPriorityRules string   `json:"high_priority_rules,omitempty"`
	LowPriorityRules  string   `json:"low_priority_rules,omitempty"`
	CategoryRules     string   `json:"category_rules,omitempty"`
}

// UpdateClassificationRules updates classification rules.
func (h *SettingsHandler) UpdateClassificationRules(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.settingsService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Settings service not available")
	}

	var req UpdateClassificationRulesRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	rules := &domain.ClassificationRules{
		UserID:            userID,
		ImportantDomains:  req.ImportantDomains,
		ImportantKeywords: req.ImportantKeywords,
		IgnoreSenders:     req.IgnoreSenders,
		IgnoreKeywords:    req.IgnoreKeywords,
		HighPriorityRules: req.HighPriorityRules,
		LowPriorityRules:  req.LowPriorityRules,
		CategoryRules:     req.CategoryRules,
	}

	if err := h.settingsService.SaveClassificationRules(c.Context(), rules); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	// Invalidate cache
	h.invalidateSettingsCache(c.Context(), userID)

	return c.JSON(rules)
}
