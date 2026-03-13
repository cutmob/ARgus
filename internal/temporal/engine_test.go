package temporal

import (
	"testing"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

func TestEngine_CreatesPersistentIncident(t *testing.T) {
	e := NewEngine(nil, DefaultRules())

	now := time.Now()
	h := types.Hazard{
		ID:                 "h1",
		RuleID:             "const_006",
		Description:        "Unguarded leading edge",
		Severity:           types.SeverityHigh,
		Confidence:         0.9,
		Occurrences:        2,
		FirstSeenAt:        now.Add(-6 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 6,
		CameraID:           "camA",
		DetectedAt:         now,
	}

	e.IngestHazard("s1", h)

	incs, err := e.GetActiveIncidents("s1")
	if err != nil {
		t.Fatalf("GetActiveIncidents error: %v", err)
	}
	if len(incs) == 0 {
		t.Fatalf("expected incident to be created")
	}

	var found *Incident
	for i := range incs {
		if incs[i].HazardType == "const_006" {
			found = &incs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected incident hazard_type const_006")
	}
	if found.LifecycleState != IncidentPersistent {
		t.Fatalf("expected lifecycle %s, got %s", IncidentPersistent, found.LifecycleState)
	}
	if found.Severity != types.SeverityCritical {
		t.Fatalf("expected severity %s, got %s", types.SeverityCritical, found.Severity)
	}
}

func TestEngine_CreatesRecurringIncident(t *testing.T) {
	e := NewEngine(nil, DefaultRules())

	now := time.Now()
	h := types.Hazard{
		ID:          "h2",
		RuleID:      "const_024",
		Description: "Equipment operating without spotter",
		Severity:    types.SeverityHigh,
		Confidence:  0.8,
		Occurrences: 3,
		FirstSeenAt: now.Add(-50 * time.Second),
		LastSeenAt:  now,
		CameraID:    "camB",
		DetectedAt:  now,
	}

	e.IngestHazard("s2", h)

	incs, err := e.GetActiveIncidents("s2")
	if err != nil {
		t.Fatalf("GetActiveIncidents error: %v", err)
	}
	if len(incs) == 0 {
		t.Fatalf("expected incident to be created")
	}
	if incs[0].LifecycleState != IncidentRecurring {
		t.Fatalf("expected lifecycle %s, got %s", IncidentRecurring, incs[0].LifecycleState)
	}
}

func TestEngine_DoesNotFireWhenOutsideWindow(t *testing.T) {
	e := NewEngine(nil, DefaultRules())

	now := time.Now()
	h := types.Hazard{
		ID:          "h3",
		RuleID:      "const_024",
		Description: "Equipment operating without spotter",
		Severity:    types.SeverityHigh,
		Confidence:  0.8,
		Occurrences: 3,
		FirstSeenAt: now.Add(-5 * time.Minute),
		LastSeenAt:  now.Add(-2 * time.Minute), // outside 60s window
		CameraID:    "camC",
		DetectedAt:  now,
	}

	e.IngestHazard("s3", h)

	incs, err := e.GetActiveIncidents("s3")
	if err != nil {
		t.Fatalf("GetActiveIncidents error: %v", err)
	}
	if len(incs) != 0 {
		t.Fatalf("expected no incidents, got %d", len(incs))
	}
}

