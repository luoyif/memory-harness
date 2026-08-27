package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestOwnerCancellationStopsActiveStageAndPreventsLaterNodes(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "cancel-active", Name: "Cancel Active", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	if err := a.Pipelines.RegisterStageHandler("transform.map", func(ctx context.Context, _ pipeline.StageInvocation) (json.RawMessage, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	definition := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.cancel.active
version: 1.0.0
name: Cancel active stage
intent: Verify that explicit cancellation reaches the active stage.
requiredCapabilities: []
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - {id: slow, stageType: transform.map, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [input], config: {}}
  - {id: after, stageType: transform.filter, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [slow], config: {field: value, equals: keep provenance}}
outputs: [{name: result, nodeId: after}]
policy: {maxStages: 4, timeoutSeconds: 30, maxModelCalls: 0}
`)
	published, err := a.Pipelines.Publish(t.Context(), "builtin.cancel", definition)
	if err != nil {
		t.Fatal(err)
	}
	type execution struct {
		result pipeline.ExecutionResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, err := a.Pipelines.Execute(context.Background(), pipeline.ExecuteInput{
			ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
			PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "cancel-active-run",
			Input: json.RawMessage(`{"value":"keep provenance"}`),
		})
		done <- execution{result: result, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("active stage did not start")
	}
	runs, err := a.Harness.ListRuns(t.Context(), project.ProjectID, "running", 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("running runs=%#v err=%v", runs, err)
	}
	if _, err := a.Pipelines.Cancel(t.Context(), runs[0].RunID, "owner-test", "test cancellation"); err != nil {
		t.Fatal(err)
	}
	select {
	case execution := <-done:
		if execution.result.Status != "cancelled" || !errors.Is(execution.err, context.Canceled) {
			t.Fatalf("execution=%#v err=%v", execution.result, execution.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop the active stage")
	}
	detail, err := a.Harness.RunDetail(t.Context(), runs[0].RunID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != "cancelled" || len(detail.Spans) != 2 || detail.Spans[1].Status != "cancelled" {
		t.Fatalf("cancelled trace=%#v", detail)
	}
	for _, span := range detail.Spans {
		if span.NodeID == "after" {
			t.Fatal("a node after cancellation was executed")
		}
	}
}

func TestDeclarativePipelinePublishesExecutesAndProducesInspectableTrace(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "pipeline-project", Name: "Pipeline Project", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Harness.RegisterType(t.Context(), harness.RegisterTypeInput{
		TypeID: "builtin.relationship.person-link", PluginID: "builtin.relationship", DisplayName: "Person Link", SchemaVersion: "1.0.0",
		Schema:          json.RawMessage(`{"type":"object","required":["person","relation"],"properties":{"person":{"type":"string"},"relation":{"type":"string"}},"additionalProperties":false}`),
		Lifecycle:       harness.Lifecycle{Initial: "candidate", States: []string{"candidate", "active"}, Transitions: map[string][]string{"candidate": {"active"}}},
		ProtectionClass: "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.relationship.capture
version: 1.0.0
name: Capture relationship
intent: Turn a reviewed relationship payload into a traceable object.
requiredCapabilities: [memory.materialize]
nodes:
  - id: input
    stageType: trigger.manual
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: []
    config: {}
  - id: normalize
    stageType: transform.map
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [input]
    config:
      merge:
        relation: collaborator
  - id: materialize
    stageType: object.materialize
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [normalize]
    config:
      type_id: builtin.relationship.person-link
      plugin_id: builtin.relationship
      plugin_version: 1.0.0
      confidence: 0.95
      importance: 0.8
outputs:
  - name: object
    nodeId: materialize
policy:
  maxStages: 8
  timeoutSeconds: 30
  maxModelCalls: 0
`)
	published, err := a.Pipelines.Publish(t.Context(), "builtin.relationship", definition)
	if err != nil {
		t.Fatal(err)
	}
	if published.ContentHash == "" || published.Definition.Nodes[1].StageType != "transform.map" {
		t.Fatalf("published=%#v", published)
	}
	if _, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "pipeline-run-denied",
		Input: json.RawMessage(`{"person":"Grace"}`),
	}); err == nil {
		t.Fatal("pipeline executed without its declared capability")
	}
	result, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "pipeline-run-1",
		Input: json.RawMessage(`{"person":"Grace"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.RunID == "" || len(result.Outputs["object"]) == 0 {
		t.Fatalf("result=%#v", result)
	}
	detail, err := a.Harness.RunDetail(t.Context(), result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != "completed" || len(detail.Spans) != 3 || len(detail.Events) != 10 {
		t.Fatalf("detail=%#v", detail)
	}
	objects, err := a.Harness.ListObjects(t.Context(), project.ProjectID, "builtin.relationship.person-link", "", 10)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects=%#v err=%v", objects, err)
	}
	var payload map[string]any
	_ = json.Unmarshal(objects[0].Revision.Payload, &payload)
	if payload["person"] != "Grace" || payload["relation"] != "collaborator" || objects[0].Revision.RunID != result.RunID {
		t.Fatalf("object=%#v", objects[0])
	}
	duplicate, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "pipeline-run-1",
		Input: json.RawMessage(`{"person":"Grace"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil || !duplicate.Duplicate || duplicate.RunID != result.RunID {
		t.Fatalf("duplicate=%#v err=%v", duplicate, err)
	}
}

func TestPipelineValidationRejectsCycleAndPausesAtReview(t *testing.T) {
	a, _ := testutil.Open(t)
	cycle := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.test.cycle
version: 1.0.0
name: Cycle
intent: Must fail.
requiredCapabilities: []
nodes:
  - {id: first, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [second], config: {}}
  - {id: second, stageType: transform.map, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [first], config: {}}
outputs: [{name: result, nodeId: second}]
policy: {maxStages: 4, timeoutSeconds: 10, maxModelCalls: 0}
`)
	if _, err := a.Pipelines.Publish(t.Context(), "builtin.test", cycle); err == nil {
		t.Fatal("cyclic pipeline was published")
	}
}

func TestPipelineCatalogReturnsOnlyLatestImmutableVersion(t *testing.T) {
	a, _ := testutil.Open(t)
	definition := func(version, name string) []byte {
		return []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.catalog.latest-test
version: ` + version + `
name: ` + name + `
intent: Verify current catalog selection while retaining exact immutable history.
requiredCapabilities: []
nodes:
  - id: input
    stageType: trigger.manual
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: []
    config: {}
outputs:
  - name: result
    nodeId: input
policy:
  maxStages: 2
  timeoutSeconds: 30
  maxModelCalls: 0
`)
	}
	if _, err := a.Pipelines.Publish(t.Context(), "builtin.catalog", definition("1.9.0", "Old catalog entry")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Pipelines.Publish(t.Context(), "builtin.catalog", definition("2.0.0", "Current catalog entry")); err != nil {
		t.Fatal(err)
	}
	items, err := a.Pipelines.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range items {
		if item.PipelineID == "builtin.catalog.latest-test" {
			found++
			if item.Version != "2.0.0" || item.Name != "Current catalog entry" {
				t.Fatalf("catalog returned stale version: %#v", item)
			}
		}
	}
	if found != 1 {
		t.Fatalf("catalog should contain one current version, found %d in %#v", found, items)
	}
	if old, err := a.Pipelines.Version(t.Context(), "builtin.catalog.latest-test", "1.9.0"); err != nil || old.Version != "1.9.0" {
		t.Fatalf("immutable history is no longer addressable: %#v err=%v", old, err)
	}
}

func TestPipelineDraftOptimisticRevisionValidationAndJSONPublish(t *testing.T) {
	a, _ := testutil.Open(t)
	definition := pipeline.Definition{
		APIVersion: pipeline.APIVersion, PipelineID: "builtin.user-workflows.my-flow", Version: "1.0.0", Name: "My flow", Intent: "Editable owner workflow.",
		Nodes:   []pipeline.Node{{ID: "input", StageType: "trigger.manual", StageVersion: "1.0.0", PluginID: harness.CorePluginID, DependsOn: []string{}, Config: json.RawMessage(`{}`)}},
		Outputs: []pipeline.Output{{Name: "result", NodeID: "input"}}, Policy: pipeline.ExecutionPolicy{MaxStages: 4, TimeoutSeconds: 30, MaxModelCalls: 0},
		Editor: pipeline.EditorMetadata{Positions: map[string]pipeline.NodePosition{"input": {X: 180, Y: 120}}},
	}
	draft, err := a.Pipelines.SaveDraft(t.Context(), pipeline.SaveDraftInput{PluginID: "builtin.user-workflows", Definition: definition})
	if err != nil || draft.Revision != 1 || draft.Definition.Editor.Positions["input"].X != 180 {
		t.Fatalf("draft=%#v err=%v", draft, err)
	}
	if _, err := a.Pipelines.SaveDraft(t.Context(), pipeline.SaveDraftInput{PluginID: "builtin.user-workflows", Definition: definition}); err == nil {
		t.Fatal("stale draft update was accepted")
	}
	definition.Intent = "Updated editable owner workflow."
	draft, err = a.Pipelines.SaveDraft(t.Context(), pipeline.SaveDraftInput{PluginID: "builtin.user-workflows", ExpectedRevision: 1, Definition: definition})
	if err != nil || draft.Revision != 2 {
		t.Fatalf("updated draft=%#v err=%v", draft, err)
	}
	validation, err := a.Pipelines.ValidateStructured("builtin.user-workflows", definition)
	if err != nil || !validation.Valid || len(validation.ExecutionOrder) != 1 || validation.ExecutionOrder[0] != "input" {
		t.Fatalf("validation=%#v err=%v", validation, err)
	}
	raw, _ := json.Marshal(definition)
	published, err := a.Pipelines.Publish(t.Context(), "builtin.user-workflows", raw)
	if err != nil || published.Definition.Editor.Positions["input"].Y != 120 {
		t.Fatalf("published=%#v err=%v", published, err)
	}
	if err := a.Pipelines.DeleteDraft(t.Context(), definition.PipelineID); err != nil {
		t.Fatal(err)
	}
	if drafts, err := a.Pipelines.ListDrafts(t.Context()); err != nil || len(drafts) != 0 {
		t.Fatalf("drafts=%#v err=%v", drafts, err)
	}
}

func TestReviewCheckpointResumesWithoutRepeatingCompletedStages(t *testing.T) {
	a, _ := testutil.Open(t)
	project, err := a.Portfolio.CreateProject(t.Context(), portfolio.ProjectInput{Slug: "review-resume", Name: "Review Resume", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Harness.RegisterType(t.Context(), harness.RegisterTypeInput{
		TypeID: "builtin.review.note", PluginID: "builtin.review", DisplayName: "Reviewed Note", SchemaVersion: "1.0.0",
		Schema:          json.RawMessage(`{"type":"object","required":["statement"],"properties":{"statement":{"type":"string"}},"additionalProperties":false}`),
		Lifecycle:       harness.Lifecycle{Initial: "active", States: []string{"active"}, Transitions: map[string][]string{}},
		ProtectionClass: "protected",
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.review.capture
version: 1.0.0
name: Reviewed capture
intent: Verify durable owner review resume.
requiredCapabilities: [memory.materialize]
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - {id: review, stageType: policy.require_review, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [input], config: {reason: protected_note}}
  - id: materialize
    stageType: object.materialize
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [review]
    config: {type_id: builtin.review.note, plugin_id: builtin.review, plugin_version: 1.0.0}
outputs: [{name: object, nodeId: materialize}]
policy: {maxStages: 8, timeoutSeconds: 30, maxModelCalls: 0}
`)
	published, err := a.Pipelines.Publish(t.Context(), "builtin.review", definition)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-a", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "review-approve",
		Input: json.RawMessage(`{"statement":"Owner checked this"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil || result.Status != "waiting_review" {
		t.Fatalf("waiting result=%#v err=%v", result, err)
	}
	reviews, err := a.Pipelines.ListReviews(t.Context(), project.ProjectID, "pending", 10)
	if err != nil || len(reviews) != 1 || reviews[0].Reason != "protected_note" {
		t.Fatalf("reviews=%#v err=%v", reviews, err)
	}
	if objects, _ := a.Harness.ListObjects(t.Context(), project.ProjectID, "builtin.review.note", "", 10); len(objects) != 0 {
		t.Fatalf("object materialized before approval: %#v", objects)
	}
	resumed, err := a.Pipelines.DecideReview(t.Context(), reviews[0].ReviewID, pipeline.ReviewDecisionInput{Decision: "approve", Note: "source verified", OwnerID: "owner-b"})
	if err != nil || resumed.Status != "completed" {
		t.Fatalf("resumed=%#v err=%v", resumed, err)
	}
	detail, err := a.Harness.RunDetail(t.Context(), result.RunID)
	if err != nil || detail.Run.Status != "completed" || len(detail.Spans) != 3 {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	inputAttempts := 0
	for _, span := range detail.Spans {
		if span.NodeID == "input" {
			inputAttempts++
		}
	}
	if inputAttempts != 1 {
		t.Fatalf("completed stage was repeated %d times", inputAttempts)
	}
	objects, err := a.Harness.ListObjects(t.Context(), project.ProjectID, "builtin.review.note", "", 10)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects=%#v err=%v", objects, err)
	}

	rejected, err := a.Pipelines.Execute(t.Context(), pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-a", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "review-reject",
		Input: json.RawMessage(`{"statement":"Do not keep this"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil || rejected.Status != "waiting_review" {
		t.Fatalf("rejected wait=%#v err=%v", rejected, err)
	}
	reviews, _ = a.Pipelines.ListReviews(t.Context(), project.ProjectID, "pending", 10)
	var rejectID string
	for _, item := range reviews {
		if item.RunID == rejected.RunID {
			rejectID = item.ReviewID
		}
	}
	decision, err := a.Pipelines.DecideReview(t.Context(), rejectID, pipeline.ReviewDecisionInput{Decision: "reject", Note: "not durable", OwnerID: "owner-b"})
	if err != nil || decision.Status != "denied" {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	objects, _ = a.Harness.ListObjects(t.Context(), project.ProjectID, "builtin.review.note", "", 10)
	if len(objects) != 1 {
		t.Fatalf("rejected run changed objects: %#v", objects)
	}
}

func TestForkFromStageReusesImmutablePrefixSnapshots(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "fork-stage", Name: "Fork Stage", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	definition := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.test.fork-stage
version: 1.0.0
name: Fork from stage
intent: Reuse immutable completed prefix outputs and rerun only the selected suffix.
requiredCapabilities: []
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - id: enrich
    stageType: transform.map
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [input]
    config: {merge: {phase: enriched}}
  - {id: after, stageType: transform.filter, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [enrich], config: {field: phase, equals: enriched}}
outputs: [{name: result, nodeId: after}]
policy: {maxStages: 4, timeoutSeconds: 30, maxModelCalls: 0}
`)
	published, err := a.Pipelines.Publish(ctx, "builtin.test", definition)
	if err != nil {
		t.Fatal(err)
	}
	original, err := a.Pipelines.Execute(ctx, pipeline.ExecuteInput{
		ProjectID: project.ProjectID, CallerType: "owner", CallerID: "owner-test", Channel: "desktop",
		PipelineID: published.PipelineID, PipelineVersion: published.Version, IdempotencyKey: "fork-original",
		Input: json.RawMessage(`{"name":"Memory Harness"}`),
	})
	if err != nil || original.Status != "completed" {
		t.Fatalf("original=%#v err=%v", original, err)
	}
	originalDetail, err := a.Harness.RunDetail(ctx, original.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(originalDetail.StageOutputs) != 3 {
		t.Fatalf("stage outputs=%#v", originalDetail.StageOutputs)
	}
	inputSnapshot, err := a.Harness.StageOutput(ctx, original.RunID, "input")
	if err != nil {
		t.Fatal(err)
	}

	forked, err := a.Pipelines.ForkFromNode(ctx, original.RunID, "enrich", "owner-test")
	if err != nil || forked.Status != "completed" {
		t.Fatalf("forked=%#v err=%v", forked, err)
	}
	if forked.RunID == original.RunID {
		t.Fatal("fork reused original run id")
	}
	forkDetail, err := a.Harness.RunDetail(ctx, forked.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if forkDetail.Run.ForkedFromRunID != original.RunID || len(forkDetail.Spans) != 2 || forkDetail.Spans[0].NodeID != "enrich" || forkDetail.Spans[1].NodeID != "after" {
		t.Fatalf("fork detail=%#v", forkDetail)
	}
	if len(forkDetail.StageOutputs) != 2 {
		t.Fatalf("fork outputs=%#v", forkDetail.StageOutputs)
	}
	var final map[string]any
	if err := json.Unmarshal(forked.Outputs["result"], &final); err != nil {
		t.Fatal(err)
	}
	if final["name"] != "Memory Harness" || final["phase"] != "enriched" {
		t.Fatalf("fork result=%#v", final)
	}
	originalInputAgain, err := a.Harness.StageOutput(ctx, original.RunID, "input")
	if err != nil || originalInputAgain.OutputHash != inputSnapshot.OutputHash {
		t.Fatalf("prefix snapshot mutated: %#v err=%v", originalInputAgain, err)
	}

	if _, err := a.Control.DB.ExecContext(ctx, `DELETE FROM harness_stage_outputs WHERE run_id=? AND node_id='input'`, original.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Pipelines.ForkFromNode(ctx, original.RunID, "enrich", "owner-test"); err == nil || !strings.Contains(err.Error(), "predates durable stage snapshots") {
		t.Fatalf("fork without prefix snapshot should fail closed: %v", err)
	}
}

func TestDryRunPreviewsPureStagesAndPerformsNoWrites(t *testing.T) {
	a, _ := testutil.Open(t)
	ctx := t.Context()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "dry-run", Name: "Dry Run", DefaultCurrency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Harness.RegisterType(ctx, harness.RegisterTypeInput{
		TypeID: "builtin.dry-run.item", PluginID: "builtin.dry-run", DisplayName: "Dry item", SchemaVersion: "1.0.0",
		Schema:          json.RawMessage(`{"type":"object","required":["name","phase"],"properties":{"name":{"type":"string"},"phase":{"type":"string"}},"additionalProperties":false}`),
		Lifecycle:       harness.Lifecycle{Initial: "candidate", States: []string{"candidate", "active"}, Transitions: map[string][]string{"candidate": {"active"}}},
		ProtectionClass: "standard",
	}); err != nil {
		t.Fatal(err)
	}
	definition := []byte(`apiVersion: memory-harness.pipeline/v1alpha1
pipelineId: builtin.dry-run.preview
version: 1.0.0
name: Dry run preview
intent: Preview pure transforms and validate a future object write without committing it.
requiredCapabilities: [memory.materialize]
nodes:
  - {id: input, stageType: trigger.manual, stageVersion: 1.0.0, pluginId: builtin.memory-harness-core, dependsOn: [], config: {}}
  - id: enrich
    stageType: transform.map
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [input]
    config: {merge: {phase: previewed}}
  - id: materialize
    stageType: object.materialize
    stageVersion: 1.0.0
    pluginId: builtin.memory-harness-core
    dependsOn: [enrich]
    config: {type_id: builtin.dry-run.item, plugin_id: builtin.dry-run, plugin_version: 1.0.0}
outputs: [{name: result, nodeId: materialize}]
policy: {maxStages: 4, timeoutSeconds: 30, maxModelCalls: 0}
`)
	published, err := a.Pipelines.Publish(ctx, "builtin.dry-run", definition)
	if err != nil {
		t.Fatal(err)
	}
	var runsBefore, objectsBefore int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_runs`).Scan(&runsBefore); err != nil {
		t.Fatal(err)
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_objects`).Scan(&objectsBefore); err != nil {
		t.Fatal(err)
	}
	preview, err := a.Pipelines.DryRun(ctx, pipeline.ExecuteInput{
		ProjectID: project.ProjectID, PipelineID: published.PipelineID, PipelineVersion: published.Version,
		Input: json.RawMessage(`{"name":"preview item"}`), EffectiveCapabilities: []string{"memory.materialize"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NoWritesPerformed || preview.PurePreviewed != 2 || preview.PlannedWrites != 1 || len(preview.Nodes) != 3 {
		t.Fatalf("preview=%#v", preview)
	}
	if preview.Nodes[1].Status != "previewed" || preview.Nodes[2].Status != "would_materialize" {
		t.Fatalf("nodes=%#v", preview.Nodes)
	}
	var enriched map[string]any
	if err := json.Unmarshal(preview.Nodes[1].Preview, &enriched); err != nil {
		t.Fatal(err)
	}
	if enriched["phase"] != "previewed" {
		t.Fatalf("enriched=%#v", enriched)
	}
	var runsAfter, objectsAfter int
	_ = a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_runs`).Scan(&runsAfter)
	_ = a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM harness_objects`).Scan(&objectsAfter)
	if runsAfter != runsBefore || objectsAfter != objectsBefore {
		t.Fatalf("dry run wrote state: runs %d->%d objects %d->%d", runsBefore, runsAfter, objectsBefore, objectsAfter)
	}
}
