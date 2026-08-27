# Memory Harness Run and Trace Contract

Status: proposed contract, schema version `memory-harness.trace/v1alpha1`

## Purpose

The trace contract records what happened during memory processing without
confusing logs, current state and canonical memory. It supports inspection,
audit, pause/resume, retry, replay, comparison and external-runtime correlation.

Trace records describe execution. They never become memory solely because they
exist.

## Concepts

### Run

A run is one version-pinned execution of a pipeline for a project and caller.
It owns immutable start context plus a mutable status projection derived from
append-only events.

Statuses:

```text
queued | running | waiting_review | paused |
completed | completed_with_warnings | failed | denied | cancelled
```

Terminal statuses never transition back to running. Retry or replay creates a
new run with `retry_of_run_id` or `forked_from_run_id`.

### Span

A span is one stage attempt. It has a parent run/span, typed inputs and outputs,
start/end time, status, execution metadata and ordered events. Retrying a stage
creates a new attempt span.

### Event

An event is an append-only occurrence such as stage start, model request,
permission check, artifact creation, review request, review decision, retry,
error or run completion.

### Artifact

An artifact is a bounded output too large or structured to keep inline. The
trace stores its media type, size, hash, retention class, redaction state and
authorized opaque reference.

### Object link

An object link points to Evidence, a Memory Object, Living Asset, project record
or external object with relation such as input, output, proposal, supersedes or
conflicts_with.

## Run snapshot

The immutable start snapshot includes:

- run and correlation IDs;
- project, caller Agent and initiating channel;
- pipeline ID/version/content hash;
- all plugin IDs/versions/manifest hashes;
- referenced prompt and model profile versions;
- trigger type and idempotency key;
- requested and effective capabilities;
- input object references and hashes;
- privacy, payload-retention and review policies;
- limits for time, concurrency, model calls, tokens and money;
- parent/retry/fork relationships;
- application and trace schema versions.

The current run projection includes status, active spans, progress, warnings,
failure summary, cost, outputs and pending review. It can be rebuilt from the
snapshot and events.

## Required span fields

- `span_id`, `run_id`, optional `parent_span_id`;
- node ID, stage type, stage version and attempt number;
- execution class and responsible plugin;
- start, end and duration;
- status and bounded error classification;
- input/output schemas, hashes and object/artifact links;
- permission decision and policy version;
- model/provider/prompt/tool metadata where applicable;
- token, cost and retry accounting;
- redaction and payload-retention state.

## Standard event types

- `run.queued`, `run.started`, `run.paused`, `run.resumed`;
- `run.waiting_review`, `run.completed`, `run.failed`, `run.denied`,
  `run.cancelled`;
- `stage.scheduled`, `stage.started`, `stage.progress`, `stage.completed`,
  `stage.failed`, `stage.cancelled`, `stage.retry_scheduled`;
- `permission.allowed`, `permission.denied`;
- `model.requested`, `model.completed`, `model.failed`, `model.fallback`;
- `tool.requested`, `tool.completed`, `tool.failed`;
- `object.proposed`, `object.materialized`, `object.rejected`;
- `artifact.created`, `artifact.redacted`, `artifact.expired`;
- `review.requested`, `review.decided`, `review.expired`;
- `external.linked`, `external.event_ingested`;
- `effect.intent_recorded`, `effect.dispatched`, `effect.receipt_recorded`,
  `effect.reconciled`;
- `diagnostic.warning`.

Every event has a monotonically increasing sequence within the run, timestamp,
type, producer, schema version and bounded data object. Ordering across runs is
not inferred from wall-clock time.

## Model observation

The trace records:

- model profile ID and resolved provider/model;
- prompt template ID/version/hash;
- request parameter summary;
- input/output artifact references and hashes;
- token usage, latency, cost and finish status;
- schema validation result;
- fallback or repair action.

Hidden provider reasoning is never required and is not represented as available
if the provider does not return it. The UI shows decisions supported by output,
rules and evidence, not invented chain-of-thought.

## Privacy and redaction

Trace metadata is always retained for audit. Payload retention is configurable:

- `metadata_only`: hashes, sizes, schemas and object references;
- `redacted`: bounded scrubbed input/output preview;
- `full_local`: encrypted or access-controlled full payload artifact;
- `ephemeral`: payload expires after the configured window while hashes remain.

Secrets, bearer tokens, API keys and credential headers are always redacted.
Finance and protected identity payloads default to metadata-only for callers
without the corresponding read permission.

## Pause, review and resume

Before a protected materialization, the stage emits a review request containing
the candidate, source links, existing-object comparison, proposed action and
policy reason. The run enters `waiting_review` and persists a resumable
checkpoint.

Accept, edit-and-accept, reject, merge, supersede and defer are distinct review
decisions. A decision records owner identity and reason. Resumption revalidates
the caller, project, plugin version and target objects; a stale decision cannot
silently overwrite a newer object.

## Retry, replay and fork

- retry repeats a failed stage or remaining graph using the same immutable run
  snapshot and a new run/span attempt;
- replay starts from original inputs using the pinned pipeline and plugin
  versions;
- fork starts from a selected checkpoint with an explicit modified input,
  configuration or pipeline version;
- completed deterministic effects may be reused only when their cache key,
  inputs, policy and plugin version match;
- model output is never claimed to be deterministic merely because parameters
  match.

### Effect intent and receipt

A stage attempt that can affect a remote system, a connector, a Vault file or
another storage transaction uses a stable `effect_key`. Its event sequence is:

```text
effect.intent_recorded -> effect.dispatched ->
effect.receipt_recorded -> effect.materialized
```

The receipt contains the provider idempotency key when supported, bounded
response metadata, result hash and an explicit outcome of `confirmed`,
`confirmed_failed` or `unknown`. Canonical materialization has a uniqueness
constraint on `(run_id, node_id, effect_key)`. An `unknown` outcome pauses for
reconciliation or policy-approved retry; it is never converted to success or
failure from absence of a response. This is exactly-once canonical commit, not
a claim of exactly-once external execution.

## External correlation

DSH, MCP and other runtimes provide an external trace/run/session ID plus source
runtime and version. Memory Harness creates or accepts one universal
`correlation_id` and records links rather than copying unbounded foreign logs.

An external event cannot materialize a canonical memory object. It must enter a
governed pipeline stage with the same project and permission checks as a local
run.

## Storage and export

SQLite stores run snapshots, spans, events, artifacts, object links and review
records transactionally. Events are append-only. An export includes canonical
JSONL projections and hashes so runs remain inspectable outside the live
database. Large artifacts use content-addressed files.

Doctor verifies:

- every run has one start snapshot;
- event sequence has no duplicate or missing committed number;
- terminal runs have one terminal event;
- completed spans have end time and output/error state;
- object links resolve or are explicitly external/tombstoned;
- artifact hash/size matches its file;
- protected materialization has an allowed review decision;
- current projections equal replayed event state;
- every materialized external effect has one matching intent and non-unknown
  receipt, while unresolved intents are visible as recovery work.

## UI views

### Graph

Shows nodes and edges with status, duration and protected-write markers.

### Timeline

Shows ordered events, retries, model/tool calls, waits and review decisions.

### Lineage

Shows final object → stage output → intermediate object → source Evidence.

### Comparison

Compares two runs, pipeline versions, model choices, costs and object diffs.

The user may inspect retained payload only when current permissions allow it;
historical access does not inherit the permissions of the original caller.

## Trace acceptance tests

- success, fallback, denial, timeout, cancellation and human-review fixtures;
- crash after stage effect but before projection update;
- timeout after an external effect but before its receipt, with explicit
  unknown/reconciliation behavior;
- restart and resume without duplicate materialization;
- concurrent stage completion with stable event ordering;
- DSH and MCP external correlation;
- payload retention and expiry;
- secret and finance redaction;
- replay/fork lineage and object comparison;
- Doctor rebuild from events.
