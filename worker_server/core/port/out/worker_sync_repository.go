package out

import (
	"context"
	"time"

	"worker_server/core/domain"
)

// SyncStateRepository - 동기화 상태 저장소 (Phase 1)
type SyncStateRepository interface {
	// ==========================================================================
	// CRUD
	// ==========================================================================
	GetByID(ctx context.Context, id int64) (*domain.SyncState, error)
	GetByConnectionID(ctx context.Context, connectionID int64) (*domain.SyncState, error)
	GetByUserID(ctx context.Context, userID string) ([]*domain.SyncState, error)
	Create(ctx context.Context, state *domain.SyncState) error
	Update(ctx context.Context, state *domain.SyncState) error
	Delete(ctx context.Context, id int64) error

	// ==========================================================================
	// Watch 관리
	// ==========================================================================
	GetExpiredWatches(ctx context.Context, before time.Time) ([]*domain.SyncState, error)
	UpdateWatchExpiry(ctx context.Context, connectionID int64, expiry time.Time, resourceID string) error

	// ==========================================================================
	// Gap Sync 관리 (Phase 2)
	// ==========================================================================
	// GetByStatus - 특정 상태인 연결 조회
	GetByStatus(ctx context.Context, status domain.SyncStatus) ([]*domain.SyncState, error)

	// GetStaleConnections - 오래된 연결 조회 (gap 체크 필요)
	GetStaleConnections(ctx context.Context, staleDuration time.Duration) ([]*domain.SyncState, error)

	// ==========================================================================
	// 상태 업데이트
	// ==========================================================================
	UpdateStatus(ctx context.Context, connectionID int64, status domain.SyncStatus, lastError string) error
	UpdateStatusWithPhase(ctx context.Context, connectionID int64, status domain.SyncStatus, phase domain.SyncPhase, lastError string) error
	UpdateHistoryID(ctx context.Context, connectionID int64, historyID uint64) error
	UpdateHistoryIDIfGreater(ctx context.Context, connectionID int64, historyID uint64) error
	IncrementSyncCount(ctx context.Context, connectionID int64, count int) error

	// ==========================================================================
	// 재시도 관리 (Phase 1 핵심)
	// ==========================================================================
	// ScheduleRetry - 재시도 예약
	ScheduleRetry(ctx context.Context, connectionID int64, nextRetryAt time.Time) error

	// GetPendingRetries - 재시도 대기 중인 상태 조회
	GetPendingRetries(ctx context.Context, before time.Time) ([]*domain.SyncState, error)

	// IncrementRetryCount - 재시도 횟수 증가
	IncrementRetryCount(ctx context.Context, connectionID int64) error

	// ResetRetryCount - 재시도 횟수 초기화 (성공 시)
	ResetRetryCount(ctx context.Context, connectionID int64) error

	// MarkFailed - 실패 상태로 마킹
	MarkFailed(ctx context.Context, connectionID int64, err string) error

	// ==========================================================================
	// 체크포인트 관리 (Phase 1 핵심)
	// ==========================================================================
	// SaveCheckpoint - 체크포인트 저장 (이어하기용)
	SaveCheckpoint(ctx context.Context, connectionID int64, pageToken string, syncedCount, totalCount int) error

	// ClearCheckpoint - 체크포인트 삭제 (동기화 완료 시)
	ClearCheckpoint(ctx context.Context, connectionID int64) error

	// GetWithCheckpoint - 체크포인트가 있는 상태 조회 (특정 연결)
	GetWithCheckpoint(ctx context.Context, connectionID int64) (*domain.SyncState, error)

	// GetAllWithCheckpoint - 체크포인트가 있는 모든 상태 조회 (백그라운드 동기화용)
	GetAllWithCheckpoint(ctx context.Context) ([]*domain.SyncState, error)

	// ==========================================================================
	// 통계 및 성능
	// ==========================================================================
	// UpdateSyncDuration - 동기화 소요시간 업데이트
	UpdateSyncDuration(ctx context.Context, connectionID int64, durationMs int) error

	// MarkFirstSyncComplete - 최초 동기화 완료 마킹
	MarkFirstSyncComplete(ctx context.Context, connectionID int64) error
}
