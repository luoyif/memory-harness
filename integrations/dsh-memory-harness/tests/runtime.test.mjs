import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { apply } from '../lib/index.js'

test('Cordis pre-step uses Plan/Receipt and turn-end reports Outcome without legacy recall', async t => {
  const requests = []
  const object = { object_id: 'obj-profile', current_revision: 2, revision: { content_hash: 'profile-hash', payload: { view_kind: 'dynamic_project', title: 'Dynamic Project', blocks: [{ label: '目标', content: '完成真实 Rich Adapter 验收。' }] } } }
  const server = createServer(async (request, response) => {
    let raw = ''
    for await (const chunk of request) raw += chunk
    const body = raw ? JSON.parse(raw) : undefined
    requests.push({ method: request.method, path: request.url, body, authorization: request.headers.authorization })
    response.setHeader('content-type', 'application/json')
    if (request.url === '/v1/agent/context/handshake') { response.end(JSON.stringify({ level: 'L5', capability_set_hash: 'caps-hash' })); return }
    if (request.url === '/v1/agent/context/plans') {
      response.end(JSON.stringify({ run_id: 'run-context-1', plan: { plan_id: 'plan-context-1', project_id: 'project-alpha', budget: { max_chars: 12000, max_tokens: 4096 }, items: [{ item_id: 'item-profile', source_kind: 'object', source_id: 'obj-profile', revision: 2, content_hash: 'profile-hash', project_id: 'project-alpha', reason_codes: ['context_profile','profile:dynamic_project'], priority: 100, presentation: 'profile' }] }, delivery_status: { 'item-profile': 'delivery_unverified' } }))
      return
    }
    if (request.url === '/v1/agent/objects/obj-profile') { response.end(JSON.stringify(object)); return }
    if (request.url === '/v1/agent/context/receipts') { response.end(JSON.stringify({ run_id: 'run-context-1', receipt: body.receipt, delivery_status: { 'item-profile': 'delivered' } })); return }
    if (request.url === '/v1/agent/outcomes') { response.end(JSON.stringify({ run_id: 'run-outcome-1', outcome: body })); return }
    if (request.url === '/v1/agent/capture') { response.end(JSON.stringify({ evidence_id: 'ev-captured' })); return }
    response.statusCode = 500; response.end(JSON.stringify({ error: { message: `unexpected ${request.method} ${request.url}` } }))
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => server.close())
  const stateDir = await mkdtemp(join(tmpdir(), 'mh-dsh-'))
  t.after(() => rm(stateDir, { recursive: true, force: true }))
  const handlers = new Map()
  const logs = []
  const snapshot = { session: { cwd: '/work/alpha' }, events: [
    { type: 'user/message', data: { source: { kind: 'user' }, content: [{ type: 'text', text: '继续完成 Rich Adapter。' }] } },
    { type: 'assistant/message', data: { message: { content: [{ type: 'text', text: '已经完成。' }] } } },
  ] }
  const ctx = {
    credentials: { resolve: async ref => ref === 'MEMORYOS_AGENT_TOKEN' ? { value: 'secret-agent-token' } : null },
    workspaceRegistry: { list: () => [{ id: 'ws-alpha', title: 'Alpha', path: '/work/alpha' }] },
    sessionQuery: { readSurface: async () => snapshot },
    systemPrompt: { context: () => {} },
    logger: { warn: message => logs.push(`WARN ${message}`), info: message => logs.push(`INFO ${message}`) },
    on: (event, handler) => { handlers.set(event, handler) },
  }
  await apply(ctx, {
    baseUrl: `http://127.0.0.1:${server.address().port}`,
    defaultProjectId: 'project-alpha', richContextEnabled: true, outcomeEnabled: true,
    recallEnabled: true, captureEnabled: true, statePath: join(stateDir, 'outbox.json'),
  })
  const preStep = handlers.get('agent/pre-step')
  assert.equal(typeof preStep, 'function')
  const signal = new AbortController().signal
  const decision = await preStep({
    agent: { session: { id: 'session-1', header: { cwd: '/work/alpha', origin: 'user' } } },
    messages: [{ id: 'turn-1', source: { kind: 'user' }, content: [{ type: 'text', text: '继续完成 Rich Adapter。' }] }],
    signal,
  }, async () => ({ kind: 'enter', messages: [] }))
  assert.equal(decision.kind, 'enter')
  assert.equal(decision.messages.length, 1)
  assert.match(decision.messages[0].content[0].text, /完成真实 Rich Adapter 验收/)
  assert.match(decision.messages[0].content[0].text, /Plan: plan-context-1/)
  const beforeTurnEnd = requests.map(item => item.path)
  assert.deepEqual(beforeTurnEnd.slice(0, 4), ['/v1/agent/context/handshake','/v1/agent/context/plans','/v1/agent/objects/obj-profile','/v1/agent/context/receipts'])
  assert.equal(beforeTurnEnd.includes('/v1/agent/recall'), false)
  assert.ok(requests.every(item => item.authorization === 'Bearer secret-agent-token'))
  const receipt = requests.find(item => item.path === '/v1/agent/context/receipts')?.body?.receipt
  assert.equal(receipt?.items?.[0]?.status, 'delivered')
  assert.equal(receipt?.items?.[0]?.content_hash, 'profile-hash')
  assert.equal(receipt?.correlation?.external_thread_id, 'session-1')
  assert.equal(receipt?.correlation?.external_turn_id, 'turn-1')

  const sessionEvent = handlers.get('session/event')
  assert.equal(typeof sessionEvent, 'function')
  sessionEvent({ id: 'session-1', header: { cwd: '/work/alpha', origin: 'user' } }, { type: 'turn/end', data: { reason: { kind: 'completed' } } })
  await new Promise(resolve => setTimeout(resolve, 800))
  const paths = requests.map(item => item.path)
  assert.ok(paths.includes('/v1/agent/outcomes'))
  assert.ok(paths.includes('/v1/agent/capture'))
  const outcome = requests.find(item => item.path === '/v1/agent/outcomes')?.body
  assert.equal(outcome?.metrics?.[0]?.name, 'turn_completed')
  assert.equal(outcome?.metrics?.[0]?.value, true)
  assert.equal(outcome?.plan_id, 'plan-context-1')
  assert.equal(outcome?.receipt_id, receipt.receipt_id)
  assert.equal(logs.some(line => line.includes('failed open')), false)
})
