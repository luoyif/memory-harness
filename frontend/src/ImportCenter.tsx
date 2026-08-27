import { ChangeEvent, FormEvent, useEffect, useMemo, useState } from 'react'
import {
  ArrowRight, Bot, CheckCircle2, FileJson2, FileText, FolderInput,
  History, Layers3, LoaderCircle, RefreshCw, ShieldCheck, UploadCloud,
} from 'lucide-react'

import { APIClient, BlueprintCurrent, Project } from './api'

type ImportMode = 'files' | 'paste' | 'conversation'
type TextDocument = { title: string; text: string; bytes: number }
type SourceSummary = { name: string; evidence: number; sessions: number; last_captured_at?: string }
type ConversationPreview = {
  conversations: number
  messages: number
  warnings: number
  date_from?: string
  date_to?: string
}

function fingerprint(parts: string[]) {
  let hash = 2166136261
  for (const value of parts) {
    for (let index = 0; index < value.length; index += 1) {
      hash ^= value.charCodeAt(index)
      hash = Math.imul(hash, 16777619)
    }
  }
  return (hash >>> 0).toString(16).padStart(8, '0')
}

function readableBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function date(value?: string) {
  if (!value) return '尚未捕获'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(parsed)
}

export function ImportCenter({
  api, project, onNavigate,
}: {
  api: APIClient
  project?: Project
  onNavigate: (target: 'home' | 'memory' | 'connections') => void
}) {
  const [mode, setMode] = useState<ImportMode>('files')
  const [documents, setDocuments] = useState<TextDocument[]>([])
  const [pasteTitle, setPasteTitle] = useState('')
  const [pasteText, setPasteText] = useState('')
  const [format, setFormat] = useState('chatgpt')
  const [conversationName, setConversationName] = useState('')
  const [conversationPayload, setConversationPayload] = useState<unknown>()
  const [preview, setPreview] = useState<ConversationPreview>()
  const [sources, setSources] = useState<SourceSummary[]>([])
  const [model, setModel] = useState<Record<string, unknown>>()
  const [blueprint, setBlueprint] = useState<BlueprintCurrent>()
  const [loading, setLoading] = useState(false)
  const [initialLoading, setInitialLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const projectID = project?.project_id || ''
  const runtime = (model?.runtime || {}) as Record<string, unknown>
  const providers = (model?.providers || []) as Array<Record<string, unknown>>
  const selectedBytes = documents.reduce((sum, item) => sum + item.bytes, 0)
  const canSubmit = mode === 'files' ? documents.length > 0 : mode === 'paste' ? Boolean(pasteText.trim()) : Boolean(conversationPayload && preview)

  async function refreshContext() {
    if (!projectID) return
    setInitialLoading(true)
    try {
      const [sourceResult, modelResult, blueprintResult] = await Promise.all([
        api.get<{ sources: SourceSummary[] }>('/v1/sources'),
        api.get<Record<string, unknown>>('/v1/model/config'),
        api.get<BlueprintCurrent>(`/v1/projects/${encodeURIComponent(projectID)}/blueprint`),
      ])
      setSources(sourceResult.sources || [])
      setModel(modelResult)
      setBlueprint(blueprintResult)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setInitialLoading(false)
    }
  }

  useEffect(() => { refreshContext() }, [projectID]) // eslint-disable-line react-hooks/exhaustive-deps

  async function selectDocuments(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files || [])
    event.target.value = ''
    setError(''); setNotice('')
    if (!files.length) return
    if (files.length > 100) { setError('一次最多导入 100 个文本或 Markdown 文件。'); return }
    try {
      const next = await Promise.all(files.map(async file => ({ title: file.name, text: await file.text(), bytes: file.size })))
      const invalid = next.find(item => !item.text.trim())
      if (invalid) throw new Error(`${invalid.title} 没有可导入的文字内容。`)
      setDocuments(next)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason))
    }
  }

  async function selectConversation(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    setError(''); setNotice(''); setPreview(undefined); setConversationPayload(undefined)
    if (!file) return
    try {
      const payload = JSON.parse(await file.text()) as unknown
      setConversationName(file.name)
      setConversationPayload(payload)
      setLoading(true)
      const result = await api.post<{ preview: ConversationPreview }>('/v1/import/conversations', {
        format, project_id: projectID, dry_run: true, payload,
      })
      setPreview(result.preview)
      setNotice('预检完成：没有写入数据。确认后才会保存原始导出并沉淀记忆。')
    } catch (reason) {
      setError(`无法预检这个文件：${reason instanceof Error ? reason.message : String(reason)}`)
    } finally {
      setLoading(false)
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!projectID || !canSubmit) return
    setLoading(true); setError(''); setNotice('')
    try {
      if (mode === 'conversation') {
        const result = await api.post<{ preview: ConversationPreview }>('/v1/import/conversations', {
          format, project_id: projectID, payload: conversationPayload,
        })
        setNotice(`导入完成：${result.preview.conversations} 个会话、${result.preview.messages} 条消息已经进入记忆生长流程。`)
        setConversationPayload(undefined); setConversationName(''); setPreview(undefined)
      } else {
        const payloadDocuments = mode === 'files' ? documents : [{ title: pasteTitle.trim() || '手动记录', text: pasteText.trim(), bytes: pasteText.length }]
        const stableID = fingerprint(payloadDocuments.flatMap(item => [item.title, item.text]))
        const result = await api.post<{ pipeline?: { knowledge_units?: number; status?: string; compiler?: string; quality_status?: string } }>('/v1/import/text', {
          source_system: mode === 'files' ? 'local-files' : 'manual-note',
          session_id: `import-${stableID}`,
          project_id: projectID,
          idempotency_key: `text:${stableID}`,
          documents: payloadDocuments.map(item => ({ title: item.title, text: item.text })),
        })
        setNotice(result.pipeline?.quality_status === 'degraded'
          ? `原文已安全导入；模型本次未完整完成，暂时只生成 ${result.pipeline?.knowledge_units || 0} 条本地预览，未冒充高质量结果。请检查模型连接后重新沉淀。`
          : `导入完成：${result.pipeline?.compiler || '当前整理方式'} 已生成 ${result.pipeline?.knowledge_units || 0} 条有来源的关键信息。重复导入同一内容不会产生副本。`)
        setDocuments([]); setPasteTitle(''); setPasteText('')
      }
      await refreshContext()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setLoading(false)
    }
  }

  const sourceEvidence = useMemo(() => sources.reduce((sum, item) => sum + item.evidence, 0), [sources])

  return <div className="import-center">
    <div className="import-overview">
      <div>
        <p>START WITH YOUR OWN MATERIAL</p>
        <h2>把第一份内容交给记忆系统</h2>
        <span>选择当前项目后导入。原文先成为不可变 Evidence，再由当前 Blueprint 逐层整理。</span>
        <div className="import-steps"><span><b>01</b>保留原文</span><i /><span><b>02</b>抽取知识</span><i /><span><b>03</b>形成记忆</span></div>
      </div>
      <dl>
        <div><dt>当前项目</dt><dd>{project?.name || '正在读取'}</dd></div>
        <div><dt>已有来源</dt><dd>{sources.length}</dd></div>
        <div><dt>原始 Evidence</dt><dd>{sourceEvidence}</dd></div>
      </dl>
    </div>

    <div className="import-layout">
      <main className="import-workbench">
        <div className="import-mode-tabs">
          <button className={mode === 'files' ? 'active' : ''} onClick={() => setMode('files')}><FolderInput size={18} /><span><strong>文件</strong><small>TXT / Markdown</small></span></button>
          <button className={mode === 'paste' ? 'active' : ''} onClick={() => setMode('paste')}><FileText size={18} /><span><strong>粘贴内容</strong><small>笔记 / 资料</small></span></button>
          <button className={mode === 'conversation' ? 'active' : ''} onClick={() => setMode('conversation')}><History size={18} /><span><strong>AI 历史会话</strong><small>ChatGPT / Claude / DeepSeek</small></span></button>
        </div>

        <form onSubmit={submit}>
          {mode === 'files' && <section className="import-pane">
            <header><div><p>LOCAL DOCUMENTS</p><h3>导入文本和 Markdown 文件</h3></div><span>最多 100 个文件</span></header>
            <label className="file-drop">
              <UploadCloud size={30} />
              <strong>{documents.length ? `已选择 ${documents.length} 个文件` : '点击选择文件'}</strong>
              <span>{documents.length ? `共 ${readableBytes(selectedBytes)}，导入前仍可检查` : '支持 .txt、.md、.markdown；文件只写入本地 MemoryOS'}</span>
              <input type="file" accept=".txt,.md,.markdown,text/plain,text/markdown" multiple onChange={selectDocuments} />
            </label>
            {documents.length > 0 && <div className="selected-files">{documents.map(item => <div key={`${item.title}:${item.bytes}`}><FileText size={15} /><div><strong>{item.title}</strong><small>{readableBytes(item.bytes)} · {item.text.slice(0, 72).replaceAll('\n', ' ')}</small></div><CheckCircle2 size={15} /></div>)}</div>}
          </section>}

          {mode === 'paste' && <section className="import-pane paste-pane">
            <header><div><p>QUICK CAPTURE</p><h3>粘贴一段需要长期记住的内容</h3></div><span>适合会议记录、想法和网页摘录</span></header>
            <label>标题<input value={pasteTitle} onChange={event => setPasteTitle(event.target.value)} placeholder="例如：Memory Harness 产品决定" /></label>
            <label>内容<textarea rows={13} value={pasteText} onChange={event => setPasteText(event.target.value)} placeholder="粘贴原始内容。系统会保留全文，再生成可追溯的关键信息和项目记忆。" /></label>
          </section>}

          {mode === 'conversation' && <section className="import-pane">
            <header><div><p>CONVERSATION ARCHIVE</p><h3>导入 AI 平台导出的历史会话</h3></div><span>先预检，再写入</span></header>
            <div className="conversation-source">
              <label>导出来源<select value={format} onChange={event => { setFormat(event.target.value); setConversationPayload(undefined); setConversationName(''); setPreview(undefined); setNotice('') }}><option value="chatgpt">ChatGPT</option><option value="claude">Claude</option><option value="deepseek">DeepSeek</option><option value="normalized">通用规范 JSON</option></select></label>
              <label className="conversation-file"><FileJson2 size={22} /><span><strong>{conversationName || '选择 JSON 导出文件'}</strong><small>读取后立即执行只读预检</small></span>{loading ? <LoaderCircle className="spin" size={18} /> : <ArrowRight size={18} />}<input type="file" accept="application/json,.json" onChange={selectConversation} /></label>
            </div>
            {preview && <div className="import-preview"><ShieldCheck size={20} /><div><strong>文件可以导入</strong><span>{preview.conversations} 个会话 · {preview.messages} 条消息 · {preview.warnings} 个解析提示</span><small>{preview.date_from ? `${date(preview.date_from)} — ${date(preview.date_to)}` : '导出中没有可靠时间信息'}</small></div></div>}
          </section>}

          {error && <p className="import-message error">{error}</p>}
          {notice && <p className="import-message success">{notice}</p>}
          <div className="import-submit">
            <div><ShieldCheck size={15} /><span>原始来源保留在本地；模型、插件和 Agent 不能覆盖它。</span></div>
            <button className="button primary" disabled={!canSubmit || loading}>{loading ? <><LoaderCircle className="spin" size={14} />处理中…</> : mode === 'conversation' ? '确认导入并沉淀' : '导入并生成记忆'}</button>
          </div>
        </form>
      </main>

      <aside className="import-context">
        <section>
          <p>CURRENT BLUEPRINT</p>
          <h3>{blueprint?.blueprint.name || '当前项目默认策略'}</h3>
          <code>{blueprint ? `${blueprint.blueprint.blueprint_id}@${blueprint.blueprint.version}` : '正在读取'}</code>
          <div className="blueprint-mini">{blueprint?.blueprint.definition.tracks.map(track => <span key={track.track_id}><Layers3 size={13} />{track.display_name}<b>{track.nodes.filter(node => node.enabled).length}</b></span>)}</div>
          <small>普通导入不需要编辑 Blueprint。只有需要更换沉淀策略时，才进入高级构建区。</small>
        </section>
        <section>
          <p>MEMORY ENGINE</p>
          <div className="engine-state"><Bot size={18} /><div><strong>{runtime.mode === 'agent' ? '模型增强沉淀' : '本地规则沉淀'}</strong><small>{runtime.mode === 'agent' ? `${providers.length} 个模型已配置` : '无需模型也可安全工作'}</small></div></div>
          <p className="engine-copy">LLM 用于更深的语义抽取，不负责保存原文。未配置模型时，系统使用确定性规则并保留完整来源。</p>
          <button className="text-button" onClick={() => onNavigate('connections')}>打开模型与 Agent 配置 <ArrowRight size={13} /></button>
        </section>
        <section>
          <p>AFTER IMPORT</p>
          <button className="after-import-link" onClick={() => onNavigate('memory')}><BrainIcon /><span><strong>查看记忆内容</strong><small>关键信息、整理结果、项目记忆和来源</small></span><ArrowRight size={14} /></button>
          <button className="after-import-link" onClick={() => onNavigate('home')}><RefreshCw size={16} /><span><strong>返回总览</strong><small>查看重要记忆和最新活动</small></span><ArrowRight size={14} /></button>
        </section>
      </aside>
    </div>

    <section className="source-ledger">
      <header><div><p>PROVENANCE LEDGER</p><h2>已经进入系统的来源</h2></div><button className="text-button" disabled={initialLoading} onClick={refreshContext}><RefreshCw size={13} />刷新</button></header>
      {sources.length ? <div>{sources.map(source => <article key={source.name}><span>{source.name.slice(0, 2).toUpperCase()}</span><div><strong>{source.name}</strong><small>最近捕获 · {date(source.last_captured_at)}</small></div><dl><div><dt>Evidence</dt><dd>{source.evidence}</dd></div><div><dt>会话</dt><dd>{source.sessions}</dd></div></dl></article>)}</div> : <div className="source-empty"><FolderInput size={22} /><strong>还没有导入任何真实内容</strong><span>完成上面的第一笔导入后，来源和会话会出现在这里。</span></div>}
    </section>
  </div>
}

function BrainIcon() {
  return <span className="brain-icon"><i /><i /><i /></span>
}
