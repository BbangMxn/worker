"use client";

import { memo, useMemo } from "react";
import { FileText, Grid, List } from "lucide-react";
import { DocumentCard, type Document, type DocumentFolder } from "@/entities/document";
import { Skeleton } from "@/shared/ui";
import { cn } from "@/shared/lib";

interface DocumentListProps {
  documents: Document[];
  folders: DocumentFolder[];
  selectedId?: string;
  selectedFolder?: string;
  onSelect: (document: Document) => void;
  onStar: (document: Document) => void;
  onSelectFolder: (folderId: string | undefined) => void;
  isLoading?: boolean;
  view?: "list" | "grid";
  onViewChange?: (view: "list" | "grid") => void;
}

function DocumentSkeleton({ view }: { view: "list" | "grid" }) {
  if (view === "grid") {
    return (
      <div className="rounded-xl border border-border-default overflow-hidden">
        <Skeleton className="h-32 rounded-none" />
        <div className="p-3 space-y-2">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-4 px-4 py-3 border-b border-border-subtle">
      <Skeleton className="w-10 h-10 rounded-lg shrink-0" />
      <div className="flex-1 space-y-2">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-3 w-32" />
      </div>
    </div>
  );
}

export const DocumentList = memo(function DocumentList({
  documents,
  folders,
  selectedId,
  selectedFolder,
  onSelect,
  onStar,
  onSelectFolder,
  isLoading = false,
  view = "list",
  onViewChange,
}: DocumentListProps) {
  // Filter by folder
  const filteredDocuments = useMemo(() => {
    if (!selectedFolder) return documents;
    return documents.filter((doc) => doc.folder === selectedFolder);
  }, [documents, selectedFolder]);

  if (isLoading) {
    return (
      <div className="flex-1 overflow-hidden" role="status" aria-label="Loading documents">
        {view === "grid" ? (
          <div className="p-4 grid grid-cols-2 lg:grid-cols-3 gap-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <DocumentSkeleton key={i} view="grid" />
            ))}
          </div>
        ) : (
          Array.from({ length: 6 }).map((_, i) => (
            <DocumentSkeleton key={i} view="list" />
          ))
        )}
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Folder tabs */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-border-subtle overflow-x-auto shrink-0">
        <button
          onClick={() => onSelectFolder(undefined)}
          className={cn(
            "px-3 py-1.5 rounded-full text-sm font-medium whitespace-nowrap transition-colors",
            !selectedFolder
              ? "bg-accent-primary text-white"
              : "bg-bg-surface text-text-secondary hover:bg-bg-hover"
          )}
        >
          All
        </button>
        {folders.map((folder) => (
          <button
            key={folder.id}
            onClick={() => onSelectFolder(folder.id)}
            className={cn(
              "flex items-center gap-2 px-3 py-1.5 rounded-full text-sm font-medium whitespace-nowrap transition-colors",
              selectedFolder === folder.id
                ? "bg-accent-primary text-white"
                : "bg-bg-surface text-text-secondary hover:bg-bg-hover"
            )}
          >
            <span
              className="w-2 h-2 rounded-full"
              style={{ backgroundColor: folder.color }}
            />
            {folder.name}
            <span className="text-xs opacity-70">({folder.documentCount})</span>
          </button>
        ))}

        {/* View toggle */}
        {onViewChange && (
          <div className="ml-auto flex items-center gap-1 bg-bg-surface rounded-md p-0.5">
            <button
              onClick={() => onViewChange("list")}
              className={cn(
                "p-1.5 rounded transition-colors",
                view === "list"
                  ? "bg-bg-base text-text-primary shadow-sm"
                  : "text-text-tertiary hover:text-text-secondary"
              )}
              aria-label="List view"
            >
              <List size={16} />
            </button>
            <button
              onClick={() => onViewChange("grid")}
              className={cn(
                "p-1.5 rounded transition-colors",
                view === "grid"
                  ? "bg-bg-base text-text-primary shadow-sm"
                  : "text-text-tertiary hover:text-text-secondary"
              )}
              aria-label="Grid view"
            >
              <Grid size={16} />
            </button>
          </div>
        )}
      </div>

      {/* Document list */}
      {filteredDocuments.length === 0 ? (
        <div className="flex-1 flex flex-col items-center justify-center text-text-tertiary p-8">
          <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mb-4">
            <FileText size={28} strokeWidth={1} className="opacity-50" />
          </div>
          <p className="text-lg font-medium text-text-secondary mb-1">No documents</p>
          <p className="text-sm text-center">
            {selectedFolder ? "This folder is empty" : "Upload documents to get started"}
          </p>
        </div>
      ) : view === "grid" ? (
        <div
          className="flex-1 overflow-y-auto p-4 grid grid-cols-2 lg:grid-cols-3 gap-4 content-start"
          role="listbox"
          aria-label="Document list"
        >
          {filteredDocuments.map((doc) => (
            <DocumentCard
              key={doc.id}
              document={doc}
              selected={doc.id === selectedId}
              onClick={() => onSelect(doc)}
              onStar={() => onStar(doc)}
              view="grid"
            />
          ))}
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto" role="listbox" aria-label="Document list">
          {filteredDocuments.map((doc) => (
            <DocumentCard
              key={doc.id}
              document={doc}
              selected={doc.id === selectedId}
              onClick={() => onSelect(doc)}
              onStar={() => onStar(doc)}
              view="list"
            />
          ))}
        </div>
      )}
    </div>
  );
});
