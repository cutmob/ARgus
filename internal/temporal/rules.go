package temporal

import (
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// RuleSpec defines a temporal rule over a stream of hazards.
// Rules are kept intentionally simple and data-driven so they can be tuned
// without changing core engine logic.
type RuleSpec struct {
	ID string `json:"id"`

	// Matching scope
	HazardRuleID   string         `json:"hazard_rule_id,omitempty"`
	MinSeverity    types.Severity `json:"min_severity,omitempty"`
	MinConfidence  float64        `json:"min_confidence,omitempty"`

	// Temporal conditions
	Window              time.Duration `json:"window,omitempty"`
	RequiredDuration    time.Duration `json:"required_duration,omitempty"`
	MinOccurrences      int           `json:"min_occurrences,omitempty"`

	// Effects when the rule fires
	Effect RuleEffect `json:"effect"`
}

// RuleEffect describes how engine state should change when a rule fires.
type RuleEffect struct {
	SetLifecycleState    *IncidentLifecycleState `json:"set_lifecycle_state,omitempty"`
	SetSeverity          *types.Severity         `json:"set_severity,omitempty"`
	CreateIncidentIfNone bool                    `json:"create_incident_if_none,omitempty"`
	MarkRecurring        bool                    `json:"mark_recurring,omitempty"`
}

