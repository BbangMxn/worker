package calendar

import (
	"context"
	"errors"
	"log"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"

	"github.com/google/uuid"
)

var (
	ErrRepoNotInitialized    = errors.New("repository not initialized")
	ErrProviderNotConfigured = errors.New("provider not configured")
)

type Service struct {
	calendarRepo            domain.CalendarRepository
	oauthService            *auth.OAuthService
	googleCalendarProvider  out.CalendarProviderPort
	outlookCalendarProvider out.CalendarProviderPort
}

func NewService(calendarRepo domain.CalendarRepository) *Service {
	return &Service{
		calendarRepo: calendarRepo,
	}
}

// NewServiceWithProviders creates calendar service with provider support.
func NewServiceWithProviders(
	calendarRepo domain.CalendarRepository,
	oauthService *auth.OAuthService,
	googleProvider out.CalendarProviderPort,
	outlookProvider out.CalendarProviderPort,
) *Service {
	return &Service{
		calendarRepo:            calendarRepo,
		oauthService:            oauthService,
		googleCalendarProvider:  googleProvider,
		outlookCalendarProvider: outlookProvider,
	}
}

// getProviderForConnection returns the appropriate provider for a connection.
func (s *Service) getProviderForConnection(ctx context.Context, connectionID int64) (out.CalendarProviderPort, error) {
	if s.oauthService == nil {
		return nil, ErrProviderNotConfigured
	}

	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	switch conn.Provider {
	case "google", "gmail":
		if s.googleCalendarProvider == nil {
			return nil, ErrProviderNotConfigured
		}
		return s.googleCalendarProvider, nil
	case "outlook", "microsoft":
		if s.outlookCalendarProvider == nil {
			return nil, ErrProviderNotConfigured
		}
		return s.outlookCalendarProvider, nil
	default:
		return nil, errors.New("unsupported provider: " + string(conn.Provider))
	}
}

func (s *Service) GetCalendar(ctx context.Context, calendarID int64) (*domain.Calendar, error) {
	if s.calendarRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	return s.calendarRepo.GetCalendarByID(calendarID)
}

func (s *Service) ListCalendars(ctx context.Context, userID uuid.UUID) ([]*domain.Calendar, error) {
	// DB에서 먼저 조회
	if s.calendarRepo != nil {
		calendars, err := s.calendarRepo.GetCalendarsByUser(userID)
		if err == nil && len(calendars) > 0 {
			return calendars, nil
		}
	}
	return []*domain.Calendar{}, nil
}

// ListCalendarsFromProvider fetches calendars directly from provider.
func (s *Service) ListCalendarsFromProvider(ctx context.Context, connectionID int64) ([]*domain.Calendar, error) {
	provider, err := s.getProviderForConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	providerCalendars, err := provider.ListCalendars(ctx, token)
	if err != nil {
		return nil, err
	}

	calendars := make([]*domain.Calendar, len(providerCalendars))
	for i, pc := range providerCalendars {
		var desc, color *string
		if pc.Description != "" {
			desc = &pc.Description
		}
		if pc.Color != "" {
			color = &pc.Color
		}

		calendars[i] = &domain.Calendar{
			UserID:       conn.UserID,
			ConnectionID: connectionID,
			Provider:     domain.CalendarProvider(conn.Provider),
			ProviderID:   pc.ID,
			Name:         pc.Name,
			Description:  desc,
			Color:        color,
			IsDefault:    pc.IsPrimary,
		}
	}

	return calendars, nil
}

func (s *Service) GetEvent(ctx context.Context, eventID int64) (*domain.CalendarEvent, error) {
	if s.calendarRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	return s.calendarRepo.GetEventByID(eventID)
}

func (s *Service) ListEvents(ctx context.Context, filter *domain.CalendarEventFilter) ([]*domain.CalendarEvent, int, error) {
	if s.calendarRepo == nil {
		return []*domain.CalendarEvent{}, 0, nil
	}
	return s.calendarRepo.ListEvents(filter)
}

// ListEventsFromProvider fetches events directly from provider.
func (s *Service) ListEventsFromProvider(ctx context.Context, connectionID int64, startTime, endTime *time.Time) ([]*domain.CalendarEvent, error) {
	provider, err := s.getProviderForConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// Default time range: 30 days back to 90 days forward
	if startTime == nil {
		t := time.Now().AddDate(0, 0, -30)
		startTime = &t
	}
	if endTime == nil {
		t := time.Now().AddDate(0, 0, 90)
		endTime = &t
	}

	result, err := provider.ListEvents(ctx, token, &out.ProviderCalendarQuery{
		CalendarID:   "primary",
		TimeMin:      startTime,
		TimeMax:      endTime,
		SingleEvents: true,
		OrderBy:      "startTime",
		PageSize:     100,
	})
	if err != nil {
		return nil, err
	}

	events := make([]*domain.CalendarEvent, len(result.Events))
	for i, pe := range result.Events {
		events[i] = s.convertProviderEvent(pe, conn.UserID, connectionID)
	}

	return events, nil
}

// convertProviderEvent converts provider event to domain event.
func (s *Service) convertProviderEvent(pe *out.ProviderCalendarEvent, userID uuid.UUID, connectionID int64) *domain.CalendarEvent {
	var desc, loc, org, meetURL, recRule *string
	if pe.Description != "" {
		desc = &pe.Description
	}
	if pe.Location != "" {
		loc = &pe.Location
	}
	if pe.OrganizerEmail != "" {
		org = &pe.OrganizerEmail
	}
	if pe.MeetingURL != "" {
		meetURL = &pe.MeetingURL
	}
	if pe.RecurrenceRule != "" {
		recRule = &pe.RecurrenceRule
	}

	event := &domain.CalendarEvent{
		UserID:         userID,
		ProviderID:     pe.ID,
		Title:          pe.Title,
		Description:    desc,
		Location:       loc,
		StartTime:      pe.StartTime,
		EndTime:        pe.EndTime,
		IsAllDay:       pe.IsAllDay,
		Timezone:       pe.Timezone,
		Status:         domain.EventStatus(pe.Status),
		Organizer:      org,
		IsRecurring:    pe.IsRecurring,
		RecurrenceRule: recRule,
		MeetingURL:     meetURL,
	}

	// Convert attendees
	if len(pe.Attendees) > 0 {
		event.Attendees = make([]string, len(pe.Attendees))
		for i, att := range pe.Attendees {
			event.Attendees[i] = att.Email
		}
	}

	// Convert reminders
	if len(pe.Reminders) > 0 {
		event.Reminders = make([]int, len(pe.Reminders))
		for i, r := range pe.Reminders {
			event.Reminders[i] = r.Minutes
		}
	}

	return event
}

func (s *Service) CreateEvent(ctx context.Context, userID uuid.UUID, req *in.CreateEventRequest) (*domain.CalendarEvent, error) {
	if s.calendarRepo == nil {
		return nil, ErrRepoNotInitialized
	}

	event := &domain.CalendarEvent{
		CalendarID:  req.CalendarID,
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Location:    req.Location,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		IsAllDay:    req.IsAllDay,
		Timezone:    req.Timezone,
		Attendees:   req.Attendees,
		Reminders:   req.Reminders,
		Status:      domain.EventStatusConfirmed,
	}

	if err := s.calendarRepo.CreateEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

func (s *Service) UpdateEvent(ctx context.Context, eventID int64, req *in.UpdateEventRequest) (*domain.CalendarEvent, error) {
	if s.calendarRepo == nil {
		return nil, ErrRepoNotInitialized
	}

	event, err := s.calendarRepo.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		event.Title = *req.Title
	}
	if req.Description != nil {
		event.Description = req.Description
	}
	if req.Location != nil {
		event.Location = req.Location
	}
	if req.StartTime != nil {
		event.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		event.EndTime = *req.EndTime
	}
	if req.Attendees != nil {
		event.Attendees = req.Attendees
	}

	if err := s.calendarRepo.UpdateEvent(event); err != nil {
		return nil, err
	}

	return event, nil
}

func (s *Service) DeleteEvent(ctx context.Context, eventID int64) error {
	if s.calendarRepo == nil {
		return ErrRepoNotInitialized
	}
	return s.calendarRepo.DeleteEvent(eventID)
}

func (s *Service) SyncCalendars(ctx context.Context, connectionID int64) error {
	provider, err := s.getProviderForConnection(ctx, connectionID)
	if err != nil {
		return err
	}

	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return err
	}

	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return err
	}

	log.Printf("[CalendarService.SyncCalendars] Starting sync for connection %d", connectionID)

	// 1. Fetch events from provider
	timeMin := time.Now().AddDate(0, 0, -30)
	timeMax := time.Now().AddDate(0, 0, 90)

	result, err := provider.InitialSync(ctx, token, &out.CalendarSyncOptions{
		CalendarID: "primary",
		TimeMin:    &timeMin,
		TimeMax:    &timeMax,
		MaxResults: 250,
	})
	if err != nil {
		return err
	}

	log.Printf("[CalendarService.SyncCalendars] Fetched %d events from provider", len(result.Events))

	// 2. Save to DB
	if s.calendarRepo != nil {
		savedCount := 0
		for _, pe := range result.Events {
			event := s.convertProviderEvent(pe, conn.UserID, connectionID)
			if err := s.calendarRepo.CreateEvent(event); err != nil {
				log.Printf("[CalendarService.SyncCalendars] Failed to save event: %v", err)
				continue
			}
			savedCount++
		}
		log.Printf("[CalendarService.SyncCalendars] Saved %d/%d events", savedCount, len(result.Events))
	}

	return nil
}
