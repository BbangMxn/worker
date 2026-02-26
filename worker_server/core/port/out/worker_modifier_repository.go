package out

import (
	"context"
	"time"

	"worker_server/core/domain"
)

// =============================================================================
// ModifierRepository - Offline-First Modifier Queue (Phase 4)
// =============================================================================

type ModifierRepository interface {
	// ==========================================================================
	// CRUD
	// ==========================================================================
	Create(ctx context.Context, modifier *domain.Modifier) error
	GetByID(ctx context.Context, id string) (*domain.Modifier, error)
	Update(ctx context.Context, modifier *domain.Modifier) error
	Delete(ctx context.Context, id string) error

	// ==========================================================================
	// Queue Operations
	// ==========================================================================
	// GetPendingByUser - 사용자의 대기 중인 modifier 조회
	GetPendingByUser(ctx context.Context, userID string) ([]*domain.Modifier, error)

	// GetPendingByConnection - 연결별 대기 중인 modifier 조회
	GetPendingByConnection(ctx context.Context, connectionID int64) ([]*domain.Modifier, error)

	// GetPendingBefore - 특정 시간 이전의 대기 중인 modifier 조회
	GetPendingBefore(ctx context.Context, before time.Time) ([]*domain.Modifier, error)

	// MarkApplied - 적용 완료 마킹
	MarkApplied(ctx context.Context, id string, serverVersion int64) error

	// MarkFailed - 실패 마킹
	MarkFailed(ctx context.Context, id string, err string) error

	// MarkConflict - 충돌 마킹
	MarkConflict(ctx context.Context, id string, conflictID string) error

	// IncrementRetry - 재시도 횟수 증가
	IncrementRetry(ctx context.Context, id string) error

	// ==========================================================================
	// Batch Operations
	// ==========================================================================
	// CreateBatch - 배치 modifier 생성
	CreateBatch(ctx context.Context, batch *domain.ModifierBatch) error

	// GetBatchByID - 배치 조회
	GetBatchByID(ctx context.Context, id string) (*domain.ModifierBatch, error)

	// ==========================================================================
	// Conflict Management
	// ==========================================================================
	// CreateConflict - 충돌 생성
	CreateConflict(ctx context.Context, conflict *domain.Conflict) error

	// GetConflictByModifier - modifier별 충돌 조회
	GetConflictByModifier(ctx context.Context, modifierID string) (*domain.Conflict, error)

	// GetUnresolvedConflicts - 미해결 충돌 조회
	GetUnresolvedConflicts(ctx context.Context, userID string) ([]*domain.Conflict, error)

	// ResolveConflict - 충돌 해결
	ResolveConflict(ctx context.Context, id string, resolution domain.ConflictResolution) error

	// ==========================================================================
	// Version Tracking
	// ==========================================================================
	// GetEmailVersion - 이메일 버전 조회
	GetEmailVersion(ctx context.Context, emailID int64) (*domain.EmailVersion, error)

	// UpdateEmailVersion - 이메일 버전 업데이트
	UpdateEmailVersion(ctx context.Context, version *domain.EmailVersion) error

	// ==========================================================================
	// Cleanup
	// ==========================================================================
	// DeleteAppliedBefore - 오래된 적용 완료 modifier 삭제
	DeleteAppliedBefore(ctx context.Context, before time.Time) (int64, error)

	// DeleteByUser - 사용자의 모든 modifier 삭제
	DeleteByUser(ctx context.Context, userID string) error
}
