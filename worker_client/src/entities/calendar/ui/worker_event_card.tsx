"use client";

import { memo } from "react";
import { Clock, MapPin, Users, MoreHorizontal } from "lucide-react";
import { cn } from "@/shared/lib";
import type { CalendarEvent, EventColor } from "../model/worker_calendar_types";

interface EventCardProps {
  event: CalendarEvent;
  selected?: boolean;
  onClick: () => void;
  compact?: boolean;
}

const colorStyles: Record<EventColor, { bg: string; border: string; text: string }> = {
  blue: { bg: "bg-blue-50", border: "border-blue-400", text: "text-blue-700" },
  green: { bg: "bg-green-50", border: "border-green-400", text: "text-green-700" },
  red: { bg: "bg-red-50", border: "border-red-400", text: "text-red-700" },
  yellow: { bg: "bg-yellow-50", border: "border-yellow-500", text: "text-yellow-700" },
  purple: { bg: "bg-purple-50", border: "border-purple-400", text: "text-purple-700" },
  orange: { bg: "bg-orange-50", border: "border-orange-400", text: "text-orange-700" },
  pink: { bg: "bg-pink-50", border: "border-pink-400", text: "text-pink-700" },
  cyan: { bg: "bg-cyan-50", border: "border-cyan-400", text: "text-cyan-700" },
};

function formatTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleTimeString("ko-KR", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function formatTimeRange(start: string, end: string): string {
  return `${formatTime(start)} - ${formatTime(end)}`;
}

export const EventCard = memo(function EventCard({
  event,
  selected = false,
  onClick,
  compact = false,
}: EventCardProps) {
  const styles = colorStyles[event.color];

  if (compact) {
    return (
      <button
        onClick={onClick}
        className={cn(
          "w-full text-left px-2 py-1 rounded text-xs truncate",
          "border-l-2 transition-colors",
          styles.bg,
          styles.border,
          styles.text,
          selected && "ring-2 ring-accent-primary"
        )}
      >
        <span className="font-medium">{event.title}</span>
        {!event.allDay && (
          <span className="ml-1 opacity-70">{formatTime(event.startTime)}</span>
        )}
      </button>
    );
  }

  return (
    <div
      role="button"
      onClick={onClick}
      className={cn(
        "group relative p-3 rounded-lg border-l-4 cursor-pointer",
        "transition-all duration-fast",
        styles.bg,
        styles.border,
        selected
          ? "ring-2 ring-accent-primary shadow-sm"
          : "hover:shadow-sm"
      )}
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <h3 className={cn("font-medium truncate", styles.text)}>
            {event.title}
          </h3>
          {!event.allDay && (
            <div className="flex items-center gap-1.5 mt-1 text-sm text-text-secondary">
              <Clock size={14} className="shrink-0" />
              <span>{formatTimeRange(event.startTime, event.endTime)}</span>
            </div>
          )}
          {event.allDay && (
            <span className="text-sm text-text-secondary">All day</span>
          )}
        </div>
        <button
          onClick={(e) => e.stopPropagation()}
          className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-black/5 transition-all"
          aria-label="More options"
        >
          <MoreHorizontal size={16} className="text-text-tertiary" />
        </button>
      </div>

      {/* Location */}
      {event.location && (
        <div className="flex items-center gap-1.5 mt-2 text-sm text-text-secondary">
          <MapPin size={14} className="shrink-0" />
          <span className="truncate">{event.location}</span>
        </div>
      )}

      {/* Attendees */}
      {event.attendees && event.attendees.length > 0 && (
        <div className="flex items-center gap-1.5 mt-2 text-sm text-text-secondary">
          <Users size={14} className="shrink-0" />
          <span className="truncate">
            {event.attendees.length} attendee{event.attendees.length > 1 ? "s" : ""}
          </span>
        </div>
      )}
    </div>
  );
});
