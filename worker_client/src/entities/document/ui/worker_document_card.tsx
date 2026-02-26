"use client";

import { memo } from "react";
import {
  FileText,
  Table,
  Presentation,
  FileImage,
  File,
  Star,
  Users,
  MoreHorizontal,
} from "lucide-react";
import { cn } from "@/shared/lib";
import type { Document, DocumentType } from "../model/worker_document_types";

interface DocumentCardProps {
  document: Document;
  selected?: boolean;
  onClick: () => void;
  onStar: () => void;
  view?: "list" | "grid";
}

const typeIcons: Record<DocumentType, typeof FileText> = {
  doc: FileText,
  sheet: Table,
  slide: Presentation,
  pdf: File,
  image: FileImage,
  other: File,
};

const typeColors: Record<DocumentType, string> = {
  doc: "text-blue-500",
  sheet: "text-green-500",
  slide: "text-orange-500",
  pdf: "text-red-500",
  image: "text-purple-500",
  other: "text-text-tertiary",
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return "Today";
  if (diffDays === 1) return "Yesterday";
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString("ko-KR", {
    month: "short",
    day: "numeric",
  });
}

export const DocumentCard = memo(function DocumentCard({
  document,
  selected = false,
  onClick,
  onStar,
  view = "list",
}: DocumentCardProps) {
  const Icon = typeIcons[document.type];

  if (view === "grid") {
    return (
      <div
        role="option"
        aria-selected={selected}
        onClick={onClick}
        className={cn(
          "group relative flex flex-col rounded-xl border cursor-pointer",
          "transition-all duration-fast",
          selected
            ? "border-accent-primary bg-accent-primary/5 shadow-sm"
            : "border-border-default bg-bg-base hover:border-border-strong hover:shadow-sm"
        )}
      >
        {/* Thumbnail */}
        <div className="h-32 bg-bg-surface rounded-t-xl flex items-center justify-center border-b border-border-subtle">
          <Icon size={40} className={cn("opacity-60", typeColors[document.type])} />
        </div>

        {/* Content */}
        <div className="p-3">
          <div className="flex items-start gap-2">
            <div className="flex-1 min-w-0">
              <h3 className="font-medium text-text-primary truncate text-sm">
                {document.title}
              </h3>
              <p className="text-xs text-text-tertiary mt-1">
                {formatDate(document.updatedAt)} · {formatSize(document.size)}
              </p>
            </div>
            <button
              onClick={(e) => {
                e.stopPropagation();
                onStar();
              }}
              className={cn(
                "p-1 rounded transition-colors shrink-0",
                document.starred
                  ? "text-yellow-500"
                  : "text-text-disabled hover:text-text-tertiary opacity-0 group-hover:opacity-100"
              )}
              aria-label={document.starred ? "Remove from starred" : "Add to starred"}
            >
              <Star size={14} fill={document.starred ? "currentColor" : "none"} />
            </button>
          </div>

          {/* Shared indicator */}
          {document.shared && (
            <div className="flex items-center gap-1 mt-2 text-xs text-text-tertiary">
              <Users size={12} />
              <span>Shared</span>
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div
      role="option"
      aria-selected={selected}
      onClick={onClick}
      className={cn(
        "group flex items-center gap-4 px-4 py-3 cursor-pointer",
        "border-b border-border-subtle",
        "transition-colors duration-fast",
        selected
          ? "bg-accent-primary/8"
          : "hover:bg-bg-hover"
      )}
    >
      {/* Icon */}
      <div
        className={cn(
          "w-10 h-10 rounded-lg flex items-center justify-center shrink-0",
          "bg-bg-surface"
        )}
      >
        <Icon size={20} className={typeColors[document.type]} />
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <h3 className="font-medium text-text-primary truncate">
            {document.title}
          </h3>
          {document.starred && (
            <Star size={14} className="text-yellow-500 shrink-0" fill="currentColor" />
          )}
        </div>
        <div className="flex items-center gap-2 mt-0.5">
          <span className="text-sm text-text-tertiary">
            {formatDate(document.updatedAt)}
          </span>
          <span className="text-text-disabled">·</span>
          <span className="text-sm text-text-tertiary">
            {formatSize(document.size)}
          </span>
          {document.shared && (
            <>
              <span className="text-text-disabled">·</span>
              <span className="flex items-center gap-1 text-sm text-text-tertiary">
                <Users size={12} />
                Shared
              </span>
            </>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onStar();
          }}
          className={cn(
            "p-1.5 rounded-md transition-colors",
            document.starred
              ? "text-yellow-500"
              : "text-text-tertiary hover:text-text-secondary hover:bg-bg-surface"
          )}
          aria-label={document.starred ? "Remove from starred" : "Add to starred"}
        >
          <Star size={16} fill={document.starred ? "currentColor" : "none"} />
        </button>
        <button
          onClick={(e) => e.stopPropagation()}
          className="p-1.5 rounded-md text-text-tertiary hover:text-text-secondary hover:bg-bg-surface transition-colors"
          aria-label="More options"
        >
          <MoreHorizontal size={16} />
        </button>
      </div>
    </div>
  );
});
