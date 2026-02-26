"use client";

import { useState } from "react";
import { Plus, Search, Users } from "lucide-react";
import { cn } from "@/shared/lib";
import { mockContacts, type Contact } from "@/entities/contact";
import { ContactList } from "@/widgets/contact-list";
import { ContactDetail } from "@/widgets/contact-detail";

export default function ContactsPage() {
  const [contacts, setContacts] = useState(mockContacts);
  const [selectedContact, setSelectedContact] = useState<Contact | null>(null);
  const [searchQuery, setSearchQuery] = useState("");

  // Filter contacts by search
  const filteredContacts = contacts.filter(
    (contact) =>
      contact.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      contact.email.toLowerCase().includes(searchQuery.toLowerCase()) ||
      contact.company?.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleSelectContact = (contact: Contact) => {
    setSelectedContact(contact);
  };

  const handleFavorite = (contact: Contact) => {
    setContacts((prev) =>
      prev.map((c) =>
        c.id === contact.id ? { ...c, favorite: !c.favorite } : c
      )
    );
    if (selectedContact?.id === contact.id) {
      setSelectedContact((prev) =>
        prev ? { ...prev, favorite: !prev.favorite } : null
      );
    }
  };

  const handleDelete = () => {
    if (selectedContact) {
      setContacts((prev) => prev.filter((c) => c.id !== selectedContact.id));
      setSelectedContact(null);
    }
  };

  return (
    <>
      {/* Contact List Panel */}
      <div
        className={cn(
          "flex flex-col border-r border-border-default transition-all duration-normal bg-bg-base",
          selectedContact ? "w-[360px]" : "flex-1 max-w-[480px]"
        )}
      >
        {/* Header */}
        <header className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <div className="flex items-center gap-2">
            <Users size={20} strokeWidth={1.5} className="text-text-tertiary" />
            <h1 className="text-lg font-semibold text-text-primary">Contacts</h1>
            <span className="text-sm text-text-tertiary">
              ({filteredContacts.length})
            </span>
          </div>
          <button
            className={cn(
              "flex items-center gap-1.5 px-3 py-1.5 rounded-md",
              "bg-accent-primary hover:bg-accent-hover text-white",
              "text-sm font-medium transition-colors"
            )}
          >
            <Plus size={16} strokeWidth={2} />
            <span>Add</span>
          </button>
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
              placeholder="Search contacts..."
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

        {/* Contact List */}
        <ContactList
          contacts={filteredContacts}
          selectedId={selectedContact?.id}
          onSelect={handleSelectContact}
          onFavorite={handleFavorite}
          groupBy="alphabet"
        />
      </div>

      {/* Contact Detail Panel */}
      {selectedContact ? (
        <div className="flex-1 min-w-[400px]">
          <ContactDetail
            contact={selectedContact}
            onBack={() => setSelectedContact(null)}
            onFavorite={() => handleFavorite(selectedContact)}
            onEdit={() => console.log("Edit contact")}
            onDelete={handleDelete}
            onEmail={() => console.log("Send email to", selectedContact.email)}
          />
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center text-text-tertiary bg-bg-base">
          <div className="text-center">
            <div className="w-16 h-16 rounded-2xl bg-bg-surface flex items-center justify-center mx-auto mb-4">
              <Users size={28} strokeWidth={1} className="opacity-50" />
            </div>
            <p className="text-lg font-medium text-text-secondary mb-1">
              Select a contact
            </p>
            <p className="text-sm">Choose from the list to view details</p>
          </div>
        </div>
      )}
    </>
  );
}
