package reporting

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// WordExporter writes Word-compatible .doc files using Office-namespace HTML.
// The output uses Microsoft Office XML namespaces and mso- CSS properties so
// Word 2016+ renders it with proper page layout, heading styles, and table
// formatting rather than raw web-HTML rendering.
type WordExporter struct {
	outputDir string
}

func NewWordExporter() *WordExporter {
	dir := os.Getenv("ARGUS_REPORTS_DIR")
	if dir == "" {
		dir = "./reports"
	}
	_ = os.MkdirAll(dir, 0750)
	return &WordExporter{outputDir: dir}
}

func (e *WordExporter) Name() string { return "word" }

func (e *WordExporter) Export(report types.InspectionReport) (string, error) {
	filename := fmt.Sprintf("argus_report_%s_%d.doc",
		report.InspectionMode,
		time.Now().UnixNano(),
	)
	path := filepath.Join(e.outputDir, filename)

	content := buildWordHTML(report)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("writing word report: %w", err)
	}

	slog.Info("report exported as WORD", "path", path)
	return filename, nil
}
