"use client";

import { memo, useCallback } from "react";
import {
  Star,
  Paperclip,
  AlertCircle,
  Clock,
  CheckCircle2,
} from "lucide-react";
import { cn, formatDate, truncate } from "@/shared/lib";
import { Avatar } from "@/shared/ui";
import type { Email, Priority, WorkflowStatus } from "../model/worker_email_types";

interface EmailCardProps {
  email: Email;
  selected?: boolean;
  onClick?: () => void;
  onStar?: () => void;
}

const PRIORITY_CONFIG: Record<
  Priority,
  { icon: typeof AlertCircle; className: string }
> = {
  urgent: { icon: AlertCircle, className: "text-red-500" },
  high: { icon: AlertCircle, className: "text-orange-500" },
  normal: { icon: AlertCircle, className: "text-text-tertiary" },
  low: { icon: AlertCircle, className: "text-text-disabled" },
};

const WORKFLOW_ICONS: Record<WorkflowStatus, typeof Clock | null> = {
  inbox: null,
  todo: Clock,
  done: CheckCircle2,
  snoozed: Clock,
};

export const EmailCard = memo(function EmailCard({
  email,
  selected,
  onClick,
  onStar,
}: EmailCardProps) {
  const handleStarClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onStar?.();
    },
    [onStar],
  );

  const isStarred = email.tags.includes("starred");
  const isImportant = email.tags.includes("important");
  const hasLabels = email.labels.length > 0;
  const senderName = email.fromName || email.fromEmail.split("@")[0];

  const WorkflowIcon = WORKFLOW_ICONS[email.workflowStatus];

  return (
    <article
      onClick={onClick}
      role="option"
      aria-selected={selected}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick?.();
        }
      }}
      className={cn(
        "flex gap-3 px-4 py-3 cursor-pointer",
        "border-b border-border-subtle",
        "transition-colors duration-fast",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-primary",
        // States
        selected
          ? "bg-bg-active"
          : !email.isRead
            ? "bg-bg-surface hover:bg-bg-hover"
            : "hover:bg-bg-hover",
      )}
    >
      {/* Avatar - Instagram style circular */}
      <Avatar name={senderName} size="md" className="shrink-0" />

      {/* Content - Uber style hierarchy */}
      <div className="flex-1 min-w-0">
        {/* Header: Sender + Time */}
        <div className="flex items-center justify-between gap-2 mb-0.5">
          <div className="flex items-center gap-2 min-w-0">
            <span
              className={cn(
                "text-sm truncate",
                !email.isRead
                  ? "font-semibold text-text-primary"
                  : "text-text-secondary",
              )}
            >
              {senderName}
            </span>
            {email.aiPriority && email.aiPriority !== "normal" && (
              <span
                className={cn(
                  "text-xs font-medium px-1.5 py-0.5 rounded",
                  email.aiPriority === "urgent" && "bg-red-100 text-red-700",
                  email.aiPriority === "high" &&
                    "bg-orange-100 text-orange-700",
                  email.aiPriority === "low" && "bg-gray-100 text-gray-600",
                )}
              >
                {email.aiPriority === "urgent"
                  ? "Urgent"
                  : email.aiPriority === "high"
                    ? "High"
                    : "Low"}
              </span>
            )}
          </div>
          <time
            dateTime={email.receivedAt}
            className="text-xs text-text-tertiary shrink-0 tabular-nums"
          >
            {formatDate(email.receivedAt)}
          </time>
        </div>

        {/* Subject */}
        <h3
          className={cn(
            "text-sm truncate mb-0.5",
            !email.isRead
              ? "font-medium text-text-primary"
              : "text-text-secondary",
          )}
        >
          {email.subject}
        </h3>

        {/* Preview / AI Summary */}
        <p className="text-sm text-text-tertiary truncate">
          {truncate(email.aiSummary || email.snippet, 80)}
        </p>

        {/* Labels, Attachments & Workflow Status */}
        {(hasLabels || email.hasAttachments || WorkflowIcon) && (
          <div className="flex items-center gap-2 mt-2">
            {email.labels.map((label) => (
              <span
                key={label}
                className="px-2 py-0.5 text-xs rounded-full bg-bg-surface text-text-secondary"
              >
                {label}
              </span>
            ))}
            {email.hasAttachments && (
              <span className="flex items-center gap-1 text-xs text-text-tertiary">
                <Paperclip size={12} />
                {email.attachments?.length || 1}
              </span>
            )}
            {WorkflowIcon && email.workflowStatus !== "inbox" && (
              <span
                className={cn(
                  "flex items-center gap-1 text-xs",
                  email.workflowStatus === "done" && "text-green-600",
                  email.workflowStatus === "todo" && "text-blue-600",
                  email.workflowStatus === "snoozed" && "text-orange-500",
                )}
              >
                <WorkflowIcon size={12} />
                {email.workflowStatus === "done" && "Done"}
                {email.workflowStatus === "todo" && "Todo"}
                {email.workflowStatus === "snoozed" && "Snoozed"}
              </span>
            )}
          </div>
        )}
      </div>

      {/* Right: Star + Unread indicator */}
      <div className="flex flex-col items-center gap-1.5 shrink-0">
        <button
          onClick={handleStarClick}
          aria-label={isStarred ? "Unstar email" : "Star email"}
          aria-pressed={isStarred}
          className={cn(
            "p-1 rounded-md transition-colors duration-fast",
            isStarred
              ? "text-semantic-warning"
              : "text-text-disabled hover:text-text-tertiary",
          )}
        >
          <Star
            size={16}
            strokeWidth={1.5}
            fill={isStarred ? "currentColor" : "none"}
          />
        </button>

        {/* Important indicator */}
        {isImportant && (
          <div
            className="w-1.5 h-1.5 rounded-full bg-orange-500"
            aria-label="Important"
          />
        )}

        {/* Unread dot */}
        {!email.isRead && (
          <div
            className="w-2 h-2 rounded-full bg-accent-primary"
            aria-label="Unread"
          />
        )}
      </div>
    </article>
  );
});
