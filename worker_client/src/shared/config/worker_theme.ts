// Design tokens - Light theme (눈에 편한 라이트 컬러)
// Instagram layout + Uber content + Superhuman icons

export const theme = {
  colors: {
    bg: {
      base: "#ffffff", // 메인 캔버스
      elevated: "#f8f9fa", // 사이드바, 헤더
      surface: "#f1f3f4", // 카드, 입력창
      overlay: "#ffffff", // 모달
      hover: "#e8eaed", // 호버
      active: "#dfe1e5", // 선택됨
    },
    text: {
      primary: "#202124", // 진한 차콜
      secondary: "#5f6368", // 중간 회색
      tertiary: "#80868b", // 연한 회색
      disabled: "#bdc1c6", // 비활성
    },
    accent: {
      primary: "#1a73e8", // Google 블루
      hover: "#1967d2",
      active: "#185abc",
      bg: "rgba(26, 115, 232, 0.08)",
    },
    border: {
      default: "#dadce0",
      subtle: "#e8eaed",
      strong: "#bdc1c6",
    },
    semantic: {
      success: "#34a853",
      warning: "#f9ab00",
      error: "#ea4335",
      info: "#1a73e8",
    },
    status: {
      unread: "#202124",
      starred: "#f9ab00",
      important: "#ea4335",
      success: "#34a853",
    },
  },
  spacing: {
    sidebar: "240px",
    sidebarCollapsed: "60px",
    header: "56px",
  },
  radius: {
    sm: "4px",
    md: "6px",
    lg: "8px",
    xl: "12px",
    full: "9999px",
  },
  transition: {
    fast: "100ms ease-out",
    normal: "150ms ease-out",
    slow: "300ms cubic-bezier(0.16, 1, 0.3, 1)",
  },
  shadow: {
    sm: "0 1px 2px rgba(60, 64, 67, 0.1)",
    md: "0 2px 6px rgba(60, 64, 67, 0.15)",
    lg: "0 4px 12px rgba(60, 64, 67, 0.15)",
  },
} as const;

export type Theme = typeof theme;
