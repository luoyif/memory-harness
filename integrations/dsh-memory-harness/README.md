# DeepSeek Harness Bridge v0.2

This first-party Cordis plugin connects DeepSeek Harness to the independent Memory Harness application. Memory Harness remains the sole long-term memory authority; DSH owns the actual prompt/tool lifecycle.

## Rich Adapter path

Before a top-level Agent step the bridge now prefers the governed FT2 path:

1. negotiate a vendor-neutral Context capability set;
2. request a project-scoped Context Plan;
3. read each pinned Object/Evidence item through Agent read APIs;
4. verify Object revision/content hash or Evidence SHA-256;
5. place only successfully verified content into the DSH step;
6. record a Context Receipt for every planned item;
7. after a completed top-level turn, report `turn_completed` as Outcome observation only.

A Context Plan never proves delivery or model use. A completed DSH turn is not treated as task success. Outcome never activates Memory, Assets or Blueprints.

## Legacy and capture paths

If Rich Adapter negotiation or Context Plan fails, the plugin can fall back to the existing project-scoped `/v1/agent/recall` path. After a completed top-level turn it can also queue the latest human exchange as immutable Evidence. Capture and Outcome retries share the credential-free local outbox.
## Required Agent permissions

For the full v0.2 path grant only the bound projects plus:

- `memory.read` — resolve Plan Object/Evidence content and legacy recall;
- `memory.capture` — optional completed-turn Evidence capture;
- `context.plan` — capability handshake and Context Plan compilation;
- `context.receipt` — delivery/trim/failure receipt recording;
- `outcome.report` — completed-turn observation.

The plugin never receives an Owner session and has no approval/activation capability. If only `memory.read`/`memory.capture` are granted, v0.2 safely degrades to the v0.1 recall/capture behavior.

## Install and configure

Register this package as a DSH bundle, then bind each DSH Workspace to an explicit Memory Harness project. An unbound workspace fails closed and performs no recall, context injection or capture.

```yaml
- insert:
    - id: memory-harness-bridge
      name: dsh-memory-harness-bridge
      config:
        baseUrl: http://127.0.0.1:19777
        credentialRef: MEMORYOS_AGENT_TOKEN
        defaultProjectId: ""
        projectBindings:
          "<dsh-workspace-id>": "<memory-harness-project-id>"        richContextEnabled: true
        outcomeEnabled: true
        recallEnabled: true
        captureEnabled: true
        recallLimit: 6
        richContextMaxItems: 8
        richContextMaxTokens: 4096
        richContextMaxChars: 12000
```

Unsent Evidence captures and Outcome observations are retained in `~/.dsh/memory-harness-bridge/outbox-v1.json` without credentials and retried on the next event or startup. Rich context injection itself requires a successful Receipt; a Receipt failure falls back instead of silently injecting untracked context.

Compatibility remains pinned to the DSH Cordis seams verified locally against `dsh-knowledge-hub` 0.4.1: `agent/pre-step`, `session/event`, `sessionQuery.readSurface`, `workspaceRegistry`, `credentials`, and `systemPrompt.context`.

Static/unit contract tests are necessary but are not called a live DSH runtime pass. Live acceptance must exercise the installed Cordis bundle against a running Memory Harness endpoint and verify the recorded Context Plan/Receipt/Outcome Run trace.
