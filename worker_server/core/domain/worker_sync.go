package domain

import (
	"time"

	"github.com/goccy/go-json"
)

// =============================================================================
// Sync Status & Phase - Bridgify Mail Sync Architecture
// =============================================================================

type SyncStatus string

const (
	SyncStatusNone           SyncStatus = "none"            // 초기 상태 (연결만 됨)
	SyncStatusPending        SyncStatus = "pending"         // 동기화 대기 중
	SyncStatusSyncing        SyncStatus = "syncing"         // 동기화 진행 중
	SyncStatusIdle           SyncStatus = "idle"            // 정상 (동기화 완료)
	SyncStatusError          SyncStatus = "error"           // 일시적 오류
	SyncStatusRetryScheduled SyncStatus = "retry_scheduled" // 재시도 예약됨
	SyncStatusGapChecking    SyncStatus = "gap_checking"    // 갭 체크 중
	SyncStatusGapSyncing     SyncStatus = "gap_syncing"     // 갭 동기화 중
	SyncStatusWatchExpired   SyncStatus = "watch_expired"   // Watch 만료됨
)

type SyncPhase string

const (
	SyncPhaseInitialFirstBatch SyncPhase = "initial_first_batch" // 최초 50개 가져오기
	SyncPhaseInitialRemaining  SyncPhase = "initial_remaining"   // 나머지 백그라운드 동기화
	SyncPhaseDelta             SyncPhase = "delta"               // 증분 동기화
	SyncPhaseGap               SyncPhase = "gap"                 // 갭 복구
	SyncPhaseFullResync        SyncPhase = "full_resync"         // 전체 재동기화
)

// =============================================================================
// SyncState - 사용자별 메일 동기화 상태 (Phase 1)
// =============================================================================

type SyncState struct {
	ID           int64    `json:"id"`
	UserID       string   `json:"user_id"`
	ConnectionID int64    `json:"connection_id"`
	Provider     Provider `json:"provider"`

	// 동기화 상태
	Status    SyncStatus `json:"status"`
	Phase     SyncPhase  `json:"phase,omitempty"`
	LastError string     `json:"last_error,omitempty"`

	// Gmail History tracking
	HistoryID uint64 `json:"history_id"`

	// Watch 상태 (Gmail Push Notification)
	WatchExpiry     time.Time `json:"watch_expiry"`
	WatchResourceID string    `json:"watch_resource_id,omitempty"`

	// 재시도 관련
	RetryCount  int       `json:"retry_count"`
	MaxRetries  int       `json:"max_retries"`
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
	FailedAt    time.Time `json:"failed_at,omitempty"`

	// 체크포인트 - 이어하기 지원
	CheckpointPageToken   string    `json:"checkpoint_page_token,omitempty"`
	CheckpointSyncedCount int       `json:"checkpoint_synced_count"`
	CheckpointTotalCount  int       `json:"checkpoint_total_count,omitempty"`
	CheckpointUpdatedAt   time.Time `json:"checkpoint_updated_at,omitempty"`

	// 통계
	TotalSynced          int64     `json:"total_synced"`
	LastSyncCount        int       `json:"last_sync_count"`
	LastSyncAt           time.Time `json:"last_sync_at,omitempty"`
	FirstSyncCompletedAt time.Time `json:"first_sync_completed_at,omitempty"`

	// 성능 측정
	AvgSyncDurationMs  int `json:"avg_sync_duration_ms,omitempty"`
	LastSyncDurationMs int `json:"last_sync_duration_ms,omitempty"`

	// 타임스탬프
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsFirstSync - 최초 동기화인지 확인
func (s *SyncState) IsFirstSync() bool {
	return s.FirstSyncCompletedAt.IsZero()
}

// CanRetry - 재시도 가능한지 확인
func (s *SyncState) CanRetry() bool {
	return s.RetryCount < s.MaxRetries
}

// NeedsRetry - 재시도가 필요한지 확인
func (s *SyncState) NeedsRetry() bool {
	if s.Status != SyncStatusRetryScheduled {
		return false
	}
	return !s.NextRetryAt.IsZero() && time.Now().After(s.NextRetryAt)
}

// HasCheckpoint - 체크포인트가 있는지 확인
func (s *SyncState) HasCheckpoint() bool {
	return s.CheckpointPageToken != ""
}

// SyncProgress - 동기화 진행률 계산
func (s *SyncState) SyncProgress() float64 {
	if s.CheckpointTotalCount == 0 {
		return 0
	}
	return float64(s.CheckpointSyncedCount) / float64(s.CheckpointTotalCount) * 100
}

// =============================================================================
// SyncJob - Redis Stream에 발행되는 동기화 작업
// =============================================================================

type SyncJob struct {
	ID           string          `json:"id"`
	Type         JobType         `json:"type"`
	UserID       string          `json:"user_id"`
	ConnectionID int64           `json:"connection_id"`
	HistoryID    uint64          `json:"history_id,omitempty"`
	Priority     Priority        `json:"priority"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	RetryCount   int             `json:"retry_count"`
	CreatedAt    time.Time       `json:"created_at"`
}

type JobType string

const (
	// Mail sync jobs
	JobMailSync      JobType = "mail.sync"
	JobMailSyncFull  JobType = "mail.sync.full"
	JobMailSyncDelta JobType = "mail.sync.delta"
	JobMailSyncGap   JobType = "mail.sync.gap" // 갭 동기화
	JobMailSend      JobType = "mail.send"
	JobMailBatch     JobType = "mail.batch"

	// AI jobs
	JobAIClassify      JobType = "ai.classify"
	JobAIClassifyBatch JobType = "ai.classify.batch"
	JobAISummarize     JobType = "ai.summarize"
	JobAIReply         JobType = "ai.reply"

	// RAG jobs
	JobRAGIndex      JobType = "rag.index"
	JobRAGBatchIndex JobType = "rag.batch_index"

	// Calendar jobs
	JobCalendarSync JobType = "calendar.sync"

	// Profile jobs
	JobProfileAnalyze JobType = "profile.analyze"
)

// =============================================================================
// HistoryChange - Gmail History API 변경사항
// =============================================================================

type HistoryChange struct {
	Type          ChangeType `json:"type"`
	MessageID     string     `json:"message_id"`
	ThreadID      string     `json:"thread_id,omitempty"`
	LabelIDs      []string   `json:"label_ids,omitempty"`
	AddedLabels   []string   `json:"added_labels,omitempty"`
	RemovedLabels []string   `json:"removed_labels,omitempty"`
}

type ChangeType string

const (
	ChangeTypeAdded        ChangeType = "messageAdded"
	ChangeTypeDeleted      ChangeType = "messageDeleted"
	ChangeTypeLabelAdded   ChangeType = "labelAdded"
	ChangeTypeLabelRemoved ChangeType = "labelRemoved"
)

// =============================================================================
// Retry Strategy - 재시도 전략
// =============================================================================

// RetryDelays - 재시도 간격 (백오프)
var RetryDelays = []time.Duration{
	30 * time.Second, // 1차: 30초
	1 * time.Minute,  // 2차: 1분
	5 * time.Minute,  // 3차: 5분
	15 * time.Minute, // 4차: 15분
	30 * time.Minute, // 5차: 30분
}

// GetRetryDelay - 재시도 횟수에 따른 대기 시간
func GetRetryDelay(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount >= len(RetryDelays) {
		return RetryDelays[len(RetryDelays)-1]
	}
	return RetryDelays[retryCount]
}
