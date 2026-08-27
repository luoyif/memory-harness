package growth

import (
	"encoding/json"
	"strings"

	"github.com/luoyif/memory-harness/internal/blueprint"
)

type blueprintGrowthPolicy struct {
	Known     bool
	Enabled   map[string]bool
	Blueprint string
	Version   string
}

func growthPolicyFromSnapshot(raw json.RawMessage) blueprintGrowthPolicy {
	policy := blueprintGrowthPolicy{Enabled: map[string]bool{}}
	if len(raw) == 0 {
		return policy
	}
	var current blueprint.Current
	if err := json.Unmarshal(raw, &current); err != nil {
		return policy
	}
	policy.Known = true
	policy.Blueprint = current.Blueprint.BlueprintID
	policy.Version = current.Blueprint.Version
	for _, track := range current.Blueprint.Definition.Tracks {
		if track.Role != "growth" && track.TrackID != "growth" {
			continue
		}
		for _, node := range track.Nodes {
			policy.Enabled[strings.TrimSpace(node.Role)] = node.Enabled
		}
	}
	return policy
}

func (p blueprintGrowthPolicy) roleEnabled(role string) bool {
	if !p.Known {
		return true
	}
	enabled, exists := p.Enabled[role]
	if !exists {
		return true
	}
	return enabled
}
