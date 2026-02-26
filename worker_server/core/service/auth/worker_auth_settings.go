package auth

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type SettingsService struct {
	settingsRepo domain.SettingsRepository
}

func NewSettingsService(settingsRepo domain.SettingsRepository) *SettingsService {
	return &SettingsService{
		settingsRepo: settingsRepo,
	}
}

func (s *SettingsService) GetSettings(ctx context.Context, userID uuid.UUID) (*domain.UserSettings, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		// Create default settings if not found (lazy initialization)
		settings = domain.DefaultUserSettings(userID)
		if createErr := s.settingsRepo.Create(settings); createErr != nil {
			// Log but continue - return defaults anyway
			// This handles race conditions where another request might have created it
		}
		return settings, nil
	}
	return settings, nil
}

func (s *SettingsService) UpdateSettings(ctx context.Context, userID uuid.UUID, updates map[string]any) (*domain.UserSettings, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		settings = domain.DefaultUserSettings(userID)
		if err := s.settingsRepo.Create(settings); err != nil {
			return nil, err
		}
	}

	// Apply updates
	if v, ok := updates["ai_enabled"].(bool); ok {
		settings.AIEnabled = v
	}
	if v, ok := updates["ai_auto_classify"].(bool); ok {
		settings.AIAutoClassify = v
	}
	if v, ok := updates["ai_tone"].(string); ok {
		settings.AITone = v
	}
	if v, ok := updates["theme"].(string); ok {
		settings.Theme = v
	}
	if v, ok := updates["language"].(string); ok {
		settings.Language = v
	}
	if v, ok := updates["timezone"].(string); ok {
		settings.Timezone = v
	}
	if v, ok := updates["default_signature"].(string); ok {
		settings.DefaultSignature = &v
	}
	if v, ok := updates["auto_reply_enabled"].(bool); ok {
		settings.AutoReplyEnabled = v
	}
	if v, ok := updates["auto_reply_message"].(string); ok {
		settings.AutoReplyMessage = &v
	}

	if err := s.settingsRepo.Update(settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// GetClassificationRules returns user's classification rules.
func (s *SettingsService) GetClassificationRules(ctx context.Context, userID uuid.UUID) (*domain.ClassificationRules, error) {
	if s.settingsRepo == nil {
		return s.getDefaultClassificationRules(userID), nil
	}

	rules, err := s.settingsRepo.GetClassificationRules(ctx, userID)
	if err != nil || rules == nil {
		return s.getDefaultClassificationRules(userID), nil
	}
	return rules, nil
}

// SaveClassificationRules saves user's classification rules.
func (s *SettingsService) SaveClassificationRules(ctx context.Context, rules *domain.ClassificationRules) error {
	if s.settingsRepo == nil {
		return nil
	}
	return s.settingsRepo.SaveClassificationRules(ctx, rules)
}

func (s *SettingsService) getDefaultClassificationRules(userID uuid.UUID) *domain.ClassificationRules {
	return &domain.ClassificationRules{
		UserID:            userID,
		ImportantDomains:  []string{},
		ImportantKeywords: []string{},
		IgnoreSenders:     []string{},
		IgnoreKeywords:    []string{},
	}
}
