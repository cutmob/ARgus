let currentSpeechToken = 0;
let activeSpeechToken = 0;
let speechQueue: Array<{ text: string; onEnd?: () => void; token: number }> = [];
let speechQueueRunning = false;
let audioContext: AudioContext | null = null;
let pcmCursorTime = 0;
let activePcmSources = new Set<AudioBufferSourceNode>();

const SPEECH_GAP_MS = 260;
const PCM_GAP_SECONDS = 0.04;

function ensureAudioContext(): AudioContext | null {
  if (typeof window === "undefined") return null;
  const AudioCtx = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioCtx) return null;
  if (!audioContext) {
    audioContext = new AudioCtx();
  }
  if (audioContext.state === "suspended") {
    void audioContext.resume().catch(() => {});
  }
  return audioContext;
}

function processSpeechQueue(): void {
  if (speechQueueRunning) return;
  if (typeof window === "undefined" || !("speechSynthesis" in window)) {
    while (speechQueue.length > 0) {
      speechQueue.shift()?.onEnd?.();
    }
    return;
  }
  const next = speechQueue.shift();
  if (!next) return;
  speechQueueRunning = true;
  activeSpeechToken = next.token;
  const utterance = new SpeechSynthesisUtterance(next.text);
  utterance.rate = 1.08;
  utterance.pitch = 0.92;
  const finish = () => {
    if (activeSpeechToken !== next.token) return;
    next.onEnd?.();
    window.setTimeout(() => {
      if (activeSpeechToken !== next.token) return;
      speechQueueRunning = false;
      processSpeechQueue();
    }, SPEECH_GAP_MS);
  };
  utterance.onend = finish;
  utterance.onerror = finish;
  window.speechSynthesis.speak(utterance);
}

export function speakResponse(text: string, onEnd?: () => void): void {
  const nextText = text.trim();
  if (!nextText) {
    onEnd?.();
    return;
  }
  currentSpeechToken += 1;
  speechQueue.push({ text: nextText, onEnd, token: currentSpeechToken });
  processSpeechQueue();
}

export function playAudioResponse(base64Audio: string, onEnd?: () => void): void {
  if (typeof window === "undefined") {
    onEnd?.();
    return;
  }
  const encoded = base64Audio.trim();
  if (!encoded) {
    onEnd?.();
    return;
  }
  const ctx = ensureAudioContext();
  if (!ctx) {
    onEnd?.();
    return;
  }
  currentSpeechToken += 1;
  activeSpeechToken = currentSpeechToken;
  const token = currentSpeechToken;
  const binary = window.atob(encoded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  const int16 = new Int16Array(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
  const float32 = new Float32Array(int16.length);
  for (let i = 0; i < int16.length; i += 1) {
    float32[i] = Math.max(-1, Math.min(1, int16[i] / 32768));
  }
  const sampleRate = 24000;
  const buffer = ctx.createBuffer(1, float32.length, sampleRate);
  buffer.copyToChannel(float32, 0);
  const source = ctx.createBufferSource();
  source.buffer = buffer;
  source.connect(ctx.destination);
  const startAt = Math.max(ctx.currentTime + 0.01, pcmCursorTime);
  pcmCursorTime = startAt + buffer.duration + PCM_GAP_SECONDS;
  activePcmSources.add(source);
  source.onended = () => {
    activePcmSources.delete(source);
    if (token === activeSpeechToken) {
      onEnd?.();
    }
  };
  source.start(startAt);
}

export function stopSpeaking(): void {
  currentSpeechToken += 1;
  activeSpeechToken = currentSpeechToken;
  speechQueue = [];
  speechQueueRunning = false;
  if (typeof window !== "undefined" && "speechSynthesis" in window) {
    window.speechSynthesis.cancel();
  }
  for (const source of activePcmSources) {
    try {
      source.stop();
    } catch {}
  }
  activePcmSources.clear();
  const ctx = ensureAudioContext();
  pcmCursorTime = ctx ? ctx.currentTime : 0;
}
