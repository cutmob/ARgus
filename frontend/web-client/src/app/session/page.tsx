"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { useArgusSession } from "@/hooks/useArgusSession";
import { useCameraContext } from "@/hooks/useCameraContext";
import { useLiveAudioInput } from "@/hooks/useLiveAudioInput";
import { useWakeWord } from "@/hooks/useWakeWord";
import { speakResponse } from "@/lib/tts";
import { resolveVoiceIntent } from "@/lib/voiceIntent";
import { INSPECTION_MODES, modeLabel } from "@/lib/modes";
import { ContextSelector } from "@/components/ContextSelector";
import { DemoGate } from "@/components/DemoGate";
import { SmartphoneSession } from "@/components/session-smartphone/SmartphoneSession";
import { CCTVSession } from "@/components/session-cctv/CCTVSession";
import { ARSession } from "@/components/session-ar/ARSession";
import type { GlassMode } from "@/components/HazardOverlay";
import type { CameraContext } from "@/lib/cameraContext";
import type { AlertThreshold } from "@/lib/types";
import { IncidentTimeline } from "@/components/IncidentTimeline";

const ALERT_THRESHOLD_OPTIONS: AlertThreshold[] = ["off", "low", "medium", "high", "critical"];
const ALERT_THRESHOLD_LABELS: Record<AlertThreshold, string> = {
  off: "Voice Off",
  low: "Low+",
  medium: "Medium+",
  high: "High+",
  critical: "Critical",
};
const ALERT_THRESHOLD_HELPERS: Record<AlertThreshold, string> = {
  off: "Never speak findings automatically",
  low: "Speaks any visible issue worth noting",
  medium: "Speaks clear concerns needing follow-up",
  high: "Speaks serious hazards needing prompt action",
  critical: "Speaks only imminent life-safety danger",
};

export default function SessionPage() {
  const [inspectionMode, setInspectionMode] = useState("construction");
  const [reportFormat, setReportFormat] = useState("pdf");
  const [overlaysVisible, setOverlaysVisible] = useState(true);
  const [gated, setGated] = useState(true);
  const [controlsOpen, setControlsOpen] = useState(false);
  const [openSelect, setOpenSelect] = useState<"context" | "mode" | "format" | "threshold" | null>(null);
  const [reportViewOpen, setReportViewOpen] = useState(false);
  const [reportTile, setReportTile] = useState<{ text: string; at: number } | null>(null);
  const [latestReport, setLatestReport] = useState<{ text: string; reportId?: string; at: number } | null>(null);
  const [cctvVoiceInputEnabled, setCctvVoiceInputEnabled] = useState(false);
  const [cctvVoiceOutputEnabled, setCctvVoiceOutputEnabled] = useState(false);
  const [arVoiceInputEnabled, setArVoiceInputEnabled] = useState(true);
  const [alertThreshold, setAlertThreshold] = useState<AlertThreshold>("high");
  const [glassMode, setGlassMode] = useState<GlassMode>("dark");
  const [demoContext, setDemoContext] = useState<CameraContext>("cctv");
  const [videoSource, setVideoSource] = useState<string | null>(null);
  const [videoName, setVideoName] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [manualContext, setManualContext] = useState<CameraContext | null>(null);
  const handledUiTranscriptAtRef = useRef(0);

  useEffect(() => {
    setGated(!localStorage.getItem("argus_demo_token"));
    const savedMode = localStorage.getItem("argus_mode");
    const savedContext = localStorage.getItem("argus_context") as CameraContext | null;
    const savedFormat = localStorage.getItem("argus_report_format");
    const savedAlertThreshold = localStorage.getItem("argus_alert_threshold");
    const savedGlassMode = localStorage.getItem("argus_glass_mode");
    const savedCctvVoiceInput = localStorage.getItem("argus_cctv_voice_input");
    const savedCctvVoiceOutput = localStorage.getItem("argus_cctv_voice_output");
    const savedArVoiceInput = localStorage.getItem("argus_ar_voice_input");
    if (savedMode) setInspectionMode(savedMode);
    if (savedContext) {
      setDemoContext(savedContext);
      setManualContext(savedContext);
    }
    if (savedFormat) setReportFormat(savedFormat);
    if (
      savedAlertThreshold === "off" ||
      savedAlertThreshold === "low" ||
      savedAlertThreshold === "medium" ||
      savedAlertThreshold === "high" ||
      savedAlertThreshold === "critical"
    ) {
      setAlertThreshold(savedAlertThreshold);
    }
    if (savedGlassMode === "dark" || savedGlassMode === "light") setGlassMode(savedGlassMode);
    if (savedCctvVoiceInput) setCctvVoiceInputEnabled(savedCctvVoiceInput === "true");
    if (savedCctvVoiceOutput) setCctvVoiceOutputEnabled(savedCctvVoiceOutput === "true");
    if (savedArVoiceInput) setArVoiceInputEnabled(savedArVoiceInput === "true");
  }, []);
  const session = useArgusSession();
  const { context, detecting } = useCameraContext();

  const handleWake = useCallback(() => {
    session.interruptSpeech();
    if (!session.isInspecting) {
      session.startInspection(inspectionMode);
      speakResponse("On it.");
    }
  }, [session, inspectionMode]);

  const wakeWordEnabled = !session.isInspecting && (manualContext ?? context) === "cctv";
  useWakeWord({ onWake: handleWake, word: "argus", enabled: wakeWordEnabled });

  // Toggle overlays with "O" key (non-AR modes)
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.target instanceof HTMLInputElement) return;
      if (e.key === "o") setOverlaysVisible((v) => !v);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    return () => {
      if (videoSource) URL.revokeObjectURL(videoSource);
    };
  }, [videoSource]);

  useEffect(() => {
    if (!controlsOpen) setOpenSelect(null);
  }, [controlsOpen]);

  useEffect(() => {
    if (!session.lastAgentResponse) return;
    const r = session.lastAgentResponse;
    const text = r.text.trim();
    const reportLike =
      r.type === "report" ||
      !!r.reportId ||
      text.toLowerCase().includes("report generated") ||
      text.toLowerCase().includes("report ready");
    if (!reportLike) return;

    const nextReport = {
      text: text || `Report generated (${reportFormat.toUpperCase()}).`,
      reportId: r.reportId,
      at: r.at,
    };
    setLatestReport(nextReport);
    setReportTile({ text: `Report ready • ${reportFormat.toUpperCase()}`, at: Date.now() });

    const timeout = setTimeout(() => {
      setReportTile((prev) => (prev && Date.now() - prev.at >= 4900 ? null : prev));
    }, 5000);
    return () => clearTimeout(timeout);
  }, [session.lastAgentResponse, reportFormat]);

  const activeContext = manualContext ?? context;
  const liveAudioEnabled =
    (activeContext === "ar" && arVoiceInputEnabled) ||
    activeContext === "smartphone" ||
    (activeContext === "cctv" && cctvVoiceInputEnabled);
  const { active: liveAudioActive, supported: liveAudioSupported } = useLiveAudioInput({
    enabled: liveAudioEnabled,
    onChunk: session.sendAudio,
  });

  useEffect(() => {
    if (activeContext === "cctv") {
      session.setVoiceOutputEnabled(cctvVoiceOutputEnabled);
    } else {
      session.setVoiceOutputEnabled(true);
    }
  }, [activeContext, cctvVoiceOutputEnabled, session]);

  useEffect(() => {
    session.setAlertThreshold(alertThreshold);
    localStorage.setItem("argus_alert_threshold", alertThreshold);
  }, [alertThreshold, session.setAlertThreshold]);

  if (gated || session.unauthorized) {
    return (
      <DemoGate
        onAccess={() => {
          setGated(false);
          if (session.unauthorized) session.resetAuth();
        }}
      />
    );
  }

  if (detecting) {
    return (
      <div className="h-screen w-screen bg-argus-bg flex items-center justify-center">
        <span className="font-display text-xs font-medium text-argus-muted tracking-[0.35em] uppercase">
          Detecting environment
        </span>
      </div>
    );
  }

  if (activeContext === "unknown") {
    return <ContextSelector onSelect={setManualContext} />;
  }

  const handleModeChange = (m: string) => {
    setInspectionMode(m);
    localStorage.setItem("argus_mode", m);
    session.switchMode(m);
  };

  const handleVideoPick = (file: File | null) => {
    if (!file) return;
    if (videoSource) URL.revokeObjectURL(videoSource);
    const url = URL.createObjectURL(file);
    setVideoSource(url);
    setVideoName(file.name);
  };

  const startDemo = () => {
    setManualContext(demoContext);
    localStorage.setItem("argus_context", demoContext);
    if (!session.isInspecting) session.startInspection(inspectionMode);
  };

  const stopDemo = () => {
    if (session.isInspecting) session.stopInspection();
  };

  const setSharedGlassMode = (next: GlassMode) => {
    setGlassMode(next);
    localStorage.setItem("argus_glass_mode", next);
  };

  useEffect(() => {
    const transcript = session.lastTranscript;
    if (!transcript || transcript.speaker !== "user") return;
    if (transcript.at <= handledUiTranscriptAtRef.current) return;
    handledUiTranscriptAtRef.current = transcript.at;

    const intent = resolveVoiceIntent(transcript.text);
    switch (intent.type) {
      case "open_report":
        setReportViewOpen(true);
        setReportTile(null);
        break;
      case "close_report":
        setReportViewOpen(false);
        break;
      case "toggle_overlays":
        setOverlaysVisible((v) => !v);
        break;
      case "show_incidents":
        setOverlaysVisible(true);
        break;
      case "hide_incidents":
        setOverlaysVisible(false);
        break;
      case "set_glass_light":
        setSharedGlassMode("light");
        break;
      case "set_glass_dark":
        setSharedGlassMode("dark");
        break;
      case "mute_voice":
        if (activeContext === "cctv") {
          setCctvVoiceOutputEnabled(false);
          localStorage.setItem("argus_cctv_voice_output", "false");
        }
        session.setVoiceOutputEnabled(false);
        break;
      case "unmute_voice":
        if (activeContext === "cctv") {
          setCctvVoiceOutputEnabled(true);
          localStorage.setItem("argus_cctv_voice_output", "true");
        }
        session.setVoiceOutputEnabled(true);
        break;
      default:
        break;
    }
  }, [activeContext, session, setSharedGlassMode]);

  const sessionWithFormat = {
    ...session,
    generateReport: () => session.generateReport(reportFormat),
  };

  const controlAnchorClass =
    activeContext === "cctv" ? "top-1 left-1/2 -translate-x-1/2" : "top-14 right-4";
  const controlInnerJustify = activeContext === "cctv" ? "justify-center" : "justify-end";
  const reportPanelTop = controlsOpen ? "11.25rem" : activeContext === "cctv" ? "6rem" : "7rem";

  const controlLauncher = (
    <div className={`fixed z-50 ${controlAnchorClass}`}>
      <input
        ref={fileInputRef}
        type="file"
        accept="video/mp4,video/*"
        className="hidden"
        onChange={(e) => handleVideoPick(e.target.files?.[0] ?? null)}
      />

      <div className={`flex items-center gap-2 ${controlInnerJustify}`}>
        <button
          onClick={() => (session.isInspecting ? stopDemo() : startDemo())}
          className="liquid-glass liquid-float liquid-pill liquid-enter h-8 px-3 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
        >
          {session.isInspecting ? "Stop" : "Start"}
        </button>
        <button
          onClick={() => setControlsOpen((v) => !v)}
          className="liquid-glass liquid-float liquid-pill liquid-enter min-w-[14rem] h-8 px-3 font-mono text-[10px] tracking-[0.14em] uppercase liquid-meta flex items-center justify-between gap-2"
        >
          <span className="truncate">{demoContext} • {modeLabel(inspectionMode)}</span>
          <span>▾</span>
        </button>
      </div>

      {controlsOpen && (
        <div className="liquid-glass liquid-float liquid-enter mt-2 w-[24rem] max-w-[calc(100vw-1rem)] rounded-2xl p-3 space-y-2.5">
          <div className="grid grid-cols-2 gap-2">
            <div className="relative">
              <button
                onClick={() => setOpenSelect((v) => (v === "context" ? null : "context"))}
                className="liquid-glass liquid-float liquid-pill h-9 w-full px-3 font-mono text-[10px] uppercase liquid-title flex items-center justify-between gap-2"
              >
                <span className="truncate">{demoContext === "smartphone" ? "mobile" : demoContext}</span>
                <span className="liquid-meta">▾</span>
              </button>
              {openSelect === "context" && (
                <div className="absolute left-0 right-0 top-[calc(100%+0.35rem)] z-10 min-w-full liquid-glass liquid-float liquid-enter liquid-menu liquid-menu-popover">
                  {(["cctv", "ar", "smartphone"] as CameraContext[]).map((ctx) => (
                    <button
                      key={ctx}
                      onClick={() => {
                        setDemoContext(ctx);
                        setManualContext(ctx);
                        setOpenSelect(null);
                      }}
                      className={`liquid-menu-item liquid-menu-item-wide font-mono text-[10px] uppercase ${
                        demoContext === ctx ? "liquid-menu-item-active" : "liquid-meta"
                      }`}
                    >
                      {ctx === "smartphone" ? "mobile" : ctx}
                    </button>
                  ))}
                </div>
              )}
            </div>
            <div className="relative">
              <button
                onClick={() => setOpenSelect((v) => (v === "mode" ? null : "mode"))}
                className="liquid-glass liquid-float liquid-pill h-9 w-full px-3 font-mono text-[10px] uppercase liquid-title flex items-center justify-between gap-2"
              >
                <span className="truncate">{modeLabel(inspectionMode)}</span>
                <span className="liquid-meta">▾</span>
              </button>
              {openSelect === "mode" && (
                <div className="absolute left-0 right-0 top-[calc(100%+0.35rem)] z-10 min-w-full max-h-52 overflow-auto liquid-glass liquid-float liquid-enter liquid-menu liquid-menu-popover">
                  {INSPECTION_MODES.map((m) => (
                    <button
                      key={m}
                      onClick={() => {
                        handleModeChange(m);
                        setOpenSelect(null);
                      }}
                      className={`liquid-menu-item liquid-menu-item-wide font-mono text-[10px] uppercase ${
                        inspectionMode === m ? "liquid-menu-item-active" : "liquid-meta"
                      }`}
                    >
                      {modeLabel(m)}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="grid grid-cols-[minmax(0,1.15fr)_minmax(0,1.4fr)_auto_auto_auto] gap-2">
            <div className="relative">
              <button
                onClick={() => setOpenSelect((v) => (v === "format" ? null : "format"))}
                className="liquid-glass liquid-float liquid-pill h-9 w-full px-3 font-mono text-[10px] uppercase liquid-title flex items-center justify-between gap-2"
              >
                <span>{reportFormat}</span>
                <span className="liquid-meta">▾</span>
              </button>
              {openSelect === "format" && (
                <div className="absolute left-0 right-0 top-[calc(100%+0.35rem)] z-10 min-w-full liquid-glass liquid-float liquid-enter liquid-menu liquid-menu-popover">
                  {["pdf", "word", "txt", "json", "csv", "html", "webhook"].map((f) => (
                    <button
                      key={f}
                      onClick={() => {
                        setReportFormat(f);
                        localStorage.setItem("argus_report_format", f);
                        setOpenSelect(null);
                      }}
                      className={`liquid-menu-item liquid-menu-item-wide font-mono text-[10px] uppercase ${
                        reportFormat === f ? "liquid-menu-item-active" : "liquid-meta"
                      }`}
                    >
                      {f}
                    </button>
                  ))}
                </div>
              )}
            </div>
            <div className="relative">
              <button
                onClick={() => setOpenSelect((v) => (v === "threshold" ? null : "threshold"))}
                className="liquid-glass liquid-float liquid-pill h-9 w-full px-3 font-mono text-[10px] uppercase liquid-title flex items-center justify-between gap-2"
              >
                <span className="truncate">{ALERT_THRESHOLD_LABELS[alertThreshold]}</span>
                <span className="liquid-meta">▾</span>
              </button>
              {openSelect === "threshold" && (
                <div className="absolute left-0 right-0 top-[calc(100%+0.35rem)] z-10 min-w-[18rem] liquid-glass liquid-float liquid-enter liquid-menu liquid-menu-popover">
                  {ALERT_THRESHOLD_OPTIONS.map((threshold) => (
                    <button
                      key={threshold}
                      onClick={() => {
                        setAlertThreshold(threshold);
                        setOpenSelect(null);
                      }}
                      className={`liquid-menu-item liquid-menu-item-wide flex items-start justify-between gap-3 font-mono text-[10px] uppercase ${
                        alertThreshold === threshold ? "liquid-menu-item-active" : "liquid-meta"
                      }`}
                    >
                      <span>{ALERT_THRESHOLD_LABELS[threshold]}</span>
                      <span className="normal-case tracking-normal text-right text-[10px] opacity-80">
                        {ALERT_THRESHOLD_HELPERS[threshold]}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
            <button
              onClick={() => fileInputRef.current?.click()}
              className="liquid-glass liquid-float liquid-pill h-9 px-3 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
              title={videoName || "Upload demo MP4"}
            >
              Upload
            </button>
            <button
              onClick={() => setSharedGlassMode(glassMode === "dark" ? "light" : "dark")}
              className="liquid-glass liquid-float liquid-pill h-9 px-3 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
              title="Toggle glass theme"
            >
              {glassMode === "dark" ? "Dark" : "Light"}
            </button>
            <button
              onClick={() => {
                setReportViewOpen(true);
                setControlsOpen(false);
              }}
              className="liquid-glass liquid-float liquid-pill h-9 px-3 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
            >
              Report
            </button>
          </div>
        </div>
      )}
    </div>
  );

  const reportTileView = reportTile && (
    <button
      onClick={() => {
        setReportViewOpen(true);
        setReportTile(null);
        setControlsOpen(false);
      }}
      className="fixed bottom-5 right-4 z-50 liquid-glass liquid-float liquid-pill liquid-enter px-3 py-1.5 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
      title="Open report view"
    >
      {reportTile.text}
    </button>
  );

  const reportPanelView = reportViewOpen && (
    <div
      className="fixed right-4 z-50 w-80 max-w-[calc(100vw-2rem)] liquid-glass liquid-float liquid-enter rounded-2xl p-3"
      style={{ top: reportPanelTop }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="font-mono text-[10px] tracking-[0.16em] uppercase liquid-meta">Report View</p>
          <p className="font-mono text-[10px] tracking-[0.14em] uppercase liquid-meta mt-1">
            Format {reportFormat.toUpperCase()}
          </p>
        </div>
        <button
          onClick={() => setReportViewOpen(false)}
          className="liquid-glass liquid-float liquid-pill px-2 py-1 font-mono text-[10px] tracking-[0.14em] uppercase liquid-title"
        >
          Dismiss
        </button>
      </div>
      <div className="mt-2 liquid-glass liquid-float rounded-xl p-2.5">
        {latestReport ? (
          <>
            <p className="font-sans text-xs leading-relaxed liquid-title">
              {latestReport.text || "Report generated and ready."}
            </p>
            {latestReport.reportId && (
              <p className="font-mono text-[10px] mt-1 liquid-meta">ID {latestReport.reportId}</p>
            )}
          </>
        ) : (
          <p className="font-sans text-xs leading-relaxed liquid-meta">
            No report yet. Say “generate report”, then “open report view”.
          </p>
        )}
      </div>
    </div>
  );

  const incidentTimeline = (
    <div className="fixed inset-0 pointer-events-none z-30">
      <div className="relative h-full w-full">
        <IncidentTimeline
          incidents={session.incidents}
          visible={overlaysVisible}
          glassMode={glassMode}
        />
      </div>
    </div>
  );

  if (activeContext === "smartphone") {
    return (
      <>
        {controlLauncher}
        {reportTileView}
        {reportPanelView}
        {incidentTimeline}
        <SmartphoneSession
          session={sessionWithFormat}
          mode={inspectionMode}
          onModeChange={handleModeChange}
          overlaysVisible={overlaysVisible}
          videoSource={videoSource}
          glassMode={glassMode}
          onGlassModeChange={setSharedGlassMode}
        />
      </>
    );
  }

  if (activeContext === "ar") {
    return (
      <>
        {controlLauncher}
        {reportTileView}
        {reportPanelView}
        {incidentTimeline}
        <ARSession
          session={sessionWithFormat}
          mode={inspectionMode}
          onModeChange={handleModeChange}
          videoSource={videoSource}
          audioInputEnabled={arVoiceInputEnabled}
          audioInputActive={liveAudioActive && activeContext === "ar"}
          audioInputSupported={liveAudioSupported}
          onAudioInputChange={(enabled: boolean) => {
            setArVoiceInputEnabled(enabled);
            localStorage.setItem("argus_ar_voice_input", String(enabled));
          }}
          glassMode={glassMode}
          onGlassModeChange={setSharedGlassMode}
          onOpenReportView={() => {
            setReportViewOpen(true);
            setReportTile(null);
          }}
          onCloseReportView={() => setReportViewOpen(false)}
        />
      </>
    );
  }

  // Default: CCTV / desktop
  return (
    <>
      {controlLauncher}
      {reportTileView}
      {reportPanelView}
      {incidentTimeline}
      <CCTVSession
        session={sessionWithFormat}
        mode={inspectionMode}
        onModeChange={handleModeChange}
        overlaysVisible={overlaysVisible}
        videoSource={videoSource}
        glassMode={glassMode}
        onGlassModeChange={setSharedGlassMode}
        voiceInputEnabled={cctvVoiceInputEnabled}
        voiceInputSupported={liveAudioSupported}
        voiceOutputEnabled={cctvVoiceOutputEnabled}
        onVoiceInputChange={(enabled: boolean) => {
          setCctvVoiceInputEnabled(enabled);
          localStorage.setItem("argus_cctv_voice_input", String(enabled));
        }}
        onVoiceOutputChange={(enabled: boolean) => {
          setCctvVoiceOutputEnabled(enabled);
          localStorage.setItem("argus_cctv_voice_output", String(enabled));
        }}
      />
    </>
  );
}
