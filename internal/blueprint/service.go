package blueprint

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/store"
	"gopkg.in/yaml.v3"
)

type Service struct{ control *store.ControlStore }

func New(control *store.ControlStore) *Service { return &Service{control: control} }

func (s *Service) decodeDefinition(pluginID string, raw []byte) (Definition, error) {
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	var definition Definition
	if err := decoder.Decode(&definition); err != nil {
		return Definition{}, err
	}
	if !strings.HasPrefix(definition.BlueprintID, strings.TrimSpace(pluginID)+".") {
		return Definition{}, errors.New("blueprint_id must be namespaced by plugin_id")
	}
	for trackIndex := range definition.Tracks {
		for nodeIndex := range definition.Tracks[trackIndex].Nodes {
			node := &definition.Tracks[trackIndex].Nodes[nodeIndex]
			if node.ConfigYAML != nil {
				encoded, err := json.Marshal(node.ConfigYAML)
				if err != nil {
					return Definition{}, err
				}
				node.Config = encoded
			}
		}
	}
	return definition, nil
}

func (s *Service) ValidateDefinition(pluginID string, raw []byte) error {
	definition, err := s.decodeDefinition(pluginID, raw)
	if err != nil {
		return err
	}
	result := Validate(definition)
	if !result.Valid {
		return errors.New(strings.Join(result.Errors, "; "))
	}
	return nil
}

func (s *Service) PublishDefinition(ctx context.Context, pluginID string, raw []byte) error {
	definition, err := s.decodeDefinition(pluginID, raw)
	if err != nil {
		return err
	}
	_, err = s.Publish(ctx, pluginID, definition)
	return err
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func hashDefinition(definition Definition) (string, []byte, error) {
	raw, err := json.Marshal(definition)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), raw, nil
}

func (s *Service) Publish(ctx context.Context, pluginID string, definition Definition) (Version, error) {
	pluginID = strings.TrimSpace(pluginID)
	definition.BlueprintID = strings.TrimSpace(definition.BlueprintID)
	definition.Version = strings.TrimSpace(definition.Version)
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.Intent = strings.TrimSpace(definition.Intent)
	if !idPattern.MatchString(pluginID) {
		return Version{}, errors.New("plugin_id must be a namespaced identifier")
	}
	if !strings.HasPrefix(definition.BlueprintID, pluginID+".") {
		return Version{}, errors.New("blueprint_id must be namespaced by plugin_id")
	}
	for trackIndex := range definition.Tracks {
		for nodeIndex := range definition.Tracks[trackIndex].Nodes {
			node := &definition.Tracks[trackIndex].Nodes[nodeIndex]
			if len(node.Config) == 0 && node.ConfigYAML != nil {
				raw, err := json.Marshal(node.ConfigYAML)
				if err != nil {
					return Version{}, fmt.Errorf("track %s node %s config: %w", definition.Tracks[trackIndex].TrackID, node.NodeID, err)
				}
				node.Config = raw
			}
			if len(node.Config) == 0 {
				node.Config = json.RawMessage(`{}`)
			}
			node.ConfigYAML = nil
			sort.Strings(node.RequiredCapabilities)
		}
	}
	validation := Validate(definition)
	if !validation.Valid {
		return Version{}, errors.New(strings.Join(validation.Errors, "; "))
	}
	hash, raw, err := hashDefinition(definition)
	if err != nil {
		return Version{}, err
	}
	if current, err := s.Version(ctx, definition.BlueprintID, definition.Version); err == nil {
		if current.ContentHash == hash {
			return current, nil
		}
		return Version{}, errors.New("published blueprint version is immutable")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Version{}, err
	}
	now := nowString()
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_blueprint_versions(blueprint_id,version,plugin_id,name,description,definition_json,content_hash,status,created_at) VALUES(?,?,?,?,?,?,?,'published',?)`, definition.BlueprintID, definition.Version, pluginID, definition.Name, definition.Description, string(raw), hash, now)
	if err != nil {
		return Version{}, err
	}
	return s.Version(ctx, definition.BlueprintID, definition.Version)
}

func scanVersion(scanner interface{ Scan(...any) error }) (Version, error) {
	var item Version
	var raw string
	err := scanner.Scan(&item.BlueprintID, &item.Version, &item.PluginID, &item.Name, &item.Description, &raw, &item.ContentHash, &item.Status, &item.CreatedAt)
	if err != nil {
		return Version{}, err
	}
	if err = json.Unmarshal([]byte(raw), &item.Definition); err != nil {
		return Version{}, err
	}
	return item, nil
}

func (s *Service) Version(ctx context.Context, blueprintID, version string) (Version, error) {
	return scanVersion(s.control.DB.QueryRowContext(ctx, `SELECT blueprint_id,version,plugin_id,name,description,definition_json,content_hash,status,created_at FROM harness_blueprint_versions WHERE blueprint_id=? AND version=?`, strings.TrimSpace(blueprintID), strings.TrimSpace(version)))
}

func (s *Service) List(ctx context.Context) ([]Version, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT blueprint_id,version FROM harness_blueprint_versions
		WHERE blueprint_id!=? OR NOT EXISTS(SELECT 1 FROM harness_blueprint_versions WHERE blueprint_id=? AND version=?)
		ORDER BY CASE WHEN blueprint_id=? THEN 0 ELSE 1 END,name,created_at DESC`, LegacyDefaultBlueprintID, DefaultBlueprintID, DefaultBlueprintVersion, DefaultBlueprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ id, version string }
	keys := []key{}
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.id, &item.version); err != nil {
			return nil, err
		}
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]Version, 0, len(keys))
	for _, key := range keys {
		item, err := s.Version(ctx, key.id, key.version)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func scanAssignment(scanner interface{ Scan(...any) error }) (Assignment, error) {
	var item Assignment
	err := scanner.Scan(&item.ProjectID, &item.BlueprintID, &item.BlueprintVersion, &item.BlueprintHash, &item.Status, &item.ActivatedBy, &item.ActivatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) assignment(ctx context.Context, projectID string) (Assignment, error) {
	return scanAssignment(s.control.DB.QueryRowContext(ctx, `SELECT project_id,blueprint_id,blueprint_version,blueprint_hash,status,activated_by,activated_at,updated_at FROM harness_project_blueprints WHERE project_id=?`, strings.TrimSpace(projectID)))
}

func (s *Service) projectExists(ctx context.Context, projectID string) error {
	var found string
	if err := s.control.DB.QueryRowContext(ctx, `SELECT project_id FROM projects WHERE project_id=? AND status!='archived'`, strings.TrimSpace(projectID)).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("project does not exist or is archived")
		}
		return err
	}
	return nil
}

func stringSet(raw string) map[string]bool {
	values := []string{}
	_ = json.Unmarshal([]byte(raw), &values)
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func (s *Service) validateAvailability(ctx context.Context, projectID string, definition Definition) ValidationResult {
	result := Validate(definition)
	if !result.Valid {
		return result
	}
	seen := map[string]bool{}
	for _, track := range definition.Tracks {
		for _, node := range track.Nodes {
			if !node.Enabled {
				continue
			}
			key := node.PluginID + "@" + node.PluginVersion
			if seen[key] {
				continue
			}
			seen[key] = true
			var status, permissionsRaw, contributionsRaw string
			if err := s.control.DB.QueryRowContext(ctx, `SELECT status,permissions_json,contributions_json FROM harness_plugin_versions WHERE plugin_id=? AND version=?`, node.PluginID, node.PluginVersion).Scan(&status, &permissionsRaw, &contributionsRaw); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("component %s requires unavailable plugin %s", node.ComponentID, key))
				continue
			}
			var contributionSet struct {
				StrategyComponents []struct {
					ComponentID  string   `json:"component_id"`
					Version      string   `json:"version"`
					Role         string   `json:"role"`
					Kind         string   `json:"kind"`
					Capabilities []string `json:"capabilities"`
				} `json:"strategy_components"`
			}
			_ = json.Unmarshal([]byte(contributionsRaw), &contributionSet)
			var declared *struct {
				ComponentID  string   `json:"component_id"`
				Version      string   `json:"version"`
				Role         string   `json:"role"`
				Kind         string   `json:"kind"`
				Capabilities []string `json:"capabilities"`
			}
			for index := range contributionSet.StrategyComponents {
				candidate := &contributionSet.StrategyComponents[index]
				if candidate.ComponentID == node.ComponentID && candidate.Version == node.ComponentVersion {
					declared = candidate
					break
				}
			}
			if declared == nil || declared.Role != node.Role || declared.Kind != node.BindingKind {
				result.Errors = append(result.Errors, fmt.Sprintf("component %s is not declared for role %s and kind %s", node.ComponentID, node.Role, node.BindingKind))
				continue
			}
			var projectStatus, grantedRaw string
			stateErr := s.control.DB.QueryRowContext(ctx, `SELECT status,granted_capabilities_json FROM harness_plugin_project_state WHERE plugin_id=? AND version=? AND project_id=?`, node.PluginID, node.PluginVersion, projectID).Scan(&projectStatus, &grantedRaw)
			explicit := stateErr == nil
			if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
				result.Errors = append(result.Errors, fmt.Sprintf("cannot inspect project grant for %s", key))
				continue
			}
			if status == "experimental" && (!explicit || projectStatus != "enabled") {
				result.Errors = append(result.Errors, fmt.Sprintf("experimental plugin %s must be explicitly enabled for this project", key))
				continue
			}
			if status != "enabled" && status != "experimental" {
				result.Errors = append(result.Errors, fmt.Sprintf("plugin %s is %s", key, status))
				continue
			}
			if explicit && projectStatus != "enabled" {
				result.Errors = append(result.Errors, fmt.Sprintf("plugin %s is disabled for this project", key))
				continue
			}
			allowed := stringSet(permissionsRaw)
			if explicit {
				allowed = stringSet(grantedRaw)
			}
			for _, capability := range node.RequiredCapabilities {
				declaredCapabilities := map[string]bool{}
				for _, value := range declared.Capabilities {
					declaredCapabilities[value] = true
				}
				if !declaredCapabilities[capability] {
					result.Errors = append(result.Errors, fmt.Sprintf("component %s did not declare capability %s", node.ComponentID, capability))
					continue
				}
				if !allowed[capability] {
					result.Errors = append(result.Errors, fmt.Sprintf("component %s requires project capability %s", node.ComponentID, capability))
				}
			}
		}
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func (s *Service) Current(ctx context.Context, projectID string) (Current, error) {
	projectID = strings.TrimSpace(projectID)
	if err := s.projectExists(ctx, projectID); err != nil {
		return Current{}, err
	}
	assignment, err := s.assignment(ctx, projectID)
	inherited := false
	if errors.Is(err, sql.ErrNoRows) {
		inherited = true
		version, versionErr := s.Version(ctx, DefaultBlueprintID, DefaultBlueprintVersion)
		if versionErr != nil {
			return Current{}, versionErr
		}
		assignment = Assignment{ProjectID: projectID, BlueprintID: version.BlueprintID, BlueprintVersion: version.Version, BlueprintHash: version.ContentHash, Status: "inherited", ActivatedBy: "system"}
	} else if err != nil {
		return Current{}, err
	}
	version, err := s.Version(ctx, assignment.BlueprintID, assignment.BlueprintVersion)
	if err != nil {
		return Current{}, err
	}
	return Current{Assignment: assignment, Blueprint: version, Inherited: inherited, Validation: s.validateAvailability(ctx, projectID, version.Definition)}, nil
}

func (s *Service) Activate(ctx context.Context, projectID, blueprintID, version, ownerID string) (Current, error) {
	projectID = strings.TrimSpace(projectID)
	if err := s.projectExists(ctx, projectID); err != nil {
		return Current{}, err
	}
	item, err := s.Version(ctx, blueprintID, version)
	if err != nil {
		return Current{}, err
	}
	validation := s.validateAvailability(ctx, projectID, item.Definition)
	if !validation.Valid {
		return Current{}, errors.New(strings.Join(validation.Errors, "; "))
	}
	now := nowString()
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_project_blueprints(project_id,blueprint_id,blueprint_version,blueprint_hash,status,activated_by,activated_at,updated_at) VALUES(?,?,?,?,'active',?,?,?) ON CONFLICT(project_id) DO UPDATE SET blueprint_id=excluded.blueprint_id,blueprint_version=excluded.blueprint_version,blueprint_hash=excluded.blueprint_hash,status=excluded.status,activated_by=excluded.activated_by,activated_at=excluded.activated_at,updated_at=excluded.updated_at`, projectID, item.BlueprintID, item.Version, item.ContentHash, strings.TrimSpace(ownerID), now, now)
	if err != nil {
		return Current{}, err
	}
	return s.Current(ctx, projectID)
}

func (s *Service) Snapshot(ctx context.Context, projectID string) (json.RawMessage, error) {
	current, err := s.Current(ctx, projectID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(current)
	return json.RawMessage(raw), err
}
