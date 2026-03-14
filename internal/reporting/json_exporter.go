package reporting

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// JSONExporter writes reports as structured JSON files.
type JSONExporter struct {
	outputDir string
}

func NewJSONExporter() *JSONExporter {
	dir := os.Getenv("ARGUS_REPORTS_DIR")
	if dir == "" {
		dir = "./reports"
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Warn("failed to create reports directory", "dir", dir, "error", err)
	}
	return &JSONExporter{outputDir: dir}
}

func (e *JSONExporter) Name() string { return "json" }

func (e *JSONExporter) Export(report types.InspectionReport) (string, error) {
	filename := fmt.Sprintf("argus_report_%s_%d.json",
		report.InspectionMode,
		time.Now().UnixNano(),
	)
	path := filepath.Join(e.outputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling report: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("writing report file: %w", err)
	}

	slog.Info("report exported as JSON", "path", path)
	return filename, nil
}
