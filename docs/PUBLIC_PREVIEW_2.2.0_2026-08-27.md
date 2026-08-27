# Memory Harness 2.2.0 Public Preview

Date: 2026-08-27  
License: Apache-2.0  
Public repository: <https://github.com/luoyif/memory-harness>  
Release tag: `v2.2.0`

## Release decision

2.2.0 is the first open-source public preview. It is suitable for source use,
controlled local evaluation, demos and laboratory deployment on Windows x64
and macOS. It is not described as a fully signed consumer release: the macOS
artifact uses an ad-hoc signature and is not notarized; the Windows installer
is not Authenticode-signed.

The platform ZIP is the primary download because it contains both the desktop
installer and the matching `memoryosd` MCP companion. A desktop-only installer
does not install the MCP command into `PATH`.

## RAG and embedding boundary

Unified recall now combines:

- SQLite FTS5 `unicode61` BM25 for Latin text and code;
- SQLite FTS5 trigram BM25 for Chinese;
- a deterministic, local `local-feature-hash-v1` embedding with 384 dimensions;
- temporal relevance and recency through reciprocal-rank fusion.

Existing indexed documents receive the embedding projection on first open.
Every document update rewrites FTS and embedding projections in one database
transaction. Doctor checks document/FTS/embedding row counts, algorithm,
dimensions and payload length.

The local projection is dependency-free and never calls an external service.
It handles bounded word-form, spelling and local text similarity; it is not
presented as a hosted semantic embedding model. A tested minimum-similarity
gate preserves useful `reconnect`/`reconnection` retrieval while rejecting a
known generic-suffix false match. Candidate work remains bounded at 600 in the
1,000-object Context Plan performance test.

## Verification completed

| Gate | Result |
| --- | --- |
| Go full tests | PASS |
| Go Vet | PASS |
| Go full Race | PASS; server package 223.363s |
| Go reachable-vulnerability scan | PASS; 0 |
| Frontend TypeScript and ESLint | PASS; 0 warnings |
| Frontend Vitest | PASS; 25 files / 68 tests |
| Frontend production build | PASS; 1,766 modules |
| Production npm audit | PASS; 0 vulnerabilities |
| Full Git history secret scan | 9 findings manually confirmed as synthetic idempotency test keys; no real credential |
| Fresh platform ZIP extraction and internal hashes | PASS |
| Intel macOS launch from read-only DMG | PASS; health `ok`, unauthenticated Owner API 401 |
| Windows 11 x64 install, logged-in-user launch and uninstall | PASS; health `ok`, unauthenticated Owner API 401 |
| Final release MCP companion Doctor | PASS on macOS and Windows; 384-dimensional embedding projection consistent |
| Real `memoryosd start` + stdio MCP Codex loop | PASS; project/context read, immutable Evidence write, 2 knowledge units, Episode, exact readback, private draft, direct share, 9 audit events |

The Codex stdio acceptance used an isolated temporary data home and a token
passed only through the child-process environment. It did not write a token to
source, logs, documentation or Evidence.

## Final artifact hashes

```text
caa8931e5ea91cf8e2962b568f76a7831e1e57775c9b3eb8cc98236e4d62d518  Memory-Harness-2.2.0-macos-universal.dmg
942b84cfa0e9396e563ff8830c1180b0611e0b0f92afab6e9d8bed8ecebf2cde  Memory-Harness-2.2.0-macos-universal.zip
2a0244b86fe40a328157c4c51398f58ca93b18402ba84d68df3a805239b0bec1  Memory-Harness-2.2.0-windows-x64-setup.exe
f8d0c3cd3983383d2a153bf264cf8f0d01e36c415e33e6fbe70095fc7d8ff0ec  Memory-Harness-2.2.0-windows-x64.zip
05c8f2361a028a92e17d24467aa8ecf4bdf8546849d3c2bc81079cb4a2d8990b  Memory-Harness-MCP-2.2.0-macos-universal
b06ad18c84d42f58fe5e606420ba0c25dfbecae66f132c452119e5c637cecd12  Memory-Harness-MCP-2.2.0-windows-x64.exe
273ee622f975f0d72d20658694be1da46e54de48ac06b1c4eb2fe9b5146276e5  README-INSTALL.txt
```

## Explicit remaining boundaries

- The macOS executable and MCP companion contain `x86_64` and `arm64` slices,
  but this release round has no real Apple Silicon machine runtime evidence.
- No Apple Developer ID, notarization or Windows Authenticode certificate was
  available. Expected platform warnings are documented rather than hidden.
- HTTP/MCP remains loopback-only by default. This release is not an Internet-
  exposed hosted service.
- The built-in embedding is a local feature projection, not a downloaded neural
  embedding model. README and API responses identify its exact algorithm.
