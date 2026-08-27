# Memory Harness Product Specification

Status: accepted development baseline on 2026-08-21

## Product promise

Memory Harness gives a person one local place to inspect, govern and program
how AI systems remember. It turns conversations, files and operations into
traceable memory objects while keeping projects, Agents and sensitive domains
separated.

The application is not complete merely because it can store or search text. A
successful product lets a non-technical owner answer:

- What entered the system?
- What did the model or rule infer from it?
- Which memory layer did it become, and why?
- What was rejected, merged, superseded or sent for review?
- Which project and Agents may use it?
- Which plugin and model version made the decision?
- Can I change the strategy without losing old history?

## Product hierarchy and first-use path

Memory Harness Desktop is the only owner-facing product entry. The standalone
loopback daemon is a headless HTTP/MCP integration surface, not a second UI.
Inbox and Personal are project boundaries inside one product, not separate
applications.

The default first-use path is deliberately ordinary-user first:

1. choose Personal or a project;
2. import files, pasted notes or an AI conversation archive;
3. inspect the source ledger and the six real memory layers;
4. search or open the project workspace;
5. review protected changes;
6. configure a model only when model-enhanced distillation is wanted.

Strategy Workbench, Pipeline Studio and Plugin Center remain available but live
inside a collapsed Advanced Builder area. Their existence must not block a new
user from importing and understanding a first memory.

## Primary users

### Local owner

Installs the application, controls projects, models, Agents, plugins, review
policies, exports and backups. The local owner has non-delegable approval
authority for protected mutations. Owner authority comes from an authenticated
desktop session, not merely from being able to reach a loopback port.

### Knowledge worker

Imports conversations and files, searches across approved scopes, reviews
candidate memories and uses project/business views without configuring the
runtime.

### Workflow designer

Clones a complete Memory Blueprint or one pipeline. They can replace strategy
plugins, add a memory layer, change recall cost, or change individual processing
steps, then publish an immutable version after validation.

### External Agent

Uses a dedicated token through MCP or HTTP. It sees only the tools, projects
and object classes allowed by its effective grants.

### Plugin developer

Builds a declarative, WebAssembly or external adapter plugin against a stable
SDK and conformance suite.

## Core jobs to be done

1. Capture a conversation, file, note, event or API payload without losing the
   original source and provenance.
2. Run a visible memory strategy that can pause, fail, retry and request human
   input.
3. Inspect and compare every intermediate object.
4. Search and compile a bounded context for one project or an explicitly
   selected portfolio scope.
5. Review protected changes before they affect identity, behavior, finance or
   active Agent assets.
6. Add a new memory layer or business transformation without rebuilding the
   product kernel.
7. Connect multiple Agents with distinct read/write permissions and audit
   trails.
8. Package, back up, restore and upgrade the application without command-line
   knowledge.

## Default product modules

### Home and Inbox

- capture volume, pending runs, failed runs and review queue;
- current project health and recent memory evolution;
- actionable items rather than vanity totals;
- no demonstration data in the production home.

### Project Space

- project overview, bounded context, goals, milestones, decisions and risks;
- project Memory Map and recent runs;
- project-scoped business and finance views when their plugins are enabled;
- explicit portfolio-wide recall rather than accidental cross-project access.

### Memory Map

- displays enabled memory types and their relationships;
- the default six-layer strategy is expanded into inspectable layers;
- custom layers appear automatically from type registration;
- every count links to real objects, and every object links to provenance.

### Run Explorer

- graph, timeline and object-lineage views;
- stage input/output, model/tool calls, policies, cost and duration;
- pause, cancel, retry and branch where policy permits;
- compare two runs or two versions of one pipeline.

### Pipeline Studio

- create from blank, built-in template or existing version;
- drag, connect and configure typed nodes;
- validate schemas, permissions, cycles and unreachable nodes before publish;
- preview against selected Evidence in dry-run mode;
- publish creates an immutable version; editing creates a draft.

### Strategy Workbench

- shows one project's complete active Memory Blueprint and content hash;
- keeps semantic growth, spatial organization and recall cost visible as three
  separate tracks;
- clones a published template into a project-owned draft;
- replaces compatible plugin components, adds/removes/reorders layers and edits
  per-component JSON configuration;
- edits Evidence preservation, model boundary, context budget and explicit
  cross-project recall policy;
- validates structure first, then project plugin permissions at activation;
- publishes an immutable version and binds it only to the selected project.

Plugin Center answers “what can be installed and trusted”; Strategy Workbench
answers “which components make up this project's memory”; Pipeline Studio
answers “how does one individual workflow execute”.

### Review Center

- queues by project, policy, object type and risk;
- shows source Evidence, candidate, existing objects and proposed relationship;
- actions: accept, edit-and-accept, reject, merge, supersede or defer;
- every action records owner identity, time, reason and resulting object.

### Search and Recall

- lexical recall is available without optional infrastructure;
- optional vector, graph and reranking providers use the same result contract;
- filters include project, object type, time, lifecycle, source and plugin;
- results show source references and why they matched;
- context compilation has an explicit character/token budget.

### Plugin Center

- installed, built-in, available update, disabled and quarantined states;
- permission preview before enablement;
- compatibility, signature, publisher, changelog and migration preview;
- per-project enablement and configuration;
- rollback to a previously retained version.

### Connections, Agents and Models

- guided setup for Codex, Claude, ChatGPT-compatible clients, DSH, OpenClaw and
  custom MCP clients;
- one-time Agent credentials with project and capability selection;
- model profiles, privacy boundary, connection test and cost controls;
- recent allowed, denied and failed operations;
- no secret displayed after creation and no secret placed in browser storage;
- owner-only changes require the desktop owner session; Agent bearer tokens are
  rejected on owner administration routes;
- credential status names the actual platform store. A volatile in-memory
  fallback is reported as unsupported for release, never as an OS keychain.

### Health, Backup and Recovery

- database integrity, Evidence/receipt parity, projection health, plugin health
  and credential availability;
- one-click export and restore to a new target;
- migration preview and pre-migration backup;
- rebuild of disposable projections;
- diagnostics bundle that redacts secrets and configurable private payloads.

## Built-in plugin catalog

### Core Memory Growth

`Evidence → Knowledge Unit → Episode → Memory Record → Living Knowledge → Agent Asset`

Protected identity, procedure, correction and behavior objects require review.

### Project Operations

Project context, goals, milestones, decisions, risks and temporal facts.

### Opportunity Intelligence

Conversation or note → need → customer/project link → qualification → score →
risk/gaps → review → opportunity → next action.

The default score is explainable and editable. The plugin does not represent a
candidate as confirmed revenue.

### Business Finance

Budgets, accounts and immutable entries using integer minor units. Currencies
remain separate unless an explicit conversion plugin and rate source are used.

### Agent Assets

Prompt, Skill, Rule, Constraint, Procedure, Tool Recipe and MCP Recipe with
version, validation, activation and rollback state.

### Conversation Connectors

Text/Markdown, ChatGPT export, Claude export, DeepSeek export and normalized
conversation JSON. Hidden reasoning remains in the raw archive and is excluded
from default distillation.

### DSH Bridge

Completed-turn capture, pre-step recall, DSH workflow trace ingestion and deep
links. DSH remains optional.

### Living Asset Vault

Versioned Markdown/structured assets, including an explicit import path for
reviewed LLM Wiki content.

## Non-goals for the first desktop release

- a public cloud synchronization service;
- a public plugin marketplace with automatic remote installation;
- collaborative multi-owner real-time editing;
- automatic activation of behavior-changing Agent assets;
- mandatory embeddings or graph database;
- arbitrary host-code plugins;
- claiming deterministic replay of nondeterministic model output;
- silently importing or rewriting an existing DSH Knowledge Hub or LLM Wiki.

## Product-level acceptance scenarios

### Add a new memory layer

An owner installs a declarative `relationship-memory` package. The Memory Map
shows the layer, Pipeline Studio can target it, a dry run validates its schema,
and a published run writes an object with full provenance. No kernel source or
SQL migration is changed.

### Build an opportunity pipeline

An owner clones the built-in opportunity pipeline, changes its qualification
threshold and inserts an approval node. A conversation creates a pending
opportunity, the Run Explorer shows every extraction and score, and approval
creates the final project object plus next action.

### Enforce Agent isolation

Codex may read and capture in Project A. DSH may read Projects A and B but may
not write finance. Each client sees a different MCP tool/resource surface.
Denied calls appear in the local audit view and create no canonical object.

### Recover from model failure

A remote model times out after Evidence capture. The run records canonical
capture success and stage failure, then pauses or uses the configured rules
fallback. Retrying reuses the same Evidence and creates a new stage attempt,
not a duplicate source.

### Install and restore

A new user installs the signed application, completes onboarding, connects an
Agent without a terminal and imports a backup. Doctor reports the same
canonical counts and hashes; disposable indexes rebuild automatically.

### Reject a loopback confused deputy

A normal browser page, an Agent token and a plugin each attempt to call owner
administration routes on `127.0.0.1`. Agent creation, grant escalation, model
secret change, plugin enablement and protected approval all fail and create an
audit event. The signed desktop WebView succeeds only with its short-lived
owner session and anti-CSRF/origin checks.
