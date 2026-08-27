import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle, ArrowRight, BookOpenCheck, CircleDollarSign, Clock3, Compass,
  FileInput, Layers3, ListChecks, Plus, RefreshCw, Settings2, ShieldAlert,
  Sparkles, Target,
} from "lucide-react";

import { APIClient, Project } from "./api";
import { ContentPreview } from "./ContentPreview";
import { UIMode } from "./ui";

type ProjectSummary = { project: Project; metrics: Record<string, number>; finance?: { currencies?: Array<Record<string, number | string>> } };
type ProjectTask = {
  task_id: string; project_id: string; title: string; description: string;
  status: "suggested" | "todo" | "in_progress" | "done" | "dismissed";
  priority: number; due_at?: string; source_kind: "manual" | "ai_suggestion";
  source_record_id?: string; source_evidence_ids: string[]; completed_at?: string;
  created_at: string; updated_at: string;
};
type ProjectDetail = {
  summary: ProjectSummary; goals: Array<Record<string, unknown>>; milestones: Array<Record<string, unknown>>;
  decisions: Array<Record<string, unknown>>; risks: Array<Record<string, unknown>>;
  context_blocks: Array<Record<string, unknown>>; finance_accounts: Array<Record<string, unknown>>;
  tasks: ProjectTask[]; automation: { project_id: string; import_mode: "auto_new" | "manual"; updated_at?: string };
};
type RecordKind = "context" | "goal" | "decision" | "risk";
type NavigateTarget = "import" | "memory" | "review" | "strategy" | "studio";

const recordNames: Record<RecordKind, string> = { context: "核心上下文", goal: "目标", decision: "决策", risk: "风险" };
const priorityCopy: Record<number, string> = { 1: "最高", 2: "较高", 3: "普通", 4: "较低", 5: "最低" };

function currency(minor: unknown, code = "CNY") {
  const amount = Number(minor || 0) / 100;
  try { return new Intl.NumberFormat("zh-CN", { style: "currency", currency: code, maximumFractionDigits: 2 }).format(amount); }
  catch { return `${code} ${amount.toFixed(2)}`; }
}
function time(value: unknown) {
  if (!value) return "尚未设置";
  const parsed = new Date(String(value));
  return Number.isNaN(parsed.valueOf()) ? String(value) : new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(parsed);
}
function EmptyRecord({ title, children }: { title: string; children: string }) {
  return <div className="project-record-empty"><span>○</span><strong>{title}</strong><small>{children}</small></div>;
}
function isToday(value?: string) {
  if (!value) return false;
  const due = new Date(value); const now = new Date();
  return due.getFullYear() === now.getFullYear() && due.getMonth() === now.getMonth() && due.getDate() === now.getDate();
}
function isOverdue(value?: string) { return Boolean(value && new Date(value).getTime() < Date.now()); }

export function ProjectWorkspace({ api, projectID, projects, select, onNavigate, mode = "basic" }: {
  api: APIClient; projectID: string; projects: ProjectSummary[]; select: (id: string) => void;
  onNavigate: (target: NavigateTarget) => void; mode?: UIMode;
}) {
  const [detail, setDetail] = useState<ProjectDetail>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [recordKind, setRecordKind] = useState<RecordKind>();
  const [taskModal, setTaskModal] = useState(false);
  const [openContent, setOpenContent] = useState("");

  async function load() {
    if (!projectID) return;
    setLoading(true); setError("");
    try { setDetail(await api.get<ProjectDetail>(`/v1/projects/${encodeURIComponent(projectID)}`)); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setLoading(false); }
  }
  useEffect(() => { void load(); }, [projectID]); // eslint-disable-line react-hooks/exhaustive-deps

  async function createTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(""); const form = new FormData(event.currentTarget);
    try {
      await api.post("/v1/project-tasks", { project_id: projectID, title: String(form.get("title") || "").trim(), description: String(form.get("description") || "").trim(), priority: Number(form.get("priority") || 3), due_at: form.get("due_at") ? new Date(String(form.get("due_at"))).toISOString() : "" });
      setTaskModal(false); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }
  async function updateTask(taskID: string, status: ProjectTask["status"]) {
    setError("");
    try { await api.patch(`/v1/project-tasks/${encodeURIComponent(taskID)}`, { status }); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }
  async function setAutomation(importMode: "auto_new" | "manual") {
    setError("");
    try { await api.patch(`/v1/projects/${encodeURIComponent(projectID)}/automation`, { import_mode: importMode }); await load(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }
  async function createRecord(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!recordKind) return; setError("");
    const form = new FormData(event.currentTarget); const title = String(form.get("title") || "").trim();
    try {
      if (recordKind === "context") await api.post("/v1/context-blocks", { project_id: projectID, label: title, description: form.get("description"), content: form.get("content"), budget_chars: Number(form.get("budget_chars") || 1600), read_only: false, source_refs: [] });
      if (recordKind === "goal") await api.post("/v1/goals", { project_id: projectID, title, description: form.get("description"), status: "active", priority: Number(form.get("priority") || 3), target_at: form.get("target_at") ? new Date(String(form.get("target_at"))).toISOString() : "" });
      if (recordKind === "decision") await api.post("/v1/decisions", { project_id: projectID, title, decision: form.get("content"), rationale: form.get("description"), decided_at: new Date().toISOString(), source_evidence_ids: [] });
      if (recordKind === "risk") await api.post("/v1/risks", { project_id: projectID, title, description: form.get("description"), probability: Number(form.get("probability") || 2), impact: Number(form.get("impact") || 2), mitigation: form.get("content"), owner: "Owner" });
      setRecordKind(undefined); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
  }

  const current = detail?.summary.project || projects.find((item) => item.project.project_id === projectID)?.project;
  const metrics = detail?.summary.metrics || projects.find((item) => item.project.project_id === projectID)?.metrics || {};
  const finance = detail?.summary.finance?.currencies || [];
  const tasks = useMemo(() => detail?.tasks || [], [detail?.tasks]);
  const taskGroups = useMemo(() => ({
    today: tasks.filter((item) => ["todo", "in_progress"].includes(item.status) && isToday(item.due_at)),
    overdue: tasks.filter((item) => ["todo", "in_progress"].includes(item.status) && !isToday(item.due_at) && isOverdue(item.due_at)),
    inProgress: tasks.filter((item) => item.status === "in_progress"),
    suggestions: tasks.filter((item) => item.status === "suggested"),
    todo: tasks.filter((item) => item.status === "todo" && !isToday(item.due_at) && !isOverdue(item.due_at)),
  }), [tasks]);

  function TaskCard({ task, suggestion = false }: { task: ProjectTask; suggestion?: boolean }) {
    return <article className={`project-task-card ${task.status}`}>
      <header><span className={`task-priority p${task.priority}`}>{priorityCopy[task.priority] || task.priority}</span><small>{task.due_at ? time(task.due_at) : "无截止时间"}</small></header>
      <h3>{task.title}</h3>{task.description && <p>{task.description}</p>}
      <footer><span>{suggestion ? `AI 建议 · ${task.source_evidence_ids.length} 条来源` : task.source_kind === "manual" ? "人工创建" : "已确认 AI 建议"}</span><div>
        {suggestion && <><button onClick={() => void updateTask(task.task_id, "dismissed")}>忽略</button><button className="primary" onClick={() => void updateTask(task.task_id, "todo")}>加入待办</button></>}
        {task.status === "todo" && <><button onClick={() => void updateTask(task.task_id, "in_progress")}>开始</button><button onClick={() => void updateTask(task.task_id, "done")}>完成</button></>}
        {task.status === "in_progress" && <button onClick={() => void updateTask(task.task_id, "done")}>完成</button>}
      </div></footer>
    </article>;
  }

  return <div className="project-workspace">
    <div className="project-command"><div><p>当前记忆空间</p><h2>{current?.name || "正在读取项目"}</h2><span>{current?.description || "把这个项目的重要上下文、行动和长期记忆放在同一处。"}</span></div><div className="project-command-actions"><button className="button" onClick={() => onNavigate("memory")}><Layers3 size={14}/>查看记忆</button><button className="button primary" onClick={() => onNavigate("import")}><FileInput size={14}/>导入原材料</button></div></div>
    {error && <p className="project-error"><AlertTriangle size={14}/>{error}</p>}

    <section className="project-task-board">
      <header><div><ListChecks size={20}/><span><strong>我的待办</strong><small>人工待办直接进入清单；AI 识别的行动项先等你确认。</small></span></div><button className="button primary" onClick={() => setTaskModal(true)}><Plus size={14}/>新建待办</button></header>
      <div className="task-summary-strip"><span className={taskGroups.overdue.length ? "danger" : ""}><b>{taskGroups.overdue.length}</b> 已逾期</span><span><b>{taskGroups.today.length}</b> 今天到期</span><span><b>{taskGroups.inProgress.length}</b> 正在处理</span><span><b>{taskGroups.suggestions.length}</b> AI 建议</span><button onClick={() => onNavigate("review")}><b>{metrics.pending_review || 0}</b> 需要我审核 <ArrowRight size={13}/></button></div>
      {taskGroups.suggestions.length > 0 && <div className="task-group suggested"><h3><Sparkles size={15}/>AI 建议待办</h3><div>{taskGroups.suggestions.map((task) => <TaskCard key={task.task_id} task={task} suggestion/>)}</div></div>}
      {taskGroups.overdue.length > 0 && <div className="task-group overdue"><h3><AlertTriangle size={15}/>已逾期</h3><div>{taskGroups.overdue.map((task) => <TaskCard key={task.task_id} task={task}/>)}</div></div>}
      {taskGroups.today.length > 0 && <div className="task-group"><h3><Clock3 size={15}/>今天到期</h3><div>{taskGroups.today.map((task) => <TaskCard key={task.task_id} task={task}/>)}</div></div>}
      {taskGroups.todo.length > 0 && <div className="task-group"><h3><ListChecks size={15}/>接下来</h3><div>{taskGroups.todo.map((task) => <TaskCard key={task.task_id} task={task}/>)}</div></div>}
      {!tasks.filter((item) => !["done", "dismissed"].includes(item.status)).length && <EmptyRecord title="当前没有待办">可以手动新建；系统发现行动项时只会先放进“AI 建议”。</EmptyRecord>}
    </section>

    <div className="project-metric-strip compact-metrics"><article><small>原材料</small><strong>{metrics.evidence || 0}<em>条</em></strong><span>保持原文不变</span></article><article><small>关键信息</small><strong>{metrics.knowledge_units || 0}<em>条</em></strong><span>从原材料识别</span></article><article><small>项目记忆</small><strong>{metrics.available_memories || 0}<em>条</em></strong><span>可供检索使用</span></article><article><small>需要我处理</small><strong>{metrics.pending_review || 0}<em>项</em></strong><span>确认后才生效</span></article></div>

    {loading ? <div className="project-loading"><RefreshCw className="spin" size={20}/>正在读取项目…</div> : <div className="project-board simplified">
      <section className="project-board-wide context-collapsed"><header><div><Compass size={18}/><span><strong>核心上下文</strong><small>默认显示摘要；全文和来源在展开后查看</small></span></div><button onClick={() => setRecordKind("context")}><Plus size={13}/>新增</button></header>{detail?.context_blocks.length ? <div className="context-blocks">{detail.context_blocks.map((item, index) => { const id = String(item.block_id || index); return <article key={id}><div><strong>{String(item.label || "上下文")}</strong><span>更新于 {time(item.updated_at)}</span></div><ContentPreview title={String(item.label || "核心上下文")} content={String(item.content || "")} meta={<span>{Array.isArray(item.source_refs) ? item.source_refs.length : 0} 条来源</span>} open={openContent === `context-${id}`} onOpen={() => setOpenContent(`context-${id}`)} onClose={() => setOpenContent("")}/><small>{String(item.description || "项目长期上下文")}</small>{mode === "advanced" && <small>AI 最多读取 {String(item.budget_chars || 0)} 字符</small>}</article>; })}</div> : <EmptyRecord title="还没有核心上下文">添加项目现状、边界条件或长期方向。</EmptyRecord>}</section>
      <section><header><div><Target size={18}/><span><strong>目标与里程碑</strong><small>{detail?.goals.length || 0} 个目标</small></span></div><button onClick={() => setRecordKind("goal")}><Plus size={13}/>新增</button></header>{detail?.goals.length ? <div className="project-record-list">{detail.goals.map((item, index) => <article key={String(item.goal_id || index)}><span>{String(item.priority || 0)}</span><div><strong>{String(item.title)}</strong><small>{String(item.status)} · {time(item.target_at)}</small></div></article>)}</div> : <EmptyRecord title="尚未设定项目目标">目标可以作为 AI 建议待办的来源，但不会自动成为正式任务。</EmptyRecord>}</section>
      <section><header><div><ShieldAlert size={18}/><span><strong>风险</strong><small>{detail?.risks.length || 0} 项</small></span></div><button onClick={() => setRecordKind("risk")}><Plus size={13}/>新增</button></header>{detail?.risks.length ? <div className="project-record-list risks">{detail.risks.map((item, index) => <article key={String(item.risk_id || index)}><span>{String(item.score || 0)}</span><div><strong>{String(item.title)}</strong><small>{String(item.status)} · {String(item.mitigation || "待制定应对")}</small></div></article>)}</div> : <EmptyRecord title="暂未登记风险">记录影响和应对方式，避免埋在聊天中。</EmptyRecord>}</section>
      <section className="project-board-wide"><header><div><BookOpenCheck size={18}/><span><strong>最近项目动态与决策</strong><small>旧决策不会从历史消失</small></span></div><button onClick={() => setRecordKind("decision")}><Plus size={13}/>新增</button></header>{detail?.decisions.length ? <div className="decision-list">{detail.decisions.map((item, index) => { const id = String(item.decision_id || index); return <article key={id}><div><strong>{String(item.title)}</strong><span>{time(item.decided_at)}</span></div><ContentPreview title={String(item.title || "项目决策")} content={String(item.decision || "")} meta={<span>{String(item.rationale || "未记录理由")}</span>} open={openContent === `decision-${id}`} onOpen={() => setOpenContent(`decision-${id}`)} onClose={() => setOpenContent("")}/><small>{String(item.rationale || "未记录理由")}</small></article>; })}</div> : <EmptyRecord title="还没有正式决策">把关键取舍与理由记录下来。</EmptyRecord>}</section>
      {mode === "advanced" && <section className="project-board-wide automation-board"><header><div><Settings2 size={18}/><span><strong>自动整理设置</strong><small>决定导入之后什么时候整理哪些材料</small></span></div></header><div className="automation-options"><label className={detail?.automation.import_mode === "auto_new" ? "selected" : ""}><input type="radio" name="automation" checked={detail?.automation.import_mode === "auto_new"} onChange={() => void setAutomation("auto_new")}/><span><strong>自动整理新增材料</strong><small>只处理本次新导入且尚未成功的内容；需要确认的结果仍会停下来。</small></span></label><label className={detail?.automation.import_mode === "manual" ? "selected" : ""}><input type="radio" name="automation" checked={detail?.automation.import_mode === "manual"} onChange={() => void setAutomation("manual")}/><span><strong>只保存，稍后手动处理</strong><small>导入时仅保存原文，你可以稍后去记忆库选择材料。</small></span></label></div><footer><button className="button" onClick={() => onNavigate("strategy")}>自定义整理方式</button><button className="button" onClick={() => onNavigate("studio")}>编辑具体步骤</button></footer></section>}
      {mode === "advanced" && <section className="project-board-wide finance-board"><header><div><CircleDollarSign size={18}/><span><strong>项目收支（可选）</strong><small>不同币种分开计算；普通使用无需填写</small></span></div><span>{detail?.finance_accounts.length || 0} 个账户</span></header>{finance.length ? <div className="finance-currencies">{finance.map((item, index) => <article key={String(item.currency || index)}><small>{String(item.currency || current?.default_currency || "CNY")}</small><strong>{currency(item.net_minor, String(item.currency || "CNY"))}</strong><span>收入 {currency(item.income_minor, String(item.currency || "CNY"))} · 支出 {currency(item.expense_minor, String(item.currency || "CNY"))}</span></article>)}</div> : <EmptyRecord title="还没有项目收支">这项完全可选，不会被普通记忆自动改写。</EmptyRecord>}</section>}
    </div>}

    {mode === "advanced" && <section className="project-switcher"><header><div><p>MEMORY SPACES</p><h2>记忆空间边界</h2></div></header><div>{projects.map((item) => <button className={item.project.project_id === projectID ? "active" : ""} key={item.project.project_id} onClick={() => select(item.project.project_id)} style={{ "--project": item.project.color } as React.CSSProperties}><span/><div><strong>{item.project.name}</strong><small>{item.project.description || item.project.slug}</small></div><b>{item.metrics.evidence || 0}</b><ArrowRight size={14}/></button>)}</div></section>}

    {taskModal && <div className="modal-backdrop" onMouseDown={() => setTaskModal(false)}><form className="modal" onSubmit={createTask} onMouseDown={(event) => event.stopPropagation()}><p className="micro">NEW TASK</p><h2>新建待办</h2><label>标题<input name="title" required maxLength={160} autoFocus/></label><label>说明（可选）<textarea name="description" rows={4}/></label><div className="form-grid"><label>优先级<select name="priority" defaultValue="3"><option value="1">最高</option><option value="2">较高</option><option value="3">普通</option><option value="4">较低</option><option value="5">最低</option></select></label><label>截止时间（可选）<input type="datetime-local" name="due_at"/></label></div><div className="modal-actions"><button type="button" className="button" onClick={() => setTaskModal(false)}>取消</button><button className="button primary">加入待办</button></div></form></div>}
    {recordKind && <div className="modal-backdrop" onMouseDown={() => setRecordKind(undefined)}><form className="modal" onSubmit={createRecord} onMouseDown={(event) => event.stopPropagation()}><p className="micro">PROJECT RECORD</p><h2>新增{recordNames[recordKind]}</h2><label>标题<input name="title" required maxLength={160}/></label>{recordKind !== "decision" && <label>说明<textarea name="description" rows={3}/></label>}{recordKind === "context" && <><label>上下文内容<textarea name="content" required rows={7}/></label>{mode === "advanced" && <label>AI 最多读取多少内容<input name="budget_chars" type="number" min="100" max="20000" defaultValue="1600"/></label>}</>}{recordKind === "goal" && <div className="form-grid"><label>优先级<select name="priority" defaultValue="3"><option value="1">最高</option><option value="2">较高</option><option value="3">普通</option><option value="4">较低</option><option value="5">最低</option></select></label><label>目标日期<input type="datetime-local" name="target_at"/></label></div>}{recordKind === "decision" && <><label>决定内容<textarea name="content" required rows={5}/></label><label>理由<textarea name="description" rows={3}/></label></>}{recordKind === "risk" && <><div className="form-grid"><label>发生概率<select name="probability" defaultValue="2"><option value="1">低</option><option value="2">中</option><option value="3">高</option><option value="4">很高</option><option value="5">确定</option></select></label><label>影响程度<select name="impact" defaultValue="2"><option value="1">低</option><option value="2">中</option><option value="3">高</option><option value="4">严重</option><option value="5">致命</option></select></label></div><label>应对方式<textarea name="content" rows={4}/></label></>}{error && <p className="form-error">{error}</p>}<div className="modal-actions"><button type="button" className="button" onClick={() => setRecordKind(undefined)}>取消</button><button className="button primary">保存记录</button></div></form></div>}
  </div>;
}
