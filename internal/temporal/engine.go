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

	// confHistMaxAge is the maximum age of confidence samples retained in the
	// rolling history. Samples older than this are pruned on each ingestion.
	// A time-bounded window (rather than fixed sample count) ensures trend
	// computation scales correctly across rules with different time windows.
	// Aligned with LogicAgent (arxiv 2602.07689): ground events into
	// discrete, time-bounded units rather than fixed sample counts.
	confHistMaxAge = 5 * time.Minute

	// confHistMaxSamples caps the absolute number of samples to prevent
	// unbounded memory growth within the time window.
	confHistMaxSamples = 200

	// sprtDecayHalfLife controls the exponential time decay applied to the
	// SPRT accumulator. Evidence older than this half-life contributes
	// exponentially less, preventing ghost incidents from stale observations.
	// Aligned with V-CORE (arxiv 2601.01804) block-causal temporal attention
	// and TaYS (arxiv 2603.02872) streaming temporal causality.
	sprtDecayHalfLife = 30 * time.Second

	// autoEscalationLLRThreshold triggers automatic escalation when the SPRT
	// log-likelihood ratio significantly exceeds the alarm threshold.
	// Aligned with Chain of Event-Centric Causal Thought (arxiv 2603.09094).
	autoEscalationLLRThreshold = 5.0

	// autoEscalationDistinctRules is the minimum number of distinct temporal
	// rules that must fire on the same session before auto-escalation triggers.
	autoEscalationDistinctRules = 2
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

// sprtEntry holds the per-key SPRT accumulator state including the timestamp
// of the last observation, used for exponential time decay.
type sprtEntry struct {
	LLR      float64
	LastSeen time.Time
}

// engine is the concrete implementation of Engine.
type engine struct {
	mu    sync.Mutex
	store Store
	rules []RuleSpec

	// sprtState maps a (sessionID, hazardRuleID, cameraID) key to the current
	// SPRT accumulator entry. Evidence accumulates across observations until an
	// alarm or clear threshold is crossed, with exponential time decay applied
	// between observations (Wald 1947 + V-CORE temporal causality).
	sprtState map[string]sprtEntry

	// confHist maps the same key to a time-bounded rolling window of
	// ConfidenceSample values used to compute real trend slopes.
	confHist map[string][]types.ConfidenceSample

	// sessionRulesFired tracks which temporal rule IDs have fired per session,
	// used for auto-escalation when multiple distinct rules converge.
	sessionRulesFired map[string]map[string]bool
}

// NewEngine constructs a new temporal engine backed by the given store and rules.
func NewEngine(store Store, rules []RuleSpec) Engine {
	if store == nil {
		store = newMemoryStore()
	}
	return &engine{
		store:             store,
		rules:             rules,
		sprtState:         make(map[string]sprtEntry),
		confHist:          make(map[string][]types.ConfidenceSample),
		sessionRulesFired: make(map[string]map[string]bool),
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

	// Update time-bounded confidence history (prune stale samples)
	e.confHist[stateKey] = appendConfSample(e.confHist[stateKey], types.ConfidenceSample{
		Confidence: hazard.Confidence,
		Timestamp:  now,
	}, now)

	// Apply exponential time decay to existing SPRT accumulator before adding
	// the new observation. This ensures stale evidence from long-ago
	// observations contributes exponentially less to the current LLR.
	entry := e.sprtState[stateKey]
	decayedLLR := decaySPRT(entry, now)
	llr := decayedLLR + observationLLR(hazard.Confidence)
	e.sprtState[stateKey] = sprtEntry{LLR: llr, LastSeen: now}

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
			IncidentID:        incidentID,
			SessionID:         sessionID,
			PrimaryHazard:     hazard,
			HazardType:        hazard.RuleID,
			Severity:          hazard.Severity,
			LifecycleState:    IncidentDetected,
			StartAt:           coalesceTime(hazard.FirstSeenAt, now),
			EndAt:             nil,
			InvolvedHazardIDs: []string{hazard.ID},
			InvolvedCameras:   uniqueNonEmpty([]string{hazard.CameraID}),
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

		// Auto-escalation: if LLR is significantly above alarm threshold OR
		// multiple distinct rules have fired for this session, escalate.
		e.mu.Lock()
		if e.sessionRulesFired[sessionID] == nil {
			e.sessionRulesFired[sessionID] = make(map[string]bool)
		}
		e.sessionRulesFired[sessionID][r.ID] = true
		distinctFired := len(e.sessionRulesFired[sessionID])
		e.mu.Unlock()

		if llr >= autoEscalationLLRThreshold || distinctFired >= autoEscalationDistinctRules {
			if incident.LifecycleState != IncidentResolved && incident.LifecycleState != IncidentAcknowledged {
				incident.LifecycleState = IncidentEscalated
			}
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
			// Reset SPRT state and confidence history so a new incident can form cleanly
			e.mu.Lock()
			e.sprtState[stateKey] = sprtEntry{}
			e.confHist[stateKey] = nil
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
		// Partial result: return incident even if evidence fetch fails
		return incident, emptyEvidence, nil //nolint:nilerr
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

// decaySPRT applies exponential time decay to an existing SPRT accumulator entry.
// The decay factor is exp(-elapsed * ln(2) / halfLife), so the LLR halves every
// sprtDecayHalfLife interval. If the entry has no previous observation, no decay
// is applied.
func decaySPRT(entry sprtEntry, now time.Time) float64 {
	if entry.LastSeen.IsZero() || entry.LLR == 0 {
		return 0
	}
	elapsed := now.Sub(entry.LastSeen).Seconds()
	if elapsed <= 0 {
		return entry.LLR
	}
	halfLifeSec := sprtDecayHalfLife.Seconds()
	if halfLifeSec <= 0 {
		return entry.LLR
	}
	decayFactor := math.Exp(-elapsed * math.Ln2 / halfLifeSec)
	return entry.LLR * decayFactor
}

// sprtKey builds the SPRT state key for a (session, hazardRule, camera) triplet.
func sprtKey(sessionID, hazardRuleID, cameraID string) string {
	return strings.Join([]string{sessionID, hazardRuleID, cameraID}, "|")
}

// --- Confidence history helpers ---

// appendConfSample adds a sample to the time-bounded rolling window,
// pruning samples older than confHistMaxAge and capping at confHistMaxSamples.
func appendConfSample(hist []types.ConfidenceSample, sample types.ConfidenceSample, now time.Time) []types.ConfidenceSample {
	// Prune samples older than the time window
	cutoff := now.Add(-confHistMaxAge)
	start := 0
	for start < len(hist) && hist[start].Timestamp.Before(cutoff) {
		start++
	}
	if start > 0 {
		hist = hist[start:]
	}

	hist = append(hist, sample)

	// Hard cap to prevent unbounded growth
	if len(hist) > confHistMaxSamples {
		hist = hist[len(hist)-confHistMaxSamples:]
	}
	return hist
}

// computeRiskTrend calculates the qualitative trend from a confidence history
// using an exponentially weighted moving average (EWMA) approach.
// Early samples contribute less than recent ones, making the trend robust
// against outliers and sensitive to recent directional changes.
func computeRiskTrend(samples []types.ConfidenceSample) string {
	if len(samples) < 3 {
		return "new"
	}
	n := len(samples)

	// Use EWMA to compute weighted means of the first and last halves.
	// This gives more weight to recent observations within each half,
	// providing smoother trend detection than simple arithmetic means.
	half := n / 2
	if half < 1 {
		half = 1
	}
	earlyEWMA := ewmaConf(samples[:half])
	lateEWMA := ewmaConf(samples[half:])
	slope := lateEWMA - earlyEWMA

	switch {
	case slope > 0.08:
		return "escalating"
	case slope < -0.08:
		return "improving"
	default:
		return "stable"
	}
}

// ewmaConf computes the exponentially weighted moving average of confidence
// samples. More recent samples (later in the slice) carry higher weight.
func ewmaConf(samples []types.ConfidenceSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	if len(samples) == 1 {
		return samples[0].Confidence
	}
	alpha := 2.0 / (float64(len(samples)) + 1.0)
	ewma := samples[0].Confidence
	for i := 1; i < len(samples); i++ {
		ewma = alpha*samples[i].Confidence + (1-alpha)*ewma
	}
	return ewma
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

// --- Rule evaluation helpers ---

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

// buildIncidentID creates a stable incident key from session, temporal rule,
// hazard rule, and camera. Description is intentionally excluded — Gemini
// describes the same real-world hazard differently across observations, which
// would create duplicate incidents for a single condition.
func buildIncidentID(sessionID, ruleID string, h types.Hazard) string {
	return strings.Join([]string{
		"inc",
		strings.TrimSpace(sessionID),
		strings.TrimSpace(ruleID),
		strings.TrimSpace(h.RuleID),
		strings.TrimSpace(h.CameraID),
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
	out := make([]string, 0, len(values))
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
