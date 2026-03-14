package reporting

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

const (
	pdfPageWidth  = 595.0 // A4 points
	pdfPageHeight = 842.0
	pdfMargin     = 40.0
	pdfBodyWidth  = 515.0 // pdfPageWidth - 2*pdfMargin
)

// PDFExporter generates PDF inspection reports.
type PDFExporter struct {
	outputDir string
}

func NewPDFExporter() *PDFExporter {
	dir := os.Getenv("ARGUS_REPORTS_DIR")
	if dir == "" {
		dir = "./reports"
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		slog.Warn("failed to create reports directory", "dir", dir, "error", err)
	}
	return &PDFExporter{outputDir: dir}
}

func (e *PDFExporter) Name() string { return "pdf" }

func (e *PDFExporter) Export(report types.InspectionReport) (string, error) {
	filename := fmt.Sprintf("argus_report_%s_%d.pdf",
		report.InspectionMode,
		time.Now().UnixNano(),
	)
	path := filepath.Join(e.outputDir, filename)

	pdfData := buildPDFReport(report)
	if err := os.WriteFile(path, pdfData, 0600); err != nil {
		return "", fmt.Errorf("writing report: %w", err)
	}

	slog.Info("report exported as PDF", "path", path)
	return filename, nil
}

// ---------------------------------------------------------------------------
// Structured PDF builder
// ---------------------------------------------------------------------------

// pdfWriter accumulates content-stream operations for multi-page PDF output.
// It tracks the current Y position and automatically creates new pages when
// the cursor approaches the bottom margin.
type pdfWriter struct {
	pages   []string // finished page content streams
	current bytes.Buffer
	y       float64 // current Y cursor (decreasing from top)
	pageNum int     // 1-indexed page number

	// Carried across pages for repeating headers/footers
	reportID string
	mode     string
}

const (
	pdfTopY       = 780.0
	pdfBottomY    = 60.0
	pdfFontBody   = 9.0
	pdfFontH1     = 20.0
	pdfFontH2     = 13.0
	pdfFontMeta   = 9.0
	pdfFontSmall  = 7.5
	pdfLineHeight = 13.0
	pdfColGap     = 3.0
)

func newPDFWriter(reportID, mode string) *pdfWriter {
	w := &pdfWriter{
		y:        pdfTopY,
		pageNum:  1,
		reportID: reportID,
		mode:     mode,
	}
	w.beginPage()
	return w
}

func (w *pdfWriter) beginPage() {
	w.current.Reset()
	w.y = pdfTopY
	w.current.WriteString("BT\n")
}

func (w *pdfWriter) endPage() {
	// Page footer
	w.current.WriteString(fmt.Sprintf("/F1 %.1f Tf\n", pdfFontSmall))
	w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", pdfMargin, 30.0))
	w.current.WriteString(fmt.Sprintf("(ARGUS AI Inspection System  |  %s  |  Page %d) Tj\n",
		escapePDFLine(w.reportID), w.pageNum))

	w.current.WriteString("ET\n")

	// Footer line
	w.current.WriteString(fmt.Sprintf("%.1f 38 m %.1f 38 l 0.4 w 0.7 0.7 0.7 RG S\n",
		pdfMargin, pdfMargin+pdfBodyWidth))

	w.pages = append(w.pages, w.current.String())
	w.pageNum++
}

func (w *pdfWriter) ensureSpace(need float64) {
	if w.y-need < pdfBottomY {
		w.endPage()
		w.beginPage()
	}
}

// text emits a single line of text at the current Y position.
func (w *pdfWriter) text(fontTag string, size float64, x, leading float64, line string) {
	w.ensureSpace(leading)
	w.current.WriteString(fmt.Sprintf("/%s %.1f Tf\n", fontTag, size))
	w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", x, w.y))
	w.current.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFLine(line)))
	w.y -= leading
}

// textBlock writes a paragraph, word-wrapping at maxChars characters per line.
func (w *pdfWriter) textBlock(fontTag string, size float64, x float64, content string, maxChars int) {
	lines := wrapText(content, maxChars)
	for _, l := range lines {
		w.text(fontTag, size, x, pdfLineHeight, l)
	}
}

// heading emits a section heading with spacing above and below.
func (w *pdfWriter) heading(title string) {
	w.ensureSpace(pdfFontH2 + 20)
	w.y -= 12 // space above heading
	w.text("F2", pdfFontH2, pdfMargin, pdfFontH2+4, title)
	// Colored underline
	w.current.WriteString("ET\n")
	w.current.WriteString("0.13 0.55 0.13 RG\n") // green accent
	w.current.WriteString(fmt.Sprintf("%.1f %.1f m %.1f %.1f l 1.5 w S\n",
		pdfMargin, w.y+2, pdfMargin+pdfBodyWidth, w.y+2))
	w.current.WriteString("0 0 0 RG\n") // reset stroke
	w.current.WriteString("BT\n")
	w.y -= 4
}

// metaLine emits a "Label:  Value" line in the header block.
func (w *pdfWriter) metaLine(label, value string) {
	if value == "" {
		return
	}
	w.ensureSpace(pdfLineHeight)
	// Bold label
	w.current.WriteString(fmt.Sprintf("/F2 %.1f Tf\n", pdfFontMeta))
	w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", pdfMargin, w.y))
	w.current.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFLine(label)))
	// Regular value — offset right, scaled to label length
	labelWidth := float64(len(label)) * pdfFontMeta * 0.52
	if labelWidth < 90 {
		labelWidth = 90
	}
	w.current.WriteString(fmt.Sprintf("/F1 %.1f Tf\n", pdfFontMeta))
	w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", pdfMargin+labelWidth, w.y))
	w.current.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFLine(value)))
	w.y -= pdfLineHeight
}

// tableRow draws a row of cells. colWidths must sum to <= pdfBodyWidth.
func (w *pdfWriter) tableRow(fontTag string, fontSize float64, colWidths []float64, cells []string) {
	w.ensureSpace(pdfLineHeight + 2)
	x := pdfMargin
	for i, cell := range cells {
		if i >= len(colWidths) {
			break
		}
		cw := colWidths[i]
		// Character width approximation: avg char width ≈ fontSize * 0.48 for Helvetica
		maxChars := int(cw / (fontSize * 0.48))
		display := cell
		if maxChars > 3 && len(display) > maxChars {
			display = display[:maxChars-2] + ".."
		}
		w.current.WriteString(fmt.Sprintf("/%s %.1f Tf\n", fontTag, fontSize))
		w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", x+2, w.y))
		w.current.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFLine(display)))
		x += cw + pdfColGap
	}
	w.y -= pdfLineHeight
}

// tableRowColored draws a row with the severity cell colored.
func (w *pdfWriter) tableRowColored(colWidths []float64, cells []string, sevIdx int, sev types.Severity) {
	w.ensureSpace(pdfLineHeight + 2)
	x := pdfMargin
	for i, cell := range cells {
		if i >= len(colWidths) {
			break
		}
		cw := colWidths[i]
		maxChars := int(cw / (pdfFontBody * 0.48))
		display := cell
		if maxChars > 3 && len(display) > maxChars {
			display = display[:maxChars-2] + ".."
		}

		if i == sevIdx {
			// Color the severity cell
			r, g, b := severityRGB(sev)
			w.current.WriteString(fmt.Sprintf("%.2f %.2f %.2f rg\n", r, g, b))
		}
		w.current.WriteString(fmt.Sprintf("/F1 %.1f Tf\n", pdfFontBody))
		w.current.WriteString(fmt.Sprintf("%.1f %.1f Td\n", x+2, w.y))
		w.current.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFLine(display)))
		if i == sevIdx {
			w.current.WriteString("0 0 0 rg\n")
		}
		x += cw + pdfColGap
	}
	w.y -= pdfLineHeight
}

// separator draws a thin horizontal line.
func (w *pdfWriter) separator() {
	w.ensureSpace(6)
	w.current.WriteString("ET\n")
	w.current.WriteString("0.85 0.85 0.85 RG\n")
	w.current.WriteString(fmt.Sprintf("%.1f %.1f m %.1f %.1f l 0.7 w S\n",
		pdfMargin, w.y, pdfMargin+pdfBodyWidth, w.y))
	w.current.WriteString("0 0 0 RG\n")
	w.y -= 4
	w.current.WriteString("BT\n")
}

// filledRect draws a filled rectangle (ends/begins text block around it).
func (w *pdfWriter) filledRect(x, y, width, height, r, g, b float64) {
	w.current.WriteString("ET\n")
	w.current.WriteString(fmt.Sprintf("%.2f %.2f %.2f rg\n", r, g, b))
	w.current.WriteString(fmt.Sprintf("%.1f %.1f %.1f %.1f re f\n", x, y, width, height))
	w.current.WriteString("0 0 0 rg\n")
	w.current.WriteString("BT\n")
}

// finalize closes the last page and returns all page streams.
func (w *pdfWriter) finalize() []string {
	w.endPage()
	return w.pages
}

// ---------------------------------------------------------------------------
// Report assembly
// ---------------------------------------------------------------------------

func buildPDFReport(report types.InspectionReport) []byte {
	w := newPDFWriter(report.ID, report.InspectionMode)

	// ── Title bar ──────────────────────────────────────────────────────
	r, g, b := riskBannerRGB(report.RiskLevel)
	w.filledRect(pdfMargin, w.y-4, pdfBodyWidth, 32, r, g, b)
	w.current.WriteString("1 1 1 rg\n") // white text
	w.text("F2", pdfFontH1, pdfMargin+10, pdfFontH1+8, "ARGUS Inspection Report")
	w.current.WriteString("0 0 0 rg\n") // back to black

	// Risk level + score bar
	w.ensureSpace(20)
	riskR, riskG, riskB := riskBannerRGB(report.RiskLevel)
	w.filledRect(pdfMargin, w.y-2, pdfBodyWidth, 18, riskR, riskG, riskB)
	w.current.WriteString("1 1 1 rg\n")
	w.text("F2", 9, pdfMargin+6, 14,
		fmt.Sprintf("Risk: %s   |   Score: %.1f   |   %s inspection   |   %d findings",
			strings.ToUpper(string(report.RiskLevel)), report.RiskScore,
			report.InspectionMode, len(report.Hazards)))
	w.current.WriteString("0 0 0 rg\n")
	w.y -= 4

	// ── Metadata block ─────────────────────────────────────────────────
	w.metaLine("Report ID:", report.ID)
	w.metaLine("Session ID:", report.SessionID)
	w.metaLine("Inspection Type:", report.InspectionMode)
	w.metaLine("Location:", report.Location)
	w.metaLine("Inspector:", report.Inspector)
	w.metaLine("Date:", report.CreatedAt.Format("2006-01-02 15:04:05"))

	// ── Summary ────────────────────────────────────────────────────────
	w.heading("Summary")
	w.textBlock("F1", pdfFontBody, pdfMargin, report.Summary, 95)

	// ── Detected Issues (table) ────────────────────────────────────────
	w.heading("Detected Issues")

	// Column widths: #(18) Desc(160) Sev(48) Conf(35) Rule(55) Camera(52) Location(65) Time(70)
	colW := []float64{18, 160, 48, 35, 55, 52, 65, 70}
	headers := []string{"#", "Description", "Severity", "Conf.", "Rule", "Camera", "Location", "Timestamp"}
	w.tableRow("F2", pdfFontBody, colW, headers)
	w.separator()

	if len(report.Hazards) == 0 {
		w.text("F1", pdfFontBody, pdfMargin, pdfLineHeight, "No hazards detected.")
	} else {
		for i, h := range report.Hazards {
			ts := ""
			if !h.DetectedAt.IsZero() {
				ts = h.DetectedAt.Format("2006-01-02 15:04")
			}
			row := []string{
				fmt.Sprintf("%d", i+1),
				h.Description,
				strings.ToUpper(string(h.Severity)),
				fmt.Sprintf("%.0f%%", h.Confidence*100),
				h.RuleID,
				h.CameraID,
				h.Location,
				ts,
			}
			w.tableRowColored(colW, row, 2, h.Severity) // color index 2 = severity column
		}
	}

	// ── Recommendations ────────────────────────────────────────────────
	w.heading("Recommendations")
	if len(report.Recommendations) == 0 {
		w.text("F1", pdfFontBody, pdfMargin, pdfLineHeight, "No recommendations at this time.")
	} else {
		for i, rec := range report.Recommendations {
			w.textBlock("F1", pdfFontBody, pdfMargin+12, fmt.Sprintf("%d. %s", i+1, rec), 90)
		}
	}

	// ── Assemble PDF bytes ─────────────────────────────────────────────
	pageStreams := w.finalize()
	return assemblePDF(pageStreams)
}

// assemblePDF takes finished content stream strings (one per page) and builds
// a valid multi-page PDF 1.4 byte sequence with two fonts (Helvetica regular
// and Helvetica-Bold).
func assemblePDF(pageStreams []string) []byte {
	type pdfObj struct{ body string }
	var objs []pdfObj

	alloc := func(body string) int {
		objs = append(objs, pdfObj{body: body})
		return len(objs) // 1-indexed
	}

	// 1: Catalog
	alloc("<< /Type /Catalog /Pages 2 0 R >>")
	// 2: Pages placeholder
	pagesSlot := alloc("PLACEHOLDER")
	// 3: Font – Helvetica (regular)
	alloc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	// 4: Font – Helvetica-Bold
	alloc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	pageIDs := make([]int, 0, len(pageStreams))
	for _, stream := range pageStreams {
		contentID := alloc(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
		pageID := alloc(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.1f %.1f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			pdfPageWidth, pdfPageHeight, contentID,
		))
		pageIDs = append(pageIDs, pageID)
	}

	// Patch Pages
	kids := make([]string, len(pageIDs))
	for i, id := range pageIDs {
		kids[i] = fmt.Sprintf("%d 0 R", id)
	}
	objs[pagesSlot-1].body = fmt.Sprintf(
		"<< /Type /Pages /Kids [%s] /Count %d >>",
		strings.Join(kids, " "), len(pageIDs),
	)

	// Serialize
	var out bytes.Buffer
	offsets := make([]int, len(objs))
	out.WriteString("%PDF-1.4\n")
	for i, o := range objs {
		offsets[i] = out.Len()
		out.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o.body))
	}

	xrefPos := out.Len()
	out.WriteString("xref\n")
	out.WriteString(fmt.Sprintf("0 %d\n", len(objs)+1))
	out.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	out.WriteString("trailer\n")
	out.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objs)+1))
	out.WriteString("startxref\n")
	out.WriteString(fmt.Sprintf("%d\n", xrefPos))
	out.WriteString("%%EOF")

	return out.Bytes()
}

// ---------------------------------------------------------------------------
// PDF helpers
// ---------------------------------------------------------------------------

// severityRGB returns an RGB triplet (0-1) for severity-colored text in PDF.
func severityRGB(s types.Severity) (float64, float64, float64) {
	switch s {
	case types.SeverityCritical:
		return 0.86, 0.15, 0.15 // #dc2626
	case types.SeverityHigh:
		return 0.92, 0.35, 0.05 // #ea580c
	case types.SeverityMedium:
		return 0.79, 0.54, 0.02 // #ca8a04
	case types.SeverityLow:
		return 0.09, 0.64, 0.20 // #16a34a
	default:
		return 0, 0, 0
	}
}

// riskBannerRGB returns a dark banner RGB for the overall risk level.
func riskBannerRGB(s types.Severity) (float64, float64, float64) {
	switch s {
	case types.SeverityCritical:
		return 0.60, 0.11, 0.11 // #991b1b
	case types.SeverityHigh:
		return 0.76, 0.25, 0.05 // #c2410c
	case types.SeverityMedium:
		return 0.63, 0.38, 0.03 // #a16207
	default:
		return 0.08, 0.50, 0.23 // #15803d
	}
}

// wrapText word-wraps a string into lines of at most maxCols characters.
func wrapText(text string, maxCols int) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= maxCols {
			result = append(result, line)
			continue
		}
		words := strings.Fields(line)
		cur := ""
		for _, w := range words {
			if cur == "" {
				cur = w
			} else if len(cur)+1+len(w) <= maxCols {
				cur += " " + w
			} else {
				result = append(result, cur)
				cur = w
			}
		}
		if cur != "" {
			result = append(result, cur)
		}
	}
	return result
}

// escapePDFLine escapes a single line of text for use inside a PDF string literal.
func escapePDFLine(s string) string {
	b := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b = append(b, '\\', '\\')
		case '(':
			b = append(b, '\\', '(')
		case ')':
			b = append(b, '\\', ')')
		case '\r', '\n', '\t':
			b = append(b, ' ')
		default:
			if c < 32 {
				continue
			}
			if c > 126 {
				b = append(b, '?')
				continue
			}
			b = append(b, c)
		}
	}
	return string(b)
}
