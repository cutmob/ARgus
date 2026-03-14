package gemini

import (
	"strings"

	"google.golang.org/genai"

	"github.com/cutmob/argus/internal/inspection"
)

// ArgusTools returns the function declarations that Gemini can call
// during a live inspection session. These are the agent's capabilities.
func ArgusTools() []*genai.Tool {
	modeList := strings.Join(inspection.CanonicalModes(), ", ")
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "inspect_frame",
					Description: "Log confirmed safety hazards from the current camera frame. Call this once per observation batch with all hazards found. For each hazard also call highlight_hazard to draw the overlay.",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"hazards": {
								Type:        "array",
								Description: "List of detected hazards",
								Items: &genai.Schema{
									Type: "object",
									Properties: map[string]*genai.Schema{
										"description": {Type: "string", Description: "What was observed"},
										"severity":    {Type: "string", Description: "low, medium, high, or critical", Enum: []string{"low", "medium", "high", "critical"}},
										"confidence":  {Type: "number", Description: "0.0 to 1.0"},
										"rule_id":     {Type: "string", Description: "Matching rule ID if applicable"},
										"location":    {Type: "string", Description: "Spatial location in the scene, e.g. 'left side near exit', 'overhead center', 'ground level right'"},
									},
									Required: []string{"description", "severity", "confidence"},
								},
							},
						},
						Required: []string{"hazards"},
					},
				},
				{
					Name:        "highlight_hazard",
					Description: "Draw a bounding box overlay on the camera feed for a detected hazard. Call this alongside inspect_frame for any hazard you can spatially localize.",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"label":    {Type: "string", Description: "Short label for the hazard"},
							"severity": {Type: "string", Description: "low, medium, high, or critical", Enum: []string{"low", "medium", "high", "critical"}},
							"location": {Type: "string", Description: "Spatial description, e.g. 'left side near exit door', 'overhead center', 'ground level right'"},
							"box_2d": {
								Type:        "array",
								Description: "Bounding box as [ymin, xmin, ymax, xmax] with values 0-1000",
								Items:       &genai.Schema{Type: "number"},
							},
						},
						Required: []string{"label", "severity"},
					},
				},
				{
					Name:        "switch_inspection_mode",
					Description: "Switch to a different inspection module. Available modes: " + modeList + ".",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"mode": {Type: "string", Description: "The inspection mode to switch to", Enum: inspection.CanonicalModes()},
						},
						Required: []string{"mode"},
					},
				},
				{
					Name:        "generate_report",
					Description: "Generate an inspection report summarizing all detected hazards, risk score, and recommendations. Call when user requests a report or says 'generate report'.",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"format": {Type: "string", Description: "Export format: json, pdf, txt, csv, html, word, doc, or webhook"},
						},
						Required: []string{"format"},
					},
				},
				{
					Name:        "get_inspection_status",
					Description: "Get the current inspection status including hazard count, risk level, and active mode. Call when user asks about status or progress.",
					Parameters: &genai.Schema{
						Type:       "object",
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "get_incidents",
					Description: "Get the list of current incident-level findings (persistent or recurring hazards) for this inspection session.",
					Parameters: &genai.Schema{
						Type:       "object",
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "log_issue",
					Description: "Log a single safety issue or hazard that the operator verbally reports or that you observe but cannot batch into inspect_frame.",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"description": {Type: "string", Description: "What was observed or reported"},
							"severity":    {Type: "string", Description: "low, medium, high, or critical", Enum: []string{"low", "medium", "high", "critical"}},
							"confidence":  {Type: "number", Description: "0.0 to 1.0"},
							"rule_id":     {Type: "string", Description: "Matching rule ID if applicable"},
						},
						Required: []string{"description", "severity"},
					},
				},
				{
					Name:        "dismiss_finding",
					Description: "Dismiss or acknowledge a previously reported hazard. Use when the operator says a finding is not a real hazard, has been resolved, or should be ignored.",
					Parameters: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"hazard_description": {Type: "string", Description: "Description of the hazard to dismiss — match against recent findings"},
							"reason":             {Type: "string", Description: "Why the finding is being dismissed (e.g. 'false positive', 'already resolved', 'not applicable')"},
						},
						Required: []string{"hazard_description"},
					},
				},
			},
		},
	}
}
