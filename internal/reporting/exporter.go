package reporting

import (
	"fmt"
	"sync"

	"github.com/cutmob/argus/pkg/types"
)

// Exporter is the interface all export destinations must implement.
// To add a new export format, implement this interface and register it.
type Exporter interface {
	// Name returns the export format identifier.
	Name() string
	// Export sends the report to its destination and returns the output
	// filename (basename only, e.g. "argus_report_construction_20260314.pdf").
	// Returns an empty string for non-file destinations (e.g. webhook).
	Export(report types.InspectionReport) (filename string, err error)
}

// ExportRegistry manages available exporters.
type ExportRegistry struct {
	mu        sync.RWMutex
	exporters map[string]Exporter
}

func NewExportRegistry() *ExportRegistry {
	return &ExportRegistry{
		exporters: make(map[string]Exporter),
	}
}

// Register adds an exporter to the registry.
func (er *ExportRegistry) Register(name string, exp Exporter) {
	er.mu.Lock()
	defer er.mu.Unlock()
	er.exporters[name] = exp
}

// Export sends a report through the named exporter and returns the output filename.
func (er *ExportRegistry) Export(name string, report types.InspectionReport) (string, error) {
	er.mu.RLock()
	exp, ok := er.exporters[name]
	er.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("exporter %q not registered", name)
	}
	return exp.Export(report)
}

// Available returns all registered exporter names.
func (er *ExportRegistry) Available() []string {
	er.mu.RLock()
	defer er.mu.RUnlock()
	names := make([]string, 0, len(er.exporters))
	for name := range er.exporters {
		names = append(names, name)
	}
	return names
}
