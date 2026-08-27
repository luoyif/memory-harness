export type OwnerSession = {
  endpoint: string
  sessionID: string
  token: string
  csrf: string
  expiresAt: string
  version: string
}

export type ConnectionState =
  | { mode: 'desktop'; session: OwnerSession }
  | { mode: 'locked'; endpoint: string; health?: Record<string, unknown>; reason: string }

const normalizeEndpoint = (value: string) => value.replace(/\/$/, '')

export async function connect(): Promise<ConnectionState> {
  const bridge = window.go?.main?.DesktopBridge
  if (bridge?.Bootstrap) {
    const result = await bridge.Bootstrap()
    return {
      mode: 'desktop',
      session: {
        endpoint: normalizeEndpoint(result.endpoint),
        sessionID: result.session_id,
        token: result.token,
        csrf: result.csrf_token,
        expiresAt: result.expires_at,
        version: result.version,
      },
    }
  }

  const endpoint = window.location.protocol.startsWith('http') ? window.location.origin : 'http://127.0.0.1:19777'
  try {
    const response = await fetch(`${endpoint}/health`, { cache: 'no-store' })
    const health = response.ok ? await response.json() : undefined
    return { mode: 'locked', endpoint, health, reason: '此页面没有桌面 Owner 会话。请从 Memory Harness 应用打开。' }
  } catch {
    return { mode: 'locked', endpoint, reason: '内核尚未启动。请启动 Memory Harness 桌面应用。' }
  }
}

export class APIClient {
  constructor(private readonly session: OwnerSession) {}

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const method = (init.method || 'GET').toUpperCase()
    const headers = new Headers(init.headers)
    headers.set('X-Memory-Harness-Owner', this.session.token)
    if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
      headers.set('X-Memory-Harness-CSRF', this.session.csrf)
    }
    if (init.body && !(init.body instanceof Blob) && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json')
    }
    const response = await fetch(`${this.session.endpoint}${path}`, { ...init, headers, cache: 'no-store' })
    if (response.status === 204) return undefined as T
    const raw = await response.text()
    let data: unknown
    try { data = raw ? JSON.parse(raw) : undefined } catch { data = raw }
    if (!response.ok) {
      const record = data as { error?: { message?: string }; errors?: string[] } | undefined
      throw new Error(record?.error?.message || record?.errors?.join('；') || `请求失败（${response.status}）`)
    }
    return data as T
  }

  get<T>(path: string) { return this.request<T>(path) }
  post<T>(path: string, body: unknown) { return this.request<T>(path, { method: 'POST', body: JSON.stringify(body) }) }
  put<T>(path: string, body: unknown) { return this.request<T>(path, { method: 'PUT', body: JSON.stringify(body) }) }
  patch<T>(path: string, body: unknown) { return this.request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }) }
  delete<T>(path: string) { return this.request<T>(path, { method: 'DELETE' }) }
}

export type Project = {
  project_id: string
  slug: string
  name: string
  description: string
  status: string
  color: string
  default_currency: string
  budget_minor: number
  summary?: {
    metrics?: Record<string, number>
    finance?: { currencies?: Array<Record<string, number | string>> }
  }
}

export type MemoryType = {
  type_id: string
  plugin_id: string
  display_name: string
  schema_version: string
  lifecycle: { initial: string; states: string[]; transitions: Record<string, string[]> }
  protection_class: string
  renderer: Record<string, unknown>
  status: string
}

export type HarnessObject = {
  object_id: string
  type_id: string
  project_id: string
  status: string
  protection_class: string
  current_revision: number
  revision: {
    payload: Record<string, unknown>
    content_hash: string
    confidence: number
    importance: number
    run_id?: string
    stage_id?: string
    plugin_id: string
    plugin_version: string
    source_evidence_ids: string[]
    source_object_ids: string[]
    created_at: string
  }
  created_at: string
  updated_at: string
}

export type HarnessRun = {
  run_id: string
  project_id: string
  caller_type: string
  caller_id: string
  channel: string
  pipeline_id: string
  pipeline_version: string
  pipeline_hash: string
  status: string
  created_at: string
  started_at?: string
  ended_at?: string
  retry_of_run_id?: string
  forked_from_run_id?: string
}

export type ModelUsageSummary = {
  calls: number; successful_calls: number; failed_calls: number; provider_reported_calls: number; priced_calls?: number
  prompt_tokens: number; completion_tokens: number; total_tokens: number; reasoning_tokens: number; cached_prompt_tokens: number
  total_latency_ms: number; max_latency_ms: number; estimated_cost_microminor: number
  currency?: string; cost_status: 'unavailable' | 'estimated' | 'mixed_currency' | string
}

export type ModelProviderUsageSummary = {
  provider_id: string; provider: string; model: string; last_call_at?: string; health: ModelUsageSummary
}

export type ModelUsageDashboard = {
  window_hours: number; generated_at: string; health: ModelUsageSummary; providers: ModelProviderUsageSummary[]
}

export type ModelCallObservation = {
  call_id: string; run_id?: string; node_id?: string; project_id?: string; stage_type?: string
  provider_id: string; provider: string; model: string; status: string; usage_source: string
  prompt_tokens: number; completion_tokens: number; total_tokens: number; reasoning_tokens?: number; cached_prompt_tokens?: number
  latency_ms: number; currency?: string; estimated_cost_microminor?: number; pricing_source: string; error_code?: string; created_at: string
}

export type RunDetail = {
  run: HarnessRun
  spans: Array<Record<string, unknown> & { span_id: string; node_id: string; stage_type: string; status: string; started_at: string; ended_at?: string }>
  events: Array<{ sequence: number; event_type: string; producer: string; data: Record<string, unknown>; created_at: string }>
  effects: Array<Record<string, unknown> & { node_id: string; effect_key: string; status: string; outcome: string }>
  stage_outputs: Array<{ run_id: string; node_id: string; output_hash: string; payload: Record<string, unknown>; created_at: string }>
  model_calls?: ModelCallObservation[]
  model_health?: ModelUsageSummary
}

export type PipelineVersion = {
  pipeline_id: string
  version: string
  plugin_id: string
  name: string
  content_hash: string
  status: string
  definition: {
    api_version: string
    pipeline_id: string
    version: string
    name: string
    intent: string
    required_capabilities: string[]
    nodes: Array<{ id: string; stage_type: string; stage_version: string; plugin_id: string; depends_on: string[]; config: Record<string, unknown> }>
    outputs: Array<{ name: string; node_id: string }>
    policy: { max_stages: number; timeout_seconds: number; max_model_calls: number }
    editor?: { positions?: Record<string, { x: number; y: number }> }
  }
}

export type PipelineDefinition = PipelineVersion['definition']

export type PipelineDraft = {
  draft_id: string
  pipeline_id: string
  plugin_id: string
  base_version?: string
  definition: PipelineDefinition
  revision: number
  created_at: string
  updated_at: string
}

export type BlueprintNode = {
  node_id: string
  role: string
  display_name: string
  binding_kind: 'memory_type' | 'stage' | 'provider' | 'policy' | string
  plugin_id: string
  plugin_version: string
  component_id: string
  component_version: string
  enabled: boolean
  required_capabilities: string[]
  config: Record<string, unknown>
}

export type BlueprintTrack = {
  track_id: string
  role: string
  display_name: string
  description: string
  nodes: BlueprintNode[]
}

export type BlueprintDefinition = {
  api_version: string
  blueprint_id: string
  version: string
  name: string
  description: string
  intent: string
  tracks: BlueprintTrack[]
  policy: {
    evidence_mode: string
    model_boundary: string
    default_context_budget: number
    cross_project_recall: boolean
  }
}

export type BlueprintValidation = {
  valid: boolean
  errors: string[]
  warnings: string[]
  track_count: number
  enabled_component_count: number
  required_capabilities: string[]
}

export type BlueprintVersion = {
  blueprint_id: string
  version: string
  plugin_id: string
  name: string
  description: string
  definition: BlueprintDefinition
  content_hash: string
  status: string
  created_at: string
}

export type BlueprintCurrent = {
  assignment: {
    project_id: string
    blueprint_id: string
    blueprint_version: string
    blueprint_hash: string
    status: string
    activated_by: string
    activated_at: string
    updated_at: string
  }
  blueprint: BlueprintVersion
  inherited: boolean
  validation: BlueprintValidation
}

export type StrategyComponent = {
  component_id: string
  version: string
  display_name: string
  description: string
  role: string
  kind: string
  stage_type?: string
  configuration?: string
  capabilities: string[]
}

export type PluginConformanceCheck = { name:string; status:string; detail:string; data?:Record<string,unknown> }
export type PluginConformanceReport = {
  plugin_id:string; version:string; project_id?:string; memory_harness_version:string; compatibility_requirement:string; compatibility_status:string
  declared_capabilities:string[]; granted_capabilities:string[]; missing_required:string[]; optional_not_granted:string[]
  configuration_schema?:Record<string,unknown>; configuration_status:string; overall_status:string; checks:PluginConformanceCheck[]
}

export type PluginVersion = {
  plugin_id: string
  version: string
  name: string
  publisher: string
  trust_class: string
  signature_status: string
  status: string
  permissions: string[]
  contributions: {
    memory_types?: unknown[]
    pipelines?: unknown[]
    stages?: unknown[]
    strategy_components?: StrategyComponent[]
    blueprints?: Array<{ blueprint_id: string; version: string; definition: string }>
    connectors?: unknown[]
    projections?: unknown[]
    views?: unknown[]
  }
  project_states: Array<{ project_id: string; status: string; granted_capabilities: string[]; config?: Record<string, unknown>; updated_at?: string }>
  manifest?: Record<string, unknown>
  content_hash?: string
  package_size_bytes?: number
}
