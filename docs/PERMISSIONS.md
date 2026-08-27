# Agent identity and permission model

MemoryOS treats every external AI runtime as a separate principal. Codex, Claude Code, DeepSeek Harness, OpenClaw and a custom MCP client must not share a token unless the owner deliberately wants them to share one identity and one audit trail.

## Enforcement order

Every governed request must pass all three checks:

1. the Bearer token hashes to an active Agent;
2. the Agent holds the named capability permission;
3. the target project is in `project_ids`, or `all_projects` was explicitly granted.

Exact Evidence reads add a fourth check: the Evidence record must be linked to an allowed project. Source Evidence attached to a new goal, decision, risk or finance entry must belong to the same target project.

## Permission matrix

| Permission | Allows | Does not allow |
|---|---|---|
| `memory.read` | Evidence search/read and governed recall | finance results, capture, review approval |
| `memory.capture` | append idempotent Evidence and run distillation | edit/delete Evidence, approve protected memory |
| `memory.propose` | propose an immutable template-governed V4 or compatible V3 Agent-asset revision with optimistic locking | approve/activate the revision, edit ordinary memory objects, bypass server validation |
| `project.read` | visible project summaries and bounded context | finance details without `finance.read`, project writes |
| `project.write` | create goals, decisions and risks | finance writes, status review approval |
| `finance.read` | project budgets, finance summaries and finance recall | finance writes |
| `finance.write` | append idempotent finance entries | reading historical finance without `finance.read` |
| `trace.read` | list/read project-authorized Runs, stages, events and redacted effects | retry, fork, cancel or mutate Runs |

An Agent with `memory.read` but not `finance.read` receives no finance hits when recall kinds are omitted. An explicit `kinds:["finance"]` request is rejected with `403`. Project summaries return `budget_minor:0`, empty currency totals and no finance entries.

## Credentials

- Tokens use the `mos_` prefix and 256 bits of random material.
- The plaintext token is returned only by create or rotate.
- SQLite stores only its SHA-256 hash.
- Rotation invalidates the previous token immediately.
- Setting `status:"disabled"` rejects the token immediately without deleting the audit history.
- Do not put tokens in source control, Markdown memory, Agent prompts or browser local storage. Configure them as the MCP client's secret environment variable `MEMORYOS_AGENT_TOKEN`.

## Audit and non-delegable authority

Allowed, denied and failed governed operations are written to `agent_audit_log` with Agent ID, action, resource, project, status, time and bounded detail JSON. The UI shows recent events; `GET /v1/agent-audit` supports deeper inspection.

External Agents cannot call the human review endpoint through the Agent API or MCP. `memory.propose` is deliberately non-delegable beyond candidate creation: it requires `expected_revision` and `edit_reason`, server-side validation is authoritative, and a successful proposal leaves the current revision unchanged. Identity-core, procedure, correction and protected asset candidates remain under the local owner review gate. There is no permission that grants an Agent the right to self-approve them.
