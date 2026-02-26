package persistence

import (
	"context"
	"database/sql"
	"time"

	"worker_server/core/domain"

	"github.com/jmoiron/sqlx"
)

// =============================================================================
// SyncStateAdapter - Phase 1 동기화 상태 어댑터
// =============================================================================

type SyncStateAdapter struct {
	db *sqlx.DB
}

func NewSyncStateAdapter(db *sqlx.DB) *SyncStateAdapter {
	return &SyncStateAdapter{db: db}
}

// =============================================================================
// Entity
// =============================================================================

type syncStateEntity struct {
	ID              int64          `db:"id"`
	UserID          string         `db:"user_id"`
	ConnectionID    int64          `db:"connection_id"`
	Provider        string         `db:"provider"`
	Status          string         `db:"status"`
	Phase           sql.NullString `db:"phase"`
	LastError       sql.NullString `db:"last_error"`
	HistoryID       int64          `db:"history_id"`
	WatchExpiry     sql.NullTime   `db:"watch_expiry"`
	WatchResourceID sql.NullString `db:"watch_resource_id"`

	// 재시도 관련
	RetryCount  int          `db:"retry_count"`
	MaxRetries  int          `db:"max_retries"`
	NextRetryAt sql.NullTime `db:"next_retry_at"`
	FailedAt    sql.NullTime `db:"failed_at"`

	// 체크포인트
	CheckpointPageToken   sql.NullString `db:"checkpoint_page_token"`
	CheckpointSyncedCount int            `db:"checkpoint_synced_count"`
	CheckpointTotalCount  sql.NullInt32  `db:"checkpoint_total_count"`
	CheckpointUpdatedAt   sql.NullTime   `db:"checkpoint_updated_at"`

	// 통계
	TotalSynced          int64        `db:"total_synced"`
	LastSyncCount        int          `db:"last_sync_count"`
	LastSyncAt           sql.NullTime `db:"last_sync_at"`
	FirstSyncCompletedAt sql.NullTime `db:"first_sync_completed_at"`

	// 성능
	AvgSyncDurationMs  sql.NullInt32 `db:"avg_sync_duration_ms"`
	LastSyncDurationMs sql.NullInt32 `db:"last_sync_duration_ms"`

	// 타임스탬프
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (e *syncStateEntity) toDomain() *domain.SyncState {
	state := &domain.SyncState{
		ID:                    e.ID,
		UserID:                e.UserID,
		ConnectionID:          e.ConnectionID,
		Provider:              domain.Provider(e.Provider),
		Status:                domain.SyncStatus(e.Status),
		HistoryID:             uint64(e.HistoryID),
		RetryCount:            e.RetryCount,
		MaxRetries:            e.MaxRetries,
		CheckpointSyncedCount: e.CheckpointSyncedCount,
		TotalSynced:           e.TotalSynced,
		LastSyncCount:         e.LastSyncCount,
		CreatedAt:             e.CreatedAt,
		UpdatedAt:             e.UpdatedAt,
	}

	// Nullable fields
	if e.Phase.Valid {
		state.Phase = domain.SyncPhase(e.Phase.String)
	}
	if e.LastError.Valid {
		state.LastError = e.LastError.String
	}
	if e.WatchExpiry.Valid {
		state.WatchExpiry = e.WatchExpiry.Time
	}
	if e.WatchResourceID.Valid {
		state.WatchResourceID = e.WatchResourceID.String
	}
	if e.NextRetryAt.Valid {
		state.NextRetryAt = e.NextRetryAt.Time
	}
	if e.FailedAt.Valid {
		state.FailedAt = e.FailedAt.Time
	}
	if e.CheckpointPageToken.Valid {
		state.CheckpointPageToken = e.CheckpointPageToken.String
	}
	if e.CheckpointTotalCount.Valid {
		state.CheckpointTotalCount = int(e.CheckpointTotalCount.Int32)
	}
	if e.CheckpointUpdatedAt.Valid {
		state.CheckpointUpdatedAt = e.CheckpointUpdatedAt.Time
	}
	if e.LastSyncAt.Valid {
		state.LastSyncAt = e.LastSyncAt.Time
	}
	if e.FirstSyncCompletedAt.Valid {
		state.FirstSyncCompletedAt = e.FirstSyncCompletedAt.Time
	}
	if e.AvgSyncDurationMs.Valid {
		state.AvgSyncDurationMs = int(e.AvgSyncDurationMs.Int32)
	}
	if e.LastSyncDurationMs.Valid {
		state.LastSyncDurationMs = int(e.LastSyncDurationMs.Int32)
	}

	return state
}

// =============================================================================
// CRUD
// =============================================================================

func (a *SyncStateAdapter) GetByID(ctx context.Context, id int64) (*domain.SyncState, error) {
	var entity syncStateEntity
	query := `SELECT * FROM sync_states WHERE id = $1`
	if err := a.db.GetContext(ctx, &entity, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity.toDomain(), nil
}

func (a *SyncStateAdapter) GetByConnectionID(ctx context.Context, connectionID int64) (*domain.SyncState, error) {
	var entity syncStateEntity
	query := `SELECT * FROM sync_states WHERE connection_id = $1`
	if err := a.db.GetContext(ctx, &entity, query, connectionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity.toDomain(), nil
}

func (a *SyncStateAdapter) GetByUserID(ctx context.Context, userID string) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	query := `SELECT * FROM sync_states WHERE user_id = $1 ORDER BY created_at DESC`
	if err := a.db.SelectContext(ctx, &entities, query, userID); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

func (a *SyncStateAdapter) Create(ctx context.Context, state *domain.SyncState) error {
	query := `
		INSERT INTO sync_states (
			user_id, connection_id, provider, status, phase,
			history_id, max_retries
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	var phase interface{}
	if state.Phase != "" {
		phase = string(state.Phase)
	}

	maxRetries := state.MaxRetries
	if maxRetries == 0 {
		maxRetries = 5 // 기본값
	}

	return a.db.QueryRowContext(ctx, query,
		state.UserID,
		state.ConnectionID,
		string(state.Provider),
		string(state.Status),
		phase,
		state.HistoryID,
		maxRetries,
	).Scan(&state.ID, &state.CreatedAt, &state.UpdatedAt)
}

func (a *SyncStateAdapter) Update(ctx context.Context, state *domain.SyncState) error {
	query := `
		UPDATE sync_states SET
			status = $1,
			phase = $2,
			last_error = $3,
			history_id = $4,
			watch_expiry = $5,
			watch_resource_id = $6,
			retry_count = $7,
			next_retry_at = $8,
			failed_at = $9,
			checkpoint_page_token = $10,
			checkpoint_synced_count = $11,
			checkpoint_total_count = $12,
			total_synced = $13,
			last_sync_count = $14,
			last_sync_at = $15
		WHERE id = $16
	`

	_, err := a.db.ExecContext(ctx, query,
		string(state.Status),
		toNullableString(string(state.Phase)),
		toNullableString(state.LastError),
		state.HistoryID,
		toNullableTime(state.WatchExpiry),
		toNullableString(state.WatchResourceID),
		state.RetryCount,
		toNullableTime(state.NextRetryAt),
		toNullableTime(state.FailedAt),
		toNullableString(state.CheckpointPageToken),
		state.CheckpointSyncedCount,
		toNullableInt(state.CheckpointTotalCount),
		state.TotalSynced,
		state.LastSyncCount,
		toNullableTime(state.LastSyncAt),
		state.ID,
	)
	return err
}

func (a *SyncStateAdapter) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM sync_states WHERE id = $1`
	_, err := a.db.ExecContext(ctx, query, id)
	return err
}

// =============================================================================
// Watch 관리
// =============================================================================

func (a *SyncStateAdapter) GetExpiredWatches(ctx context.Context, before time.Time) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	query := `
		SELECT * FROM sync_states
		WHERE watch_expiry IS NOT NULL
		  AND watch_expiry < $1
		  AND status NOT IN ('watch_expired', 'error', 'none')
		ORDER BY watch_expiry ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query, before); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

func (a *SyncStateAdapter) UpdateWatchExpiry(ctx context.Context, connectionID int64, expiry time.Time, resourceID string) error {
	query := `
		UPDATE sync_states SET
			watch_expiry = $1,
			watch_resource_id = $2,
			status = 'idle'
		WHERE connection_id = $3
	`
	_, err := a.db.ExecContext(ctx, query, expiry, resourceID, connectionID)
	return err
}

// =============================================================================
// Gap Sync 관리 (Phase 2)
// =============================================================================

func (a *SyncStateAdapter) GetByStatus(ctx context.Context, status domain.SyncStatus) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	query := `
		SELECT * FROM sync_states
		WHERE status = $1
		ORDER BY updated_at DESC
	`
	if err := a.db.SelectContext(ctx, &entities, query, string(status)); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

func (a *SyncStateAdapter) GetStaleConnections(ctx context.Context, staleDuration time.Duration) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	staleTime := time.Now().Add(-staleDuration)

	// idle 상태이면서 마지막 동기화가 staleDuration보다 오래된 연결
	// 또는 Watch가 만료된 연결
	query := `
		SELECT * FROM sync_states
		WHERE status = 'idle'
		  AND first_sync_completed_at IS NOT NULL
		  AND (
			last_sync_at IS NULL
			OR last_sync_at < $1
			OR (watch_expiry IS NOT NULL AND watch_expiry < NOW())
		  )
		ORDER BY last_sync_at ASC NULLS FIRST
	`
	if err := a.db.SelectContext(ctx, &entities, query, staleTime); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

// =============================================================================
// 상태 업데이트
// =============================================================================

func (a *SyncStateAdapter) UpdateStatus(ctx context.Context, connectionID int64, status domain.SyncStatus, lastError string) error {
	query := `
		UPDATE sync_states SET
			status = $1,
			last_error = $2
		WHERE connection_id = $3
	`
	_, err := a.db.ExecContext(ctx, query, string(status), toNullableString(lastError), connectionID)
	return err
}

func (a *SyncStateAdapter) UpdateStatusWithPhase(ctx context.Context, connectionID int64, status domain.SyncStatus, phase domain.SyncPhase, lastError string) error {
	query := `
		UPDATE sync_states SET
			status = $1,
			phase = $2,
			last_error = $3
		WHERE connection_id = $4
	`
	_, err := a.db.ExecContext(ctx, query, string(status), string(phase), toNullableString(lastError), connectionID)
	return err
}

func (a *SyncStateAdapter) UpdateHistoryID(ctx context.Context, connectionID int64, historyID uint64) error {
	query := `
		UPDATE sync_states SET
			history_id = $1,
			last_sync_at = NOW()
		WHERE connection_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, historyID, connectionID)
	return err
}

func (a *SyncStateAdapter) UpdateHistoryIDIfGreater(ctx context.Context, connectionID int64, historyID uint64) error {
	query := `
		UPDATE sync_states SET
			history_id = $1,
			last_sync_at = NOW()
		WHERE connection_id = $2 AND (history_id IS NULL OR history_id < $1)
	`
	_, err := a.db.ExecContext(ctx, query, historyID, connectionID)
	return err
}

func (a *SyncStateAdapter) IncrementSyncCount(ctx context.Context, connectionID int64, count int) error {
	query := `
		UPDATE sync_states SET
			total_synced = total_synced + $1,
			last_sync_count = $1,
			last_sync_at = NOW()
		WHERE connection_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, count, connectionID)
	return err
}

// =============================================================================
// 재시도 관리 (Phase 1 핵심)
// =============================================================================

func (a *SyncStateAdapter) ScheduleRetry(ctx context.Context, connectionID int64, nextRetryAt time.Time) error {
	query := `
		UPDATE sync_states SET
			status = 'retry_scheduled',
			next_retry_at = $1,
			retry_count = retry_count + 1
		WHERE connection_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, nextRetryAt, connectionID)
	return err
}

func (a *SyncStateAdapter) GetPendingRetries(ctx context.Context, before time.Time) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	query := `
		SELECT * FROM sync_states
		WHERE status = 'retry_scheduled'
		  AND next_retry_at IS NOT NULL
		  AND next_retry_at <= $1
		  AND retry_count < max_retries
		ORDER BY next_retry_at ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query, before); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

func (a *SyncStateAdapter) IncrementRetryCount(ctx context.Context, connectionID int64) error {
	query := `
		UPDATE sync_states SET
			retry_count = retry_count + 1
		WHERE connection_id = $1
	`
	_, err := a.db.ExecContext(ctx, query, connectionID)
	return err
}

func (a *SyncStateAdapter) ResetRetryCount(ctx context.Context, connectionID int64) error {
	query := `
		UPDATE sync_states SET
			retry_count = 0,
			next_retry_at = NULL,
			failed_at = NULL,
			last_error = NULL
		WHERE connection_id = $1
	`
	_, err := a.db.ExecContext(ctx, query, connectionID)
	return err
}

func (a *SyncStateAdapter) MarkFailed(ctx context.Context, connectionID int64, errMsg string) error {
	query := `
		UPDATE sync_states SET
			status = 'error',
			last_error = $1,
			failed_at = NOW()
		WHERE connection_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, errMsg, connectionID)
	return err
}

// =============================================================================
// 체크포인트 관리 (Phase 1 핵심)
// =============================================================================

func (a *SyncStateAdapter) SaveCheckpoint(ctx context.Context, connectionID int64, pageToken string, syncedCount, totalCount int) error {
	query := `
		UPDATE sync_states SET
			checkpoint_page_token = $1,
			checkpoint_synced_count = $2,
			checkpoint_total_count = $3,
			checkpoint_updated_at = NOW()
		WHERE connection_id = $4
	`
	_, err := a.db.ExecContext(ctx, query, pageToken, syncedCount, totalCount, connectionID)
	return err
}

func (a *SyncStateAdapter) ClearCheckpoint(ctx context.Context, connectionID int64) error {
	query := `
		UPDATE sync_states SET
			checkpoint_page_token = NULL,
			checkpoint_synced_count = 0,
			checkpoint_total_count = NULL,
			checkpoint_updated_at = NULL
		WHERE connection_id = $1
	`
	_, err := a.db.ExecContext(ctx, query, connectionID)
	return err
}

func (a *SyncStateAdapter) GetWithCheckpoint(ctx context.Context, connectionID int64) (*domain.SyncState, error) {
	var entity syncStateEntity
	query := `
		SELECT * FROM sync_states
		WHERE connection_id = $1
		  AND checkpoint_page_token IS NOT NULL
	`
	if err := a.db.GetContext(ctx, &entity, query, connectionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity.toDomain(), nil
}

func (a *SyncStateAdapter) GetAllWithCheckpoint(ctx context.Context) ([]*domain.SyncState, error) {
	var entities []syncStateEntity
	query := `
		SELECT * FROM sync_states
		WHERE checkpoint_page_token IS NOT NULL
		  AND checkpoint_page_token != ''
		ORDER BY checkpoint_updated_at ASC
	`
	if err := a.db.SelectContext(ctx, &entities, query); err != nil {
		return nil, err
	}

	states := make([]*domain.SyncState, len(entities))
	for i, e := range entities {
		states[i] = e.toDomain()
	}
	return states, nil
}

// =============================================================================
// 통계 및 성능
// =============================================================================

func (a *SyncStateAdapter) UpdateSyncDuration(ctx context.Context, connectionID int64, durationMs int) error {
	// 이동 평균 계산: (기존평균 * 0.8) + (새값 * 0.2)
	query := `
		UPDATE sync_states SET
			last_sync_duration_ms = $1,
			avg_sync_duration_ms = COALESCE(
				(avg_sync_duration_ms * 0.8 + $1 * 0.2)::INT,
				$1
			)
		WHERE connection_id = $2
	`
	_, err := a.db.ExecContext(ctx, query, durationMs, connectionID)
	return err
}

func (a *SyncStateAdapter) MarkFirstSyncComplete(ctx context.Context, connectionID int64) error {
	query := `
		UPDATE sync_states SET
			first_sync_completed_at = NOW(),
			status = 'idle',
			phase = NULL,
			retry_count = 0,
			next_retry_at = NULL,
			failed_at = NULL
		WHERE connection_id = $1
		  AND first_sync_completed_at IS NULL
	`
	_, err := a.db.ExecContext(ctx, query, connectionID)
	return err
}

// =============================================================================
// Helper functions
// =============================================================================

func toNullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func toNullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func toNullableInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}
