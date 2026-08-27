import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'

import { exportImported, importBundle } from '../lib/consumer.js'

const repo = fileURLToPath(new URL('../../../', import.meta.url))
const helper = './integrations/portable-bundle-generic/go_fixture'

function go(...args) {
  return execFileSync('go', ['run', helper, ...args], { cwd: repo, encoding: 'utf8' }).trim()
}

test('Go -> generic Node client -> Go preserves protocol identity with explicit degradation', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'mh-go-node-'))
  const first = path.join(root, 'go.mhbundle.tar.gz')
  const second = path.join(root, 'node.mhbundle.tar.gz')
  const generated = JSON.parse(go('generate', first))
  assert.ok(generated.object_count >= 4)
  assert.equal(generated.evidence_count, 1)
  const store = importBundle(first, {
    capabilities: ['evidence:v1', 'plugin:builtin.living-asset-vault', 'object-type:builtin.living-asset-vault.knowledge-product.v1'],
    knownObjectTypes: ['builtin.living-asset-vault.knowledge-product.v1'],
    supportsPresentations: true,
  })
  assert.equal(store.objects.every(item => item.local_status === 'candidate'), true)
  assert.equal(store.evidence.every(item => item.local_status === 'quarantine'), true)
  assert.ok(store.report.missing_capabilities.length > 0)
  assert.ok(store.report.unmapped_object_types.length > 0)
  assert.deepEqual(store.report.permission_delta, [])

  exportImported(store, second)
  const before = JSON.parse(go('inspect', first))
  const after = JSON.parse(go('inspect', second))
  assert.equal(after.manifest.bundle_id, before.manifest.bundle_id)
  assert.equal(after.manifest.bundle_hash, before.manifest.bundle_hash)
  assert.deepEqual(after.objects, before.objects)
  assert.deepEqual(after.evidence, before.evidence)
})
