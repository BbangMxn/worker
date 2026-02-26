package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ShortcutAdapter implements ShortcutRepository
type ShortcutAdapter struct {
	db *sqlx.DB
}

// NewShortcutAdapter creates a new ShortcutAdapter
func NewShortcutAdapter(db *sqlx.DB) *ShortcutAdapter {
	return &ShortcutAdapter{db: db}
}

// Ensure ShortcutAdapter implements ShortcutRepository
var _ out.ShortcutRepository = (*ShortcutAdapter)(nil)

// shortcutRow represents the database row
type shortcutRow struct {
	ID        int64     `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	Preset    string    `db:"preset"`
	Enabled   bool      `db:"enabled"`
	ShowHints bool      `db:"show_hints"`
	Shortcuts []byte    `db:"shortcuts"` // JSONB
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Get retrieves user's shortcut settings
func (a *ShortcutAdapter) Get(ctx context.Context, userID uuid.UUID) (*domain.KeyboardShortcuts, error) {
	query := `
		SELECT id, user_id, preset, enabled, show_hints, shortcuts, created_at, updated_at
		FROM keyboard_shortcuts
		WHERE user_id = $1
	`

	var row shortcutRow
	err := a.db.QueryRowxContext(ctx, query, userID).StructScan(&row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found, return nil without error
		}
		return nil, err
	}

	return rowToShortcuts(&row)
}

// Upsert creates or updates user's shortcut settings
func (a *ShortcutAdapter) Upsert(ctx context.Context, shortcuts *domain.KeyboardShortcuts) error {
	shortcutsJSON, err := json.Marshal(shortcuts.Shortcuts)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO keyboard_shortcuts (user_id, preset, enabled, show_hints, shortcuts, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			preset = EXCLUDED.preset,
			enabled = EXCLUDED.enabled,
			show_hints = EXCLUDED.show_hints,
			shortcuts = EXCLUDED.shortcuts,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	err = a.db.QueryRowxContext(
		ctx, query,
		shortcuts.UserID,
		shortcuts.Preset,
		shortcuts.Enabled,
		shortcuts.ShowHints,
		shortcutsJSON,
	).Scan(&shortcuts.ID, &shortcuts.CreatedAt, &shortcuts.UpdatedAt)

	return err
}

// Delete removes user's shortcut settings
func (a *ShortcutAdapter) Delete(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM keyboard_shortcuts WHERE user_id = $1`
	_, err := a.db.ExecContext(ctx, query, userID)
	return err
}

// rowToShortcuts converts database row to domain model
func rowToShortcuts(row *shortcutRow) (*domain.KeyboardShortcuts, error) {
	var shortcuts map[string]string
	if len(row.Shortcuts) > 0 {
		if err := json.Unmarshal(row.Shortcuts, &shortcuts); err != nil {
			return nil, err
		}
	}
	if shortcuts == nil {
		shortcuts = make(map[string]string)
	}

	return &domain.KeyboardShortcuts{
		ID:        row.ID,
		UserID:    row.UserID,
		Preset:    domain.ShortcutPreset(row.Preset),
		Enabled:   row.Enabled,
		ShowHints: row.ShowHints,
		Shortcuts: shortcuts,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}
