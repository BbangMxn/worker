package calendar

import (
	"context"
	"fmt"
	"log"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// SyncService handles calendar synchronization with real-time push notifications.
type SyncService struct {
	calendarRepo     domain.CalendarRepository
	syncRepo         out.CalendarSyncRepository
	calendarProvider out.CalendarProviderPort
	oauthService     *auth.OAuthService
	messageProducer  out.MessageProducer
	realtime         out.RealtimePort
}

// NewSyncService creates a new calendar sync service.
func NewSyncService(
	calendarRepo domain.CalendarRepository,
	syncRepo out.CalendarSyncRepository,
	calendarProvider out.CalendarProviderPort,
	oauthService *auth.OAuthService,
	messageProducer out.MessageProducer,
	realtime out.RealtimePort,
) *SyncService {
	return &SyncService{
		calendarRepo:     calendarRepo,
		syncRepo:         syncRepo,
		calendarProvider: calendarProvider,
		oauthService:     oauthService,
		messageProducer:  messageProducer,
		realtime:         realtime,
	}
}

// =============================================================================
// Initial Sync
// =============================================================================

// InitialSync performs initial calendar sync after OAuth connection.
func (s *SyncService) InitialSync(ctx context.Context, userID string, connectionID int64) error {
	log.Printf("[SyncService.InitialSync] Starting for connection %d", connectionID)

	// 1. Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	// 2. Get connection info
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	// 3. List all calendars
	calendars, err := s.calendarProvider.ListCalendars(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to list calendars: %w", err)
	}

	log.Printf("[SyncService.InitialSync] Found %d calendars", len(calendars))

	// 4. Sync each calendar
	for _, providerCal := range calendars {
		// Save calendar to DB
		calendar := s.convertProviderCalendar(providerCal, userID, connectionID, conn.Provider)
		if s.calendarRepo != nil {
			if err := s.calendarRepo.CreateCalendar(calendar); err != nil {
				log.Printf("[SyncService] Failed to save calendar: %v", err)
				continue
			}
		}

		// Sync events for this calendar
		if err := s.syncCalendarEvents(ctx, token, userID, connectionID, providerCal.ID, calendar.ID); err != nil {
			log.Printf("[SyncService] Failed to sync events for calendar %s: %v", providerCal.ID, err)
			continue
		}

		// Setup watch for push notifications
		watchResp, err := s.calendarProvider.Watch(ctx, token, providerCal.ID)
		if err != nil {
			log.Printf("[SyncService] Failed to setup watch for calendar %s: %v", providerCal.ID, err)
		} else if s.syncRepo != nil {
			s.syncRepo.UpdateWatchExpiry(ctx, connectionID, providerCal.ID, watchResp.Expiration, watchResp.ChannelID)
			log.Printf("[SyncService] Watch setup for calendar %s, expires %v", providerCal.ID, watchResp.Expiration)
		}
	}

	// 5. Send realtime notification
	if s.realtime != nil {
		s.realtime.Push(ctx, userID, &domain.RealtimeEvent{
			Type:      domain.EventCalendarSyncCompleted,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"connection_id":   connectionID,
				"calendars_count": len(calendars),
			},
		})
	}

	log.Printf("[SyncService.InitialSync] Completed for connection %d", connectionID)
	return nil
}

// syncCalendarEvents syncs events for a single calendar.
func (s *SyncService) syncCalendarEvents(ctx context.Context, token *oauth2.Token, userID string, connectionID int64, providerCalID string, localCalID int64) error {
	// Default: sync 30 days back, 90 days forward
	timeMin := time.Now().AddDate(0, 0, -30)
	timeMax := time.Now().AddDate(0, 0, 90)

	result, err := s.calendarProvider.InitialSync(ctx, token, &out.CalendarSyncOptions{
		CalendarID: providerCalID,
		TimeMin:    &timeMin,
		TimeMax:    &timeMax,
		MaxResults: 250,
	})
	if err != nil {
		return fmt.Errorf("failed to sync events: %w", err)
	}

	log.Printf("[SyncService] Synced %d events for calendar %s", len(result.Events), providerCalID)

	// Save events to DB
	savedCount := 0
	for _, providerEvent := range result.Events {
		event := s.convertProviderEvent(providerEvent, userID, localCalID)
		if s.calendarRepo != nil {
			if err := s.calendarRepo.CreateEvent(event); err != nil {
				log.Printf("[SyncService] Failed to save event: %v", err)
				continue
			}
			savedCount++
		}
	}

	// Save sync token for incremental sync
	if s.syncRepo != nil && result.NextSyncToken != "" {
		s.syncRepo.UpdateSyncToken(ctx, connectionID, providerCalID, result.NextSyncToken)
	}

	log.Printf("[SyncService] Saved %d/%d events", savedCount, len(result.Events))
	return nil
}

// =============================================================================
// Delta Sync (Incremental)
// =============================================================================

// DeltaSync performs incremental sync triggered by webhook notification.
func (s *SyncService) DeltaSync(ctx context.Context, connectionID int64, calendarID string) error {
	log.Printf("[SyncService.DeltaSync] Starting for connection %d, calendar %s", connectionID, calendarID)

	// 1. Get sync state
	state, err := s.syncRepo.GetByCalendarID(ctx, connectionID, calendarID)
	if err != nil || state == nil {
		log.Printf("[SyncService.DeltaSync] No sync state found, performing initial sync")
		conn, err := s.oauthService.GetConnection(ctx, connectionID)
		if err != nil {
			return err
		}
		return s.InitialSync(ctx, conn.UserID.String(), connectionID)
	}

	// 2. Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	// 3. Incremental sync using sync token
	result, err := s.calendarProvider.IncrementalSync(ctx, token, calendarID, state.SyncToken)
	if err != nil {
		// Sync token expired - need full sync
		if providerErr, ok := err.(*out.ProviderError); ok && providerErr.Code == out.ProviderErrSyncRequired {
			log.Printf("[SyncService.DeltaSync] Sync token expired, performing full sync")
			return s.InitialSync(ctx, state.UserID, connectionID)
		}
		return fmt.Errorf("failed to incremental sync: %w", err)
	}

	// 4. Get local calendar ID
	var localCalID int64
	if s.calendarRepo != nil {
		// Find local calendar by provider ID
		calendars, _ := s.calendarRepo.GetCalendarsByUser(uuid.MustParse(state.UserID))
		for _, cal := range calendars {
			if cal.ProviderID == calendarID {
				localCalID = cal.ID
				break
			}
		}
	}

	// 5. Process new/updated events
	for _, providerEvent := range result.Events {
		event := s.convertProviderEvent(providerEvent, state.UserID, localCalID)
		if s.calendarRepo != nil {
			// Try update first, then create
			if err := s.calendarRepo.UpdateEvent(event); err != nil {
				s.calendarRepo.CreateEvent(event)
			}
		}

		// Send realtime notification for new events
		if s.realtime != nil {
			s.realtime.Push(ctx, state.UserID, &domain.RealtimeEvent{
				Type:      domain.EventCalendarUpdated,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"event_id":    event.ID,
					"title":       event.Title,
					"start_time":  event.StartTime,
					"calendar_id": localCalID,
				},
			})
		}
	}

	// 6. Process deleted events
	for _, deletedID := range result.DeletedIDs {
		if s.calendarRepo != nil {
			// Find and delete by provider ID
			events, _, _ := s.calendarRepo.ListEvents(&domain.CalendarEventFilter{
				UserID: uuid.MustParse(state.UserID),
			})
			for _, e := range events {
				if e.ProviderID == deletedID {
					s.calendarRepo.DeleteEvent(e.ID)
					break
				}
			}
		}
	}

	// 7. Update sync token
	if s.syncRepo != nil && result.NextSyncToken != "" {
		s.syncRepo.UpdateSyncToken(ctx, connectionID, calendarID, result.NextSyncToken)
	}

	log.Printf("[SyncService.DeltaSync] Completed: %d updated, %d deleted",
		len(result.Events), len(result.DeletedIDs))
	return nil
}

// =============================================================================
// Watch Management
// =============================================================================

// RenewExpiredWatches renews watches that are about to expire.
func (s *SyncService) RenewExpiredWatches(ctx context.Context) error {
	if s.syncRepo == nil {
		return nil
	}

	// Get watches expiring in the next 24 hours
	expireBefore := time.Now().Add(24 * time.Hour)
	states, err := s.syncRepo.GetExpiredWatches(ctx, expireBefore)
	if err != nil {
		return fmt.Errorf("failed to get expired watches: %w", err)
	}

	log.Printf("[SyncService.RenewExpiredWatches] Found %d watches to renew", len(states))

	for _, state := range states {
		token, err := s.oauthService.GetOAuth2Token(ctx, state.ConnectionID)
		if err != nil {
			log.Printf("[SyncService] Failed to get token for connection %d: %v", state.ConnectionID, err)
			continue
		}

		// Stop old watch
		if state.WatchID != "" {
			s.calendarProvider.StopWatch(ctx, token, state.WatchID, state.CalendarID)
		}

		// Create new watch
		watchResp, err := s.calendarProvider.Watch(ctx, token, state.CalendarID)
		if err != nil {
			log.Printf("[SyncService] Failed to renew watch for calendar %s: %v", state.CalendarID, err)
			s.syncRepo.UpdateStatus(ctx, state.ConnectionID, state.CalendarID, "watch_expired", err.Error())
			continue
		}

		s.syncRepo.UpdateWatchExpiry(ctx, state.ConnectionID, state.CalendarID, watchResp.Expiration, watchResp.ChannelID)
		log.Printf("[SyncService] Renewed watch for calendar %s, expires %v", state.CalendarID, watchResp.Expiration)
	}

	return nil
}

// =============================================================================
// Free/Busy Query
// =============================================================================

// GetFreeBusy queries free/busy information for calendars.
func (s *SyncService) GetFreeBusy(ctx context.Context, connectionID int64, calendarIDs []string, timeMin, timeMax time.Time) (map[string][]*out.TimePeriod, error) {
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	resp, err := s.calendarProvider.GetFreeBusy(ctx, token, &out.FreeBusyRequest{
		CalendarIDs: calendarIDs,
		TimeMin:     timeMin,
		TimeMax:     timeMax,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get free/busy: %w", err)
	}

	return resp.Calendars, nil
}

// FindFreeSlots finds available time slots.
func (s *SyncService) FindFreeSlots(ctx context.Context, connectionID int64, calendarIDs []string, timeMin, timeMax time.Time, duration time.Duration) ([]out.TimePeriod, error) {
	busyPeriods, err := s.GetFreeBusy(ctx, connectionID, calendarIDs, timeMin, timeMax)
	if err != nil {
		return nil, err
	}

	// Merge all busy periods
	allBusy := make([]*out.TimePeriod, 0)
	for _, periods := range busyPeriods {
		allBusy = append(allBusy, periods...)
	}

	// Sort by start time
	sortTimePeriods(allBusy)

	// Find free slots
	freeSlots := make([]out.TimePeriod, 0)
	currentTime := timeMin

	for _, busy := range allBusy {
		if busy.Start.After(currentTime) {
			gap := busy.Start.Sub(currentTime)
			if gap >= duration {
				freeSlots = append(freeSlots, out.TimePeriod{
					Start: currentTime,
					End:   busy.Start,
				})
			}
		}
		if busy.End.After(currentTime) {
			currentTime = busy.End
		}
	}

	// Check remaining time
	if timeMax.After(currentTime) && timeMax.Sub(currentTime) >= duration {
		freeSlots = append(freeSlots, out.TimePeriod{
			Start: currentTime,
			End:   timeMax,
		})
	}

	return freeSlots, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func (s *SyncService) convertProviderCalendar(pc *out.ProviderCalendar, userID string, connectionID int64, provider domain.OAuthProvider) *domain.Calendar {
	var desc, color *string
	if pc.Description != "" {
		desc = &pc.Description
	}
	if pc.Color != "" {
		color = &pc.Color
	}

	return &domain.Calendar{
		UserID:       uuid.MustParse(userID),
		ConnectionID: connectionID,
		Provider:     domain.CalendarProvider(provider),
		ProviderID:   pc.ID,
		Name:         pc.Name,
		Description:  desc,
		Color:        color,
		IsDefault:    pc.IsPrimary,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func (s *SyncService) convertProviderEvent(pe *out.ProviderCalendarEvent, userID string, calendarID int64) *domain.CalendarEvent {
	var desc, loc, org *string
	if pe.Description != "" {
		desc = &pe.Description
	}
	if pe.Location != "" {
		loc = &pe.Location
	}
	if pe.OrganizerEmail != "" {
		org = &pe.OrganizerEmail
	}

	event := &domain.CalendarEvent{
		CalendarID:  calendarID,
		UserID:      uuid.MustParse(userID),
		ProviderID:  pe.ID,
		Title:       pe.Title,
		Description: desc,
		Location:    loc,
		StartTime:   pe.StartTime,
		EndTime:     pe.EndTime,
		IsAllDay:    pe.IsAllDay,
		Timezone:    pe.Timezone,
		Status:      domain.EventStatus(pe.Status),
		Organizer:   org,
		IsRecurring: pe.IsRecurring,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
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

	// Meeting URL
	if pe.MeetingURL != "" {
		event.MeetingURL = &pe.MeetingURL
	}

	// Recurrence
	if pe.RecurrenceRule != "" {
		event.RecurrenceRule = &pe.RecurrenceRule
	}

	return event
}

func sortTimePeriods(periods []*out.TimePeriod) {
	for i := 0; i < len(periods)-1; i++ {
		for j := i + 1; j < len(periods); j++ {
			if periods[j].Start.Before(periods[i].Start) {
				periods[i], periods[j] = periods[j], periods[i]
			}
		}
	}
}
