"use client";

import { memo } from "react";
import { Star, Building2, Mail, Phone } from "lucide-react";
import { cn } from "@/shared/lib";
import { Avatar } from "@/shared/ui";
import type { Contact } from "../model/worker_contact_types";

interface ContactCardProps {
  contact: Contact;
  selected?: boolean;
  onClick?: () => void;
  onFavorite?: () => void;
  compact?: boolean;
}

export const ContactCard = memo(function ContactCard({
  contact,
  selected,
  onClick,
  onFavorite,
  compact = false,
}: ContactCardProps) {
  const handleFavoriteClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onFavorite?.();
  };

  if (compact) {
    return (
      <button
        onClick={onClick}
        className={cn(
          "flex items-center gap-3 w-full px-4 py-2.5 text-left",
          "transition-colors duration-fast",
          "hover:bg-bg-hover",
          selected && "bg-bg-active"
        )}
      >
        <Avatar name={contact.name} src={contact.avatar} size="sm" />
        <div className="flex-1 min-w-0">
          <span className="text-sm font-medium text-text-primary truncate block">
            {contact.name}
          </span>
        </div>
        {contact.favorite && (
          <Star size={14} className="text-status-starred fill-status-starred" />
        )}
      </button>
    );
  }

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
        "flex gap-4 px-4 py-3 cursor-pointer",
        "border-b border-border-subtle",
        "transition-colors duration-fast",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-primary",
        selected ? "bg-bg-active" : "hover:bg-bg-hover"
      )}
    >
      <Avatar name={contact.name} src={contact.avatar} size="lg" />

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h3 className="text-sm font-semibold text-text-primary truncate">
            {contact.name}
          </h3>
          {contact.favorite && (
            <Star
              size={14}
              className="text-status-starred fill-status-starred shrink-0"
            />
          )}
        </div>

        {contact.position && contact.company && (
          <p className="text-sm text-text-secondary truncate mb-1">
            {contact.position} · {contact.company}
          </p>
        )}

        <div className="flex items-center gap-4 text-xs text-text-tertiary">
          <span className="flex items-center gap-1 truncate">
            <Mail size={12} />
            {contact.email}
          </span>
          {contact.phone && (
            <span className="flex items-center gap-1 shrink-0">
              <Phone size={12} />
              {contact.phone}
            </span>
          )}
        </div>

        {contact.tags.length > 0 && (
          <div className="flex gap-1.5 mt-2">
            {contact.tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="px-2 py-0.5 text-xs rounded-full bg-bg-surface text-text-secondary"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>

      <button
        onClick={handleFavoriteClick}
        className={cn(
          "p-1.5 rounded-md transition-colors duration-fast shrink-0 self-start",
          contact.favorite
            ? "text-status-starred"
            : "text-text-disabled hover:text-text-tertiary"
        )}
        aria-label={contact.favorite ? "Remove from favorites" : "Add to favorites"}
      >
        <Star
          size={16}
          strokeWidth={1.5}
          fill={contact.favorite ? "currentColor" : "none"}
        />
      </button>
    </article>
  );
});
