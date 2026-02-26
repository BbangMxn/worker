package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/port/out"

	"github.com/jmoiron/sqlx"
)

// CalendarSyncAdapter implements out.CalendarSyncRepository using PostgreSQL.
type CalendarSyncAdapter struct {
	db *sqlx.DB
}

// NewCalendarSyncAdapter creates a new CalendarSyncAdapter.
func NewCalendarSyncAdapter(db *sqlx.DB) *CalendarSyncAdapter {
	return &CalendarSyncAdapter{db: db}
}

// calendarSyncRow represents the database row for calendar sync state.
type calendarSyncRow struct {
	ID              int64          `db:"id"`
	UserID          string         `db:"user_id"`
	ConnectionID    int64          `db:"connection_id"`
	CalendarID      string         `db:"calendar_id"`
	Provider        string         `db:"provider"`
	SyncToken       sql.NullString `db:"sync_token"`
	WatchID         sql.NullString `db:"watch_id"`
	WatchExpiry     sql.NullTime   `db:"watch_expiry"`
	WatchResourceID sql.NullString `db:"watch_resource_id"`
	Status          string         `db:"status"`
	LastSyncAt      sql.NullTime   `db:"last_sync_at"`
	LastError       sql.NullString `db:"last_error"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

func (r *calendarSyncRow) toEntity() *out.CalendarSyncState {
	state := &out.CalendarSyncState{
		ID:           r.ID,
		UserID:       r.UserID,
		ConnectionID: r.ConnectionID,
		CalendarID:   r.CalendarID,
		Provider:     r.Provider,
		Status:       r.Status,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}

	if r.SyncToken.Valid {
		state.SyncToken = r.SyncToken.String
	}
	if r.WatchID.Valid {
		state.WatchID = r.WatchID.String
	}
	if r.WatchExpiry.Valid {
		state.WatchExpiry = r.WatchExpiry.Time
	}
	if r.LastSyncAt.Valid {
		state.LastSyncAt = r.LastSyncAt.Time
	}
	if r.LastError.Valid {
		state.ErrorMessage = r.LastError.String
	}

	return state
}

// GetByConnectionID gets all sync states for a connection.
func (a *CalendarSyncAdapter) GetByConnectionID(ctx context.Context, connectionID int64) (*out.CalendarSyncState, error) {
	query := `SELECT * FROM calendar_sync_states WHERE connection_id = $1 LIMIT 1`

	var row calendarSyncRow
	err := a.db.QueryRowxContext(ctx, query, connectionID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByCalendarID gets sync state for a specific calendar.
func (a *CalendarSyncAdapter) GetByCalendarID(ctx context.Context, connectionID int64, calendarID string) (*out.CalendarSyncState, error) {
	query := `SELECT * FROM calendar_sync_states WHERE connection_id = $1 AND calendar_id = $2`

	var row calendarSyncRow
	err := a.db.QueryRowxContext(ctx, query, connectionID, calendarID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByWatchID gets sync state by watch ID (for webhook handling).
func (a *CalendarSyncAdapter) GetByWatchID(ctx context.Context, watchID string) (*out.CalendarSyncState, error) {
	query := `SELECT * FROM calendar_sync_states WHERE watch_id = $1`

	var row calendarSyncRow
	err := a.db.QueryRowxContext(ctx, query, watchID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// Create creates a new sync state.
func (a *CalendarSyncAdapter) Create(ctx context.Context, state *out.CalendarSyncState) error {
	if state.Status == "" {
		state.Status = "idle"
	}
	if state.Provider == "" {
		state.Provider = "google"
	}

	query := `
		INSERT INTO calendar_sync_states (
			user_id, connection_id, calendar_id, provider,
			sync_token, watch_id, watch_expiry, watch_resource_id,
			status, last_sync_at, last_error
		) VALUES (
			$1, $2, $3, $4,
			NULLIF($5, ''), NULLIF($6, ''), $7, NULLIF($8, ''),
			$9, $10, NULLIF($11, '')
		)
		ON CONFLICT (connection_id, calendar_id) DO UPDATE SET
			sync_token = COALESCE(EXCLUDED.sync_token, calendar_sync_states.sync_token),
			watch_id = COALESCE(EXCLUDED.watch_id, calendar_sync_states.watch_id),
			watch_expiry = COALESCE(EXCLUDED.watch_expiry, calendar_sync_states.watch_expiry),
			status = EXCLUDED.status,
			last_sync_at = EXCLUDED.last_sync_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	var watchExpiry sql.NullTime
	if !state.WatchExpiry.IsZero() {
		watchExpiry = sql.NullTime{Time: state.WatchExpiry, Valid: true}
	}

	var lastSyncAt sql.NullTime
	if !state.LastSyncAt.IsZero() {
		lastSyncAt = sql.NullTime{Time: state.LastSyncAt, Valid: true}
	}

	return a.db.QueryRowxContext(ctx, query,
		state.UserID, state.ConnectionID, state.CalendarID, state.Provider,
		state.SyncToken, state.WatchID, watchExpiry, "",
		state.Status, lastSyncAt, state.ErrorMessage,
	).Scan(&state.ID, &state.CreatedAt, &state.UpdatedAt)
}

// Update updates a sync state.
func (a *CalendarSyncAdapter) Update(ctx context.Context, state *out.CalendarSyncState) error {
	query := `
		UPDATE calendar_sync_states SET
			sync_token = NULLIF($1, ''),
			watch_id = NULLIF($2, ''),
			watch_expiry = $3,
			status = $4,
			last_sync_at = $5,
			last_error = NULLIF($6, ''),
			updated_at = NOW()
		WHERE id = $7
	`

	var watchExpiry sql.NullTime
	if !state.WatchExpiry.IsZero() {
		watchExpiry = sql.NullTime{Time: state.WatchExpiry, Valid: true}
	}

	var lastSyncAt sql.NullTime
	if !state.LastSyncAt.IsZero() {
		lastSyncAt = sql.NullTime{Time: state.LastSyncAt, Valid: true}
	}

	result, err := a.db.ExecContext(ctx, query,
		state.SyncToken, state.WatchID, watchExpiry,
		state.Status, lastSyncAt, state.ErrorMessage,
		state.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("calendar sync state not found")
	}
	return nil
}

// UpdateSyncToken updates only the sync token.
func (a *CalendarSyncAdapter) UpdateSyncToken(ctx context.Context, connectionID int64, calendarID, syncToken string) error {
	query := `
		UPDATE calendar_sync_states SET
			sync_token = $1,
			last_sync_at = NOW(),
			updated_at = NOW()
		WHERE connection_id = $2 AND calendar_id = $3
	`

	_, err := a.db.ExecContext(ctx, query, syncToken, connectionID, calendarID)
	return err
}

// UpdateWatchExpiry updates watch information.
func (a *CalendarSyncAdapter) UpdateWatchExpiry(ctx context.Context, connectionID int64, calendarID string, expiry time.Time, watchID string) error {
	query := `
		UPDATE calendar_sync_states SET
			watch_id = $1,
			watch_expiry = $2,
			updated_at = NOW()
		WHERE connection_id = $3 AND calendar_id = $4
	`

	result, err := a.db.ExecContext(ctx, query, watchID, expiry, connectionID, calendarID)
	if err != nil {
		return err
	}

	// If no rows updated, create a new record
	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Get connection info to get user_id
		var userID string
		err := a.db.QueryRowContext(ctx,
			`SELECT user_id FROM oauth_connections WHERE id = $1`,
			connectionID,
		).Scan(&userID)
		if err != nil {
			return fmt.Errorf("failed to get connection info: %w", err)
		}

		insertQuery := `
			INSERT INTO calendar_sync_states (user_id, connection_id, calendar_id, provider, watch_id, watch_expiry, status)
			VALUES ($1, $2, $3, 'google', $4, $5, 'idle')
		`
		_, err = a.db.ExecContext(ctx, insertQuery, userID, connectionID, calendarID, watchID, expiry)
		return err
	}

	return nil
}

// UpdateStatus updates the sync status.
func (a *CalendarSyncAdapter) UpdateStatus(ctx context.Context, connectionID int64, calendarID, status, errorMsg string) error {
	query := `
		UPDATE calendar_sync_states SET
			status = $1,
			last_error = NULLIF($2, ''),
			updated_at = NOW()
		WHERE connection_id = $3 AND calendar_id = $4
	`

	_, err := a.db.ExecContext(ctx, query, status, errorMsg, connectionID, calendarID)
	return err
}

// GetExpiredWatches gets watches that are expiring soon.
func (a *CalendarSyncAdapter) GetExpiredWatches(ctx context.Context, before time.Time) ([]*out.CalendarSyncState, error) {
	query := `
		SELECT * FROM calendar_sync_states
		WHERE watch_expiry IS NOT NULL AND watch_expiry < $1
		ORDER BY watch_expiry ASC
	`

	rows, err := a.db.QueryxContext(ctx, query, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []*out.CalendarSyncState
	for rows.Next() {
		var row calendarSyncRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		states = append(states, row.toEntity())
	}

	return states, nil
}

// Delete deletes a sync state.
func (a *CalendarSyncAdapter) Delete(ctx context.Context, connectionID int64, calendarID string) error {
	query := `DELETE FROM calendar_sync_states WHERE connection_id = $1 AND calendar_id = $2`
	_, err := a.db.ExecContext(ctx, query, connectionID, calendarID)
	return err
}

// ListByConnectionID lists all sync states for a connection.
func (a *CalendarSyncAdapter) ListByConnectionID(ctx context.Context, connectionID int64) ([]*out.CalendarSyncState, error) {
	query := `SELECT * FROM calendar_sync_states WHERE connection_id = $1 ORDER BY calendar_id`

	rows, err := a.db.QueryxContext(ctx, query, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []*out.CalendarSyncState
	for rows.Next() {
		var row calendarSyncRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		states = append(states, row.toEntity())
	}

	return states, nil
}

// Ensure CalendarSyncAdapter implements out.CalendarSyncRepository
var _ out.CalendarSyncRepository = (*CalendarSyncAdapter)(nil)
