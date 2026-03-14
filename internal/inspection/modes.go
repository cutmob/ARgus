package inspection

import "strings"

// ModeAliases maps spoken/colloquial names to canonical module names.
// This is the single source of truth — intent_parser, controller, and tool
// descriptions all derive from this map.
var ModeAliases = map[string]string{
	"elevator":     "elevator",
	"lift":         "elevator",
	"construction": "construction",
	"building site": "construction",
	"warehouse":    "warehouse",
	"storage":      "warehouse",
	"facility":     "facility",
	"restaurant":   "restaurant",
	"kitchen":      "kitchen",
	"factory":      "manufacturing",
	"shop floor":   "manufacturing",
	"manufacturing": "manufacturing",
	"office":       "office",
	"retail":       "retail",
	"shop":         "retail",
	"parking":      "parking",
	"car park":     "parking",
	"datacenter":   "datacenter",
	"data center":  "datacenter",
	"data centre":  "datacenter",
	"cold storage": "cold-storage",
	"cold room":    "cold-storage",
	"freezer":      "cold-storage",
	"loading dock": "loading-dock",
	"loading bay":  "loading-dock",
	"laboratory":   "laboratory",
	"lab":          "laboratory",
	"healthcare":   "healthcare",
	"hospital":     "healthcare",
	"hotel":        "hotel",
	"school":       "school",
	"electrical":   "electrical",
	"rooftop":      "rooftop",
	"roof":         "rooftop",
	"refinery":     "refinery",
	"fleet":        "fleet",
	"general":      "general",
}

// ResolveModeAlias normalises a spoken mode name to its canonical module name.
func ResolveModeAlias(mode string) string {
	if resolved, ok := ModeAliases[strings.ToLower(strings.TrimSpace(mode))]; ok {
		return resolved
	}
	return mode
}

// CanonicalModes returns de-duplicated canonical module names, sorted for
// deterministic output (e.g. tool description strings).
func CanonicalModes() []string {
	seen := make(map[string]bool)
	var modes []string
	for _, v := range ModeAliases {
		if !seen[v] {
			seen[v] = true
			modes = append(modes, v)
		}
	}
	// Sort for deterministic output
	for i := 0; i < len(modes); i++ {
		for j := i + 1; j < len(modes); j++ {
			if modes[j] < modes[i] {
				modes[i], modes[j] = modes[j], modes[i]
			}
		}
	}
	return modes
}
