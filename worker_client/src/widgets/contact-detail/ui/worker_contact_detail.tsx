"use client";

import { memo } from "react";
import {
  ArrowLeft,
  Star,
  Mail,
  Phone,
  Building2,
  Edit,
  Trash2,
  MoreHorizontal,
  Copy,
  MessageSquare,
} from "lucide-react";
import { cn } from "@/shared/lib";
import { Avatar, IconButton } from "@/shared/ui";
import type { Contact } from "@/entities/contact";

interface ContactDetailProps {
  contact: Contact;
  onBack: () => void;
  onFavorite: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onEmail: () => void;
}

export const ContactDetail = memo(function ContactDetail({
  contact,
  onBack,
  onFavorite,
  onEdit,
  onDelete,
  onEmail,
}: ContactDetailProps) {
  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

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
          <IconButton onClick={onEdit} aria-label="Edit contact">
            <Edit size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton onClick={onDelete} aria-label="Delete contact">
            <Trash2 size={20} strokeWidth={1.5} />
          </IconButton>
          <IconButton
            onClick={onFavorite}
            active={contact.favorite}
            aria-label={contact.favorite ? "Remove from favorites" : "Add to favorites"}
          >
            <Star
              size={20}
              strokeWidth={1.5}
              fill={contact.favorite ? "currentColor" : "none"}
            />
          </IconButton>
          <IconButton aria-label="More options">
            <MoreHorizontal size={20} strokeWidth={1.5} />
          </IconButton>
        </div>
      </header>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-2xl mx-auto p-6">
          {/* Profile Header */}
          <div className="flex flex-col items-center text-center mb-8">
            <Avatar name={contact.name} src={contact.avatar} size="xl" className="mb-4 w-24 h-24 text-2xl" />
            <h1 className="text-2xl font-semibold text-text-primary mb-1">
              {contact.name}
            </h1>
            {contact.position && contact.company && (
              <p className="text-text-secondary">
                {contact.position} at {contact.company}
              </p>
            )}
            {contact.tags.length > 0 && (
              <div className="flex gap-2 mt-3">
                {contact.tags.map((tag) => (
                  <span
                    key={tag}
                    className="px-3 py-1 text-sm rounded-full bg-bg-surface text-text-secondary"
                  >
                    {tag}
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* Quick Actions */}
          <div className="flex justify-center gap-3 mb-8">
            <button
              onClick={onEmail}
              className={cn(
                "flex flex-col items-center gap-2 px-6 py-3 rounded-xl",
                "bg-accent-primary text-white",
                "hover:bg-accent-hover transition-colors"
              )}
            >
              <Mail size={20} strokeWidth={1.5} />
              <span className="text-sm font-medium">Email</span>
            </button>
            {contact.phone && (
              <a
                href={`tel:${contact.phone}`}
                className={cn(
                  "flex flex-col items-center gap-2 px-6 py-3 rounded-xl",
                  "bg-bg-surface text-text-primary border border-border-default",
                  "hover:bg-bg-hover transition-colors"
                )}
              >
                <Phone size={20} strokeWidth={1.5} />
                <span className="text-sm font-medium">Call</span>
              </a>
            )}
            <button
              className={cn(
                "flex flex-col items-center gap-2 px-6 py-3 rounded-xl",
                "bg-bg-surface text-text-primary border border-border-default",
                "hover:bg-bg-hover transition-colors"
              )}
            >
              <MessageSquare size={20} strokeWidth={1.5} />
              <span className="text-sm font-medium">Message</span>
            </button>
          </div>

          {/* Contact Info */}
          <div className="space-y-1">
            <h2 className="text-sm font-semibold text-text-tertiary uppercase tracking-wider mb-3">
              Contact Info
            </h2>

            {/* Email */}
            <div
              className={cn(
                "flex items-center gap-4 px-4 py-3 rounded-lg",
                "hover:bg-bg-hover transition-colors group cursor-pointer"
              )}
              onClick={() => copyToClipboard(contact.email)}
            >
              <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                <Mail size={18} className="text-text-tertiary" />
              </div>
              <div className="flex-1">
                <p className="text-sm text-text-tertiary">Email</p>
                <p className="text-text-primary">{contact.email}</p>
              </div>
              <Copy
                size={16}
                className="text-text-disabled opacity-0 group-hover:opacity-100 transition-opacity"
              />
            </div>

            {/* Phone */}
            {contact.phone && (
              <div
                className={cn(
                  "flex items-center gap-4 px-4 py-3 rounded-lg",
                  "hover:bg-bg-hover transition-colors group cursor-pointer"
                )}
                onClick={() => copyToClipboard(contact.phone!)}
              >
                <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                  <Phone size={18} className="text-text-tertiary" />
                </div>
                <div className="flex-1">
                  <p className="text-sm text-text-tertiary">Phone</p>
                  <p className="text-text-primary">{contact.phone}</p>
                </div>
                <Copy
                  size={16}
                  className="text-text-disabled opacity-0 group-hover:opacity-100 transition-opacity"
                />
              </div>
            )}

            {/* Company */}
            {contact.company && (
              <div className="flex items-center gap-4 px-4 py-3 rounded-lg">
                <div className="w-10 h-10 rounded-full bg-bg-surface flex items-center justify-center">
                  <Building2 size={18} className="text-text-tertiary" />
                </div>
                <div className="flex-1">
                  <p className="text-sm text-text-tertiary">Company</p>
                  <p className="text-text-primary">
                    {contact.company}
                    {contact.position && ` · ${contact.position}`}
                  </p>
                </div>
              </div>
            )}
          </div>

          {/* Notes */}
          {contact.notes && (
            <div className="mt-6">
              <h2 className="text-sm font-semibold text-text-tertiary uppercase tracking-wider mb-3">
                Notes
              </h2>
              <div className="px-4 py-3 rounded-lg bg-bg-surface">
                <p className="text-text-primary whitespace-pre-wrap">
                  {contact.notes}
                </p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
});
