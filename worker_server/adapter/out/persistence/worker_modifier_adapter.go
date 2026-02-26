package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/domain"

	"github.com/jmoiron/sqlx"
)

// =============================================================================
// ModifierAdapter - Offline-First Modifier Queue (Phase 4)
// =============================================================================

type ModifierAdapter struct {
	db *sqlx.DB
}

func NewModifierAdapter(db *sqlx.DB) *ModifierAdapter {
	return &ModifierAdapter{db: db}
}

// =============================================================================
// Entity
// =============================================================================

type modifierEntity struct {
	ID            string         `db:"id"`
	UserID        string         `db:"user_id"`
	ConnectionID  int64          `db:"connection_id"`
	Type          string         `db:"type"`
	Status        string         `db:"status"`
	EmailID       sql.NullInt64  `db:"email_id"`
	ExternalID    sql.NullString `db:"external_id"`
	ThreadID      sql.NullString `db:"thread_id"`
	Params        []byte         `db:"params"`
	ClientVersion int64          `db:"client_version"`
	ServerVersion sql.NullInt64  `db:"server_version"`
	CreatedAt     time.Time      `db:"created_at"`
	AppliedAt     sql.NullTime   `db:"applied_at"`
	FailedAt      sql.NullTime   `db:"failed_at"`
	RetryCount    int            `db:"retry_count"`
	LastError     sql.NullString `db:"last_error"`
}

func (e *modifierEntity) toDomain() *domain.Modifier {
	m := &domain.Modifier{
		ID:            e.ID,
		UserID:        e.UserID,
		ConnectionID:  e.ConnectionID,
		Type:          domain.ModifierType(e.Type),
		Status:        domain.ModifierStatus(e.Status),
		ClientVersion: e.ClientVersion,
		CreatedAt:     e.CreatedAt,
		RetryCount:    e.RetryCount,
	}

	if e.EmailID.Valid {
		m.EmailID = e.EmailID.Int64
	}
	if e.ExternalID.Valid {
		m.ExternalID = e.ExternalID.String
	}
	if e.ThreadID.Valid {
		m.ThreadID = e.ThreadID.String
	}
	if e.ServerVersion.Valid {
		m.ServerVersion = e.ServerVersion.Int64
	}
	if e.AppliedAt.Valid {
		m.AppliedAt = &e.AppliedAt.Time
	}
	if e.FailedAt.Valid {
		m.FailedAt = &e.FailedAt.Time
	}
	if e.LastError.Valid {
		m.LastError = e.LastError.String
	}

	// Parse params
	if len(e.Params) > 0 {
		json.Unmarshal(e.Params, &m.Params)
	}

	return m
}

// =============================================================================
// CRUD
// =============================================================================

func (a *ModifierAdapter) Create(ctx context.Context, modifier *domain.Modifier) error {
	params, _ := json.Marshal(modifier.Params)

	query := `
		INSERT INTO modifiers (
			id, user_id, connection_id, type, status,
			email_id, external_id, thread_id, params,
			client_version, retry_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := a.db.ExecContext(ctx, query,
		modifier.ID,
		modifier.UserID,
		modifier.ConnectionID,
		string(modifier.Type),
		string(modifier.Status),
		modNullInt64(modifier.EmailID),
		modNullString(modifier.ExternalID),
		modNullString(modifier.ThreadID),
		params,
		modifier.ClientVersion,
		modifier.RetryCount,
	)
	return err
}

func (a *ModifierAdapter) GetByID(ctx context.Context, id string) (*domain.Modifier, error) {
	var entity modifierEntity
	query := `SELECT * FROM modifiers WHERE id = $1`
	if err := a.db.GetContext(ctx, &entity, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity.toDomain(), nil
}

func (a *ModifierAdapter) Update(ctx context.Context, modifier *domain.Modifier) error {
	params, _ := json.Marshal(modifier.Params)

	query := `
		UPDATE modifiers SET
			status = $1,
			server_version = $2,
			applied_at = $3,
			failed_at = $4,
			retry_count = $5,
			last_error = $6,
			params = $7
		WHERE id = $8
	`

	_, err := a.db.ExecContext(ctx, query,
		string(modifier.Status),
		modNullInt64(modifier.ServerVersion),
		modNullTime(modifier.AppliedAt),
		modNullTime(modifier.FailedAt),
		modifier.RetryCount,
		modNullString(modifier.LastError),
		params,
		modifier.ID,
	)
	return err
}

func (a *ModifierAdapter) Delete(ctx context.Context, id string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM modifiers WHERE id = $1`, id)
	return err
}

// =============================================================================
// Queue Operations
// =============================================================================

func (a *ModifierAdapter) GetPendingByUser(ctx context.Context, userID string) ([]*domain.Modifier, error) {
	var entities []modifierEntity
	query := `
		SELECT * FROM modifiers
		WHERE user_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query, userID); err != nil {
		return nil, err
	}

	modifiers := make([]*domain.Modifier, len(entities))
	for i, e := range entities {
		modifiers[i] = e.toDomain()
	}
	return modifiers, nil
}

func (a *ModifierAdapter) GetPendingByConnection(ctx context.Context, connectionID int64) ([]*domain.Modifier, error) {
	var entities []modifierEntity
	query := `
		SELECT * FROM modifiers
		WHERE connection_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query, connectionID); err != nil {
		return nil, err
	}

	modifiers := make([]*domain.Modifier, len(entities))
	for i, e := range entities {
		modifiers[i] = e.toDomain()
	}
	return modifiers, nil
}

func (a *ModifierAdapter) GetPendingBefore(ctx context.Context, before time.Time) ([]*domain.Modifier, error) {
	var entities []modifierEntity
	query := `
		SELECT * FROM modifiers
		WHERE status = 'pending' AND created_at < $1
		ORDER BY created_at ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query, before); err != nil {
		return nil, err
	}

	modifiers := make([]*domain.Modifier, len(entities))
	for i, e := range entities {
		modifiers[i] = e.toDomain()
	}
	return modifiers, nil
}

func (a *ModifierAdapter) MarkApplied(ctx context.Context, id string, serverVersion int64) error {
	query := `
		UPDATE modifiers SET
			status = 'applied',
			server_version = $1,
			applied_at = NOW()
		WHERE id = $2
	`
	_, err := a.db.ExecContext(ctx, query, serverVersion, id)
	return err
}

func (a *ModifierAdapter) MarkFailed(ctx context.Context, id string, errMsg string) error {
	query := `
		UPDATE modifiers SET
			status = 'failed',
			last_error = $1,
			failed_at = NOW()
		WHERE id = $2
	`
	_, err := a.db.ExecContext(ctx, query, errMsg, id)
	return err
}

func (a *ModifierAdapter) MarkConflict(ctx context.Context, id string, conflictID string) error {
	query := `
		UPDATE modifiers SET
			status = 'conflict'
		WHERE id = $1
	`
	_, err := a.db.ExecContext(ctx, query, id)
	return err
}

func (a *ModifierAdapter) IncrementRetry(ctx context.Context, id string) error {
	query := `
		UPDATE modifiers SET
			retry_count = retry_count + 1
		WHERE id = $1
	`
	_, err := a.db.ExecContext(ctx, query, id)
	return err
}

// =============================================================================
// Batch Operations
// =============================================================================

func (a *ModifierAdapter) CreateBatch(ctx context.Context, batch *domain.ModifierBatch) error {
	params, _ := json.Marshal(batch.Params)

	query := `
		INSERT INTO modifier_batches (
			id, user_id, connection_id, type, status, email_ids, params
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := a.db.ExecContext(ctx, query,
		batch.ID,
		batch.UserID,
		batch.ConnectionID,
		string(batch.Type),
		string(batch.Status),
		batch.EmailIDs,
		params,
	)
	return err
}

func (a *ModifierAdapter) GetBatchByID(ctx context.Context, id string) (*domain.ModifierBatch, error) {
	// TODO: Implement
	return nil, nil
}

// =============================================================================
// Conflict Management
// =============================================================================

type conflictEntity struct {
	ID          string         `db:"id"`
	ModifierID  string         `db:"modifier_id"`
	Type        string         `db:"type"`
	Resolution  sql.NullString `db:"resolution"`
	ClientState []byte         `db:"client_state"`
	ServerState []byte         `db:"server_state"`
	ResolvedAt  sql.NullTime   `db:"resolved_at"`
	ResolvedBy  sql.NullString `db:"resolved_by"`
	CreatedAt   time.Time      `db:"created_at"`
}

func (e *conflictEntity) toDomain() *domain.Conflict {
	c := &domain.Conflict{
		ID:         e.ID,
		ModifierID: e.ModifierID,
		Type:       domain.ConflictType(e.Type),
		CreatedAt:  e.CreatedAt,
	}

	if e.Resolution.Valid {
		c.Resolution = domain.ConflictResolution(e.Resolution.String)
	}
	if e.ResolvedAt.Valid {
		c.ResolvedAt = &e.ResolvedAt.Time
	}
	if e.ResolvedBy.Valid {
		c.ResolvedBy = e.ResolvedBy.String
	}

	json.Unmarshal(e.ClientState, &c.ClientState)
	json.Unmarshal(e.ServerState, &c.ServerState)

	return c
}

func (a *ModifierAdapter) CreateConflict(ctx context.Context, conflict *domain.Conflict) error {
	clientState, _ := json.Marshal(conflict.ClientState)
	serverState, _ := json.Marshal(conflict.ServerState)

	query := `
		INSERT INTO conflicts (
			id, modifier_id, type, resolution, client_state, server_state,
			resolved_at, resolved_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := a.db.ExecContext(ctx, query,
		conflict.ID,
		conflict.ModifierID,
		string(conflict.Type),
		modNullString(string(conflict.Resolution)),
		clientState,
		serverState,
		modNullTime(conflict.ResolvedAt),
		modNullString(conflict.ResolvedBy),
	)
	return err
}

func (a *ModifierAdapter) GetConflictByModifier(ctx context.Context, modifierID string) (*domain.Conflict, error) {
	var entity conflictEntity
	query := `SELECT * FROM conflicts WHERE modifier_id = $1`
	if err := a.db.GetContext(ctx, &entity, query, modifierID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity.toDomain(), nil
}

func (a *ModifierAdapter) GetUnresolvedConflicts(ctx context.Context, userID string) ([]*domain.Conflict, error) {
	var entities []conflictEntity
	query := `
		SELECT c.* FROM conflicts c
		JOIN modifiers m ON m.id = c.modifier_id
		WHERE m.user_id = $1 AND c.resolved_at IS NULL
		ORDER BY c.created_at DESC
	`
	if err := a.db.SelectContext(ctx, &entities, query, userID); err != nil {
		return nil, err
	}

	conflicts := make([]*domain.Conflict, len(entities))
	for i, e := range entities {
		conflicts[i] = e.toDomain()
	}
	return conflicts, nil
}

func (a *ModifierAdapter) ResolveConflict(ctx context.Context, id string, resolution domain.ConflictResolution) error {
	query := `
		UPDATE conflicts SET
			resolution = $1,
			resolved_at = NOW(),
			resolved_by = 'user'
		WHERE id = $2
	`
	_, err := a.db.ExecContext(ctx, query, string(resolution), id)
	return err
}

// =============================================================================
// Version Tracking
// =============================================================================

type emailVersionEntity struct {
	EmailID       int64          `db:"email_id"`
	Version       int64          `db:"version"`
	ModType       sql.NullString `db:"mod_type"`
	ModSource     sql.NullString `db:"mod_source"`
	ModAt         time.Time      `db:"mod_at"`
	PreviousState []byte         `db:"previous_state"`
}

func (a *ModifierAdapter) GetEmailVersion(ctx context.Context, emailID int64) (*domain.EmailVersion, error) {
	var entity emailVersionEntity
	query := `SELECT * FROM email_versions WHERE email_id = $1`
	if err := a.db.GetContext(ctx, &entity, query, emailID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	version := &domain.EmailVersion{
		EmailID: entity.EmailID,
		Version: entity.Version,
		ModAt:   entity.ModAt,
	}
	if entity.ModType.Valid {
		version.ModType = entity.ModType.String
	}
	if entity.ModSource.Valid {
		version.ModSource = entity.ModSource.String
	}

	return version, nil
}

func (a *ModifierAdapter) UpdateEmailVersion(ctx context.Context, version *domain.EmailVersion) error {
	query := `
		INSERT INTO email_versions (email_id, version, mod_type, mod_source, mod_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (email_id) DO UPDATE SET
			version = EXCLUDED.version,
			mod_type = EXCLUDED.mod_type,
			mod_source = EXCLUDED.mod_source,
			mod_at = EXCLUDED.mod_at
	`
	_, err := a.db.ExecContext(ctx, query,
		version.EmailID,
		version.Version,
		version.ModType,
		version.ModSource,
		version.ModAt,
	)
	return err
}

// =============================================================================
// Cleanup
// =============================================================================

func (a *ModifierAdapter) DeleteAppliedBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := a.db.ExecContext(ctx,
		`DELETE FROM modifiers WHERE status = 'applied' AND applied_at < $1`,
		before,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (a *ModifierAdapter) DeleteByUser(ctx context.Context, userID string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM modifiers WHERE user_id = $1`, userID)
	return err
}

// =============================================================================
// Helper functions
// =============================================================================

func modNullInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func modNullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func modNullTime(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}
