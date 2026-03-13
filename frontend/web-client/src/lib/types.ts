export type Severity = "low" | "medium" | "high" | "critical";
export type AlertThreshold = "off" | Severity;

export interface Hazard {
  id: string;
  rule_id: string;
  description: string;
  severity: Severity;
  confidence: number;
  occurrences?: number;
  first_seen_at?: string;
  last_seen_at?: string;
  persistence_seconds?: number;
  risk_trend?: string;
  location?: string;
  bbox?: BBox;
  frame_id?: string;
  image_url?: string;
  camera_id?: string;
  detected_at: string;
}

export interface ActionCard {
  title: string;
  priority: string;
  reason: string;
  actions: string[];
  camera_id?: string;
  hazard_ref_id?: string;
}

export interface BBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Overlay {
  type: string;
  label: string;
  bbox?: BBox;
  severity: string;
  color: string;
}

// Incident is the temporal reasoning unit pushed from the backend after each
// hazard ingest. Carries SPRT confirmation, lifecycle state, and trend data
// for the IncidentTimeline panel.
export interface Incident {
  incident_id: string;
  hazard_type: string;
  severity: Severity;
  lifecycle_state: "detected" | "persistent" | "escalated" | "acknowledged" | "resolved" | "recurring";
  start_at: string;
  last_seen?: string;
  duration_seconds?: number;
  rules_triggered?: string[];
  peak_llr?: number;
  sprt_confirmed?: boolean;
  risk_trend?: string;
  cameras?: string[];
  snapshot_count?: number;
}

export interface InspectionReport {
  id: string;
  session_id: string;
  inspection_mode: string;
  location: string;
  inspector: string;
  hazards: Hazard[];
  risk_level: string;
  risk_score: number;
  summary: string;
  recommendations: string[];
  created_at: string;
}

export interface InspectionModule {
  name: string;
  version: string;
  description: string;
  industry: string;
  rules: InspectionRule[];
  metadata: ModuleMetadata;
}

export interface InspectionRule {
  rule_id: string;
  description: string;
  severity: string;
  visual_signals: string[];
  category: string;
  enabled: boolean;
}

export interface ModuleMetadata {
  author: string;
  version: string;
  tags: string[];
  required_objects?: string[];
}
