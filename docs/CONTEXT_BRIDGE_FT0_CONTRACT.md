# FT0 Context Bridge Contract Freeze

Status: **FROZEN v1alpha1 contract baseline** (2026-08-22). This document defines interoperability semantics; it does not claim FT1 persistence/API is implemented.

## Authority boundary

Memory Core owns Evidence, Object/Revision, Project, Policy and Audit. An external Harness owns the actual prompt/context/tool lifecycle. The Context Bridge only proves what was planned and what the external Harness reports or proves happened; it is not a second canonical store.

The four vendor-neutral contracts are:

- `memory-harness.context-capability-set/v1alpha1`
- `memory-harness.context-plan/v1alpha1`
- `memory-harness.context-receipt/v1alpha1`
- `memory-harness.outcome-feedback/v1alpha1`

JSON Schemas live in `schemas/context/`; fixed positive fixtures live in `testdata/context/`; minimal runtime types and fail-closed validators live in `internal/contextbridge/`.

## Evidence states

These states are deliberately distinct:

`retrieved -> planned -> delivered -> placed -> used -> affected_outcome`

A later state is never inferred from an earlier state. In particular, a Plan proves only intent. If no trustworthy Receipt exists, every planned item remains `delivery_unverified`. A transport ACK is not proof of placement or use. Outcome feedback is an observation with evaluator/source/confidence, not reward truth and not permission to mutate Memory or Blueprint current revisions.

## Capability vocabulary

Current v1alpha1 vocabulary: `recall`, `capture`, `pre_turn_injection`, `thread_lifecycle`, `item_lifecycle`, `compaction_hook`, `approval_callback`, `context_receipt`, `outcome_feedback`. Vendor brand names are intentionally absent.

External runtime/protocol/thread/turn/item identifiers are optional correlation fields. They must never become local Object IDs or bypass local project scope.

## Scope, permissions and write boundary

All FT1 endpoints must authenticate through the existing Agent Principal and Project Grant. Existing tokens receive no new permission automatically. The opt-in permission vocabulary is:

| action | permission | effect |
| --- | --- | --- |
| compile a Context Plan | `context.plan` | read authorized current revisions and append Plan trace only |
| submit a Context Receipt | `context.receipt` | append receipt trace only; cannot alter source objects |
| report Outcome Feedback | `outcome.report` | append outcome trace/audit only; cannot trigger adaptation |

Owner-only Revision approval, grant expansion, plugin trust, restore, secrets and published Blueprint mutation remain non-delegable.

## Idempotency, hash and limits

Every Plan/Receipt/Outcome carries an `idempotency_key`. Context items pin source ID, Object revision when applicable, content hash, project, reasons and presentation. Receipt items must reference a planned item and may not report a different revision/hash as delivered.

Runtime validators cap a contract at 2 MiB and a Plan at 256 items. Capability handshake also declares item/total limits. Retention modes are `none|session|ttl|provider_policy`; redaction support is `none|supported|required|unknown`.

Hashes are deterministic SHA-256 of canonical Go-struct JSON with the self-hash field cleared. Hashes identify the contract payload; they do not prove the truth of external provider claims.

## Failure semantics

- unknown capability: reject;
- item outside plan project: reject;
- Object item without revision/hash: reject;
- receipt references unknown item: reject;
- receipt revision/hash mismatch: reject;
- `completeness=complete` but missing a plan item: reject;
- absent receipt: `delivery_unverified`, never `delivered`;
- malformed Outcome metric/confidence/cost: reject;
- external correlation missing: allowed, but no higher lifecycle evidence may be inferred.

## FT0 gate

FT0 is complete only when Go validators, schema JSON and fixtures pass tests and existing Agent permissions remain opt-in. FT1 may persist these contracts only as existing Harness Run/Event/StageOutput/Audit records; it must not create a second canonical Context database.
