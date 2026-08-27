# Memory Harness Plugin Contract

Status: proposed contract, API version `memory-harness.plugin/v1alpha1`

## Purpose

The plugin contract lets the product gain memory layers, processing strategies,
connectors, projections and application views without granting extensions
direct access to canonical stores or owner secrets.

The contract separates three concepts that must not be conflated:

- a **memory type** defines a persistent object schema and lifecycle;
- a **stage type** defines a bounded computation with typed input and output;
- a **pipeline** connects stages into a versioned directed acyclic graph.
- a **strategy component** is a replaceable implementation for one Blueprint
  slot such as `growth.episode`, `organization.topic` or `recall.deep`;
- a **Blueprint** composes those slots into a complete project memory strategy.

A plugin may contribute any combination of these concepts.

## Package format

The distributable suffix is `.mhplugin`. It is a deterministic ZIP archive:

```text
manifest.yaml
schemas/
  memory-types/*.schema.json
  stages/*.input.schema.json
  stages/*.output.schema.json
pipelines/*.pipeline.yaml
blueprints/*.blueprint.yaml
prompts/*
ui/
  settings.schema.json
  contributions.json
wasm/                       # optional
docs/
  README.md
  CHANGELOG.md
SIGNATURE                   # required outside developer mode
```

Archive entries are normalized, size limited and path validated before
extraction. Symlinks, absolute paths, traversal, devices and undeclared files
are rejected.

## Manifest

Required fields:

```yaml
apiVersion: memory-harness.plugin/v1alpha1
kind: Plugin
metadata:
  id: com.example.relationship-memory
  name: Relationship Memory
  version: 1.0.0
  publisher: com.example
  license: Apache-2.0
compatibility:
  memoryHarness: ">=2.0.0 <3.0.0"
trust:
  class: declarative
contributes:
  memoryTypes: []
  stages: []
  pipelines: []
  strategyComponents: []
  blueprints: []
  connectors: []
  projections: []
  views: []
permissions:
  required: []
  optional: []
configuration:
  schema: ui/settings.schema.json
```

Unknown top-level fields fail validation for v1alpha1. Metadata does not grant
authority. A valid signature proves package identity, not trustworthiness.

Outside developer mode, the signer must chain to an owner-approved trust entry
or a bundled first-party root. The trust store records fingerprint, publisher,
scope, approval time and revocation state. Revoked or newly untrusted packages
cannot start runs; historical manifests remain inspectable. Offline install
shows the exact fingerprint and permission diff before approval. The first
desktop release does not imply a public marketplace or automatic trust.

## Extension identifiers

- plugin IDs use reverse-domain notation;
- contributed IDs are namespaced by plugin ID;
- IDs are immutable after publication;
- display names are localized labels and are never authorization keys;
- versions use semantic versioning;
- every run pins the exact plugin and pipeline version used.

## Memory type contribution

A memory type declares:

- stable type ID and display metadata;
- payload JSON Schema;
- lifecycle states and allowed transitions;
- protection class;
- retention and redaction defaults;
- permitted relation types;
- search fields and optional projection hint;
- generic renderer schema or a declared UI contribution;
- declarative migration descriptions between its own payload schema versions.

The kernel envelope remains stable. Plugins own only their payload schema.
Schema validation is bounded by size, depth, property count and evaluation time.
Remote `$ref`, executable formats and ambiguous recursive schemas are rejected.

## Strategy component and Blueprint contributions

A strategy component declares a stable `componentId`, semantic `version`,
human label, `role`, kernel `kind`, optional `stageType`, optional configuration
schema and required capabilities. `role` is the replacement slot;
`kind` (`memory_type`, `stage`, `provider` or `policy`) is the stable kernel
primitive. A component cannot change its role or kind without a new ID/version.

A Blueprint definition uses API version
`memory-harness.blueprint/v1alpha1`, is namespaced by its publishing plugin and
contains tracks of component bindings. Every binding pins the plugin,
component, versions, enablement, declared capabilities and JSON configuration.
The kernel requires `growth`, `organization` and `recall` tracks, rejects secret
material, and requires Evidence to preserve the verbatim source. Additional
nodes and future tracks remain data, not compiled application navigation.

Plugin installation preflights all declared Blueprint files. Publishing a
plugin atomically publishes its immutable Blueprint versions. Enabling a
Blueprint for a project additionally verifies that every component is actually
declared by the referenced plugin and that the project granted every required
capability. Experimental plugins require explicit project enablement.

## Stage type contribution

A stage type declares:

- input and output JSON Schemas;
- configuration JSON Schema;
- deterministic, model, human, connector or subpipeline execution class;
- required capabilities;
- retry safety and idempotency behavior;
- timeout, maximum payload and artifact policies;
- whether it may pause for input;
- redaction and trace-retention behavior;
- optional compensation behavior.

A stage never receives a raw database handle or unscoped filesystem path.
Every input is a copied value or opaque object/artifact reference authorized for
the current run.

## Pipeline definition

A pipeline definition contains:

- stable pipeline ID, immutable version and human-readable intent;
- typed inputs and outputs;
- trigger definitions;
- nodes, ports, edges and conditional routes;
- stage configuration and model-profile references;
- review and failure policy;
- maximum concurrency, duration, model calls and budget;
- required project/plugin capabilities;
- output materialization rules;
- test fixtures and expected invariants.

Publish-time validation rejects:

- cycles except a bounded kernel loop node;
- unreachable nodes or outputs;
- incompatible port schemas;
- undeclared capability use;
- missing review before a protected write;
- unbounded fan-out, retry or loop policies;
- secret values embedded in the definition;
- references to mutable `latest` plugin or prompt versions.

Publishing freezes an immutable definition. Editing a published pipeline creates
a draft derived from that version.

## Built-in stage classes

- `trigger.manual`, `trigger.capture`, `trigger.schedule`, `trigger.connector`;
- `evidence.select`, `evidence.chunk`;
- `memory.retrieve`, `memory.compare`, `memory.relate`;
- `transform.map`, `transform.filter`, `transform.score`;
- `llm.extract`, `llm.classify`, `llm.synthesize`;
- `policy.route`, `policy.require_review`;
- `review.human`;
- `object.materialize`, `asset.materialize`;
- `connector.call`, `notification.emit`;
- `pipeline.subpipeline`;
- `control.parallel`, `control.join`, `control.bounded_loop`.

LLM stages return schema-constrained candidates. They cannot commit memory
objects directly.

## Execution and trust classes

### Declarative

Uses only built-in stage types and registered external capabilities. This is the
default and safest DIY format.

### Sandboxed WebAssembly

Runs in a memory- and time-bounded WebAssembly runtime. The host exposes only
declared JSON capability calls. There is no ambient filesystem, network, clock,
randomness, process or secret access. If deterministic replay matters, time and
random values are explicit recorded inputs.

WebAssembly is not a release assumption. It is admitted only after a concrete
runtime spike proves fuel/time/memory limits, cancellation, deterministic host
calls, capability mediation and package portability. If the spike fails, the
first release supports declarative packages and governed external adapters
only; the public contract must not advertise executable WASM as available.

### External adapter

Connects through governed MCP, loopback HTTP or supervised stdio. The adapter
has a separate principal, explicit project grants, timeouts and output limits.
It cannot inherit the installing owner's authority.

First-party code compiled into the application is a separate trust class and
still uses the same stage/run contracts for observability.

## Permission model

Effective permission is:

```text
caller grant
  intersection project policy
  intersection plugin grant
  intersection pipeline declaration
  intersection stage requirement
```

An empty intersection denies the operation. There is no fallback to owner
authority. Opaque object handles are names, not bearer capabilities; every
dereference repeats authorization.

Initial capabilities:

- `evidence.read`, `evidence.capture`;
- `memory.read`, `memory.propose`, `memory.materialize`;
- `project.read`, `project.write`;
- `finance.read`, `finance.write`;
- `asset.read`, `asset.propose`, `asset.activate`;
- `model.invoke` scoped to model profile;
- `connector.invoke` scoped to connector;
- `notification.emit`;
- `trace.read_payload` for sensitive retained payloads.

`asset.activate`, protected `memory.materialize` and high-risk finance actions
remain owner-reviewable even if declared.

Owner-session capabilities are not part of the plugin namespace. A plugin,
Agent or external adapter cannot request `owner.*`, approve its own protected
write, create another principal, broaden a project grant or install/enable a
package. The desktop owner API evaluates its own authenticated session before
the normal capability intersection.

## Configuration and secrets

- plugin configuration is validated against its schema and namespaced by
  plugin, project and environment;
- secret fields are written to the platform secret store and replaced with
  opaque references;
- APIs return `has_secret`, never plaintext;
- plugin export omits secret values;
- disabling or uninstalling a plugin does not delete canonical objects;
- a plugin migration cannot see secrets that are not explicitly declared.

## UI contributions

Plugins may contribute navigation entries, dashboard cards, object renderers,
settings forms and Pipeline Studio nodes through declarative schemas. They do
not inject arbitrary HTML or JavaScript into the owner application.

Each contribution declares placement, required permission, supported object
type and empty/error/loading states. The application owns layout, accessibility,
theme, navigation and security boundaries.

## Lifecycle

States:

```text
discovered → verified → installed → enabled
                         ↓          ↓
                      disabled ← quarantined
                         ↓
                      removed
```

Installation performs package, signature, compatibility, schema, permission and
migration preflight. Enablement is per workspace/project when supported.
Updates show permission and migration diffs before approval. Rollback restores a
retained plugin version and configuration snapshot; it never rewinds canonical
objects created by later runs.

Plugin migrations never receive SQL, filesystem or secret-store handles. The
plugin supplies bounded old/new schemas plus a declarative or sandboxed pure
payload transform. The kernel validates every transformed payload, writes new
revisions through its normal transaction/effect journal and records the old
revision. Preflight is read-only; apply requires a backup and owner review;
interruption is resumable. Rollback selects the retained plugin and
configuration but does not pretend that later canonical objects never existed.
If a new payload cannot be read by the old version, downgrade is blocked and
restore-from-backup is offered explicitly.

Connector and filesystem effects use kernel-issued effect keys. The kernel
records intent and receipt around each call; a plugin cannot claim retry safety
without a conformance fixture for success, timeout-before-effect,
timeout-after-effect and crash-before-commit.

Uninstall is blocked while an active run pins the plugin. Historical runs keep
manifest and schema snapshots sufficient for inspection.

## Conformance suite

Every published plugin must pass:

- manifest and deterministic-package validation;
- schema boundary and fuzz tests;
- declared/observed capability equality;
- no path escape or undeclared network access;
- idempotent retry contract where declared;
- cancellation and timeout quiescence;
- trace completeness and secret redaction;
- migration forward, rollback and interrupted-migration recovery;
- signer revocation and trust-store downgrade behavior;
- effect intent/receipt recovery for an ambiguous external timeout;
- protected-write review enforcement;
- disable/uninstall behavior with historical objects;
- UI contribution empty, loading, error and narrow-window rendering.
