package harness_test

import (
	"encoding/json"
	"testing"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func registerRelationshipType(t *testing.T, a harnessFixture) harness.MemoryType {
	t.Helper()
	item, err := a.service.RegisterType(t.Context(), harness.RegisterTypeInput{
		TypeID:        "memory.relationship",
		PluginID:      "builtin.test-plugin",
		DisplayName:   "Relationship",
		SchemaVersion: "1.0.0",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["person","relation"],
			"properties":{
				"person":{"type":"string","minLength":1,"maxLength":120},
				"relation":{"type":"string","enum":["colleague","friend","family"]},
				"note":{"type":"string","maxLength":1000}
			},
			"additionalProperties":false
		}`),
		Lifecycle: harness.Lifecycle{
			Initial: "candidate",
			States:  []string{"candidate", "active", "superseded"},
			Transitions: map[string][]string{
				"candidate": {"active"},
				"active":    {"superseded"},
			},
		},
		ProtectionClass: "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

type harnessFixture struct {
	service *harness.Service
	project string
}

func newFixture(t *testing.T) harnessFixture {
	t.Helper()
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{
		Slug: "harness-test", Name: "Harness Test", DefaultCurrency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	return harnessFixture{service: a.Harness, project: project.ProjectID}
}

func TestGenericMemoryTypeValidationLifecycleAndIdempotentMaterialization(t *testing.T) {
	fixture := newFixture(t)
	typeDef := registerRelationshipType(t, fixture)
	if typeDef.PluginID != "builtin.test-plugin" || typeDef.Lifecycle.Initial != "candidate" {
		t.Fatalf("type=%#v", typeDef)
	}
	listed, err := fixture.service.ListTypes(t.Context())
	found := false
	for _, item := range listed {
		found = found || item.TypeID == "memory.relationship"
	}
	if err != nil || len(listed) < 10 || !found {
		t.Fatalf("types=%#v err=%v", listed, err)
	}

	_, err = fixture.service.Materialize(t.Context(), harness.MaterializeInput{
		TypeID: "memory.relationship", ProjectID: fixture.project,
		Payload:  json.RawMessage(`{"person":"Lin","relation":"unknown"}`),
		PluginID: "builtin.test-plugin", PluginVersion: "1.0.0", IdempotencyKey: "bad-payload",
	})
	if err == nil {
		t.Fatal("invalid enum payload was accepted")
	}

	created, err := fixture.service.Materialize(t.Context(), harness.MaterializeInput{
		TypeID: "memory.relationship", ProjectID: fixture.project,
		Payload:    json.RawMessage(`{"person":"Lin","relation":"colleague"}`),
		Confidence: .9, Importance: .7,
		SourceEvidenceIDs: []string{"ev-2", "ev-1", "ev-1"},
		PluginID:          "builtin.test-plugin", PluginVersion: "1.0.0", IdempotencyKey: "relationship-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "candidate" || created.CurrentRevision != 1 || created.Revision.ContentHash == "" {
		t.Fatalf("created=%#v", created)
	}
	if len(created.Revision.SourceEvidenceIDs) != 2 || created.Revision.SourceEvidenceIDs[0] != "ev-1" {
		t.Fatalf("sources=%#v", created.Revision.SourceEvidenceIDs)
	}

	duplicate, err := fixture.service.Materialize(t.Context(), harness.MaterializeInput{
		TypeID: "memory.relationship", ProjectID: fixture.project,
		Payload:  json.RawMessage(`{"person":"Lin","relation":"colleague"}`),
		PluginID: "builtin.test-plugin", PluginVersion: "1.0.0", IdempotencyKey: "relationship-1",
	})
	if err != nil || !duplicate.Duplicate || duplicate.ObjectID != created.ObjectID || duplicate.CurrentRevision != 1 {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}

	updated, err := fixture.service.Materialize(t.Context(), harness.MaterializeInput{
		ObjectID: created.ObjectID, TypeID: "memory.relationship", ProjectID: fixture.project, Status: "active",
		Payload:    json.RawMessage(`{"person":"Lin","relation":"friend","note":"confirmed by owner"}`),
		Confidence: .95, Importance: .8,
		SourceObjectIDs: []string{created.ObjectID},
		PluginID:        "builtin.test-plugin", PluginVersion: "1.1.0", IdempotencyKey: "relationship-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "active" || updated.CurrentRevision != 2 || updated.Revision.Status != "active" {
		t.Fatalf("updated=%#v", updated)
	}

	_, err = fixture.service.Materialize(t.Context(), harness.MaterializeInput{
		ObjectID: created.ObjectID, TypeID: "memory.relationship", ProjectID: fixture.project, Status: "candidate",
		Payload:  json.RawMessage(`{"person":"Lin","relation":"friend"}`),
		PluginID: "builtin.test-plugin", PluginVersion: "1.1.0", IdempotencyKey: "relationship-invalid-transition",
	})
	if err == nil {
		t.Fatal("invalid lifecycle transition was accepted")
	}
}

func TestRunTraceSpansAndEffectReconciliation(t *testing.T) {
	fixture := newFixture(t)
	run, err := fixture.service.StartRun(t.Context(), harness.StartRunInput{
		ProjectID: fixture.project, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: "pipeline.capture", PipelineVersion: "1.0.0", PipelineHash: "sha256:pipeline-v1",
		IdempotencyKey: "run-1", Snapshot: json.RawMessage(`{"model":"rules","plugins":["builtin.test-plugin@1.0.0"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := fixture.service.StartRun(t.Context(), harness.StartRunInput{
		ProjectID: fixture.project, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: "pipeline.capture", PipelineVersion: "1.0.0", PipelineHash: "sha256:pipeline-v1",
		IdempotencyKey: "run-1", Snapshot: json.RawMessage(`{}`),
	})
	if err != nil || !duplicate.Duplicate || duplicate.RunID != run.RunID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}
	if _, err := fixture.service.AppendEvent(t.Context(), run.RunID, "run.started", harness.CorePluginID, map[string]any{"reason": "test"}); err != nil {
		t.Fatal(err)
	}
	span, err := fixture.service.StartSpan(t.Context(), run.RunID, "", "extract", "extract.candidates", "1.0.0", "builtin.test-plugin", "sha256:input", map[string]any{"items": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.FinishSpan(t.Context(), span.SpanID, "completed", "sha256:output", map[string]any{"items": 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.service.RecordEffectIntent(t.Context(), run.RunID, "publish", "external-1", "sha256:request-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.MarkEffectDispatched(t.Context(), run.RunID, "publish", "external-1", "provider-key-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.RecordEffectReceipt(t.Context(), run.RunID, "publish", "external-1", "unknown", "", map[string]any{"timeout": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.MarkEffectMaterialized(t.Context(), run.RunID, "publish", "external-1"); err == nil {
		t.Fatal("unknown external outcome was materialized")
	}

	if _, err := fixture.service.RecordEffectIntent(t.Context(), run.RunID, "publish", "external-2", "sha256:request-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.MarkEffectDispatched(t.Context(), run.RunID, "publish", "external-2", "provider-key-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.RecordEffectReceipt(t.Context(), run.RunID, "publish", "external-2", "confirmed", "sha256:result-2", map[string]any{"remote_id": "remote-2"}); err != nil {
		t.Fatal(err)
	}
	materialized, err := fixture.service.MarkEffectMaterialized(t.Context(), run.RunID, "publish", "external-2")
	if err != nil || materialized.Status != "materialized" {
		t.Fatalf("effect=%#v err=%v", materialized, err)
	}
	if _, err := fixture.service.AppendEvent(t.Context(), run.RunID, "run.completed_with_warnings", harness.CorePluginID, map[string]any{"unknown_effects": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AppendEvent(t.Context(), run.RunID, "late.event", harness.CorePluginID, map[string]any{}); err == nil {
		t.Fatal("terminal run accepted a late event")
	}

	detail, err := fixture.service.RunDetail(t.Context(), run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != "completed_with_warnings" || len(detail.Spans) != 1 || len(detail.Effects) != 2 || len(detail.Events) < 10 {
		t.Fatalf("detail=%#v", detail)
	}
	for index, event := range detail.Events {
		if event.Sequence != index+1 || event.SchemaVersion != harness.TraceSchemaVersion {
			t.Fatalf("event[%d]=%#v", index, event)
		}
	}
}
