"use client";

import { memo } from "react";
import {
  ArrowLeft,
  Star,
  Download,
  Share2,
  Edit,
  Trash2,
  MoreHorizontal,
  FileText,
  Table,
  Presentation,
  FileImage,
  File,
  Clock,
  User,
  Users,
  FolderOpen,
} from "lucide-react";
import { cn } from "@/shared/lib";
import { IconButton } from "@/shared/ui";
import type { Document, DocumentType } from "@/entities/document";

interface DocumentDetailProps {
  document: Document;
  onBack: () => void;
  onStar: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onDownload: () => void;
  onShare: () => void;
}

const typeIcons: Record<DocumentType, typeof FileText> = {
  doc: FileText,
  sheet: Table,
  slide: Presentation,
  pdf: File,
  image: FileImage,
  other: File,
};

const typeLabels: Record<DocumentType, string> = {
  doc: "Document",
  sheet: "Spreadsheet",
  slide: "Presentation",
  pdf: "PDF",
  image: "Image",
  other: "File",
};

const typeColors: Record<DocumentType, string> = {
  doc: "text-blue-500 bg-blue-50",
  sheet: "text-green-500 bg-green-50",
  slide: "text-orange-500 bg-orange-50",
  pdf: "text-red-500 bg-red-50",
  image: "text-purple-500 bg-purple-50",
  other: "text-text-tertiary bg-bg-surface",
};

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("ko-KR", {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export const DocumentDetail = memo(function DocumentDetail({
  document,
  onBack,
  onStar,
  onEdit,
  onDelete,
  onDownload,
  onShare,
}: DocumentDetailProps) {
  const Icon = typeIcons[document.type];

  return (
    <div className="flex flex-col h-full bg-bg-base">
      {/* Header */}
      <header className="flex items-center justify-between px-4 h-14 border-b border-border-subtle shrink-0">
        <div className="flex items-center gap-2">
          <IconButton onClick={onBack} aria-label="Back to list">
            <ArrowLeft size={20} strokeWidth={1.5} />
          </IconButton>
        </div>

        <div className="flex items-center gap-1">
          <IconButton onClick={onDownload} aria-label="Download">
            <Download size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onShare} aria-label="Share">
            <Share2 size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onEdit} aria-label="Edit document">
            <Edit size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onDelete} aria-label="Delete document">
            <Trash2 size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton
            onClick={onStar}
            active={document.starred}
            aria-label={document.starred ? "Remove from starred" : "Add to starred"}
          >
            <Star
              size={20}
              strokeWidth={1.5}
              fill={document.starred ? "currentColor" : "none"}
            />
          </IconButton>
          <IconButton aria-label="More options">
            <MoreHorizontal size={20} strokeWidth={1.5} />
          </IconButton>
        </div>
      </header>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-3xl mx-auto p-6">
          {/* Document Header */}
          <div className="flex items-start gap-4 mb-8">
            <div
              className={cn(
                "w-16 h-16 rounded-xl flex items-center justify-center shrink-0",
                typeColors[document.type]
              )}
            >
              <Icon size={32} />
            </div>
            <div className="flex-1 min-w-0">
              <h1 className="text-2xl font-semibold text-text-primary mb-1">
                {document.title}
              </h1>
              <p className="text-text-secondary">
                {typeLabels[document.type]} · {formatSize(document.size)}
              </p>
            </div>
          </div>

          {/* Quick Actions */}
          <div className="flex gap-3 mb-8">
            <button
              onClick={onEdit}
              className={cn(
                "flex items-center gap-2 px-5 py-2.5 rounded-lg",
                "bg-accent-primary text-white",
                "hover:bg-accent-hover transition-colors font-medium"
              )}
            >
              <Edit size={18} strokeWidth={1.5} />
              <span>Open</span>
            </button>
            <button
              onClick={onDownload}
              className={cn(
                "flex items-center gap-2 px-5 py-2.5 rounded-lg",
                "bg-bg-surface text-text-primary border border-border-default",
                "hover:bg-bg-hover transition-colors font-medium"
              )}
            >
              <Download size={18} strokeWidth={1.5} />
              <span>Download</span>
            </button>
            <button
              onClick={onShare}
              className={cn(
                "flex items-center gap-2 px-5 py-2.5 rounded-lg",
                "bg-bg-surface text-text-primary border border-border-default",
                "hover:bg-bg-hover transition-colors font-medium"
              )}
            >
              <Share2 size={18} strokeWidth={1.5} />
              <span>Share</span>
            </button>
          </div>

          {/* Document Preview */}
          {document.content && (
            <div className="mb-8">
              <h2 className="text-sm font-semibold text-text-tertiary uppercase tracking-wider mb-3">
                Preview
              </h2>
              <div className="p-6 rounded-xl bg-bg-surface border border-border-subtle">
                <div className="prose prose-sm max-w-none text-text-primary whitespace-pre-wrap">
                  {document.content}
                </div>
              </div>
            </div>
          )}

          {/* Document Info */}
          <div className="space-y-1">
            <h2 className="text-sm font-semibold text-text-tertiary uppercase tracking-wider mb-3">
              Details
            </h2>

            {/* Owner */}
            <div className="flex items-center gap-4 px-4 py-3 rounded-lg hover:bg-bg-hover transition-colors">
              <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                <User size={18} className="text-text-tertiary" />
              </div>
              <div className="flex-1">
                <p className="text-sm text-text-tertiary">Owner</p>
                <p className="text-text-primary">{document.owner}</p>
              </div>
            </div>

            {/* Folder */}
            {document.folder && (
              <div className="flex items-center gap-4 px-4 py-3 rounded-lg hover:bg-bg-hover transition-colors">
                <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                  <FolderOpen size={18} className="text-text-tertiary" />
                </div>
                <div className="flex-1">
                  <p className="text-sm text-text-tertiary">Folder</p>
                  <p className="text-text-primary">{document.folder}</p>
                </div>
              </div>
            )}

            {/* Modified */}
            <div className="flex items-center gap-4 px-4 py-3 rounded-lg hover:bg-bg-hover transition-colors">
              <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                <Clock size={18} className="text-text-tertiary" />
              </div>
              <div className="flex-1">
                <p className="text-sm text-text-tertiary">Last modified</p>
                <p className="text-text-primary">{formatDate(document.updatedAt)}</p>
              </div>
            </div>

            {/* Created */}
            <div className="flex items-center gap-4 px-4 py-3 rounded-lg hover:bg-bg-hover transition-colors">
              <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                <Clock size={18} className="text-text-tertiary" />
              </div>
              <div className="flex-1">
                <p className="text-sm text-text-tertiary">Created</p>
                <p className="text-text-primary">{formatDate(document.createdAt)}</p>
              </div>
            </div>

            {/* Shared with */}
            {document.shared && document.sharedWith && document.sharedWith.length > 0 && (
              <div className="flex items-center gap-4 px-4 py-3 rounded-lg hover:bg-bg-hover transition-colors">
                <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                  <Users size={18} className="text-text-tertiary" />
                </div>
                <div className="flex-1">
                  <p className="text-sm text-text-tertiary">Shared with</p>
                  <p className="text-text-primary">{document.sharedWith.join(", ")}</p>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
});
