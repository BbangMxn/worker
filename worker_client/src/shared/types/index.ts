import type { Email as EmailType } from "@/entities/email";

// Re-export from entities (single source of truth)
export type {
  Email,
  EmailFolder,
  EmailAttachment,
  EmailThread,
  EmailFilter,
  EmailLabel,
  EmailDisplayData,
  WorkflowStatus,
  Category,
  Priority,
  Tag,
  AIStatus,
} from "@/entities/email";

export type { Contact, ContactGroup } from "@/entities/contact";
export type { CalendarEvent, Calendar, Attendee } from "@/entities/calendar";
export type { Document, DocumentFolder } from "@/entities/document";

// API-specific types
export type {
  User,
  OAuthProvider,
  OAuthConnection,
  Company,
  ApiContact,
  EmailAccount,
  Macro,
  Template,
  SearchResult,
} from "./worker_api_types";

// UI-level simple types
export interface Label {
  id: string;
  name: string;
  color: string;
}

export interface Folder {
  id: string;
  name: string;
  icon: string;
  count?: number;
  shortcut?: string;
}

export interface Thread {
  id: string;
  emails: EmailType[];
  subject: string;
  participants: { name: string; email: string }[];
  lastActivity: string;
  unreadCount: number;
}

export interface Command {
  id: string;
  name: string;
  shortcut: string;
  icon?: string;
  description?: string;
  category: string;
  action: () => void;
}
