package out

import (
	"context"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// ReminderRepository defines the interface for reminder persistence
type ReminderRepository interface {
	// Reminder CRUD
	GetReminder(ctx context.Context, id int64) (*domain.Reminder, error)
	ListReminders(ctx context.Context, filter *domain.ReminderFilter) ([]*domain.Reminder, int, error)
	CreateReminder(ctx context.Context, reminder *domain.Reminder) error
	UpdateReminder(ctx context.Context, reminder *domain.Reminder) error
	DeleteReminder(ctx context.Context, id int64) error

	// Batch operations
	CreateReminders(ctx context.Context, reminders []*domain.Reminder) error

	// Status updates
	MarkReminderSent(ctx context.Context, id int64) error
	CancelReminder(ctx context.Context, id int64) error
	SnoozeReminder(ctx context.Context, id int64, until time.Time) error

	// By source
	GetRemindersBySource(ctx context.Context, sourceType domain.ReminderSourceType, sourceID string) ([]*domain.Reminder, error)
	DeleteRemindersBySource(ctx context.Context, sourceType domain.ReminderSourceType, sourceID string) error

	// Scheduling
	GetPendingReminders(ctx context.Context, until time.Time, limit int) ([]*domain.Reminder, error)
	GetSnoozedReminders(ctx context.Context, until time.Time) ([]*domain.Reminder, error)

	// ReminderRule CRUD
	GetRule(ctx context.Context, id int64) (*domain.ReminderRule, error)
	ListRules(ctx context.Context, filter *domain.ReminderRuleFilter) ([]*domain.ReminderRule, int, error)
	CreateRule(ctx context.Context, rule *domain.ReminderRule) error
	UpdateRule(ctx context.Context, rule *domain.ReminderRule) error
	DeleteRule(ctx context.Context, id int64) error

	// Rule operations
	GetEnabledRules(ctx context.Context, userID uuid.UUID, targetType domain.ReminderSourceType) ([]*domain.ReminderRule, error)
	ToggleRule(ctx context.Context, id int64, enabled bool) error
}
