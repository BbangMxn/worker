package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Notification - 사용자 알림 (DB 저장용)
// =============================================================================

type Notification struct {
	ID         int64                `json:"id"`
	UserID     uuid.UUID            `json:"user_id"`
	Type       NotificationType     `json:"type"`
	Title      string               `json:"title"`
	Body       string               `json:"body,omitempty"`
	Data       map[string]any       `json:"data,omitempty"`
	EntityType string               `json:"entity_type,omitempty"`
	EntityID   int64                `json:"entity_id,omitempty"`
	IsRead     bool                 `json:"is_read"`
	ReadAt     *time.Time           `json:"read_at,omitempty"`
	Priority   NotificationPriority `json:"priority"`
	CreatedAt  time.Time            `json:"created_at"`
	ExpiresAt  *time.Time           `json:"expires_at,omitempty"`
}

type NotificationType string

const (
	NotificationTypeEmail    NotificationType = "email"
	NotificationTypeCalendar NotificationType = "calendar"
	NotificationTypeSystem   NotificationType = "system"
	NotificationTypeSync     NotificationType = "sync"
	NotificationTypeAI       NotificationType = "ai"
)

type NotificationPriority string

const (
	NotificationPriorityLow    NotificationPriority = "low"
	NotificationPriorityNormal NotificationPriority = "normal"
	NotificationPriorityHigh   NotificationPriority = "high"
	NotificationPriorityUrgent NotificationPriority = "urgent"
)

// NotificationFilter - 알림 조회 필터
type NotificationFilter struct {
	UserID     uuid.UUID
	Type       *NotificationType
	Priority   *NotificationPriority
	IsRead     *bool
	EntityType string
	EntityID   int64
	Limit      int
	Offset     int
}

// NotificationRepository - 알림 저장소 인터페이스
type NotificationRepository interface {
	Create(notification *Notification) error
	GetByID(id int64) (*Notification, error)
	List(filter *NotificationFilter) ([]*Notification, int, error)
	MarkAsRead(id int64) error
	MarkAllAsRead(userID uuid.UUID) error
	Delete(id int64) error
	DeleteByUserID(userID uuid.UUID) error
	CountUnread(userID uuid.UUID) (int64, error)
}

// =============================================================================
// RealtimeEvent - SSE로 프론트엔드에 전송되는 이벤트
// =============================================================================

// RealtimeEvent - SSE로 프론트엔드에 전송되는 이벤트
type RealtimeEvent struct {
	Type      EventType   `json:"type"`
	Seq       int64       `json:"seq"` // 순서 보장용 시퀀스 번호
	UserID    string      `json:"-"`   // 전송 대상 (JSON 제외)
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

type EventType string

const (
	// Email events
	EventNewEmail        EventType = "email.new"
	EventEmailClassified EventType = "email.classified"
	EventEmailSummarized EventType = "email.summarized"
	EventEmailUpdated    EventType = "email.updated"
	EventEmailDeleted    EventType = "email.deleted"

	// Email state change events (실시간 UI 동기화)
	EventEmailRead       EventType = "email.read"
	EventEmailUnread     EventType = "email.unread"
	EventEmailStarred    EventType = "email.starred"
	EventEmailUnstarred  EventType = "email.unstarred"
	EventEmailArchived   EventType = "email.archived"
	EventEmailTrashed    EventType = "email.trashed"
	EventEmailMoved      EventType = "email.moved"
	EventEmailLabeled    EventType = "email.labeled"
	EventEmailSnoozed    EventType = "email.snoozed"
	EventEmailUnsnoozed  EventType = "email.unsnoozed"
	EventEmailBatchState EventType = "email.batch_state" // 일괄 상태 변경

	// Sync events
	EventSyncStarted    EventType = "sync.started"
	EventSyncFirstBatch EventType = "sync.first_batch" // Phase 1: 첫 50개 완료
	EventSyncProgress   EventType = "sync.progress"
	EventSyncCompleted  EventType = "sync.completed"
	EventSyncError      EventType = "sync.error"
	EventSyncRetry      EventType = "sync.retry" // 재시도 예약됨

	// OAuth events
	EventTokenExpired EventType = "oauth.token_expired" // 토큰 만료 - 재연결 필요

	// Calendar events
	EventCalendarUpdated       EventType = "calendar.updated"
	EventCalendarSyncCompleted EventType = "calendar.sync_completed"
	EventCalendarEventCreated  EventType = "calendar.event_created"
	EventCalendarEventDeleted  EventType = "calendar.event_deleted"

	// System events
	EventConnected    EventType = "connected"
	EventDisconnected EventType = "disconnected"
)

// NewEmailData - 새 메일 이벤트 데이터
type NewEmailData struct {
	EmailID   int64  `json:"email_id"`
	Subject   string `json:"subject"`
	From      string `json:"from"`
	FromName  string `json:"from_name,omitempty"`
	Snippet   string `json:"snippet"`
	Folder    string `json:"folder"`
	IsRead    bool   `json:"is_read"`
	HasAttach bool   `json:"has_attachments"`
}

// ClassifiedData - AI 분류 완료 이벤트 데이터

// FullEmailData - 전체 이메일 데이터 (Push-centric SSE용)
// 서버가 body까지 포함한 전체 데이터를 푸시하여 클라이언트 API 호출 제거
type FullEmailData struct {
	EmailID      int64    `json:"email_id"`
	ProviderID   string   `json:"provider_id"`
	ThreadID     string   `json:"thread_id,omitempty"`
	Subject      string   `json:"subject"`
	From         string   `json:"from"`
	FromName     string   `json:"from_name,omitempty"`
	To           []string `json:"to,omitempty"`
	Cc           []string `json:"cc,omitempty"`
	Snippet      string   `json:"snippet"`
	Body         string   `json:"body,omitempty"`      // HTML body
	BodyText     string   `json:"body_text,omitempty"` // Plain text body
	Folder       string   `json:"folder"`
	LabelIDs     []string `json:"label_ids,omitempty"`
	IsRead       bool     `json:"is_read"`
	IsStarred    bool     `json:"is_starred"`
	HasAttach    bool     `json:"has_attachments"`
	ReceivedAt   string   `json:"received_at"`
	ConnectionID int64    `json:"connection_id"`
}
type ClassifiedData struct {
	EmailID    int64   `json:"email_id"`
	Category   string  `json:"category"`
	Priority   string  `json:"priority"`
	Intent     string  `json:"intent,omitempty"`
	Summary    string  `json:"summary,omitempty"`
	Confidence float64 `json:"confidence"`
}

// SyncProgressData - 동기화 진행 상황
type SyncProgressData struct {
	ConnectionID int64  `json:"connection_id"`
	Total        int    `json:"total,omitempty"`
	Current      int    `json:"current,omitempty"`
	Status       string `json:"status"`
	Phase        string `json:"phase,omitempty"` // initial_first_batch, initial_remaining, delta, gap
	RetryCount   int    `json:"retry_count,omitempty"`
	NextRetryAt  string `json:"next_retry_at,omitempty"`
}

// SummarizedData - AI 요약 완료 이벤트 데이터
type SummarizedData struct {
	EmailID int64  `json:"email_id"`
	Summary string `json:"summary"`
}

// =============================================================================
// EmailStateChangeData - 이메일 상태 변경 이벤트 데이터 (실시간 UI 동기화)
// =============================================================================

// EmailStateChangeData - 단일 이메일 상태 변경
type EmailStateChangeData struct {
	EmailID      int64  `json:"email_id"`
	ConnectionID int64  `json:"connection_id,omitempty"`
	Action       string `json:"action"` // read, unread, starred, unstarred, archived, trashed, moved, labeled, snoozed, unsnoozed

	// 변경된 값들 (선택적)
	IsRead    *bool   `json:"is_read,omitempty"`
	IsStarred *bool   `json:"is_starred,omitempty"`
	Folder    *string `json:"folder,omitempty"`
	Labels    *struct {
		Added   []string `json:"added,omitempty"`
		Removed []string `json:"removed,omitempty"`
	} `json:"labels,omitempty"`
	SnoozeUntil *time.Time `json:"snooze_until,omitempty"`

	// 타임스탬프
	Timestamp time.Time `json:"timestamp"`
}

// EmailBatchStateChangeData - 일괄 상태 변경
type EmailBatchStateChangeData struct {
	EmailIDs     []int64 `json:"email_ids"`
	ConnectionID int64   `json:"connection_id,omitempty"`
	Action       string  `json:"action"`

	// 변경된 값들
	IsRead      *bool      `json:"is_read,omitempty"`
	IsStarred   *bool      `json:"is_starred,omitempty"`
	Folder      *string    `json:"folder,omitempty"`
	SnoozeUntil *time.Time `json:"snooze_until,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// NewEmailStateChangeEvent creates a new email state change event.
func NewEmailStateChangeEvent(userID string, data *EmailStateChangeData) *RealtimeEvent {
	eventType := EmailActionToEventType(data.Action)
	return &RealtimeEvent{
		Type:      eventType,
		UserID:    userID,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewEmailBatchStateChangeEvent creates a batch state change event.
func NewEmailBatchStateChangeEvent(userID string, data *EmailBatchStateChangeData) *RealtimeEvent {
	return &RealtimeEvent{
		Type:      EventEmailBatchState,
		UserID:    userID,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// EmailActionToEventType converts action string to EventType.
func EmailActionToEventType(action string) EventType {
	switch action {
	case "read":
		return EventEmailRead
	case "unread":
		return EventEmailUnread
	case "starred":
		return EventEmailStarred
	case "unstarred":
		return EventEmailUnstarred
	case "archived":
		return EventEmailArchived
	case "trashed":
		return EventEmailTrashed
	case "moved":
		return EventEmailMoved
	case "labeled":
		return EventEmailLabeled
	case "snoozed":
		return EventEmailSnoozed
	case "unsnoozed":
		return EventEmailUnsnoozed
	default:
		return EventEmailUpdated
	}
}

// =============================================================================
// NotificationSettings - 사용자별 알림 설정
// =============================================================================

// NotificationSettings represents user notification preferences.
type NotificationSettings struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// 채널별 활성화
	PushEnabled    bool `json:"push_enabled"`    // 모바일 푸시
	EmailEnabled   bool `json:"email_enabled"`   // 이메일 알림
	DesktopEnabled bool `json:"desktop_enabled"` // 데스크톱 알림
	InAppEnabled   bool `json:"in_app_enabled"`  // 인앱 알림 (기본 true)

	// 이메일 알림 설정
	NotifyNewEmail      bool `json:"notify_new_email"`      // 새 메일 알림
	NotifyImportantOnly bool `json:"notify_important_only"` // 중요 메일만
	NotifyFromVIPOnly   bool `json:"notify_from_vip_only"`  // VIP 발신자만
	NotifyMentions      bool `json:"notify_mentions"`       // 멘션된 경우만

	// 캘린더 알림 설정
	NotifyCalendarEvents   bool `json:"notify_calendar_events"`   // 일정 알림
	NotifyCalendarReminder bool `json:"notify_calendar_reminder"` // 일정 리마인더
	ReminderMinutesBefore  int  `json:"reminder_minutes_before"`  // 몇 분 전 알림 (기본 15)

	// 동기화 알림 설정
	NotifySyncComplete bool `json:"notify_sync_complete"` // 동기화 완료 알림
	NotifySyncError    bool `json:"notify_sync_error"`    // 동기화 에러 알림

	// AI 알림 설정
	NotifyAIClassified bool `json:"notify_ai_classified"` // AI 분류 완료
	NotifyAISummarized bool `json:"notify_ai_summarized"` // AI 요약 완료

	// 방해 금지 시간 (Quiet Hours)
	QuietHoursEnabled  bool   `json:"quiet_hours_enabled"`
	QuietHoursStart    string `json:"quiet_hours_start"`    // HH:MM 형식 (예: "22:00")
	QuietHoursEnd      string `json:"quiet_hours_end"`      // HH:MM 형식 (예: "08:00")
	QuietHoursTimezone string `json:"quiet_hours_timezone"` // 타임존 (예: "Asia/Seoul")

	// VIP 발신자 (이 목록의 발신자는 항상 알림)
	VIPSenders []string `json:"vip_senders"` // 이메일 주소 목록

	// 뮤트된 스레드 (이 스레드는 알림 안 함)
	MutedThreadIDs []int64 `json:"muted_thread_ids"`

	// 뮤트된 발신자 (이 발신자는 알림 안 함)
	MutedSenders []string `json:"muted_senders"`

	// 우선순위 필터 (이 우선순위 이상만 알림)
	MinPriorityForNotification int `json:"min_priority_for_notification"` // 1=urgent, 2=high, 3=normal, 4=low (기본 3)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultNotificationSettings returns default notification settings for new users.
func DefaultNotificationSettings(userID uuid.UUID) *NotificationSettings {
	return &NotificationSettings{
		UserID:         userID,
		PushEnabled:    true,
		EmailEnabled:   false,
		DesktopEnabled: true,
		InAppEnabled:   true,

		NotifyNewEmail:      true,
		NotifyImportantOnly: false,
		NotifyFromVIPOnly:   false,
		NotifyMentions:      true,

		NotifyCalendarEvents:   true,
		NotifyCalendarReminder: true,
		ReminderMinutesBefore:  15,

		NotifySyncComplete: false,
		NotifySyncError:    true,

		NotifyAIClassified: false,
		NotifyAISummarized: false,

		QuietHoursEnabled:  false,
		QuietHoursStart:    "22:00",
		QuietHoursEnd:      "08:00",
		QuietHoursTimezone: "Asia/Seoul",

		VIPSenders:     []string{},
		MutedThreadIDs: []int64{},
		MutedSenders:   []string{},

		MinPriorityForNotification: 3, // normal 이상
	}
}

// ShouldNotify checks if a notification should be sent based on settings.
func (s *NotificationSettings) ShouldNotify(notifType NotificationType, priority NotificationPriority, senderEmail string, threadID int64) bool {
	// 인앱 알림이 꺼져있으면 알림 안 함
	if !s.InAppEnabled {
		return false
	}

	// 방해 금지 시간 체크
	if s.QuietHoursEnabled && s.isInQuietHours() {
		// VIP 발신자는 방해 금지 시간에도 알림
		if !s.isVIPSender(senderEmail) {
			return false
		}
	}

	// 뮤트된 발신자 체크
	if s.isMutedSender(senderEmail) {
		return false
	}

	// 뮤트된 스레드 체크
	if threadID > 0 && s.isMutedThread(threadID) {
		return false
	}

	// 타입별 체크
	switch notifType {
	case NotificationTypeEmail:
		if !s.NotifyNewEmail {
			return false
		}
		// VIP만 알림 설정
		if s.NotifyFromVIPOnly && !s.isVIPSender(senderEmail) {
			return false
		}
		// 중요 메일만 알림 설정
		if s.NotifyImportantOnly && priority != NotificationPriorityUrgent && priority != NotificationPriorityHigh {
			return false
		}

	case NotificationTypeCalendar:
		if !s.NotifyCalendarEvents {
			return false
		}

	case NotificationTypeSync:
		// 동기화 에러는 항상 알림 (설정에 따라)
		if priority == NotificationPriorityHigh || priority == NotificationPriorityUrgent {
			return s.NotifySyncError
		}
		return s.NotifySyncComplete

	case NotificationTypeAI:
		// AI 알림은 기본적으로 안 보냄 (설정에 따라)
		return s.NotifyAIClassified || s.NotifyAISummarized
	}

	// 우선순위 체크
	priorityLevel := s.getPriorityLevel(priority)
	if priorityLevel > s.MinPriorityForNotification {
		return false
	}

	return true
}

// isInQuietHours checks if current time is within quiet hours.
func (s *NotificationSettings) isInQuietHours() bool {
	if s.QuietHoursStart == "" || s.QuietHoursEnd == "" {
		return false
	}

	loc, err := time.LoadLocation(s.QuietHoursTimezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	currentMinutes := now.Hour()*60 + now.Minute()

	startHour, startMin := parseTime(s.QuietHoursStart)
	endHour, endMin := parseTime(s.QuietHoursEnd)

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin

	// 자정을 넘어가는 경우 (예: 22:00 ~ 08:00)
	if startMinutes > endMinutes {
		return currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	// 같은 날인 경우 (예: 13:00 ~ 14:00)
	return currentMinutes >= startMinutes && currentMinutes < endMinutes
}

// parseTime parses HH:MM format.
func parseTime(timeStr string) (hour, min int) {
	if len(timeStr) < 5 {
		return 0, 0
	}
	_, _ = fmt.Sscanf(timeStr, "%d:%d", &hour, &min)
	return
}

// isVIPSender checks if sender is in VIP list.
func (s *NotificationSettings) isVIPSender(email string) bool {
	for _, vip := range s.VIPSenders {
		if vip == email {
			return true
		}
	}
	return false
}

// isMutedSender checks if sender is muted.
func (s *NotificationSettings) isMutedSender(email string) bool {
	for _, muted := range s.MutedSenders {
		if muted == email {
			return true
		}
	}
	return false
}

// isMutedThread checks if thread is muted.
func (s *NotificationSettings) isMutedThread(threadID int64) bool {
	for _, id := range s.MutedThreadIDs {
		if id == threadID {
			return true
		}
	}
	return false
}

// getPriorityLevel converts priority to numeric level.
func (s *NotificationSettings) getPriorityLevel(priority NotificationPriority) int {
	switch priority {
	case NotificationPriorityUrgent:
		return 1
	case NotificationPriorityHigh:
		return 2
	case NotificationPriorityNormal:
		return 3
	case NotificationPriorityLow:
		return 4
	default:
		return 3
	}
}

// NotificationSettingsRepository defines the repository interface.
type NotificationSettingsRepository interface {
	Get(ctx context.Context, userID uuid.UUID) (*NotificationSettings, error)
	Create(ctx context.Context, settings *NotificationSettings) error
	Update(ctx context.Context, settings *NotificationSettings) error
	UpdatePartial(ctx context.Context, userID uuid.UUID, updates map[string]any) error

	// VIP/Mute 관리
	AddVIPSender(ctx context.Context, userID uuid.UUID, email string) error
	RemoveVIPSender(ctx context.Context, userID uuid.UUID, email string) error
	AddMutedSender(ctx context.Context, userID uuid.UUID, email string) error
	RemoveMutedSender(ctx context.Context, userID uuid.UUID, email string) error
	AddMutedThread(ctx context.Context, userID uuid.UUID, threadID int64) error
	RemoveMutedThread(ctx context.Context, userID uuid.UUID, threadID int64) error
}
