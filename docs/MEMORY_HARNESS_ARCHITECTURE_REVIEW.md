# Memory Harness Architecture Review Record

Status: review completed with required changes; owner decision pending

## Review target

- `docs/ADR-009-MEMORY-HARNESS-PLATFORM.md`
- `docs/MEMORY_HARNESS_PRODUCT_SPEC.md`
- `docs/MEMORY_HARNESS_PLUGIN_SPEC.md`
- `docs/MEMORY_HARNESS_TRACE_SPEC.md`
- `docs/MEMORY_HARNESS_UI_SPEC.md`
- `docs/MEMORY_HARNESS_DELIVERY_PLAN.md`

## Review method

The review has three independent parts:

1. local consistency and contract checks;
2. adversarial external review through the user-authorized signed-in ChatGPT
   web interface;
3. final evidence-based disposition by the implementer.

External suggestions are not accepted by authority or popularity. Each material
claim must be checked against the current code, a primary specification, an
explicit product invariant or an executable test. Suggestions outside the
authorized local-first, independent-product scope are rejected or deferred with
a reason.

## Local baseline evidence

On 2026-08-21, before adding implementation code:

- `go test -count=1 ./...`: PASS;
- `go vet ./...`: PASS;
- `go test -race ./...`: PASS;
- `node --check internal/server/ui/app.js`: PASS;
- architecture documents: created as proposed, not represented as implemented;
- production data home: not used or mutated by this review.

## Questions for external review

1. Which kernel/plugin boundaries are likely to become unstable or create a
   distributed monolith?
2. Is the canonical-data model unambiguous for Evidence, generic objects,
   Living Assets and projections?
3. Can a plugin add a new memory layer without kernel/schema/UI changes?
4. Which capability, review or sandbox paths permit privilege escalation?
5. Does the run/trace model distinguish execution evidence from memory and
   support crash-safe retry without claiming deterministic LLM replay?
6. Is optional DSH integration sufficiently decoupled from standalone use?
7. What migration or rollback failure modes are missing?
8. Does the UI expose all important layers and trajectories without forcing
   normal users into workflow-engine complexity?
9. Is Wails v2 plus Go Core and compiled React a coherent packaging decision?
10. Which acceptance criteria are not objectively testable?

## External review result

The user-authorized review was run in the signed-in ChatGPT web application.
Only absolute local paths were sent; no files were uploaded or pasted. The
reviewer used Remote Desktop Commander in read-only mode to inspect the six
documents, the MemoryOS worktree and the available DeepSeekHarness directory.

Review conversation: `https://chatgpt.com/c/6a87fdea-d33c-83ea-915a-e9075e37aa3f`.

The reviewer completed the source/document audit and reported visible findings
before its final required-format message was interrupted. The interruption is
not treated as acceptance. Each captured material finding below was checked
again against local source. The evidence-backed direction was: the independent
Harness architecture is viable, but the pre-review text was not safe to
implement without required changes.

Verified review facts:

- MemoryOS had 17 modified tracked files plus untracked v1.1 and architecture
  work; the review documents were proposals, not implemented capabilities;
- the available DeepSeekHarness path was not a Git repository and did not
  contain a verifiable DSH core checkout; it did contain local Knowledge Hub
  and usage plugins, LLM Wiki experiments and backups;
- current owner-facing `/v1/...` administration routes were registered without
  a distinct owner authentication boundary; protected review accepted an empty
  reviewer as `local-user`;
- the non-macOS default model secret store was process memory, while the model
  configuration response always named `macOS Keychain`;
- existing Go test, vet, race and JavaScript syntax checks passed, but they do
  not test the target generic plugin runtime, universal trace, Wails package or
  migration rollback;
- the current fixed six-layer/tabled implementation, the proposed generic
  Object Store, Living Asset file ownership, plugin migration rollback and
  cross-medium effect idempotency required explicit transition contracts;
- the existing DSH Knowledge Hub file-writing behavior cannot become a second
  canonical write path after Memory Harness cutover.

Implementer verdict: **ACCEPT WITH REQUIRED CHANGES**.

## Blocking findings and disposition

### B1 — Owner and Agent control planes were conflated

Severity: critical.

Failure scenario: any local web page or process that can reach the loopback
port invokes Agent creation, grant rotation, model credential mutation or a
protected review as the implicit owner.

Evidence: `internal/server/http.go`, `internal/server/agent_api.go`,
`internal/server/model_api.go`, `internal/server/memory_api.go`, and the
`local-user` fallback in `internal/memory/engine.go`.

Disposition: accepted and corrected in the ADR, product, UI and delivery
contracts. Owner sessions, Origin/CSRF checks and negative tests are now release
requirements. This is a target correction; current v1.1 code remains exposed
until implementation.

### B2 — Credential-store claim was platform-inaccurate

Severity: high.

Failure scenario: a Windows/Linux build reports a secure OS store while the
secret disappears on restart or remains only in process memory.

Evidence: `internal/modelconfig/secret_other.go` uses `MemorySecretStore` and
`internal/server/model_api.go` always reports `macOS Keychain`.

Disposition: accepted. Every claimed platform must implement and accurately
name a persistent secure store; volatile fallback is not release-qualified.

### B3 — Living Asset ownership could create two authorities

Severity: high.

Failure scenario: Object Store lifecycle points to revision A while an edited
Markdown file appears to be revision B, allowing UI, search and Agent recall to
disagree.

Disposition: accepted. Object Store is now the sole identity/revision/lifecycle
authority. Vault files are content bodies or exports; external edits re-enter
as Evidence and a proposed revision.

### B4 — Cross-store effects lacked an ambiguous-outcome contract

Severity: high.

Failure scenario: a connector or filesystem effect succeeds, the process dies
before commit, and retry duplicates the remote effect or canonical object.

Disposition: accepted. The trace contract now defines intent, dispatch,
receipt, reconciliation and exactly-once canonical materialization without
claiming exactly-once external execution.

### B5 — Plugin migration and package trust were underspecified

Severity: high.

Failure scenario: a signed but untrusted package receives direct migration
access, rewrites canonical rows, or cannot be downgraded without silent loss.

Disposition: accepted. Signer trust/revocation, kernel-mediated pure payload
transforms, backup, resume and explicit downgrade blocking are now contractual.

### B6 — WASM feasibility was assumed rather than proven

Severity: medium/high.

Failure scenario: the public plugin API promises sandboxed execution before the
chosen runtime can prove bounded resource use, cancellation or portable host
calls.

Disposition: accepted as a spike gate. A failed spike narrows the first release
to declarative packages and governed external adapters.

### B7 — DSH compatibility could not be verified from the supplied path

Severity: high for the DSH claim, non-blocking for standalone operation.

Failure scenario: the bridge is designed from a local plugin snapshot against
an inferred DSH protocol and breaks after installation or upgrade.

Disposition: accepted. DSH remains optional/experimental until a real core
version or published contract passes pinned compatibility fixtures.

### B8 — The dirty v1.1 baseline obscured migration lineage

Severity: high for implementation control.

Failure scenario: generic-store migration changes are mixed with existing
uncommitted Agent/model/MCP/UI work, so rollback cannot identify which state to
restore.

Disposition: accepted. Phase 0 now requires a baseline patch/manifest, fixture,
counts and hashes before implementation. Existing user work must not be
discarded.

## Important non-blocking findings

- The frontend scope is large. React/TypeScript plus Wails is coherent, but a
  packaging spike must prove lifecycle, WebView owner identity and clean-machine
  installation before the application shell is treated as solved.
- Pipeline Studio must preserve a simple guided mode and an accessible list
  editor; a canvas alone would fail the non-technical-user requirement.
- Opportunity and finance remain first-party domain plugins/modules, not
  kernel concepts. Their polished default experience must not weaken this
  boundary.
- Passing current tests proves the current candidate is internally healthy; it
  does not validate target architecture claims.

## Contradiction matrix

| Topic | Pre-review ambiguity/code fact | Resolved contract |
| --- | --- | --- |
| Owner identity | Loopback access acted as owner | Short-lived desktop owner session; Agent tokens never inherit it |
| Living Assets | Object row and Markdown path could both appear current | Object Store owns revision/lifecycle; Vault holds addressed content/export |
| Model secrets | All UI responses said macOS Keychain | Report the actual platform store; volatile storage fails release |
| Plugin migration | Plugin-owned migration functions vs no direct store access | Kernel runs bounded pure transforms and writes revisions |
| External effects | Retry existed without an unknown-outcome receipt | Intent/receipt/reconciliation with unique canonical commit |
| DSH support | Adapter design inferred from local plugin directory | Pin a real protocol/core version before advertising support |
| Extensible layers | Current Go/SQL/JS encode six layers | Compatibility projection first; generic registry must pass no-kernel-change test |

## Missing objective tests added to the plan

1. Open the loopback UI in an ordinary browser; owner mutations return denial
   and create no object/grant/secret change.
2. Send an Agent bearer token to each owner-only route; every request is denied.
3. Crash after a remote effect and before receipt/commit; recovery exposes an
   unknown effect and creates at most one canonical object.
4. Edit a Vault export outside the application; canonical revision does not
   change until the edit is captured as Evidence and reviewed.
5. Revoke a plugin signer while runs and historical traces exist; new runs stop
   while history remains inspectable.
6. Interrupt a plugin payload migration; restart resumes or restores without
   partially visible revisions.
7. Install a declarative custom memory type; it appears in Memory Map and can
   materialize without kernel source, SQL or hard-coded UI edits.
8. Run the DSH bridge against every advertised pinned version, including
   disconnect and protocol-drift fixtures; an unverified version is rejected or
   labeled experimental.
9. Install the signed desktop build on a clean machine; owner setup, Agent
   connection, restart, backup and restore require no terminal.
10. Restart each claimed platform build; configured model secrets remain in the
    named secure store and never appear in UI storage, logs or exports.

## Disposition

1. **Fixed before implementation:** owner control plane, Living Asset
   authority, effect receipts, plugin migration/trust and accurate secret-store
   contract.
2. **Validate through Phase 0 spikes:** Wails owner identity/packaging, WASM
   sandbox feasibility and DSH protocol boundary.
3. **Preserve with compatibility layer:** current fixed six-layer APIs and
   existing v1.1 data until reconciliation tests pass.
4. **Defer with guardrail:** public marketplace, automatic remote plugin trust,
   mandatory graph/vector services and unsupported DSH versions.
5. **Reject:** direct plugin database/filesystem writes, DSH/LLM Wiki as a
   second canonical authority, unauthenticated owner mutations and claims of
   deterministic LLM or exactly-once remote execution.

## Final decision

The architecture direction is approved by the implementer as **ACCEPT WITH
REQUIRED CHANGES**, and the documents now contain those changes. ADR-009 remains
proposed until the owner reviews this record. Product implementation must not
start before that decision and the Phase 0 baseline/spike gates. No target
Harness capability is represented as implemented by this review.
