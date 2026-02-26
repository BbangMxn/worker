package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
)

// Intent represents user's intention from chat message
type Intent struct {
	Type       IntentType     `json:"type"`
	Action     string         `json:"action"`
	Entities   map[string]any `json:"entities"`
	Confidence float64        `json:"confidence"`
}

type IntentType string

const (
	IntentTypeCalendar IntentType = "calendar"
	IntentTypeMail     IntentType = "mail"
	IntentTypeGeneral  IntentType = "general"
)

// CalendarIntent represents parsed calendar-related intent
type CalendarIntent struct {
	Action      string   `json:"action"`                // create, list, update, delete, suggest_time
	Provider    string   `json:"provider,omitempty"`    // google, outlook, all (default: all)
	CalendarID  int64    `json:"calendar_id,omitempty"` // specific calendar ID if known
	Title       string   `json:"title,omitempty"`
	Date        string   `json:"date,omitempty"`     // "tomorrow", "2024-01-15", "next monday"
	Time        string   `json:"time,omitempty"`     // "3pm", "15:00", "afternoon"
	Duration    int      `json:"duration,omitempty"` // minutes
	Location    string   `json:"location,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
	Description string   `json:"description,omitempty"`
	StartDate   string   `json:"start_date,omitempty"` // for list action
	EndDate     string   `json:"end_date,omitempty"`   // for list action
}

// EventProposal represents a proposed calendar event
type EventProposal struct {
	Provider    string    `json:"provider"`              // google, outlook
	CalendarID  int64     `json:"calendar_id,omitempty"` // target calendar
	Title       string    `json:"title"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Location    string    `json:"location,omitempty"`
	Attendees   []string  `json:"attendees,omitempty"`
	Description string    `json:"description,omitempty"`
	Confidence  float64   `json:"confidence"`
}

// TimeSlotSuggestion represents available time slots
type TimeSlotSuggestion struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Score     float64   `json:"score"` // preference score
}

// DetectIntent analyzes user message and detects intent
func (c *Client) DetectIntent(ctx context.Context, message string) (*Intent, error) {
	systemPrompt := `You are an intent detection AI. Analyze the user message and detect their intention.

Respond with JSON only:
{
  "type": "calendar|mail|general",
  "action": "specific action (create, list, send, search, etc.)",
  "entities": {extracted entities like date, time, title, recipients, etc.},
  "confidence": 0.0-1.0
}

For calendar intents, extract: title, date, time, duration, location, attendees
For mail intents, extract: recipients, subject, action (send, search, reply)
For general: just provide the type as "general"`

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, message)
	if err != nil {
		return nil, err
	}

	var intent Intent
	resp = cleanJSONResponse(resp)
	if err := json.Unmarshal([]byte(resp), &intent); err != nil {
		// Default to general intent
		return &Intent{Type: IntentTypeGeneral, Confidence: 0.5}, nil
	}

	return &intent, nil
}

// ParseCalendarIntent extracts calendar-specific intent details
func (c *Client) ParseCalendarIntent(ctx context.Context, message string, currentTime time.Time) (*CalendarIntent, error) {
	systemPrompt := fmt.Sprintf(`You are a calendar intent parser. Current time: %s

Parse the user message and extract calendar-related information.
Respond with JSON only:
{
  "action": "create|list|update|delete|suggest_time|find_free",
  "provider": "google|outlook|all",
  "title": "event title if mentioned",
  "date": "date in YYYY-MM-DD format or relative like 'tomorrow'",
  "time": "time in HH:MM format (24h)",
  "duration": duration in minutes (default 60),
  "location": "location if mentioned",
  "attendees": ["email1", "email2"],
  "description": "description if mentioned",
  "start_date": "for list action, start of range",
  "end_date": "for list action, end of range"
}

Provider detection:
- "구글 캘린더", "Google Calendar", "구글에" → "google"
- "아웃룩", "Outlook", "마이크로소프트" → "outlook"
- No specific mention → "all" (show/create on all connected calendars)

Convert relative dates:
- "tomorrow", "내일" → actual date
- "next monday", "다음주 월요일" → actual date
- "this week", "이번 주" → start_date and end_date
- "3pm", "오후 3시" → "15:00"`, currentTime.Format("2006-01-02 15:04"))

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, message)
	if err != nil {
		return nil, err
	}

	var intent CalendarIntent
	resp = cleanJSONResponse(resp)
	if err := json.Unmarshal([]byte(resp), &intent); err != nil {
		return nil, fmt.Errorf("failed to parse calendar intent: %w", err)
	}

	return &intent, nil
}

// GenerateEventProposal creates an event proposal from calendar intent
func (c *Client) GenerateEventProposal(ctx context.Context, intent *CalendarIntent, currentTime time.Time) (*EventProposal, error) {
	// Parse date and time
	startTime, endTime, err := parseDateTime(intent.Date, intent.Time, intent.Duration, currentTime)
	if err != nil {
		return nil, err
	}

	// Default to "all" if no provider specified
	provider := intent.Provider
	if provider == "" {
		provider = "all"
	}

	return &EventProposal{
		Provider:    provider,
		CalendarID:  intent.CalendarID,
		Title:       intent.Title,
		StartTime:   startTime,
		EndTime:     endTime,
		Location:    intent.Location,
		Attendees:   intent.Attendees,
		Description: intent.Description,
		Confidence:  0.9,
	}, nil
}

// SuggestTimeSlots suggests available time slots based on existing events
func (c *Client) SuggestTimeSlots(ctx context.Context, existingEvents []EventSummary, preferences TimePreferences) ([]TimeSlotSuggestion, error) {
	systemPrompt := `You are a scheduling assistant. Given existing calendar events and user preferences, suggest optimal meeting times.

Respond with JSON array of suggested time slots:
[
  {"start_time": "2024-01-15T09:00:00Z", "end_time": "2024-01-15T10:00:00Z", "score": 0.9},
  {"start_time": "2024-01-15T14:00:00Z", "end_time": "2024-01-15T15:00:00Z", "score": 0.8}
]

Consider:
- Avoid conflicts with existing events
- Prefer user's preferred hours
- Leave buffer time between meetings
- Score higher for morning slots if user prefers mornings`

	eventsJSON, _ := json.Marshal(existingEvents)
	prefsJSON, _ := json.Marshal(preferences)
	userPrompt := fmt.Sprintf("Existing events: %s\nPreferences: %s\nSuggest 3-5 optimal time slots.", eventsJSON, prefsJSON)

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	var suggestions []TimeSlotSuggestion
	resp = cleanJSONResponse(resp)
	if err := json.Unmarshal([]byte(resp), &suggestions); err != nil {
		return nil, err
	}

	return suggestions, nil
}

// FormatCalendarResponse formats calendar data into natural language
func (c *Client) FormatCalendarResponse(ctx context.Context, action string, events []EventSummary, proposal *EventProposal) (string, error) {
	systemPrompt := `You are a helpful calendar assistant. Format the calendar information in a natural, conversational way.
Be concise but informative. Use Korean if the original query was in Korean.`

	var userPrompt string
	switch action {
	case "list":
		eventsJSON, _ := json.Marshal(events)
		userPrompt = fmt.Sprintf("Format these calendar events in a friendly list:\n%s", eventsJSON)
	case "create":
		proposalJSON, _ := json.Marshal(proposal)
		userPrompt = fmt.Sprintf("Confirm this event creation proposal:\n%s", proposalJSON)
	default:
		userPrompt = "Provide a helpful response about the calendar action."
	}

	return c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
}

// Helper types
type EventSummary struct {
	Title     string    `json:"title"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Location  string    `json:"location,omitempty"`
}

type TimePreferences struct {
	PreferredStartHour int      `json:"preferred_start_hour"` // e.g., 9
	PreferredEndHour   int      `json:"preferred_end_hour"`   // e.g., 18
	PreferredDays      []string `json:"preferred_days"`       // ["monday", "tuesday", ...]
	MeetingDuration    int      `json:"meeting_duration"`     // minutes
	BufferMinutes      int      `json:"buffer_minutes"`       // between meetings
}

// Helper functions
func cleanJSONResponse(resp string) string {
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	return strings.TrimSpace(resp)
}

func parseDateTime(dateStr, timeStr string, duration int, currentTime time.Time) (time.Time, time.Time, error) {
	if duration == 0 {
		duration = 60 // default 1 hour
	}

	// Parse date
	var date time.Time
	switch strings.ToLower(dateStr) {
	case "today", "오늘":
		date = currentTime
	case "tomorrow", "내일":
		date = currentTime.AddDate(0, 0, 1)
	case "next week", "다음주":
		date = currentTime.AddDate(0, 0, 7)
	default:
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			date = currentTime.AddDate(0, 0, 1) // default to tomorrow
		} else {
			date = parsed
		}
	}

	// Parse time
	hour, minute := 9, 0 // default 9am
	if timeStr != "" {
		parsed, err := time.Parse("15:04", timeStr)
		if err == nil {
			hour = parsed.Hour()
			minute = parsed.Minute()
		}
	}

	startTime := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, currentTime.Location())
	endTime := startTime.Add(time.Duration(duration) * time.Minute)

	return startTime, endTime, nil
}
