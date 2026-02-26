"use client";

import { memo, useMemo } from "react";
import { cn } from "@/shared/lib";
import { EventCard, type CalendarEvent } from "@/entities/calendar";

interface CalendarWeekViewProps {
  date: Date;
  events: CalendarEvent[];
  selectedEventId?: string;
  onSelectEvent: (event: CalendarEvent) => void;
  onSelectDate: (date: Date) => void;
}

const HOURS = Array.from({ length: 24 }, (_, i) => i);

function formatHour(hour: number): string {
  return `${hour.toString().padStart(2, "0")}:00`;
}

function getWeekDays(date: Date): Date[] {
  const day = date.getDay();
  const start = new Date(date);
  start.setDate(date.getDate() - day);

  return Array.from({ length: 7 }, (_, i) => {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    return d;
  });
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function getEventPosition(event: CalendarEvent): { top: number; height: number } {
  const start = new Date(event.startTime);
  const end = new Date(event.endTime);

  const startMinutes = start.getHours() * 60 + start.getMinutes();
  const endMinutes = end.getHours() * 60 + end.getMinutes();

  const top = (startMinutes / (24 * 60)) * 100;
  const height = ((endMinutes - startMinutes) / (24 * 60)) * 100;

  return { top, height: Math.max(height, 2) };
}

export const CalendarWeekView = memo(function CalendarWeekView({
  date,
  events,
  selectedEventId,
  onSelectEvent,
  onSelectDate,
}: CalendarWeekViewProps) {
  const weekDays = useMemo(() => getWeekDays(date), [date]);
  const today = useMemo(() => new Date(), []);

  const eventsByDay = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>();
    weekDays.forEach((day) => {
      const key = day.toDateString();
      map.set(
        key,
        events.filter((event) => {
          const eventDate = new Date(event.startTime);
          return isSameDay(eventDate, day) && !event.allDay;
        })
      );
    });
    return map;
  }, [events, weekDays]);

  const allDayEvents = useMemo(() => {
    return events.filter((event) => {
      const eventDate = new Date(event.startTime);
      return weekDays.some((day) => isSameDay(eventDate, day)) && event.allDay;
    });
  }, [events, weekDays]);

  const now = new Date();
  const currentTimePosition = ((now.getHours() * 60 + now.getMinutes()) / (24 * 60)) * 100;

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="shrink-0 border-b border-border-subtle">
        <div className="flex">
          <div className="w-16 shrink-0" />
          {weekDays.map((day, index) => {
            const isToday = isSameDay(day, today);
            const dayOfWeek = day.getDay();
            return (
              <div
                key={index}
                className="flex-1 text-center py-2 border-l border-border-subtle first:border-l-0"
              >
                <div
                  className={cn(
                    "text-xs font-medium mb-1",
                    dayOfWeek === 0 ? "text-red-500" : dayOfWeek === 6 ? "text-blue-500" : "text-text-tertiary"
                  )}
                >
                  {day.toLocaleDateString("ko-KR", { weekday: "short" })}
                </div>
                <button
                  onClick={() => onSelectDate(day)}
                  className={cn(
                    "w-8 h-8 rounded-full text-sm font-semibold transition-colors",
                    isToday
                      ? "bg-accent-primary text-white"
                      : "text-text-primary hover:bg-bg-hover"
                  )}
                >
                  {day.getDate()}
                </button>
              </div>
            );
          })}
        </div>

        {/* All Day Events Row */}
        {allDayEvents.length > 0 && (
          <div className="flex border-t border-border-subtle">
            <div className="w-16 shrink-0 px-2 py-1 text-xs text-text-tertiary">
              All day
            </div>
            {weekDays.map((day, index) => {
              const dayAllDayEvents = allDayEvents.filter((e) =>
                isSameDay(new Date(e.startTime), day)
              );
              return (
                <div
                  key={index}
                  className="flex-1 border-l border-border-subtle first:border-l-0 p-1 space-y-1"
                >
                  {dayAllDayEvents.map((event) => (
                    <EventCard
                      key={event.id}
                      event={event}
                      selected={event.id === selectedEventId}
                      onClick={() => onSelectEvent(event)}
                      compact
                    />
                  ))}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Time Grid */}
      <div className="flex-1 overflow-y-auto">
        <div className="flex relative" style={{ height: `${24 * 60}px` }}>
          {/* Time Column */}
          <div className="w-16 shrink-0 relative">
            {HOURS.map((hour) => (
              <div
                key={hour}
                className="absolute left-0 right-0"
                style={{ top: `${(hour / 24) * 100}%` }}
              >
                <span className="absolute -top-2 left-2 text-xs text-text-tertiary">
                  {formatHour(hour)}
                </span>
              </div>
            ))}
          </div>

          {/* Day Columns */}
          {weekDays.map((day, dayIndex) => {
            const dayEvents = eventsByDay.get(day.toDateString()) || [];
            const isToday = isSameDay(day, today);

            return (
              <div
                key={dayIndex}
                className="flex-1 relative border-l border-border-subtle first:border-l-0"
              >
                {/* Hour Lines */}
                {HOURS.map((hour) => (
                  <div
                    key={hour}
                    className="absolute left-0 right-0 border-t border-border-subtle"
                    style={{ top: `${(hour / 24) * 100}%` }}
                  />
                ))}

                {/* Current Time Indicator */}
                {isToday && (
                  <div
                    className="absolute left-0 right-0 z-20 pointer-events-none"
                    style={{ top: `${currentTimePosition}%` }}
                  >
                    <div className="h-0.5 bg-red-500" />
                  </div>
                )}

                {/* Events */}
                {dayEvents.map((event) => {
                  const { top, height } = getEventPosition(event);
                  return (
                    <div
                      key={event.id}
                      className="absolute left-1 right-1"
                      style={{ top: `${top}%`, height: `${height}%`, minHeight: "30px" }}
                    >
                      <EventCard
                        event={event}
                        selected={event.id === selectedEventId}
                        onClick={() => onSelectEvent(event)}
                        compact
                      />
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
});
