package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	geminiPkg "github.com/cutmob/argus/internal/gemini"
	"github.com/cutmob/argus/internal/inspection"
	"github.com/cutmob/argus/internal/reporting"
	"github.com/cutmob/argus/internal/session"
	"github.com/cutmob/argus/internal/temporal"
	"github.com/cutmob/argus/internal/vision"
	"github.com/cutmob/argus/pkg/types"
)

// ControllerConfig holds all dependencies for the agent controller.
type ControllerConfig struct {
	SessionManager *session.Manager
	RuleEngine     *inspection.RuleEngine
	HazardDetector *inspection.HazardDetector
	Detector       *vision.Detector
	ReportBuilder  *reporting.ReportBuilder
	ModuleLoader   *inspection.ModuleLoader
	GeminiClient   *geminiPkg.Client
	TemporalEngine temporal.Engine
	OnResponse     func(sessionID string, resp *AgentResponse)
}

// Controller is the ARGUS agent brain.
// It orchestrates vision analysis, intent handling, Gemini Live sessions,
// and response generation.
type Controller struct {
	sessions     *session.Manager
	rules        *inspection.RuleEngine
	hazards      *inspection.HazardDetector
	detector     *vision.Detector
	reports      *reporting.ReportBuilder
	modules      *inspection.ModuleLoader
	gemini       *geminiPkg.Client
	temporal     temporal.Engine
	intentParser *IntentParser
	responseMgr  *ResponseManager
	onResponse   func(sessionID string, resp *AgentResponse)

	mu             sync.RWMutex
	liveSessions   map[string]*geminiPkg.LiveSession
	// resumeHandles stores the last GoAway resumption handle per session so
	// we can re-establish Gemini Live sessions with temporal state continuity.
	resumeHandles  map[string]string
	// lastInspectCall tracks the last inspect_frame call time per session
	// to enforce a minimum debounce interval between calls.
	lastInspectCall map[string]time.Time
	// dismissedHazards tracks operator-dismissed hazard descriptions per session
	// so they are not re-reported unless conditions materially change.
	dismissedHazards map[string]map[string]string
}

func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		sessions:     cfg.SessionManager,
		rules:        cfg.RuleEngine,
		hazards:      cfg.HazardDetector,
		detector:     cfg.Detector,
		reports:      cfg.ReportBuilder,
		modules:      cfg.ModuleLoader,
		gemini:       cfg.GeminiClient,
		temporal:     cfg.TemporalEngine,
		intentParser: NewIntentParser(),
		responseMgr:  NewResponseManager(),
		onResponse:   cfg.OnResponse,
		liveSessions:     make(map[string]*geminiPkg.LiveSession),
		resumeHandles:    make(map[string]string),
		lastInspectCall:  make(map[string]time.Time),
		dismissedHazards: make(map[string]map[string]string),
	}
}

// HandleFrame processes an incoming video frame through the full pipeline.
func (c *Controller) HandleFrame(sessionID string, frame types.Frame) {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		slog.Warn("frame for unknown session", "session_id", sessionID)
		return
	}

	// Store in rolling buffer for temporal reasoning
	sess.FrameBuffer.Push(frame)

	// Run local vision pipeline (sampling + event detection)
	events := c.detector.ProcessFrame(sessionID, frame)

	for _, event := range events {
		c.processVisionEvent(sess, event)
	}
}

// HandleAudio forwards audio chunks to the Gemini Live session.
// Audio must be raw 16-bit PCM, 16kHz, little-endian, mono.
func (c *Controller) HandleAudio(sessionID string, chunk types.AudioChunk) {
	_, ok := c.sessions.Get(sessionID)
	if !ok {
		return
	}

	c.mu.RLock()
	ls, hasLive := c.liveSessions[sessionID]
	c.mu.RUnlock()

	if !hasLive || !ls.IsActive() {
		return
	}

	if err := ls.SendAudio(chunk.Data); err != nil {
		slog.Error("failed to send audio to gemini",
			"session_id", sessionID,
			"error", err,
		)
	}
}

// HandleEvent processes control events from the WebSocket.
func (c *Controller) HandleEvent(sessionID string, event types.VisionEvent) {
	if event.Type == types.EventUserQuery {
		slog.Debug("user query event", "session_id", sessionID)
	}
}

// HandleIntent processes a parsed user intent.
func (c *Controller) HandleIntent(sessionID string, intent types.AgentIntent) *AgentResponse {
	switch intent.Type {
	case types.IntentStartInspection:
		return c.startInspection(sessionID, intent)
	case types.IntentStopInspection:
		return c.stopInspection(sessionID)
	case types.IntentSwitchMode:
		return c.switchMode(sessionID, intent.Mode)
	case types.IntentExportReport, types.IntentGenerateReport:
		return c.generateReport(sessionID, intent)
	case types.IntentQueryHazards:
		return c.queryHazards(sessionID)
	case types.IntentQueryStatus:
		return c.queryStatus(sessionID)
	case types.IntentOperatorActions:
		return c.operatorActions(sessionID)
	default:
		return c.responseMgr.Conversation("I'm listening. How can I help with the inspection?")
	}
}

// HandleRawText parses natural language and executes the mapped intent.
func (c *Controller) HandleRawText(sessionID string, text string) *AgentResponse {
	intent := c.intentParser.Parse(text)
	intent.RawText = text
	if intent.Type == types.IntentConversation {
		return c.responseMgr.Conversation("I'm listening. Say inspect, report, mode switch, status, or top actions.")
	}
	return c.HandleIntent(sessionID, intent)
}

func (c *Controller) startInspection(sessionID string, intent types.AgentIntent) *AgentResponse {
	mode := intent.Mode
	if mode == "" {
		mode = "general"
	}

	mod, err := c.modules.Load(mode)
	if err != nil {
		return c.responseMgr.Error("I don't have an inspection module for " + mode)
	}

	cameraID := ""
	if intent.Parameters != nil {
		cameraID = intent.Parameters["camera_id"]
	}
	if intent.Parameters == nil {
		intent.Parameters = make(map[string]string)
	}

	sess := c.sessions.Create(session.SessionConfig{
		SessionID:      sessionID,
		InspectionMode: mode,
		RulesetID:      mod.Name + "_v" + mod.Version,
		CameraID:       cameraID,
		BufferSize:     30,
		Metadata:       intent.Parameters,
	})

	c.rules.LoadRules(sessionID, mod.Rules)

	runtimeContext := strings.Join([]string{
		"Session started: " + time.Now().Format("2006-01-02T15:04:05Z07:00"),
		"Rules loaded: " + itoa(len(mod.Rules)),
		c.rules.BuildPromptContext(sessionID),
	}, "\n")
	systemPrompt := geminiPkg.BuildLiveInspectionPrompt(
		mod.SystemPrompt,
		mode,
		cameraID,
		strings.TrimSpace(intent.Parameters["alert_threshold"]),
		runtimeContext,
		c.buildEnvironmentFamiliarity(cameraID),
	)

	// Start Gemini Live session for real-time bidirectional streaming
	ctx := context.Background()
	liveSession, err := geminiPkg.NewLiveSession(ctx, c.gemini, geminiPkg.LiveSessionConfig{
		SessionID:    sessionID,
		SystemPrompt: systemPrompt,
		Tools:        geminiPkg.ArgusTools(),
		OnText:       c.handleGeminiText,
		OnAudio:      c.handleGeminiAudio,
		OnToolCall:   c.handleGeminiToolCall,
		OnTranscript: c.handleGeminiTranscript,
		OnGoAway:     c.handleGeminiGoAway,
	})
	if err != nil {
		slog.Error("failed to start gemini live session",
			"session_id", sessionID,
			"error", err,
		)
		return c.responseMgr.Error("Failed to connect to Gemini Live API: " + err.Error())
	}

	c.mu.Lock()
	c.liveSessions[sessionID] = liveSession
	c.mu.Unlock()

	slog.Info("inspection started",
		"session_id", sess.ID,
		"mode", mode,
		"rules_count", len(mod.Rules),
	)

	return c.responseMgr.Voice(
		"Starting " + mode + " inspection. I have " +
			itoa(len(mod.Rules)) + " rules loaded. Point the camera and I'll begin analyzing.",
	)
}

func (c *Controller) UpdateSessionPreferences(sessionID string, prefs map[string]string) *AgentResponse {
	if len(prefs) == 0 {
		return nil
	}
	if !c.sessions.UpdateMetadata(sessionID, prefs) {
		return nil
	}

	c.mu.RLock()
	ls, hasLive := c.liveSessions[sessionID]
	c.mu.RUnlock()

	if hasLive && ls.IsActive() {
		parts := make([]string, 0, len(prefs))
		if threshold := strings.TrimSpace(prefs["alert_threshold"]); threshold != "" {
			parts = append(parts, "stay silent by default and speak proactive findings only for "+threshold+" severity and above")
		}
		if len(parts) > 0 {
			_ = ls.SendText("Operator preferences updated: " + strings.Join(parts, ". "))
		}
	}
	return nil
}

func (c *Controller) stopInspection(sessionID string) *AgentResponse {
	c.mu.Lock()
	if ls, ok := c.liveSessions[sessionID]; ok {
		ls.Close()
		delete(c.liveSessions, sessionID)
	}
	c.mu.Unlock()

	sess, ok := c.sessions.Close(sessionID)
	if !ok {
		return c.responseMgr.Error("No active inspection to stop.")
	}

	c.rules.ClearSession(sessionID)

	return c.responseMgr.Voice(
		"Inspection complete. Found " + itoa(len(sess.Hazards)) +
			" issues. Say 'generate report' to create the inspection report.",
	)
}

// resolveModeAlias normalises voice-command mode names to canonical module names.
// Delegates to the single source of truth in the inspection package.
func resolveModeAlias(mode string) string {
	return inspection.ResolveModeAlias(mode)
}

func (c *Controller) switchMode(sessionID string, mode string) *AgentResponse {
	mode = resolveModeAlias(mode)
	mod, err := c.modules.Load(mode)
	if err != nil {
		available := c.modules.ListAvailable()
		return c.responseMgr.Error(
			"Module '" + mode + "' not found. Available: " + joinStrings(available),
		)
	}

	c.rules.LoadRules(sessionID, mod.Rules)

	c.mu.RLock()
	ls, hasLive := c.liveSessions[sessionID]
	c.mu.RUnlock()

	if hasLive && ls.IsActive() {
		switchMsg := "Inspection mode changed to " + mode + ". New rules:\n"
		for i, r := range mod.Rules {
			switchMsg += itoa(i+1) + ". [" + string(r.Severity) + "] " + r.Description + "\n"
		}
		if err := ls.SendText(switchMsg); err != nil {
			slog.Error("failed to notify live session of mode switch", "error", err)
		}
	}

	return c.responseMgr.Voice(
		"Switched to " + mode + " inspection mode. " +
			itoa(len(mod.Rules)) + " rules active.",
	)
}

func (c *Controller) generateReport(sessionID string, intent types.AgentIntent) *AgentResponse {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		return c.responseMgr.Error("No active session found.")
	}

	format := intent.Format
	if format == "" {
		format = "json"
	}

	report := types.InspectionReport{
		ID:             sessionID + "_report",
		SessionID:      sessionID,
		InspectionMode: sess.InspectionMode,
		Hazards:        sess.Hazards,
		RiskLevel:      c.hazards.CalculateRiskLevel(sess.Hazards),
		RiskScore:      c.hazards.CalculateRiskScore(sess.Hazards),
		CreatedAt:      time.Now(),
	}

	filename, err := c.reports.Build(report, format)
	if err != nil {
		return c.responseMgr.Error("Failed to generate report: " + err.Error())
	}

	summary := "Report generated with " + itoa(len(report.Hazards)) +
		" findings. Risk level: " + string(report.RiskLevel) + "."
	downloadURL := ""
	if filename != "" {
		downloadURL = "/api/v1/reports/files/" + filename
	}
	return c.responseMgr.ReportReady(report.ID, summary, downloadURL)
}

func (c *Controller) queryHazards(sessionID string) *AgentResponse {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		return c.responseMgr.Error("No active session.")
	}

	if len(sess.Hazards) == 0 && c.temporal == nil {
		return c.responseMgr.Voice("No hazards detected so far.")
	}

	// If a temporal engine is available, prefer incident-based summary.
	if c.temporal != nil {
		incidents, err := c.temporal.GetActiveIncidents(sessionID)
		if err == nil && len(incidents) > 0 {
			total := len(incidents)
			persistent := 0
			recurring := 0
			critical := 0
			for _, inc := range incidents {
				if inc.LifecycleState == temporal.IncidentPersistent {
					persistent++
				}
				if inc.LifecycleState == temporal.IncidentRecurring || inc.LifecycleState == temporal.IncidentEscalated {
					recurring++
				}
				if inc.Severity == types.SeverityCritical || inc.Severity == types.SeverityHigh {
					critical++
				}
			}

			summary := "Active inspection with " + itoa(total) + " incident-level findings. "
			if critical > 0 {
				summary += itoa(critical) + " are high or critical severity. "
			}
			if persistent > 0 {
				summary += itoa(persistent) + " are persistent over time. "
			}
			if recurring > 0 {
				summary += itoa(recurring) + " are recurring or escalating patterns."
			}
			return c.responseMgr.Voice(strings.TrimSpace(summary))
		}
	}

	// Fallback to session hazard-based summary.
	if len(sess.Hazards) == 0 {
		return c.responseMgr.Voice("No hazards detected so far.")
	}

	summary := itoa(len(sess.Hazards)) + " hazards detected. "
	high := 0
	for _, h := range sess.Hazards {
		if h.Severity == types.SeverityHigh || h.Severity == types.SeverityCritical {
			high++
		}
	}
	if high > 0 {
		summary += itoa(high) + " are high severity or above."
	}
	for _, h := range sess.Hazards {
		if h.PersistenceSeconds >= 15 {
			summary += " Persistent hazard: " + h.Description +
				" for " + itoa(h.PersistenceSeconds) + " seconds, trend " + h.RiskTrend + "."
			break
		}
	}

	return c.responseMgr.Voice(summary)
}

func (c *Controller) queryStatus(sessionID string) *AgentResponse {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		return c.responseMgr.Voice("No active inspection session.")
	}

	base := "Active " + sess.InspectionMode + " inspection. " +
		itoa(len(sess.Hazards)) + " hazards found. " +
		"Risk score: " + ftoa(sess.RiskScore) + "."

	// Enrich with temporal incident summary when available.
	if c.temporal != nil {
		if summary, err := c.temporal.GetIncidentSummary(sessionID, time.Now().Add(-1*time.Hour), time.Now()); err == nil {
			if summary.IncidentCount > 0 {
				base += " " + itoa(summary.IncidentCount) + " incident-level findings over the last hour."
			}
		}
	}

	return c.responseMgr.Voice(base)
}

func (c *Controller) operatorActions(sessionID string) *AgentResponse {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		return c.responseMgr.Error("No active inspection session.")
	}
	if len(sess.Hazards) == 0 {
		return c.responseMgr.OperatorActions("No immediate actions. Continue monitoring and keep exits clear.", nil)
	}

	actions := make([]types.ActionCard, 0, 3)
	for i := 0; i < len(sess.Hazards) && len(actions) < 3; i++ {
		h := sess.Hazards[i]
		priority := "monitor"
		switch h.Severity {
		case types.SeverityCritical:
			priority = "immediate"
		case types.SeverityHigh:
			priority = "urgent"
		case types.SeverityMedium:
			priority = "high"
		}

		card := types.ActionCard{
			Title:       "Mitigate: " + h.Description,
			Priority:    priority,
			Reason:      "Rule " + h.RuleID + ", confidence " + ftoa(h.Confidence*100) + "%, camera " + h.CameraID,
			CameraID:    h.CameraID,
			HazardRefID: h.ID,
			Actions: []string{
				"Secure the area and warn nearby personnel",
				"Assign a supervisor to verify mitigation now",
				"Re-scan this zone after corrective action",
			},
		}
		if h.PersistenceSeconds >= 10 {
			card.Reason += ", persisted " + itoa(h.PersistenceSeconds) + "s (" + h.RiskTrend + ")"
		}
		actions = append(actions, card)
	}

	return c.responseMgr.OperatorActions(
		"Top "+itoa(len(actions))+" immediate actions generated for this zone.",
		actions,
	)
}

// processVisionEvent handles events from the local vision pipeline.
func (c *Controller) processVisionEvent(sess *session.Session, event types.VisionEvent) {
	switch event.Type {
	case types.EventHazardCandidate, types.EventSceneChange, types.EventPeriodicSample:
		c.sendFrameToGemini(sess, event)
	}
}

// sendFrameToGemini sends a frame to Gemini for analysis.
// Uses the Live session if active, falls back to one-shot GenerateContent.
func (c *Controller) sendFrameToGemini(sess *session.Session, event types.VisionEvent) {
	c.mu.RLock()
	ls, hasLive := c.liveSessions[sess.ID]
	c.mu.RUnlock()

	// Live session path: stream the frame with spatial/temporal context
	if hasLive && ls.IsActive() && event.Frame != nil {
		// Inject timestamp + camera context so Gemini can reason about time and space
		meta := "[FRAME " + event.Frame.Timestamp.Format("2006-01-02T15:04:05Z07:00") +
			" | camera:" + event.Frame.CameraID + "]"
		if err := ls.SendText(meta); err != nil {
			slog.Debug("failed to send frame metadata", "error", err)
		}
		if err := ls.SendVideoFrame(event.Frame.Data); err != nil {
			slog.Error("failed to send frame to gemini live",
				"session_id", sess.ID,
				"error", err,
			)
		}
		return
	}

	// Fallback: one-shot GenerateContent for frame analysis
	if event.Frame != nil && c.gemini != nil {
		rules := c.rules.GetRules(sess.ID)
		req := types.GeminiRequest{
			SessionID: sess.ID,
			Frame:     event.Frame,
			Objects:   event.Objects,
			Rules:     rules,
			Context:   c.rules.BuildPromptContext(sess.ID),
		}

		ctx := context.Background()
		resp, err := c.gemini.AnalyzeFrame(ctx, req)
		if err != nil {
			slog.Error("gemini frame analysis failed",
				"session_id", sess.ID,
				"error", err,
			)
			return
		}

		c.processGeminiResponse(sess.ID, resp)
	}
}

// processGeminiResponse handles a structured response from Gemini.
func (c *Controller) processGeminiResponse(sessionID string, resp *types.GeminiResponse) {
	if resp == nil {
		return
	}

	// Grab recent frames once — used for FrameBuffer → EvidencePack wiring.
	var recentFrames []types.Frame
	if sess, ok := c.sessions.Get(sessionID); ok && sess.FrameBuffer != nil {
		recentFrames = sess.FrameBuffer.Recent(5)
	}

	incidentChanged := false
	for _, h := range resp.Hazards {
		c.sessions.AddHazard(sessionID, h)
		// GAP 5: feed each hazard into the temporal engine with frame evidence so
		// SPRT accumulation, confidence history, and EvidencePack snapshots are live.
		if c.temporal != nil {
			c.temporal.IngestHazardWithFrames(sessionID, h, recentFrames)
			incidentChanged = true
		}
	}

	// GAP 7: push an incidents_update message whenever incidents may have changed
	// so the frontend timeline panel stays current without polling.
	if incidentChanged && c.onResponse != nil {
		c.pushIncidentsUpdate(sessionID)
	}

	if c.onResponse == nil {
		return
	}

	text := strings.TrimSpace(resp.TextResponse)
	voice := strings.TrimSpace(resp.VoiceResponse)
	message := voice
	if message == "" {
		message = text
	}

	if len(resp.Hazards) > 0 {
		alertResp := c.responseMgr.HazardAlert(message, resp.Hazards)
		if !c.shouldSpeakFindingsForSession(sessionID, resp.Hazards) {
			alertResp.Voice = ""
		}
		c.onResponse(sessionID, alertResp)
		return
	}

	if message != "" && voice != "" {
		c.onResponse(sessionID, c.responseMgr.Voice(message))
	}
}

// pushIncidentsUpdate emits an incidents_update WebSocket message containing
// all active incidents so the frontend IncidentTimeline panel can re-render.
func (c *Controller) pushIncidentsUpdate(sessionID string) {
	if c.temporal == nil || c.onResponse == nil {
		return
	}
	incidents, err := c.temporal.GetActiveIncidents(sessionID)
	if err != nil || len(incidents) == 0 {
		return
	}

	type incidentPush struct {
		ID             string  `json:"incident_id"`
		HazardType     string  `json:"hazard_type"`
		Severity       string  `json:"severity"`
		LifecycleState string  `json:"lifecycle_state"`
		StartAt        string  `json:"start_at"`
		LastSeen       string  `json:"last_seen,omitempty"`
		DurationSecs   float64 `json:"duration_seconds,omitempty"`
		RulesTriggered []string `json:"rules_triggered,omitempty"`
		PeakLLR        float64 `json:"peak_llr,omitempty"`
		SPRTConfirmed  bool    `json:"sprt_confirmed,omitempty"`
		RiskTrend      string  `json:"risk_trend,omitempty"`
		Cameras        []string `json:"cameras,omitempty"`
		SnapshotCount  int     `json:"snapshot_count,omitempty"`
	}

	out := make([]incidentPush, 0, len(incidents))
	for _, inc := range incidents {
		p := incidentPush{
			ID:             inc.IncidentID,
			HazardType:     inc.HazardType,
			Severity:       string(inc.Severity),
			LifecycleState: string(inc.LifecycleState),
			StartAt:        inc.StartAt.Format(time.RFC3339),
			Cameras:        inc.InvolvedCameras,
		}
		if ev, err2 := func() (temporal.EvidencePack, error) {
			_, ev, err := c.temporal.GetIncidentWithEvidence(inc.IncidentID)
			return ev, err
		}(); err2 == nil && ev.IncidentID != "" {
			p.LastSeen = ev.LastSeen.Format(time.RFC3339)
			p.DurationSecs = ev.LastSeen.Sub(ev.FirstSeen).Seconds()
			p.RulesTriggered = ev.RulesTriggered
			p.PeakLLR = ev.PeakLLR
			p.SPRTConfirmed = ev.SPRTConfirmed
			p.RiskTrend = ev.RiskTrend
			p.SnapshotCount = len(ev.Snapshots)
		}
		out = append(out, p)
	}

	c.onResponse(sessionID, &AgentResponse{
		Type:      "incidents_update",
		Incidents: out,
	})
}

func (c *Controller) shouldSpeakFindingsForSession(sessionID string, hazards []types.Hazard) bool {
	sess, ok := c.sessions.Get(sessionID)
	if !ok || sess == nil {
		return c.hazards.ShouldAlert(hazards)
	}
	threshold := "high"
	if sess.Metadata != nil && strings.TrimSpace(sess.Metadata["alert_threshold"]) != "" {
		threshold = strings.TrimSpace(sess.Metadata["alert_threshold"])
	}
	return hazardsMeetThreshold(hazards, threshold)
}

func hazardsMeetThreshold(hazards []types.Hazard, threshold string) bool {
	if strings.EqualFold(threshold, "off") {
		return false
	}
	minRank := thresholdRank(threshold)
	for _, hazard := range hazards {
		if thresholdRank(string(hazard.Severity)) >= minRank {
			return true
		}
	}
	return false
}

func thresholdRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 3
	}
}

func (c *Controller) buildEnvironmentFamiliarity(cameraID string) string {
	if strings.TrimSpace(cameraID) == "" {
		return ""
	}
	profile, ok := c.sessions.GetEnvironmentProfile(cameraID)
	if !ok || profile == nil {
		return ""
	}
	parts := []string{
		"Camera " + cameraID + " has " + itoa(profile.InspectionCount) + " prior inspection session(s) in this runtime.",
	}
	if profile.LastInspectionMode != "" {
		parts = append(parts, "Most recent inspection mode: "+profile.LastInspectionMode+".")
	}
	if len(profile.FamiliarHazards) > 0 {
		limit := len(profile.FamiliarHazards)
		if limit > 5 {
			limit = 5
		}
		hazardParts := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			h := profile.FamiliarHazards[i]
			hazardParts = append(hazardParts, h.Description+" ("+itoa(h.Count)+"x, highest "+string(h.HighestSeverity)+")")
		}
		parts = append(parts, "Recurring environment patterns: "+strings.Join(hazardParts, "; ")+".")
	}
	return strings.Join(parts, "\n")
}

// --- Gemini Live session callbacks ---

// handleGeminiGoAway is called when the Gemini Live server signals an imminent
// disconnection. We store the resumption handle and schedule a transparent
// reconnect that re-injects temporal state as context.
func (c *Controller) handleGeminiGoAway(sessionID, handle string) {
	slog.Warn("gemini goaway — scheduling reconnect", "session_id", sessionID)
	c.mu.Lock()
	if handle != "" {
		c.resumeHandles[sessionID] = handle
	}
	c.mu.Unlock()

	go func() {
		time.Sleep(500 * time.Millisecond)
		sess, ok := c.sessions.Get(sessionID)
		if !ok {
			return
		}
		mod, err := c.modules.Load(sess.InspectionMode)
		if err != nil {
			return
		}
		alertThreshold := ""
		if sess.Metadata != nil {
			alertThreshold = sess.Metadata["alert_threshold"]
		}
		runtimeContext := strings.Join([]string{
			"Session resumed: " + time.Now().Format("2006-01-02T15:04:05Z07:00"),
			"Rules loaded: " + itoa(len(mod.Rules)),
			c.rules.BuildPromptContext(sessionID),
			c.buildTemporalResumeContext(sessionID),
		}, "\n")
		systemPrompt := geminiPkg.BuildLiveInspectionPrompt(
			mod.SystemPrompt, sess.InspectionMode, sess.CameraID,
			alertThreshold, runtimeContext,
			c.buildEnvironmentFamiliarity(sess.CameraID),
		)
		ctx := context.Background()
		newLS, err := geminiPkg.NewLiveSession(ctx, c.gemini, geminiPkg.LiveSessionConfig{
			SessionID:    sessionID,
			SystemPrompt: systemPrompt,
			Tools:        geminiPkg.ArgusTools(),
			OnText:       c.handleGeminiText,
			OnAudio:      c.handleGeminiAudio,
			OnToolCall:   c.handleGeminiToolCall,
			OnTranscript: c.handleGeminiTranscript,
			OnGoAway:     c.handleGeminiGoAway,
		})
		if err != nil {
			slog.Error("failed to reconnect gemini live after goaway", "session_id", sessionID, "error", err)
			return
		}
		c.mu.Lock()
		c.liveSessions[sessionID] = newLS
		c.mu.Unlock()
		slog.Info("gemini live reconnected after goaway", "session_id", sessionID)
	}()
}

// buildTemporalResumeContext serializes active incident state into a compact
// text block for injection into a resumed Gemini Live session. Implements the
// "memory injection on reconnect" pattern from NeurIPS 2024 streaming video
// understanding research and Google Live API best-practices documentation.
func (c *Controller) buildTemporalResumeContext(sessionID string) string {
	if c.temporal == nil {
		return ""
	}
	incidents, err := c.temporal.GetActiveIncidents(sessionID)
	if err != nil || len(incidents) == 0 {
		return ""
	}
	lines := []string{
		"TEMPORAL CONTEXT RESUME [" + time.Now().Format("2006-01-02T15:04:05Z07:00") + "]",
		"The following incidents were active when the session was interrupted.",
		"Treat these as known, ongoing incidents unless you observe them resolved:",
	}
	for _, inc := range incidents {
		line := "- " + string(inc.Severity) + " | " + string(inc.LifecycleState) +
			" | " + inc.HazardType + " | started " + inc.StartAt.Format("15:04:05")
		if len(inc.InvolvedCameras) > 0 {
			line += " | camera: " + strings.Join(inc.InvolvedCameras, ",")
		}
		if _, ev, evErr := c.temporal.GetIncidentWithEvidence(inc.IncidentID); evErr == nil && ev.IncidentID != "" {
			line += " | trend: " + ev.RiskTrend
			if ev.SPRTConfirmed {
				line += " | SPRT-confirmed"
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (c *Controller) handleGeminiText(sessionID, text string) {
	slog.Debug("gemini text response", "session_id", sessionID, "text", text)

	var resp types.GeminiResponse
	if err := json.Unmarshal([]byte(text), &resp); err == nil {
		c.processGeminiResponse(sessionID, &resp)
		return
	}

	if c.onResponse != nil {
		c.onResponse(sessionID, c.responseMgr.Voice(text))
	}
}

func (c *Controller) handleGeminiAudio(sessionID string, data []byte) {
	if c.onResponse != nil {
		c.onResponse(sessionID, &AgentResponse{
			Type:      "audio",
			AudioData: data,
		})
	}
}

func (c *Controller) handleGeminiToolCall(sessionID string, calls []*genai.FunctionCall) {
	slog.Info("gemini tool call", "session_id", sessionID, "count", len(calls))

	responses := make([]*genai.FunctionResponse, 0, len(calls))

	for _, call := range calls {
		result := c.executeToolCall(sessionID, *call)
		responses = append(responses, &genai.FunctionResponse{
			ID:       call.ID,
			Name:     call.Name,
			Response: result,
		})
	}

	c.mu.RLock()
	ls, ok := c.liveSessions[sessionID]
	c.mu.RUnlock()

	if ok && ls.IsActive() {
		if err := ls.SendToolResponse(responses); err != nil {
			slog.Error("failed to send tool response", "session_id", sessionID, "error", err)
		}
	}
}

func (c *Controller) handleGeminiTranscript(sessionID, speaker, text string) {
	slog.Info("transcript",
		"session_id", sessionID,
		"speaker", speaker,
		"text", text,
	)

	if c.onResponse != nil && text != "" {
		c.onResponse(sessionID, &AgentResponse{
			Type:    "transcript",
			Text:    text,
			Speaker: speaker,
		})
	}

	// Do NOT run intent parsing on Live session transcripts — Gemini already
	// handles commands natively via function calls. Running the intent parser
	// here would duplicate tool-call actions (e.g. generating two reports).
}

// --- Tool execution ---

func (c *Controller) executeToolCall(sessionID string, call genai.FunctionCall) map[string]any {
	switch call.Name {
	case "inspect_frame":
		return c.toolInspectFrame(sessionID, call.Args)
	case "highlight_hazard":
		return c.toolHighlightHazard(sessionID, call.Args)
	case "switch_inspection_mode":
		return c.toolSwitchMode(sessionID, call.Args)
	case "generate_report":
		return c.toolGenerateReport(sessionID, call.Args)
	case "log_issue":
		return c.toolLogIssue(sessionID, call.Args)
	case "get_inspection_status":
		return c.toolGetStatus(sessionID)
	case "get_incidents":
		return c.toolGetIncidents(sessionID)
	case "dismiss_finding":
		return c.toolDismissFinding(sessionID, call.Args)
	default:
		return map[string]any{"error": "unknown tool: " + call.Name}
	}
}

func (c *Controller) toolGetIncidents(sessionID string) map[string]any {
	if c.temporal == nil {
		return map[string]any{"status": "temporal_engine_unavailable"}
	}

	incidents, err := c.temporal.GetActiveIncidents(sessionID)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	type incidentView struct {
		ID             string `json:"id"`
		HazardType     string `json:"hazard_type"`
		Severity       string `json:"severity"`
		LifecycleState string `json:"lifecycle_state"`
		StartAt        string `json:"start_at"`
		LastSeen       string `json:"last_seen"`
	}

	out := make([]incidentView, 0, len(incidents))
	for _, inc := range incidents {
		lastSeen := ""
		if _, evidence, err := c.temporal.GetIncidentWithEvidence(inc.IncidentID); err == nil {
			lastSeen = evidence.LastSeen.Format(time.RFC3339)
		}
		out = append(out, incidentView{
			ID:             inc.IncidentID,
			HazardType:     inc.HazardType,
			Severity:       string(inc.Severity),
			LifecycleState: string(inc.LifecycleState),
			StartAt:        inc.StartAt.Format(time.RFC3339),
			LastSeen:       lastSeen,
		})
	}

	return map[string]any{
		"status":    "ok",
		"incidents": out,
	}
}

const inspectFrameDebounce = 2 * time.Second

func (c *Controller) toolInspectFrame(sessionID string, args map[string]any) map[string]any {
	// Debounce: reject calls that arrive faster than the minimum interval
	// to prevent model over-calling from flooding the temporal engine.
	c.mu.Lock()
	if last, ok := c.lastInspectCall[sessionID]; ok && time.Since(last) < inspectFrameDebounce {
		c.mu.Unlock()
		return map[string]any{"status": "debounced", "message": "inspect_frame called too frequently, wait briefly"}
	}
	c.lastInspectCall[sessionID] = time.Now()
	c.mu.Unlock()

	hazardsRaw, ok := args["hazards"]
	if !ok {
		return map[string]any{"status": "no hazards provided"}
	}

	hazardsJSON, err := json.Marshal(hazardsRaw)
	if err != nil {
		return map[string]any{"error": "invalid hazards format"}
	}

	var hazardInputs []struct {
		Description string  `json:"description"`
		Severity    string  `json:"severity"`
		Confidence  float64 `json:"confidence"`
		RuleID      string  `json:"rule_id"`
		Location    string  `json:"location"`
	}
	if err := json.Unmarshal(hazardsJSON, &hazardInputs); err != nil {
		return map[string]any{"error": "failed to parse hazards"}
	}

	sess, _ := c.sessions.Get(sessionID)
	camID := ""
	if sess != nil {
		camID = sess.CameraID
	}
	// Grab recent frames for temporal engine evidence attachment
	var recentFrames []types.Frame
	if sess != nil && sess.FrameBuffer != nil {
		recentFrames = sess.FrameBuffer.Recent(5)
	}

	for _, h := range hazardInputs {
		hazard := types.Hazard{
			ID:          sessionID + "_" + itoa(int(time.Now().UnixMilli())),
			RuleID:      h.RuleID,
			Description: h.Description,
			Severity:    types.Severity(h.Severity),
			Confidence:  h.Confidence,
			Location:    h.Location,
			CameraID:    camID,
			DetectedAt:  time.Now(),
		}
		c.sessions.AddHazard(sessionID, hazard)

		// Feed into temporal engine for SPRT accumulation and incident tracking
		if c.temporal != nil {
			c.temporal.IngestHazardWithFrames(sessionID, hazard, recentFrames)
		}
	}

	// Push incident update so frontend timeline stays current
	if len(hazardInputs) > 0 && c.temporal != nil {
		c.pushIncidentsUpdate(sessionID)
	}

	return map[string]any{"status": "logged", "count": len(hazardInputs)}
}

func (c *Controller) toolHighlightHazard(sessionID string, args map[string]any) map[string]any {
	label, _ := args["label"].(string)
	severity, _ := args["severity"].(string)
	location, _ := args["location"].(string)

	color := "#ffcc00"
	switch types.Severity(severity) {
	case types.SeverityCritical:
		color = "#ff0000"
	case types.SeverityHigh:
		color = "#ff4444"
	case types.SeverityMedium:
		color = "#ffaa00"
	}

	displayLabel := label
	if location != "" {
		displayLabel = label + " — " + location
	}

	overlay := types.Overlay{
		Type:     "hazard_box",
		Label:    displayLabel,
		Severity: types.Severity(severity),
		Color:    color,
	}

	// Parse Gemini box_2d [ymin, xmin, ymax, xmax] (0-1000) → normalized 0-1
	if raw, ok := args["box_2d"]; ok {
		if arr, ok := raw.([]any); ok && len(arr) >= 4 {
			toF := func(v any) float64 {
				switch n := v.(type) {
				case float64:
					return n
				case int:
					return float64(n)
				default:
					return 0
				}
			}
			ymin := toF(arr[0]) / 1000.0
			xmin := toF(arr[1]) / 1000.0
			ymax := toF(arr[2]) / 1000.0
			xmax := toF(arr[3]) / 1000.0
			overlay.BBox = &types.BBox{
				X:      xmin,
				Y:      ymin,
				Width:  xmax - xmin,
				Height: ymax - ymin,
			}
		}
	}

	if c.onResponse != nil {
		c.onResponse(sessionID, &AgentResponse{
			Type:     "overlay",
			Overlays: []types.Overlay{overlay},
		})
	}

	return map[string]any{"status": "highlighted", "label": label}
}

func (c *Controller) toolSwitchMode(sessionID string, args map[string]any) map[string]any {
	mode, _ := args["mode"].(string)
	resp := c.switchMode(sessionID, mode)
	return map[string]any{"status": "switched", "mode": mode, "message": resp.Text}
}

func (c *Controller) toolGenerateReport(sessionID string, args map[string]any) map[string]any {
	format, _ := args["format"].(string)
	if format == "" {
		format = "json"
	}
	intent := types.AgentIntent{Type: types.IntentGenerateReport, Format: format}
	resp := c.generateReport(sessionID, intent)
	// Push report_ready to the client immediately so the frontend can show the download link.
	if c.onResponse != nil {
		c.onResponse(sessionID, resp)
	}
	result := map[string]any{"status": "generated", "message": resp.Text}
	if resp.DownloadURL != "" {
		result["download_url"] = resp.DownloadURL
	}
	return result
}

func (c *Controller) toolLogIssue(sessionID string, args map[string]any) map[string]any {
	desc, _ := args["description"].(string)
	sev, _ := args["severity"].(string)
	conf, _ := args["confidence"].(float64)
	ruleID, _ := args["rule_id"].(string)

	logSess, _ := c.sessions.Get(sessionID)
	logCamID := ""
	if logSess != nil {
		logCamID = logSess.CameraID
	}
	c.sessions.AddHazard(sessionID, types.Hazard{
		ID:          sessionID + "_" + itoa(int(time.Now().UnixMilli())),
		RuleID:      ruleID,
		Description: desc,
		Severity:    types.Severity(sev),
		Confidence:  conf,
		CameraID:    logCamID,
		DetectedAt:  time.Now(),
	})

	return map[string]any{"status": "logged", "description": desc}
}

func (c *Controller) toolGetStatus(sessionID string) map[string]any {
	sess, ok := c.sessions.Get(sessionID)
	if !ok {
		return map[string]any{"error": "no active session"}
	}

	result := map[string]any{
		"mode":         sess.InspectionMode,
		"hazard_count": len(sess.Hazards),
		"risk_score":   c.hazards.CalculateRiskScore(sess.Hazards),
		"risk_level":   string(c.hazards.CalculateRiskLevel(sess.Hazards)),
		"state":        string(sess.State),
	}

	if c.temporal != nil {
		if summary, err := c.temporal.GetIncidentSummary(sessionID, time.Now().Add(-1*time.Hour), time.Now()); err == nil {
			result["incident_count"] = summary.IncidentCount
			result["incident_by_severity"] = summary.BySeverity
			result["incident_by_hazard_type"] = summary.ByHazardType
		}
	}

	return result
}

func (c *Controller) toolDismissFinding(sessionID string, args map[string]any) map[string]any {
	desc, _ := args["hazard_description"].(string)
	reason, _ := args["reason"].(string)
	if strings.TrimSpace(desc) == "" {
		return map[string]any{"error": "hazard_description is required"}
	}

	c.mu.Lock()
	if c.dismissedHazards[sessionID] == nil {
		c.dismissedHazards[sessionID] = make(map[string]string)
	}
	c.dismissedHazards[sessionID][strings.ToLower(strings.TrimSpace(desc))] = reason
	c.mu.Unlock()

	slog.Info("hazard dismissed by operator",
		"session_id", sessionID,
		"description", desc,
		"reason", reason,
	)

	return map[string]any{"status": "dismissed", "description": desc, "reason": reason}
}
