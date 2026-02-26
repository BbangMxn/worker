package provider

import (
	"context"
	"fmt"
	"log"
	"time"

	"worker_server/core/port/out"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// GoogleCalendarAdapter implements CalendarProviderPort for Google Calendar.
type GoogleCalendarAdapter struct {
	oauthConfig *oauth2.Config
	pubsubTopic string // Google Pub/Sub topic for push notifications
}

// NewGoogleCalendarAdapter creates a new Google Calendar adapter.
func NewGoogleCalendarAdapter(oauthConfig *oauth2.Config, pubsubTopic string) *GoogleCalendarAdapter {
	return &GoogleCalendarAdapter{
		oauthConfig: oauthConfig,
		pubsubTopic: pubsubTopic,
	}
}

// getService creates a Calendar service with token.
func (a *GoogleCalendarAdapter) getService(ctx context.Context, token *oauth2.Token) (*calendar.Service, error) {
	client := a.oauthConfig.Client(ctx, token)
	return calendar.NewService(ctx, option.WithHTTPClient(client))
}

// =============================================================================
// Calendar Operations
// =============================================================================

// ListCalendars lists all calendars.
func (a *GoogleCalendarAdapter) ListCalendars(ctx context.Context, token *oauth2.Token) ([]*out.ProviderCalendar, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	list, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list calendars: %w", err)
	}

	calendars := make([]*out.ProviderCalendar, 0, len(list.Items))
	for _, cal := range list.Items {
		calendars = append(calendars, &out.ProviderCalendar{
			ID:          cal.Id,
			Name:        cal.Summary,
			Description: cal.Description,
			Color:       cal.BackgroundColor,
			IsPrimary:   cal.Primary,
			IsSelected:  cal.Selected,
		})
	}

	return calendars, nil
}

// GetCalendar gets a single calendar.
func (a *GoogleCalendarAdapter) GetCalendar(ctx context.Context, token *oauth2.Token, calendarID string) (*out.ProviderCalendar, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	cal, err := svc.CalendarList.Get(calendarID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get calendar: %w", err)
	}

	return &out.ProviderCalendar{
		ID:          cal.Id,
		Name:        cal.Summary,
		Description: cal.Description,
		Color:       cal.BackgroundColor,
		IsPrimary:   cal.Primary,
		IsSelected:  cal.Selected,
	}, nil
}

// =============================================================================
// Event Operations
// =============================================================================

// ListEvents lists events from a calendar.
func (a *GoogleCalendarAdapter) ListEvents(ctx context.Context, token *oauth2.Token, query *out.ProviderCalendarQuery) (*out.ProviderCalendarListResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	calendarID := query.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	req := svc.Events.List(calendarID).
		SingleEvents(query.SingleEvents).
		Context(ctx)

	if query.TimeMin != nil {
		req = req.TimeMin(query.TimeMin.Format(time.RFC3339))
	}
	if query.TimeMax != nil {
		req = req.TimeMax(query.TimeMax.Format(time.RFC3339))
	}
	if query.PageSize > 0 {
		req = req.MaxResults(int64(query.PageSize))
	}
	if query.PageToken != "" {
		req = req.PageToken(query.PageToken)
	}
	if query.OrderBy != "" {
		req = req.OrderBy(query.OrderBy)
	}

	resp, err := req.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	events := make([]*out.ProviderCalendarEvent, 0, len(resp.Items))
	for _, item := range resp.Items {
		events = append(events, a.convertEvent(item, calendarID))
	}

	return &out.ProviderCalendarListResult{
		Events:        events,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// GetEvent gets a single event.
func (a *GoogleCalendarAdapter) GetEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) (*out.ProviderCalendarEvent, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	event, err := svc.Events.Get(calendarID, eventID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	return a.convertEvent(event, calendarID), nil
}

// CreateEvent creates a new event.
func (a *GoogleCalendarAdapter) CreateEvent(ctx context.Context, token *oauth2.Token, calendarID string, event *out.ProviderCalendarEvent) (*out.ProviderCalendarEvent, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	gcalEvent := a.toGoogleEvent(event)

	// Apply sendUpdates parameter for attendee notifications
	// Valid values: "all", "externalOnly", "none"
	sendUpdates := "none"
	if event.SendNotifications != "" {
		sendUpdates = event.SendNotifications
	}

	created, err := svc.Events.Insert(calendarID, gcalEvent).
		SendUpdates(sendUpdates).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create event: %w", err)
	}

	return a.convertEvent(created, calendarID), nil
}

// UpdateEvent updates an existing event.
func (a *GoogleCalendarAdapter) UpdateEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string, event *out.ProviderCalendarEvent) (*out.ProviderCalendarEvent, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	gcalEvent := a.toGoogleEvent(event)

	// Apply sendUpdates parameter for attendee notifications
	sendUpdates := "none"
	if event.SendNotifications != "" {
		sendUpdates = event.SendNotifications
	}

	updated, err := svc.Events.Update(calendarID, eventID, gcalEvent).
		SendUpdates(sendUpdates).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to update event: %w", err)
	}

	return a.convertEvent(updated, calendarID), nil
}

// DeleteEvent deletes an event.
func (a *GoogleCalendarAdapter) DeleteEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	if err := svc.Events.Delete(calendarID, eventID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	return nil
}

// =============================================================================
// Sync Operations
// =============================================================================

// InitialSync performs initial calendar sync.
func (a *GoogleCalendarAdapter) InitialSync(ctx context.Context, token *oauth2.Token, opts *out.CalendarSyncOptions) (*out.CalendarSyncResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	calendarID := opts.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	// Default: sync events from 30 days ago to 90 days ahead
	timeMin := time.Now().AddDate(0, 0, -30)
	timeMax := time.Now().AddDate(0, 0, 90)

	if opts.TimeMin != nil {
		timeMin = *opts.TimeMin
	}
	if opts.TimeMax != nil {
		timeMax = *opts.TimeMax
	}

	req := svc.Events.List(calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(int64(opts.MaxResults)).
		Context(ctx)

	resp, err := req.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	events := make([]*out.ProviderCalendarEvent, 0, len(resp.Items))
	for _, item := range resp.Items {
		events = append(events, a.convertEvent(item, calendarID))
	}

	return &out.CalendarSyncResult{
		Events:        events,
		NextSyncToken: resp.NextSyncToken,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// IncrementalSync performs incremental sync using sync token.
func (a *GoogleCalendarAdapter) IncrementalSync(ctx context.Context, token *oauth2.Token, calendarID, syncToken string) (*out.CalendarSyncResult, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	resp, err := svc.Events.List(calendarID).
		SyncToken(syncToken).
		Context(ctx).
		Do()

	if err != nil {
		// Sync token expired - need full sync
		log.Printf("[GoogleCalendar.IncrementalSync] Sync token expired, full sync required")
		return nil, &out.ProviderError{
			Code:    out.ProviderErrSyncRequired,
			Message: "sync token expired",
		}
	}

	events := make([]*out.ProviderCalendarEvent, 0, len(resp.Items))
	deletedIDs := make([]string, 0)

	for _, item := range resp.Items {
		if item.Status == "cancelled" {
			deletedIDs = append(deletedIDs, item.Id)
		} else {
			events = append(events, a.convertEvent(item, calendarID))
		}
	}

	return &out.CalendarSyncResult{
		Events:        events,
		DeletedIDs:    deletedIDs,
		NextSyncToken: resp.NextSyncToken,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// =============================================================================
// Watch (Push Notifications)
// =============================================================================

// Watch sets up push notifications for calendar changes.
func (a *GoogleCalendarAdapter) Watch(ctx context.Context, token *oauth2.Token, calendarID string) (*out.CalendarWatchResponse, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	if calendarID == "" {
		calendarID = "primary"
	}

	// Create watch channel
	channelID := fmt.Sprintf("calendar-%s-%d", calendarID, time.Now().UnixNano())

	channel := &calendar.Channel{
		Id:         channelID,
		Type:       "web_hook",
		Address:    a.pubsubTopic,                                  // Webhook URL
		Expiration: time.Now().Add(7 * 24 * time.Hour).UnixMilli(), // 7 days
	}

	resp, err := svc.Events.Watch(calendarID, channel).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to setup watch: %w", err)
	}

	return &out.CalendarWatchResponse{
		ChannelID:  resp.Id,
		ResourceID: resp.ResourceId,
		Expiration: time.UnixMilli(resp.Expiration),
	}, nil
}

// StopWatch stops push notifications.
func (a *GoogleCalendarAdapter) StopWatch(ctx context.Context, token *oauth2.Token, channelID, resourceID string) error {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to create calendar service: %w", err)
	}

	channel := &calendar.Channel{
		Id:         channelID,
		ResourceId: resourceID,
	}

	if err := svc.Channels.Stop(channel).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to stop watch: %w", err)
	}

	return nil
}

// =============================================================================
// Free/Busy Query
// =============================================================================

// GetFreeBusy queries free/busy information.
func (a *GoogleCalendarAdapter) GetFreeBusy(ctx context.Context, token *oauth2.Token, req *out.FreeBusyRequest) (*out.FreeBusyResponse, error) {
	svc, err := a.getService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	items := make([]*calendar.FreeBusyRequestItem, len(req.CalendarIDs))
	for i, id := range req.CalendarIDs {
		items[i] = &calendar.FreeBusyRequestItem{Id: id}
	}

	freeBusyReq := &calendar.FreeBusyRequest{
		TimeMin: req.TimeMin.Format(time.RFC3339),
		TimeMax: req.TimeMax.Format(time.RFC3339),
		Items:   items,
	}

	resp, err := svc.Freebusy.Query(freeBusyReq).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to query free/busy: %w", err)
	}

	result := &out.FreeBusyResponse{
		Calendars: make(map[string][]*out.TimePeriod),
	}

	for calID, calData := range resp.Calendars {
		periods := make([]*out.TimePeriod, 0, len(calData.Busy))
		for _, busy := range calData.Busy {
			start, _ := time.Parse(time.RFC3339, busy.Start)
			end, _ := time.Parse(time.RFC3339, busy.End)
			periods = append(periods, &out.TimePeriod{
				Start: start,
				End:   end,
			})
		}
		result.Calendars[calID] = periods
	}

	return result, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func (a *GoogleCalendarAdapter) convertEvent(event *calendar.Event, calendarID string) *out.ProviderCalendarEvent {
	result := &out.ProviderCalendarEvent{
		ID:          event.Id,
		CalendarID:  calendarID,
		Title:       event.Summary,
		Description: event.Description,
		Location:    event.Location,
		Status:      event.Status,
		Visibility:  event.Visibility,
		Color:       event.ColorId,
	}

	// Parse times
	if event.Start != nil {
		if event.Start.DateTime != "" {
			t, _ := time.Parse(time.RFC3339, event.Start.DateTime)
			result.StartTime = t
			result.Timezone = event.Start.TimeZone
		} else if event.Start.Date != "" {
			t, _ := time.Parse("2006-01-02", event.Start.Date)
			result.StartTime = t
			result.IsAllDay = true
		}
	}

	if event.End != nil {
		if event.End.DateTime != "" {
			t, _ := time.Parse(time.RFC3339, event.End.DateTime)
			result.EndTime = t
		} else if event.End.Date != "" {
			t, _ := time.Parse("2006-01-02", event.End.Date)
			result.EndTime = t
		}
	}

	// Organizer
	if event.Organizer != nil {
		result.OrganizerEmail = event.Organizer.Email
	}

	// Attendees
	if len(event.Attendees) > 0 {
		result.Attendees = make([]*out.ProviderAttendee, len(event.Attendees))
		for i, att := range event.Attendees {
			result.Attendees[i] = &out.ProviderAttendee{
				Email:    att.Email,
				Name:     att.DisplayName,
				Status:   att.ResponseStatus,
				Optional: att.Optional,
			}
		}
	}

	// Recurrence
	if len(event.Recurrence) > 0 {
		result.IsRecurring = true
		result.RecurrenceRule = event.Recurrence[0]
	}

	// Conference/Meeting
	if event.ConferenceData != nil {
		for _, ep := range event.ConferenceData.EntryPoints {
			if ep.EntryPointType == "video" {
				result.MeetingURL = ep.Uri
				break
			}
		}
		if event.ConferenceData.ConferenceSolution != nil {
			result.MeetingProvider = event.ConferenceData.ConferenceSolution.Name
		}
	}

	// Reminders
	if event.Reminders != nil && event.Reminders.Overrides != nil {
		result.Reminders = make([]*out.ProviderReminder, len(event.Reminders.Overrides))
		for i, r := range event.Reminders.Overrides {
			result.Reminders[i] = &out.ProviderReminder{
				Method:  r.Method,
				Minutes: int(r.Minutes),
			}
		}
	}

	return result
}

func (a *GoogleCalendarAdapter) toGoogleEvent(event *out.ProviderCalendarEvent) *calendar.Event {
	gcalEvent := &calendar.Event{
		Summary:     event.Title,
		Description: event.Description,
		Location:    event.Location,
		Status:      event.Status,
		Visibility:  event.Visibility,
	}

	// Times
	if event.IsAllDay {
		gcalEvent.Start = &calendar.EventDateTime{
			Date: event.StartTime.Format("2006-01-02"),
		}
		gcalEvent.End = &calendar.EventDateTime{
			Date: event.EndTime.Format("2006-01-02"),
		}
	} else {
		tz := event.Timezone
		if tz == "" {
			tz = "UTC"
		}
		gcalEvent.Start = &calendar.EventDateTime{
			DateTime: event.StartTime.Format(time.RFC3339),
			TimeZone: tz,
		}
		gcalEvent.End = &calendar.EventDateTime{
			DateTime: event.EndTime.Format(time.RFC3339),
			TimeZone: tz,
		}
	}

	// Attendees
	if len(event.Attendees) > 0 {
		gcalEvent.Attendees = make([]*calendar.EventAttendee, len(event.Attendees))
		for i, att := range event.Attendees {
			gcalEvent.Attendees[i] = &calendar.EventAttendee{
				Email:       att.Email,
				DisplayName: att.Name,
				Optional:    att.Optional,
			}
		}
	}

	// Reminders
	if len(event.Reminders) > 0 {
		overrides := make([]*calendar.EventReminder, len(event.Reminders))
		for i, r := range event.Reminders {
			overrides[i] = &calendar.EventReminder{
				Method:  r.Method,
				Minutes: int64(r.Minutes),
			}
		}
		gcalEvent.Reminders = &calendar.EventReminders{
			UseDefault: false,
			Overrides:  overrides,
		}
	}

	// Recurrence
	if event.RecurrenceRule != "" {
		gcalEvent.Recurrence = []string{event.RecurrenceRule}
	}

	return gcalEvent
}

// Ensure interface compliance
var _ out.CalendarProviderPort = (*GoogleCalendarAdapter)(nil)
