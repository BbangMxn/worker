"use client";

import { memo, useMemo } from "react";
import { cn } from "@/shared/lib";
import { EventCard, type CalendarEvent } from "@/entities/calendar";

interface CalendarDayViewProps {
  date: Date;
  events: CalendarEvent[];
  selectedEventId?: string;
  onSelectEvent: (event: CalendarEvent) => void;
}

const HOURS = Array.from({ length: 24 }, (_, i) => i);

function formatHour(hour: number): string {
  return `${hour.toString().padStart(2, "0")}:00`;
}

function getEventPosition(event: CalendarEvent): { top: number; height: number } {
  const start = new Date(event.startTime);
  const end = new Date(event.endTime);

  const startMinutes = start.getHours() * 60 + start.getMinutes();
  const endMinutes = end.getHours() * 60 + end.getMinutes();

  const top = (startMinutes / (24 * 60)) * 100;
  const height = ((endMinutes - startMinutes) / (24 * 60)) * 100;

  return { top, height: Math.max(height, 2) }; // Minimum 2% height for visibility
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

export const CalendarDayView = memo(function CalendarDayView({
  date,
  events,
  selectedEventId,
  onSelectEvent,
}: CalendarDayViewProps) {
  const dayEvents = useMemo(() => {
    return events.filter((event) => {
      const eventDate = new Date(event.startTime);
      return isSameDay(eventDate, date) && !event.allDay;
    });
  }, [events, date]);

  const allDayEvents = useMemo(() => {
    return events.filter((event) => {
      const eventDate = new Date(event.startTime);
      return isSameDay(eventDate, date) && event.allDay;
    });
  }, [events, date]);

  const now = new Date();
  const isToday = isSameDay(date, now);
  const currentTimePosition = isToday
    ? ((now.getHours() * 60 + now.getMinutes()) / (24 * 60)) * 100
    : null;

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* All Day Events */}
      {allDayEvents.length > 0 && (
        <div className="shrink-0 border-b border-border-subtle px-4 py-2">
          <div className="flex items-center gap-2">
            <span className="text-xs text-text-tertiary w-16 shrink-0">All day</span>
            <div className="flex-1 flex flex-wrap gap-2">
              {allDayEvents.map((event) => (
                <EventCard
                  key={event.id}
                  event={event}
                  selected={event.id === selectedEventId}
                  onClick={() => onSelectEvent(event)}
                  compact
                />
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Time Grid */}
      <div className="flex-1 overflow-y-auto">
        <div className="relative" style={{ height: `${24 * 60}px` }}>
          {/* Hour Lines */}
          {HOURS.map((hour) => (
            <div
              key={hour}
              className="absolute left-0 right-0 border-t border-border-subtle"
              style={{ top: `${(hour / 24) * 100}%` }}
            >
              <span className="absolute -top-2.5 left-2 text-xs text-text-tertiary bg-bg-base px-1">
                {formatHour(hour)}
              </span>
            </div>
          ))}

          {/* Current Time Indicator */}
          {currentTimePosition !== null && (
            <div
              className="absolute left-16 right-0 z-20 pointer-events-none"
              style={{ top: `${currentTimePosition}%` }}
            >
              <div className="relative">
                <div className="absolute -left-1.5 -top-1.5 w-3 h-3 rounded-full bg-red-500" />
                <div className="h-0.5 bg-red-500" />
              </div>
            </div>
          )}

          {/* Events */}
          <div className="absolute left-20 right-4 top-0 bottom-0">
            {dayEvents.map((event) => {
              const { top, height } = getEventPosition(event);
              return (
                <div
                  key={event.id}
                  className="absolute left-0 right-0 px-1"
                  style={{ top: `${top}%`, height: `${height}%`, minHeight: "40px" }}
                >
                  <EventCard
                    event={event}
                    selected={event.id === selectedEventId}
                    onClick={() => onSelectEvent(event)}
                  />
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
});
