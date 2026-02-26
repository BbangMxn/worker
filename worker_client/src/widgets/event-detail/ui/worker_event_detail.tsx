"use client";

import { memo } from "react";
import {
  X,
  Edit,
  Trash2,
  Clock,
  MapPin,
  Users,
  Bell,
  Calendar,
  Repeat,
  MoreHorizontal,
} from "lucide-react";
import { cn } from "@/shared/lib";
import { IconButton } from "@/shared/ui";
import type { CalendarEvent, EventColor } from "@/entities/calendar";

interface EventDetailProps {
  event: CalendarEvent;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

const colorStyles: Record<EventColor, string> = {
  blue: "bg-blue-500",
  green: "bg-green-500",
  red: "bg-red-500",
  yellow: "bg-yellow-500",
  purple: "bg-purple-500",
  orange: "bg-orange-500",
  pink: "bg-pink-500",
  cyan: "bg-cyan-500",
};

function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString("ko-KR", {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatTimeRange(start: string, end: string, allDay: boolean): string {
  if (allDay) {
    const startDate = new Date(start);
    return startDate.toLocaleDateString("ko-KR", {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "short",
    }) + " (All day)";
  }

  const startDate = new Date(start);
  const endDate = new Date(end);

  const startStr = startDate.toLocaleDateString("ko-KR", {
    month: "short",
    day: "numeric",
    weekday: "short",
  });

  const startTime = startDate.toLocaleTimeString("ko-KR", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });

  const endTime = endDate.toLocaleTimeString("ko-KR", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });

  return `${startStr} ${startTime} - ${endTime}`;
}

function getAttendeeStatusColor(status: string): string {
  switch (status) {
    case "accepted":
      return "text-green-600";
    case "declined":
      return "text-red-600";
    case "tentative":
      return "text-yellow-600";
    default:
      return "text-text-tertiary";
  }
}

function getAttendeeStatusLabel(status: string): string {
  switch (status) {
    case "accepted":
      return "Accepted";
    case "declined":
      return "Declined";
    case "tentative":
      return "Maybe";
    default:
      return "Pending";
  }
}

export const EventDetail = memo(function EventDetail({
  event,
  onClose,
  onEdit,
  onDelete,
}: EventDetailProps) {
  return (
    <div className="w-[360px] flex flex-col border-l border-border-default bg-bg-base h-full">
      {/* Header */}
      <header className="flex items-center justify-between px-4 h-14 border-b border-border-subtle shrink-0">
        <div className="flex items-center gap-2">
          <div className={cn("w-3 h-3 rounded-full", colorStyles[event.color])} />
          <span className="text-sm font-medium text-text-secondary">Event Details</span>
        </div>
        <div className="flex items-center gap-1">
          <IconButton onClick={onEdit} aria-label="Edit event">
            <Edit size={18} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onDelete} aria-label="Delete event">
            <Trash2 size={18} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onClose} aria-label="Close">
            <X size={18} strokeWidth={1.5} />
          </IconButton>
        </div>
      </header>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {/* Title */}
        <h1 className="text-xl font-semibold text-text-primary mb-4">
          {event.title}
        </h1>

        {/* Time */}
        <div className="flex items-start gap-3 mb-4">
          <Clock size={18} className="text-text-tertiary mt-0.5 shrink-0" />
          <div>
            <p className="text-text-primary">
              {formatTimeRange(event.startTime, event.endTime, event.allDay)}
            </p>
            {event.recurring && (
              <div className="flex items-center gap-1.5 mt-1 text-sm text-text-secondary">
                <Repeat size={14} />
                <span>
                  Repeats {event.recurring.frequency}
                  {event.recurring.interval > 1 && ` every ${event.recurring.interval}`}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Location */}
        {event.location && (
          <div className="flex items-start gap-3 mb-4">
            <MapPin size={18} className="text-text-tertiary mt-0.5 shrink-0" />
            <p className="text-text-primary">{event.location}</p>
          </div>
        )}

        {/* Description */}
        {event.description && (
          <div className="mb-4 p-3 rounded-lg bg-bg-surface">
            <p className="text-text-primary whitespace-pre-wrap text-sm">
              {event.description}
            </p>
          </div>
        )}

        {/* Attendees */}
        {event.attendees && event.attendees.length > 0 && (
          <div className="mb-4">
            <div className="flex items-center gap-2 mb-2">
              <Users size={18} className="text-text-tertiary" />
              <span className="text-sm font-medium text-text-secondary">
                {event.attendees.length} Attendee{event.attendees.length > 1 ? "s" : ""}
              </span>
            </div>
            <div className="space-y-2 pl-7">
              {event.attendees.map((attendee, index) => (
                <div
                  key={index}
                  className="flex items-center justify-between py-1.5"
                >
                  <div>
                    <p className="text-sm text-text-primary">
                      {attendee.name || attendee.email}
                    </p>
                    {attendee.name && (
                      <p className="text-xs text-text-tertiary">{attendee.email}</p>
                    )}
                  </div>
                  <span
                    className={cn(
                      "text-xs font-medium",
                      getAttendeeStatusColor(attendee.status)
                    )}
                  >
                    {getAttendeeStatusLabel(attendee.status)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Reminders */}
        {event.reminders && event.reminders.length > 0 && (
          <div className="mb-4">
            <div className="flex items-center gap-2 mb-2">
              <Bell size={18} className="text-text-tertiary" />
              <span className="text-sm font-medium text-text-secondary">Reminders</span>
            </div>
            <div className="space-y-1 pl-7">
              {event.reminders.map((reminder, index) => (
                <p key={index} className="text-sm text-text-primary">
                  {reminder.minutesBefore < 60
                    ? `${reminder.minutesBefore} minutes before`
                    : `${reminder.minutesBefore / 60} hour${reminder.minutesBefore > 60 ? "s" : ""} before`}
                  {" "}({reminder.type})
                </p>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Footer Actions */}
      <div className="shrink-0 p-4 border-t border-border-subtle">
        <div className="flex gap-2">
          <button
            onClick={onEdit}
            className={cn(
              "flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg",
              "bg-accent-primary hover:bg-accent-hover text-white",
              "font-medium transition-colors"
            )}
          >
            <Edit size={16} />
            <span>Edit</span>
          </button>
          <button
            onClick={onDelete}
            className={cn(
              "px-4 py-2 rounded-lg",
              "bg-bg-surface hover:bg-red-50 text-red-600 border border-border-default",
              "font-medium transition-colors"
            )}
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>
    </div>
  );
});
