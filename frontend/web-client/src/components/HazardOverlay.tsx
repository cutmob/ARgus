"use client";

import type { Overlay } from "@/lib/types";

interface HazardOverlayProps {
  overlays: Overlay[];
  visible: boolean;
}

const SEVERITY_COLOR: Record<string, string> = {
  low: "#22c55e",
  medium: "#f59e0b",
  high: "#ef4444",
  critical: "#dc2626",
};

const SEVERITY_BG: Record<string, string> = {
  low: "rgba(34,197,94,0.10)",
  medium: "rgba(245,158,11,0.10)",
  high: "rgba(239,68,68,0.12)",
  critical: "rgba(220,38,38,0.14)",
};

/**
 * Glassmorphic hazard overlays rendered as DOM elements over the video feed.
 *
 * Design principles (from Google Glimmer + SafeSpect research):
 * - Dark glassmorphism: backdrop-filter blur with low-opacity tinted fill
 * - Bold monospace labels for readability on transparent/camera backgrounds
 * - Traffic-light severity colors (green → amber → red)
 * - Corner brackets for spatial anchoring without occluding the scene
 * - Minimal text: severity + short label only
 *
 * BBox coordinates are normalized 0–1 (converted from Gemini's 0–1000 on the backend).
 */
export function HazardOverlay({ overlays, visible }: HazardOverlayProps) {
  if (!visible || overlays.length === 0) return null;

  return (
    <div className="absolute inset-0 pointer-events-none z-10">
      {overlays.map((overlay, i) => {
        if (!overlay.bbox) return null;
        const { x, y, width, height } = overlay.bbox;
        const color = SEVERITY_COLOR[overlay.severity] ?? "#FF5F1F";
        const bg = SEVERITY_BG[overlay.severity] ?? "rgba(255,95,31,0.10)";

        return (
          <div
            key={i}
            className="absolute"
            style={{
              left: `${x * 100}%`,
              top: `${y * 100}%`,
              width: `${width * 100}%`,
              height: `${height * 100}%`,
            }}
          >
            {/* Glass fill */}
            <div
              className="absolute inset-0"
              style={{
                background: bg,
                backdropFilter: "blur(2px)",
                WebkitBackdropFilter: "blur(2px)",
                border: `1px solid ${color}33`,
                borderRadius: 2,
              }}
            />

            {/* Corner brackets */}
            <Corner pos="tl" color={color} />
            <Corner pos="tr" color={color} />
            <Corner pos="bl" color={color} />
            <Corner pos="br" color={color} />

            {/* Label pill — above the box */}
            <div
              className="absolute flex items-center gap-1.5"
              style={{
                bottom: "calc(100% + 4px)",
                left: 0,
                padding: "2px 6px",
                background: "rgba(0,0,0,0.75)",
                backdropFilter: "blur(8px)",
                WebkitBackdropFilter: "blur(8px)",
                borderRadius: 2,
                border: `1px solid ${color}44`,
              }}
            >
              {/* Severity dot */}
              <div
                className="shrink-0"
                style={{
                  width: 5,
                  height: 5,
                  borderRadius: "50%",
                  background: color,
                  boxShadow: `0 0 4px ${color}88`,
                }}
              />
              <span
                className="font-mono text-[9px] font-bold tracking-[0.12em] uppercase whitespace-nowrap"
                style={{ color }}
              >
                {overlay.severity}
              </span>
              {overlay.label && (
                <span
                  className="font-mono text-[9px] tracking-wide whitespace-nowrap"
                  style={{ color: "rgba(255,255,255,0.6)" }}
                >
                  {overlay.label}
                </span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function Corner({ pos, color }: { pos: "tl" | "tr" | "bl" | "br"; color: string }) {
  const size = 10;
  const style: React.CSSProperties = {
    position: "absolute",
    width: size,
    height: size,
    pointerEvents: "none",
  };

  if (pos === "tl") {
    style.top = -1;
    style.left = -1;
    style.borderTop = `1.5px solid ${color}`;
    style.borderLeft = `1.5px solid ${color}`;
  } else if (pos === "tr") {
    style.top = -1;
    style.right = -1;
    style.borderTop = `1.5px solid ${color}`;
    style.borderRight = `1.5px solid ${color}`;
  } else if (pos === "bl") {
    style.bottom = -1;
    style.left = -1;
    style.borderBottom = `1.5px solid ${color}`;
    style.borderLeft = `1.5px solid ${color}`;
  } else {
    style.bottom = -1;
    style.right = -1;
    style.borderBottom = `1.5px solid ${color}`;
    style.borderRight = `1.5px solid ${color}`;
  }

  return <div style={style} />;
}
