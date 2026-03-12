package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/cutmob/argus/pkg/types"
)

// hazardResponseSchema defines the JSON schema for structured hazard output
// from Gemini's GenerateContent API.
func hazardResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hazards": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"rule_id":     map[string]any{"type": "string", "description": "ID of the matched inspection rule, or empty"},
						"description": map[string]any{"type": "string", "description": "What was observed"},
						"severity":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
						"confidence":  map[string]any{"type": "number", "description": "0.0 to 1.0"},
						"location":    map[string]any{"type": "string", "description": "Spatial location in the scene, e.g. 'left side near exit door', 'overhead center', 'ground level right'"},
						"camera_id":   map[string]any{"type": "string", "description": "Camera that captured this hazard"},
					},
					"required": []string{"description", "severity", "confidence"},
				},
			},
			"text_response":  map[string]any{"type": "string", "description": "Brief summary of findings"},
			"voice_response": map[string]any{"type": "string", "description": "Short spoken alert if hazards found, empty if safe"},
			"scene_safe":     map[string]any{"type": "boolean"},
		},
		"required": []string{"hazards", "text_response", "scene_safe"},
	}
}

func BuildInspectionPrompt(
	modulePrompt string,
	mode string,
	cameraID string,
	alertThreshold string,
	runtimeContext string,
	environmentFamiliarity string,
) string {
	sections := []string{
		"ROLE\nYou are ARGUS, a high-reliability visual safety inspection copilot. Your job is to identify visually supported hazards, compliance risks, and notable safety conditions without inventing evidence.",
		"CORE POLICY\n- Ground every finding in visible evidence from the current scene.\n- Do not invent unseen hazards, hidden causes, or standards that cannot be reasonably inferred.\n- If evidence is ambiguous, reduce confidence and say what needs physical verification.\n- Prefer precision over recall when evidence is weak.\n- When no meaningful hazards are visible, return an empty hazards array and set scene_safe to true.\n- Keep text_response concise and operationally useful.\n- Use voice_response only for concise operator-friendly speech. It may be empty when no spoken alert is warranted.",
	}

	modulePrompt = strings.TrimSpace(modulePrompt)
	if modulePrompt != "" {
		sections = append(sections, "DOMAIN GUIDANCE\n"+modulePrompt)
	}

	runtimeLines := []string{
		"Active inspection mode: " + mode,
	}
	if cameraID != "" {
		runtimeLines = append(runtimeLines, "Camera: "+cameraID)
	}
	if alertThreshold != "" {
		runtimeLines = append(runtimeLines, "Spoken findings threshold: "+alertThreshold+" and above. Stay conversational, but only proactively announce findings that meet this threshold.")
	}
	runtimeLines = append(runtimeLines,
		"Describe hazard locations spatially (for example: left side, near exit, overhead, ground level, foreground/background).",
		"Reference the camera when relevant.",
	)
	if strings.TrimSpace(environmentFamiliarity) != "" {
		runtimeLines = append(runtimeLines, "Retrieved environment familiarity:\n"+strings.TrimSpace(environmentFamiliarity))
	}
	if strings.TrimSpace(runtimeContext) != "" {
		runtimeLines = append(runtimeLines, strings.TrimSpace(runtimeContext))
	}
	sections = append(sections, "RUNTIME CONTEXT\n"+strings.Join(runtimeLines, "\n"))

	sections = append(sections,
		"OUTPUT CONTRACT\nReturn valid JSON only. Use this schema:\n{\n  \"hazards\": [\n    {\n      \"rule_id\": \"string\",\n      \"description\": \"string\",\n      \"severity\": \"low|medium|high|critical\",\n      \"confidence\": 0.0,\n      \"location\": \"string\",\n      \"camera_id\": \"string\"\n    }\n  ],\n  \"text_response\": \"short operator summary\",\n  \"voice_response\": \"short spoken summary or empty string\",\n  \"scene_safe\": true\n}",
		"DECISION RULES\n- Include only findings supported by visible evidence.\n- If a standard is referenced, only do so when it is reasonably applicable.\n- If the scene is partially occluded or ambiguous, mention uncertainty in text_response.\n- Rank imminent life-safety risks above routine deficiencies.\n- Do not output markdown fences or commentary outside the JSON object.",
	)

	return strings.Join(sections, "\n\n")
}

// buildAnalysisPrompt constructs the vision analysis prompt from rules and detected objects.
func buildAnalysisPrompt(req types.GeminiRequest) string {
	runtimeParts := make([]string, 0, 4)
	if req.Frame != nil {
		runtimeParts = append(runtimeParts, fmt.Sprintf("Frame captured: %s", req.Frame.Timestamp.Format("2006-01-02T15:04:05Z07:00")))
	}
	if len(req.Objects) > 0 {
		objectLines := make([]string, 0, len(req.Objects))
		for _, obj := range req.Objects {
			objectLines = append(objectLines, fmt.Sprintf("- %s (confidence: %.0f%%)", obj.Label, obj.Confidence*100))
		}
		runtimeParts = append(runtimeParts, "Objects detected in scene:\n"+strings.Join(objectLines, "\n"))
	}
	return BuildInspectionPrompt(
		"",
		"one-shot analysis",
		func() string {
			if req.Frame != nil {
				return req.Frame.CameraID
			}
			return ""
		}(),
		"",
		strings.TrimSpace(strings.Join([]string{strings.Join(runtimeParts, "\n"), req.Context}, "\n\n")),
		"",
	)
}

// parseGenerateContentResponse extracts structured data from a GenerateContent result.
func parseGenerateContentResponse(result *genai.GenerateContentResponse) (*types.GeminiResponse, error) {
	if result == nil || len(result.Candidates) == 0 {
		return &types.GeminiResponse{
			TextResponse: "No analysis available for this frame.",
		}, nil
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return &types.GeminiResponse{
			TextResponse: "No content in response.",
		}, nil
	}

	text := ""
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}

	if text == "" {
		return &types.GeminiResponse{
			TextResponse: "Empty response from model.",
		}, nil
	}

	var resp types.GeminiResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		// Not valid JSON — treat as plain text
		return &types.GeminiResponse{
			TextResponse:  text,
			VoiceResponse: text,
		}, nil
	}

	return &resp, nil
}
