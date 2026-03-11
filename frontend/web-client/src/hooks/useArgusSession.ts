"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { speakResponse } from "@/lib/tts";

interface Overlay {
  type: string;
  label: string;
  bbox?: { x: number; y: number; width: number; height: number };
  severity: string;
  color: string;
}

interface Hazard {
  id: string;
  description: string;
  severity: string;
  confidence: number;
  location?: string;
  camera_id?: string;
  detected_at: string;
}

interface WebSocketMessage {
  type: string;
  session_id: string;
  payload: unknown;
  timestamp: string;
}

const WS_URL = process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080/ws";

function generateCameraId(): string {
  // Stable per-device ID persisted in localStorage
  const key = "argus_camera_id";
  let id = typeof window !== "undefined" ? localStorage.getItem(key) : null;
  if (!id) {
    id = "cam_" + Math.random().toString(36).slice(2, 10);
    if (typeof window !== "undefined") localStorage.setItem(key, id);
  }
  return id;
}

export function useArgusSession() {
  const [connected, setConnected]       = useState(false);
  const [isInspecting, setIsInspecting] = useState(false);
  const [hazards, setHazards]           = useState<Hazard[]>([]);
  const [overlays, setOverlays]         = useState<Overlay[]>([]);
  const [riskLevel, setRiskLevel]       = useState<string>("low");
  const [sessionId, setSessionId]       = useState<string>("");
  const [processing, setProcessing]     = useState(false);
  const [speaking, setSpeaking]         = useState(false);

  const wsRef          = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<NodeJS.Timeout>();

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const cameraId = generateCameraId();
    const url = WS_URL + (WS_URL.includes("?") ? "&" : "?") + "camera_id=" + cameraId;
    const ws = new WebSocket(url);

    ws.onopen = () => {
      setConnected(true);
      setProcessing(false);
    };

    ws.onmessage = (event) => {
      try {
        const msg: WebSocketMessage = JSON.parse(event.data);
        handleMessage(msg);
      } catch (err) {
        console.error("[ARGUS] Invalid message:", err);
      }
    };

    ws.onclose = () => {
      setConnected(false);
      reconnectTimer.current = setTimeout(connect, 3000);
    };

    ws.onerror = (err) => {
      console.error("[ARGUS] WebSocket error:", err);
      ws.close();
    };

    wsRef.current = ws;
  }, []);

  const handleMessage = useCallback((msg: WebSocketMessage) => {
    setProcessing(false);

    switch (msg.type) {
      case "session_created":
        setSessionId(msg.session_id);
        break;

      case "hazard_detected": {
        const hazard = msg.payload as Hazard;
        setHazards((prev) => [hazard, ...prev]);
        break;
      }

      case "overlays_update": {
        const newOverlays = msg.payload as Overlay[];
        setOverlays(newOverlays);
        break;
      }

      case "risk_update": {
        const data = msg.payload as { risk_level: string };
        setRiskLevel(data.risk_level);
        break;
      }

      case "voice_response": {
        const data = msg.payload as { text: string };
        setSpeaking(true);
        speakResponse(data.text, () => setSpeaking(false));
        break;
      }

      case "inspection_started":
        setIsInspecting(true);
        break;

      case "inspection_stopped":
        setIsInspecting(false);
        break;
    }
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, [connect]);

  const sendFrame = useCallback((blob: Blob) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
    blob.arrayBuffer().then((buffer) => {
      const typeByte  = new Uint8Array([0x01]);
      const frameData = new Uint8Array(buffer);
      const message   = new Uint8Array(typeByte.length + frameData.length);
      message.set(typeByte, 0);
      message.set(frameData, 1);
      wsRef.current?.send(message.buffer);
    });
  }, []);

  const sendCommand = useCallback(
    (type: string, payload: Record<string, unknown> = {}) => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
      const msg = {
        type,
        session_id: sessionId,
        payload,
        timestamp: new Date().toISOString(),
      };
      wsRef.current.send(JSON.stringify(msg));
      setProcessing(true);
    },
    [sessionId]
  );

  const startInspection = useCallback(
    (mode: string) => {
      sendCommand("start_inspection", { mode });
      setIsInspecting(true);
      setHazards([]);
      setOverlays([]);
    },
    [sendCommand]
  );

  const stopInspection = useCallback(() => {
    sendCommand("stop_inspection");
    setIsInspecting(false);
  }, [sendCommand]);

  const switchMode = useCallback(
    (mode: string) => { sendCommand("switch_mode", { mode }); },
    [sendCommand]
  );

  const generateReport = useCallback(() => {
    sendCommand("generate_report", { format: "json" });
  }, [sendCommand]);

  return {
    connected,
    isInspecting,
    hazards,
    overlays,
    riskLevel,
    sessionId,
    processing,
    speaking,
    sendFrame,
    startInspection,
    stopInspection,
    switchMode,
    generateReport,
  };
}
