// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"fmt"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// CalendarDomainWrapper wraps CalendarAdapter to implement domain.CalendarRepository.
// Note: CalendarAdapter stores events with user-based access, not calendar-based.
// This wrapper adapts the interface for domain usage.
type CalendarDomainWrapper struct {
	adapter *CalendarAdapter
}

// NewCalendarDomainWrapper creates a wrapper that implements domain.CalendarRepository.
func NewCalendarDomainWrapper(adapter *CalendarAdapter) *CalendarDomainWrapper {
	return &CalendarDomainWrapper{adapter: adapter}
}

// GetCalendarByID implements domain.CalendarRepository.
// Note: Current adapter doesn't have separate calendar table, returns synthetic calendar.
func (w *CalendarDomainWrapper) GetCalendarByID(id int64) (*domain.Calendar, error) {
	// Since adapter doesn't have calendar table, return nil
	return nil, nil
}

// GetCalendarsByUser implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) GetCalendarsByUser(userID uuid.UUID) ([]*domain.Calendar, error) {
	ctx := context.Background()

	// Get calendars from DB
	entities, err := w.adapter.ListCalendarsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert to domain calendars
	calendars := make([]*domain.Calendar, len(entities))
	for i, e := range entities {
		var desc, color *string
		if e.Description != "" {
			desc = &e.Description
		}
		if e.Color != "" {
			color = &e.Color
		}

		calendars[i] = &domain.Calendar{
			ID:           e.ID,
			UserID:       e.UserID,
			ConnectionID: e.ConnectionID,
			Provider:     domain.CalendarProvider(e.Provider),
			ProviderID:   e.ProviderID,
			Name:         e.Name,
			Description:  desc,
			Color:        color,
			IsDefault:    e.IsDefault,
			CreatedAt:    e.CreatedAt,
			UpdatedAt:    e.UpdatedAt,
		}
	}

	return calendars, nil
}

// CreateCalendar implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) CreateCalendar(cal *domain.Calendar) error {
	ctx := context.Background()

	// Convert domain calendar to entity
	entity := &out.CalendarListEntity{
		UserID:       cal.UserID,
		ConnectionID: cal.ConnectionID,
		Provider:     string(cal.Provider),
		ProviderID:   cal.ProviderID,
		Name:         cal.Name,
		IsDefault:    cal.IsDefault,
	}
	if cal.Description != nil {
		entity.Description = *cal.Description
	}
	if cal.Color != nil {
		entity.Color = *cal.Color
	}

	// Save to calendars table
	if err := w.adapter.CreateCalendarList(ctx, entity); err != nil {
		return err
	}

	// Update the domain object with the generated ID
	cal.ID = entity.ID
	return nil
}

// UpdateCalendar implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) UpdateCalendar(cal *domain.Calendar) error {
	// Not supported
	return nil
}

// DeleteCalendar implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) DeleteCalendar(id int64) error {
	// Not supported
	return nil
}

// GetEventByID implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) GetEventByID(id int64) (*domain.CalendarEvent, error) {
	// Need userID - use empty UUID and rely on ID match
	ctx := context.Background()
	entity, err := w.adapter.GetByID(ctx, uuid.Nil, id)
	if err != nil {
		return nil, err
	}
	return w.entityToDomain(entity), nil
}

// ListEvents implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) ListEvents(filter *domain.CalendarEventFilter) ([]*domain.CalendarEvent, int, error) {
	ctx := context.Background()

	query := &out.CalendarListQuery{
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	if filter.StartTime != nil {
		query.StartTime = filter.StartTime
	}
	if filter.EndTime != nil {
		query.EndTime = filter.EndTime
	}
	if filter.Search != nil {
		query.Search = *filter.Search
	}
	if filter.Status != nil {
		query.Status = string(*filter.Status)
	}

	entities, total, err := w.adapter.List(ctx, filter.UserID, query)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*domain.CalendarEvent, len(entities))
	for i, e := range entities {
		result[i] = w.entityToDomain(e)
	}
	return result, total, nil
}

// CreateEvent implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) CreateEvent(event *domain.CalendarEvent) error {
	ctx := context.Background()
	entity := w.domainToEntity(event)
	if err := w.adapter.Create(ctx, entity); err != nil {
		return err
	}
	event.ID = entity.ID
	return nil
}

// UpdateEvent implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) UpdateEvent(event *domain.CalendarEvent) error {
	ctx := context.Background()
	entity := w.domainToEntity(event)
	return w.adapter.Update(ctx, entity)
}

// DeleteEvent implements domain.CalendarRepository.
func (w *CalendarDomainWrapper) DeleteEvent(id int64) error {
	ctx := context.Background()
	return w.adapter.Delete(ctx, uuid.Nil, id)
}

// entityToDomain converts out.CalendarEventEntity to domain.CalendarEvent.
func (w *CalendarDomainWrapper) entityToDomain(e *out.CalendarEventEntity) *domain.CalendarEvent {
	if e == nil {
		return nil
	}

	var desc, loc, org, meetURL, recRule *string
	if e.Description != "" {
		desc = &e.Description
	}
	if e.Location != "" {
		loc = &e.Location
	}
	if e.OrganizerEmail != "" {
		org = &e.OrganizerEmail
	}
	if e.MeetingURL != "" {
		meetURL = &e.MeetingURL
	}
	if e.RecurrenceRule != "" {
		recRule = &e.RecurrenceRule
	}

	// Convert attendees
	attendees := make([]string, len(e.Attendees))
	for i, a := range e.Attendees {
		attendees[i] = a.Email
	}

	// Convert reminders
	reminders := make([]int, len(e.Reminders))
	for i, r := range e.Reminders {
		reminders[i] = r.Minutes
	}

	return &domain.CalendarEvent{
		ID:             e.ID,
		UserID:         e.UserID,
		ProviderID:     e.ProviderID,
		Title:          e.Title,
		Description:    desc,
		Location:       loc,
		StartTime:      e.StartTime,
		EndTime:        e.EndTime,
		IsAllDay:       e.IsAllDay,
		Timezone:       e.Timezone,
		Status:         domain.EventStatus(e.Status),
		Organizer:      org,
		Attendees:      attendees,
		IsRecurring:    e.IsRecurring,
		RecurrenceRule: recRule,
		Reminders:      reminders,
		MeetingURL:     meetURL,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

// domainToEntity converts domain.CalendarEvent to out.CalendarEventEntity.
func (w *CalendarDomainWrapper) domainToEntity(e *domain.CalendarEvent) *out.CalendarEventEntity {
	if e == nil {
		return nil
	}

	entity := &out.CalendarEventEntity{
		ID:           e.ID,
		UserID:       e.UserID,
		ProviderID:   e.ProviderID,
		Title:        e.Title,
		StartTime:    e.StartTime,
		EndTime:      e.EndTime,
		IsAllDay:     e.IsAllDay,
		Timezone:     e.Timezone,
		Status:       string(e.Status),
		IsRecurring:  e.IsRecurring,
		Visibility:   "default",
		Transparency: "opaque",
		Categories:   []string{},
		Attachments:  []out.CalendarAttachmentEntity{},
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}

	// CalendarID: domain uses int64, entity uses string
	if e.CalendarID > 0 {
		entity.CalendarID = fmt.Sprintf("%d", e.CalendarID)
	}

	if e.Description != nil {
		entity.Description = *e.Description
	}
	if e.Location != nil {
		entity.Location = *e.Location
	}
	if e.Organizer != nil {
		entity.OrganizerEmail = *e.Organizer
	}
	if e.MeetingURL != nil {
		entity.MeetingURL = *e.MeetingURL
	}
	if e.RecurrenceRule != nil {
		entity.RecurrenceRule = *e.RecurrenceRule
	}

	// Convert attendees (ensure not nil)
	entity.Attendees = make([]out.AttendeeEntity, 0, len(e.Attendees))
	for _, email := range e.Attendees {
		entity.Attendees = append(entity.Attendees, out.AttendeeEntity{Email: email})
	}

	// Convert reminders (ensure not nil)
	entity.Reminders = make([]out.ReminderEntity, 0, len(e.Reminders))
	for _, mins := range e.Reminders {
		entity.Reminders = append(entity.Reminders, out.ReminderEntity{Minutes: mins})
	}

	return entity
}

// Ensure CalendarDomainWrapper implements domain.CalendarRepository
var _ domain.CalendarRepository = (*CalendarDomainWrapper)(nil)
