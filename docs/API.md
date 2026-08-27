# MemoryOS v1.1 HTTP, UI and MCP API

All listeners are restricted to loopback addresses. JSON responses and embedded UI resources use `Cache-Control: no-store`; the browser never reads SQLite or JSONL directly.

Every JSON error has one stable shape:

```json
{"error":{"code":"agent_project_forbidden","message":"Agent is not granted access to this project"}}
```

Unknown JSON fields and multiple JSON values in one request are rejected on the governed Agent and model-configuration endpoints.

## Canonical Evidence and compilation

- `POST /v1/evidence` — append one Evidence envelope, route it to a registered project and compile its session;
- `POST /v1/evidence/batch` — validate and append 1–500 envelopes, then compile each affected session once;
- `GET /v1/evidence?id=<id>` or `GET /v1/evidence/{id}` — read exact immutable Evidence;
- `GET /v1/process/sources?project_id=...` — list whole-conversation/file processing units with `pending`, `processing`, `completed`, `changed` or `failed` status;
- `POST /v1/process` — preferred selected-source form is `{"project_id":"project-...","session_ids":["..."],"mode":"incremental|force"}`. Incremental mode skips successfully processed unchanged sources; force mode only rebuilds the selected scope. Every item reports success, failure or skip independently. Cross-project selections are rejected before processing and canonical Evidence is never replaced. Legacy `session_id + force` and project-only calls remain compatible;
- `GET|POST /v1/project-tasks` and `PATCH /v1/project-tasks/{id}` — list/create Owner tasks and move them through `suggested`, `todo`, `in_progress`, `done` or `dismissed`. The public create endpoint always creates an authoritative manual `todo`; AI suggestions are materialized internally with provenance and cannot bypass acceptance;
- `GET|PATCH /v1/projects/{id}/automation` — read or set import processing to `auto_new` (default, only the current new material) or `manual` (save now, process later);
- `GET /v1/layers?project_id=<id>` — project-scoped six-layer counts and pending-review count without loading record bodies;
- `GET /v1/episodes?project_id=<id>&limit=<n>&offset=<n>`, `/v1/episodes/{id}` — inspect paginated session summaries and extracted units through authoritative project links;
- `GET /v1/knowledge-units?project_id=<id>&limit=<n>&offset=<n>` — inspect paginated structured Knowledge Unit v2 records. Each record separates source speaker, asserted-by identity and semantic subject; includes subject/predicate/object, participants, locations, event/valid time, epistemic state and the exact Evidence quote span;
- `GET /v1/sessions/{id}/participants` — read the session-scoped participant registry used for pronoun and speaker resolution;
- `PUT /v1/sessions/{id}/participants` — replace that registry. At most one participant may use role `first_person_speaker`; this is an explicit owner declaration that otherwise-unlabelled first-person speech belongs to that person. Ordinary participant names and aliases never imply that mapping;
- `GET /v1/memories?project_id=<id>&limit=<n>&offset=<n>`, `/v1/memories/{id}/trace` — inspect paginated governed long-term records and full provenance in one project;
- `GET /v1/memory-pins?project_id=<id>&limit=<n>` — list memories the Owner explicitly pinned to the project home page;
- `PUT /v1/memories/{id}/pin` with `{"project_id":"<id>","pinned":true|false}` — pin or unpin one project memory. A pin is presentation-only Owner state: it never changes the memory body, summary, Evidence references or importance;
- `GET /v1/operations`, `GET /v1/operations/{id}` and `POST /v1/operations/{id}/review` — list protected changes, read the exact proposed memory/unit/episode/Evidence needed for a decision, then submit `{"decision":"approve"}` or `{"decision":"reject"}`;
- `GET /v1/living?project_id=<id>`, `GET /v1/living/{id}` — project-scoped growing documents plus readable Markdown and source memories;
- `GET /v1/assets?project_id=<id>&type=<type>`, `GET /v1/assets/{id}` — first-pass Agent-asset candidates plus source memories, classifier evidence and the newest available V4/V3 governed Object/Revision/Review chain. Formal types are `prompt`, `skill`, `rule`, `constraint`, `procedure`, `tool_recipe` and `mcp`; ambiguous candidates remain `unclassified` until Owner resolution;
- `GET /v1/asset-templates` — the seven V4 type-specific field contracts used by the desktop editor and server validator;
- `POST /v1/projects/{id}/assets/refine` — manually compile selected candidate `asset_ids` into V4 template-governed assets. `mode=incremental` skips unchanged sources before model invocation; `mode=force` rechecks only the selected scope. Model failure creates a non-activatable template skeleton, never a false PASS. Evidence is not reprocessed or mutated;
- `GET /v1/projects/{id}/activity-calendar?days=371` — project-scoped daily Evidence intake, generated Knowledge Units/Episodes/Memories and due/completed task counts. `output` is explicitly defined as new Knowledge Units + Episodes + long-term Memories;
- `GET /v1/graph?project_id=<id>&limit=<n>` — the real Evidence-to-knowledge-to-memory-to-asset relationship projection used by the visual graph.
- `GET /v1/graph/semantic?project_id=<id>&limit=<n>` — project-scoped entity/assertion graph. Edges include the forward predicate, inverse label, confidence and source Evidence ID. Unresolved subjects are deliberately excluded instead of being mapped to Owner.

Capture is globally idempotent by `evidence_id`: a byte-equivalent canonical duplicate returns `200`; conflicting content for the same ID returns `409`. A derived processing failure is reported without pretending the canonical append rolled back.

List responses contain `total` (the full project-scoped count) and `offset` (the current page position). `total` is a record count, never a usage count. Project summaries distinguish `knowledge_units`, all `memories`, currently usable `available_memories`, and `pending_review`; protected candidates remain visible in the library with their status instead of making the library appear empty.

The default growth pipeline is `builtin.core-memory-growth.default-import@3.0.0`:

1. `trigger.manual` freezes project/session/Evidence input;
2. `memory.compile` extracts governed Knowledge Unit v2 candidates;
3. `project.derive` refreshes project context and operating projections;
4. `memory.semantic_graph` materializes resolved entities, assertions and temporal facts;
5. `memory.materialize` writes typed, revisioned plugin objects with Run and source provenance;
6. `search.refresh` rebuilds the project-scoped recall projection.

Owner cancellation propagates to the active stage context. The active span becomes `cancelled`, later nodes do not start, and the Run remains `cancelled`; cancellation is not a display-only event.

`GET /v1/pipelines` returns only the highest semantic version of each pipeline for a clear catalog. Exact older immutable versions remain addressable during execution and Run replay.

## Project spaces and operating records

- `GET|POST /v1/projects`, `GET /v1/projects/{id}`;
- `POST /v1/project-links` — associate an existing canonical record to another project without duplicating the source;
- `GET|POST /v1/context-blocks` — bounded, source-bearing project context;
- `GET|POST /v1/goals`, `PATCH /v1/goals/{id}`;
- `GET|POST /v1/milestones`, `PATCH /v1/milestones/{id}`;
- `GET|POST /v1/decisions` — a new decision may explicitly supersede an older one;
- `GET|POST /v1/risks`, `PATCH /v1/risks/{id}`;
- `GET|POST /v1/finance/accounts`;
- `GET|POST /v1/finance/entries`, `PATCH /v1/finance/entries/{id}` — patch only permits audited transition to `void`;
- `GET /v1/finance/summary` — separate totals for every currency.

The project registry is the only project authority. Unknown aliases route to Inbox; user text never becomes a filesystem path. Finance uses signed integer minor units, rejects zero values and cross-project accounts, and never adds different currencies together.

## Temporal facts

- `GET /v1/facts?project_id=<id>&as_of=<RFC3339>&include_history=1`;
- `POST /v1/facts`.

Facts carry `recorded_at`, `valid_from` and optional `valid_until`. A correction creates a new fact with `supersedes_fact_id`; historical records remain queryable.

- `GET /v1/timeline?project_id=<id>&anchor_at=<RFC3339>&from=<RFC3339>&until=<RFC3339>&kinds=fact,goal,...` unifies facts, goals, milestones, decisions, Episodes, Memory, Runs, finance and Evidence around one time anchor. It reports `active/upcoming/overdue/historical/completed/past` relation, temporal relevance and explicit pairwise `correlations` such as `overlaps`, `contains`, `during` and `near_in_time`. Correlation reasons include timestamp proximity, interval overlap, same kind, shared Evidence and task-time context; temporal proximity is never represented as causation.

## Recall

`POST /v1/search` searches raw Evidence with source, session, time, scope, neighboring-turn and seen-session controls.

`POST /v1/search/unified` searches every application layer:

```json
{
  "query": "why did we choose project isolation",
  "project_id": "project-...",
  "all_projects": false,
  "kinds": ["decision", "memory", "evidence"],
  "as_of": "2026-08-20T12:00:00Z",
  "include_history": false,
  "limit": 20
}
```

`project_id` is required unless `all_projects` is explicitly true. Results include project, kind, stable source ID, status, validity interval, lexical/recency/temporal ranks, temporal relation/relevance, metadata and a recall context ID. Supplying `as_of` increases the temporal-rank weight so historical recall is ranked around that anchor instead of merely by present recency. `POST /v1/recall/feedback` records helpful/irrelevant feedback without mutating canonical memory.

Recall uses FTS5 `unicode61` BM25 for Latin/code, `trigram` BM25 for CJK of three or more characters, substring fallback for very short input, and a deterministic 384-dimensional `local-feature-hash-v1` embedding. Lexical rank, embedding similarity, temporal relevance and recency are combined with reciprocal-rank fusion. Results expose `embedding`, `embedding_dimensions`, `vector_rank` and `vector_similarity` so clients can distinguish the signals. Both FTS and embedding projections are derived and rebuildable; the local feature embedding is not a hosted semantic model.

## Memory Blueprints and strategy composition

- `GET /v1/blueprints` — list immutable published Blueprint versions;
- `POST /v1/blueprints/validate` — statically validate tracks, bindings,
  Evidence/model policy, limits and embedded-secret rules;
- `POST /v1/blueprints` — publish an immutable owner Blueprint version;
- `GET /v1/projects/{id}/blueprint` — read the effective project Blueprint,
  inherited/explicit assignment, content hash and availability validation;
- `PUT /v1/projects/{id}/blueprint` — activate an exact Blueprint ID/version
  after verifying plugin state and project capability grants.

The default Blueprint combines six-step memory growth, Palace-style structural
organization and four-level progressive recall. These are independent tracks,
and every node references an exact plugin strategy component. Pipeline run
snapshots include the effective Blueprint; retry and stage-fork reuse the original snapshot. The Growth track is executable policy, not display metadata: disabling `growth.knowledge`, `growth.episode`, `growth.long-term`, `growth.living` or `growth.agent-asset` suppresses that Harness materialization role in the default business Pipeline and records the disabled roles in the Run trace.

## Harness revisions, replay and zero-write preview

- `GET /v1/harness/objects/{id}/revisions` — immutable revision history;
- `POST /v1/harness/objects/{id}/revisions` — append an Owner revision proposal. `expected_revision` and non-empty `edit_reason` are mandatory; stale optimistic-lock values fail before the proposal is written;
- `GET /v1/harness/revision-reviews`, `GET /v1/harness/revision-reviews/{id}` and `POST /v1/harness/revision-reviews/{id}/decision` — inspect Diff/validation and approve or reject. Protected V4/V3 Agent assets can become `active` only when server-side validation passed. Activating a V4 asset supersedes an older active V2/V3/V4 object for the same `asset_id`;
- `POST /v1/harness/objects/{id}/rollback` — restore historical content by appending a new revision; history is never deleted;
- `POST /v1/harness/runs/{id}/retry` — replay from the original immutable run input and record `retry_of_run_id`;
- `POST /v1/harness/runs/{id}/fork` with `{"node_id":"..."}` — create a new Run that reuses immutable completed-prefix stage snapshots and executes only the requested suffix. Old Runs that predate durable stage snapshots fail closed and require whole-Run retry;
- `POST /v1/pipelines/dry-run` — zero-write Pipeline preflight. Pure JSON stages execute in memory, object materialization performs schema validation only, Review Gates are simulated, and model/business/search stages are planned without model invocation or database/index mutation.

Each successful stage stores a bounded, content-hashed `harness_stage_outputs` snapshot for replay. A `(run_id,node_id)` output is immutable; attempting to record different content for the same completed node is rejected.

## Plugin lifecycle

- `GET /v1/plugins/{id}/{version}/impact?project_id=<id>` — preview current objects, historical revisions/Runs, Pipeline/Blueprint versions, enabled projects and active Blueprint references before retirement;
- `POST /v1/plugins/{id}/{version}/retire` — retire only when no project or active Blueprint still depends on the version. Retirement clears the executable package blob, marks contributed Memory Types disabled and preserves Object/Revision/Run/Pipeline history;
- reinstalling the exact same semantic version is allowed only with the identical content hash and re-enables its immutable type contracts. Different bytes under the same semantic version are rejected and must be published as a new version.

## Knowledge products

`builtin.living-asset-vault.knowledge-product.v1` is the Object-Store authority for `report`, `diary`, `personal_capability`, `personal_profile`, `project_brief`, `decision_log` and `risk_report`. The default growth Pipeline materializes a project brief; human-authored products capture their visible text as Evidence before the product object is created. `locked_fields` can protect `title`, `summary` or `body`; later automatic regeneration appends a revision while preserving locked human content.

## Imports and connectors

- `POST /v1/import/text` — 1–100 text or Markdown documents;
- `POST /v1/import/conversations` — ChatGPT, Claude, DeepSeek or normalized JSON; supports `dry_run` preflight;
- `GET|POST /v1/connectors` — durable connector registry;
- `GET /v1/sources` — real source/evidence/session summaries.

Conversation imports preserve the original JSON under the local sources archive before derived compilation. Platform thinking/reasoning blocks stay in the raw archive and are excluded from default memory distillation. Connector and import idempotency keys make retries safe.

## Health and recovery

- `GET /health`, `/v1/version`;
- `GET /v1/dashboard`, `/v1/today`, `/v1/jobs`, `/v1/health/detail`;
- `GET /v1/doctor` — checks ledger, receipts, project links, foreign keys, six layers, both Evidence FTS indexes, both unified FTS indexes, and the local embedding count/algorithm/dimensions/payload;
- `POST /v1/rebuild/search` — rebuild Evidence and unified search projections.

CLI:

- `memoryosd start`
- `memoryosd doctor`
- `memoryosd rebuild`
- `memoryosd export --output <file.tar.gz>`
- `memoryosd restore --input <file.tar.gz> --home <empty-target>`
- `memoryosd mcp`
- `memoryosd version`

## Desktop application and headless integration

Memory Harness Desktop is the canonical owner interface. It starts the same Go
core on an ephemeral loopback port and establishes a short-lived owner session
through the desktop bridge. The standalone `memoryosd start` listener (commonly
`127.0.0.1:19777`) is for headless HTTP and MCP integration; it is not a second
owner application. It never receives desktop owner authority merely because a
browser can reach the loopback address.

### Local owner configuration API

These endpoints back the loopback-only **连接与健康** page. They are owner administration endpoints and do not accept an Agent token as a substitute for local owner access:

- `GET|POST /v1/agents` — list principals or create one; creation returns the plaintext token once;
- `PATCH /v1/agents/{agent_id}` — replace status, permissions and project grants;
- `POST /v1/agents/{agent_id}/rotate-token` — invalidate the old token and return its replacement once;
- `GET /v1/agent-audit?agent_id=<id>` — latest allowed, denied and failed Agent events;
- `GET /v1/model/config` — runtime, redacted providers, connection presets, the dated model/protocol knowledge catalog and privacy boundary;
- `POST /v1/model/providers`, `PATCH /v1/model/providers/{id}` — create or update a provider; `api_key` is write-only. `protocol` is one of `openai_responses`, `openai_chat` or `anthropic_messages`; a full endpoint ending in `/responses`, `/chat/completions` or `/messages` is normalized to its base URL;
- `POST /v1/model/providers/{id}/test` — authenticate and inspect the provider's current `/models` response. Returned `model_details` combines that live list with known protocol metadata; live presence is authoritative, while the built-in descriptions are only a dated selection aid;
- `PUT /v1/model/runtime` — select `rules` or `agent`; rules fallback is always enabled in v1.1;
- `GET /v1/integrations/capabilities` — machine-readable MCP tool, permission and documentation manifest.

Create an Agent:

```json
{
  "name": "Codex · MemoryOS",
  "kind": "codex",
  "permissions": ["memory.read", "memory.capture", "project.read"],
  "all_projects": false,
  "project_ids": ["project-..."]
}
```

### Authenticated Agent HTTP API

Send `Authorization: Bearer mos_...` on every Agent request. Authentication, the named permission and the project grant are checked independently.

- `GET /v1/agent/me` — current principal and effective grants;
- `GET /v1/agent/projects` — only granted projects; budget and finance are redacted without `finance.read`;
- `GET /v1/agent/projects/{id}/context?as_of=<RFC3339>` — project context; finance entries require `finance.read`;
- `GET /v1/agent/projects/{id}/blueprint` — exact active Memory Blueprint and
  validation; requires `project.read` and the project grant;
- `POST /v1/agent/search` — raw Evidence search in one project;
- `POST /v1/agent/recall` — unified recall; `all_projects:true` additionally requires an all-project grant;
- `GET /v1/agent/evidence/{id}` — exact Evidence after record-to-project authorization;
- `POST /v1/agent/capture` — idempotent canonical Evidence write; requires `memory.capture`. The same normalized request and key returns the original Evidence without rerunning the pipeline; changed content with that key returns `409`;
- `POST /v1/agent/project-records` — goal, decision or risk; requires `project.write`;
- `POST /v1/agent/finance-entries` — finance entry; requires `finance.write`;
- `GET /v1/agent/objects`, `GET /v1/agent/objects/{id}` — project-authorized generic Harness objects;
- `GET /v1/agent/team/tasks` — active collaboration tasks that directly include this Agent;
- `GET|POST /v1/agent/team/tasks/{id}/private` — read or write only this Agent's private task drafts;
- `GET|POST /v1/agent/team/tasks/{id}/blackboard` — read directly visible contributions or explicitly submit one;
- `POST /v1/agent/team/blackboard/{id}/share` — replace direct recipients only for an entry authored by this Agent; recipients cannot forward it;
- `POST /v1/agent/team/tasks/{id}/leave-proposal` — create an Owner review instead of changing task membership immediately;
- `POST /v1/agent/objects/{id}/revision-proposals` — propose a V4 or compatible V3 governed Agent-asset revision only. Requires `memory.propose`, `expected_revision`, `edit_reason`, complete payload and stable `idempotency_key`. The server recomputes validation; the proposal stays pending and does not move `current_revision`. Agent tokens are not accepted on Owner approval endpoints.

Example Agent capture:

```json
{
  "project_id": "project-...",
  "idempotency_key": "codex-session-42-turn-8",
  "source_system": "codex",
  "session_id": "codex-session-42",
  "role": "assistant",
  "text": "We decided to retain the deterministic fallback.",
  "observed_at": "2026-08-21T12:30:00Z"
}
```

The response distinguishes canonical success from derived failure. If Evidence was safely appended but model/rule processing failed, the response contains `pipeline_status:"failed"`; it never claims the append was rolled back.

The authenticated MCP bridge exposes the same governed operations through 24
tools, including temporal context, active governed assets, revision proposal, generic objects, traces and project Blueprint inspection. See
`docs/MCP.md` and `docs/openapi-agent.yaml` for the exact contracts.

## Explicit boundaries

The local `rules-v2` compiler is deterministic, conservative and inspectable: it keeps explicit durable signals and leaves ordinary speech only in Evidence. Configured model output is schema-validated, Evidence-linked and forced through the same risk policy. Long transcripts are distilled in bounded chunks using a compact provider contract that is expanded locally into Knowledge Unit v2. An automatic first import may expose a clearly labelled local-rules preview when the provider is unavailable; an explicit rebuild never replaces an existing model projection with partial or fallback output. Missing speaker identity is not guessed: unresolved first-person statements remain reviewable but are excluded from the semantic graph until the owner supplies participant context. Every successful default import emits a completed six-stage Harness Run and projects explicit decisions, goals and risks into the selected project. LLM Wiki remains an external multi-format adapter boundary. MemoryOS never silently mutates an existing Wiki, activates Prompt/Skill/Rule/MCP assets, deploys host configuration or hard-deletes canonical Evidence.
