"use client";

import { Suspense, useState, useMemo, useEffect, useCallback } from "react";
import { useSearchParams } from "next/navigation";
import { Inbox } from "lucide-react";
import { EmailList } from "@/widgets/email-list";
import { EmailDetail } from "@/widgets/email-detail";
import { ComposeModal } from "@/widgets/compose";
import { mockEmails, type Email } from "@/entities/email";

const FOLDER_LABELS: Record<string, string> = {
  all: "All Mail",
  inbox: "Inbox",
  todo: "Todo",
  done: "Done",
  starred: "Starred",
  sent: "Sent",
  drafts: "Drafts",
  archive: "Archive",
  trash: "Trash",
};

function MailPageContent() {
  const searchParams = useSearchParams();
  const activeFolder = searchParams?.get("folder") || "inbox";

  const [selectedEmail, setSelectedEmail] = useState<Email | null>(null);
  const [emails, setEmails] = useState(mockEmails);
  const [composeOpen, setComposeOpen] = useState(false);

  // Listen for compose event from layout
  useEffect(() => {
    const handleCompose = () => setComposeOpen(true);
    window.addEventListener("app:compose", handleCompose);
    return () => window.removeEventListener("app:compose", handleCompose);
  }, []);

  // Clear selection when folder changes
  useEffect(() => {
    setSelectedEmail(null);
  }, [activeFolder]);

  // Filter emails based on active folder
  const filteredEmails = useMemo(() => {
    switch (activeFolder) {
      case "all":
        return emails.filter((e) => e.folder !== "trash");
      case "inbox":
        return emails.filter(
          (e) => e.folder === "inbox" && e.workflowStatus === "inbox",
        );
      case "todo":
        return emails.filter((e) => e.workflowStatus === "todo");
      case "done":
        return emails.filter((e) => e.workflowStatus === "done");
      case "starred":
        return emails.filter((e) => e.tags.includes("starred"));
      case "sent":
        return emails.filter((e) => e.folder === "sent");
      case "drafts":
        return emails.filter((e) => e.folder === "draft");
      case "archive":
        return emails.filter((e) => e.folder === "archive");
      case "trash":
        return emails.filter((e) => e.folder === "trash");
      default:
        return emails.filter((e) => e.folder === "inbox");
    }
  }, [emails, activeFolder]);

  // Email actions
  const handleSelectEmail = useCallback((email: Email) => {
    setSelectedEmail(email);
    setEmails((prev) =>
      prev.map((e) => (e.id === email.id ? { ...e, isRead: true } : e)),
    );
  }, []);

  const handleStarEmail = useCallback((email: Email) => {
    const updateTags = (e: Email) => ({
      ...e,
      tags: e.tags.includes("starred")
        ? e.tags.filter((t) => t !== "starred")
        : [...e.tags, "starred" as const],
    });

    setEmails((prev) =>
      prev.map((e) => (e.id === email.id ? updateTags(e) : e)),
    );
    setSelectedEmail((prev) =>
      prev?.id === email.id ? updateTags(prev) : prev,
    );
  }, []);

  const handleMarkTodo = useCallback(() => {
    if (!selectedEmail) return;
    const updated = { ...selectedEmail, workflowStatus: "todo" as const };
    setEmails((prev) =>
      prev.map((e) => (e.id === selectedEmail.id ? updated : e)),
    );
    setSelectedEmail(updated);
  }, [selectedEmail]);

  const handleMarkDone = useCallback(() => {
    if (!selectedEmail) return;
    const updated = { ...selectedEmail, workflowStatus: "done" as const };
    setEmails((prev) =>
      prev.map((e) => (e.id === selectedEmail.id ? updated : e)),
    );
    setSelectedEmail(updated);
  }, [selectedEmail]);

  const handleArchive = useCallback(() => {
    if (!selectedEmail) return;
    setEmails((prev) =>
      prev.map((e) =>
        e.id === selectedEmail.id ? { ...e, folder: "archive" as const } : e,
      ),
    );
    setSelectedEmail(null);
  }, [selectedEmail]);

  const handleDelete = useCallback(() => {
    if (!selectedEmail) return;
    setEmails((prev) =>
      prev.map((e) =>
        e.id === selectedEmail.id ? { ...e, folder: "trash" as const } : e,
      ),
    );
    setSelectedEmail(null);
  }, [selectedEmail]);

  const handleSendEmail = useCallback(
    (data: { to: string; subject: string; body: string }) => {
      console.log("Sending email:", data);
      setComposeOpen(false);
    },
    [],
  );

  return (
    <>
      {/* Email List Panel - Fixed width for stability */}
      <div className="w-[380px] shrink-0 flex flex-col border-r border-border-default bg-bg-base">
        <header className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <h1 className="text-lg font-semibold text-text-primary">
            {FOLDER_LABELS[activeFolder] || "Inbox"}
          </h1>
          <span className="text-sm text-text-tertiary tabular-nums">
            {filteredEmails.length} email
            {filteredEmails.length !== 1 ? "s" : ""}
          </span>
        </header>

        <EmailList
          emails={filteredEmails}
          selectedId={selectedEmail?.id}
          onSelect={handleSelectEmail}
          onStar={handleStarEmail}
        />
      </div>

      {/* Email Detail Panel */}
      {selectedEmail ? (
        <div className="flex-1 min-w-[400px] animate-slide-in-right">
          <EmailDetail
            email={selectedEmail}
            onBack={() => setSelectedEmail(null)}
            onStar={() => handleStarEmail(selectedEmail)}
            onReply={() => setComposeOpen(true)}
            onForward={() => setComposeOpen(true)}
            onArchive={handleArchive}
            onDelete={handleDelete}
            onMarkTodo={handleMarkTodo}
            onMarkDone={handleMarkDone}
          />
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center bg-bg-base">
          <div className="text-center">
            <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mx-auto mb-4">
              <Inbox size={28} strokeWidth={1} className="text-text-disabled" />
            </div>
            <p className="text-lg font-medium text-text-secondary mb-1">
              Select an email
            </p>
            <p className="text-sm text-text-tertiary">
              Choose from the list to read
            </p>
          </div>
        </div>
      )}

      {/* Compose Modal */}
      <ComposeModal
        isOpen={composeOpen}
        onClose={() => setComposeOpen(false)}
        onSend={handleSendEmail}
      />
    </>
  );
}

export default function MailPage() {
  return (
    <Suspense fallback={<MailPageSkeleton />}>
      <MailPageContent />
    </Suspense>
  );
}

function MailPageSkeleton() {
  return (
    <>
      <div className="w-[380px] shrink-0 flex flex-col border-r border-border-default bg-bg-base">
        <header className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <div className="h-6 w-24 bg-bg-surface rounded animate-pulse" />
          <div className="h-4 w-16 bg-bg-surface rounded animate-pulse" />
        </header>
        <div className="flex-1 p-2 space-y-2">
          {[...Array(8)].map((_, i) => (
            <div key={i} className="p-3 rounded-lg bg-bg-surface animate-pulse">
              <div className="h-4 w-32 bg-bg-hover rounded mb-2" />
              <div className="h-3 w-48 bg-bg-hover rounded mb-1" />
              <div className="h-3 w-full bg-bg-hover rounded" />
            </div>
          ))}
        </div>
      </div>
      <div className="flex-1 flex items-center justify-center bg-bg-base">
        <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
      </div>
    </>
  );
}
