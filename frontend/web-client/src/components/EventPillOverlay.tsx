"use client";

import { useMemo, useState } from "react";
import type { GlassMode } from "@/components/HazardOverlay";
import type { Hazard, Overlay, Severity } from "@/lib/types";

interface EventPillOverlayProps {
  hazards: Hazard[];
  overlays: Overlay[];
  visible: boolean;
  glassMode?: GlassMode;
  interactive?: boolean;
  expandMode?: "click" | "tap" | "none";
  placementMode?: "follow" | "stack-top-left";
  maxItems?: number;
}

const SEVERITY_COLOR: Record<Severity, string> = {
  low: "#22c55e",
  medium: "#f59e0b",
  high: "#ef4444",
  critical: "#dc2626",
};

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function buildPillPosition(
  hazard: Hazard,
  overlay: Overlay | undefined,
  index: number,
  placementMode: "follow" | "stack-top-left"
) {
  if (placementMode === "stack-top-left") {
    return {
      left: "16px",
      top: `${56 + index * 58}px`,
    };
  }
  const bbox = overlay?.bbox ?? hazard.bbox;
  if (bbox) {
    return {
      left: `${clamp((bbox.x + bbox.width + 0.012) * 100, 6, 82)}%`,
      top: `${clamp((bbox.y + Math.min(bbox.height * 0.18, 0.08)) * 100, 6, 86)}%`,
    };
  }

  return {
    right: "16px",
    top: `${16 + index * 62}px`,
  };
}

export function EventPillOverlay({
  hazards,
  overlays,
  visible,
  glassMode = "dark",
  interactive = true,
  expandMode = "click",
  placementMode = "follow",
  maxItems = 3,
}: EventPillOverlayProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const items = useMemo(() => {
    return hazards.slice(0, maxItems).map((hazard, index) => {
      const overlay = overlays.find((item) => item.label === hazard.description || item.bbox === hazard.bbox);
      return {
        hazard,
        index,
        overlay,
        position: buildPillPosition(hazard, overlay, index, placementMode),
      };
    });
  }, [hazards, overlays, maxItems, placementMode]);

  if (!visible || items.length === 0) return null;

  const dark = glassMode === "dark";

  return (
    <div className="absolute inset-0 z-20 pointer-events-none">
      {items.map(({ hazard, position }) => {
        const expanded = expandedId === hazard.id;
        const color = SEVERITY_COLOR[hazard.severity] ?? "#FF5F1F";
        return (
          <div
            key={hazard.id}
            className="absolute max-w-[min(18rem,calc(100vw-2rem))]"
            style={position}
          >
            <button
              type="button"
              onClick={() => {
                if (!interactive || expandMode === "none") return;
                setExpandedId((current) => (current === hazard.id ? null : hazard.id));
              }}
              className="pointer-events-auto liquid-glass liquid-float liquid-pill liquid-enter px-3 py-2 text-left"
              style={{
                background: dark ? "rgba(14,14,14,0.34)" : "rgba(255,255,255,0.26)",
                border: dark ? "1px solid rgba(255,255,255,0.08)" : "1px solid rgba(255,255,255,0.34)",
                backdropFilter: dark ? "blur(14px) saturate(1.08)" : "blur(18px) saturate(1.16)",
                WebkitBackdropFilter: dark ? "blur(14px) saturate(1.08)" : "blur(18px) saturate(1.16)",
                boxShadow: dark
                  ? "0 8px 20px rgba(0,0,0,0.14), inset 0 1px 0 rgba(255,255,255,0.04)"
                  : "0 8px 18px rgba(0,0,0,0.08), inset 0 1px 0 rgba(255,255,255,0.52)",
                minWidth: 164,
                opacity: expanded ? 0.98 : 0.88,
              }}
              title={
                expandMode === "tap"
                  ? "Tap to expand"
                  : expandMode === "click"
                  ? "Click to expand"
                  : undefined
              }
            >
              <div className="flex items-center gap-2">
                <span
                  className="h-1.5 w-1.5 rounded-full shrink-0"
                  style={{ background: color, boxShadow: `0 0 6px ${color}44` }}
                />
                <span
                  className="font-mono text-[9px] uppercase tracking-[0.16em]"
                  style={{ color: dark ? `${color}cc` : `${color}bb` }}
                >
                  {hazard.severity}
                </span>
                <span className="font-mono text-[9px] uppercase tracking-[0.14em] liquid-meta ml-auto opacity-75">
                  {Math.round(hazard.confidence * 100)}%
                </span>
              </div>
              <p className="font-sans text-[11px] mt-1 leading-relaxed liquid-title line-clamp-2">
                {hazard.description}
              </p>
              {expanded && (
                <div
                  className="mt-2 pt-2 flex items-start gap-2"
                  style={{ borderTop: dark ? "1px solid rgba(255,255,255,0.06)" : "1px solid rgba(255,255,255,0.24)" }}
                >
                  <div className="relative flex flex-col items-center mt-0.5">
                    <span
                      className="w-0.5 h-8 rounded-full"
                      style={{ background: dark ? "rgba(255,255,255,0.08)" : "rgba(0,0,0,0.12)" }}
                    />
                    <span
                      className="w-1 h-1 rounded-full absolute -top-0.5"
                      style={{ background: color }}
                    />
                    <span
                      className="w-1 h-1 rounded-full absolute -bottom-0.5"
                      style={{ background: color }}
                    />
                  </div>
                  <div className="space-y-0.5">
                    <p className="font-mono text-[10px] leading-relaxed liquid-meta opacity-80">
                      {(hazard.rule_id || "rule:n/a") + " • " + (hazard.camera_id || "camera:n/a")}
                    </p>
                    <p className="font-mono text-[10px] leading-relaxed liquid-meta opacity-80">
                      {(hazard.persistence_seconds ?? 0) + "s • " + (hazard.risk_trend || "new")}
                    </p>
                  </div>
                </div>
              )}
            </button>
          </div>
        );
      })}
    </div>
  );
}
