"use client";

import type { Overlay } from "@/lib/types";

interface RingOverlayProps {
  overlays: Overlay[];
  containerWidth: number;
  containerHeight: number;
}

const SEVERITY_COLOR: Record<string, string> = {
  low: "#22c55e",
  medium: "#f59e0b",
  high: "#ef4444",
  critical: "#dc2626",
};

export function RingOverlay({ overlays, containerWidth, containerHeight }: RingOverlayProps) {
  if (overlays.length === 0) return null;

  return (
    <svg
      className="absolute inset-0 pointer-events-none"
      width={containerWidth}
      height={containerHeight}
      viewBox={`0 0 ${containerWidth} ${containerHeight}`}
    >
      {overlays.map((overlay, i) => {
        if (!overlay.bbox) return null;
        const { x, y, width: bw, height: bh } = overlay.bbox;
        const cx = (x + bw / 2) * containerWidth;
        const cy = (y + bh / 2) * containerHeight;
        const r = (Math.max(bw, bh) / 2) * Math.min(containerWidth, containerHeight);
        const color = SEVERITY_COLOR[overlay.severity] ?? "#FF5F1F";

        return (
          <g key={i}>
            <circle
              cx={cx}
              cy={cy}
              r={r}
              fill="none"
              stroke={color}
              strokeWidth={1.5}
              opacity={0.7}
            />
            <circle
              cx={cx}
              cy={cy}
              r={r + 8}
              fill="none"
              stroke={color}
              strokeWidth={0.5}
              opacity={0.25}
            />
            <text
              x={cx}
              y={cy - r - 10}
              textAnchor="middle"
              fill={color}
              fontSize={10}
              fontFamily="var(--font-ibm-plex-mono), monospace"
              letterSpacing={2}
              opacity={0.9}
            >
              {overlay.label.toUpperCase()}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
