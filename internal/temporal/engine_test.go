package temporal

import (
	"math"
	"testing"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// ---------------------------------------------------------------------------
// Existing tests (preserved & updated)
// ---------------------------------------------------------------------------

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
	// With only one rule fired, single-rule incidents stay at their rule-set
	// lifecycle unless auto-escalation kicks in. Because this is the first and
	// only rule for session s1, the lifecycle should reflect auto-escalation
	// only if LLR >= 5.0. A single 0.9-confidence observation yields
	// LLR ≈ 2.197, which is below both alarm and escalation thresholds.
	// The rule's RequiredDuration is 5s and the hazard persists 6s, so the
	// temporal condition is satisfied and the rule fires with its effect
	// (persistent + critical). With only 1 distinct rule fired, no
	// auto-escalation applies.
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
	// With only one distinct rule fired on session s2, auto-escalation does
	// not override the recurring state.
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

// ---------------------------------------------------------------------------
// SPRT decay tests
// ---------------------------------------------------------------------------

func TestDecaySPRT_ZeroEntry(t *testing.T) {
	result := decaySPRT(sprtEntry{}, time.Now())
	if result != 0 {
		t.Fatalf("expected 0 for zero entry, got %f", result)
	}
}

func TestDecaySPRT_NoElapsed(t *testing.T) {
	now := time.Now()
	entry := sprtEntry{LLR: 2.0, LastSeen: now}
	result := decaySPRT(entry, now)
	if result != 2.0 {
		t.Fatalf("expected 2.0 for zero elapsed, got %f", result)
	}
}

func TestDecaySPRT_HalvesAtHalfLife(t *testing.T) {
	now := time.Now()
	entry := sprtEntry{LLR: 4.0, LastSeen: now.Add(-sprtDecayHalfLife)}
	result := decaySPRT(entry, now)
	// After exactly one half-life, LLR should be halved (within floating-point tolerance)
	if math.Abs(result-2.0) > 0.01 {
		t.Fatalf("expected ~2.0 after one half-life, got %f", result)
	}
}

func TestDecaySPRT_DecaysOverTime(t *testing.T) {
	now := time.Now()
	entry := sprtEntry{LLR: 4.0, LastSeen: now.Add(-2 * sprtDecayHalfLife)}
	result := decaySPRT(entry, now)
	// After two half-lives, LLR should be quartered
	if math.Abs(result-1.0) > 0.01 {
		t.Fatalf("expected ~1.0 after two half-lives, got %f", result)
	}
}

// ---------------------------------------------------------------------------
// SPRT accumulation across multiple observations
// ---------------------------------------------------------------------------

func TestEngine_SPRTAccumulatesAcrossObservations(t *testing.T) {
	statePersistent := IncidentPersistent
	sevCritical := types.SeverityCritical
	rules := []RuleSpec{
		{
			ID:           "test_sprt_rule",
			HazardRuleID: "test_001",
			MinSeverity:  types.SeverityHigh,
			Window:       60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				SetSeverity:          &sevCritical,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)
	eng := e.(*engine)

	now := time.Now()
	// Feed multiple high-confidence observations rapidly to accumulate LLR
	for i := 0; i < 5; i++ {
		h := types.Hazard{
			ID:          "acc_h",
			RuleID:      "test_001",
			Description: "Test hazard",
			Severity:    types.SeverityHigh,
			Confidence:  0.95,
			CameraID:    "cam1",
			DetectedAt:  now.Add(time.Duration(i) * time.Second),
		}
		e.IngestHazard("sprt_session", h)
	}

	key := sprtKey("sprt_session", "test_001", "cam1")
	eng.mu.Lock()
	entry := eng.sprtState[key]
	eng.mu.Unlock()

	// 5 observations of 0.95 confidence should yield a significantly positive LLR
	if entry.LLR <= 0 {
		t.Fatalf("expected positive accumulated LLR, got %f", entry.LLR)
	}

	// With rapid-fire high-confidence observations, SPRT should eventually
	// cross the alarm threshold and create an incident
	incs, err := e.GetActiveIncidents("sprt_session")
	if err != nil {
		t.Fatalf("GetActiveIncidents error: %v", err)
	}
	if len(incs) == 0 {
		t.Fatalf("expected incident from accumulated SPRT evidence")
	}
}

// ---------------------------------------------------------------------------
// SPRT clear threshold and incident resolution
// ---------------------------------------------------------------------------

func TestEngine_SPRTClearResolvesIncident(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_clear_rule",
			HazardRuleID:     "clear_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()

	// First, create an incident with high-confidence observation
	h := types.Hazard{
		ID:                 "clear_h1",
		RuleID:             "clear_001",
		Description:        "Test hazard for clear",
		Severity:           types.SeverityHigh,
		Confidence:         0.95,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 5,
		CameraID:           "cam1",
		DetectedAt:         now,
	}
	e.IngestHazard("clear_sess", h)

	// Verify incident exists
	incs, _ := e.GetActiveIncidents("clear_sess")
	if len(incs) == 0 {
		t.Fatalf("expected incident to be created first")
	}

	// Now feed many low-confidence observations to drive LLR below clear threshold
	for i := 0; i < 30; i++ {
		lowH := types.Hazard{
			ID:                 "clear_h1",
			RuleID:             "clear_001",
			Description:        "Test hazard for clear",
			Severity:           types.SeverityHigh,
			Confidence:         0.05,
			FirstSeenAt:        now.Add(-5 * time.Second),
			LastSeenAt:         now.Add(time.Duration(i+1) * time.Second),
			PersistenceSeconds: 5 + i + 1,
			CameraID:           "cam1",
			DetectedAt:         now.Add(time.Duration(i+1) * time.Second),
		}
		e.IngestHazard("clear_sess", lowH)
	}

	// After many low-confidence observations, the incident should be resolved
	incs, _ = e.GetActiveIncidents("clear_sess")
	if len(incs) != 0 {
		t.Fatalf("expected incident to be resolved (cleared), got %d active", len(incs))
	}
}

// ---------------------------------------------------------------------------
// Confidence history clearing on resolve
// ---------------------------------------------------------------------------

func TestEngine_ConfHistClearedOnResolve(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_hist_rule",
			HazardRuleID:     "hist_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)
	eng := e.(*engine)

	now := time.Now()
	key := sprtKey("hist_sess", "hist_001", "cam1")

	// Create incident
	h := types.Hazard{
		ID:                 "hist_h1",
		RuleID:             "hist_001",
		Description:        "Test hazard for hist",
		Severity:           types.SeverityHigh,
		Confidence:         0.95,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 5,
		CameraID:           "cam1",
		DetectedAt:         now,
	}
	e.IngestHazard("hist_sess", h)

	eng.mu.Lock()
	histBefore := len(eng.confHist[key])
	eng.mu.Unlock()
	if histBefore == 0 {
		t.Fatalf("expected confidence history to be populated")
	}

	// Drive LLR below clear to resolve
	for i := 0; i < 30; i++ {
		lowH := types.Hazard{
			ID:                 "hist_h1",
			RuleID:             "hist_001",
			Description:        "Test hazard for hist",
			Severity:           types.SeverityHigh,
			Confidence:         0.05,
			FirstSeenAt:        now.Add(-5 * time.Second),
			LastSeenAt:         now.Add(time.Duration(i+1) * time.Second),
			PersistenceSeconds: 5 + i + 1,
			CameraID:           "cam1",
			DetectedAt:         now.Add(time.Duration(i+1) * time.Second),
		}
		e.IngestHazard("hist_sess", lowH)
	}

	eng.mu.Lock()
	histAfter := eng.confHist[key]
	eng.mu.Unlock()
	if histAfter != nil {
		t.Fatalf("expected confidence history to be nil after resolve, got len=%d", len(histAfter))
	}
}

// ---------------------------------------------------------------------------
// Time-bounded confidence history pruning
// ---------------------------------------------------------------------------

func TestAppendConfSample_PrunesOldSamples(t *testing.T) {
	now := time.Now()
	var hist []types.ConfidenceSample

	// Add samples spread over 10 minutes (beyond confHistMaxAge of 5 min)
	for i := 0; i < 20; i++ {
		ts := now.Add(-time.Duration(20-i) * 30 * time.Second) // 10 min span
		hist = appendConfSample(hist, types.ConfidenceSample{
			Confidence: 0.5,
			Timestamp:  ts,
		}, now)
	}

	// All samples older than 5 minutes should be pruned
	for _, s := range hist {
		age := now.Sub(s.Timestamp)
		if age > confHistMaxAge+time.Second {
			t.Fatalf("found sample older than confHistMaxAge: age=%v", age)
		}
	}
}

func TestAppendConfSample_CapsAtMaxSamples(t *testing.T) {
	now := time.Now()
	var hist []types.ConfidenceSample

	// Add more than confHistMaxSamples samples, all within time window
	for i := 0; i < confHistMaxSamples+50; i++ {
		ts := now.Add(-time.Duration(confHistMaxSamples+50-i) * time.Millisecond)
		hist = appendConfSample(hist, types.ConfidenceSample{
			Confidence: 0.7,
			Timestamp:  ts,
		}, now)
	}

	if len(hist) > confHistMaxSamples {
		t.Fatalf("expected max %d samples, got %d", confHistMaxSamples, len(hist))
	}
}

// ---------------------------------------------------------------------------
// EWMA trend computation
// ---------------------------------------------------------------------------

func TestComputeRiskTrend_Escalating(t *testing.T) {
	samples := make([]types.ConfidenceSample, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		// Confidence rising from 0.3 to 0.9
		samples[i] = types.ConfidenceSample{
			Confidence: 0.3 + float64(i)*0.067,
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}
	}
	trend := computeRiskTrend(samples)
	if trend != "escalating" {
		t.Fatalf("expected escalating trend, got %s", trend)
	}
}

func TestComputeRiskTrend_Improving(t *testing.T) {
	samples := make([]types.ConfidenceSample, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		// Confidence falling from 0.9 to 0.3
		samples[i] = types.ConfidenceSample{
			Confidence: 0.9 - float64(i)*0.067,
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}
	}
	trend := computeRiskTrend(samples)
	if trend != "improving" {
		t.Fatalf("expected improving trend, got %s", trend)
	}
}

func TestComputeRiskTrend_Stable(t *testing.T) {
	samples := make([]types.ConfidenceSample, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		samples[i] = types.ConfidenceSample{
			Confidence: 0.7, // constant
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}
	}
	trend := computeRiskTrend(samples)
	if trend != "stable" {
		t.Fatalf("expected stable trend, got %s", trend)
	}
}

func TestComputeRiskTrend_NewWithFewSamples(t *testing.T) {
	samples := []types.ConfidenceSample{
		{Confidence: 0.5, Timestamp: time.Now()},
		{Confidence: 0.6, Timestamp: time.Now()},
	}
	trend := computeRiskTrend(samples)
	if trend != "new" {
		t.Fatalf("expected new trend with <3 samples, got %s", trend)
	}
}

// ---------------------------------------------------------------------------
// EWMA helper
// ---------------------------------------------------------------------------

func TestEwmaConf_SingleSample(t *testing.T) {
	samples := []types.ConfidenceSample{
		{Confidence: 0.75},
	}
	result := ewmaConf(samples)
	if result != 0.75 {
		t.Fatalf("expected 0.75, got %f", result)
	}
}

func TestEwmaConf_Empty(t *testing.T) {
	result := ewmaConf(nil)
	if result != 0 {
		t.Fatalf("expected 0, got %f", result)
	}
}

func TestEwmaConf_WeightsRecentMore(t *testing.T) {
	// 9 low values followed by 1 high value — EWMA should be pulled toward the high
	samples := make([]types.ConfidenceSample, 10)
	for i := 0; i < 9; i++ {
		samples[i] = types.ConfidenceSample{Confidence: 0.1}
	}
	samples[9] = types.ConfidenceSample{Confidence: 1.0}

	ewma := ewmaConf(samples)
	mean := 0.0
	for _, s := range samples {
		mean += s.Confidence
	}
	mean /= float64(len(samples))

	// EWMA should be higher than arithmetic mean because it weights the recent high value more
	if ewma <= mean {
		t.Fatalf("expected EWMA (%f) > arithmetic mean (%f)", ewma, mean)
	}
}

// ---------------------------------------------------------------------------
// Auto-escalation tests
// ---------------------------------------------------------------------------

func TestEngine_AutoEscalatesOnHighLLR(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_esc_rule",
			HazardRuleID:     "esc_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()
	// Feed many high-confidence observations rapidly to push LLR past autoEscalationLLRThreshold
	for i := 0; i < 10; i++ {
		h := types.Hazard{
			ID:                 "esc_h",
			RuleID:             "esc_001",
			Description:        "High LLR hazard",
			Severity:           types.SeverityHigh,
			Confidence:         0.99,
			FirstSeenAt:        now.Add(-5 * time.Second),
			LastSeenAt:         now.Add(time.Duration(i) * time.Second),
			PersistenceSeconds: 5 + i,
			CameraID:           "cam1",
			DetectedAt:         now.Add(time.Duration(i) * time.Second),
		}
		e.IngestHazard("esc_session", h)
	}

	incs, err := e.GetActiveIncidents("esc_session")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(incs) == 0 {
		t.Fatalf("expected incident")
	}
	if incs[0].LifecycleState != IncidentEscalated {
		t.Fatalf("expected escalated lifecycle from high LLR, got %s", incs[0].LifecycleState)
	}
}

func TestEngine_AutoEscalatesOnMultipleDistinctRules(t *testing.T) {
	statePersistent := IncidentPersistent
	sevCritical := types.SeverityCritical
	rules := []RuleSpec{
		{
			ID:               "rule_A",
			HazardRuleID:     "multi_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
		{
			ID:               "rule_B",
			HazardRuleID:     "multi_002",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				SetSeverity:          &sevCritical,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()

	// Fire first rule
	h1 := types.Hazard{
		ID:                 "multi_h1",
		RuleID:             "multi_001",
		Description:        "First rule hazard",
		Severity:           types.SeverityHigh,
		Confidence:         0.85,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 5,
		CameraID:           "cam1",
		DetectedAt:         now,
	}
	e.IngestHazard("multi_sess", h1)

	// Fire second rule — should trigger auto-escalation (2 distinct rules)
	h2 := types.Hazard{
		ID:                 "multi_h2",
		RuleID:             "multi_002",
		Description:        "Second rule hazard",
		Severity:           types.SeverityHigh,
		Confidence:         0.85,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now.Add(1 * time.Second),
		PersistenceSeconds: 6,
		CameraID:           "cam2",
		DetectedAt:         now.Add(1 * time.Second),
	}
	e.IngestHazard("multi_sess", h2)

	incs, _ := e.GetActiveIncidents("multi_sess")
	foundEscalated := false
	for _, inc := range incs {
		if inc.LifecycleState == IncidentEscalated {
			foundEscalated = true
			break
		}
	}
	if !foundEscalated {
		t.Fatalf("expected at least one escalated incident from multiple distinct rules, got lifecycles: %v", lifecycleStates(incs))
	}
}

// ---------------------------------------------------------------------------
// Frame evidence attachment
// ---------------------------------------------------------------------------

func TestEngine_AttachesFrameEvidence(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_frames_rule",
			HazardRuleID:     "frame_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()
	h := types.Hazard{
		ID:                 "frame_h1",
		RuleID:             "frame_001",
		Description:        "Hazard with frames",
		Severity:           types.SeverityHigh,
		Confidence:         0.9,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 5,
		CameraID:           "cam1",
		DetectedAt:         now,
	}
	frames := []types.Frame{
		{ID: "f1", CameraID: "cam1", Timestamp: now.Add(-2 * time.Second)},
		{ID: "f2", CameraID: "cam1", Timestamp: now.Add(-1 * time.Second)},
		{ID: "f3", CameraID: "cam1", Timestamp: now},
	}
	e.IngestHazardWithFrames("frame_sess", h, frames)

	incs, _ := e.GetActiveIncidents("frame_sess")
	if len(incs) == 0 {
		t.Fatalf("expected incident")
	}

	_, evidence, err := e.GetIncidentWithEvidence(incs[0].IncidentID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(evidence.Snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(evidence.Snapshots))
	}
}

// ---------------------------------------------------------------------------
// Incident summary
// ---------------------------------------------------------------------------

func TestEngine_GetIncidentSummary(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_summary_rule",
			HazardRuleID:     "sum_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()
	h := types.Hazard{
		ID:                 "sum_h1",
		RuleID:             "sum_001",
		Description:        "Summary test hazard",
		Severity:           types.SeverityHigh,
		Confidence:         0.9,
		FirstSeenAt:        now.Add(-5 * time.Second),
		LastSeenAt:         now,
		PersistenceSeconds: 5,
		CameraID:           "cam1",
		DetectedAt:         now,
	}
	e.IngestHazard("sum_sess", h)

	summary, err := e.GetIncidentSummary("sum_sess", now.Add(-1*time.Minute), now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if summary.IncidentCount == 0 {
		t.Fatalf("expected at least one incident in summary")
	}
	if summary.ByHazardType["sum_001"] == 0 {
		t.Fatalf("expected sum_001 in ByHazardType")
	}
}

// ---------------------------------------------------------------------------
// Evidence pack SPRT fields
// ---------------------------------------------------------------------------

func TestEngine_EvidencePackSPRTFields(t *testing.T) {
	statePersistent := IncidentPersistent
	rules := []RuleSpec{
		{
			ID:               "test_evi_rule",
			HazardRuleID:     "evi_001",
			MinSeverity:      types.SeverityHigh,
			RequiredDuration: 1 * time.Second,
			Window:           60 * time.Second,
			Effect: RuleEffect{
				SetLifecycleState:    &statePersistent,
				CreateIncidentIfNone: true,
			},
		},
	}
	e := NewEngine(nil, rules)

	now := time.Now()
	// Feed enough high-confidence observations to cross SPRT alarm threshold
	for i := 0; i < 5; i++ {
		h := types.Hazard{
			ID:                 "evi_h",
			RuleID:             "evi_001",
			Description:        "Evidence SPRT test",
			Severity:           types.SeverityHigh,
			Confidence:         0.95,
			FirstSeenAt:        now.Add(-5 * time.Second),
			LastSeenAt:         now.Add(time.Duration(i) * time.Second),
			PersistenceSeconds: 5 + i,
			CameraID:           "cam1",
			DetectedAt:         now.Add(time.Duration(i) * time.Second),
		}
		e.IngestHazard("evi_sess", h)
	}

	incs, _ := e.GetActiveIncidents("evi_sess")
	if len(incs) == 0 {
		t.Fatalf("expected incident")
	}
	_, pack, _ := e.GetIncidentWithEvidence(incs[0].IncidentID)
	if pack.PeakLLR <= 0 {
		t.Fatalf("expected positive PeakLLR, got %f", pack.PeakLLR)
	}
	if !pack.SPRTConfirmed {
		t.Fatalf("expected SPRTConfirmed=true after high-confidence observations")
	}
	if pack.RiskTrend == "" {
		t.Fatalf("expected non-empty RiskTrend")
	}
}

// ---------------------------------------------------------------------------
// Nil engine safety
// ---------------------------------------------------------------------------

func TestEngine_NilSafety(t *testing.T) {
	var e *engine

	// These should not panic
	e.IngestHazard("s", types.Hazard{})
	e.IngestHazardWithFrames("s", types.Hazard{}, nil)

	incs, err := e.GetActiveIncidents("s")
	if err != nil || incs != nil {
		t.Fatalf("expected nil, nil for nil engine")
	}

	inc, evi, err := e.GetIncidentWithEvidence("x")
	if err != nil || inc.IncidentID != "" || evi.IncidentID != "" {
		t.Fatalf("expected empty results for nil engine")
	}

	sum, err := e.GetIncidentSummary("s", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("expected no error for nil engine summary")
	}
	if sum.SessionID != "s" {
		t.Fatalf("expected sessionID to be set even for nil engine")
	}
}

// ---------------------------------------------------------------------------
// ObservationLLR
// ---------------------------------------------------------------------------

func TestObservationLLR_HighConfidence(t *testing.T) {
	llr := observationLLR(0.95)
	if llr <= 0 {
		t.Fatalf("expected positive LLR for high confidence, got %f", llr)
	}
}

func TestObservationLLR_LowConfidence(t *testing.T) {
	llr := observationLLR(0.05)
	if llr >= 0 {
		t.Fatalf("expected negative LLR for low confidence, got %f", llr)
	}
}

func TestObservationLLR_FiftyFifty(t *testing.T) {
	llr := observationLLR(0.5)
	if math.Abs(llr) > 0.001 {
		t.Fatalf("expected ~0 LLR for 0.5 confidence, got %f", llr)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func lifecycleStates(incs []Incident) []IncidentLifecycleState {
	states := make([]IncidentLifecycleState, len(incs))
	for i, inc := range incs {
		states[i] = inc.LifecycleState
	}
	return states
}
