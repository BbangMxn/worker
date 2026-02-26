package domain

import (
	"time"
)

// =============================================================================
// Offline-First Domain (Phase 4)
// =============================================================================
//
// Superhuman-style Modifier Queue for offline operations.
// 오프라인에서 수행한 작업을 저장하고 온라인 복귀 시 서버에 적용합니다.

// ModifierType - 수정 작업 유형
type ModifierType string

const (
	ModifierMarkRead     ModifierType = "mark_read"
	ModifierMarkUnread   ModifierType = "mark_unread"
	ModifierArchive      ModifierType = "archive"
	ModifierTrash        ModifierType = "trash"
	ModifierDelete       ModifierType = "delete"
	ModifierStar         ModifierType = "star"
	ModifierUnstar       ModifierType = "unstar"
	ModifierMoveToFolder ModifierType = "move_to_folder"
	ModifierAddLabel     ModifierType = "add_label"
	ModifierRemoveLabel  ModifierType = "remove_label"
	ModifierSnooze       ModifierType = "snooze"
	ModifierUnsnooze     ModifierType = "unsnooze"
)

// ModifierStatus - 수정 작업 상태
type ModifierStatus string

const (
	ModifierStatusPending   ModifierStatus = "pending"   // 적용 대기
	ModifierStatusApplying  ModifierStatus = "applying"  // 적용 중
	ModifierStatusApplied   ModifierStatus = "applied"   // 적용 완료
	ModifierStatusFailed    ModifierStatus = "failed"    // 적용 실패
	ModifierStatusConflict  ModifierStatus = "conflict"  // 충돌 발생
	ModifierStatusCancelled ModifierStatus = "cancelled" // 취소됨
)

// Modifier - 오프라인 수정 작업
type Modifier struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	ConnectionID int64          `json:"connection_id"`
	Type         ModifierType   `json:"type"`
	Status       ModifierStatus `json:"status"`

	// 대상
	EmailID    int64  `json:"email_id,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	ThreadID   string `json:"thread_id,omitempty"`

	// 파라미터
	Params ModifierParams `json:"params,omitempty"`

	// 버전 관리 (충돌 감지용)
	ClientVersion int64 `json:"client_version"` // 클라이언트 로컬 버전
	ServerVersion int64 `json:"server_version"` // 서버 버전 (적용 후)

	// 타임스탬프
	CreatedAt  time.Time  `json:"created_at"`
	AppliedAt  *time.Time `json:"applied_at,omitempty"`
	FailedAt   *time.Time `json:"failed_at,omitempty"`
	RetryCount int        `json:"retry_count"`

	// 에러
	LastError string `json:"last_error,omitempty"`
}

// ModifierParams - 수정 작업 파라미터
type ModifierParams struct {
	Folder    string     `json:"folder,omitempty"`     // move_to_folder
	Label     string     `json:"label,omitempty"`      // add_label, remove_label
	SnoozeAt  *time.Time `json:"snooze_at,omitempty"`  // snooze
	IsRead    *bool      `json:"is_read,omitempty"`    // mark_read, mark_unread
	IsStarred *bool      `json:"is_starred,omitempty"` // star, unstar
}

// ModifierBatch - 배치 수정 요청 (여러 이메일 동시 수정)
type ModifierBatch struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	ConnectionID int64          `json:"connection_id"`
	Type         ModifierType   `json:"type"`
	Status       ModifierStatus `json:"status"`
	EmailIDs     []int64        `json:"email_ids"`
	Params       ModifierParams `json:"params,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	AppliedAt    *time.Time     `json:"applied_at,omitempty"`
}

// =============================================================================
// Conflict Resolution
// =============================================================================

// ConflictType - 충돌 유형
type ConflictType string

const (
	ConflictTypeVersionMismatch ConflictType = "version_mismatch" // 버전 불일치
	ConflictTypeDeleted         ConflictType = "deleted"          // 서버에서 삭제됨
	ConflictTypeMoved           ConflictType = "moved"            // 서버에서 이동됨
	ConflictTypeModified        ConflictType = "modified"         // 서버에서 수정됨
)

// ConflictResolution - 충돌 해결 방식
type ConflictResolution string

const (
	ResolutionClientWins ConflictResolution = "client_wins" // 클라이언트 우선
	ResolutionServerWins ConflictResolution = "server_wins" // 서버 우선
	ResolutionMerge      ConflictResolution = "merge"       // 병합
	ResolutionManual     ConflictResolution = "manual"      // 사용자 선택
)

// Conflict - 충돌 정보
type Conflict struct {
	ID          string             `json:"id"`
	ModifierID  string             `json:"modifier_id"`
	Type        ConflictType       `json:"type"`
	Resolution  ConflictResolution `json:"resolution,omitempty"`
	ClientState map[string]any     `json:"client_state"` // 클라이언트 상태
	ServerState map[string]any     `json:"server_state"` // 서버 상태
	ResolvedAt  *time.Time         `json:"resolved_at,omitempty"`
	ResolvedBy  string             `json:"resolved_by,omitempty"` // auto, user
	CreatedAt   time.Time          `json:"created_at"`
}

// =============================================================================
// Version Tracking
// =============================================================================

// EmailVersion - 이메일 버전 추적
type EmailVersion struct {
	EmailID       int64     `json:"email_id"`
	Version       int64     `json:"version"`
	ModType       string    `json:"mod_type"`   // 마지막 수정 유형
	ModSource     string    `json:"mod_source"` // client, server, sync
	ModAt         time.Time `json:"mod_at"`
	PreviousState string    `json:"previous_state,omitempty"` // JSON (롤백용)
}

// =============================================================================
// Helper Methods
// =============================================================================

// IsPending returns true if modifier is waiting to be applied
func (m *Modifier) IsPending() bool {
	return m.Status == ModifierStatusPending
}

// CanRetry returns true if modifier can be retried
func (m *Modifier) CanRetry() bool {
	return m.Status == ModifierStatusFailed && m.RetryCount < 3
}

// IsTerminal returns true if modifier is in a terminal state
func (m *Modifier) IsTerminal() bool {
	return m.Status == ModifierStatusApplied ||
		m.Status == ModifierStatusCancelled ||
		(m.Status == ModifierStatusFailed && m.RetryCount >= 3)
}

// NeedsConflictResolution returns true if modifier has a conflict
func (m *Modifier) NeedsConflictResolution() bool {
	return m.Status == ModifierStatusConflict
}
