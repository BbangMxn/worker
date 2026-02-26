package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/port/out"

	"golang.org/x/oauth2"
)

const (
	msGraphBaseURL    = "https://graph.microsoft.com/v1.0"
	msGraphBetaURL    = "https://graph.microsoft.com/beta"
	outlookTimeFormat = "2006-01-02T15:04:05"
)

// OutlookCalendarAdapter implements CalendarProviderPort for Microsoft Outlook/365.
type OutlookCalendarAdapter struct {
	oauthConfig     *oauth2.Config
	notificationURL string // Webhook URL for change notifications
}

// NewOutlookCalendarAdapter creates a new Outlook Calendar adapter.
func NewOutlookCalendarAdapter(oauthConfig *oauth2.Config, notificationURL string) *OutlookCalendarAdapter {
	return &OutlookCalendarAdapter{
		oauthConfig:     oauthConfig,
		notificationURL: notificationURL,
	}
}

// getClient creates an HTTP client with token.
func (a *OutlookCalendarAdapter) getClient(ctx context.Context, token *oauth2.Token) *http.Client {
	return a.oauthConfig.Client(ctx, token)
}

// =============================================================================
// Calendar Operations
// =============================================================================

// ListCalendars lists all calendars.
func (a *OutlookCalendarAdapter) ListCalendars(ctx context.Context, token *oauth2.Token) ([]*out.ProviderCalendar, error) {
	client := a.getClient(ctx, token)

	req, err := http.NewRequestWithContext(ctx, "GET", msGraphBaseURL+"/me/calendars", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Prefer", "outlook.timezone=\"UTC\"")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list calendars: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list calendars failed with status %d", resp.StatusCode)
	}

	var result struct {
		Value []struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Color             string `json:"color"`
			IsDefaultCalendar bool   `json:"isDefaultCalendar"`
			CanEdit           bool   `json:"canEdit"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	calendars := make([]*out.ProviderCalendar, len(result.Value))
	for i, cal := range result.Value {
		calendars[i] = &out.ProviderCalendar{
			ID:        cal.ID,
			Name:      cal.Name,
			Color:     cal.Color,
			IsPrimary: cal.IsDefaultCalendar,
		}
	}

	return calendars, nil
}

// GetCalendar gets a single calendar.
func (a *OutlookCalendarAdapter) GetCalendar(ctx context.Context, token *oauth2.Token, calendarID string) (*out.ProviderCalendar, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/calendars/" + calendarID
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get calendar failed with status %d", resp.StatusCode)
	}

	var cal struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Color             string `json:"color"`
		IsDefaultCalendar bool   `json:"isDefaultCalendar"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&cal); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &out.ProviderCalendar{
		ID:        cal.ID,
		Name:      cal.Name,
		Color:     cal.Color,
		IsPrimary: cal.IsDefaultCalendar,
	}, nil
}

// =============================================================================
// Event Operations
// =============================================================================

// ListEvents lists events from a calendar.
func (a *OutlookCalendarAdapter) ListEvents(ctx context.Context, token *oauth2.Token, query *out.ProviderCalendarQuery) (*out.ProviderCalendarListResult, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/calendar/events"
	if query.CalendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + query.CalendarID + "/events"
	}

	// Build query parameters
	params := url.Values{}
	params.Set("$orderby", "start/dateTime")

	if query.PageSize > 0 {
		params.Set("$top", fmt.Sprintf("%d", query.PageSize))
	}

	if query.TimeMin != nil {
		params.Set("$filter", fmt.Sprintf("start/dateTime ge '%s'", query.TimeMin.Format(outlookTimeFormat)))
	}

	if query.TimeMax != nil {
		filter := params.Get("$filter")
		if filter != "" {
			filter += " and "
		}
		filter += fmt.Sprintf("end/dateTime le '%s'", query.TimeMax.Format(outlookTimeFormat))
		params.Set("$filter", filter)
	}

	if query.PageToken != "" {
		endpoint = query.PageToken // Microsoft uses full URL as skip token
	} else {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Prefer", "outlook.timezone=\"UTC\"")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list events failed with status %d", resp.StatusCode)
	}

	var result struct {
		Value    []outlookEvent `json:"value"`
		NextLink string         `json:"@odata.nextLink"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	events := make([]*out.ProviderCalendarEvent, len(result.Value))
	for i, ev := range result.Value {
		events[i] = a.convertEvent(&ev, query.CalendarID)
	}

	return &out.ProviderCalendarListResult{
		Events:        events,
		NextPageToken: result.NextLink,
	}, nil
}

// GetEvent gets a single event.
func (a *OutlookCalendarAdapter) GetEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) (*out.ProviderCalendarEvent, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/events/" + eventID
	if calendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + calendarID + "/events/" + eventID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Prefer", "outlook.timezone=\"UTC\"")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get event failed with status %d", resp.StatusCode)
	}

	var ev outlookEvent
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertEvent(&ev, calendarID), nil
}

// CreateEvent creates a new event.
func (a *OutlookCalendarAdapter) CreateEvent(ctx context.Context, token *oauth2.Token, calendarID string, event *out.ProviderCalendarEvent) (*out.ProviderCalendarEvent, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/calendar/events"
	if calendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + calendarID + "/events"
	}

	body := a.toOutlookEvent(event)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create event failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ev outlookEvent
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertEvent(&ev, calendarID), nil
}

// UpdateEvent updates an existing event.
func (a *OutlookCalendarAdapter) UpdateEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string, event *out.ProviderCalendarEvent) (*out.ProviderCalendarEvent, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/events/" + eventID
	if calendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + calendarID + "/events/" + eventID
	}

	body := a.toOutlookEvent(event)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update event failed with status %d", resp.StatusCode)
	}

	var ev outlookEvent
	if err := json.NewDecoder(resp.Body).Decode(&ev); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertEvent(&ev, calendarID), nil
}

// DeleteEvent deletes an event.
func (a *OutlookCalendarAdapter) DeleteEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) error {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/events/" + eventID
	if calendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + calendarID + "/events/" + eventID
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete event failed with status %d", resp.StatusCode)
	}

	return nil
}

// =============================================================================
// Sync Operations
// =============================================================================

// InitialSync performs initial calendar sync.
func (a *OutlookCalendarAdapter) InitialSync(ctx context.Context, token *oauth2.Token, opts *out.CalendarSyncOptions) (*out.CalendarSyncResult, error) {
	// Default: sync events from 30 days ago to 90 days ahead
	timeMin := time.Now().AddDate(0, 0, -30)
	timeMax := time.Now().AddDate(0, 0, 90)

	if opts.TimeMin != nil {
		timeMin = *opts.TimeMin
	}
	if opts.TimeMax != nil {
		timeMax = *opts.TimeMax
	}

	query := &out.ProviderCalendarQuery{
		CalendarID: opts.CalendarID,
		TimeMin:    &timeMin,
		TimeMax:    &timeMax,
		PageSize:   opts.MaxResults,
	}

	result, err := a.ListEvents(ctx, token, query)
	if err != nil {
		return nil, err
	}

	// Get delta link for future sync
	deltaLink, err := a.getDeltaLink(ctx, token, opts.CalendarID)
	if err != nil {
		// Non-fatal, just log
		deltaLink = ""
	}

	return &out.CalendarSyncResult{
		Events:        result.Events,
		NextSyncToken: deltaLink,
		NextPageToken: result.NextPageToken,
	}, nil
}

// IncrementalSync performs incremental sync using delta token.
func (a *OutlookCalendarAdapter) IncrementalSync(ctx context.Context, token *oauth2.Token, calendarID, deltaToken string) (*out.CalendarSyncResult, error) {
	client := a.getClient(ctx, token)

	req, err := http.NewRequestWithContext(ctx, "GET", deltaToken, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Prefer", "outlook.timezone=\"UTC\"")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to sync events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		// Delta token expired
		return nil, &out.ProviderError{
			Code:    out.ProviderErrSyncRequired,
			Message: "delta token expired",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sync events failed with status %d", resp.StatusCode)
	}

	var result struct {
		Value     []outlookEvent `json:"value"`
		NextLink  string         `json:"@odata.nextLink"`
		DeltaLink string         `json:"@odata.deltaLink"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	events := make([]*out.ProviderCalendarEvent, 0)
	deletedIDs := make([]string, 0)

	for _, ev := range result.Value {
		if ev.Removed != nil {
			deletedIDs = append(deletedIDs, ev.ID)
		} else {
			events = append(events, a.convertEvent(&ev, calendarID))
		}
	}

	nextToken := result.DeltaLink
	if nextToken == "" {
		nextToken = result.NextLink
	}

	return &out.CalendarSyncResult{
		Events:        events,
		DeletedIDs:    deletedIDs,
		NextSyncToken: nextToken,
		NextPageToken: result.NextLink,
	}, nil
}

// getDeltaLink gets initial delta link for future sync.
func (a *OutlookCalendarAdapter) getDeltaLink(ctx context.Context, token *oauth2.Token, calendarID string) (string, error) {
	client := a.getClient(ctx, token)

	endpoint := msGraphBaseURL + "/me/calendarView/delta"
	if calendarID != "" {
		endpoint = msGraphBaseURL + "/me/calendars/" + calendarID + "/calendarView/delta"
	}

	// Get delta link by requesting with current time range
	now := time.Now()
	params := url.Values{}
	params.Set("startDateTime", now.AddDate(0, 0, -30).Format(time.RFC3339))
	params.Set("endDateTime", now.AddDate(0, 0, 90).Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Prefer", "odata.maxpagesize=1")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		DeltaLink string `json:"@odata.deltaLink"`
		NextLink  string `json:"@odata.nextLink"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.DeltaLink != "" {
		return result.DeltaLink, nil
	}
	return result.NextLink, nil
}

// =============================================================================
// Watch (Change Notifications / Subscriptions)
// =============================================================================

// Watch sets up change notifications for calendar changes.
func (a *OutlookCalendarAdapter) Watch(ctx context.Context, token *oauth2.Token, calendarID string) (*out.CalendarWatchResponse, error) {
	client := a.getClient(ctx, token)

	resource := "/me/events"
	if calendarID != "" {
		resource = "/me/calendars/" + calendarID + "/events"
	}

	subscription := map[string]interface{}{
		"changeType":         "created,updated,deleted",
		"notificationUrl":    a.notificationURL,
		"resource":           resource,
		"expirationDateTime": time.Now().Add(3 * 24 * time.Hour).Format(time.RFC3339), // 3 days max
		"clientState":        "calendar-watch",
	}

	jsonBody, err := json.Marshal(subscription)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal subscription: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", msGraphBaseURL+"/subscriptions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create subscription failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		ID                 string `json:"id"`
		ExpirationDateTime string `json:"expirationDateTime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	expiration, _ := time.Parse(time.RFC3339, result.ExpirationDateTime)

	return &out.CalendarWatchResponse{
		ChannelID:  result.ID,
		ResourceID: calendarID,
		Expiration: expiration,
	}, nil
}

// StopWatch stops change notifications.
func (a *OutlookCalendarAdapter) StopWatch(ctx context.Context, token *oauth2.Token, subscriptionID, _ string) error {
	client := a.getClient(ctx, token)

	req, err := http.NewRequestWithContext(ctx, "DELETE", msGraphBaseURL+"/subscriptions/"+subscriptionID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete subscription failed with status %d", resp.StatusCode)
	}

	return nil
}

// RenewWatch renews a subscription.
func (a *OutlookCalendarAdapter) RenewWatch(ctx context.Context, token *oauth2.Token, subscriptionID string) (*out.CalendarWatchResponse, error) {
	client := a.getClient(ctx, token)

	body := map[string]string{
		"expirationDateTime": time.Now().Add(3 * 24 * time.Hour).Format(time.RFC3339),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", msGraphBaseURL+"/subscriptions/"+subscriptionID, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to renew subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("renew subscription failed with status %d", resp.StatusCode)
	}

	var result struct {
		ID                 string `json:"id"`
		ExpirationDateTime string `json:"expirationDateTime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	expiration, _ := time.Parse(time.RFC3339, result.ExpirationDateTime)

	return &out.CalendarWatchResponse{
		ChannelID:  result.ID,
		Expiration: expiration,
	}, nil
}

// =============================================================================
// Free/Busy Query
// =============================================================================

// GetFreeBusy queries free/busy information.
func (a *OutlookCalendarAdapter) GetFreeBusy(ctx context.Context, token *oauth2.Token, req *out.FreeBusyRequest) (*out.FreeBusyResponse, error) {
	client := a.getClient(ctx, token)

	schedules := make([]string, len(req.CalendarIDs))
	for i, id := range req.CalendarIDs {
		schedules[i] = id
	}

	body := map[string]interface{}{
		"schedules":                schedules,
		"startTime":                map[string]string{"dateTime": req.TimeMin.Format(outlookTimeFormat), "timeZone": "UTC"},
		"endTime":                  map[string]string{"dateTime": req.TimeMax.Format(outlookTimeFormat), "timeZone": "UTC"},
		"availabilityViewInterval": 30, // 30 minute slots
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", msGraphBaseURL+"/me/calendar/getSchedule", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get schedule failed with status %d", resp.StatusCode)
	}

	var result struct {
		Value []struct {
			ScheduleID    string `json:"scheduleId"`
			ScheduleItems []struct {
				Status string `json:"status"`
				Start  struct {
					DateTime string `json:"dateTime"`
				} `json:"start"`
				End struct {
					DateTime string `json:"dateTime"`
				} `json:"end"`
			} `json:"scheduleItems"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	freeBusy := &out.FreeBusyResponse{
		Calendars: make(map[string][]*out.TimePeriod),
	}

	for _, schedule := range result.Value {
		periods := make([]*out.TimePeriod, 0)
		for _, item := range schedule.ScheduleItems {
			if item.Status == "busy" || item.Status == "tentative" {
				start, _ := time.Parse(outlookTimeFormat, item.Start.DateTime)
				end, _ := time.Parse(outlookTimeFormat, item.End.DateTime)
				periods = append(periods, &out.TimePeriod{
					Start: start,
					End:   end,
				})
			}
		}
		freeBusy.Calendars[schedule.ScheduleID] = periods
	}

	return freeBusy, nil
}

// =============================================================================
// Helper Types
// =============================================================================

type outlookEvent struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	BodyPreview string `json:"bodyPreview"`
	Body        struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	Start struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	} `json:"end"`
	Location struct {
		DisplayName string `json:"displayName"`
	} `json:"location"`
	IsAllDay  bool `json:"isAllDay"`
	Organizer struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"organizer"`
	Attendees []struct {
		Type   string `json:"type"`
		Status struct {
			Response string `json:"response"`
		} `json:"status"`
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"attendees"`
	Recurrence *struct {
		Pattern struct {
			Type string `json:"type"`
		} `json:"pattern"`
	} `json:"recurrence"`
	OnlineMeeting *struct {
		JoinUrl string `json:"joinUrl"`
	} `json:"onlineMeeting"`
	OnlineMeetingProvider string `json:"onlineMeetingProvider"`
	ShowAs                string `json:"showAs"`
	Importance            string `json:"importance"`
	Sensitivity           string `json:"sensitivity"`
	Removed               *struct {
		Reason string `json:"reason"`
	} `json:"@removed"`
}

func (a *OutlookCalendarAdapter) convertEvent(ev *outlookEvent, calendarID string) *out.ProviderCalendarEvent {
	event := &out.ProviderCalendarEvent{
		ID:          ev.ID,
		CalendarID:  calendarID,
		Title:       ev.Subject,
		Description: ev.Body.Content,
		Location:    ev.Location.DisplayName,
		IsAllDay:    ev.IsAllDay,
		Status:      ev.ShowAs,
		Visibility:  ev.Sensitivity,
	}

	// Parse times
	if ev.Start.DateTime != "" {
		t, _ := time.Parse(outlookTimeFormat, ev.Start.DateTime)
		event.StartTime = t
		event.Timezone = ev.Start.TimeZone
	}
	if ev.End.DateTime != "" {
		t, _ := time.Parse(outlookTimeFormat, ev.End.DateTime)
		event.EndTime = t
	}

	// Organizer
	event.OrganizerEmail = ev.Organizer.EmailAddress.Address

	// Attendees
	if len(ev.Attendees) > 0 {
		event.Attendees = make([]*out.ProviderAttendee, len(ev.Attendees))
		for i, att := range ev.Attendees {
			event.Attendees[i] = &out.ProviderAttendee{
				Email:    att.EmailAddress.Address,
				Name:     att.EmailAddress.Name,
				Status:   att.Status.Response,
				Optional: att.Type == "optional",
			}
		}
	}

	// Recurrence
	if ev.Recurrence != nil {
		event.IsRecurring = true
		event.RecurrenceRule = ev.Recurrence.Pattern.Type
	}

	// Online meeting
	if ev.OnlineMeeting != nil {
		event.MeetingURL = ev.OnlineMeeting.JoinUrl
		event.MeetingProvider = ev.OnlineMeetingProvider
	}

	return event
}

func (a *OutlookCalendarAdapter) toOutlookEvent(event *out.ProviderCalendarEvent) map[string]interface{} {
	tz := event.Timezone
	if tz == "" {
		tz = "UTC"
	}

	result := map[string]interface{}{
		"subject": event.Title,
		"body": map[string]string{
			"contentType": "HTML",
			"content":     event.Description,
		},
		"start": map[string]string{
			"dateTime": event.StartTime.Format(outlookTimeFormat),
			"timeZone": tz,
		},
		"end": map[string]string{
			"dateTime": event.EndTime.Format(outlookTimeFormat),
			"timeZone": tz,
		},
		"isAllDay": event.IsAllDay,
	}

	if event.Location != "" {
		result["location"] = map[string]string{
			"displayName": event.Location,
		}
	}

	// Attendees
	if len(event.Attendees) > 0 {
		attendees := make([]map[string]interface{}, len(event.Attendees))
		for i, att := range event.Attendees {
			attType := "required"
			if att.Optional {
				attType = "optional"
			}
			attendees[i] = map[string]interface{}{
				"type": attType,
				"emailAddress": map[string]string{
					"address": att.Email,
					"name":    att.Name,
				},
			}
		}
		result["attendees"] = attendees
	}

	return result
}

// Ensure interface compliance
var _ out.CalendarProviderPort = (*OutlookCalendarAdapter)(nil)
