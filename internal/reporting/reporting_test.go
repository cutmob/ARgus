package reporting

import (
	"strings"
	"testing"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// ---------------------------------------------------------------------------
// Helper — builds a sample report
// ---------------------------------------------------------------------------

func sampleReport() types.InspectionReport {
	now := time.Now()
	return types.InspectionReport{
		ID:             "test-001",
		SessionID:      "sess-abc",
		InspectionMode: "construction",
		Location:       "Building A, Floor 3",
		Inspector:      "Unit Test",
		RiskLevel:      types.SeverityHigh,
		RiskScore:      18.5,
		CreatedAt:      now,
		Hazards: []types.Hazard{
			{
				ID:          "h1",
				RuleID:      "const_006",
				Description: "Unguarded leading edge near elevator shaft",
				Severity:    types.SeverityCritical,
				Confidence:  0.95,
				Location:    "north wall, floor 3",
				CameraID:    "cam1",
				DetectedAt:  now.Add(-2 * time.Minute),
			},
			{
				ID:          "h2",
				RuleID:      "const_024",
				Description: "Equipment operating without spotter",
				Severity:    types.SeverityHigh,
				Confidence:  0.82,
				Location:    "east bay",
				CameraID:    "cam2",
				DetectedAt:  now.Add(-1 * time.Minute),
			},
			{
				ID:          "h3",
				RuleID:      "const_010",
				Description: "Missing hard hat in active work zone",
				Severity:    types.SeverityMedium,
				Confidence:  0.70,
				Location:    "center area",
				CameraID:    "cam1",
				DetectedAt:  now,
			},
		},
	}
}

func emptyReport() types.InspectionReport {
	return types.InspectionReport{
		ID:             "test-empty",
		SessionID:      "sess-xyz",
		InspectionMode: "facility",
		RiskLevel:      types.SeverityLow,
		RiskScore:      0,
		CreatedAt:      time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Summary generation
// ---------------------------------------------------------------------------

func TestGenerateSummary_WithHazards(t *testing.T) {
	rb := NewReportBuilder(NewExportRegistry())
	report := sampleReport()
	summary := rb.generateSummary(report)

	if !strings.Contains(summary, "Construction") {
		t.Errorf("expected mode name in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "3 finding") {
		t.Errorf("expected finding count in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "1 critical") {
		t.Errorf("expected critical count in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "HIGH") || !strings.Contains(summary, "prompt corrective") {
		t.Errorf("expected risk assessment in summary, got: %s", summary)
	}
}

func TestGenerateSummary_Empty(t *testing.T) {
	rb := NewReportBuilder(NewExportRegistry())
	report := emptyReport()
	summary := rb.generateSummary(report)

	if !strings.Contains(summary, "no hazards") {
		t.Errorf("expected 'no hazards' in empty summary, got: %s", summary)
	}
}

// ---------------------------------------------------------------------------
// Recommendations generation
// ---------------------------------------------------------------------------

func TestGenerateRecommendations_WithHazards(t *testing.T) {
	rb := NewReportBuilder(NewExportRegistry())
	report := sampleReport()
	recs := rb.generateRecommendations(report)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	// Should be sorted by severity — critical first
	if !strings.HasPrefix(recs[0], "IMMEDIATE ACTION REQUIRED") {
		t.Errorf("expected critical hazard first, got: %s", recs[0])
	}

	// Should include follow-up recommendation for high risk
	found := false
	for _, r := range recs {
		if strings.Contains(r, "follow-up inspection") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected follow-up inspection recommendation for high-risk report")
	}
}

func TestGenerateRecommendations_Empty(t *testing.T) {
	rb := NewReportBuilder(NewExportRegistry())
	report := emptyReport()
	recs := rb.generateRecommendations(report)

	if recs != nil {
		t.Errorf("expected nil recommendations for empty report, got %d", len(recs))
	}
}

func TestGenerateRecommendations_NoDuplicates(t *testing.T) {
	rb := NewReportBuilder(NewExportRegistry())
	report := sampleReport()
	// Add a duplicate hazard
	report.Hazards = append(report.Hazards, report.Hazards[0])
	recs := rb.generateRecommendations(report)

	seen := map[string]bool{}
	for _, r := range recs {
		if seen[r] {
			t.Errorf("duplicate recommendation: %s", r)
		}
		seen[r] = true
	}
}

// ---------------------------------------------------------------------------
// HTML rendering
// ---------------------------------------------------------------------------

func TestBuildHTMLReport_ContainsCriticalStructure(t *testing.T) {
	html := buildHTMLReport(sampleReport())

	checks := []string{
		"<!doctype html>",
		"ARGUS Inspection Report",
		"@page",                       // print CSS
		"break-inside",                // print CSS
		"print-color-adjust",          // print CSS
		"#dc2626",                     // critical severity color
		"Unguarded leading edge",      // hazard description
		"sev-badge",                   // severity badge class
		"risk-banner",                 // risk banner
		"report-footer",               // footer
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("HTML missing expected content: %q", check)
		}
	}
}

func TestBuildHTMLReport_Empty(t *testing.T) {
	html := buildHTMLReport(emptyReport())
	if !strings.Contains(html, "No hazards detected") {
		t.Error("expected empty-state message in HTML")
	}
}

// ---------------------------------------------------------------------------
// Word HTML rendering
// ---------------------------------------------------------------------------

func TestBuildWordHTML_ContainsOfficeNamespaces(t *testing.T) {
	word := buildWordHTML(sampleReport())

	checks := []string{
		"urn:schemas-microsoft-com:office:office",
		"urn:schemas-microsoft-com:office:word",
		"mso-header-margin",
		"Section1",
		"ProgId",
		"Word.Document",
	}
	for _, check := range checks {
		if !strings.Contains(word, check) {
			t.Errorf("Word HTML missing expected content: %q", check)
		}
	}
}

// ---------------------------------------------------------------------------
// PDF generation
// ---------------------------------------------------------------------------

func TestBuildPDFReport_ValidPDF(t *testing.T) {
	pdf := buildPDFReport(sampleReport())

	// Check PDF header
	if !strings.HasPrefix(string(pdf), "%PDF-1.4") {
		t.Error("expected PDF-1.4 header")
	}
	// Check PDF trailer
	if !strings.HasSuffix(string(pdf), "%%EOF") {
		t.Error("expected EOF trailer")
	}
	// Check it contains content
	if len(pdf) < 500 {
		t.Errorf("PDF suspiciously small: %d bytes", len(pdf))
	}
}

func TestBuildPDFReport_Empty(t *testing.T) {
	pdf := buildPDFReport(emptyReport())
	pdfStr := string(pdf)
	if !strings.Contains(pdfStr, "No hazards detected") {
		t.Error("expected empty-state message in PDF")
	}
}

func TestBuildPDFReport_MultiplePages(t *testing.T) {
	report := sampleReport()
	// Add many hazards to force page break
	for i := 0; i < 50; i++ {
		report.Hazards = append(report.Hazards, types.Hazard{
			ID:          "bulk",
			Description: "Bulk hazard for page-break test",
			Severity:    types.SeverityMedium,
			Confidence:  0.6,
			DetectedAt:  time.Now(),
		})
	}
	pdf := buildPDFReport(report)
	pdfStr := string(pdf)

	// Multiple pages should produce multiple /Type /Page objects
	pageCount := strings.Count(pdfStr, "/Type /Page /Parent")
	if pageCount < 2 {
		t.Errorf("expected multiple pages, found %d /Type /Page objects", pageCount)
	}
}

// ---------------------------------------------------------------------------
// Plain text rendering
// ---------------------------------------------------------------------------

func TestBuildPlainTextReport(t *testing.T) {
	txt := buildPlainTextReport(sampleReport())

	checks := []string{
		"ARGUS INSPECTION REPORT",
		"Unguarded leading edge",
		"RECOMMENDATIONS",
		"Generated by ARGUS",
	}
	for _, check := range checks {
		if !strings.Contains(txt, check) {
			t.Errorf("plain text missing expected content: %q", check)
		}
	}
}

// ---------------------------------------------------------------------------
// PDF helpers
// ---------------------------------------------------------------------------

func TestEscapePDFLine(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"a(b)c", `a\(b\)c`},
		{`back\slash`, `back\\slash`},
		{"line\nbreak", "line break"},
		{"high\x80byte", "high?byte"},
	}
	for _, tt := range tests {
		got := escapePDFLine(tt.in)
		if got != tt.want {
			t.Errorf("escapePDFLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrapText(t *testing.T) {
	lines := wrapText("the quick brown fox jumps over the lazy dog", 20)
	for _, l := range lines {
		if len(l) > 20 {
			t.Errorf("line exceeds max: %q (%d chars)", l, len(l))
		}
	}
	joined := strings.Join(lines, " ")
	if joined != "the quick brown fox jumps over the lazy dog" {
		t.Errorf("wrapped text lost content: %q", joined)
	}
}

func TestSeverityRGB(t *testing.T) {
	r, g, b := severityRGB(types.SeverityCritical)
	if r < 0.5 || g > 0.5 {
		t.Errorf("critical should be reddish, got (%.2f, %.2f, %.2f)", r, g, b)
	}
	r, g, b = severityRGB(types.SeverityLow)
	if g < 0.5 {
		t.Errorf("low should be greenish, got (%.2f, %.2f, %.2f)", r, g, b)
	}
}

// ---------------------------------------------------------------------------
// Export registry
// ---------------------------------------------------------------------------

func TestExportRegistry_RoundTrip(t *testing.T) {
	reg := NewExportRegistry()
	reg.Register("json", NewJSONExporter())
	reg.Register("txt", NewTXTExporter())

	names := reg.Available()
	if len(names) != 2 {
		t.Errorf("expected 2 exporters, got %d", len(names))
	}

	_, err := reg.Export("missing", sampleReport())
	if err == nil {
		t.Error("expected error for missing exporter")
	}
}

// ---------------------------------------------------------------------------
// Report builder eviction
// ---------------------------------------------------------------------------

func TestReportBuilder_EvictsOldest(t *testing.T) {
	reg := NewExportRegistry()
	rb := NewReportBuilder(reg)

	base := time.Now()
	// Fill to capacity with maxStoredReports (use a smaller set to test the logic)
	for i := 0; i < maxStoredReports+5; i++ {
		r := types.InspectionReport{
			ID:        strings.Repeat("x", 5) + string(rune('A'+i%26)) + strings.Repeat("y", 3),
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}
		r.ID = r.CreatedAt.Format("20060102150405.000000000")
		_, _ = rb.Build(r, "")
	}

	rb.mu.RLock()
	count := len(rb.reports)
	rb.mu.RUnlock()
	if count > maxStoredReports {
		t.Errorf("expected at most %d reports, got %d", maxStoredReports, count)
	}
}
