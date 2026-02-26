// Email types aligned with backend models
// See: src/shared/types/api.ts for full API types

export type EmailFolder = "inbox" | "sent" | "draft" | "trash" | "archive";
export type WorkflowStatus = "inbox" | "todo" | "done" | "snoozed";
export type Category = "work" | "personal" | "promo" | "news";
export type Priority = "urgent" | "high" | "normal" | "low";
export type Tag = "important" | "starred";
export type AIStatus = "pending" | "processing" | "completed" | "failed";

export interface EmailAttachment {
  id: string;
  name: string;
  mimeType: string;
  size: number;
  url?: string;
}

export interface Email {
  id: number;
  connectionId?: number;
  externalId?: string;
  threadId?: number;
  // Sender info
  fromEmail: string;
  fromName?: string;
  // Recipients
  toEmails: string[];
  ccEmails?: string[];
  bccEmails?: string[];
  // Content
  subject: string;
  snippet: string;
  body?: string;
  htmlBody?: string;
  // Status
  folder: EmailFolder;
  tags: Tag[];
  labels: string[];
  isRead: boolean;
  // Attachments
  hasAttachments: boolean;
  attachments?: EmailAttachment[];
  // Workflow
  workflowStatus: WorkflowStatus;
  snoozedUntil?: string;
  // AI Processing
  aiStatus: AIStatus;
  aiCategory?: Category;
  aiPriority?: Priority;
  aiSummary?: string;
  // Timestamps
  receivedAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface EmailThread {
  id: number;
  emails: Email[];
  subject: string;
  participants: string[]; // email addresses
  lastActivity: string;
  unreadCount: number;
}

export interface EmailFilter {
  folder?: EmailFolder;
  workflowStatus?: WorkflowStatus;
  category?: Category;
  priority?: Priority;
  tag?: Tag;
  isRead?: boolean;
  search?: string;
}

export interface EmailLabel {
  id: string;
  name: string;
  color: string;
}

// UI-specific derived types for display
export interface EmailDisplayData {
  id: number;
  from: {
    name: string;
    email: string;
  };
  subject: string;
  snippet: string;
  date: string;
  isRead: boolean;
  isStarred: boolean;
  isImportant: boolean;
  hasAttachments: boolean;
  category?: Category;
  priority?: Priority;
  workflowStatus: WorkflowStatus;
}

// Helper to convert Email to EmailDisplayData
export function toEmailDisplayData(email: Email): EmailDisplayData {
  return {
    id: email.id,
    from: {
      name: email.fromName || email.fromEmail.split("@")[0],
      email: email.fromEmail,
    },
    subject: email.subject,
    snippet: email.snippet,
    date: email.receivedAt,
    isRead: email.isRead,
    isStarred: email.tags.includes("starred"),
    isImportant: email.tags.includes("important"),
    hasAttachments: email.hasAttachments,
    category: email.aiCategory,
    priority: email.aiPriority,
    workflowStatus: email.workflowStatus,
  };
}
