# Model configuration and Memory Agent

MemoryOS starts in `rules` mode. This path is local, deterministic and requires no provider. The optional `agent` mode uses an OpenAI-compatible chat-completions API for knowledge-candidate extraction while keeping canonical Evidence, validation, policy and review local.

## Supported configurations

- OpenAI preset: `https://api.openai.com/v1`;
- DeepSeek preset: `https://api.deepseek.com/v1`;
- OpenAI-compatible or local provider, including loopback HTTP endpoints.

Non-loopback providers must use HTTPS. Provider URLs cannot contain credentials, query parameters or fragments. Remote providers require an API key. Loopback providers may omit it.

## Secret boundary

On macOS, API keys are stored as generic passwords in the system Keychain under a data-root-specific service name. SQLite stores only `has_secret`; list/config responses never include the key. Editing a provider with a blank `api_key` preserves the current key. `clear_api_key:true` deletes it. Disabling an active provider or removing a required key automatically returns the runtime to rules mode.

## Data sent to the provider

Only the Evidence turns of the session currently being distilled are sent. Long Evidence is split on sentence boundaries into bounded chunks before model calls, with at most three chunks processed concurrently, so a recording is not submitted as one oversized prompt and silently reduced to raw sentence splitting. Each chunk is asked for a small high-precision set rather than a sentence-by-sentence rewrite. Successful chunks are retained if another chunk times out; the Episode compiler records a `partial-X-of-Y` suffix so the incomplete run remains visible. The system instruction declares that Evidence text is untrusted data, asks for JSON candidates, requires the primary language of the Evidence and one exact `evidence_id` per claim. Existing project-wide memory, finance, credentials and unrelated sessions are not inserted into this request.

The provider receives:

- selected model name;
- session ID;
- Evidence ID, source, role, visible text, scopes and observed time for the current session.

Do not enable Agent mode for a provider whose privacy terms are unsuitable for that Evidence.

## Local validation and fallback

Provider output is not trusted. The memory engine checks:

- compatible JSON shape and bounded candidate count;
- exact Evidence ID membership in the current session;
- statement length, unit type, tier, risk and confidence range;
- standalone meaning after filler, false starts, repetitions and vague references are removed;
- procedure, identity and correction candidates use risk C or D;
- every accepted candidate retains its source Evidence scope and time.

Malformed output, a timeout, HTTP error, missing credential or a policy violation uses `rules-v2/fallback`. This fallback intentionally keeps only explicit goals, decisions, risks, outcomes, stable identity/preferences and procedures; ordinary spoken sentences do not become durable knowledge. Canonical Evidence capture remains successful and inspectable. Rules fallback stays enabled; strict fail-closed distillation is intentionally not offered.

MiniMax providers receive their supported `reasoning_split` option so the final JSON can be decoded without mixing the reasoning stream into the answer. That vendor-specific field is not sent to OpenAI, DeepSeek or arbitrary compatible endpoints.

## Configuration workflow

1. Open **AI 控制中心**.
2. Add a provider or choose a preset.
3. Save the model and optional API key.
4. Run **检测连接** to authenticate and inspect `/models`.
5. Select the provider in the runtime bar to enable Agent mode.
6. Import or capture a new session and inspect its Episode compiler and source-linked Knowledge Units.

For an older Episode that was compiled with a previous strategy, open **记忆库 → 会话复盘** and choose **用当前模型重新沉淀**. This operation preserves the Evidence ledger, replaces only rebuildable knowledge/memory projections, increments the Episode revision and creates a new inspectable Harness Run.

The connection test proves compatible authentication and model listing. The automated acceptance suite separately exercises `/chat/completions`, schema validation and deterministic failure fallback against a controlled provider.
