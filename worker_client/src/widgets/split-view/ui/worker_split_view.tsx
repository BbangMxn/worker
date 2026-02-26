"use client";

import { ReactNode, useState, useRef, useCallback, useEffect } from "react";
import { cn } from "@/shared/lib";

interface SplitViewProps {
  sidebar: ReactNode;
  list: ReactNode;
  detail: ReactNode;
  sidebarCollapsed?: boolean;
  onSidebarCollapsedChange?: (collapsed: boolean) => void;
  listWidth?: number;
  onListWidthChange?: (width: number) => void;
  className?: string;
}

const MIN_LIST_WIDTH = 280;
const MAX_LIST_WIDTH = 500;
const DEFAULT_LIST_WIDTH = 360;

export function SplitView({
  sidebar,
  list,
  detail,
  sidebarCollapsed = false,
  onSidebarCollapsedChange,
  listWidth: controlledListWidth,
  onListWidthChange,
  className,
}: SplitViewProps) {
  // List panel width state
  const [internalListWidth, setInternalListWidth] = useState(() => {
    if (typeof window !== "undefined") {
      const saved = localStorage.getItem("worker-list-width");
      return saved ? parseInt(saved, 10) : DEFAULT_LIST_WIDTH;
    }
    return DEFAULT_LIST_WIDTH;
  });

  const listWidth = controlledListWidth ?? internalListWidth;
  const setListWidth = onListWidthChange ?? setInternalListWidth;

  // Resize state
  const [isResizing, setIsResizing] = useState(false);
  const resizeRef = useRef<{ startX: number; startWidth: number } | null>(null);

  // Handle resize
  const handleResizeStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      resizeRef.current = {
        startX: e.clientX,
        startWidth: listWidth,
      };
    },
    [listWidth]
  );

  useEffect(() => {
    if (!isResizing) return;

    const handleMouseMove = (e: MouseEvent) => {
      if (!resizeRef.current) return;

      const delta = e.clientX - resizeRef.current.startX;
      const newWidth = Math.min(
        MAX_LIST_WIDTH,
        Math.max(MIN_LIST_WIDTH, resizeRef.current.startWidth + delta)
      );
      setListWidth(newWidth);
    };

    const handleMouseUp = () => {
      setIsResizing(false);
      resizeRef.current = null;
      // Persist to localStorage
      localStorage.setItem("worker-list-width", listWidth.toString());
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);

    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };
  }, [isResizing, listWidth, setListWidth]);

  // Sidebar width
  const sidebarWidth = sidebarCollapsed ? 60 : 240;

  return (
    <div
      className={cn("h-screen flex bg-bg-base overflow-hidden", className)}
      style={{
        // Prevent layout shift by using CSS custom properties
        ["--sidebar-width" as string]: `${sidebarWidth}px`,
        ["--list-width" as string]: `${listWidth}px`,
      }}
    >
      {/* Sidebar - Fixed position */}
      <aside
        className={cn(
          "h-full shrink-0 transition-[width] duration-normal ease-smooth",
          "border-r border-border-subtle bg-bg-elevated"
        )}
        style={{ width: sidebarWidth }}
      >
        {sidebar}
      </aside>

      {/* List Panel - Fixed width with resize handle */}
      <div
        className="h-full shrink-0 flex flex-col border-r border-border-default bg-bg-base relative"
        style={{ width: listWidth }}
      >
        {list}

        {/* Resize handle */}
        <div
          onMouseDown={handleResizeStart}
          className={cn(
            "absolute top-0 right-0 w-1 h-full cursor-col-resize z-10",
            "transition-colors duration-fast",
            "hover:bg-accent-primary",
            isResizing && "bg-accent-primary"
          )}
        />
      </div>

      {/* Detail Panel - Flex grow with min-width */}
      <main className="flex-1 h-full min-w-[500px] overflow-hidden bg-bg-base">
        {detail}
      </main>

      {/* Resize overlay to prevent text selection */}
      {isResizing && (
        <div className="fixed inset-0 z-50 cursor-col-resize" />
      )}
    </div>
  );
}
