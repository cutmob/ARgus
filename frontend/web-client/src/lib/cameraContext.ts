export type CameraContext = "smartphone" | "cctv" | "ar" | "unknown";

export interface ContextInfo {
  context: CameraContext;
  label: string;
  description: string;
}

export const CONTEXT_INFO: Record<CameraContext, ContextInfo> = {
  smartphone: {
    context: "smartphone",
    label: "Mobile",
    description: "Handheld camera inspection",
  },
  cctv: {
    context: "cctv",
    label: "Fixed Camera",
    description: "CCTV or desktop multi-feed monitoring",
  },
  ar: {
    context: "ar",
    label: "AR Headset",
    description: "Spatial heads-up display",
  },
  unknown: {
    context: "unknown",
    label: "Unknown",
    description: "Select your environment manually",
  },
};
