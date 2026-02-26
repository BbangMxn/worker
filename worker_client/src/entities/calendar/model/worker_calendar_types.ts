export type EventColor =
  | "blue"
  | "green"
  | "red"
  | "yellow"
  | "purple"
  | "orange"
  | "pink"
  | "cyan";

export interface CalendarEvent {
  id: string;
  title: string;
  description?: string;
  startTime: string;
  endTime: string;
  allDay: boolean;
  location?: string;
  color: EventColor;
  calendarId: string;
  attendees?: Attendee[];
  reminders?: Reminder[];
  recurring?: RecurringRule;
  createdAt: string;
  updatedAt: string;
}

export interface Attendee {
  email: string;
  name?: string;
  status: "pending" | "accepted" | "declined" | "tentative";
}

export interface Reminder {
  type: "email" | "notification";
  minutesBefore: number;
}

export interface RecurringRule {
  frequency: "daily" | "weekly" | "monthly" | "yearly";
  interval: number;
  endDate?: string;
  count?: number;
}

export interface Calendar {
  id: string;
  name: string;
  color: EventColor;
  isDefault: boolean;
  isVisible: boolean;
}

export type CalendarView = "day" | "week" | "month";
