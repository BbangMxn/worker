"use client";

import { memo, useMemo } from "react";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { cn } from "@/shared/lib";
import type { Calendar, CalendarEvent, EventColor } from "@/entities/calendar";

interface CalendarSidebarProps {
  currentDate: Date;
  calendars: Calendar[];
  events: CalendarEvent[];
  onDateChange: (date: Date) => void;
  onToggleCalendar: (calendarId: string) => void;
  onCreateEvent: () => void;
}

const COLOR_DOTS: Record<EventColor, string> = {
  blue: "bg-blue-500",
  green: "bg-green-500",
  red: "bg-red-500",
  yellow: "bg-yellow-500",
  purple: "bg-purple-500",
  orange: "bg-orange-500",
  pink: "bg-pink-500",
  cyan: "bg-cyan-500",
};

function getDaysInMonth(year: number, month: number): Date[] {
  const days: Date[] = [];
  const firstDay = new Date(year, month, 1);
  const lastDay = new Date(year, month + 1, 0);

  // Add days from previous month to fill the first week
  const startDay = firstDay.getDay();
  for (let i = startDay - 1; i >= 0; i--) {
    days.push(new Date(year, month, -i));
  }

  // Add days of current month
  for (let i = 1; i <= lastDay.getDate(); i++) {
    days.push(new Date(year, month, i));
  }

  // Add days from next month to complete the last week
  const remaining = 42 - days.length; // 6 weeks * 7 days
  for (let i = 1; i <= remaining; i++) {
    days.push(new Date(year, month + 1, i));
  }

  return days;
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function isSameMonth(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth();
}

export const CalendarSidebar = memo(function CalendarSidebar({
  currentDate,
  calendars,
  events,
  onDateChange,
  onToggleCalendar,
  onCreateEvent,
}: CalendarSidebarProps) {
  const today = useMemo(() => new Date(), []);
  const days = useMemo(
    () => getDaysInMonth(currentDate.getFullYear(), currentDate.getMonth()),
    [currentDate]
  );

  const eventsMap = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>();
    events.forEach((event) => {
      const dateKey = new Date(event.startTime).toDateString();
      if (!map.has(dateKey)) map.set(dateKey, []);
      map.get(dateKey)!.push(event);
    });
    return map;
  }, [events]);

  const handlePrevMonth = () => {
    onDateChange(new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1));
  };

  const handleNextMonth = () => {
    onDateChange(new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1));
  };

  const handleToday = () => {
    onDateChange(new Date());
  };

  return (
    <div className="w-[280px] flex flex-col border-r border-border-default bg-bg-base shrink-0">
      {/* Create Button */}
      <div className="p-4">
        <button
          onClick={onCreateEvent}
          className={cn(
            "w-full flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg",
            "bg-accent-primary hover:bg-accent-hover text-white",
            "font-medium transition-colors shadow-sm"
          )}
        >
          <Plus size={20} strokeWidth={2} />
          <span>New Event</span>
        </button>
      </div>

      {/* Mini Calendar */}
      <div className="px-4 pb-4">
        {/* Month Navigation */}
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-semibold text-text-primary">
            {currentDate.toLocaleDateString("ko-KR", {
              year: "numeric",
              month: "long",
            })}
          </h3>
          <div className="flex items-center gap-1">
            <button
              onClick={handlePrevMonth}
              className="p-1 rounded hover:bg-bg-hover transition-colors"
              aria-label="Previous month"
            >
              <ChevronLeft size={18} className="text-text-secondary" />
            </button>
            <button
              onClick={handleToday}
              className="px-2 py-0.5 text-xs font-medium text-text-secondary hover:bg-bg-hover rounded transition-colors"
            >
              Today
            </button>
            <button
              onClick={handleNextMonth}
              className="p-1 rounded hover:bg-bg-hover transition-colors"
              aria-label="Next month"
            >
              <ChevronRight size={18} className="text-text-secondary" />
            </button>
          </div>
        </div>

        {/* Day Headers */}
        <div className="grid grid-cols-7 mb-1">
          {["일", "월", "화", "수", "목", "금", "토"].map((day, i) => (
            <div
              key={day}
              className={cn(
                "text-center text-xs font-medium py-1",
                i === 0 ? "text-red-500" : i === 6 ? "text-blue-500" : "text-text-tertiary"
              )}
            >
              {day}
            </div>
          ))}
        </div>

        {/* Days Grid */}
        <div className="grid grid-cols-7 gap-0.5">
          {days.map((day, index) => {
            const isCurrentMonth = isSameMonth(day, currentDate);
            const isToday = isSameDay(day, today);
            const isSelected = isSameDay(day, currentDate);
            const dayEvents = eventsMap.get(day.toDateString()) || [];
            const dayOfWeek = day.getDay();

            return (
              <button
                key={index}
                onClick={() => onDateChange(day)}
                className={cn(
                  "relative aspect-square flex flex-col items-center justify-center rounded-full",
                  "text-sm transition-colors",
                  !isCurrentMonth && "text-text-disabled",
                  isCurrentMonth && dayOfWeek === 0 && "text-red-500",
                  isCurrentMonth && dayOfWeek === 6 && "text-blue-500",
                  isCurrentMonth && dayOfWeek > 0 && dayOfWeek < 6 && "text-text-primary",
                  isToday && !isSelected && "bg-bg-surface font-semibold",
                  isSelected && "bg-accent-primary text-white font-semibold",
                  !isSelected && "hover:bg-bg-hover"
                )}
              >
                {day.getDate()}
                {dayEvents.length > 0 && !isSelected && (
                  <span className="absolute bottom-1 w-1 h-1 rounded-full bg-accent-primary" />
                )}
              </button>
            );
          })}
        </div>
      </div>

      {/* My Calendars */}
      <div className="flex-1 overflow-y-auto px-4 pb-4">
        <h3 className="text-xs font-semibold text-text-tertiary uppercase tracking-wider mb-2">
          My Calendars
        </h3>
        <div className="space-y-1">
          {calendars.map((calendar) => (
            <label
              key={calendar.id}
              className="flex items-center gap-3 px-2 py-1.5 rounded-md hover:bg-bg-hover cursor-pointer transition-colors"
            >
              <input
                type="checkbox"
                checked={calendar.isVisible}
                onChange={() => onToggleCalendar(calendar.id)}
                className="sr-only"
              />
              <span
                className={cn(
                  "w-4 h-4 rounded border-2 flex items-center justify-center transition-colors",
                  calendar.isVisible
                    ? COLOR_DOTS[calendar.color].replace("bg-", "border-").replace("500", "500") +
                        " " +
                        COLOR_DOTS[calendar.color]
                    : "border-border-default"
                )}
              >
                {calendar.isVisible && (
                  <svg
                    className="w-2.5 h-2.5 text-white"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={3}
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                )}
              </span>
              <span className="text-sm text-text-primary">{calendar.name}</span>
            </label>
          ))}
        </div>
      </div>
    </div>
  );
});
