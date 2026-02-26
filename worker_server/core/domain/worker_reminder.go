package domain

import (
	"time"

	"github.com/google/uuid"
)

// ReminderSourceType represents what the reminder is for
type ReminderSourceType string

const (
	ReminderSourceTodo     ReminderSourceType = "todo"
	ReminderSourceCalendar ReminderSourceType = "calendar"
	ReminderSourceEmail    ReminderSourceType = "email"
	ReminderSourceContact  ReminderSourceType = "contact"
	ReminderSourceCustom   ReminderSourceType = "custom"
)

// ReminderStatus represents the status of a reminder
type ReminderStatus string

const (
	ReminderStatusPending   ReminderStatus = "pending"
	ReminderStatusSent      ReminderStatus = "sent"
	ReminderStatusCancelled ReminderStatus = "cancelled"
	ReminderStatusSnoozed   ReminderStatus = "snoozed"
)

// ReminderChannel represents delivery channel
type ReminderChannel string

const (
	ReminderChannelPush    ReminderChannel = "push"
	ReminderChannelEmail   ReminderChannel = "email"
	ReminderChannelWebhook ReminderChannel = "webhook"
)

// ReminderTriggerType represents when the reminder triggers
type ReminderTriggerType string

const (
	ReminderTriggerBeforeDue   ReminderTriggerType = "before_due"
	ReminderTriggerAfterCreate ReminderTriggerType = "after_create"
	ReminderTriggerDaily       ReminderTriggerType = "daily"
	ReminderTriggerOnce        ReminderTriggerType = "once"
)

// Reminder represents an actual reminder to be sent
type Reminder struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Rule reference (nullable - manual reminders have no rule)
	RuleID *int64 `json:"rule_id,omitempty"`

	// Source linking (polymorphic)
	SourceType ReminderSourceType `json:"source_type"`
	SourceID   *string            `json:"source_id,omitempty"`

	// Content
	Title   string  `json:"title"`
	Message *string `json:"message,omitempty"`
	URL     *string `json:"url,omitempty"`

	// Schedule
	RemindAt time.Time `json:"remind_at"`
	Timezone string    `json:"timezone"`

	// Delivery
	Channels []ReminderChannel `json:"channels"`

	// Status
	Status       ReminderStatus `json:"status"`
	SentAt       *time.Time     `json:"sent_at,omitempty"`
	SnoozedUntil *time.Time     `json:"snoozed_until,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReminderRule represents user-defined reminder rules
type ReminderRule struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Basic info
	Name      string `json:"name"`
	IsEnabled bool   `json:"is_enabled"`

	// Target
	TargetType ReminderSourceType `json:"target_type"` // todo, calendar, email

	// Conditions (optional filters)
	Conditions map[string]interface{} `json:"conditions,omitempty"` // {"priority": [1,2], "area": "work"}

	// Trigger
	TriggerType   ReminderTriggerType `json:"trigger_type"`
	OffsetMinutes *int                `json:"offset_minutes,omitempty"` // for before_due
	TriggerTime   *string             `json:"trigger_time,omitempty"`   // for daily: "09:00"

	// Delivery
	Channels []ReminderChannel `json:"channels"`

	// Ordering
	SortOrder int `json:"sort_order"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReminderFilter represents filter options for listing reminders
type ReminderFilter struct {
	UserID     uuid.UUID
	Status     *ReminderStatus
	Statuses   []ReminderStatus
	SourceType *ReminderSourceType
	SourceID   *string
	RuleID     *int64

	// Time filters
	RemindAtFrom *time.Time
	RemindAtTo   *time.Time
	Pending      bool // shortcut for status=pending AND remind_at <= now

	// Pagination
	Limit  int
	Offset int

	// Sorting
	SortBy    string // remind_at, created_at
	SortOrder string // asc, desc
}

// ReminderRuleFilter represents filter options for listing rules
type ReminderRuleFilter struct {
	UserID     uuid.UUID
	TargetType *ReminderSourceType
	IsEnabled  *bool

	Limit  int
	Offset int
}

// IsPending returns true if the reminder is pending
func (r *Reminder) IsPending() bool {
	return r.Status == ReminderStatusPending
}

// IsDue returns true if the reminder should be sent now
func (r *Reminder) IsDue() bool {
	if !r.IsPending() {
		return false
	}
	return r.RemindAt.Before(time.Now()) || r.RemindAt.Equal(time.Now())
}

// MarkSent marks the reminder as sent
func (r *Reminder) MarkSent() {
	now := time.Now()
	r.Status = ReminderStatusSent
	r.SentAt = &now
	r.UpdatedAt = now
}

// Cancel cancels the reminder
func (r *Reminder) Cancel() {
	r.Status = ReminderStatusCancelled
	r.UpdatedAt = time.Now()
}

// Snooze postpones the reminder
func (r *Reminder) Snooze(until time.Time) {
	r.Status = ReminderStatusSnoozed
	r.SnoozedUntil = &until
	r.RemindAt = until
	r.UpdatedAt = time.Now()
}

// Unsnooze reactivates a snoozed reminder
func (r *Reminder) Unsnooze() {
	if r.Status == ReminderStatusSnoozed {
		r.Status = ReminderStatusPending
		r.SnoozedUntil = nil
		r.UpdatedAt = time.Now()
	}
}

// MatchesConditions checks if a todo matches the rule's conditions
func (r *ReminderRule) MatchesConditions(todo *Todo) bool {
	if r.Conditions == nil || len(r.Conditions) == 0 {
		return true
	}

	// Check priority condition
	if priorities, ok := r.Conditions["priority"].([]interface{}); ok {
		matched := false
		for _, p := range priorities {
			if int(p.(float64)) == int(todo.Priority) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check area condition
	if area, ok := r.Conditions["area"].(string); ok {
		if todo.Area == nil || *todo.Area != area {
			return false
		}
	}

	// Check project_id condition
	if projectID, ok := r.Conditions["project_id"].(float64); ok {
		if todo.ProjectID == nil || *todo.ProjectID != int64(projectID) {
			return false
		}
	}

	return true
}
