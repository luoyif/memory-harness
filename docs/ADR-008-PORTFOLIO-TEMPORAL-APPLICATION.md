# ADR-008: Project, Temporal and Business Application Layer

Status: accepted for implementation on 2026-08-21

## Context

MemoryOS M2 proves the traceable growth path from Evidence to Agent Assets, but
it does not yet provide the product boundaries needed for a complete personal
and project operating system. In particular, scope hints are not an authority,
search only covers raw Evidence, validity time is not queryable, and project
budgets are not structured records.

The design was checked against the mechanisms used by Mem0 (scoping,
extraction and hybrid retrieval), Graphiti (bi-temporal facts and episode
provenance), Letta (budgeted context blocks), Cognee (dataset-aware ingest and
recall), and LangMem (semantic, episodic and procedural memory). These are
mechanism references, not storage dependencies.

## Decision

1. Keep the Go single-binary, JSONL Evidence, SQLite control/search and
   embedded-web architecture. No vector or graph service is mandatory.
2. Add a project registry as the sole authority for project IDs. User text and
   scope hints may match aliases but can never become filesystem paths.
3. Keep one canonical Evidence object and associate it to projects through
   explicit `record_projects` links. Cross-project use never duplicates the raw
   source.
4. Add bi-temporal facts with `recorded_at`, `valid_from` and `valid_until`.
   Corrections append and supersede; they never destroy prior facts or Evidence.
5. Add a rebuildable unified FTS index across Evidence, memories, facts,
   projects, decisions, risks, goals and Agent Assets. Retrieval is project
   scoped by default and cross-project only when explicitly requested.
6. Add project operations and finance as typed records. Money is stored as
   signed integer minor units plus ISO-style currency code. Totals never mix
   currencies silently.
7. Add bounded context blocks as an inspectable compilation surface. Blocks
   reference canonical records and never become hidden write authority.
8. Add connector definitions and import batches with durable cursors and
   idempotency keys. Imported content still enters as Evidence before derived
   memory.
9. Preserve the existing review gate. Procedural/behavioral assets remain
   proposals until validation and policy approval.

## Failure and idempotency

- Stable IDs or explicit idempotency keys make project, fact, transaction and
  import operations safe to retry.
- A failed derived-index update does not roll back canonical data; Doctor and
  rebuild repair derived search state.
- Project resolution fails closed to the Inbox project when no confident match
  exists. It never guesses a project path.
- Finance rejects zero amounts, invalid currencies and cross-project account
  references. Duplicate idempotency keys return the original transaction.

## Migration and rollback

- Schema `0.3` is an additive migration from M2. Existing tables and JSONL are
  unchanged; new records and associations are added in new tables.
- Existing records without a project association are linked to the built-in
  Inbox project without rewriting source Evidence.
- Search data remains disposable. Removing `cache/search.sqlite` and running
  rebuild recreates both Evidence and unified documents.
- Rollback to M2 remains possible after copying the data home: M2 ignores the
  new tables, while all pre-existing records remain readable.

## Acceptance contract

- project isolation and explicit portfolio search;
- point-in-time fact queries and retained superseded history;
- goal, milestone, decision and risk CRUD with source links;
- per-project budgets, accounts and balanced currency summaries;
- idempotent connector/import batches;
- unified provenance-bearing search;
- consistent export containing JSONL, living Markdown and a SQLite snapshot;
- restart recovery, search rebuild, race tests and browser end-to-end checks.

