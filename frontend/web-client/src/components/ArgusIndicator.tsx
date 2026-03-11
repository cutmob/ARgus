"use client";

export type IndicatorState = "idle" | "processing" | "speaking";

interface ArgusIndicatorProps {
  state: IndicatorState;
  className?: string;
}

export function ArgusIndicator({ state, className = "" }: ArgusIndicatorProps) {
  if (state === "idle") return null;

  if (state === "processing") {
    return (
      <svg
        width="11"
        height="11"
        viewBox="0 0 12 12"
        className={`animate-arc-spin shrink-0 ${className}`}
      >
        <circle cx="6" cy="6" r="5" fill="none" stroke="#1c1c1c" strokeWidth="1.5" />
        <path
          d="M 6 1 A 5 5 0 0 1 11 6"
          fill="none"
          stroke="#FF5F1F"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
      </svg>
    );
  }

  return (
    <div className={`flex items-end gap-px shrink-0 ${className}`} style={{ height: 9 }}>
      <div className="tts-bar tts-bar-1" />
      <div className="tts-bar tts-bar-2" />
      <div className="tts-bar tts-bar-3" />
    </div>
  );
}
