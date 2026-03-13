"use client";

import { useEffect, useRef, useState } from "react";

interface UseLiveAudioInputOptions {
  enabled: boolean;
  onChunk: (chunk: Uint8Array) => void;
}

interface UseLiveAudioInputResult {
  active: boolean;
  supported: boolean;
}

const TARGET_SAMPLE_RATE = 16000;
// Fallback ScriptProcessor buffer — used only when AudioWorklet unavailable.
const FALLBACK_BUFFER_SIZE = 1024;
// RMS energy VAD gate — chunks quieter than this (~-46 dBFS) are silence.
const VAD_RMS_THRESHOLD = 0.005;

export function useLiveAudioInput({
  enabled,
  onChunk,
}: UseLiveAudioInputOptions): UseLiveAudioInputResult {
  const [active, setActive] = useState(false);
  const onChunkRef = useRef(onChunk);
  onChunkRef.current = onChunk;

  const streamRef    = useRef<MediaStream | null>(null);
  const contextRef   = useRef<AudioContext | null>(null);
  const workletRef   = useRef<AudioWorkletNode | null>(null);
  const processorRef = useRef<ScriptProcessorNode | null>(null);
  const sourceRef    = useRef<MediaStreamAudioSourceNode | null>(null);
  const sinkRef      = useRef<GainNode | null>(null);

  useEffect(() => {
    if (!enabled) {
      teardown();
      return;
    }

    if (typeof window === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      setActive(false);
      return;
    }

    let cancelled = false;

    navigator.mediaDevices
      .getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        video: false,
      })
      .then(async (stream) => {
        if (cancelled) { stream.getTracks().forEach((t) => t.stop()); return; }

        const AudioCtx =
          window.AudioContext ||
          (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
        if (!AudioCtx) { stream.getTracks().forEach((t) => t.stop()); return; }

        const context = new AudioCtx();
        if (context.state === "suspended") await context.resume().catch(() => {});

        const source = context.createMediaStreamSource(stream);
        const sink = context.createGain();
        sink.gain.value = 0;
        sink.connect(context.destination);

        // --- AudioWorklet path (preferred: 20ms chunks at 16kHz, VAD in worklet) ---
        let usedWorklet = false;
        if (context.audioWorklet) {
          try {
            await context.audioWorklet.addModule("/argus-audio-worklet.js");
            const worklet = new AudioWorkletNode(context, "argus-audio-processor");
            worklet.port.onmessage = (ev: MessageEvent<{ pcm16: Int16Array }>) => {
              if (cancelled) return;
              onChunkRef.current(new Uint8Array(ev.data.pcm16.buffer));
            };
            source.connect(worklet);
            worklet.connect(sink);
            workletRef.current = worklet;
            usedWorklet = true;
          } catch {
            // Worklet unavailable — fall through to ScriptProcessor
          }
        }

        // --- ScriptProcessor fallback with client-side VAD gate ---
        if (!usedWorklet) {
          const processor = context.createScriptProcessor(FALLBACK_BUFFER_SIZE, 1, 1);
          processor.onaudioprocess = (ev) => {
            if (cancelled) return;
            const input = ev.inputBuffer.getChannelData(0);
            let sumSq = 0;
            for (let i = 0; i < input.length; i++) sumSq += input[i] * input[i];
            if (Math.sqrt(sumSq / input.length) < VAD_RMS_THRESHOLD) return;
            const pcm16 = downsampleToPCM16(input, ev.inputBuffer.sampleRate, TARGET_SAMPLE_RATE);
            if (pcm16.byteLength > 0) onChunkRef.current(new Uint8Array(pcm16.buffer));
          };
          source.connect(processor);
          processor.connect(sink);
          processorRef.current = processor;
        }

        streamRef.current  = stream;
        contextRef.current = context;
        sourceRef.current  = source;
        sinkRef.current    = sink;
        setActive(true);
      })
      .catch(() => setActive(false));

    return () => { cancelled = true; teardown(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled]);

  function teardown() {
    workletRef.current?.port.close();
    workletRef.current?.disconnect();
    processorRef.current?.disconnect();
    sourceRef.current?.disconnect();
    sinkRef.current?.disconnect();
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = sourceRef.current = workletRef.current = processorRef.current = sinkRef.current = null;
    if (contextRef.current && contextRef.current.state !== "closed") {
      void contextRef.current.close().catch(() => {});
    }
    contextRef.current = null;
    setActive(false);
  }

  return {
    active,
    supported: typeof window !== "undefined" && !!navigator.mediaDevices?.getUserMedia,
  };
}

function downsampleToPCM16(input: Float32Array, inputRate: number, outputRate: number): Int16Array {
  if (inputRate === outputRate) {
    const pcm = new Int16Array(input.length);
    for (let i = 0; i < input.length; i++) {
      const s = Math.max(-1, Math.min(1, input[i]));
      pcm[i] = s < 0 ? s * 32768 : s * 32767;
    }
    return pcm;
  }
  const ratio = inputRate / outputRate;
  const outLen = Math.max(1, Math.round(input.length / ratio));
  const output = new Int16Array(outLen);
  let oIdx = 0, iIdx = 0;
  while (oIdx < outLen) {
    const next = Math.min(input.length, Math.round((oIdx + 1) * ratio));
    let acc = 0, count = 0;
    for (let i = iIdx; i < next; i++) { acc += input[i]; count++; }
    const s = Math.max(-1, Math.min(1, count > 0 ? acc / count : 0));
    output[oIdx++] = s < 0 ? s * 32768 : s * 32767;
    iIdx = next;
  }
  return output;
}
