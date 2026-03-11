"use client";

import type { CameraContext } from "@/lib/cameraContext";

interface ContextSelectorProps {
  onSelect: (context: CameraContext) => void;
}

const OPTIONS: { context: Exclude<CameraContext, "unknown">; label: string; sub: string }[] = [
  { context: "smartphone", label: "Mobile",       sub: "Handheld camera inspection" },
  { context: "cctv",       label: "Fixed Camera", sub: "CCTV or desktop monitoring"  },
  { context: "ar",         label: "AR Headset",   sub: "Spatial heads-up display"    },
];

export function ContextSelector({ onSelect }: ContextSelectorProps) {
  return (
    <div className="h-screen w-screen bg-black flex flex-col items-center justify-center">
      <div className="w-72">
        {/* Wordmark */}
        <div className="mb-12">
          <p className="font-display text-2xl font-bold tracking-[-0.02em] uppercase text-white">
            ARGUS
          </p>
          <p className="font-mono text-xs tracking-[0.25em] uppercase mt-1" style={{ color: "#4a4a4a" }}>
            Select Environment
          </p>
        </div>

        {/* Options */}
        <div>
          {OPTIONS.map(({ context, label, sub }, i) => (
            <button
              key={context}
              onClick={() => onSelect(context)}
              className="group w-full flex items-start justify-between py-5 text-left transition-colors duration-100"
              style={{
                borderTop: i === 0 ? "1px solid #1c1c1c" : "none",
                borderBottom: "1px solid #1c1c1c",
              }}
            >
              <div>
                <span
                  className="block font-display text-sm font-semibold uppercase tracking-wide transition-colors"
                  style={{ color: "#f0f0f0" }}
                >
                  {label}
                </span>
                <span className="block font-sans text-xs font-light mt-0.5" style={{ color: "#4a4a4a" }}>
                  {sub}
                </span>
              </div>
              <span
                className="font-mono text-base mt-0.5 transition-colors group-hover:translate-x-0.5 duration-100"
                style={{ color: "#1c1c1c" }}
              >
                →
              </span>
            </button>
          ))}
        </div>

        {/* Footer hint */}
        <p className="font-mono text-xs mt-8" style={{ color: "#2a2a2a" }}>
          Environment is detected automatically on load.
        </p>
      </div>
    </div>
  );
}
