package temporal

import (
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// IncidentLifecycleState represents the lifecycle of a safety incident.
type IncidentLifecycleState string

const (
	IncidentDetected    IncidentLifecycleState = "detected"
	IncidentPersistent  IncidentLifecycleState = "persistent"
	IncidentEscalated   IncidentLifecycleState = "escalated"
	IncidentAcknowledged IncidentLifecycleState = "acknowledged"
	IncidentResolved    IncidentLifecycleState = "resolved"
	IncidentRecurring   IncidentLifecycleState = "recurring"
)

// Incident is a higher-level temporal object built from one or more hazards.
// It is the unit of "violation over time" exposed to reports, Live tools, and UI.
type Incident struct {
	IncidentID        string                 `json:"incident_id"`
	SessionID         string                 `json:"session_id"`
	PrimaryHazard     types.Hazard           `json:"primary_hazard"`
	HazardType        string                 `json:"hazard_type"`
	Severity          types.Severity         `json:"severity"`
	LifecycleState    IncidentLifecycleState `json:"lifecycle_state"`
	StartAt           time.Time              `json:"start_at"`
	EndAt             *time.Time             `json:"end_at,omitempty"`
	InvolvedHazardIDs []string               `json:"involved_hazard_ids,omitempty"`
	InvolvedCameras   []string               `json:"involved_cameras,omitempty"`
	InvolvedZones     []string               `json:"involved_zones,omitempty"`
}

// SnapshotRef points to a frame or clip that evidences part of an incident timeline.
type SnapshotRef struct {
	CameraID   string    `json:"camera_id"`
	Timestamp  time.Time `json:"timestamp"`
	StorageKey string    `json:"storage_key"`
}

// OperatorActionRef records an operator action related to an incident.
type OperatorActionRef struct {
	ActionID  string    `json:"action_id"`
	Type      string    `json:"type"`
	ActorID   string    `json:"actor_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ExportRef records a downstream export connected to an incident.
type ExportRef struct {
	ExportID  string    `json:"export_id"`
	Format    string    `json:"format"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
}

// EvidencePack is the audit-ready temporal evidence bundle for a single incident.
type EvidencePack struct {
	IncidentID      string              `json:"incident_id"`
	RulesTriggered  []string            `json:"rules_triggered,omitempty"`
	FirstSeen       time.Time           `json:"first_seen"`
	PeakTime        time.Time           `json:"peak_time"`
	LastSeen        time.Time           `json:"last_seen"`
	// PeakLLR is the highest SPRT log-likelihood ratio observed during this incident.
	// A value >= 2.89 means the incident has crossed the SPRT alarm threshold
	// (α=0.05, β=0.10), giving a mathematically grounded confirmation of hazard presence.
	PeakLLR         float64             `json:"peak_llr,omitempty"`
	// SPRTConfirmed is true once the log-likelihood ratio crossed the alarm threshold.
	SPRTConfirmed   bool                `json:"sprt_confirmed,omitempty"`
	// RiskTrend is the computed slope direction from the confidence history window.
	RiskTrend       string              `json:"risk_trend,omitempty"`
	Snapshots       []SnapshotRef       `json:"snapshots,omitempty"`
	OperatorActions []OperatorActionRef `json:"operator_actions,omitempty"`
	Exports         []ExportRef         `json:"exports,omitempty"`
}

// Summary provides an aggregate view of incidents for a session and time range.
type Summary struct {
	SessionID     string            `json:"session_id"`
	From          time.Time         `json:"from"`
	To            time.Time         `json:"to"`
	IncidentCount int               `json:"incident_count"`
	ByHazardType  map[string]int    `json:"by_hazard_type,omitempty"`
	BySeverity    map[string]int    `json:"by_severity,omitempty"`
	TopPersistent []Incident        `json:"top_persistent,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

