"use client";

import { useState, useMemo, useCallback, useEffect } from "react";
import { ChevronLeft, ChevronRight, Calendar } from "lucide-react";
import { cn } from "@/shared/lib";
import {
  mockEvents,
  mockCalendars,
  type CalendarEvent,
  type Calendar as CalendarType,
  type CalendarView,
} from "@/entities/calendar";
import { CalendarSidebar } from "@/widgets/calendar-sidebar";
import { CalendarDayView, CalendarWeekView } from "@/widgets/calendar-view";
import { EventDetail } from "@/widgets/event-detail";

export default function CalendarPage() {
  const [currentDate, setCurrentDate] = useState(new Date());
  const [view, setView] = useState<CalendarView>("week");
  const [calendars, setCalendars] = useState<CalendarType[]>(mockCalendars);
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(
    null,
  );

  // Listen for compose event from layout (new event)
  useEffect(() => {
    const handleCompose = () => {
      console.log("Create new event");
    };
    window.addEventListener("app:compose", handleCompose);
    return () => window.removeEventListener("app:compose", handleCompose);
  }, []);

  // Filter events by visible calendars
  const visibleEvents = useMemo(() => {
    const visibleCalendarIds = calendars
      .filter((c) => c.isVisible)
      .map((c) => c.id);
    return mockEvents.filter((e) => visibleCalendarIds.includes(e.calendarId));
  }, [calendars]);

  const handleToggleCalendar = useCallback((calendarId: string) => {
    setCalendars((prev) =>
      prev.map((c) =>
        c.id === calendarId ? { ...c, isVisible: !c.isVisible } : c,
      ),
    );
  }, []);

  const handlePrev = useCallback(() => {
    setCurrentDate((prev) => {
      const d = new Date(prev);
      if (view === "day") {
        d.setDate(d.getDate() - 1);
      } else if (view === "week") {
        d.setDate(d.getDate() - 7);
      } else {
        d.setMonth(d.getMonth() - 1);
      }
      return d;
    });
  }, [view]);

  const handleNext = useCallback(() => {
    setCurrentDate((prev) => {
      const d = new Date(prev);
      if (view === "day") {
        d.setDate(d.getDate() + 1);
      } else if (view === "week") {
        d.setDate(d.getDate() + 7);
      } else {
        d.setMonth(d.getMonth() + 1);
      }
      return d;
    });
  }, [view]);

  const handleToday = useCallback(() => {
    setCurrentDate(new Date());
  }, []);

  const handleSelectDate = useCallback((date: Date) => {
    setCurrentDate(date);
    setView("day");
  }, []);

  const getHeaderTitle = useCallback(() => {
    if (view === "day") {
      return currentDate.toLocaleDateString("ko-KR", {
        year: "numeric",
        month: "long",
        day: "numeric",
        weekday: "long",
      });
    } else if (view === "week") {
      const start = new Date(currentDate);
      start.setDate(currentDate.getDate() - currentDate.getDay());
      const end = new Date(start);
      end.setDate(start.getDate() + 6);

      if (start.getMonth() === end.getMonth()) {
        return `${start.toLocaleDateString("ko-KR", { year: "numeric", month: "long" })} ${start.getDate()} - ${end.getDate()}일`;
      }
      return `${start.toLocaleDateString("ko-KR", { month: "short", day: "numeric" })} - ${end.toLocaleDateString("ko-KR", { month: "short", day: "numeric" })}`;
    }
    return currentDate.toLocaleDateString("ko-KR", {
      year: "numeric",
      month: "long",
    });
  }, [currentDate, view]);

  return (
    <>
      {/* Calendar Sidebar - Fixed width */}
      <CalendarSidebar
        currentDate={currentDate}
        calendars={calendars}
        events={visibleEvents}
        onDateChange={setCurrentDate}
        onToggleCalendar={handleToggleCalendar}
        onCreateEvent={() => console.log("Create event")}
      />

      {/* Main Calendar View */}
      <div className="flex-1 flex flex-col min-w-0 bg-bg-base">
        {/* Header */}
        <header className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-1">
              <button
                onClick={handlePrev}
                className="p-1.5 rounded-lg hover:bg-bg-hover transition-colors"
                aria-label="Previous"
              >
                <ChevronLeft size={20} className="text-text-secondary" />
              </button>
              <button
                onClick={handleNext}
                className="p-1.5 rounded-lg hover:bg-bg-hover transition-colors"
                aria-label="Next"
              >
                <ChevronRight size={20} className="text-text-secondary" />
              </button>
            </div>
            <button
              onClick={handleToday}
              className="px-3 py-1.5 text-sm font-medium text-text-secondary hover:bg-bg-hover rounded-lg transition-colors border border-border-default"
            >
              Today
            </button>
            <h1 className="text-lg font-semibold text-text-primary">
              {getHeaderTitle()}
            </h1>
          </div>

          {/* View Switcher */}
          <div className="flex items-center gap-1 bg-bg-surface rounded-lg p-1">
            {(["day", "week", "month"] as CalendarView[]).map((v) => (
              <button
                key={v}
                onClick={() => setView(v)}
                className={cn(
                  "px-3 py-1.5 text-sm font-medium rounded-md transition-colors capitalize",
                  view === v
                    ? "bg-bg-base text-text-primary shadow-sm"
                    : "text-text-secondary hover:text-text-primary",
                )}
              >
                {v}
              </button>
            ))}
          </div>
        </header>

        {/* Calendar Content */}
        {view === "day" && (
          <CalendarDayView
            date={currentDate}
            events={visibleEvents}
            selectedEventId={selectedEvent?.id}
            onSelectEvent={setSelectedEvent}
          />
        )}
        {view === "week" && (
          <CalendarWeekView
            date={currentDate}
            events={visibleEvents}
            selectedEventId={selectedEvent?.id}
            onSelectEvent={setSelectedEvent}
            onSelectDate={handleSelectDate}
          />
        )}
        {view === "month" && (
          <div className="flex-1 flex items-center justify-center bg-bg-base">
            <div className="text-center">
              <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mx-auto mb-4">
                <Calendar
                  size={28}
                  strokeWidth={1}
                  className="text-text-disabled"
                />
              </div>
              <p className="text-lg font-medium text-text-secondary mb-1">
                Month view
              </p>
              <p className="text-sm text-text-tertiary">Coming soon</p>
            </div>
          </div>
        )}
      </div>

      {/* Event Detail Panel */}
      {selectedEvent && (
        <div className="animate-slide-in-right">
          <EventDetail
            event={selectedEvent}
            onClose={() => setSelectedEvent(null)}
            onEdit={() => console.log("Edit event", selectedEvent.id)}
            onDelete={() => {
              console.log("Delete event", selectedEvent.id);
              setSelectedEvent(null);
            }}
          />
        </div>
      )}
    </>
  );
}
