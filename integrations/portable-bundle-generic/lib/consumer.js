import fs from 'node:fs'
import crypto from 'node:crypto'
import zlib from 'node:zlib'

export const SCHEMA = 'memory-harness.bundle/v1alpha1'
const hash = raw => crypto.createHash('sha256').update(raw).digest('hex')
const text = raw => raw.toString('utf8').replace(/\0.*$/s, '')
const pad512 = size => (512 - (size % 512)) % 512

function safeName(name) {
  return name && !name.startsWith('/') && !name.split('/').includes('..')
}

function parseOctal(raw) {
  const value = text(raw).trim()
  return value ? Number.parseInt(value, 8) : 0
}

export function readTarGzip(path) {
  const raw = zlib.gunzipSync(fs.readFileSync(path))
  const entries = new Map()
  let offset = 0
  while (offset + 512 <= raw.length) {
    const header = raw.subarray(offset, offset + 512)
    if (header.every(byte => byte === 0)) break
    const name = text(header.subarray(0, 100))
    const size = parseOctal(header.subarray(124, 136))
    if (!safeName(name)) throw new Error(`unsafe bundle entry ${name}`)
    if (size < 0 || size > 16 * 1024 * 1024) throw new Error(`entry too large ${name}`)
    const start = offset + 512
    const end = start + size
    if (end > raw.length) throw new Error(`short bundle entry ${name}`)
    if (entries.has(name)) throw new Error(`duplicate bundle entry ${name}`)
    entries.set(name, Buffer.from(raw.subarray(start, end)))
    offset = end + pad512(size)
  }
  return entries
}

function jsonLines(raw) {
  return raw.toString('utf8').split(/\r?\n/).filter(Boolean).map(line => JSON.parse(line))
}

function bundleDigest(objectsRaw, evidenceRaw, blobs) {
  const digest = crypto.createHash('sha256')
  digest.update(objectsRaw)
  digest.update(evidenceRaw)
  for (const key of [...blobs.keys()].sort()) {
    digest.update(key)
    digest.update(blobs.get(key))
  }
  return `sha256:${digest.digest('hex')}`
}

export function loadBundle(path) {
  const entries = readTarGzip(path)
  const manifestRaw = entries.get('bundle-manifest.json')
  const objectsRaw = entries.get('objects.jsonl')
  const evidenceRaw = entries.get('evidence.jsonl')
  if (!manifestRaw || !objectsRaw || !evidenceRaw) throw new Error('bundle core entries missing')
  const manifest = JSON.parse(manifestRaw)
  if (manifest.schema_version !== SCHEMA) throw new Error(`unsupported schema ${manifest.schema_version}`)
  const objects = jsonLines(objectsRaw)
  const evidence = jsonLines(evidenceRaw)
  const blobs = new Map()
  for (const [name, raw] of entries) {
    if (!name.startsWith('blobs/sha256/')) continue
    const key = name.slice('blobs/sha256/'.length).replace(/\.json$/, '')
    if (!/^[0-9a-f]{64}$/.test(key) || hash(raw) !== key) throw new Error(`blob checksum mismatch ${name}`)
    blobs.set(key, raw)
  }
  if (bundleDigest(objectsRaw, evidenceRaw, blobs) !== manifest.bundle_hash) throw new Error('bundle hash mismatch')
  if (objects.length !== manifest.object_count || evidence.length !== manifest.evidence_count) throw new Error('manifest counts mismatch')
  for (const object of objects) {
    for (const revision of object.revisions || []) {
      const blob = blobs.get(revision.blob_sha256)
      if (!blob || hash(blob) !== revision.content_hash) throw new Error(`object hash mismatch ${object.object_id}`)
    }
  }
  for (const item of evidence) {
    const blob = blobs.get(item.blob_sha256)
    if (!blob || hash(blob) !== item.line_hash) throw new Error(`evidence hash mismatch ${item.evidence_id}`)
  }
  return { manifest, objects, evidence, blobs, objectsRaw, evidenceRaw }
}

function executable(typeID) {
  return /agent-asset|prompt|skill|tool_recipe|mcp|procedure|procedural|constraint|rule/i.test(typeID || '')
}
function scan(raw, subject, isExecutable) {
  const value = raw.toString('utf8').toLowerCase()
  const signals = ['ignore previous instructions', 'override system', 'bypass owner review', 'disable safety']
  const findings = []
  for (const signal of signals) {
    if (!value.includes(signal)) continue
    findings.push({ severity: isExecutable ? 'blocked' : 'warning', code: 'instruction_injection_signal', subject, detail: signal })
    break
  }
  if (/(https?|file):\/\/|(?:\.\.\/){2,}/i.test(value)) {
    findings.push({ severity: isExecutable ? 'blocked' : 'warning', code: 'remote_or_path_reference', subject, detail: 'remote/file/path reference' })
  }
  return findings
}

export function compatibility(bundle, options = {}) {
  const capabilities = new Set(options.capabilities || [])
  const knownTypes = new Set(options.knownObjectTypes || [])
  const missing = (bundle.manifest.required_capabilities || []).filter(value => !capabilities.has(value))
  const unmapped = [...new Set(bundle.objects.map(item => item.type_id).filter(value => knownTypes.size && !knownTypes.has(value)))].sort()
  const findings = []
  for (const object of bundle.objects) {
    for (const revision of object.revisions || []) findings.push(...scan(bundle.blobs.get(revision.blob_sha256), `object:${object.object_id}@R${revision.revision}`, executable(object.type_id)))
  }
  for (const item of bundle.evidence) findings.push(...scan(bundle.blobs.get(item.blob_sha256), `evidence:${item.evidence_id}`, false))
  const blocked = findings.some(item => item.severity === 'blocked')
  const degradations = []
  if (unmapped.length) degradations.push(options.supportsPresentations ? 'unknown types are presentation-only' : 'unknown types remain generic candidates')
  if (missing.length) degradations.push('target lacks source capabilities')
  return {
    compatible: !blocked && missing.length === 0 && unmapped.length === 0,
    blocked,
    missing_capabilities: [...new Set(missing)].sort(),
    unmapped_object_types: unmapped,
    findings,
    degradations,
    permission_delta: [],
    import_mode: 'generic-quarantine+candidate-only',
  }
}

export function importBundle(path, options = {}) {
  const bundle = loadBundle(path)
  const report = compatibility(bundle, options)
  if (report.blocked) throw new Error('preflight blocked')
  return {
    manifest: bundle.manifest,
    report,
    evidence: bundle.evidence.map(item => ({ ...item, local_status: 'quarantine' })),
    objects: bundle.objects.map(item => ({ ...item, local_status: 'candidate' })),
    _bundle: bundle,
  }
}

function putOctal(buffer, offset, length, value) {
  const raw = value.toString(8).padStart(length - 1, '0') + '\0'
  buffer.write(raw, offset, length, 'ascii')
}
function tarHeader(name, size) {
  if (Buffer.byteLength(name) > 100) throw new Error(`tar path too long ${name}`)
  const header = Buffer.alloc(512)
  header.write(name, 0, 100, 'utf8')
  putOctal(header, 100, 8, 0o600)
  putOctal(header, 108, 8, 0)
  putOctal(header, 116, 8, 0)
  putOctal(header, 124, 12, size)
  putOctal(header, 136, 12, 0)
  header.fill(0x20, 148, 156)
  header.write('0', 156, 1, 'ascii')
  header.write('ustar\0', 257, 6, 'ascii')
  header.write('00', 263, 2, 'ascii')
  let sum = 0
  for (const byte of header) sum += byte
  const checksum = sum.toString(8).padStart(6, '0') + '\0 '
  header.write(checksum, 148, 8, 'ascii')
  return header
}

function tarEntry(name, raw) {
  const padding = Buffer.alloc(pad512(raw.length))
  return Buffer.concat([tarHeader(name, raw.length), raw, padding])
}

export function exportImported(store, outputPath) {
  const bundle = store._bundle
  if (!bundle) throw new Error('generic store has no imported bundle')
  const entries = []
  entries.push(tarEntry('bundle-manifest.json', Buffer.from(JSON.stringify(bundle.manifest, null, 2))))
  entries.push(tarEntry('objects.jsonl', bundle.objectsRaw))
  entries.push(tarEntry('evidence.jsonl', bundle.evidenceRaw))
  for (const key of [...bundle.blobs.keys()].sort()) {
    entries.push(tarEntry(`blobs/sha256/${key}.json`, bundle.blobs.get(key)))
  }
  entries.push(Buffer.alloc(1024))
  const tar = Buffer.concat(entries)
  fs.writeFileSync(outputPath, zlib.gzipSync(tar, { level: 9, mtime: 0 }))
  return loadBundle(outputPath)
}
export function writeProtocolBundle({ sourceProjectID = 'generic-project', objects = [], evidence = [], blobs = new Map(), requiredCapabilities = [], rootObjectIDs = [] }, outputPath) {
  const objectsRaw = Buffer.from(objects.map(item => JSON.stringify(item)).join('\n') + (objects.length ? '\n' : ''))
  const evidenceRaw = Buffer.from(evidence.map(item => JSON.stringify(item)).join('\n') + (evidence.length ? '\n' : ''))
  const bundleHash = bundleDigest(objectsRaw, evidenceRaw, blobs)
  const manifest = {
    schema_version: SCHEMA,
    bundle_id: `bundle-${bundleHash.slice(7, 31)}`,
    created_at: new Date(0).toISOString(),
    source_project_id: sourceProjectID,
    selection: { root_object_ids: rootObjectIDs, include_dependencies: true },
    object_count: objects.length,
    evidence_count: evidence.length,
    required_capabilities: [...new Set(requiredCapabilities)].sort(),
    bundle_hash: bundleHash,
    signature: { status: 'unsigned', algorithm: 'none' },
  }
  const entries = [
    tarEntry('bundle-manifest.json', Buffer.from(JSON.stringify(manifest, null, 2))),
    tarEntry('objects.jsonl', objectsRaw),
    tarEntry('evidence.jsonl', evidenceRaw),
  ]
  for (const key of [...blobs.keys()].sort()) entries.push(tarEntry(`blobs/sha256/${key}.json`, blobs.get(key)))
  entries.push(Buffer.alloc(1024))
  fs.writeFileSync(outputPath, zlib.gzipSync(Buffer.concat(entries), { level: 9, mtime: 0 }))
  return loadBundle(outputPath)
}
