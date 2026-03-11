"use client";

import { useRef, useEffect } from "react";
import { HazardOverlay } from "@/components/HazardOverlay";
import type { Overlay } from "@/lib/types";

interface CameraViewProps {
  overlays: Overlay[];
  overlaysVisible?: boolean;
  onFrame: (data: Blob) => void;
}

export function CameraView({ overlays, overlaysVisible = true, onFrame }: CameraViewProps) {
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

  return (
    <div className="relative w-full h-full bg-black overflow-hidden">
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="w-full h-full object-cover"
      />
      <canvas ref={canvasRef} className="hidden" />

      <HazardOverlay overlays={overlays} visible={overlaysVisible} />

      <div className="feed-corner feed-corner-tl" />
      <div className="feed-corner feed-corner-tr" />
      <div className="feed-corner feed-corner-bl" />
      <div className="feed-corner feed-corner-br" />

      {/* LIVE indicator */}
      <div className="absolute top-2.5 right-2.5 flex items-center gap-1.5 pointer-events-none">
        <div className="w-1 h-1 rounded-full" style={{ background: "#FF5F1F" }} />
        <span className="font-mono text-xs tracking-widest" style={{ color: "rgba(255,255,255,0.2)" }}>
          LIVE
        </span>
      </div>
    </div>
  );
}
