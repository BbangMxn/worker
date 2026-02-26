"use client";

import { forwardRef, ButtonHTMLAttributes } from "react";
import { cn } from "@/shared/lib";

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  size?: "sm" | "md" | "lg";
  active?: boolean;
  variant?: "default" | "ghost";
}

const SIZE_CLASSES = {
  sm: "w-8 h-8",
  md: "w-9 h-9",
  lg: "w-10 h-10",
} as const;

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  (
    { className, size = "md", active, variant = "default", children, ...props },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center rounded-md",
          "transition-colors duration-fast",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2 focus-visible:ring-offset-bg-base",
          "disabled:opacity-50 disabled:pointer-events-none",
          // Variant styles
          variant === "default" && [
            "text-text-tertiary hover:text-text-primary hover:bg-bg-hover",
            active && "text-accent-primary bg-accent-bg",
          ],
          variant === "ghost" && [
            "text-text-tertiary hover:text-text-primary",
            active && "text-accent-primary",
          ],
          SIZE_CLASSES[size],
          className,
        )}
        {...props}
      >
        {children}
      </button>
    );
  },
);

IconButton.displayName = "IconButton";
