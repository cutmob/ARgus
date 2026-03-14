package agent

import (
	"strings"

	"github.com/cutmob/argus/internal/inspection"
	"github.com/cutmob/argus/pkg/types"
)

// IntentParser extracts structured intents from user speech/text.
// In production, Gemini Live handles intent extraction natively via function
// calling — this parser is an intentionally simple fallback for the non-Live
// path (e.g. HandleRawText). Exact substring matching is acceptable here
// because voice transcription errors are handled by Gemini, not this parser.
type IntentParser struct{}

func NewIntentParser() *IntentParser {
	return &IntentParser{}
}

// Parse converts raw text into a structured AgentIntent.
func (ip *IntentParser) Parse(text string) types.AgentIntent {
	lower := strings.ToLower(strings.TrimSpace(text))

	intent := types.AgentIntent{
		RawText:    text,
		Parameters: make(map[string]string),
	}

	switch {
	case ip.matchesAny(lower, "start inspection", "begin inspection", "inspect", "kick off inspection", "inspection on", "start scan", "begin scan"):
		intent.Type = types.IntentStartInspection
		intent.Mode = ip.extractMode(lower)

	case ip.matchesAny(lower, "stop inspection", "end inspection", "finish inspection", "done", "halt inspection", "pause inspection", "inspection off", "stop scan"):
		intent.Type = types.IntentStopInspection

	case ip.matchesAny(lower, "switch to", "change to", "switch mode", "set mode", "use mode", "go to"):
		intent.Type = types.IntentSwitchMode
		intent.Mode = ip.extractMode(lower)

	case ip.matchesAny(lower, "generate report", "create report", "make report", "export report", "export results", "run report", "report now"):
		intent.Type = types.IntentGenerateReport
		intent.Format = ip.extractFormat(lower)

	case ip.matchesAny(lower, "send to", "export to", "push to", "send report to", "share to", "deliver to"):
		intent.Type = types.IntentExportReport
		intent.Target = ip.extractTarget(lower)
		intent.Format = ip.extractFormat(lower)

	case ip.matchesAny(lower, "what hazards", "what issues", "what problems", "findings", "describe scene", "what do you see", "summarize scene"):
		intent.Type = types.IntentQueryHazards

	case ip.matchesAny(lower, "immediate actions", "top 3 actions", "top three actions", "next actions", "what should we do now", "top actions"):
		intent.Type = types.IntentOperatorActions

	case ip.matchesAny(lower, "status", "how's it going", "what's happening", "update", "risk level", "current risk", "how many hazards"):
		intent.Type = types.IntentQueryStatus

	default:
		intent.Type = types.IntentConversation
	}

	return intent
}

func (ip *IntentParser) matchesAny(text string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func (ip *IntentParser) extractMode(text string) string {
	// Use the shared mode alias map from the inspection package
	for alias, mode := range inspection.ModeAliases {
		if strings.Contains(text, alias) {
			return mode
		}
	}
	return "general"
}

func (ip *IntentParser) extractFormat(text string) string {
	switch {
	case strings.Contains(text, "word"), strings.Contains(text, "docx"), strings.Contains(text, "doc"):
		return "word"
	case strings.Contains(text, "text"), strings.Contains(text, "txt"):
		return "txt"
	case strings.Contains(text, "csv"), strings.Contains(text, "spreadsheet"), strings.Contains(text, "excel"):
		return "csv"
	case strings.Contains(text, "html"), strings.Contains(text, "web page"), strings.Contains(text, "browser"):
		return "html"
	case strings.Contains(text, "pdf"):
		return "pdf"
	case strings.Contains(text, "json"):
		return "json"
	case strings.Contains(text, "webhook"):
		return "webhook"
	default:
		return "json"
	}
}

func (ip *IntentParser) extractTarget(text string) string {
	targets := []string{"slack", "email", "webhook", "maintenance", "jira", "notion"}
	for _, t := range targets {
		if strings.Contains(text, t) {
			return t
		}
	}
	return "webhook"
}
