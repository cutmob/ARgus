"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { useVoiceCommands } from "@/hooks/useVoiceCommands";
import { speakResponse } from "@/lib/tts";
import { ArgusIndicator } from "@/components/ArgusIndicator";
import { RingOverlay } from "./RingOverlay";
import type { Hazard, Overlay } from "@/lib/types";

interface ARSessionProps {
  session: {
    connected: boolean;
    isInspecting: boolean;
    hazards: Hazard[];
    overlays: Overlay[];
    riskLevel: string;
    processing: boolean;
    speaking: boolean;
    sendFrame: (blob: Blob) => void;
    startInspection: (mode: string) => void;
    stopInspection: () => void;
    generateReport: () => void;
  };
  mode: string;
}

const RISK_COLOR: Record<string, string> = {
  low:      "#22c55e",
  medium:   "#f59e0b",
  high:     "#ef4444",
  critical: "#ef4444",
};

export function ARSession({ session, mode }: ARSessionProps) {
  const videoRef     = useRef<HTMLVideoElement>(null);
  const canvasRef    = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [voiceEnabled, setVoiceEnabled] = useState(false);
  const [dims, setDims] = useState({ w: 1280, h: 720 });

  const indicatorState = session.speaking
    ? "speaking"
    : session.processing
    ? "processing"
    : "idle";

  useEffect(() => {
    let stream: MediaStream;
    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: "environment" }, audio: false })
      .then((s) => {
        stream = s;
        if (!videoRef.current) return;
        videoRef.current.srcObject = s;
        videoRef.current.onloadedmetadata = () => {
          if (!videoRef.current) return;
          setDims({
            w: videoRef.current.videoWidth  || window.innerWidth,
            h: videoRef.current.videoHeight || window.innerHeight,
          });
        };
      })
      .catch(() => {});
    return () => stream?.getTracks().forEach((t) => t.stop());
  }, []);

  useEffect(() => {
    function onResize() {
      if (containerRef.current) {
        setDims({ w: containerRef.current.offsetWidth, h: containerRef.current.offsetHeight });
      }
    }
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  useEffect(() => {
    if (!session.isInspecting) return;
    const id = setInterval(() => {
      const video  = videoRef.current;
      const canvas = canvasRef.current;
      if (!canvas || !video || video.readyState < 2) return;
      canvas.width  = video.videoWidth;
      canvas.height = video.videoHeight;
      canvas.getContext("2d")?.drawImage(video, 0, 0);
      canvas.toBlob((blob) => blob && session.sendFrame(blob), "image/jpeg", 0.7);
    }, 500);
    return () => clearInterval(id);
  }, [session.isInspecting, session.sendFrame]);

  const handleVoiceCommand = useCallback(
    (transcript: string) => {
      if (transcript.includes("inspect") || transcript.includes("start")) {
        session.startInspection(mode);
        speakResponse("Inspection started");
      } else if (transcript.includes("stop")) {
        session.stopInspection();
        speakResponse("Inspection stopped");
      } else if (transcript.includes("report")) {
        session.generateReport();
        speakResponse("Generating report");
      } else if (transcript.includes("status")) {
        speakResponse(
          `${session.hazards.length} hazard${session.hazards.length !== 1 ? "s" : ""} detected. Risk level ${session.riskLevel}.`
        );
      }
    },
    [session, mode]
  );

  useVoiceCommands({ onCommand: handleVoiceCommand, enabled: voiceEnabled });

  const riskColor = RISK_COLOR[session.riskLevel] ?? "#4a4a4a";

  return (
    <div ref={containerRef} className="h-screen w-screen bg-black relative overflow-hidden">
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="absolute inset-0 w-full h-full object-cover"
      />
      <canvas ref={canvasRef} className="hidden" />

      <RingOverlay
        overlays={session.overlays}
        containerWidth={dims.w}
        containerHeight={dims.h}
      />

      {/* Top-left */}
      <div className="absolute top-5 left-5 z-20 flex items-center gap-2.5">
        <span className="font-display text-xs font-bold tracking-[0.25em] uppercase" style={{ color: "rgba(255,255,255,0.35)" }}>
          ARGUS
        </span>
        <ArgusIndicator state={indicatorState} />
      </div>

      {/* Top-right: hazard count */}
      <div className="absolute top-5 right-5 z-20 text-right">
        <span className="font-mono font-normal leading-none" style={{ fontSize: 28, color: riskColor }}>
          {session.hazards.length.toString().padStart(2, "0")}
        </span>
        <p className="font-mono text-xs mt-0.5 tracking-widest" style={{ color: "rgba(255,255,255,0.2)" }}>
          HAZARDS
        </p>
      </div>

      {/* Connection dot */}
      <div
        className="absolute top-6 left-1/2 -translate-x-1/2 z-20 w-1 h-1 rounded-full"
        style={{ background: session.connected ? "#FF5F1F" : "#1c1c1c" }}
      />

      {/* Bottom-left: risk */}
      <div className="absolute bottom-7 left-5 z-20">
        <span
          className="font-display text-xs font-bold tracking-[0.2em] uppercase"
          style={{ color: riskColor }}
        >
          {session.riskLevel}
        </span>
      </div>

      {/* Bottom-right: controls */}
      <div className="absolute bottom-7 right-5 z-20 flex items-center gap-2.5">
        {!voiceEnabled && (
          <button
            onClick={() =>
              session.isInspecting ? session.stopInspection() : session.startInspection(mode)
            }
            className="font-display text-xs font-bold tracking-[0.2em] uppercase px-3 py-1.5 transition-colors duration-100"
            style={
              session.isInspecting
                ? { border: "1px solid rgba(239,68,68,0.4)", color: "#ef4444" }
                : { border: "1px solid rgba(255,95,31,0.4)", color: "#FF5F1F" }
            }
          >
            {session.isInspecting ? "STOP" : "INSPECT"}
          </button>
        )}
        <button
          onClick={() => setVoiceEnabled((v) => !v)}
          className="font-display text-xs font-medium tracking-[0.15em] uppercase px-3 py-1.5 transition-colors duration-100"
          style={
            voiceEnabled
              ? { border: "1px solid rgba(255,95,31,0.6)", color: "#FF5F1F" }
              : { border: "1px solid rgba(255,255,255,0.1)", color: "rgba(255,255,255,0.25)" }
          }
        >
          VOICE
        </button>
      </div>

      {voiceEnabled && (
        <div className="absolute bottom-14 right-5 z-20 text-right">
          <p className="font-mono text-xs tracking-wider" style={{ color: "rgba(255,255,255,0.18)" }}>
            inspect · stop · status · report
          </p>
        </div>
      )}
    </div>
  );
}
