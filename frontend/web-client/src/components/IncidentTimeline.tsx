"use client";

import { useState } from "react";
import type { GlassMode } from "@/components/HazardOverlay";
import type { Incident, Severity } from "@/lib/types";

interface IncidentTimelineProps {
  incidents: Incident[];
  visible: boolean;
  glassMode?: GlassMode;
}

const SEVERITY_COLOR: Record<Severity, string> = {
  low:      "#22c55e",
  medium:   "#f59e0b",
  high:     "#ef4444",
  critical: "#dc2626",
};

const LIFECYCLE_LABEL: Record<Incident["lifecycle_state"], string> = {
  detected:     "Detected",
  persistent:   "Persistent",
  escalated:    "Escalated",
  acknowledged: "Acknowledged",
  resolved:     "Resolved",
  recurring:    "Recurring",
};

const TREND_ICON: Record<string, string> = {
  escalating: "↑",
  stable:     "→",
  improving:  "↓",
  new:        "•",
};

function formatDuration(secs?: number): string {
  if (!secs || secs < 1) return "< 1s";
  if (secs < 60) return `${Math.round(secs)}s`;
  const m = Math.floor(secs / 60);
  const s = Math.round(secs % 60);
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

function formatTime(iso?: string): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return iso;
  }
}

function lllrBar(llr: number | undefined): { width: string; color: string } {
  if (llr === undefined) return { width: "0%", color: "#444" };
  // SPRT alarm at 2.89, scale bar to max of 6
  const pct = Math.min(100, Math.max(0, (llr / 6) * 100));
  const color = llr >= 2.89 ? "#22c55e" : llr >= 1.5 ? "#f59e0b" : "#6b7280";
  return { width: `${pct.toFixed(1)}%`, color };
}

export function IncidentTimeline({ incidents, visible, glassMode = "dark" }: IncidentTimelineProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  if (!visible || incidents.length === 0) return null;

  const dark = glassMode === "dark";

  const bg        = dark ? "rgba(10,10,10,0.72)"         : "rgba(255,255,255,0.72)";
  const border    = dark ? "1px solid rgba(255,255,255,0.09)" : "1px solid rgba(0,0,0,0.10)";
  const backdrop  = dark ? "blur(18px) saturate(1.1)"    : "blur(22px) saturate(1.2)";
  const textMeta  = dark ? "rgba(255,255,255,0.46)"      : "rgba(0,0,0,0.46)";
  const textMain  = dark ? "rgba(255,255,255,0.88)"      : "rgba(0,0,0,0.88)";
  const rowHover  = dark ? "rgba(255,255,255,0.04)"      : "rgba(0,0,0,0.04)";
  const divider   = dark ? "rgba(255,255,255,0.07)"      : "rgba(0,0,0,0.07)";

  return (
    <div
      className="absolute bottom-4 left-4 right-4 z-30 pointer-events-auto"
      style={{ maxWidth: 560, margin: "0 auto" }}
    >
      <div
        style={{
          background: bg,
          border,
          backdropFilter: backdrop,
          WebkitBackdropFilter: backdrop,
          borderRadius: 14,
          overflow: "hidden",
          boxShadow: dark
            ? "0 12px 40px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.05)"
            : "0 12px 32px rgba(0,0,0,0.12), inset 0 1px 0 rgba(255,255,255,0.6)",
        }}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between px-4 py-2.5"
          style={{ borderBottom: `1px solid ${divider}` }}
        >
          <span className="font-mono text-[10px] uppercase tracking-[0.18em]" style={{ color: textMeta }}>
            Incident Timeline
          </span>
          <span className="font-mono text-[10px]" style={{ color: textMeta }}>
            {incidents.length} active
          </span>
        </div>

        {/* Incident rows */}
        <div style={{ maxHeight: 320, overflowY: "auto" }}>
          {incidents.map((inc) => {
            const color   = SEVERITY_COLOR[inc.severity] ?? "#888";
            const expanded = expandedId === inc.incident_id;
            const bar     = lllrBar(inc.peak_llr);
            const trend   = TREND_ICON[inc.risk_trend ?? "new"] ?? "•";

            return (
              <button
                key={inc.incident_id}
                type="button"
                onClick={() => setExpandedId((cur) => cur === inc.incident_id ? null : inc.incident_id)}
                className="w-full text-left transition-colors"
                style={{
                  padding: "10px 16px",
                  borderBottom: `1px solid ${divider}`,
                  background: expanded ? rowHover : "transparent",
                }}
              >
                {/* Row summary */}
                <div className="flex items-center gap-2">
                  {/* Severity dot */}
                  <span
                    className="h-2 w-2 rounded-full shrink-0"
                    style={{ background: color, boxShadow: `0 0 6px ${color}55` }}
                  />
                  {/* Lifecycle badge */}
                  <span
                    className="font-mono text-[9px] uppercase tracking-[0.14em] px-1.5 py-0.5 rounded"
                    style={{
                      color,
                      background: `${color}18`,
                      border: `1px solid ${color}33`,
                    }}
                  >
                    {LIFECYCLE_LABEL[inc.lifecycle_state] ?? inc.lifecycle_state}
                  </span>
                  {/* Hazard type */}
                  <span className="font-mono text-[10px] truncate flex-1" style={{ color: textMain }}>
                    {inc.hazard_type.replace(/_/g, " ")}
                  </span>
                  {/* Trend + duration */}
                  <span className="font-mono text-[9px] shrink-0" style={{ color: textMeta }}>
                    {trend} {formatDuration(inc.duration_seconds)}
                  </span>
                </div>

                {/* SPRT LLR bar */}
                <div className="mt-1.5 flex items-center gap-2">
                  <span className="font-mono text-[8px] shrink-0" style={{ color: textMeta }}>
                    LLR
                  </span>
                  <div
                    className="flex-1 rounded-full overflow-hidden"
                    style={{ height: 3, background: dark ? "rgba(255,255,255,0.08)" : "rgba(0,0,0,0.08)" }}
                  >
                    <div
                      style={{
                        width: bar.width,
                        height: "100%",
                        background: bar.color,
                        borderRadius: "inherit",
                        transition: "width 0.4s ease",
                      }}
                    />
                  </div>
                  {inc.sprt_confirmed && (
                    <span className="font-mono text-[8px] shrink-0" style={{ color: "#22c55e" }}>
                      ✓ confirmed
                    </span>
                  )}
                </div>

                {/* Expanded detail */}
                {expanded && (
                  <div
                    className="mt-3 space-y-1"
                    style={{ borderTop: `1px solid ${divider}`, paddingTop: 10 }}
                  >
                    <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                      <Detail label="Started"     value={formatTime(inc.start_at)}    textMeta={textMeta} textMain={textMain} />
                      <Detail label="Last seen"   value={formatTime(inc.last_seen)}   textMeta={textMeta} textMain={textMain} />
                      <Detail label="Duration"    value={formatDuration(inc.duration_seconds)} textMeta={textMeta} textMain={textMain} />
                      <Detail label="Peak LLR"    value={inc.peak_llr != null ? inc.peak_llr.toFixed(2) : "—"} textMeta={textMeta} textMain={textMain} />
                      <Detail label="Trend"       value={inc.risk_trend ?? "new"}     textMeta={textMeta} textMain={textMain} />
                      <Detail label="Snapshots"   value={String(inc.snapshot_count ?? 0)} textMeta={textMeta} textMain={textMain} />
                    </div>
                    {inc.cameras && inc.cameras.length > 0 && (
                      <Detail label="Cameras" value={inc.cameras.join(", ")} textMeta={textMeta} textMain={textMain} />
                    )}
                    {inc.rules_triggered && inc.rules_triggered.length > 0 && (
                      <Detail label="Rules" value={inc.rules_triggered.join(", ")} textMeta={textMeta} textMain={textMain} />
                    )}
                  </div>
                )}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function Detail({
  label,
  value,
  textMeta,
  textMain,
}: {
  label: string;
  value: string;
  textMeta: string;
  textMain: string;
}) {
  return (
    <div className="flex gap-1.5">
      <span className="font-mono text-[9px] shrink-0 w-16" style={{ color: textMeta }}>
        {label}
      </span>
      <span className="font-mono text-[9px] truncate" style={{ color: textMain }}>
        {value}
      </span>
    </div>
  );
}
