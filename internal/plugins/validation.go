package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

var (
	reverseDomainPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9][a-z0-9-]*)+$`)
	semverPattern        = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)
)

var knownCapabilities = map[string]bool{
	"evidence.read": true, "evidence.capture": true,
	"memory.read": true, "memory.propose": true, "memory.materialize": true,
	"project.read": true, "project.write": true,
	"finance.read": true, "finance.write": true,
	"asset.read": true, "asset.propose": true, "asset.activate": true,
	"model.invoke": true, "connector.invoke": true, "notification.emit": true,
	"trace.read_payload": true,
	"team.private":       true, "team.blackboard.read": true, "team.blackboard.write": true, "team.blackboard.share": true,
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func normalizeCapabilities(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !knownCapabilities[value] {
			return nil, fmt.Errorf("unknown capability %q", value)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func validateManifest(pkg parsedPackage) (Manifest, error) {
	manifest := pkg.Manifest
	if manifest.APIVersion != APIVersion || manifest.Kind != "Plugin" {
		return Manifest{}, fmt.Errorf("manifest must be apiVersion %s and kind Plugin", APIVersion)
	}
	if !reverseDomainPattern.MatchString(manifest.Metadata.ID) {
		return Manifest{}, errors.New("metadata.id must be a reverse-domain identifier")
	}
	if strings.TrimSpace(manifest.Metadata.Name) == "" || strings.TrimSpace(manifest.Metadata.Publisher) == "" || strings.TrimSpace(manifest.Metadata.License) == "" {
		return Manifest{}, errors.New("metadata name, publisher and license are required")
	}
	if !semverPattern.MatchString(manifest.Metadata.Version) {
		return Manifest{}, errors.New("metadata.version must be semantic versioning")
	}
	if manifest.Compatibility.MemoryHarness == "" {
		return Manifest{}, errors.New("compatibility.memoryHarness is required")
	}
	if manifest.Trust.Class != "declarative" && manifest.Trust.Class != "wasm" && manifest.Trust.Class != "external" {
		return Manifest{}, errors.New("trust.class must be declarative, wasm or external")
	}
	if manifest.Trust.Class == "wasm" {
		return Manifest{}, errors.New("WASM plugins are not admitted: security spike has not passed")
	}
	permissions, err := normalizeCapabilities(append(append([]string{}, manifest.Permissions.Required...), manifest.Permissions.Optional...))
	if err != nil {
		return Manifest{}, err
	}
	_ = permissions
	if schemaPath := strings.TrimSpace(manifest.Configuration.Schema); schemaPath != "" {
		if !safePackagePath(schemaPath) || !strings.HasPrefix(schemaPath, "schemas/") {
			return Manifest{}, errors.New("configuration.schema must be a safe schemas/ path")
		}
		raw, ok := pkg.Files[schemaPath]
		if !ok {
			return Manifest{}, errors.New("configuration schema is missing")
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil || schema["type"] != "object" {
			return Manifest{}, errors.New("configuration schema must be a JSON object schema")
		}
	}
	seenTypes := map[string]bool{}
	for _, item := range manifest.Contributes.MemoryTypes {
		if !strings.HasPrefix(item.TypeID, manifest.Metadata.ID+".") {
			return Manifest{}, fmt.Errorf("memory type %q must be namespaced by plugin id", item.TypeID)
		}
		if seenTypes[item.TypeID] {
			return Manifest{}, fmt.Errorf("duplicate memory type %q", item.TypeID)
		}
		seenTypes[item.TypeID] = true
		if !safePackagePath(item.SchemaPath) || !strings.HasPrefix(item.SchemaPath, "schemas/memory-types/") {
			return Manifest{}, fmt.Errorf("memory type %q has invalid schema path", item.TypeID)
		}
		if _, ok := pkg.Files[item.SchemaPath]; !ok {
			return Manifest{}, fmt.Errorf("memory type %q schema is missing", item.TypeID)
		}
		if item.RendererPath != "" {
			if !safePackagePath(item.RendererPath) || !strings.HasPrefix(item.RendererPath, "ui/") {
				return Manifest{}, fmt.Errorf("memory type %q has invalid renderer path", item.TypeID)
			}
			if _, ok := pkg.Files[item.RendererPath]; !ok {
				return Manifest{}, fmt.Errorf("memory type %q renderer is missing", item.TypeID)
			}
		}
	}
	for _, item := range manifest.Contributes.Pipelines {
		if !strings.HasPrefix(item.PipelineID, manifest.Metadata.ID+".") || !safePackagePath(item.Definition) || !strings.HasPrefix(item.Definition, "pipelines/") {
			return Manifest{}, fmt.Errorf("pipeline %q is not namespaced or has an invalid definition path", item.PipelineID)
		}
		if _, ok := pkg.Files[item.Definition]; !ok {
			return Manifest{}, fmt.Errorf("pipeline %q definition is missing", item.PipelineID)
		}
	}
	seenComponents := map[string]bool{}
	for _, item := range manifest.Contributes.StrategyComponents {
		if !strings.HasPrefix(item.ComponentID, manifest.Metadata.ID+".") || !semverPattern.MatchString(item.Version) {
			return Manifest{}, fmt.Errorf("strategy component %q is not namespaced or versioned", item.ComponentID)
		}
		if seenComponents[item.ComponentID+"@"+item.Version] {
			return Manifest{}, fmt.Errorf("duplicate strategy component %q", item.ComponentID)
		}
		seenComponents[item.ComponentID+"@"+item.Version] = true
		if strings.TrimSpace(item.DisplayName) == "" || strings.TrimSpace(item.Role) == "" {
			return Manifest{}, fmt.Errorf("strategy component %q requires displayName and role", item.ComponentID)
		}
		if item.Kind != "memory_type" && item.Kind != "stage" && item.Kind != "provider" && item.Kind != "policy" {
			return Manifest{}, fmt.Errorf("strategy component %q has invalid kind", item.ComponentID)
		}
		if _, err := normalizeCapabilities(item.Capabilities); err != nil {
			return Manifest{}, fmt.Errorf("strategy component %q: %w", item.ComponentID, err)
		}
		if item.Configuration != "" {
			if !safePackagePath(item.Configuration) || !strings.HasPrefix(item.Configuration, "schemas/") {
				return Manifest{}, fmt.Errorf("strategy component %q has invalid configuration schema", item.ComponentID)
			}
			if _, ok := pkg.Files[item.Configuration]; !ok {
				return Manifest{}, fmt.Errorf("strategy component %q configuration schema is missing", item.ComponentID)
			}
		}
	}
	for _, item := range manifest.Contributes.Blueprints {
		if !strings.HasPrefix(item.BlueprintID, manifest.Metadata.ID+".") || !semverPattern.MatchString(item.Version) || !safePackagePath(item.Definition) || !strings.HasPrefix(item.Definition, "blueprints/") {
			return Manifest{}, fmt.Errorf("blueprint %q is not namespaced, versioned or has an invalid definition path", item.BlueprintID)
		}
		if _, ok := pkg.Files[item.Definition]; !ok {
			return Manifest{}, fmt.Errorf("blueprint %q definition is missing", item.BlueprintID)
		}
	}
	return manifest, nil
}
