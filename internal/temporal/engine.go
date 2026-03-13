package temporal

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// SPRT thresholds — Wald sequential probability ratio test.
// α = 0.05 (false alarm rate), β = 0.10 (missed detection rate)
// Alarm threshold A = log((1-β)/α) = log(18)  ≈ 2.89
// Clear threshold  B = log(β/(1-α)) = log(0.105) ≈ -2.25
const (
	sprtAlarmThreshold = 2.89
	sprtClearThreshold = -2.25
	sprtEpsilon        = 1e-6 // prevents log(0)
	confHistMaxSamples = 20   // rolling window size for confidence history
)

// Engine defines the public interface for the temporal incident engine.
type Engine interface {
	// IngestHazard adds a new hazard observation into the temporal engine.
	IngestHazard(sessionID string, hazard types.Hazard)

	// IngestHazardWithFrames is IngestHazard plus frame evidence attachment.
	// Pass recent frames from the session FrameBuffer so the engine can store
	// snapshot references inside the EvidencePack for audit trails.
	IngestHazardWithFrames(sessionID string, hazard types.Hazard, frames []types.Frame)

	// GetActiveIncidents returns incidents currently active for a session.
	GetActiveIncidents(sessionID string) ([]Incident, error)

	// GetIncidentWithEvidence fetches a single incident and its evidence pack.
	GetIncidentWithEvidence(incidentID string) (Incident, EvidencePack, error)

	// GetIncidentSummary returns an aggregate view of incidents for a session
	// and time window, suitable for status queries and reports.
	GetIncidentSummary(sessionID string, from, to time.Time) (Summary, error)
}

// engine is the concrete implementation of Engine.
type engine struct {
	mu    sync.Mutex
	store Store
	rules []RuleSpec

	// sprtState maps a (sessionID, hazardRuleID, cameraID) key to the current
	// cumulative log-likelihood ratio. This is the SPRT accumulator described
	// by Wald (1947) — evidence accumulates across observations until an alarm
	// or clear threshold is crossed.
	sprtState map[string]float64

	// confHist maps the same key to a rolling window of ConfidenceSample values
	// used to compute real trend slopes (escalating / stable / improving).
	confHist map[string][]types.ConfidenceSample
}

// NewEngine constructs a new temporal engine backed by the given store and rules.
func NewEngine(store Store, rules []RuleSpec) Engine {
	if store == nil {
		store = newMemoryStore()
	}
	return &engine{
		store:     store,
		rules:     rules,
		sprtState: make(map[string]float64),
		confHist:  make(map[string][]types.ConfidenceSample),
	}
}

// IngestHazard processes a hazard observation through SPRT accumulation,
// confidence history tracking, and temporal rule evaluation.
func (e *engine) IngestHazard(sessionID string, hazard types.Hazard) {
	e.ingestCore(sessionID, hazard, nil)
}

// IngestHazardWithFrames calls IngestHazard and attaches frame snapshot
// references to the incident EvidencePack for visual audit trails.
func (e *engine) IngestHazardWithFrames(sessionID string, hazard types.Hazard, frames []types.Frame) {
	e.ingestCore(sessionID, hazard, frames)
}

// ingestCore is the shared implementation for both IngestHazard and
// IngestHazardWithFrames.
func (e *engine) ingestCore(sessionID string, hazard types.Hazard, frames []types.Frame) {
	if e == nil {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = hazard.ID
	}

	_ = e.store.SaveHazard(sessionID, hazard)

	now := hazard.DetectedAt
	if now.IsZero() {
		now = time.Now()
	}

	// --- SPRT + confidence history update ---
	stateKey := sprtKey(sessionID, hazard.RuleID, hazard.CameraID)
	e.mu.Lock()
	// Update confidence history
	e.confHist[stateKey] = appendConfSample(e.confHist[stateKey], types.ConfidenceSample{
		Confidence: hazard.Confidence,
		Timestamp:  now,
	})
	// Accumulate SPRT log-likelihood ratio
	llr := e.sprtState[stateKey] + observationLLR(hazard.Confidence)
	e.sprtState[stateKey] = llr
	confSamples := e.confHist[stateKey]
	e.mu.Unlock()

	// Compute real trend from confidence slope
	trend := computeRiskTrend(confSamples)
	// Stamp trend back on the hazard so rules and responses can read it
	hazard.RiskTrend = trend
	hazard.ConfidenceHistory = confSamples

	// Evaluate temporal rules that match this hazard's rule_id
	for i := range e.rules {
		r := e.rules[i]
		if strings.TrimSpace(r.HazardRuleID) == "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(r.HazardRuleID), strings.TrimSpace(hazard.RuleID)) {
			continue
		}
		if !hazardMeetsRuleThresholds(hazard, r) {
			continue
		}
		if !hazardSatisfiesTemporalConditions(hazard, r, now) {
			// SPRT override: if LLR has crossed the alarm threshold, allow incident
			// creation even if time/occurrence conditions aren't fully satisfied yet.
			// This enables early detection on sustained high-confidence observations
			// before the minimum duration window has elapsed.
			if llr < sprtAlarmThreshold {
				continue
			}
		}

		incidentID := buildIncidentID(sessionID, r.ID, hazard)

		incident := Incident{
			IncidentID:     incidentID,
			SessionID:      sessionID,
			PrimaryHazard:  hazard,
			HazardType:     hazard.RuleID,
			Severity:       hazard.Severity,
			LifecycleState: IncidentDetected,
			StartAt:        coalesceTime(hazard.FirstSeenAt, now),
			EndAt:          nil,
			InvolvedHazardIDs: []string{hazard.ID},
			InvolvedCameras: uniqueNonEmpty([]string{hazard.CameraID}),
		}

		// Apply rule effects
		if r.Effect.SetLifecycleState != nil {
			incident.LifecycleState = *r.Effect.SetLifecycleState
		}
		if r.Effect.MarkRecurring {
			incident.LifecycleState = IncidentRecurring
		}
		if r.Effect.SetSeverity != nil && severityRank(*r.Effect.SetSeverity) > severityRank(incident.Severity) {
			incident.Severity = *r.Effect.SetSeverity
		}

		_, _ = e.store.UpsertIncident(incident)

		// Persist / update EvidencePack
		pack, err := e.store.GetEvidence(incidentID)
		if err != nil || pack.IncidentID == "" {
			pack = EvidencePack{
				IncidentID: incidentID,
				FirstSeen:  incident.StartAt,
				PeakTime:   now,
				LastSeen:   now,
			}
		}
		pack.LastSeen = now
		if now.After(pack.PeakTime) && severityRank(incident.Severity) >= severityRank(incident.PrimaryHazard.Severity) {
			pack.PeakTime = now
		}
		pack.RulesTriggered = appendUnique(pack.RulesTriggered, r.ID)
		pack.RiskTrend = trend

		// Update SPRT confirmation fields
		if llr > pack.PeakLLR {
			pack.PeakLLR = llr
		}
		if llr >= sprtAlarmThreshold {
			pack.SPRTConfirmed = true
		}

		// Attach frame evidence snapshots from the rolling FrameBuffer
		if len(frames) > 0 {
			pack.Snapshots = attachFrameSnapshots(pack.Snapshots, frames, incidentID)
		}

		_ = e.store.SaveEvidence(pack)

		// SPRT clear: if the LLR has dropped below the clear threshold, resolve the incident
		if llr <= sprtClearThreshold {
			endAt := now
			incident.EndAt = &endAt
			incident.LifecycleState = IncidentResolved
			_, _ = e.store.UpsertIncident(incident)
			// Reset SPRT state so a new incident can form
			e.mu.Lock()
			e.sprtState[stateKey] = 0
			e.mu.Unlock()
		}
	}
}

func (e *engine) GetActiveIncidents(sessionID string) ([]Incident, error) {
	if e == nil {
		return nil, nil
	}
	return e.store.GetActiveIncidents(sessionID)
}

func (e *engine) GetIncidentWithEvidence(incidentID string) (Incident, EvidencePack, error) {
	var emptyIncident Incident
	var emptyEvidence EvidencePack
	if e == nil {
		return emptyIncident, emptyEvidence, nil
	}
	incident, err := e.store.GetIncident(incidentID)
	if err != nil {
		return emptyIncident, emptyEvidence, err
	}
	evidence, err := e.store.GetEvidence(incidentID)
	if err != nil {
		return incident, emptyEvidence, nil
	}
	return incident, evidence, nil
}

func (e *engine) GetIncidentSummary(sessionID string, from, to time.Time) (Summary, error) {
	summary := Summary{
		SessionID:    sessionID,
		From:         from,
		To:           to,
		ByHazardType: make(map[string]int),
		BySeverity:   make(map[string]int),
	}
	if e == nil {
		return summary, nil
	}
	incidents, err := e.store.GetIncidentsInRange(sessionID, from, to)
	if err != nil {
		return summary, err
	}
	summary.IncidentCount = len(incidents)
	for _, inc := range incidents {
		if inc.HazardType != "" {
			summary.ByHazardType[inc.HazardType]++
		}
		if inc.Severity != "" {
			summary.BySeverity[string(inc.Severity)]++
		}
	}
	return summary, nil
}

// --- SPRT helpers ---

// observationLLR computes the per-observation log-likelihood ratio update.
// p1 = P(observation | hazard present) ≈ confidence
// p0 = P(observation | hazard absent)  ≈ 1 - confidence
// llr_increment = log(p1/p0)
func observationLLR(confidence float64) float64 {
	p1 := math.Max(confidence, sprtEpsilon)
	p0 := math.Max(1.0-confidence, sprtEpsilon)
	return math.Log(p1 / p0)
}

// sprtKey builds the SPRT state key for a (session, hazardRule, camera) triplet.
func sprtKey(sessionID, hazardRuleID, cameraID string) string {
	return strings.Join([]string{sessionID, hazardRuleID, cameraID}, "|")
}

// --- Confidence history helpers ---

// appendConfSample adds a sample to the rolling window, capping at confHistMaxSamples.
func appendConfSample(hist []types.ConfidenceSample, sample types.ConfidenceSample) []types.ConfidenceSample {
	hist = append(hist, sample)
	if len(hist) > confHistMaxSamples {
		hist = hist[len(hist)-confHistMaxSamples:]
	}
	return hist
}

// computeRiskTrend calculates the qualitative trend from a confidence history.
// Uses the slope between the first and last thirds of the window.
// Aligned with Shahar's (1997) temporal abstraction: rate abstraction → trend classification.
func computeRiskTrend(samples []types.ConfidenceSample) string {
	if len(samples) < 3 {
		return "new"
	}
	n := len(samples)
	// Compare mean of last third vs first third to detect directional drift
	third := n / 3
	if third < 1 {
		third = 1
	}
	earlyMean := meanConf(samples[:third])
	lateMean := meanConf(samples[n-third:])
	slope := lateMean - earlyMean
	switch {
	case slope > 0.08:
		return "escalating"
	case slope < -0.08:
		return "improving"
	default:
		return "stable"
	}
}

func meanConf(samples []types.ConfidenceSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range samples {
		sum += s.Confidence
	}
	return sum / float64(len(samples))
}

// --- Frame evidence helpers ---

// attachFrameSnapshots appends new snapshot refs from recent frames, deduplicating by FrameID.
func attachFrameSnapshots(existing []SnapshotRef, frames []types.Frame, _ string) []SnapshotRef {
	seen := make(map[string]bool, len(existing))
	for _, s := range existing {
		seen[s.StorageKey] = true
	}
	for _, f := range frames {
		if f.ID == "" {
			continue
		}
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		existing = append(existing, SnapshotRef{
			CameraID:   f.CameraID,
			Timestamp:  f.Timestamp,
			StorageKey: f.ID,
		})
	}
	// Keep max 10 snapshots per incident (most recent)
	if len(existing) > 10 {
		existing = existing[len(existing)-10:]
	}
	return existing
}

// --- Rule evaluation helpers (unchanged logic, kept here for locality) ---

func hazardMeetsRuleThresholds(h types.Hazard, r RuleSpec) bool {
	if strings.TrimSpace(string(r.MinSeverity)) != "" {
		if severityRank(h.Severity) < severityRank(r.MinSeverity) {
			return false
		}
	}
	if r.MinConfidence > 0 && h.Confidence > 0 && h.Confidence < r.MinConfidence {
		return false
	}
	return true
}

func hazardSatisfiesTemporalConditions(h types.Hazard, r RuleSpec, now time.Time) bool {
	if r.MinOccurrences > 0 {
		if h.Occurrences < r.MinOccurrences {
			return false
		}
	}
	if r.RequiredDuration > 0 {
		if h.PersistenceSeconds > 0 {
			if time.Duration(h.PersistenceSeconds)*time.Second < r.RequiredDuration {
				return false
			}
		} else {
			start := coalesceTime(h.FirstSeenAt, now)
			end := coalesceTime(h.LastSeenAt, now)
			if end.Before(start) {
				end = now
			}
			if end.Sub(start) < r.RequiredDuration {
				return false
			}
		}
	}
	if r.Window > 0 {
		last := coalesceTime(h.LastSeenAt, coalesceTime(h.DetectedAt, now))
		if now.Sub(last) > r.Window {
			return false
		}
	}
	return true
}

func buildIncidentID(sessionID, ruleID string, h types.Hazard) string {
	desc := strings.ToLower(strings.TrimSpace(h.Description))
	desc = strings.ReplaceAll(desc, "|", " ")
	desc = strings.ReplaceAll(desc, "\n", " ")
	return strings.Join([]string{
		"inc",
		strings.TrimSpace(sessionID),
		strings.TrimSpace(ruleID),
		strings.TrimSpace(h.RuleID),
		strings.TrimSpace(h.CameraID),
		desc,
	}, "|")
}

func severityRank(s types.Severity) int {
	switch s {
	case types.SeverityLow:
		return 1
	case types.SeverityMedium:
		return 2
	case types.SeverityHigh:
		return 3
	case types.SeverityCritical:
		return 4
	default:
		return 0
	}
}

func coalesceTime(a, b time.Time) time.Time {
	if !a.IsZero() {
		return a
	}
	return b
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func appendUnique(values []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, v) {
			return values
		}
	}
	return append(values, v)
}

func appendUniqueSlice(values []string, more []string) []string {
	for _, v := range more {
		values = appendUnique(values, v)
	}
	return values
}
