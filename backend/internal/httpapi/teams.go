package httpapi

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"strings"
)

//go:embed teams.json
var teamsJSON []byte

// teamMap is keyed by lowercased, trimmed email and holds the resolved country name.
var teamMap map[string]string

// knownTeams is the full set of accepted country names (all lowercase) including
// alternate spellings accepted by the game bundle.
var knownTeams = []string{
	"argentina",
	"brasil",
	"brazil",
	"germany",
	"netharland",
	"netherlands",
	"holland",
	"spain",
	"portugal",
	"france",
	"belgium",
}

func init() {
	raw := map[string]string{}
	if err := json.Unmarshal(teamsJSON, &raw); err != nil {
		slog.Error("teams: failed to parse teams.json", "err", err)
		teamMap = map[string]string{}
		return
	}

	teamMap = make(map[string]string, len(raw))
	for k, v := range raw {
		// Skip comment/meta keys that lack an "@" sign.
		if !strings.Contains(k, "@") {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		teamMap[key] = v

		// Warn on typos: value must match one of the known accepted names (case-insensitive).
		lower := strings.ToLower(strings.TrimSpace(v))
		known := false
		for _, t := range knownTeams {
			if lower == t {
				known = true
				break
			}
		}
		if !known {
			slog.Warn("teams: unrecognised country name in teams.json — bundle will show neutral flag",
				"email", k, "team", v)
		}
	}
}

// teamForEmail returns the mapped country name for the given email address, or
// "" if the email is not in the map. Lookup is case-insensitive.
func teamForEmail(email string) string {
	return teamMap[strings.ToLower(strings.TrimSpace(email))]
}
