"use client";

import { forwardRef, InputHTMLAttributes } from "react";
import { cn } from "@/shared/lib";

export interface InputProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "size"
> {
  icon?: React.ReactNode;
  error?: boolean;
  inputSize?: "sm" | "md" | "lg";
}

const SIZE_CLASSES = {
  sm: "h-8 text-sm",
  md: "h-9 text-sm",
  lg: "h-11 text-base",
} as const;

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, icon, error, inputSize = "md", ...props }, ref) => {
    return (
      <div className="relative">
        {icon && (
          <div className="absolute left-3 top-1/2 -translate-y-1/2 text-text-tertiary pointer-events-none">
            {icon}
          </div>
        )}
        <input
          ref={ref}
          className={cn(
            "w-full bg-bg-surface border rounded-md",
            "text-text-primary placeholder:text-text-disabled",
            "transition-colors duration-fast",
            "focus:outline-none focus:ring-1",
            // Error state
            error
              ? "border-semantic-error focus:border-semantic-error focus:ring-semantic-error"
              : "border-border-default focus:border-accent-primary focus:ring-accent-primary",
            // Size
            SIZE_CLASSES[inputSize],
            // Padding based on icon
            icon ? "pl-10 pr-4" : "px-4",
            className,
          )}
          {...props}
        />
      </div>
    );
  },
);

Input.displayName = "Input";
