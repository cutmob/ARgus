package temporal

import (
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// DefaultRules returns the initial enterprise-safe temporal ruleset.
// These rules are intentionally conservative and map to existing module rule_ids.
func DefaultRules() []RuleSpec {
	sevCritical := types.SeverityCritical

	statePersistent := IncidentPersistent
	stateEscalated := IncidentEscalated
	stateRecurring := IncidentRecurring

	return []RuleSpec{
		// --- Construction ---
		{
			ID:             "temp_const_persistent_edge",
			HazardRuleID:   "const_006", // unguarded leading edge
			MinSeverity:    types.SeverityHigh,
			RequiredDuration: 5 * time.Second,
			Window:         10 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				SetSeverity:          &sevCritical,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_const_recurring_equipment_exposure",
			HazardRuleID:  "const_024", // equipment operating without spotter
			MinSeverity:   types.SeverityHigh,
			Window:        60 * time.Second,
			MinOccurrences: 3,
			Effect: RuleEffect{
				SetLifecycleState:    &stateEscalated,
				MarkRecurring:        true,
				CreateIncidentIfNone: true,
			},
		},

		// --- Facility ---
		{
			ID:             "temp_fac_persistent_blocked_exit",
			HazardRuleID:   "fac_001",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 120 * time.Second,
			Window:         300 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:             "temp_fac_panel_clearance_violation",
			HazardRuleID:   "fac_004",
			MinSeverity:    types.SeverityHigh,
			RequiredDuration: 300 * time.Second,
			Window:         900 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:             "temp_fac_persistent_sprinkler_obstruction",
			HazardRuleID:   "fac_013",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 600 * time.Second,
			Window:         1800 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &stateEscalated,
				CreateIncidentIfNone: true,
			},
		},

		// --- Healthcare ---
		{
			ID:            "temp_hc_corridor_storage_recurring",
			HazardRuleID:  "hc_005",
			MinSeverity:   types.SeverityMedium,
			Window:        1800 * time.Second,
			MinOccurrences: 3,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:             "temp_hc_persistent_overfilled_sharps",
			HazardRuleID:   "hc_013",
			MinSeverity:    types.SeverityHigh,
			RequiredDuration: 3600 * time.Second,
			Window:         7200 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				SetSeverity:          &sevCritical,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_hc_isolation_controls_recurring",
			HazardRuleID:  "hc_035",
			MinSeverity:   types.SeverityHigh,
			Window:        7200 * time.Second,
			MinOccurrences: 2,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_hc_hh_station_recurring",
			HazardRuleID:  "hc_063",
			MinSeverity:   types.SeverityHigh,
			Window:        14400 * time.Second,
			MinOccurrences: 3,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},

		// --- Hotel ---
		{
			ID:             "temp_htl_persistent_stairwell_storage",
			HazardRuleID:   "htl_009",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 600 * time.Second,
			Window:         1800 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_htl_detector_recurring",
			HazardRuleID:  "htl_013",
			MinSeverity:   types.SeverityCritical,
			Window:        86400 * time.Second,
			MinOccurrences: 2,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:             "temp_htl_pool_barrier_persistent",
			HazardRuleID:   "htl_020",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 300 * time.Second,
			Window:         900 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &stateEscalated,
				CreateIncidentIfNone: true,
			},
		},

		// --- Kitchen ---
		{
			ID:             "temp_kit_hot_hold_unsafe",
			HazardRuleID:   "kit_025",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 900 * time.Second,
			Window:         1800 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_kit_cold_hold_recurring",
			HazardRuleID:  "kit_026",
			MinSeverity:   types.SeverityCritical,
			Window:        14400 * time.Second,
			MinOccurrences: 2,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:             "temp_kit_suppression_persistent",
			HazardRuleID:   "kit_004",
			MinSeverity:    types.SeverityCritical,
			RequiredDuration: 3600 * time.Second,
			Window:         7200 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &stateEscalated,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:            "temp_kit_pest_infestation",
			HazardRuleID:  "kit_014",
			MinSeverity:   types.SeverityCritical,
			Window:        86400 * time.Second,
			MinOccurrences: 2,
			Effect: RuleEffect{
				SetLifecycleState:    &stateRecurring,
				CreateIncidentIfNone: true,
			},
		},
	}
}

