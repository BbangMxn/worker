"use client";

import { memo } from "react";
import { Inbox } from "lucide-react";
import { EmailCard, type Email } from "@/entities/email";

interface EmailListProps {
  emails: Email[];
  selectedId?: number;
  onSelect: (email: Email) => void;
  onStar: (email: Email) => void;
  isLoading?: boolean;
}

// Skeleton component for loading state
function EmailSkeleton() {
  return (
    <div className="flex gap-3 px-4 py-3 border-b border-border-subtle animate-pulse">
      {/* Avatar skeleton */}
      <div className="w-10 h-10 rounded-full bg-bg-surface shrink-0" />

      {/* Content skeleton */}
      <div className="flex-1 min-w-0 space-y-2">
        <div className="flex justify-between gap-4">
          <div className="h-4 bg-bg-surface rounded w-32" />
          <div className="h-3 bg-bg-surface rounded w-12" />
        </div>
        <div className="h-4 bg-bg-surface rounded w-48" />
        <div className="h-3 bg-bg-surface rounded w-full" />
      </div>
    </div>
  );
}

export const EmailList = memo(function EmailList({
  emails,
  selectedId,
  onSelect,
  onStar,
  isLoading = false,
}: EmailListProps) {
  // Loading state
  if (isLoading) {
    return (
      <div
        className="flex-1 overflow-hidden"
        role="status"
        aria-label="Loading emails"
      >
        {Array.from({ length: 8 }).map((_, i) => (
          <EmailSkeleton key={i} />
        ))}
      </div>
    );
  }

  // Empty state
  if (emails.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-text-tertiary p-8">
        <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mb-4">
          <Inbox size={28} strokeWidth={1} className="opacity-50" />
        </div>
        <p className="text-lg font-medium text-text-secondary mb-1">
          No emails
        </p>
        <p className="text-sm text-center">
          Your inbox is empty. New messages will appear here.
        </p>
      </div>
    );
  }

  return (
    <div
      className="flex-1 overflow-y-auto"
      role="listbox"
      aria-label="Email list"
    >
      {emails.map((email) => (
        <EmailCard
          key={email.id}
          email={email}
          selected={email.id === selectedId}
          onClick={() => onSelect(email)}
          onStar={() => onStar(email)}
        />
      ))}
    </div>
  );
});
