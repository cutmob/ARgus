package reporting

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// maxStoredReports is the upper bound on in-memory report retention.
// When exceeded the oldest reports are evicted first.
const maxStoredReports = 500

// ReportBuilder constructs and stores inspection reports.
type ReportBuilder struct {
	mu       sync.RWMutex
	reports  map[string]*types.InspectionReport
	registry *ExportRegistry
}

func NewReportBuilder(registry *ExportRegistry) *ReportBuilder {
	return &ReportBuilder{
		reports:  make(map[string]*types.InspectionReport),
		registry: registry,
	}
}

// Build creates a report and exports it in the specified format.
// Returns the output filename (basename) for file-based exporters, or "" for webhook/none.
func (rb *ReportBuilder) Build(report types.InspectionReport, format string) (string, error) {
	if report.ID == "" {
		report.ID = fmt.Sprintf("report_%d", time.Now().UnixMilli())
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now()
	}

	// Generate summary and recommendations
	report.Summary = rb.generateSummary(report)
	report.Recommendations = rb.generateRecommendations(report)

	// Store — evict oldest if at capacity
	rb.mu.Lock()
	if len(rb.reports) >= maxStoredReports {
		rb.evictOldestLocked()
	}
	rb.reports[report.ID] = &report
	rb.mu.Unlock()

	slog.Info("report built",
		"id", report.ID,
		"mode", report.InspectionMode,
		"hazards", len(report.Hazards),
		"risk_level", report.RiskLevel,
	)

	// Export
	if format != "" {
		return rb.registry.Export(format, report)
	}
	return "", nil
}

// Get retrieves a stored report by ID.
func (rb *ReportBuilder) Get(id string) (*types.InspectionReport, bool) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	r, ok := rb.reports[id]
	return r, ok
}

// List returns all stored reports.
func (rb *ReportBuilder) List() []*types.InspectionReport {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	result := make([]*types.InspectionReport, 0, len(rb.reports))
	for _, r := range rb.reports {
		result = append(result, r)
	}
	return result
}

// Export sends an existing report through an exporter and returns the output filename.
func (rb *ReportBuilder) Export(reportID string, format string) (string, error) {
	report, ok := rb.Get(reportID)
	if !ok {
		return "", fmt.Errorf("report %s not found", reportID)
	}
	return rb.registry.Export(format, *report)
}

// ---------------------------------------------------------------------------
// Summary generation — structured narrative instead of bare counts
// ---------------------------------------------------------------------------

func (rb *ReportBuilder) generateSummary(report types.InspectionReport) string {
	if len(report.Hazards) == 0 {
		return fmt.Sprintf(
			"%s inspection completed with no hazards detected. The inspected area appears to be in compliance with applicable safety standards at the time of observation.",
			titleCase(report.InspectionMode),
		)
	}

	counts := map[types.Severity]int{}
	ruleCategories := map[string]bool{}
	camerasSeen := map[string]bool{}
	for _, h := range report.Hazards {
		counts[h.Severity]++
		if h.RuleID != "" {
			// Extract category prefix from rule_id (e.g., "const" from "const_006")
			parts := strings.SplitN(h.RuleID, "_", 2)
			if len(parts) > 0 {
				ruleCategories[parts[0]] = true
			}
		}
		if h.CameraID != "" {
			camerasSeen[h.CameraID] = true
		}
	}

	critical := counts[types.SeverityCritical]
	high := counts[types.SeverityHigh]
	medium := counts[types.SeverityMedium]
	low := counts[types.SeverityLow]

	var parts []string
	parts = append(parts, fmt.Sprintf(
		"%s inspection completed with %d finding(s) across %d camera(s).",
		titleCase(report.InspectionMode), len(report.Hazards), max(len(camerasSeen), 1),
	))

	// Severity breakdown
	var sevParts []string
	if critical > 0 {
		sevParts = append(sevParts, fmt.Sprintf("%d critical", critical))
	}
	if high > 0 {
		sevParts = append(sevParts, fmt.Sprintf("%d high", high))
	}
	if medium > 0 {
		sevParts = append(sevParts, fmt.Sprintf("%d medium", medium))
	}
	if low > 0 {
		sevParts = append(sevParts, fmt.Sprintf("%d low", low))
	}
	parts = append(parts, fmt.Sprintf("Severity breakdown: %s.", strings.Join(sevParts, ", ")))

	// Risk assessment
	switch report.RiskLevel {
	case types.SeverityCritical:
		parts = append(parts, "Overall risk is CRITICAL — immediate intervention is required to address imminent life-safety hazards.")
	case types.SeverityHigh:
		parts = append(parts, "Overall risk is HIGH — prompt corrective action is recommended before conditions worsen.")
	case types.SeverityMedium:
		parts = append(parts, "Overall risk is MEDIUM — findings should be addressed during the next scheduled maintenance or safety review.")
	default:
		parts = append(parts, "Overall risk is LOW — minor housekeeping items noted for routine follow-up.")
	}

	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Recommendations generation — actionable, grouped by severity
// ---------------------------------------------------------------------------

func (rb *ReportBuilder) generateRecommendations(report types.InspectionReport) []string {
	if len(report.Hazards) == 0 {
		return nil
	}

	// Sort hazards by severity (critical first), then by confidence (descending)
	sorted := make([]types.Hazard, len(report.Hazards))
	copy(sorted, report.Hazards)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := severityWeight(sorted[i].Severity), severityWeight(sorted[j].Severity)
		if si != sj {
			return si > sj
		}
		return sorted[i].Confidence > sorted[j].Confidence
	})

	recs := make([]string, 0, len(report.Hazards)+3)
	seen := map[string]bool{}

	// Generate specific recommendations per hazard, avoiding duplicates
	for _, h := range sorted {
		key := strings.ToLower(strings.TrimSpace(h.Description))
		if seen[key] {
			continue
		}
		seen[key] = true

		prefix := actionPrefix(h.Severity)
		location := ""
		if h.Location != "" {
			location = fmt.Sprintf(" (%s)", h.Location)
		}
		recs = append(recs, fmt.Sprintf("%s: %s%s", prefix, h.Description, location))
	}

	// Append overall risk-level guidance
	switch report.RiskLevel {
	case types.SeverityCritical:
		recs = append(recs, "Halt operations in affected areas until critical findings are resolved and verified.")
	case types.SeverityHigh:
		recs = append(recs, "Schedule corrective action within 24 hours for high-severity findings.")
	case types.SeverityMedium:
		recs = append(recs, "Include medium-severity items in the next scheduled safety review cycle.")
	}

	// Re-inspection recommendation for high/critical
	hasSevere := report.RiskLevel == types.SeverityCritical || report.RiskLevel == types.SeverityHigh
	if hasSevere {
		recs = append(recs, "Conduct a follow-up inspection after remediation to verify corrective actions.")
	}

	return recs
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// evictOldestLocked removes the report with the earliest CreatedAt timestamp.
// Caller must hold rb.mu.
func (rb *ReportBuilder) evictOldestLocked() {
	var oldestID string
	var oldestTime time.Time
	for id, r := range rb.reports {
		if oldestID == "" || r.CreatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = r.CreatedAt
		}
	}
	if oldestID != "" {
		delete(rb.reports, oldestID)
		slog.Debug("evicted oldest report", "id", oldestID)
	}
}

func severityWeight(s types.Severity) int {
	switch s {
	case types.SeverityCritical:
		return 4
	case types.SeverityHigh:
		return 3
	case types.SeverityMedium:
		return 2
	case types.SeverityLow:
		return 1
	default:
		return 0
	}
}

func actionPrefix(s types.Severity) string {
	switch s {
	case types.SeverityCritical:
		return "IMMEDIATE ACTION REQUIRED"
	case types.SeverityHigh:
		return "Urgent"
	case types.SeverityMedium:
		return "Recommended"
	default:
		return "Note"
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (rb *ReportBuilder) HandleCreateReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Format    string `json:"format"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "report creation should be triggered via voice or agent",
	}); err != nil {
		slog.Error("failed to encode report response", "error", err)
	}
}

func (rb *ReportBuilder) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/v1/reports/"):]
	report, ok := rb.Get(id)
	if !ok {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		slog.Error("failed to encode report response", "error", err)
	}
}
