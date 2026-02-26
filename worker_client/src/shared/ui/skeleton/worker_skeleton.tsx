"use client";

import { cn } from "@/shared/lib";

interface SkeletonProps {
  className?: string;
  variant?: "text" | "circular" | "rectangular";
  width?: string | number;
  height?: string | number;
}

export function Skeleton({
  className,
  variant = "rectangular",
  width,
  height,
}: SkeletonProps) {
  return (
    <div
      className={cn(
        "bg-border-subtle animate-pulse",
        variant === "circular" && "rounded-full",
        variant === "text" && "rounded h-4",
        variant === "rectangular" && "rounded-md",
        className,
      )}
      style={{ width, height }}
      aria-hidden="true"
    />
  );
}

// Pre-built skeleton patterns
export function EmailCardSkeleton() {
  return (
    <div className="flex gap-3 px-4 py-3 border-b border-border-subtle bg-bg-base">
      <Skeleton variant="circular" className="w-10 h-10 shrink-0" />
      <div className="flex-1 min-w-0 space-y-2">
        <div className="flex justify-between gap-4">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-12" />
        </div>
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-3 w-full" />
      </div>
    </div>
  );
}

export function EmailDetailSkeleton() {
  return (
    <div className="flex flex-col h-full bg-bg-base">
      {/* Header */}
      <div className="flex items-center justify-between px-4 h-14 border-b border-border-subtle">
        <Skeleton className="w-9 h-9" />
        <div className="flex gap-2">
          <Skeleton className="w-9 h-9" />
          <Skeleton className="w-9 h-9" />
          <Skeleton className="w-9 h-9" />
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 p-6">
        <div className="max-w-3xl mx-auto space-y-6">
          <Skeleton className="h-8 w-3/4" />

          <div className="flex gap-4">
            <Skeleton variant="circular" className="w-12 h-12" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-4 w-48" />
              <Skeleton className="h-3 w-32" />
            </div>
          </div>

          <div className="space-y-3 pt-4">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-5/6" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        </div>
      </div>
    </div>
  );
}

export function SidebarSkeleton() {
  return (
    <div className="w-[200px] flex flex-col bg-bg-elevated">
      <div className="h-14 flex items-center px-4 border-b border-border-subtle">
        <Skeleton className="h-5 w-20" />
      </div>
      <div className="p-3">
        <Skeleton className="h-10 w-full rounded-md" />
      </div>
      <div className="px-2 space-y-1">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full rounded-md" />
        ))}
      </div>
    </div>
  );
}
