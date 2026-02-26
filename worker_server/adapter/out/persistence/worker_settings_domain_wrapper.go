// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// SettingsDomainWrapper wraps SettingsAdapter to implement domain.SettingsRepository.
type SettingsDomainWrapper struct {
	adapter *SettingsAdapter
}

// NewSettingsDomainWrapper creates a wrapper that implements domain.SettingsRepository.
func NewSettingsDomainWrapper(adapter *SettingsAdapter) *SettingsDomainWrapper {
	return &SettingsDomainWrapper{adapter: adapter}
}

// GetByUserID implements domain.SettingsRepository.
func (w *SettingsDomainWrapper) GetByUserID(userID uuid.UUID) (*domain.UserSettings, error) {
	return w.adapter.GetByUserID(userID)
}

// Create implements domain.SettingsRepository.
func (w *SettingsDomainWrapper) Create(settings *domain.UserSettings) error {
	return w.adapter.Create(settings)
}

// Update implements domain.SettingsRepository.
func (w *SettingsDomainWrapper) Update(settings *domain.UserSettings) error {
	return w.adapter.Update(settings)
}

// GetClassificationRules implements domain.SettingsRepository.
func (w *SettingsDomainWrapper) GetClassificationRules(ctx context.Context, userID uuid.UUID) (*domain.ClassificationRules, error) {
	return w.adapter.DomainGetClassificationRules(ctx, userID)
}

// SaveClassificationRules implements domain.SettingsRepository.
func (w *SettingsDomainWrapper) SaveClassificationRules(ctx context.Context, rules *domain.ClassificationRules) error {
	return w.adapter.DomainSaveClassificationRules(ctx, rules)
}

// Ensure SettingsDomainWrapper implements domain.SettingsRepository
var _ domain.SettingsRepository = (*SettingsDomainWrapper)(nil)
