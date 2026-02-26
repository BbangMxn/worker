import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        // Backgrounds - 눈에 편한 라이트 테마
        bg: {
          base: "#ffffff", // 메인 캔버스 (순백)
          elevated: "#f8f9fa", // 사이드바, 헤더 (아주 연한 회색)
          surface: "#f1f3f4", // 카드, 입력창 배경
          overlay: "#ffffff", // 모달, 드롭다운
          hover: "#e8eaed", // 호버 상태
          active: "#dfe1e5", // 선택됨/활성
          // Legacy aliases
          primary: "#ffffff",
          secondary: "#f8f9fa",
          tertiary: "#f1f3f4",
        },

        // Text - 눈에 편한 회색 톤
        text: {
          primary: "#202124", // 제목, 중요 텍스트 (진한 차콜)
          secondary: "#5f6368", // 본문 (중간 회색)
          tertiary: "#80868b", // 힌트, 타임스탬프 (연한 회색)
          disabled: "#bdc1c6", // 비활성
          // Legacy alias
          muted: "#bdc1c6",
        },

        // Accent - 부드러운 블루
        accent: {
          DEFAULT: "#1a73e8", // Google 블루
          primary: "#1a73e8",
          hover: "#1967d2",
          active: "#185abc",
          bg: "rgba(26, 115, 232, 0.08)",
          // Legacy alias
          muted: "#185abc",
        },

        // Semantic - 부드러운 톤
        semantic: {
          success: "#34a853", // 그린
          warning: "#f9ab00", // 옐로우
          error: "#ea4335", // 레드
          info: "#1a73e8", // 블루
        },

        // Borders - 섬세한 구분선
        border: {
          DEFAULT: "#dadce0", // 기본 보더
          default: "#dadce0",
          subtle: "#e8eaed", // 연한 보더
          strong: "#bdc1c6", // 진한 보더
          // Legacy alias
          light: "#bdc1c6",
        },

        // Status
        status: {
          unread: "#202124",
          starred: "#f9ab00", // 골드
          important: "#ea4335",
          success: "#34a853",
        },
      },

      fontFamily: {
        sans: [
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Roboto",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
      },

      fontSize: {
        "2xs": ["0.625rem", { lineHeight: "0.875rem" }],
        xs: ["0.75rem", { lineHeight: "1rem" }],
        sm: ["0.875rem", { lineHeight: "1.25rem" }],
        base: ["1rem", { lineHeight: "1.5rem" }],
        lg: ["1.125rem", { lineHeight: "1.75rem" }],
        xl: ["1.25rem", { lineHeight: "1.75rem" }],
        "2xl": ["1.5rem", { lineHeight: "2rem" }],
        "3xl": ["1.875rem", { lineHeight: "2.25rem" }],
      },

      spacing: {
        "18": "4.5rem",
        "22": "5.5rem",
        // Layout
        sidebar: "240px",
        "sidebar-collapsed": "60px",
        "list-panel": "360px",
        header: "56px",
      },

      borderRadius: {
        sm: "4px",
        md: "6px",
        lg: "8px",
        xl: "12px",
        "2xl": "16px",
      },

      transitionDuration: {
        instant: "0ms",
        fast: "100ms",
        normal: "150ms",
        slow: "300ms",
      },

      transitionTimingFunction: {
        smooth: "cubic-bezier(0.16, 1, 0.3, 1)",
      },

      animation: {
        "fade-in": "fadeIn 100ms ease-out",
        "fade-in-fast": "fadeIn 50ms ease-out",
        "slide-up": "slideUp 150ms cubic-bezier(0.16, 1, 0.3, 1)",
        "slide-in-right": "slideInRight 200ms cubic-bezier(0.16, 1, 0.3, 1)",
        "slide-in-left": "slideInLeft 200ms cubic-bezier(0.16, 1, 0.3, 1)",
        "scale-in": "scaleIn 150ms ease-out",
        skeleton: "skeleton 1.5s infinite",
      },

      keyframes: {
        fadeIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        slideUp: {
          "0%": { opacity: "0", transform: "translateY(8px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        slideInRight: {
          "0%": { opacity: "0", transform: "translateX(16px)" },
          "100%": { opacity: "1", transform: "translateX(0)" },
        },
        slideInLeft: {
          "0%": { opacity: "0", transform: "translateX(-16px)" },
          "100%": { opacity: "1", transform: "translateX(0)" },
        },
        scaleIn: {
          "0%": { opacity: "0", transform: "scale(0.95)" },
          "100%": { opacity: "1", transform: "scale(1)" },
        },
        skeleton: {
          "0%": { backgroundPosition: "200% 0" },
          "100%": { backgroundPosition: "-200% 0" },
        },
      },

      boxShadow: {
        sm: "0 1px 2px rgba(60, 64, 67, 0.1)",
        md: "0 2px 6px rgba(60, 64, 67, 0.15)",
        lg: "0 4px 12px rgba(60, 64, 67, 0.15)",
        xl: "0 8px 24px rgba(60, 64, 67, 0.2)",
        // Legacy alias
        elevated: "0 2px 6px rgba(60, 64, 67, 0.15)",
        card: "0 1px 2px rgba(60, 64, 67, 0.1)",
      },

      // Grid template for 3-panel layout
      gridTemplateColumns: {
        app: "240px minmax(300px, 400px) 1fr",
        "app-collapsed": "60px minmax(280px, 350px) 1fr",
      },

      gridTemplateRows: {
        app: "56px 1fr",
      },

      minWidth: {
        detail: "500px",
      },
    },
  },
  plugins: [],
};

export default config;
