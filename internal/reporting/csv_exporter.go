package reporting

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// CSVExporter writes hazard rows for spreadsheet and BI workflows.
type CSVExporter struct {
	outputDir string
}

func NewCSVExporter() *CSVExporter {
	dir := os.Getenv("ARGUS_REPORTS_DIR")
	if dir == "" {
		dir = "./reports"
	}
	_ = os.MkdirAll(dir, 0750)
	return &CSVExporter{outputDir: dir}
}

func (e *CSVExporter) Name() string { return "csv" }

func (e *CSVExporter) Export(report types.InspectionReport) (string, error) {
	filename := fmt.Sprintf("argus_report_%s_%d.csv",
		report.InspectionMode,
		time.Now().UnixNano(),
	)
	path := filepath.Join(e.outputDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating csv report: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"report_id", "session_id", "inspection_mode",
		"risk_level", "risk_score", "report_date",
		"hazard_index", "description", "severity", "confidence",
		"rule_id", "camera_id", "location", "detected_at",
		"risk_trend", "persistence_seconds",
	}
	if err := w.Write(header); err != nil {
		return "", fmt.Errorf("writing csv header: %w", err)
	}

	reportDate := ""
	if !report.CreatedAt.IsZero() {
		reportDate = report.CreatedAt.Format(time.RFC3339)
	}

	if len(report.Hazards) == 0 {
		row := []string{
			report.ID, report.SessionID, report.InspectionMode,
			string(report.RiskLevel), fmt.Sprintf("%.1f", report.RiskScore), reportDate,
			"", "", "", "", "", "", "", "", "", "",
		}
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("writing csv row: %w", err)
		}
	} else {
		for i, h := range report.Hazards {
			detectedAt := ""
			if !h.DetectedAt.IsZero() {
				detectedAt = h.DetectedAt.Format(time.RFC3339)
			}
			row := []string{
				report.ID,
				report.SessionID,
				report.InspectionMode,
				string(report.RiskLevel),
				fmt.Sprintf("%.1f", report.RiskScore),
				reportDate,
				fmt.Sprintf("%d", i+1),
				h.Description,
				string(h.Severity),
				fmt.Sprintf("%.4f", h.Confidence),
				h.RuleID,
				h.CameraID,
				h.Location,
				detectedAt,
				h.RiskTrend,
				fmt.Sprintf("%d", h.PersistenceSeconds),
			}
			if err := w.Write(row); err != nil {
				return "", fmt.Errorf("writing csv row: %w", err)
			}
		}
	}

	if err := w.Error(); err != nil {
		return "", fmt.Errorf("finalizing csv: %w", err)
	}

	// Write a separate summary CSV section after a blank line
	if err := w.Write([]string{}); err != nil {
		return "", fmt.Errorf("writing csv separator: %w", err)
	}
	summaryHeader := []string{"metric", "value"}
	if err := w.Write(summaryHeader); err != nil {
		return "", fmt.Errorf("writing csv summary header: %w", err)
	}
	summaryRows := [][]string{
		{"total_findings", fmt.Sprintf("%d", len(report.Hazards))},
		{"risk_level", string(report.RiskLevel)},
		{"risk_score", fmt.Sprintf("%.1f", report.RiskScore)},
		{"inspection_mode", report.InspectionMode},
		{"summary", strings.ReplaceAll(report.Summary, "\n", " ")},
	}
	for _, row := range summaryRows {
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("writing csv summary: %w", err)
		}
	}

	slog.Info("report exported as CSV", "path", path)
	return filename, nil
}
