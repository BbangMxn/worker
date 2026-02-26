"use client";

import { memo } from "react";
import { cn, getInitials } from "@/shared/lib";

interface AvatarProps {
  src?: string;
  name: string;
  size?: "sm" | "md" | "lg" | "xl";
  className?: string;
}

const SIZE_CLASSES = {
  sm: "w-8 h-8 text-xs",
  md: "w-10 h-10 text-sm",
  lg: "w-12 h-12 text-base",
  xl: "w-14 h-14 text-lg",
} as const;

// Gradient palette for fallback avatars
const GRADIENTS = [
  "from-violet-500 to-purple-600",
  "from-blue-500 to-cyan-500",
  "from-emerald-500 to-teal-500",
  "from-orange-500 to-red-500",
  "from-pink-500 to-rose-500",
  "from-indigo-500 to-blue-500",
] as const;

function getGradient(name: string): string {
  const index = name.charCodeAt(0) % GRADIENTS.length;
  return GRADIENTS[index];
}

export const Avatar = memo(function Avatar({
  src,
  name,
  size = "md",
  className,
}: AvatarProps) {
  const sizeClass = SIZE_CLASSES[size];

  // Image avatar
  if (src) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={src}
        alt={name}
        loading="lazy"
        className={cn(
          "rounded-full object-cover shrink-0",
          sizeClass,
          className,
        )}
      />
    );
  }

  // Fallback: Initials with gradient
  return (
    <div
      className={cn(
        "flex items-center justify-center rounded-full shrink-0",
        "bg-gradient-to-br font-medium text-white",
        getGradient(name),
        sizeClass,
        className,
      )}
      role="img"
      aria-label={name}
    >
      {getInitials(name)}
    </div>
  );
});
