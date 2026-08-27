import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { createServer } from 'node:http'
import test from 'node:test'

import { MemoryHarnessClient } from '../lib/index.js'
import { capabilitySet, contextMessage, outcomePayload, receiptPayload, resolveContextPlan } from '../lib/rich-context.js'

test('rich context resolves only pinned object/evidence content and builds auditable receipt', async t => {
  const evidence = { evidence_id: 'ev-1', content: [{ type: 'text', text: 'Exact Evidence text.' }] }
  const evidenceRaw = JSON.stringify(evidence)
  const evidenceHash = createHash('sha256').update(evidenceRaw).digest('hex')
  const server = createServer((request, response) => {
    response.setHeader('content-type', 'application/json')
    if (request.url === '/v1/agent/objects/obj-profile') {
      response.end(JSON.stringify({ object_id: 'obj-profile', current_revision: 3, revision: { content_hash: 'profile-hash', payload: { view_kind: 'dynamic_project', title: 'Dynamic Project', blocks: [{ label: '当前目标', content: '完成 FT2 Rich Adapter。' }] } } }))
      return
    }
    if (request.url === '/v1/agent/evidence/ev-1') { response.end(`${evidenceRaw}\n`); return }
    response.statusCode = 404; response.end('{}')
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => server.close())
  const client = new MemoryHarnessClient(`http://127.0.0.1:${server.address().port}`, async () => 'agent-token')
  const plan = {
    plan_id: 'plan-1', project_id: 'project-alpha', budget: { max_chars: 12000 },
    items: [
      { item_id: 'item-profile', source_kind: 'object', source_id: 'obj-profile', revision: 3, content_hash: 'profile-hash', project_id: 'project-alpha', reason_codes: ['context_profile','profile:dynamic_project'], priority: 100, presentation: 'profile' },
      { item_id: 'item-evidence', source_kind: 'evidence', source_id: 'ev-1', content_hash: evidenceHash, project_id: 'project-alpha', reason_codes: ['unified_recall'], priority: 80, presentation: 'verbatim' },
    ],
  }
  const resolved = await resolveContextPlan(client, plan, 12000)
  assert.equal(resolved.length, 2)
  assert.ok(resolved.every(item => item.status === 'delivered'))
  assert.match(resolved[0].content, /完成 FT2 Rich Adapter/)
  assert.match(resolved[1].content, /Exact Evidence text/)
  const message = contextMessage('current task', plan, resolved)
  assert.match(message.content[0].text, /UNTRUSTED REFERENCE MATERIAL/)
  assert.match(message.content[0].text, /profile:dynamic_project/)
  const receipt = receiptPayload({ run_id: 'run-1', plan }, resolved, { external_thread_id: 'session-1', external_turn_id: 'turn-1' }, 'idem-1')
  assert.equal(receipt.run_id, 'run-1')
  assert.equal(receipt.receipt.completeness, 'complete')
  assert.deepEqual(receipt.receipt.items.map(item => item.status), ['delivered','delivered'])
  const outcome = outcomePayload({ projectID: 'project-alpha', runID: 'run-1', planID: 'plan-1', receiptID: receipt.receipt.receipt_id, turnID: 'turn-1', correlation: receipt.receipt.correlation, startedAt: Date.now() - 50 })
  assert.equal(outcome.metrics[0].name, 'turn_completed')
  assert.equal(outcome.metrics[0].value, true)
})