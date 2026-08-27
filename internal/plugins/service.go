package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control    *store.ControlStore
	harness    *harness.Service
	pipelines  PipelinePublisher
	blueprints BlueprintPublisher
}

type PipelinePublisher interface {
	ValidateDefinition(pluginID string, raw []byte) error
	PublishDefinition(ctx context.Context, pluginID string, raw []byte) error
}

type BlueprintPublisher interface {
	ValidateDefinition(pluginID string, raw []byte) error
	PublishDefinition(ctx context.Context, pluginID string, raw []byte) error
}

func New(control *store.ControlStore, harnessService *harness.Service, pipelines ...PipelinePublisher) *Service {
	service := &Service{control: control, harness: harnessService}
	if len(pipelines) > 0 {
		service.pipelines = pipelines[0]
	}
	return service
}

func (s *Service) SetBlueprintPublisher(publisher BlueprintPublisher) { s.blueprints = publisher }

func stringJSON(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func auditID() string {
	id, _ := harnessID("plugin-event-")
	return id
}

func harnessID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := cryptoRandRead(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func (s *Service) audit(ctx context.Context, pluginID, version, action, status string, detail any) {
	raw, _ := json.Marshal(detail)
	_, _ = s.control.DB.ExecContext(ctx, `INSERT INTO harness_plugin_audit(event_id,plugin_id,version,action,status,detail_json,created_at) VALUES(?,?,?,?,?,?,?)`, auditID(), pluginID, version, action, status, string(raw), nowUTC())
}

func (s *Service) ApproveSigner(ctx context.Context, input TrustSignerInput) (TrustedSigner, error) {
	input.SignerID = strings.TrimSpace(input.SignerID)
	input.Publisher = strings.TrimSpace(input.Publisher)
	if !reverseDomainPattern.MatchString(input.SignerID) || input.Publisher == "" {
		return TrustedSigner{}, errors.New("valid signer_id and publisher are required")
	}
	publicKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input.PublicKeyBase64))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return TrustedSigner{}, errors.New("public_key_base64 must contain an Ed25519 public key")
	}
	scope := input.Scope
	if len(scope) == 0 {
		scope = []string{input.Publisher + ".*"}
	}
	for _, entry := range scope {
		if entry != input.Publisher+".*" && entry != input.Publisher {
			return TrustedSigner{}, errors.New("signer scope must remain within its publisher namespace")
		}
	}
	sum := sha256.Sum256(publicKey)
	fingerprint := "sha256:" + hex.EncodeToString(sum[:])
	now := nowUTC()
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_trusted_signers(signer_id,publisher,public_key_base64,fingerprint,scope_json,status,approved_at) VALUES(?,?,?,?,?,'active',?) ON CONFLICT(signer_id) DO UPDATE SET publisher=excluded.publisher,public_key_base64=excluded.public_key_base64,fingerprint=excluded.fingerprint,scope_json=excluded.scope_json,status='active',approved_at=excluded.approved_at,revoked_at=NULL`, input.SignerID, input.Publisher, strings.TrimSpace(input.PublicKeyBase64), fingerprint, stringJSON(scope), now)
	if err != nil {
		return TrustedSigner{}, err
	}
	return s.Signer(ctx, input.SignerID)
}

func (s *Service) Signer(ctx context.Context, signerID string) (TrustedSigner, error) {
	var item TrustedSigner
	var scopeRaw string
	var revoked sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT signer_id,publisher,fingerprint,scope_json,status,approved_at,revoked_at FROM harness_trusted_signers WHERE signer_id=?`, signerID).Scan(&item.SignerID, &item.Publisher, &item.Fingerprint, &scopeRaw, &item.Status, &item.ApprovedAt, &revoked)
	if err != nil {
		return TrustedSigner{}, err
	}
	_ = json.Unmarshal([]byte(scopeRaw), &item.Scope)
	item.RevokedAt = revoked.String
	return item, nil
}

func (s *Service) RevokeSigner(ctx context.Context, signerID string) error {
	now := nowUTC()
	result, err := s.control.DB.ExecContext(ctx, `UPDATE harness_trusted_signers SET status='revoked',revoked_at=? WHERE signer_id=? AND status='active'`, now, strings.TrimSpace(signerID))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return sql.ErrNoRows
	}
	_, err = s.control.DB.ExecContext(ctx, `UPDATE harness_plugin_versions SET status='quarantined',updated_at=? WHERE signer_id=? AND status='enabled'`, now, strings.TrimSpace(signerID))
	return err
}

func signerAllows(scope []string, publisher, pluginID string) bool {
	for _, item := range scope {
		if item == publisher && pluginID == publisher || item == publisher+".*" && strings.HasPrefix(pluginID, publisher+".") {
			return true
		}
	}
	return false
}

func (s *Service) verifySignature(ctx context.Context, pkg parsedPackage, developerMode bool) (string, string, error) {
	if pkg.Signature.SignerID == "" {
		if developerMode {
			return "", "developer_unsigned", nil
		}
		return "", "missing", errors.New("signed plugin package required outside developer mode")
	}
	if pkg.Signature.Algorithm != "ed25519" {
		return "", "invalid", errors.New("only Ed25519 plugin signatures are supported")
	}
	var publicKeyRaw, publisher, scopeRaw, status string
	err := s.control.DB.QueryRowContext(ctx, `SELECT public_key_base64,publisher,scope_json,status FROM harness_trusted_signers WHERE signer_id=?`, pkg.Signature.SignerID).Scan(&publicKeyRaw, &publisher, &scopeRaw, &status)
	if err != nil {
		return pkg.Signature.SignerID, "untrusted", errors.New("plugin signer is not owner-approved")
	}
	if status != "active" {
		return pkg.Signature.SignerID, "revoked", errors.New("plugin signer is revoked")
	}
	var scope []string
	_ = json.Unmarshal([]byte(scopeRaw), &scope)
	if publisher != pkg.Manifest.Metadata.Publisher || !signerAllows(scope, publisher, pkg.Manifest.Metadata.ID) {
		return pkg.Signature.SignerID, "scope_denied", errors.New("plugin id is outside signer publisher scope")
	}
	publicKey, keyErr := base64.StdEncoding.DecodeString(publicKeyRaw)
	signature, signatureErr := base64.StdEncoding.DecodeString(pkg.Signature.Signature)
	if keyErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !ed25519.Verify(ed25519.PublicKey(publicKey), pkg.Digest, signature) {
		return pkg.Signature.SignerID, "invalid", errors.New("plugin signature verification failed")
	}
	return pkg.Signature.SignerID, "verified", nil
}

func (s *Service) Install(ctx context.Context, raw []byte, options InstallOptions) (PluginVersion, error) {
	pkg, err := parsePackage(raw)
	if err != nil {
		return PluginVersion{}, err
	}
	manifest, err := validateManifest(pkg)
	if err != nil {
		s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "install", "denied", map[string]any{"error": err.Error()})
		return PluginVersion{}, err
	}
	signerID, signatureStatus, err := s.verifySignature(ctx, pkg, options.DeveloperMode)
	if err != nil {
		s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "install", "denied", map[string]any{"error": err.Error(), "signature_status": signatureStatus})
		return PluginVersion{}, err
	}
	required, err := normalizeCapabilities(manifest.Permissions.Required)
	if err != nil {
		return PluginVersion{}, err
	}
	granted, err := normalizeCapabilities(options.Capabilities)
	if err != nil {
		return PluginVersion{}, err
	}
	grantSet := map[string]bool{}
	for _, capability := range granted {
		grantSet[capability] = true
	}
	for _, capability := range required {
		if !grantSet[capability] && options.EnableProject != "" {
			return PluginVersion{}, fmt.Errorf("required capability %q was not granted", capability)
		}
	}

	var existingStatus, existingHash string
	existingErr := s.control.DB.QueryRowContext(ctx, `SELECT status,content_hash FROM harness_plugin_versions WHERE plugin_id=? AND version=?`, manifest.Metadata.ID, manifest.Metadata.Version).Scan(&existingStatus, &existingHash)
	if existingErr != nil && existingErr != sql.ErrNoRows {
		return PluginVersion{}, existingErr
	}
	if existingErr == nil && existingHash != pkg.Hash {
		return PluginVersion{}, errors.New("plugin version already exists with different content; publish a new semantic version")
	}

	// Preflight schemas through the kernel before recording an installation.
	for _, contribution := range manifest.Contributes.MemoryTypes {
		renderer := json.RawMessage(`{}`)
		if contribution.RendererPath != "" {
			renderer = pkg.Files[contribution.RendererPath]
		}
		if _, err := s.harness.RegisterType(ctx, harness.RegisterTypeInput{
			TypeID: contribution.TypeID, PluginID: manifest.Metadata.ID, DisplayName: contribution.DisplayName,
			SchemaVersion: contribution.SchemaVersion, Schema: pkg.Files[contribution.SchemaPath],
			Lifecycle: contribution.Lifecycle, ProtectionClass: contribution.ProtectionClass, Renderer: renderer,
		}); err != nil {
			s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "install", "failed", map[string]any{"memory_type": contribution.TypeID, "error": err.Error()})
			return PluginVersion{}, fmt.Errorf("memory type %s: %w", contribution.TypeID, err)
		}
	}
	if len(manifest.Contributes.Pipelines) > 0 && s.pipelines == nil {
		return PluginVersion{}, errors.New("pipeline runtime is unavailable")
	}
	for _, contribution := range manifest.Contributes.Pipelines {
		if err := s.pipelines.ValidateDefinition(manifest.Metadata.ID, pkg.Files[contribution.Definition]); err != nil {
			return PluginVersion{}, fmt.Errorf("pipeline %s: %w", contribution.PipelineID, err)
		}
	}
	if len(manifest.Contributes.Blueprints) > 0 && s.blueprints == nil {
		return PluginVersion{}, errors.New("blueprint runtime is unavailable")
	}
	for _, contribution := range manifest.Contributes.Blueprints {
		if err := s.blueprints.ValidateDefinition(manifest.Metadata.ID, pkg.Files[contribution.Definition]); err != nil {
			return PluginVersion{}, fmt.Errorf("blueprint %s: %w", contribution.BlueprintID, err)
		}
	}
	contributionsJSON, _ := json.Marshal(manifest.Contributes)
	allPermissions, _ := normalizeCapabilities(append(append([]string{}, manifest.Permissions.Required...), manifest.Permissions.Optional...))
	now := nowUTC()
	status := "installed"
	if options.EnableProject != "" {
		status = "enabled"
	}
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_plugin_versions(plugin_id,version,name,publisher,trust_class,signer_id,signature_status,content_hash,manifest_yaml,package_blob,package_size,permissions_json,contributions_json,status,installed_at,updated_at) VALUES(?,?,?,?,?,nullif(?,''),?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(plugin_id,version) DO UPDATE SET
  name=excluded.name,publisher=excluded.publisher,trust_class=excluded.trust_class,signer_id=excluded.signer_id,signature_status=excluded.signature_status,
  manifest_yaml=excluded.manifest_yaml,package_blob=excluded.package_blob,package_size=excluded.package_size,permissions_json=excluded.permissions_json,
  contributions_json=excluded.contributions_json,status=excluded.status,updated_at=excluded.updated_at
WHERE harness_plugin_versions.status='uninstalled' AND harness_plugin_versions.content_hash=excluded.content_hash`, manifest.Metadata.ID, manifest.Metadata.Version, manifest.Metadata.Name, manifest.Metadata.Publisher, manifest.Trust.Class, signerID, signatureStatus, pkg.Hash, string(pkg.ManifestRaw), raw, len(raw), stringJSON(allPermissions), string(contributionsJSON), status, now, now)
	if err != nil {
		return PluginVersion{}, err
	}
	if options.EnableProject != "" {
		_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_plugin_project_state(plugin_id,version,project_id,status,granted_capabilities_json,config_json,updated_at) VALUES(?,?,?,'enabled',?,'{}',?) ON CONFLICT(plugin_id,version,project_id) DO UPDATE SET status='enabled',granted_capabilities_json=excluded.granted_capabilities_json,updated_at=excluded.updated_at`, manifest.Metadata.ID, manifest.Metadata.Version, options.EnableProject, stringJSON(granted), now)
		if err != nil {
			return PluginVersion{}, err
		}
	}
	for _, contribution := range manifest.Contributes.Pipelines {
		if err := s.pipelines.PublishDefinition(ctx, manifest.Metadata.ID, pkg.Files[contribution.Definition]); err != nil {
			s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "publish_pipeline", "failed", map[string]any{"pipeline_id": contribution.PipelineID, "error": err.Error()})
			return PluginVersion{}, fmt.Errorf("publish pipeline %s: %w", contribution.PipelineID, err)
		}
	}
	for _, contribution := range manifest.Contributes.Blueprints {
		if err := s.blueprints.PublishDefinition(ctx, manifest.Metadata.ID, pkg.Files[contribution.Definition]); err != nil {
			s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "publish_blueprint", "failed", map[string]any{"blueprint_id": contribution.BlueprintID, "error": err.Error()})
			return PluginVersion{}, fmt.Errorf("publish blueprint %s: %w", contribution.BlueprintID, err)
		}
	}
	s.audit(ctx, manifest.Metadata.ID, manifest.Metadata.Version, "install", "allowed", map[string]any{"content_hash": pkg.Hash, "signature_status": signatureStatus, "project_id": options.EnableProject})
	return s.Plugin(ctx, manifest.Metadata.ID, manifest.Metadata.Version)
}

func (s *Service) Plugin(ctx context.Context, pluginID, version string) (PluginVersion, error) {
	var item PluginVersion
	var signer sql.NullString
	var permissionsRaw, contributionsRaw, manifestRaw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT plugin_id,version,name,publisher,trust_class,signer_id,signature_status,content_hash,permissions_json,contributions_json,status,installed_at,updated_at,manifest_yaml,package_size FROM harness_plugin_versions WHERE plugin_id=? AND version=?`, pluginID, version).Scan(&item.PluginID, &item.Version, &item.Name, &item.Publisher, &item.TrustClass, &signer, &item.SignatureStatus, &item.ContentHash, &permissionsRaw, &contributionsRaw, &item.Status, &item.InstalledAt, &item.UpdatedAt, &manifestRaw, &item.PackageSizeBytes)
	if err != nil {
		return PluginVersion{}, err
	}
	item.SignerID = signer.String
	_ = json.Unmarshal([]byte(permissionsRaw), &item.Permissions)
	_ = json.Unmarshal([]byte(contributionsRaw), &item.Contributions)
	var manifest Manifest
	_ = yamlUnmarshalStrict([]byte(manifestRaw), &manifest)
	item.Manifest, _ = json.Marshal(manifest)
	rows, err := s.control.DB.QueryContext(ctx, `SELECT project_id,status,granted_capabilities_json,config_json,updated_at FROM harness_plugin_project_state WHERE plugin_id=? AND version=? ORDER BY project_id`, pluginID, version)
	if err != nil {
		return PluginVersion{}, err
	}
	for rows.Next() {
		var state ProjectState
		var capabilitiesRaw, configRaw string
		if err := rows.Scan(&state.ProjectID, &state.Status, &capabilitiesRaw, &configRaw, &state.UpdatedAt); err != nil {
			rows.Close()
			return PluginVersion{}, err
		}
		_ = json.Unmarshal([]byte(capabilitiesRaw), &state.GrantedCapabilities)
		state.Config = json.RawMessage(configRaw)
		item.ProjectStates = append(item.ProjectStates, state)
	}
	if err := rows.Close(); err != nil {
		return PluginVersion{}, err
	}
	if item.ProjectStates == nil {
		item.ProjectStates = []ProjectState{}
	}
	return item, nil
}

func (s *Service) List(ctx context.Context) ([]PluginVersion, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT plugin_id,version FROM harness_plugin_versions ORDER BY plugin_id,installed_at DESC`)
	if err != nil {
		return nil, err
	}
	type id struct{ plugin, version string }
	ids := []id{}
	for rows.Next() {
		var item id
		if err := rows.Scan(&item.plugin, &item.version); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]PluginVersion, 0, len(ids))
	for _, id := range ids {
		item, err := s.Plugin(ctx, id.plugin, id.version)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) SetProjectState(ctx context.Context, pluginID, version, projectID, status string, capabilities []string, config json.RawMessage) (PluginVersion, error) {
	if status != "enabled" && status != "disabled" {
		return PluginVersion{}, errors.New("plugin project status must be enabled or disabled")
	}
	granted, err := normalizeCapabilities(capabilities)
	if err != nil {
		return PluginVersion{}, err
	}
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	plugin, err := s.Plugin(ctx, pluginID, version)
	if err != nil {
		return PluginVersion{}, err
	}
	if status == "enabled" && (plugin.Status == "uninstalled" || plugin.Status == "quarantined") {
		return PluginVersion{}, fmt.Errorf("plugin version %s cannot be enabled while status=%s", version, plugin.Status)
	}
	allowed := map[string]bool{}
	for _, capability := range plugin.Permissions {
		allowed[capability] = true
	}
	for _, capability := range granted {
		if !allowed[capability] {
			return PluginVersion{}, fmt.Errorf("capability %q is not declared by this plugin", capability)
		}
	}
	var configValue any
	if err := decodeStrictJSON(config, &configValue); err != nil {
		return PluginVersion{}, fmt.Errorf("config: %w", err)
	}
	if schema, schemaErr := s.configurationSchema(ctx, plugin); schemaErr != nil {
		return PluginVersion{}, fmt.Errorf("config schema: %w", schemaErr)
	} else if len(schema) > 0 {
		canonical, validateErr := harness.ValidateAgainstSchema(schema, config)
		if validateErr != nil {
			return PluginVersion{}, fmt.Errorf("config: %w", validateErr)
		}
		config = canonical
	}
	now := nowUTC()
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_plugin_project_state(plugin_id,version,project_id,status,granted_capabilities_json,config_json,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(plugin_id,version,project_id) DO UPDATE SET status=excluded.status,granted_capabilities_json=excluded.granted_capabilities_json,config_json=excluded.config_json,updated_at=excluded.updated_at`, pluginID, version, projectID, status, stringJSON(granted), string(config), now)
	if err != nil {
		return PluginVersion{}, err
	}
	s.audit(ctx, pluginID, version, "project_state", "allowed", map[string]any{"project_id": projectID, "status": status, "capabilities": granted})
	return s.Plugin(ctx, pluginID, version)
}

func (s *Service) RegisterBuiltin(ctx context.Context, spec BuiltinSpec) (PluginVersion, error) {
	if !strings.HasPrefix(spec.PluginID, "builtin.") || !semverPattern.MatchString(spec.Version) || strings.TrimSpace(spec.Name) == "" {
		return PluginVersion{}, errors.New("built-in plugin requires builtin namespace, semantic version and name")
	}
	if spec.Status == "" {
		spec.Status = "enabled"
	}
	if spec.Status != "enabled" && spec.Status != "disabled" && spec.Status != "experimental" {
		return PluginVersion{}, errors.New("invalid built-in plugin status")
	}
	permissions, err := normalizeCapabilities(spec.Permissions)
	if err != nil {
		return PluginVersion{}, err
	}
	manifest := Manifest{
		APIVersion: APIVersion, Kind: "Plugin",
		Metadata:      Metadata{ID: spec.PluginID, Name: spec.Name, Version: spec.Version, Publisher: "memory-harness", License: "Bundled"},
		Compatibility: Compatibility{MemoryHarness: ">=2.0.0 <3.0.0"}, Trust: Trust{Class: "declarative"},
		Contributes: spec.Contributions, Permissions: Permissions{Required: permissions},
	}
	manifestRaw, _ := json.Marshal(manifest)
	contributionsRaw, _ := json.Marshal(spec.Contributions)
	contentHash := "builtin:" + hashText(string(manifestRaw))
	now := nowUTC()
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO harness_plugin_versions(plugin_id,version,name,publisher,trust_class,signature_status,content_hash,manifest_yaml,package_size,permissions_json,contributions_json,status,installed_at,updated_at) VALUES(?,?,?,'memory-harness','first_party','bundled',?,?,0,?,?,?, ?,?) ON CONFLICT(plugin_id,version) DO UPDATE SET name=excluded.name,permissions_json=excluded.permissions_json,contributions_json=excluded.contributions_json,status=CASE WHEN harness_plugin_versions.status='quarantined' THEN harness_plugin_versions.status ELSE excluded.status END,updated_at=excluded.updated_at`, spec.PluginID, spec.Version, spec.Name, contentHash, string(manifestRaw), stringJSON(permissions), string(contributionsRaw), spec.Status, now, now)
	if err != nil {
		return PluginVersion{}, err
	}
	return s.Plugin(ctx, spec.PluginID, spec.Version)
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Impact(ctx context.Context, pluginID, version, projectID string) (PluginImpact, error) {
	pluginID = strings.TrimSpace(pluginID)
	version = strings.TrimSpace(version)
	projectID = strings.TrimSpace(projectID)
	plugin, err := s.Plugin(ctx, pluginID, version)
	if err != nil {
		return PluginImpact{}, err
	}
	impact := PluginImpact{PluginID: pluginID, Version: version, ProjectID: projectID, PackageBytesReclaimed: plugin.PackageSizeBytes, HistoryPreserved: true, Blockers: []string{}}
	queries := []struct {
		target *int
		query  string
		args   []any
	}{
		{&impact.CurrentObjects, `SELECT count(*) FROM harness_objects o JOIN harness_object_revisions r ON r.object_id=o.object_id AND r.revision=o.current_revision WHERE r.plugin_id=? AND r.plugin_version=?`, []any{pluginID, version}},
		{&impact.HistoricalRevisions, `SELECT count(*) FROM harness_object_revisions WHERE plugin_id=? AND plugin_version=?`, []any{pluginID, version}},
		{&impact.HistoricalRuns, `SELECT count(DISTINCT run_id) FROM harness_spans WHERE plugin_id=?`, []any{pluginID}},
		{&impact.PipelineVersions, `SELECT count(*) FROM harness_pipeline_versions WHERE plugin_id=?`, []any{pluginID}},
		{&impact.BlueprintVersions, `SELECT count(*) FROM harness_blueprint_versions WHERE plugin_id=?`, []any{pluginID}},
		{&impact.EnabledProjects, `SELECT count(*) FROM harness_plugin_project_state WHERE plugin_id=? AND version=? AND status='enabled'`, []any{pluginID, version}},
	}
	for _, item := range queries {
		if err := s.control.DB.QueryRowContext(ctx, item.query, item.args...).Scan(item.target); err != nil {
			return PluginImpact{}, err
		}
	}
	needle := `%"plugin_id":"` + pluginID + `"%`
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_project_blueprints p JOIN harness_blueprint_versions b ON b.blueprint_id=p.blueprint_id AND b.version=p.blueprint_version WHERE p.status='active' AND b.definition_json LIKE ?`, needle).Scan(&impact.ActiveBlueprintRefs); err != nil {
		return PluginImpact{}, err
	}
	if strings.HasPrefix(pluginID, "builtin.") {
		impact.Blockers = append(impact.Blockers, "built-in plugins are part of the product baseline")
	}
	if plugin.Status == "uninstalled" {
		impact.Blockers = append(impact.Blockers, "plugin version is already retired")
	}
	if impact.EnabledProjects > 0 {
		impact.Blockers = append(impact.Blockers, fmt.Sprintf("%d project(s) still enable this plugin version", impact.EnabledProjects))
	}
	if impact.ActiveBlueprintRefs > 0 {
		impact.Blockers = append(impact.Blockers, fmt.Sprintf("%d active project blueprint(s) still reference this plugin", impact.ActiveBlueprintRefs))
	}
	impact.CanRetire = len(impact.Blockers) == 0
	return impact, nil
}

func (s *Service) Retire(ctx context.Context, pluginID, version string) (PluginVersion, error) {
	impact, err := s.Impact(ctx, pluginID, version, "")
	if err != nil {
		return PluginVersion{}, err
	}
	if !impact.CanRetire {
		return PluginVersion{}, fmt.Errorf("plugin cannot be retired: %s", strings.Join(impact.Blockers, "; "))
	}
	now := nowUTC()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return PluginVersion{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE harness_plugin_versions SET status='uninstalled',package_blob=NULL,package_size=0,updated_at=? WHERE plugin_id=? AND version=? AND status<>'uninstalled'`, now, pluginID, version)
	if err != nil {
		return PluginVersion{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return PluginVersion{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE harness_memory_types SET status='disabled',updated_at=? WHERE plugin_id=?`, now, pluginID); err != nil {
		return PluginVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return PluginVersion{}, err
	}
	s.audit(ctx, pluginID, version, "retire", "allowed", map[string]any{
		"history_preserved": true, "current_objects": impact.CurrentObjects, "historical_revisions": impact.HistoricalRevisions,
		"historical_runs": impact.HistoricalRuns, "package_bytes_reclaimed": impact.PackageBytesReclaimed,
	})
	return s.Plugin(ctx, pluginID, version)
}
