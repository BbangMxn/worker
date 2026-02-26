"use client";

import { memo, useMemo } from "react";
import { Users } from "lucide-react";
import { ContactCard, type Contact } from "@/entities/contact";
import { Skeleton } from "@/shared/ui";

interface ContactListProps {
  contacts: Contact[];
  selectedId?: string;
  onSelect: (contact: Contact) => void;
  onFavorite: (contact: Contact) => void;
  isLoading?: boolean;
  groupBy?: "none" | "alphabet" | "company";
}

function ContactSkeleton() {
  return (
    <div className="flex gap-4 px-4 py-3 border-b border-border-subtle">
      <Skeleton variant="circular" className="w-12 h-12 shrink-0" />
      <div className="flex-1 space-y-2">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-3 w-48" />
        <Skeleton className="h-3 w-40" />
      </div>
    </div>
  );
}

export const ContactList = memo(function ContactList({
  contacts,
  selectedId,
  onSelect,
  onFavorite,
  isLoading = false,
  groupBy = "alphabet",
}: ContactListProps) {
  // Group contacts
  const groupedContacts = useMemo(() => {
    if (groupBy === "none") {
      return { "": contacts };
    }

    const sorted = [...contacts].sort((a, b) => a.name.localeCompare(b.name));

    if (groupBy === "alphabet") {
      return sorted.reduce((acc, contact) => {
        const letter = contact.name.charAt(0).toUpperCase();
        if (!acc[letter]) acc[letter] = [];
        acc[letter].push(contact);
        return acc;
      }, {} as Record<string, Contact[]>);
    }

    if (groupBy === "company") {
      return sorted.reduce((acc, contact) => {
        const company = contact.company || "No Company";
        if (!acc[company]) acc[company] = [];
        acc[company].push(contact);
        return acc;
      }, {} as Record<string, Contact[]>);
    }

    return { "": contacts };
  }, [contacts, groupBy]);

  if (isLoading) {
    return (
      <div className="flex-1 overflow-hidden" role="status" aria-label="Loading contacts">
        {Array.from({ length: 6 }).map((_, i) => (
          <ContactSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (contacts.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-text-tertiary p-8">
        <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mb-4">
          <Users size={28} strokeWidth={1} className="opacity-50" />
        </div>
        <p className="text-lg font-medium text-text-secondary mb-1">No contacts</p>
        <p className="text-sm text-center">Add contacts to get started</p>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto" role="listbox" aria-label="Contact list">
      {Object.entries(groupedContacts).map(([group, groupContacts]) => (
        <div key={group}>
          {group && groupBy !== "none" && (
            <div className="sticky top-0 z-10 px-4 py-2 bg-bg-elevated border-b border-border-subtle">
              <span className="text-xs font-semibold text-text-tertiary uppercase tracking-wider">
                {group}
              </span>
            </div>
          )}
          {groupContacts.map((contact) => (
            <ContactCard
              key={contact.id}
              contact={contact}
              selected={contact.id === selectedId}
              onClick={() => onSelect(contact)}
              onFavorite={() => onFavorite(contact)}
            />
          ))}
        </div>
      ))}
    </div>
  );
});
