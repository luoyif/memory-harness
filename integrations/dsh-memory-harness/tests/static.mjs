import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const host = await readFile(new URL('../lib/index.js', import.meta.url), 'utf8')
const pkg = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'))
assert.equal(pkg.version, '0.2.0')
for (const marker of ['agent/pre-step', 'session/event', 'turn/end', '/v1/agent/context/handshake', '/v1/agent/context/plans', '/v1/agent/context/receipts', '/v1/agent/outcomes', '/v1/agent/recall', '/v1/agent/capture', 'projectBindings', 'idempotency_key', 'outbox-v1.json']) {
  assert.ok(host.includes(marker), `host missing ${marker}`)
}
console.log('Memory Harness DSH bridge static contracts PASS')
