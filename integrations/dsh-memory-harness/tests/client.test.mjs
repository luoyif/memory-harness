import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import test from 'node:test'

import { Config, MemoryHarnessClient, projectForWorkspace, renderLatestExchange } from '../lib/index.js'

test('client uses bearer Agent token and project-scoped payloads', async (t) => {
  const received = []
  const server = createServer(async (request, response) => {
    let body = ''
    for await (const chunk of request) body += chunk
    received.push({ path: request.url, authorization: request.headers.authorization, body: JSON.parse(body) })
    response.setHeader('content-type', 'application/json')
    response.end(JSON.stringify(request.url.endsWith('/recall') ? { hits: [{ title: 'Decision', kind: 'decision', source_id: 'd1', status: 'active', snippet: 'Use explicit review.' }] } : { evidence_id: 'ev1' }))
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => server.close())
  const { port } = server.address()
  const client = new MemoryHarnessClient(`http://127.0.0.1:${port}`, async () => 'secret-agent-token')
  await client.recall('project-alpha', 'review policy', 4)
  await client.capture({ project_id: 'project-alpha', idempotency_key: 'stable-1', source_system: 'deepseek-harness', session_id: 's1', text: 'done' })
  assert.deepEqual(received.map(item => item.path), ['/v1/agent/recall', '/v1/agent/capture'])
  assert.ok(received.every(item => item.authorization === 'Bearer secret-agent-token'))
  assert.ok(received.every(item => item.body.project_id === 'project-alpha'))
  assert.equal(JSON.stringify(received).includes('secret-agent-token'), true)
})

test('configuration rejects insecure remote HTTP and mapping fails closed', () => {
  assert.throws(() => Config['~standard'].validate({ baseUrl: 'http://example.com:19777' }), /HTTPS or loopback/)
  const ctx = { workspaceRegistry: { list: () => [{ id: 'ws-1', title: 'Alpha', path: '/work/alpha' }] } }
  const config = Config['~standard'].validate({ baseUrl: 'http://127.0.0.1:19777', projectBindings: { 'ws-1': 'project-alpha' } }).value
  assert.equal(projectForWorkspace(ctx, config, '/work/alpha/src').projectID, 'project-alpha')
  assert.equal(projectForWorkspace(ctx, config, '/work/beta').projectID, '')
})

test('latest exchange excludes older turns and tool output', () => {
  const snapshot = { events: [
    { type: 'user/message', data: { source: { kind: 'user' }, content: [{ type: 'text', text: 'old' }] } },
    { type: 'assistant/message', data: { message: { content: [{ type: 'text', text: 'old answer' }] } } },
    { type: 'user/message', data: { source: { kind: 'user' }, content: [{ type: 'text', text: 'new request' }] } },
    { type: 'tool/result', data: { message: { content: [{ type: 'text', text: 'private tool dump' }] } } },
    { type: 'assistant/message', data: { message: { content: [{ type: 'text', text: 'new answer' }] } } },
  ] }
  const text = renderLatestExchange(snapshot)
  assert.match(text, /new request/)
  assert.match(text, /new answer/)
  assert.doesNotMatch(text, /old answer|private tool dump/)
})
