package blueprint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	idPattern         = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9][a-z0-9_-]*)+$`)
	semverPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*\.[a-z][a-z0-9_-]*$`)
)

var sensitiveConfigKeys = map[string]bool{
	"api_key": true, "apikey": true, "secret": true, "password": true,
	"access_token": true, "refresh_token": true, "credential": true,
}

func configHasSecret(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			key = strings.ToLower(strings.TrimSpace(key))
			if sensitiveConfigKeys[key] || configHasSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if configHasSecret(child) {
				return true
			}
		}
	}
	return false
}

func Validate(definition Definition) ValidationResult {
	result := ValidationResult{Errors: []string{}, Warnings: []string{}, TrackCount: len(definition.Tracks)}
	if definition.APIVersion != APIVersion {
		result.Errors = append(result.Errors, fmt.Sprintf("api_version must be %s", APIVersion))
	}
	if !idPattern.MatchString(strings.TrimSpace(definition.BlueprintID)) {
		result.Errors = append(result.Errors, "blueprint_id must be a namespaced identifier")
	}
	if !semverPattern.MatchString(strings.TrimSpace(definition.Version)) {
		result.Errors = append(result.Errors, "version must use semantic versioning")
	}
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Intent) == "" {
		result.Errors = append(result.Errors, "name and intent are required")
	}
	if definition.Policy.EvidenceMode != "verbatim" && definition.Policy.EvidenceMode != "normalized_with_verbatim" {
		result.Errors = append(result.Errors, "policy.evidence_mode must preserve verbatim evidence")
	}
	if definition.Policy.ModelBoundary != "local_only" && definition.Policy.ModelBoundary != "configured_provider" && definition.Policy.ModelBoundary != "rules_only" {
		result.Errors = append(result.Errors, "policy.model_boundary is invalid")
	}
	if definition.Policy.DefaultContextBudget < 512 || definition.Policy.DefaultContextBudget > 1_000_000 {
		result.Errors = append(result.Errors, "policy.default_context_budget must be between 512 and 1000000 characters")
	}

	trackIDs := map[string]bool{}
	requiredTracks := map[string]bool{"growth": false, "organization": false, "recall": false}
	capabilities := map[string]bool{}
	for trackIndex, track := range definition.Tracks {
		prefix := fmt.Sprintf("tracks[%d]", trackIndex)
		if strings.TrimSpace(track.TrackID) == "" || strings.TrimSpace(track.Role) == "" || strings.TrimSpace(track.DisplayName) == "" {
			result.Errors = append(result.Errors, prefix+" requires track_id, role and display_name")
		}
		if trackIDs[track.TrackID] {
			result.Errors = append(result.Errors, prefix+" has a duplicate track_id")
		}
		trackIDs[track.TrackID] = true
		if _, ok := requiredTracks[track.Role]; ok {
			requiredTracks[track.Role] = true
		}
		if len(track.Nodes) == 0 {
			result.Warnings = append(result.Warnings, prefix+" has no strategy components")
		}
		nodeIDs := map[string]bool{}
		for nodeIndex, node := range track.Nodes {
			nodePrefix := fmt.Sprintf("%s.nodes[%d]", prefix, nodeIndex)
			if strings.TrimSpace(node.NodeID) == "" || strings.TrimSpace(node.Role) == "" || strings.TrimSpace(node.DisplayName) == "" {
				result.Errors = append(result.Errors, nodePrefix+" requires node_id, role and display_name")
			}
			if nodeIDs[node.NodeID] {
				result.Errors = append(result.Errors, nodePrefix+" has a duplicate node_id")
			}
			nodeIDs[node.NodeID] = true
			if node.BindingKind != "memory_type" && node.BindingKind != "stage" && node.BindingKind != "provider" && node.BindingKind != "policy" {
				result.Errors = append(result.Errors, nodePrefix+" has an invalid binding_kind")
			}
			if !idPattern.MatchString(node.PluginID) || !semverPattern.MatchString(node.PluginVersion) {
				result.Errors = append(result.Errors, nodePrefix+" has an invalid plugin id or version")
			}
			if !strings.HasPrefix(node.ComponentID, node.PluginID+".") || !semverPattern.MatchString(node.ComponentVersion) {
				result.Errors = append(result.Errors, nodePrefix+" component must be namespaced by its plugin and versioned")
			}
			if len(node.Config) == 0 {
				node.Config = json.RawMessage(`{}`)
			}
			var config any
			if !json.Valid(node.Config) || json.Unmarshal(node.Config, &config) != nil {
				result.Errors = append(result.Errors, nodePrefix+" config must be valid JSON")
			} else if configHasSecret(config) {
				result.Errors = append(result.Errors, nodePrefix+" config contains a secret-like field; use the model or connector secret store")
			}
			for _, capability := range node.RequiredCapabilities {
				if !capabilityPattern.MatchString(capability) {
					result.Errors = append(result.Errors, nodePrefix+" has an invalid capability "+capability)
				} else {
					capabilities[capability] = true
				}
			}
			if node.Enabled {
				result.EnabledComponentCount++
			}
		}
	}
	for role, present := range requiredTracks {
		if !present {
			result.Errors = append(result.Errors, "blueprint requires a "+role+" track")
		}
	}
	for capability := range capabilities {
		result.RequiredCapabilities = append(result.RequiredCapabilities, capability)
	}
	sort.Strings(result.RequiredCapabilities)
	result.Valid = len(result.Errors) == 0
	return result
}
