package plugins_test

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/plugins"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func packageDigest(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "SIGNATURE" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		digest.Write([]byte(name))
		digest.Write([]byte{0})
		digest.Write(files[name])
		digest.Write([]byte{0})
	}
	return digest.Sum(nil)
}

func buildPackage(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o600)
		header.SetModTime(time.Unix(0, 0).UTC())
		entry, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func relationshipFiles() map[string][]byte {
	return map[string][]byte{
		"manifest.yaml": []byte(`apiVersion: memory-harness.plugin/v1alpha1
kind: Plugin
metadata:
  id: com.example.relationship
  name: Relationship Memory
  version: 1.0.0
  publisher: com.example
  license: Apache-2.0
compatibility:
  memoryHarness: ">=2.0.0 <3.0.0"
trust:
  class: declarative
contributes:
  memoryTypes:
    - typeId: com.example.relationship.person-link
      displayName: Person Link
      schemaVersion: 1.0.0
      schema: schemas/memory-types/person-link.schema.json
      lifecycle:
        initial: candidate
        states: [candidate, active, superseded]
        transitions:
          candidate: [active]
          active: [superseded]
      protectionClass: private
      renderer: ui/person-link.renderer.json
  stages: []
  pipelines:
    - pipelineId: com.example.relationship.capture
      version: 1.0.0
      definition: pipelines/capture.pipeline.yaml
  strategyComponents:
    - {componentId: com.example.relationship.growth, version: 1.0.0, displayName: Relationship Growth, role: growth.relationship, kind: memory_type, capabilities: [memory.materialize]}
    - {componentId: com.example.relationship.organize, version: 1.0.0, displayName: Relationship Organization, role: organization.relationship, kind: provider, capabilities: [memory.materialize]}
    - {componentId: com.example.relationship.recall, version: 1.0.0, displayName: Relationship Recall, role: recall.relationship, kind: stage, stageType: memory.retrieve, capabilities: [memory.materialize]}
  blueprints:
    - {blueprintId: com.example.relationship.default, version: 1.0.0, definition: blueprints/default.blueprint.yaml}
  connectors: []
  projections: []
  views: []
permissions:
  required: [memory.materialize]
  optional: [memory.read]
configuration:
  schema: ""
`),
		"schemas/memory-types/person-link.schema.json": []byte(`{"type":"object","required":["person","relation"],"properties":{"person":{"type":"string"},"relation":{"type":"string"}},"additionalProperties":false}`),
		"ui/person-link.renderer.json":                 []byte(`{"title_field":"person","subtitle_field":"relation"}`),
		"pipelines/capture.pipeline.yaml": []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: com.example.relationship.capture
version: 1.0.0
name: Capture relationship
intent: Materialize a custom relationship object.
requiredCapabilities: [memory.materialize]
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - id: materialize
    stageType: object.materialize
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [input]
    config:
      type_id: com.example.relationship.person-link
      plugin_id: com.example.relationship
      plugin_version: 1.0.0
      confidence: 0.9
      importance: 0.7
outputs: [{name: object, nodeId: materialize}]
policy: {maxStages: 4, timeoutSeconds: 30, maxModelCalls: 0}
`),
		"blueprints/default.blueprint.yaml": []byte(`apiVersion: memory-harness.blueprint/v1alpha1
blueprintId: com.example.relationship.default
version: 1.0.0
name: Relationship Blueprint
description: A plugin-contributed project memory strategy.
intent: Preserve Evidence and organize relationship memory.
policy: {evidenceMode: normalized_with_verbatim, modelBoundary: rules_only, defaultContextBudget: 4096, crossProjectRecall: false}
tracks:
  - trackId: growth
    role: growth
    displayName: Growth
    nodes:
      - {nodeId: relation, role: growth.relationship, displayName: Relationship, bindingKind: memory_type, pluginId: com.example.relationship, pluginVersion: 1.0.0, componentId: com.example.relationship.growth, componentVersion: 1.0.0, enabled: true, requiredCapabilities: [memory.materialize], config: {review: true}}
  - trackId: organization
    role: organization
    displayName: Organization
    nodes:
      - {nodeId: relation-scope, role: organization.relationship, displayName: Relationship Scope, bindingKind: provider, pluginId: com.example.relationship, pluginVersion: 1.0.0, componentId: com.example.relationship.organize, componentVersion: 1.0.0, enabled: true, requiredCapabilities: [memory.materialize], config: {scope: project}}
  - trackId: recall
    role: recall
    displayName: Recall
    nodes:
      - {nodeId: relation-recall, role: recall.relationship, displayName: Relationship Recall, bindingKind: stage, pluginId: com.example.relationship, pluginVersion: 1.0.0, componentId: com.example.relationship.recall, componentVersion: 1.0.0, enabled: true, requiredCapabilities: [memory.materialize], config: {limit: 10}}
`),
	}
}

func TestSignedDeclarativePluginAddsMemoryTypeWithoutKernelChange(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "plugin-project", Name: "Plugin Project", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	files := relationshipFiles()
	signatureRaw, _ := json.Marshal(plugins.Signature{
		SignerID: "com.example.signer", Algorithm: "ed25519",
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, packageDigest(files))),
	})
	files["SIGNATURE"] = signatureRaw
	raw := buildPackage(t, files)

	if _, err := a.Plugins.Install(t.Context(), raw, plugins.InstallOptions{EnableProject: project.ProjectID, Capabilities: []string{"memory.materialize"}}); err == nil {
		t.Fatal("untrusted signed plugin was installed")
	}
	signer, err := a.Plugins.ApproveSigner(t.Context(), plugins.TrustSignerInput{
		SignerID: "com.example.signer", Publisher: "com.example",
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicKey), Scope: []string{"com.example.*"},
	})
	if err != nil || signer.Fingerprint == "" {
		t.Fatalf("signer=%#v err=%v", signer, err)
	}
	installed, err := a.Plugins.Install(t.Context(), raw, plugins.InstallOptions{EnableProject: project.ProjectID, Capabilities: []string{"memory.materialize"}})
	if err != nil {
		t.Fatal(err)
	}
	if installed.SignatureStatus != "verified" || installed.Status != "enabled" || len(installed.ProjectStates) != 1 {
		t.Fatalf("installed=%#v", installed)
	}
	if len(installed.Contributions.StrategyComponents) != 3 || len(installed.Contributions.Blueprints) != 1 {
		t.Fatalf("strategy contributions=%#v", installed.Contributions)
	}
	active, err := a.Blueprints.Activate(t.Context(), project.ProjectID, "com.example.relationship.default", "1.0.0", "plugin-test")
	if err != nil || !active.Validation.Valid || active.Blueprint.PluginID != "com.example.relationship" {
		t.Fatalf("plugin blueprint=%#v err=%v", active, err)
	}
	if _, err := a.Harness.Type(t.Context(), "com.example.relationship.person-link"); err != nil {
		t.Fatal(err)
	}
	pipelineResult, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "plugin-test", Channel: "desktop",
		PipelineID: "com.example.relationship.capture", PipelineVersion: "1.0.0", IdempotencyKey: "plugin-pipeline-1",
		Input: json.RawMessage(`{"person":"Ada","relation":"collaborator"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil || pipelineResult.Status != "completed" {
		t.Fatalf("pipeline=%#v err=%v", pipelineResult, err)
	}
	object, err := a.Harness.Materialize(t.Context(), harness.MaterializeInput{
		TypeID: "com.example.relationship.person-link", ProjectID: project.ProjectID,
		Payload:  json.RawMessage(`{"person":"Ada","relation":"collaborator"}`),
		PluginID: "com.example.relationship", PluginVersion: "1.0.0", IdempotencyKey: "plugin-object-direct-1",
	})
	if err != nil || object.TypeID != "com.example.relationship.person-link" {
		t.Fatalf("object=%#v err=%v", object, err)
	}
	if err := a.Plugins.RevokeSigner(t.Context(), "com.example.signer"); err != nil {
		t.Fatal(err)
	}
	quarantined, err := a.Plugins.Plugin(t.Context(), installed.PluginID, installed.Version)
	if err != nil || quarantined.Status != "quarantined" {
		t.Fatalf("quarantined=%#v err=%v", quarantined, err)
	}
}

func TestPluginPackageRejectsTraversalAndUnsignedProductionInstall(t *testing.T) {
	a, _ := testutil.Open(t)
	files := relationshipFiles()
	if _, err := a.Plugins.Install(t.Context(), buildPackage(t, files), plugins.InstallOptions{DeveloperMode: false}); err == nil {
		t.Fatal("unsigned production plugin was accepted")
	}
	malicious := map[string][]byte{"manifest.yaml": files["manifest.yaml"], "../escape": []byte("bad")}
	if _, err := a.Plugins.Install(t.Context(), buildPackage(t, malicious), plugins.InstallOptions{DeveloperMode: true}); err == nil {
		t.Fatal("path traversal package was accepted")
	}
}

func TestProjectStateOnlyGrantsDeclaredCapabilitiesAndDoesNotChangeGlobalStatus(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "plugin-policy", Name: "Plugin Policy", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := a.Plugins.RegisterBuiltin(t.Context(), plugins.BuiltinSpec{
		PluginID: "builtin.policy-test", Version: "1.0.0", Name: "Policy Test",
		Permissions: []string{"memory.read", "memory.materialize"}, Status: "enabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plugins.SetProjectState(t.Context(), registered.PluginID, registered.Version, project.ProjectID, "enabled", []string{"owner.admin"}, json.RawMessage(`{}`)); err == nil {
		t.Fatal("undeclared project capability was granted")
	}
	updated, err := a.Plugins.SetProjectState(t.Context(), registered.PluginID, registered.Version, project.ProjectID, "disabled", []string{"memory.read"}, json.RawMessage(`{"mode":"manual"}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "enabled" || len(updated.ProjectStates) != 1 || updated.ProjectStates[0].Status != "disabled" {
		t.Fatalf("project state changed global status or was not persisted: %#v", updated)
	}
	if len(updated.ProjectStates[0].GrantedCapabilities) != 1 || updated.ProjectStates[0].GrantedCapabilities[0] != "memory.read" {
		t.Fatalf("unexpected grants: %#v", updated.ProjectStates[0])
	}
}

func TestPluginImpactBlocksUnsafeRetireAndRetirePreservesHistory(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "plugin-retire", Name: "Plugin Retire", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	raw := buildPackage(t, relationshipFiles())
	installed, err := a.Plugins.Install(ctx, raw, plugins.InstallOptions{DeveloperMode: true, EnableProject: project.ProjectID, Capabilities: []string{"memory.materialize"}})
	if err != nil {
		t.Fatal(err)
	}
	object, err := a.Harness.Materialize(ctx, harness.MaterializeInput{
		TypeID: "com.example.relationship.person-link", ProjectID: project.ProjectID,
		Payload:  json.RawMessage(`{"person":"Grace","relation":"collaborator"}`),
		PluginID: installed.PluginID, PluginVersion: installed.Version, IdempotencyKey: "retire-history-object",
	})
	if err != nil {
		t.Fatal(err)
	}
	impact, err := a.Plugins.Impact(ctx, installed.PluginID, installed.Version, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.CanRetire || impact.EnabledProjects != 1 || impact.CurrentObjects != 1 || len(impact.Blockers) == 0 {
		t.Fatalf("unsafe retire impact=%#v", impact)
	}
	if _, err := a.Plugins.Retire(ctx, installed.PluginID, installed.Version); err == nil {
		t.Fatal("enabled plugin was retired")
	}
	if _, err := a.Plugins.SetProjectState(ctx, installed.PluginID, installed.Version, project.ProjectID, "disabled", []string{"memory.materialize"}, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	impact, err = a.Plugins.Impact(ctx, installed.PluginID, installed.Version, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !impact.CanRetire || impact.EnabledProjects != 0 || impact.HistoricalRevisions < 1 || impact.PackageBytesReclaimed <= 0 || !impact.HistoryPreserved {
		t.Fatalf("safe retire impact=%#v", impact)
	}
	retired, err := a.Plugins.Retire(ctx, installed.PluginID, installed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if retired.Status != "uninstalled" || retired.PackageSizeBytes != 0 {
		t.Fatalf("retired=%#v", retired)
	}
	retiredType, err := a.Harness.Type(ctx, "com.example.relationship.person-link")
	if err != nil {
		t.Fatal(err)
	}
	if retiredType.Status != "disabled" {
		t.Fatalf("retired type=%#v", retiredType)
	}
	if _, err := a.Harness.Materialize(ctx, harness.MaterializeInput{TypeID: retiredType.TypeID, ProjectID: project.ProjectID, Payload: json.RawMessage(`{"person":"Lin","relation":"collaborator"}`), PluginID: installed.PluginID, PluginVersion: installed.Version, IdempotencyKey: "retired-materialize"}); err == nil {
		t.Fatal("retired plugin memory type remained executable")
	}
	if _, err := a.Harness.Object(ctx, object.ObjectID); err != nil {
		t.Fatalf("historical object became unreadable: %v", err)
	}
	if _, err := a.Plugins.SetProjectState(ctx, installed.PluginID, installed.Version, project.ProjectID, "enabled", []string{"memory.materialize"}, json.RawMessage(`{}`)); err == nil {
		t.Fatal("retired plugin could be re-enabled without reinstall")
	}
	reinstalled, err := a.Plugins.Install(ctx, raw, plugins.InstallOptions{DeveloperMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if reinstalled.Status != "installed" || reinstalled.PackageSizeBytes <= 0 || reinstalled.ContentHash != installed.ContentHash {
		t.Fatalf("reinstall=%#v", reinstalled)
	}
	reinstalledType, err := a.Harness.Type(ctx, "com.example.relationship.person-link")
	if err != nil || reinstalledType.Status != "enabled" {
		t.Fatalf("reinstalled type=%#v err=%v", reinstalledType, err)
	}
	if _, err := a.Plugins.SetProjectState(ctx, installed.PluginID, installed.Version, project.ProjectID, "enabled", []string{"memory.materialize"}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("reinstalled plugin could not be enabled: %v", err)
	}
	if _, err := a.Harness.Object(ctx, object.ObjectID); err != nil {
		t.Fatalf("historical object was not preserved across reinstall: %v", err)
	}
	builtinImpact, err := a.Plugins.Impact(ctx, "builtin.core-memory-growth", "2.0.0", project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if builtinImpact.CanRetire {
		t.Fatal("built-in plugin was marked retireable")
	}
}

func TestPluginConformanceAndSchemaDrivenProjectConfig(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "plugin-conformance", Name: "Plugin Conformance", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	files := relationshipFiles()
	manifest := string(files["manifest.yaml"])
	manifest = strings.Replace(manifest, "configuration:\n  schema: \"\"", "configuration:\n  schema: schemas/config.schema.json", 1)
	files["manifest.yaml"] = []byte(manifest)
	files["schemas/config.schema.json"] = []byte(`{"type":"object","required":["mode"],"properties":{"mode":{"type":"string","enum":["safe","strict"]},"limit":{"type":"integer","minimum":1,"maximum":20},"enabled":{"type":"boolean"}},"additionalProperties":false}`)
	raw := buildPackage(t, files)
	installed, err := a.Plugins.Install(ctx, raw, plugins.InstallOptions{DeveloperMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plugins.SetProjectState(ctx, installed.PluginID, installed.Version, project.ProjectID, "enabled", []string{"memory.materialize"}, json.RawMessage(`{"mode":"unsafe"}`)); err == nil {
		t.Fatal("invalid schema config was accepted")
	}
	updated, err := a.Plugins.SetProjectState(ctx, installed.PluginID, installed.Version, project.ProjectID, "enabled", []string{"memory.materialize"}, json.RawMessage(`{"mode":"safe","limit":5,"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.ProjectStates) != 1 {
		t.Fatalf("state=%#v", updated.ProjectStates)
	}
	report, err := a.Plugins.Conformance(ctx, installed.PluginID, installed.Version, project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if report.CompatibilityStatus != "compatible" || report.ConfigurationStatus != "valid" {
		t.Fatalf("report=%#v", report)
	}
	if len(report.MissingRequired) != 0 || len(report.OptionalNotGranted) != 1 || report.OptionalNotGranted[0] != "memory.read" {
		t.Fatalf("permission diff=%#v", report)
	}
	if len(report.ConfigurationSchema) == 0 {
		t.Fatal("configuration schema missing from conformance")
	}
	foundNoEffects := false
	for _, check := range report.Checks {
		if check.Name == "external_effect_probe" && check.Status == "not_executed" {
			foundNoEffects = true
		}
	}
	if !foundNoEffects {
		t.Fatalf("checks=%#v", report.Checks)
	}
}
