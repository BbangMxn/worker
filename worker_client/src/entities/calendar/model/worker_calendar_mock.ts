import type { CalendarEvent, Calendar } from "./worker_calendar_types";

// Get current date for realistic mock data
const today = new Date();
const year = today.getFullYear();
const month = today.getMonth();
const date = today.getDate();

function formatDateTime(d: Date): string {
  return d.toISOString();
}

function addHours(d: Date, hours: number): Date {
  return new Date(d.getTime() + hours * 60 * 60 * 1000);
}

function addDays(d: Date, days: number): Date {
  return new Date(d.getTime() + days * 24 * 60 * 60 * 1000);
}

export const mockCalendars: Calendar[] = [
  { id: "cal1", name: "업무", color: "blue", isDefault: true, isVisible: true },
  { id: "cal2", name: "개인", color: "green", isDefault: false, isVisible: true },
  { id: "cal3", name: "미팅", color: "purple", isDefault: false, isVisible: true },
  { id: "cal4", name: "휴일", color: "red", isDefault: false, isVisible: true },
];

const baseDate = new Date(year, month, date, 9, 0, 0);

export const mockEvents: CalendarEvent[] = [
  {
    id: "e1",
    title: "팀 스탠드업 미팅",
    description: "일일 진행상황 공유 및 이슈 논의",
    startTime: formatDateTime(baseDate),
    endTime: formatDateTime(addHours(baseDate, 0.5)),
    allDay: false,
    location: "회의실 A",
    color: "blue",
    calendarId: "cal1",
    attendees: [
      { email: "kim@company.com", name: "김철수", status: "accepted" },
      { email: "lee@company.com", name: "이영희", status: "accepted" },
      { email: "park@company.com", name: "박지민", status: "pending" },
    ],
    reminders: [{ type: "notification", minutesBefore: 10 }],
    createdAt: formatDateTime(addDays(baseDate, -7)),
    updatedAt: formatDateTime(addDays(baseDate, -7)),
  },
  {
    id: "e2",
    title: "프로젝트 리뷰",
    description: "Q1 프로젝트 진행 현황 리뷰 및 피드백",
    startTime: formatDateTime(addHours(baseDate, 2)),
    endTime: formatDateTime(addHours(baseDate, 3)),
    allDay: false,
    location: "대회의실",
    color: "purple",
    calendarId: "cal3",
    attendees: [
      { email: "manager@company.com", name: "팀장", status: "accepted" },
      { email: "dev@company.com", name: "개발팀", status: "accepted" },
    ],
    reminders: [
      { type: "email", minutesBefore: 60 },
      { type: "notification", minutesBefore: 15 },
    ],
    createdAt: formatDateTime(addDays(baseDate, -3)),
    updatedAt: formatDateTime(addDays(baseDate, -1)),
  },
  {
    id: "e3",
    title: "점심 약속",
    startTime: formatDateTime(addHours(baseDate, 3)),
    endTime: formatDateTime(addHours(baseDate, 4)),
    allDay: false,
    location: "회사 근처 식당",
    color: "green",
    calendarId: "cal2",
    createdAt: formatDateTime(addDays(baseDate, -2)),
    updatedAt: formatDateTime(addDays(baseDate, -2)),
  },
  {
    id: "e4",
    title: "클라이언트 미팅",
    description: "신규 프로젝트 제안 발표",
    startTime: formatDateTime(addHours(baseDate, 5)),
    endTime: formatDateTime(addHours(baseDate, 6.5)),
    allDay: false,
    location: "외부 - 클라이언트 오피스",
    color: "orange",
    calendarId: "cal3",
    attendees: [
      { email: "client@external.com", name: "클라이언트", status: "accepted" },
    ],
    reminders: [
      { type: "email", minutesBefore: 120 },
      { type: "notification", minutesBefore: 30 },
    ],
    createdAt: formatDateTime(addDays(baseDate, -5)),
    updatedAt: formatDateTime(addDays(baseDate, -1)),
  },
  {
    id: "e5",
    title: "운동",
    startTime: formatDateTime(addHours(baseDate, 9)),
    endTime: formatDateTime(addHours(baseDate, 10)),
    allDay: false,
    location: "헬스장",
    color: "cyan",
    calendarId: "cal2",
    recurring: {
      frequency: "weekly",
      interval: 1,
    },
    createdAt: formatDateTime(addDays(baseDate, -30)),
    updatedAt: formatDateTime(addDays(baseDate, -30)),
  },
  // Tomorrow
  {
    id: "e6",
    title: "코드 리뷰",
    description: "PR #245 리뷰",
    startTime: formatDateTime(addHours(addDays(baseDate, 1), 1)),
    endTime: formatDateTime(addHours(addDays(baseDate, 1), 2)),
    allDay: false,
    color: "blue",
    calendarId: "cal1",
    createdAt: formatDateTime(baseDate),
    updatedAt: formatDateTime(baseDate),
  },
  {
    id: "e7",
    title: "1:1 미팅",
    description: "팀장과 주간 1:1",
    startTime: formatDateTime(addHours(addDays(baseDate, 1), 3)),
    endTime: formatDateTime(addHours(addDays(baseDate, 1), 3.5)),
    allDay: false,
    location: "팀장실",
    color: "purple",
    calendarId: "cal3",
    createdAt: formatDateTime(addDays(baseDate, -7)),
    updatedAt: formatDateTime(addDays(baseDate, -7)),
  },
  // Day after tomorrow
  {
    id: "e8",
    title: "팀 회식",
    startTime: formatDateTime(addHours(addDays(baseDate, 2), 9)),
    endTime: formatDateTime(addHours(addDays(baseDate, 2), 12)),
    allDay: false,
    location: "강남역 근처",
    color: "pink",
    calendarId: "cal2",
    createdAt: formatDateTime(addDays(baseDate, -10)),
    updatedAt: formatDateTime(addDays(baseDate, -5)),
  },
  // All day event
  {
    id: "e9",
    title: "연차",
    startTime: formatDateTime(addDays(baseDate, 5)),
    endTime: formatDateTime(addDays(baseDate, 6)),
    allDay: true,
    color: "red",
    calendarId: "cal4",
    createdAt: formatDateTime(addDays(baseDate, -14)),
    updatedAt: formatDateTime(addDays(baseDate, -14)),
  },
];
