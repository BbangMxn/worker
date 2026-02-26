"use client";

import { memo } from "react";
import {
  ArrowLeft,
  Star,
  Reply,
  Forward,
  MoreHorizontal,
  Trash2,
  Archive,
  Paperclip,
  Download,
  Clock,
  CheckCircle2,
  AlertCircle,
  Brain,
} from "lucide-react";
import { cn, formatFileSize } from "@/shared/lib";
import { Avatar, IconButton } from "@/shared/ui";
import type { Email, Priority, Category } from "@/entities/email";

interface EmailDetailProps {
  email: Email;
  onBack: () => void;
  onStar: () => void;
  onReply: () => void;
  onForward: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onMarkTodo?: () => void;
  onMarkDone?: () => void;
}

const PRIORITY_LABELS: Record<Priority, { label: string; className: string }> = {
  urgent: { label: "Urgent", className: "bg-red-100 text-red-700" },
  high: { label: "High Priority", className: "bg-orange-100 text-orange-700" },
  normal: { label: "Normal", className: "bg-gray-100 text-gray-600" },
  low: { label: "Low Priority", className: "bg-gray-100 text-gray-500" },
};

const CATEGORY_LABELS: Record<Category, { label: string; className: string }> = {
  work: { label: "Work", className: "bg-blue-100 text-blue-700" },
  personal: { label: "Personal", className: "bg-green-100 text-green-700" },
  promo: { label: "Promo", className: "bg-purple-100 text-purple-700" },
  news: { label: "News", className: "bg-cyan-100 text-cyan-700" },
};

export const EmailDetail = memo(function EmailDetail({
  email,
  onBack,
  onStar,
  onReply,
  onForward,
  onArchive,
  onDelete,
  onMarkTodo,
  onMarkDone,
}: EmailDetailProps) {
  const isStarred = email.tags.includes("starred");
  const hasAttachments =
    email.hasAttachments && email.attachments && email.attachments.length > 0;
  const senderName = email.fromName || email.fromEmail.split("@")[0];

  return (
    <div className="flex flex-col h-full bg-bg-base">
      {/* Header - Fixed */}
      <header className="flex items-center justify-between px-4 h-14 border-b border-border-subtle shrink-0">
        <div className="flex items-center gap-2">
          <IconButton onClick={onBack} aria-label="Back to list">
            <ArrowLeft size={20} strokeWidth={1.5} />
          </IconButton>
        </div>

        <div className="flex items-center gap-1">
          {email.workflowStatus !== "done" && onMarkDone && (
            <IconButton onClick={onMarkDone} aria-label="Mark as done">
              <CheckCircle2 size={20} strokeWidth={1.5} />
            </IconButton>
          )}
          {email.workflowStatus === "inbox" && onMarkTodo && (
            <IconButton onClick={onMarkTodo} aria-label="Add to todo">
              <Clock size={20} strokeWidth={1.5} />
            </IconButton>
          )}
          <IconButton onClick={onArchive} aria-label="Archive">
            <Archive size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onDelete} aria-label="Delete">
            <Trash2 size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton
            onClick={onStar}
            active={isStarred}
            aria-label={isStarred ? "Unstar" : "Star"}
            aria-pressed={isStarred}
          >
            <Star
              size={20}
              strokeWidth={1.5}
              fill={isStarred ? "currentColor" : "none"}
            />
          </IconButton>
          <IconButton aria-label="More options">
            <MoreHorizontal size={20} strokeWidth={1.5} />
          </IconButton>
        </div>
      </header>

      {/* Content - Scrollable */}
      <div className="flex-1 overflow-y-auto">
        <article className="max-w-3xl mx-auto p-6">
          {/* Subject */}
          <h1 className="text-2xl font-semibold text-text-primary mb-4 leading-tight">
            {email.subject}
          </h1>

          {/* AI Tags */}
          {(email.aiCategory || email.aiPriority) && (
            <div className="flex flex-wrap gap-2 mb-4">
              {email.aiPriority && email.aiPriority !== "normal" && (
                <span
                  className={cn(
                    "inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium",
                    PRIORITY_LABELS[email.aiPriority].className,
                  )}
                >
                  <AlertCircle size={12} />
                  {PRIORITY_LABELS[email.aiPriority].label}
                </span>
              )}
              {email.aiCategory && (
                <span
                  className={cn(
                    "inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium",
                    CATEGORY_LABELS[email.aiCategory].className,
                  )}
                >
                  {CATEGORY_LABELS[email.aiCategory].label}
                </span>
              )}
              {email.workflowStatus !== "inbox" && (
                <span
                  className={cn(
                    "inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium",
                    email.workflowStatus === "done" &&
                      "bg-green-100 text-green-700",
                    email.workflowStatus === "todo" &&
                      "bg-blue-100 text-blue-700",
                    email.workflowStatus === "snoozed" &&
                      "bg-orange-100 text-orange-700",
                  )}
                >
                  {email.workflowStatus === "done" && (
                    <CheckCircle2 size={12} />
                  )}
                  {email.workflowStatus === "todo" && <Clock size={12} />}
                  {email.workflowStatus === "snoozed" && <Clock size={12} />}
                  {email.workflowStatus.charAt(0).toUpperCase() +
                    email.workflowStatus.slice(1)}
                </span>
              )}
            </div>
          )}

          {/* AI Summary */}
          {email.aiSummary && (
            <div className="mb-6 p-4 rounded-lg bg-bg-surface border border-border-subtle">
              <div className="flex items-center gap-2 mb-2 text-sm font-medium text-text-secondary">
                <Brain size={16} className="text-accent-primary" />
                AI Summary
              </div>
              <p className="text-sm text-text-primary">{email.aiSummary}</p>
            </div>
          )}

          {/* Sender Info */}
          <div className="flex items-start gap-4 mb-8">
            <Avatar name={senderName} size="lg" className="shrink-0" />

            <div className="flex-1 min-w-0">
              <div className="flex items-center justify-between gap-4 flex-wrap">
                <div className="min-w-0">
                  <span className="font-semibold text-text-primary">
                    {senderName}
                  </span>
                  <span className="text-text-tertiary text-sm ml-2">
                    &lt;{email.fromEmail}&gt;
                  </span>
                </div>
                <time
                  dateTime={email.receivedAt}
                  className="text-sm text-text-tertiary shrink-0"
                >
                  {new Date(email.receivedAt).toLocaleDateString("ko-KR", {
                    month: "short",
                    day: "numeric",
                    year: "numeric",
                    hour: "numeric",
                    minute: "2-digit",
                  })}
                </time>
              </div>

              <div className="text-sm text-text-tertiary mt-1">
                to {email.toEmails.join(", ")}
                {email.ccEmails && email.ccEmails.length > 0 && (
                  <span className="ml-2">cc: {email.ccEmails.join(", ")}</span>
                )}
              </div>
            </div>
          </div>

          {/* Body */}
          <div className="text-text-primary whitespace-pre-wrap leading-relaxed text-base">
            {email.body || email.snippet}
          </div>

          {/* Attachments */}
          {hasAttachments && email.attachments && (
            <section className="mt-8 pt-6 border-t border-border-subtle">
              <h2 className="text-sm font-medium text-text-secondary mb-3 flex items-center gap-2">
                <Paperclip size={16} strokeWidth={1.5} />
                {email.attachments.length} Attachment
                {email.attachments.length > 1 ? "s" : ""}
              </h2>

              <div className="grid gap-2">
                {email.attachments.map((attachment) => (
                  <a
                    key={attachment.id}
                    href={attachment.url || "#"}
                    download
                    className={cn(
                      "flex items-center gap-3 p-3 rounded-lg",
                      "bg-bg-surface hover:bg-bg-hover",
                      "transition-colors duration-fast group",
                    )}
                  >
                    <div className="w-10 h-10 rounded-lg bg-bg-hover flex items-center justify-center shrink-0">
                      <Paperclip
                        size={18}
                        strokeWidth={1.5}
                        className="text-text-tertiary"
                      />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-text-primary truncate group-hover:text-accent-primary transition-colors">
                        {attachment.name}
                      </div>
                      <div className="text-xs text-text-tertiary">
                        {formatFileSize(attachment.size)}
                      </div>
                    </div>
                    <Download
                      size={16}
                      strokeWidth={1.5}
                      className="text-text-tertiary opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
                    />
                  </a>
                ))}
              </div>
            </section>
          )}
        </article>
      </div>

      {/* Footer Actions - Fixed */}
      <footer className="flex items-center gap-2 px-4 py-3 border-t border-border-subtle shrink-0">
        <button
          onClick={onReply}
          className={cn(
            "flex items-center gap-2 px-4 py-2 rounded-md",
            "bg-accent-primary hover:bg-accent-hover text-white",
            "transition-colors duration-fast",
            "text-sm font-medium",
          )}
        >
          <Reply size={16} strokeWidth={1.5} />
          Reply
        </button>
        <button
          onClick={onForward}
          className={cn(
            "flex items-center gap-2 px-4 py-2 rounded-md",
            "bg-bg-surface hover:bg-bg-hover",
            "transition-colors duration-fast",
            "text-sm font-medium text-text-primary",
          )}
        >
          <Forward size={16} strokeWidth={1.5} />
          Forward
        </button>
      </footer>
    </div>
  );
});
