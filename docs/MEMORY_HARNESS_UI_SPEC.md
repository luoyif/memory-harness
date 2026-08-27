# Memory Harness Interaction and UI Specification

Status: proposed for review on 2026-08-21

## Interaction principles

1. Show the current outcome first, with provenance one interaction away.
2. Use project scope as a persistent visible boundary.
3. Present memory growth as real objects and transitions, not decorative
   metrics.
4. Keep review, permission and model-use state visible before a consequential
   action.
5. Make the default strategy usable without exposing workflow-engine jargon.
6. Put advanced DIY controls in Studio while keeping normal capture and search
   simple.
7. Empty, loading, denied, degraded and failed states are designed states.
8. Every graph also has a keyboard-accessible list/timeline representation.

## Application shell

The desktop shell contains:

- global project switcher;
- unified search entry;
- health and active-run indicators;
- notifications and pending-review count;
- primary navigation;
- contextual action area;
- owner/profile and privacy state.

The application never hides an active cross-project scope. Portfolio recall
uses an explicit label and confirmation when protected domains are included.
The shell also shows whether the owner session is active, locked or expired.
Refreshing or opening the loopback URL in an ordinary browser does not silently
obtain owner authority.

## Navigation

### Work

- **Home** — today, inbox, active/failed runs and next actions;
- **Projects** — portfolio and project workspaces;
- **Search** — scoped recall and context compilation.

### Memory

- **Memory Map** — layers/types, relations and object inspection;
- **Runs** — trajectory explorer;
- **Review** — protected changes and conflicts.

### Build

- **Studio** — pipeline templates, drafts, validation and versions;
- **Plugins** — installed extensions and permissions;
- **Connections** — sources, Agents, models and DSH.

### System

- **Health & Backup** — Doctor, migrations, export and restore;
- **Settings** — privacy, retention, appearance and advanced policies.

The main navigation may collapse advanced Build/System sections during initial
onboarding, but the underlying pages remain directly addressable.

## Home

Top-level cards are actionable:

- Evidence awaiting processing;
- runs waiting for review;
- failed or degraded runs;
- recent memory changes;
- upcoming project milestones and next actions;
- connector/model/plugin health issues.

Selecting a count opens its filtered list. The page does not display fabricated
recommendations when no data exists.

## Project workspace

Header:

- project name, lifecycle and privacy badge;
- active pipelines and enabled plugins;
- bounded context usage;
- capture and search actions.

Tabs:

- Overview;
- Memory;
- Runs;
- Goals & Decisions;
- Risks;
- Opportunity, Finance or other plugin-contributed tabs;
- Sources and access.

The project page shows why an object belongs to the project and whether the link
is primary, shared or inferred/pending.

## Memory Map

### Layer view

Displays ordered or branched memory types contributed by enabled plugins. The
built-in six layers show their real counts, pending/active status and conversion
rates. Custom types use registered labels and schemas.

### Object explorer

Left: filterable object list. Center: rendered object. Right: provenance,
versions, relations, permissions and lifecycle.

Required actions:

- open source Evidence;
- open producing run and stage;
- compare revisions;
- inspect conflicts/supersession;
- request correction;
- add/remove a project link where authorized;
- export an object with provenance.

### Graph view

Shows selected objects and typed relations. It is a projection and states its
scope and truncation. A list alternative provides the same information.

## Run Explorer

### Run list

Filters: project, status, pipeline, plugin, caller, trigger, model, time, review
state and cost. Rows show the latest meaningful state and do not label a
canonical Evidence success as a complete pipeline success.

### Run detail

Header: status, pipeline version, project, caller, trigger, start/end, cost and
privacy mode.

Views:

- Graph;
- Timeline;
- Object lineage;
- Inputs/outputs;
- Evaluation and warnings.

Selecting a node opens a stage inspector with configuration, attempts,
permissions, input/output references, model/tool observations, errors and
artifacts. Sensitive payloads require current permission and an explicit reveal.

Actions: pause, resume, cancel, retry, replay and fork. The UI explains whether
the action reuses deterministic effects or invokes a model again.

## Pipeline Studio

### Catalog

Groups built-in templates, project pipelines, personal pipelines and imported
packages. Each card shows version, risk class, required plugins, last validation
and usage.

### Editor

- node palette grouped by trigger, memory, transform, model, policy, review,
  output and integration;
- typed ports and highlighted compatible targets;
- properties inspector generated from configuration schema;
- minimap, zoom, auto-layout and keyboard list editor;
- draft autosave and explicit publish;
- validation panel with error-to-node navigation;
- immutable version history and diff.

### Preview

The owner selects existing Evidence or a synthetic local fixture. Preview uses
dry-run materialization, displays the same trace UI and makes no canonical
object unless the owner explicitly promotes the result through review.

### Simple mode

For ordinary users, a guided form exposes:

- when to run;
- what to extract;
- model or local rules;
- acceptance threshold;
- what needs approval;
- where the result appears.

The canvas remains available under Advanced.

## Review Center

The comparison surface contains:

- source Evidence excerpts and links;
- new candidate and confidence;
- matching current objects;
- proposed NEW/UPDATE/MERGE/SUPERSEDE/CONFLICT action;
- policy reason and affected Agents/projects;
- producing run, plugin, model and prompt versions.

Bulk approval is disabled for protected identity, behavior, procedure, finance
and Agent activation. Rejection and edit-and-accept require an optional or
mandatory reason according to policy.

## Connections

### Agent wizard

1. choose client type;
2. choose projects;
3. choose capabilities;
4. review effective access;
5. generate one-time credential;
6. copy a client-specific configuration or use supported one-click setup;
7. run read/write/denial connection checks;
8. display resulting audit events.

The user does not need to type a terminal command for normal setup.

Owner credentials and Agent credentials are separate. The UI never stores an
owner session or bearer token in localStorage/sessionStorage, never places one
in a URL, and clears the in-memory owner session when the desktop instance
locks or exits.

### Model center

Shows local/remote privacy boundary, provider, model, credential state, test
result, rate/cost limits and rules fallback. Connection success and extraction
quality are reported separately.

### DSH

Shows adapter status, bound workspace/projects, capture mode, recall mode and
last correlated runs. DSH is labeled Optional Integration rather than Core.

## Plugin Center

The install review shows:

- publisher/signature and source;
- memory types, stages, pipelines and views contributed;
- required/optional capabilities;
- network/model/connector use;
- configuration and data migrations;
- project scope;
- compatibility and rollback availability.

A quarantined plugin cannot start new runs. Historical objects and traces remain
viewable through generic renderers.

## Responsive and accessibility requirements

- desktop target from 1280×720 upward;
- usable narrow mode at 390×844 for browser/mobile inspection, without claiming
  a native mobile package;
- WCAG 2.2 AA color contrast for primary workflows;
- complete keyboard path for capture, search, review and Studio list editor;
- focus restoration after dialogs;
- labels and descriptions for graph nodes and ports;
- status is never communicated by color alone;
- reduced-motion support;
- Simplified Chinese first, localization-ready message catalog.

## Visual language

The product should feel like a calm instrument panel, not a generic admin
dashboard. Use restrained color, strong typographic hierarchy, generous object
inspection space and consistent provenance markers. Graphs and animation serve
causal understanding; they are not decorative backgrounds.

## UI acceptance tests

- first-run onboarding with no data;
- populated Home and Project workspace;
- six built-in and one custom memory layer;
- successful, failed, denied and waiting-review run details;
- Studio create, validate, preview, publish and version diff;
- opportunity pipeline review and materialization;
- Agent setup plus an intentional denied operation;
- plugin permission diff, quarantine and rollback;
- secret/payload reveal permission;
- ordinary-browser, forged-Origin, missing-CSRF and Agent-token attempts
  against owner-only actions are denied;
- expired/locked owner-session behavior and audit visibility;
- keyboard-only review and pipeline list editing;
- desktop and 390×844 layouts;
- browser console errors and accessibility scan.
