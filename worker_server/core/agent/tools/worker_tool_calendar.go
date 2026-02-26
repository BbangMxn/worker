package tools

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// CalendarListTool lists calendar events
type CalendarListTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarListTool(calendarRepo domain.CalendarRepository) *CalendarListTool {
	return &CalendarListTool{calendarRepo: calendarRepo}
}

func (t *CalendarListTool) Name() string           { return "calendar.list" }
func (t *CalendarListTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarListTool) Description() string {
	return "List calendar events. Can filter by provider (google/outlook), date range, and limit."
}

func (t *CalendarListTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "provider", Type: "string", Description: "Calendar provider: google, outlook, or all", Enum: []string{"google", "outlook", "all"}, Default: "all"},
		{Name: "start_date", Type: "string", Description: "Start date (YYYY-MM-DD or 'today', 'tomorrow', 'this week')"},
		{Name: "end_date", Type: "string", Description: "End date (YYYY-MM-DD)"},
		{Name: "limit", Type: "number", Description: "Maximum events to return", Default: 10},
	}
}

func (t *CalendarListTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	provider := getStringArg(args, "provider", "all")
	startDateStr := getStringArg(args, "start_date", "today")
	endDateStr := getStringArg(args, "end_date", "")
	limit := getIntArg(args, "limit", 10)

	// Parse dates
	now := time.Now()
	startTime := parseRelativeDate(startDateStr, now)
	endTime := startTime.AddDate(0, 0, 7) // Default 1 week
	if endDateStr != "" {
		endTime = parseRelativeDate(endDateStr, now).AddDate(0, 0, 1)
	}

	// Get calendars by provider
	calendars, err := t.getCalendarsByProvider(userID, provider)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Collect events
	var allEvents []*domain.CalendarEvent
	for _, cal := range calendars {
		filter := &domain.CalendarEventFilter{
			UserID:     userID,
			CalendarID: &cal.ID,
			StartTime:  &startTime,
			EndTime:    &endTime,
			Limit:      limit,
		}
		events, _, err := t.calendarRepo.ListEvents(filter)
		if err != nil {
			continue
		}
		allEvents = append(allEvents, events...)
	}

	return &ToolResult{
		Success: true,
		Data:    allEvents,
		Message: fmt.Sprintf("Found %d events", len(allEvents)),
	}, nil
}

func (t *CalendarListTool) getCalendarsByProvider(userID uuid.UUID, provider string) ([]*domain.Calendar, error) {
	calendars, err := t.calendarRepo.GetCalendarsByUser(userID)
	if err != nil {
		return nil, err
	}

	if provider == "all" || provider == "" {
		return calendars, nil
	}

	var filtered []*domain.Calendar
	for _, cal := range calendars {
		if string(cal.Provider) == provider {
			filtered = append(filtered, cal)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no %s calendar connected", provider)
	}
	return filtered, nil
}

// CalendarCreateTool creates a new calendar event
type CalendarCreateTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarCreateTool(calendarRepo domain.CalendarRepository) *CalendarCreateTool {
	return &CalendarCreateTool{calendarRepo: calendarRepo}
}

func (t *CalendarCreateTool) Name() string           { return "calendar.create" }
func (t *CalendarCreateTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarCreateTool) Description() string {
	return "Create a new calendar event. Specify provider to choose which calendar (google/outlook)."
}

func (t *CalendarCreateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "title", Type: "string", Description: "Event title", Required: true},
		{Name: "start_time", Type: "string", Description: "Start time (ISO 8601 or 'tomorrow 3pm')", Required: true},
		{Name: "end_time", Type: "string", Description: "End time (ISO 8601 or duration like '1h')", Required: true},
		{Name: "provider", Type: "string", Description: "Calendar provider: google or outlook", Enum: []string{"google", "outlook"}, Required: true},
		{Name: "description", Type: "string", Description: "Event description"},
		{Name: "location", Type: "string", Description: "Event location"},
		{Name: "attendees", Type: "array", Description: "List of attendee emails"},
	}
}

func (t *CalendarCreateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	title := getStringArg(args, "title", "")
	startTimeStr := getStringArg(args, "start_time", "")
	endTimeStr := getStringArg(args, "end_time", "")
	provider := getStringArg(args, "provider", "")
	description := getStringArg(args, "description", "")
	location := getStringArg(args, "location", "")
	attendees := getStringArrayArg(args, "attendees")

	// Parse times
	now := time.Now()
	startTime, err := parseFlexibleTime(startTimeStr, now)
	if err != nil {
		return &ToolResult{Success: false, Error: "invalid start_time format"}, nil
	}

	endTime, err := parseFlexibleTime(endTimeStr, now)
	if err != nil {
		// Try parsing as duration
		if duration, ok := parseDuration(endTimeStr); ok {
			endTime = startTime.Add(duration)
		} else {
			return &ToolResult{Success: false, Error: "invalid end_time format"}, nil
		}
	}

	// Get target calendar
	calendar, err := t.getDefaultCalendar(userID, provider)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Create proposal for confirmation
	proposal := &ActionProposal{
		ID:     uuid.New().String(),
		Action: "calendar.create",
		Description: fmt.Sprintf("Create event '%s' on %s (%s)",
			title,
			startTime.Format("2006-01-02 15:04"),
			provider),
		Data: map[string]any{
			"calendar_id": calendar.ID,
			"title":       title,
			"start_time":  startTime,
			"end_time":    endTime,
			"description": description,
			"location":    location,
			"attendees":   attendees,
			"provider":    provider,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to create event '%s' on %s calendar", title, provider),
		Proposal: proposal,
	}, nil
}

func (t *CalendarCreateTool) getDefaultCalendar(userID uuid.UUID, provider string) (*domain.Calendar, error) {
	calendars, err := t.calendarRepo.GetCalendarsByUser(userID)
	if err != nil {
		return nil, err
	}

	for _, cal := range calendars {
		if string(cal.Provider) == provider {
			if cal.IsDefault {
				return cal, nil
			}
		}
	}

	// Return first matching provider
	for _, cal := range calendars {
		if string(cal.Provider) == provider {
			return cal, nil
		}
	}

	return nil, fmt.Errorf("no %s calendar found", provider)
}

// CalendarFindFreeTool finds available time slots
type CalendarFindFreeTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarFindFreeTool(calendarRepo domain.CalendarRepository) *CalendarFindFreeTool {
	return &CalendarFindFreeTool{calendarRepo: calendarRepo}
}

func (t *CalendarFindFreeTool) Name() string           { return "calendar.find_free" }
func (t *CalendarFindFreeTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarFindFreeTool) Description() string {
	return "Find available time slots across calendars for scheduling meetings."
}

func (t *CalendarFindFreeTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "duration", Type: "number", Description: "Meeting duration in minutes", Required: true},
		{Name: "start_date", Type: "string", Description: "Start searching from this date", Default: "today"},
		{Name: "end_date", Type: "string", Description: "Search until this date", Default: "next week"},
		{Name: "provider", Type: "string", Description: "Check specific provider or all", Enum: []string{"google", "outlook", "all"}, Default: "all"},
		{Name: "working_hours_only", Type: "boolean", Description: "Only suggest 9am-6pm slots", Default: true},
	}
}

func (t *CalendarFindFreeTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	duration := getIntArg(args, "duration", 60)
	startDateStr := getStringArg(args, "start_date", "today")
	endDateStr := getStringArg(args, "end_date", "next week")
	provider := getStringArg(args, "provider", "all")
	workingHoursOnly := getBoolArg(args, "working_hours_only", true)

	now := time.Now()
	startDate := parseRelativeDate(startDateStr, now)
	endDate := parseRelativeDate(endDateStr, now)

	// Get all events in range
	calendars, _ := t.calendarRepo.GetCalendarsByUser(userID)
	var busySlots []timeSlot

	for _, cal := range calendars {
		if provider != "all" && string(cal.Provider) != provider {
			continue
		}

		filter := &domain.CalendarEventFilter{
			UserID:     userID,
			CalendarID: &cal.ID,
			StartTime:  &startDate,
			EndTime:    &endDate,
		}
		events, _, _ := t.calendarRepo.ListEvents(filter)
		for _, e := range events {
			busySlots = append(busySlots, timeSlot{start: e.StartTime, end: e.EndTime})
		}
	}

	// Find free slots
	freeSlots := findFreeSlots(startDate, endDate, busySlots, time.Duration(duration)*time.Minute, workingHoursOnly)

	return &ToolResult{
		Success: true,
		Data:    freeSlots,
		Message: fmt.Sprintf("Found %d available time slots", len(freeSlots)),
	}, nil
}

type timeSlot struct {
	start time.Time
	end   time.Time
}

func findFreeSlots(start, end time.Time, busy []timeSlot, duration time.Duration, workingHoursOnly bool) []timeSlot {
	var free []timeSlot

	current := start
	for current.Before(end) && len(free) < 10 {
		slotEnd := current.Add(duration)

		// Check working hours
		if workingHoursOnly {
			hour := current.Hour()
			if hour < 9 || hour >= 18 {
				current = current.Add(time.Hour)
				continue
			}
		}

		// Check if slot is free
		isFree := true
		for _, b := range busy {
			if !(slotEnd.Before(b.start) || current.After(b.end)) {
				isFree = false
				break
			}
		}

		if isFree {
			free = append(free, timeSlot{start: current, end: slotEnd})
		}

		current = current.Add(30 * time.Minute)
	}

	return free
}

// Helper functions
func getStringArg(args map[string]any, key, defaultVal string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return defaultVal
}

func getIntArg(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	if v, ok := args[key].(int); ok {
		return v
	}
	return defaultVal
}

func getBoolArg(args map[string]any, key string, defaultVal bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return defaultVal
}

func getStringArrayArg(args map[string]any, key string) []string {
	if v, ok := args[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func parseRelativeDate(s string, now time.Time) time.Time {
	switch s {
	case "today", "오늘":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "tomorrow", "내일":
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	case "this week", "이번 주":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	case "next week", "다음 주":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-weekday+8, 0, 0, 0, 0, now.Location())
	default:
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t
		}
		return now
	}
}

func parseFlexibleTime(s string, now time.Time) (time.Time, error) {
	// Try ISO 8601
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try date + time
	if t, err := time.Parse("2006-01-02 15:04", s); err == nil {
		return t, nil
	}
	// Try just time (use today's date)
	if t, err := time.Parse("15:04", s); err == nil {
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

func parseDuration(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, true
	}
	return 0, false
}

// =============================================================================
// Calendar Action Tools (Destructive - Require Proposal)
// =============================================================================

// CalendarDeleteTool deletes a calendar event
type CalendarDeleteTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarDeleteTool(calendarRepo domain.CalendarRepository) *CalendarDeleteTool {
	return &CalendarDeleteTool{calendarRepo: calendarRepo}
}

func (t *CalendarDeleteTool) Name() string           { return "calendar.delete" }
func (t *CalendarDeleteTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarDeleteTool) Description() string {
	return "Delete a calendar event. Requires confirmation before deletion."
}

func (t *CalendarDeleteTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "event_id", Type: "number", Description: "Event ID to delete", Required: true},
		{Name: "notify_attendees", Type: "boolean", Description: "Send cancellation to attendees", Default: true},
	}
}

func (t *CalendarDeleteTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	eventID := int64(getIntArg(args, "event_id", 0))
	notifyAttendees := getBoolArg(args, "notify_attendees", true)

	if eventID == 0 {
		return &ToolResult{Success: false, Error: "event_id is required"}, nil
	}

	// Get event to verify ownership
	event, err := t.calendarRepo.GetEventByID(eventID)
	if err != nil {
		return &ToolResult{Success: false, Error: "event not found"}, nil
	}

	// Verify user has access to this calendar
	calendar, err := t.calendarRepo.GetCalendarByID(event.CalendarID)
	if err != nil || calendar.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "calendar.delete",
		Description: fmt.Sprintf("Delete event '%s' on %s", event.Title, event.StartTime.Format("2006-01-02 15:04")),
		Data: map[string]any{
			"event_id":         eventID,
			"calendar_id":      event.CalendarID,
			"title":            event.Title,
			"provider":         calendar.Provider,
			"provider_id":      event.ProviderID,
			"notify_attendees": notifyAttendees,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to delete event '%s'", event.Title),
		Proposal: proposal,
	}, nil
}

// CalendarUpdateTool updates a calendar event
type CalendarUpdateTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarUpdateTool(calendarRepo domain.CalendarRepository) *CalendarUpdateTool {
	return &CalendarUpdateTool{calendarRepo: calendarRepo}
}

func (t *CalendarUpdateTool) Name() string           { return "calendar.update" }
func (t *CalendarUpdateTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarUpdateTool) Description() string {
	return "Update a calendar event. Can change title, time, location, or description."
}

func (t *CalendarUpdateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "event_id", Type: "number", Description: "Event ID to update", Required: true},
		{Name: "title", Type: "string", Description: "New event title"},
		{Name: "start_time", Type: "string", Description: "New start time (ISO 8601 or 'tomorrow 3pm')"},
		{Name: "end_time", Type: "string", Description: "New end time (ISO 8601 or duration like '1h')"},
		{Name: "description", Type: "string", Description: "New event description"},
		{Name: "location", Type: "string", Description: "New event location"},
		{Name: "notify_attendees", Type: "boolean", Description: "Notify attendees of changes", Default: true},
	}
}

func (t *CalendarUpdateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	eventID := int64(getIntArg(args, "event_id", 0))

	if eventID == 0 {
		return &ToolResult{Success: false, Error: "event_id is required"}, nil
	}

	// Get event to verify ownership
	event, err := t.calendarRepo.GetEventByID(eventID)
	if err != nil {
		return &ToolResult{Success: false, Error: "event not found"}, nil
	}

	// Verify user has access to this calendar
	calendar, err := t.calendarRepo.GetCalendarByID(event.CalendarID)
	if err != nil || calendar.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Build update data
	updateData := map[string]any{
		"event_id":    eventID,
		"calendar_id": event.CalendarID,
		"provider":    calendar.Provider,
		"provider_id": event.ProviderID,
	}

	changes := []string{}
	now := time.Now()

	if title := getStringArg(args, "title", ""); title != "" && title != event.Title {
		updateData["title"] = title
		changes = append(changes, fmt.Sprintf("title to '%s'", title))
	}

	if startTimeStr := getStringArg(args, "start_time", ""); startTimeStr != "" {
		startTime, err := parseFlexibleTime(startTimeStr, now)
		if err != nil {
			return &ToolResult{Success: false, Error: "invalid start_time format"}, nil
		}
		updateData["start_time"] = startTime
		changes = append(changes, fmt.Sprintf("start time to %s", startTime.Format("2006-01-02 15:04")))
	}

	if endTimeStr := getStringArg(args, "end_time", ""); endTimeStr != "" {
		endTime, err := parseFlexibleTime(endTimeStr, now)
		if err != nil {
			if duration, ok := parseDuration(endTimeStr); ok {
				if st, ok := updateData["start_time"].(time.Time); ok {
					endTime = st.Add(duration)
				} else {
					endTime = event.StartTime.Add(duration)
				}
			} else {
				return &ToolResult{Success: false, Error: "invalid end_time format"}, nil
			}
		}
		updateData["end_time"] = endTime
		changes = append(changes, fmt.Sprintf("end time to %s", endTime.Format("2006-01-02 15:04")))
	}

	if description := getStringArg(args, "description", ""); description != "" {
		updateData["description"] = description
		changes = append(changes, "description")
	}

	if location := getStringArg(args, "location", ""); location != "" {
		updateData["location"] = location
		changes = append(changes, fmt.Sprintf("location to '%s'", location))
	}

	notifyAttendees := getBoolArg(args, "notify_attendees", true)
	updateData["notify_attendees"] = notifyAttendees

	if len(changes) == 0 {
		return &ToolResult{Success: false, Error: "no changes specified"}, nil
	}

	description := fmt.Sprintf("Update event '%s': change %s", event.Title, joinChanges(changes))

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "calendar.update",
		Description: description,
		Data:        updateData,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to update event '%s'", event.Title),
		Proposal: proposal,
	}, nil
}

// CalendarGetTool retrieves a specific calendar event
type CalendarGetTool struct {
	calendarRepo domain.CalendarRepository
}

func NewCalendarGetTool(calendarRepo domain.CalendarRepository) *CalendarGetTool {
	return &CalendarGetTool{calendarRepo: calendarRepo}
}

func (t *CalendarGetTool) Name() string           { return "calendar.get" }
func (t *CalendarGetTool) Category() ToolCategory { return CategoryCalendar }

func (t *CalendarGetTool) Description() string {
	return "Get detailed information about a specific calendar event."
}

func (t *CalendarGetTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "event_id", Type: "number", Description: "Event ID to retrieve", Required: true},
	}
}

func (t *CalendarGetTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	eventID := int64(getIntArg(args, "event_id", 0))

	if eventID == 0 {
		return &ToolResult{Success: false, Error: "event_id is required"}, nil
	}

	// Get event
	event, err := t.calendarRepo.GetEventByID(eventID)
	if err != nil {
		return &ToolResult{Success: false, Error: "event not found"}, nil
	}

	// Verify user has access to this calendar
	calendar, err := t.calendarRepo.GetCalendarByID(event.CalendarID)
	if err != nil || calendar.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	result := map[string]any{
		"id":          event.ID,
		"title":       event.Title,
		"start_time":  event.StartTime.Format(time.RFC3339),
		"end_time":    event.EndTime.Format(time.RFC3339),
		"description": event.Description,
		"location":    event.Location,
		"attendees":   event.Attendees,
		"is_all_day":  event.IsAllDay,
		"status":      event.Status,
		"provider":    calendar.Provider,
		"calendar":    calendar.Name,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
		Message: fmt.Sprintf("Event: %s", event.Title),
	}, nil
}

// Helper function for joining changes
func joinChanges(changes []string) string {
	if len(changes) == 0 {
		return ""
	}
	if len(changes) == 1 {
		return changes[0]
	}
	if len(changes) == 2 {
		return changes[0] + " and " + changes[1]
	}
	result := ""
	for i, c := range changes {
		if i == len(changes)-1 {
			result += "and " + c
		} else {
			result += c + ", "
		}
	}
	return result
}
