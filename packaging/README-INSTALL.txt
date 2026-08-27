Memory Harness 2.2.0 Public Preview / 公开预览版

IMPORTANT / 重要说明
- This public preview is open source but is not Developer ID/notarized or
  Authenticode-signed. Verify SHA256SUMS.txt before running it.
- 本公开预览版已开源，但尚未使用 Apple Developer ID、公证或 Windows
  Authenticode 签名。运行前请先核对 SHA256SUMS.txt。
- The platform ZIP is the complete download: desktop installer + memoryosd MCP
  companion + this guide + checksums.
- 平台 ZIP 才是完整交付：桌面安装程序 + memoryosd MCP 伴侣 + 本说明 + 哈希。

macOS Intel / Apple Silicon
1. Open Memory-Harness-2.2.0-macos-universal.dmg and drag Memory Harness to
   Applications.
2. If macOS blocks the unsigned preview, use Finder to right-click the app and
   choose Open, then confirm. Do not disable system-wide security protection.
3. Keep the included memoryosd file in a stable folder if you need Codex/MCP.
   Its binary contains both x86_64 and arm64 slices.
4. Intel macOS has been runtime-tested. The Apple Silicon slices and bundle
   structure are verified, but this release round has no real M-series runtime
   evidence.

Windows 11 x64
1. Run Memory-Harness-2.2.0-windows-x64-setup.exe.
2. SmartScreen may show Unknown publisher because the preview is unsigned.
   Continue only after the SHA-256 value matches SHA256SUMS.txt.
3. Keep memoryosd.exe in a stable folder if you need Codex/MCP.

First use / 第一次使用
1. Open Memory Harness and create a memory space on the Home page.
2. Import one conversation or document.
3. Process only new or failed source material.
4. Review AI suggestions, then search and open exact source Evidence.
5. The default data home is Documents/Knowledge/MemoryOS for the current user.

Codex / MCP
1. Start the companion locally:
     memoryosd start --addr 127.0.0.1:19777
2. In Memory Harness, open 连接与健康, create a separate Codex Agent, select
   its projects and permissions, and save the one-time token.
3. Configure the MCP client with the companion's absolute path, these arguments:
     mcp --endpoint http://127.0.0.1:19777
   and the secret environment variable:
     MEMORYOS_AGENT_TOKEN=<one-time-token>
4. Never write the token into source code, documentation, logs, prompts or
   captured Evidence.

RAG and embeddings / 检索与向量
- Recall combines SQLite FTS5 BM25, a deterministic local 384-dimensional
  feature embedding, temporal relevance and recency.
- The local embedding does not download a model or send content externally. It
  is rebuildable and does not require a vector database. It is not presented as
  a hosted semantic embedding model.

Security / 安全
- HTTP and MCP listen on loopback only by default.
- Model keys use supported system secret storage or explicit secret references.
- Installers contain no API key, Agent token, test database or personal Evidence.
- Back up the data home before upgrades. Do not use a real data home for tests.

Source, documentation and issues:
https://github.com/luoyif/memory-harness
