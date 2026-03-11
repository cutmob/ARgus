"use client";

import { useEffect, useRef, useCallback } from "react";

interface UseWakeWordOptions {
  onWake: () => void;
  word?: string;
  enabled?: boolean;
}

/**
 * Always-on wake word detector using the browser's built-in SpeechRecognition API.
 * Listens continuously in the background. When the wake word is heard in interim
 * or final results, fires onWake(). Auto-restarts on end/error to stay persistent.
 */
export function useWakeWord({
  onWake,
  word = "argus",
  enabled = true,
}: UseWakeWordOptions) {
  const recognitionRef = useRef<SpeechRecognition | null>(null);
  const onWakeRef = useRef(onWake);
  const enabledRef = useRef(enabled);

  // Keep refs current so callbacks never go stale
  onWakeRef.current = onWake;
  enabledRef.current = enabled;

  const stop = useCallback(() => {
    if (recognitionRef.current) {
      recognitionRef.current.onend = null;
      recognitionRef.current.abort();
      recognitionRef.current = null;
    }
  }, []);

  const start = useCallback(() => {
    if (typeof window === "undefined") return;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const SR = (window as any).SpeechRecognition ?? (window as any).webkitSpeechRecognition;
    if (!SR) return;

    stop();

    const recognition: SpeechRecognition = new SR();
    recognition.continuous = true;
    recognition.interimResults = true; // catch word mid-sentence for low latency
    recognition.lang = "en-US";
    recognition.maxAlternatives = 1;

    let fired = false; // debounce — fire once per utterance
    const target = word.toLowerCase();

    recognition.onresult = (event) => {
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const transcript = event.results[i][0].transcript.toLowerCase();
        if (!fired && transcript.includes(target)) {
          fired = true;
          onWakeRef.current();
        }
        // Reset debounce when utterance finalises
        if (event.results[i].isFinal) fired = false;
      }
    };

    recognition.onend = () => {
      recognitionRef.current = null;
      // Auto-restart to keep always-on behaviour
      if (enabledRef.current) start();
    };

    recognition.onerror = (e) => {
      // "no-speech" and "aborted" are expected — just restart
      if (e.error === "not-allowed" || e.error === "service-not-allowed") return;
      recognitionRef.current = null;
      if (enabledRef.current) start();
    };

    recognitionRef.current = recognition;
    recognition.start();
  }, [stop, word]);

  useEffect(() => {
    if (enabled) start();
    else stop();
    return stop;
  }, [enabled, start, stop]);
}
