# Generic Portable Bundle Fixture

This integration is an independent Node.js consumer for `memory-harness.bundle/v1alpha1`.
It intentionally does **not** link the MemoryOS Go portable-bundle implementation.

Its purpose is interoperability testing, not a second canonical memory store. The fixture:

- validates tar paths, entry sizes, bundle hash, blob hash, Object revision hash and Evidence hash;
- reports missing capabilities and unmapped Object types explicitly;
- treats imported Evidence as `quarantine` and imported Objects as `candidate`;
- never expands permissions and never activates imported memory;
- blocks executable Prompt/Skill/Rule/Procedure injection signals;
- reconstructs a protocol bundle from its generic quarantine/candidate store.

Run `npm test` in this directory. The cross-implementation test executes a Go fixture to prove Go → Node → Go round-trip identity for real Growth artifacts.
