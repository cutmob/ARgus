"use client";

import { useRef, useEffect, useCallback } from "react";

interface Overlay {
  type: string;
  label: string;
  bbox?: { x: number; y: number; width: number; height: number };
  severity: string;
  color: string;
}

interface CameraViewProps {
  overlays: Overlay[];
  onFrame: (data: Blob) => void;
}

export function CameraView({ overlays, onFrame }: CameraViewProps) {
  const videoRef  = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const streamRef = useRef<MediaStream | null>(null);

  useEffect(() => {
    async function startCamera() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          video: { width: 1280, height: 720, facingMode: "environment" },
          audio: false,
        });
        streamRef.current = stream;
        if (videoRef.current) videoRef.current.srcObject = stream;
      } catch (err) {
        console.error("[ARGUS] Camera access denied:", err);
      }
    }
    startCamera();
    return () => { streamRef.current?.getTracks().forEach((t) => t.stop()); };
  }, []);

  useEffect(() => {
    const interval = setInterval(() => {
      if (!videoRef.current || !canvasRef.current) return;
      const canvas = canvasRef.current;
      const ctx    = canvas.getContext("2d");
      if (!ctx) return;
      canvas.width  = videoRef.current.videoWidth;
      canvas.height = videoRef.current.videoHeight;
      ctx.drawImage(videoRef.current, 0, 0);
      canvas.toBlob((blob) => { if (blob) onFrame(blob); }, "image/jpeg", 0.7);
    }, 3000);
    return () => clearInterval(interval);
  }, [onFrame]);

  const drawOverlays = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    for (const overlay of overlays) {
      if (!overlay.bbox) continue;
      const { x, y, width, height } = overlay.bbox;

      // Thin bounding box at low opacity
      ctx.strokeStyle = overlay.color;
      ctx.lineWidth   = 1;
      ctx.globalAlpha = 0.4;
      ctx.strokeRect(x, y, width, height);
      ctx.globalAlpha = 1;

      // Corner brackets
      const cs = 8;
      ctx.lineWidth = 1.5;
      const corners = [
        [x,         y,           cs,  0, 0,   cs],
        [x + width, y,          -cs,  0, 0,   cs],
        [x,         y + height,  cs,  0, 0,  -cs],
        [x + width, y + height, -cs,  0, 0,  -cs],
      ] as const;
      for (const [cx, cy, dx1, , , dy2] of corners) {
        ctx.beginPath();
        ctx.moveTo(cx + dx1, cy);
        ctx.lineTo(cx, cy);
        ctx.lineTo(cx, cy + dy2);
        ctx.stroke();
      }

      // Label
      ctx.font      = "400 10px 'IBM Plex Mono', monospace";
      const label   = `${overlay.severity.toUpperCase()} · ${overlay.label}`;
      const tw      = ctx.measureText(label).width;
      ctx.fillStyle = overlay.color;
      ctx.globalAlpha = 0.9;
      ctx.fillRect(x, y - 16, tw + 10, 16);
      ctx.globalAlpha = 1;
      ctx.fillStyle = "#000";
      ctx.fillText(label, x + 5, y - 4);
    }
  }, [overlays]);

  useEffect(() => { drawOverlays(); }, [overlays, drawOverlays]);

  return (
    <div className="relative w-full h-full bg-black overflow-hidden">
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="w-full h-full object-cover"
      />
      <canvas
        ref={canvasRef}
        className="absolute inset-0 w-full h-full pointer-events-none"
      />
      <div className="feed-corner feed-corner-tl" />
      <div className="feed-corner feed-corner-tr" />
      <div className="feed-corner feed-corner-bl" />
      <div className="feed-corner feed-corner-br" />

      {/* Static LIVE label — orange dot, matches landing accent */}
      <div className="absolute top-2.5 right-2.5 flex items-center gap-1.5 pointer-events-none">
        <div className="w-1 h-1 rounded-full" style={{ background: "#FF5F1F" }} />
        <span className="font-mono text-xs tracking-widest" style={{ color: "rgba(255,255,255,0.2)" }}>
          LIVE
        </span>
      </div>
    </div>
  );
}
