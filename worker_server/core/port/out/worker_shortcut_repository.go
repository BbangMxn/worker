package out

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// ShortcutRepository defines the interface for keyboard shortcut persistence
type ShortcutRepository interface {
	// Get retrieves user's shortcut settings
	Get(ctx context.Context, userID uuid.UUID) (*domain.KeyboardShortcuts, error)

	// Upsert creates or updates user's shortcut settings
	Upsert(ctx context.Context, shortcuts *domain.KeyboardShortcuts) error

	// Delete removes user's shortcut settings (resets to default)
	Delete(ctx context.Context, userID uuid.UUID) error
}
