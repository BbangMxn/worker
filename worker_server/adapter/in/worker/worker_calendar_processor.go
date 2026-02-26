package worker

import (
	"context"
	"fmt"

	"worker_server/core/service/calendar"
	"worker_server/pkg/logger"
)

// CalendarProcessor handles calendar-related jobs.
type CalendarProcessor struct {
	calendarSyncService *calendar.SyncService
}

// NewCalendarProcessor creates a new calendar processor.
func NewCalendarProcessor(calendarSyncService *calendar.SyncService) *CalendarProcessor {
	return &CalendarProcessor{
		calendarSyncService: calendarSyncService,
	}
}

// ProcessSync processes calendar sync jobs using Push-based real-time sync.
func (p *CalendarProcessor) ProcessSync(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[CalendarSyncPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[CalendarProcessor.ProcessSync] connection=%d, user=%s, full=%v",
		payload.ConnectionID, payload.UserID, payload.FullSync)

	// CalendarSyncService is required for real-time sync
	if p.calendarSyncService == nil {
		return fmt.Errorf("calendarSyncService not initialized")
	}

	if payload.FullSync {
		// Initial sync: fetches calendars/events and sets up Watch for Push notifications
		return p.calendarSyncService.InitialSync(ctx, payload.UserID, payload.ConnectionID)
	}

	// Delta sync triggered by webhook notification
	if payload.CalendarID != "" {
		return p.calendarSyncService.DeltaSync(ctx, payload.ConnectionID, payload.CalendarID)
	}

	// No specific calendar - sync all calendars
	return p.calendarSyncService.InitialSync(ctx, payload.UserID, payload.ConnectionID)
}

// ProcessDeltaSync processes webhook-triggered delta sync.
func (p *CalendarProcessor) ProcessDeltaSync(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[CalendarSyncPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[CalendarProcessor.ProcessDeltaSync] connection=%d, calendar=%s",
		payload.ConnectionID, payload.CalendarID)

	if p.calendarSyncService == nil {
		return fmt.Errorf("calendarSyncService not initialized")
	}

	return p.calendarSyncService.DeltaSync(ctx, payload.ConnectionID, payload.CalendarID)
}

// ProcessRenewWatches processes watch renewal jobs.
func (p *CalendarProcessor) ProcessRenewWatches(ctx context.Context, msg *Message) error {
	logger.Info("[CalendarProcessor.ProcessRenewWatches] Renewing expired watches")

	if p.calendarSyncService == nil {
		return fmt.Errorf("calendarSyncService not initialized")
	}

	return p.calendarSyncService.RenewExpiredWatches(ctx)
}
