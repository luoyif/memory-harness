# Contributing

Thank you for improving Memory Harness. Keep changes small enough to audit and
preserve the central boundary: immutable source Evidence is authoritative;
derived indexes, summaries, memories and assets must remain traceable and
rebuildable.

## Before opening a pull request

1. Use an isolated `--home`; never use a real personal memory directory in
   tests, screenshots or fixtures.
2. Do not commit model keys, Agent tokens, exported databases or personal
   Evidence.
3. Add tests for authorization, project isolation and provenance when those
   boundaries are touched.
4. Run:

   ```bash
   go test -count=1 ./...
   go vet ./...
   cd frontend
   npm ci
   npm run typecheck
   npm run lint
   npm test
   npm run build
   ```

5. Explain user-visible behavior and migration impact. Schema changes must be
   additive unless a separately reviewed migration proves otherwise.

## Design principles

- Basic mode should be understandable without memory-system terminology.
- Advanced features may be powerful, but should be inspectable and reversible.
- AI may propose; the Owner confirms protected state.
- MCP permissions and UI wording must match actual server enforcement.
- A successful import does not imply every derived stage succeeded; surface
  partial results and warnings explicitly.

Contributions are accepted under the Apache License 2.0.
