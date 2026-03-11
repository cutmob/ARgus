import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{js,ts,jsx,tsx,mdx}"],
  theme: {
    extend: {
      fontFamily: {
        display: ["var(--font-space-grotesk)", "sans-serif"],
        sans:    ["var(--font-figtree)", "sans-serif"],
        mono:    ["var(--font-ibm-plex-mono)", "monospace"],
      },
      colors: {
        argus: {
          bg:      "#000000",
          surface: "#080808",
          panel:   "#0f0f0f",
          border:  "#1c1c1c",
          muted:   "#4a4a4a",
          dim:     "#7a7a7a",
          text:    "#f0f0f0",
          orange:  "#FF5F1F",  // primary accent — matches landing
          danger:  "#ef4444",
          safe:    "#22c55e",
          amber:   "#f59e0b",
        },
      },
      keyframes: {
        "arc-spin": {
          "0%":   { transform: "rotate(0deg)" },
          "100%": { transform: "rotate(360deg)" },
        },
      },
      animation: {
        "arc-spin": "arc-spin 1s linear infinite",
      },
    },
  },
  plugins: [],
};

export default config;
