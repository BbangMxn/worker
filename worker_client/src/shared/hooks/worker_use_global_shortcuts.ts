"use client";

import { useEffect, useCallback } from "react";

interface UseGlobalShortcutsOptions {
  onCommandPalette?: () => void;
  onCompose?: () => void;
  onSearch?: () => void;
  enabled?: boolean;
}

export function useGlobalShortcuts({
  onCommandPalette,
  onCompose,
  onSearch,
  enabled = true,
}: UseGlobalShortcutsOptions) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!enabled) return;

      // Ignore if typing in input/textarea
      const target = e.target as HTMLElement;
      const isTyping =
        target.tagName === "INPUT" ||
        target.tagName === "TEXTAREA" ||
        target.isContentEditable;

      // Command Palette: Cmd/Ctrl + K (always active)
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        onCommandPalette?.();
        return;
      }

      // Skip other shortcuts if typing
      if (isTyping) return;

      // Compose: C
      if (e.key === "c" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault();
        onCompose?.();
        return;
      }

      // Search: /
      if (e.key === "/" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault();
        onSearch?.();
        return;
      }
    },
    [enabled, onCommandPalette, onCompose, onSearch]
  );

  useEffect(() => {
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);
}
