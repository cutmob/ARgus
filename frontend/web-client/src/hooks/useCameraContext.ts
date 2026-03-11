"use client";

import { useState, useEffect } from "react";
import type { CameraContext } from "@/lib/cameraContext";

interface UseCameraContextReturn {
  context: CameraContext;
  detecting: boolean;
}

export function useCameraContext(): UseCameraContextReturn {
  const [context, setContext] = useState<CameraContext>("unknown");
  const [detecting, setDetecting] = useState(true);

  useEffect(() => {
    async function detect() {
      try {
        // 1. WebXR immersive-ar → AR headset
        if ("xr" in navigator) {
          try {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const supported = await (navigator as any).xr.isSessionSupported("immersive-ar");
            if (supported) {
              setContext("ar");
              return;
            }
          } catch {
            // XR API unavailable
          }
        }

        const hasTouch = navigator.maxTouchPoints > 0 || "ontouchstart" in window;
        const isPortrait = window.innerHeight > window.innerWidth;

        // 2. Touch + portrait = smartphone
        if (hasTouch && isPortrait) {
          setContext("smartphone");
          return;
        }

        // 3. Touch + landscape = tablet/mobile → still smartphone context
        if (hasTouch) {
          try {
            const devices = await navigator.mediaDevices.enumerateDevices();
            const hasVideo = devices.some((d) => d.kind === "videoinput");
            if (hasVideo) {
              setContext("smartphone");
              return;
            }
          } catch {
            // enumerateDevices unavailable
          }
          setContext("smartphone");
          return;
        }

        // 4. No touch = desktop / fixed camera setup
        setContext("cctv");
      } finally {
        setDetecting(false);
      }
    }

    detect();
  }, []);

  return { context, detecting };
}
