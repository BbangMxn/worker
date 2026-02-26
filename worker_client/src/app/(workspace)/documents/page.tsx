"use client";

import { useState } from "react";
import { Plus, Search, FileText, Upload } from "lucide-react";
import { cn } from "@/shared/lib";
import {
  mockDocuments,
  mockFolders,
  type Document,
} from "@/entities/document";
import { DocumentList } from "@/widgets/document-list";
import { DocumentDetail } from "@/widgets/document-detail";

export default function DocumentsPage() {
  const [documents, setDocuments] = useState(mockDocuments);
  const [selectedDocument, setSelectedDocument] = useState<Document | null>(null);
  const [selectedFolder, setSelectedFolder] = useState<string | undefined>();
  const [searchQuery, setSearchQuery] = useState("");
  const [view, setView] = useState<"list" | "grid">("list");

  // Filter documents by search
  const filteredDocuments = documents.filter((doc) =>
    doc.title.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleSelectDocument = (document: Document) => {
    setSelectedDocument(document);
  };

  const handleStar = (document: Document) => {
    setDocuments((prev) =>
      prev.map((d) =>
        d.id === document.id ? { ...d, starred: !d.starred } : d
      )
    );
    if (selectedDocument?.id === document.id) {
      setSelectedDocument((prev) =>
        prev ? { ...prev, starred: !prev.starred } : null
      );
    }
  };

  const handleDelete = () => {
    if (selectedDocument) {
      setDocuments((prev) => prev.filter((d) => d.id !== selectedDocument.id));
      setSelectedDocument(null);
    }
  };

  return (
    <>
      {/* Document List Panel */}
      <div
        className={cn(
          "flex flex-col border-r border-border-default transition-all duration-normal bg-bg-base",
          selectedDocument ? "w-[400px]" : "flex-1 max-w-[560px]"
        )}
      >
        {/* Header */}
        <header className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <div className="flex items-center gap-2">
            <FileText size={20} strokeWidth={1.5} className="text-text-tertiary" />
            <h1 className="text-lg font-semibold text-text-primary">Documents</h1>
            <span className="text-sm text-text-tertiary">
              ({filteredDocuments.length})
            </span>
          </div>
          <div className="flex items-center gap-2">
            <button
              className={cn(
                "flex items-center gap-1.5 px-3 py-1.5 rounded-md",
                "bg-bg-surface hover:bg-bg-hover text-text-secondary",
                "text-sm font-medium transition-colors border border-border-default"
              )}
            >
              <Upload size={16} strokeWidth={2} />
              <span>Upload</span>
            </button>
            <button
              className={cn(
                "flex items-center gap-1.5 px-3 py-1.5 rounded-md",
                "bg-accent-primary hover:bg-accent-hover text-white",
                "text-sm font-medium transition-colors"
              )}
            >
              <Plus size={16} strokeWidth={2} />
              <span>New</span>
            </button>
          </div>
        </header>

        {/* Search */}
        <div className="px-4 py-3 border-b border-border-subtle">
          <div className="relative">
            <Search
              size={16}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary"
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search documents..."
              className={cn(
                "w-full h-9 pl-9 pr-4 rounded-md",
                "bg-bg-surface border border-border-default",
                "text-sm text-text-primary placeholder:text-text-disabled",
                "focus:outline-none focus:border-accent-primary focus:ring-1 focus:ring-accent-primary",
                "transition-colors"
              )}
            />
          </div>
        </div>

        {/* Document List */}
        <DocumentList
          documents={filteredDocuments}
          folders={mockFolders}
          selectedId={selectedDocument?.id}
          selectedFolder={selectedFolder}
          onSelect={handleSelectDocument}
          onStar={handleStar}
          onSelectFolder={setSelectedFolder}
          view={view}
          onViewChange={setView}
        />
      </div>

      {/* Document Detail Panel */}
      {selectedDocument ? (
        <div className="flex-1 min-w-[400px]">
          <DocumentDetail
            document={selectedDocument}
            onBack={() => setSelectedDocument(null)}
            onStar={() => handleStar(selectedDocument)}
            onEdit={() => console.log("Edit document")}
            onDelete={handleDelete}
            onDownload={() => console.log("Download document")}
            onShare={() => console.log("Share document")}
          />
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center text-text-tertiary bg-bg-base">
          <div className="text-center">
            <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mx-auto mb-4">
              <FileText size={28} strokeWidth={1} className="opacity-50" />
            </div>
            <p className="text-lg font-medium text-text-secondary mb-1">
              Select a document
            </p>
            <p className="text-sm">Choose from the list to view details</p>
          </div>
        </div>
      )}
    </>
  );
}
