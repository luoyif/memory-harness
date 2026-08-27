import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Clock3,
  GitMerge,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  Users,
} from "lucide-react";
import { APIClient, HarnessObject } from "./api";
import { UIMode } from "./ui";

type Agent = {
  agent_id: string;
  name: string;
  status: string;
  all_projects: boolean;
  project_ids: string[];
};
type Task = {
  task_id: string;
  project_id: string;
  title: string;
  member_agent_ids: string[];
  status: string;
  created_at: string;
  expires_at: string;
  closed_at?: string;
};
type Blackboard = {
  entry_id: string;
  task_id: string;
  project_id: string;
  topic: string;
  claim_key: string;
  claim_value: string;
  content: string;
  direct_share_agent_ids: string[];
  meta: {
    agent_id: string;
    run_id?: string;
    source_evidence_ids: string[];
    confidence: number;
    epistemic_status: string;
    created_at: string;
    expires_at: string;
  };
};
type Conflict = {
  conflict_id: string;
  task_id: string;
  topic: string;
  claim_key: string;
  entry_ids: string[];
  agent_ids: string[];
  status: string;
};
type Durable = {
  durable_id: string;
  task_id: string;
  entry_ids: string[];
  title: string;
  summary: string;
  body: string;
  source_agent_ids: string[];
  epistemic_status: string;
};
type TaskDetail = {
  object: HarnessObject;
  task: Task;
  blackboard: HarnessObject[];
};
type CenterProps = {
  api: APIClient;
  projectID: string;
  mode?: UIMode;
  onOpenReview?: () => void;
};

const payload = <T,>(object: HarnessObject) => object.revision.payload as unknown as T;
const stamp = (value?: string) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
const errText = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

export function TeamMemoryCenter({ api, projectID, mode = "advanced", onOpenReview }: CenterProps) {
  const [tasks, setTasks] = useState<HarnessObject[]>([]);
  const [conflicts, setConflicts] = useState<HarnessObject[]>([]);
  const [conflictReviews, setConflictReviews] = useState<Record<string, string>>({});
  const [durables, setDurables] = useState<HarnessObject[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selectedTask, setSelectedTask] = useState<TaskDetail>();
  const [selectedDurable, setSelectedDurable] = useState<HarnessObject>();
  const [taskModal, setTaskModal] = useState(false);
  const [durableModal, setDurableModal] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!projectID) return;
    setBusy(true);
    setError("");
    try {
      const [taskResult, conflictResult, durableResult, agentResult] = await Promise.all([
        api.get<{ tasks: HarnessObject[] }>(`/v1/team/tasks?project_id=${encodeURIComponent(projectID)}&limit=200`),
        api.get<{ conflicts: HarnessObject[]; review_status?: Record<string, string> }>(`/v1/team/conflicts?project_id=${encodeURIComponent(projectID)}&limit=200`),
        api.get<{ durables: HarnessObject[] }>(`/v1/team/durables?project_id=${encodeURIComponent(projectID)}&limit=200`),
        api.get<{ agents: Agent[] }>("/v1/agents"),
      ]);
      setTasks(taskResult.tasks || []);
      setConflicts(conflictResult.conflicts || []);
      setConflictReviews(conflictResult.review_status || {});
      setDurables(durableResult.durables || []);
      setAgents(agentResult.agents || []);
    } catch (reason) {
      setError(errText(reason));
    } finally {
      setBusy(false);
    }
  }, [api, projectID]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    setSelectedTask(undefined);
    setSelectedDurable(undefined);
    setTaskModal(false);
    setDurableModal(false);
  }, [projectID]);

  const projectAgents = useMemo(() => agents.filter((agent) =>
    agent.status === "active" && (agent.all_projects || agent.project_ids?.includes(projectID))), [agents, projectID]);
  const metrics = useMemo(() => ({
    activeTasks: tasks.filter((item) => item.status === "active").length,
    pendingConflicts: conflicts.filter((item) => (conflictReviews[item.object_id] || "missing") === "pending").length,
    durableCandidates: durables.filter((item) => item.status === "candidate").length,
    activeDurables: durables.filter((item) => item.status === "active").length,
  }), [tasks, conflicts, conflictReviews, durables]);

  async function openTask(id: string) {
    try {
      setSelectedTask(await api.get<TaskDetail>(`/v1/team/tasks/${encodeURIComponent(id)}`));
    } catch (reason) {
      setError(errText(reason));
    }
  }

  async function refreshTask() {
    if (!selectedTask) return;
    await openTask(selectedTask.object.object_id);
    await load();
  }

  async function refreshAll(message?: string) {
    if (message) setNotice(message);
    await load();
  }

  return <div className="team-memory-center">
    <section className="team-memory-hero">
      <div>
        <p>多 AI 协作 · 由你控制共享范围</p>
        <h2>不同 AI 可以参与同一任务，但看不到彼此的私密草稿。</h2>
        <span>Memory Harness 不会自动启动或调度外部 AI。它只负责参与者、项目权限、共享对象、冲突处理和长期保存。</span>
      </div>
      <div className="experience-actions">
        <button className="button light" disabled={busy} onClick={() => void load()}><RefreshCw size={14}/>刷新</button>
        <button className="button" aria-label="新建 Team Task（协作任务）" disabled={!projectAgents.length} onClick={() => setTaskModal(true)}><Users size={14}/>新建协作任务</button>
      </div>
    </section>
    <section className="team-onboarding" aria-label="多 AI 协作四步">
      <article><b>1</b><div><strong>连接不同 AI</strong><small>每个 AI 使用自己的身份和权限。</small></div></article>
      <article><b>2</b><div><strong>选择任务参与者</strong><small>只有直接加入的 AI 能参与。</small></div></article>
      <article><b>3</b><div><strong>主动提交共享内容</strong><small>私密草稿只对作者自己可见。</small></div></article>
      <article><b>4</b><div><strong>你确认冲突和长期保存</strong><small>系统不会自动覆盖或写入长期记忆。</small></div></article>
    </section>
    {notice && <p className="surface-notice"><ShieldCheck size={14}/>{notice}</p>}
    {error && <p className="form-error inline-error">{error}</p>}
    {mode === "advanced" && <div className="experience-metrics">
      <article><span>Active Tasks</span><strong>{metrics.activeTasks}</strong></article>
      <article className={metrics.pendingConflicts ? "danger" : ""}><span>Conflict Review</span><strong>{metrics.pendingConflicts}</strong></article>
      <article><span>Durable Candidates</span><strong>{metrics.durableCandidates}</strong></article>
      <article><span>Active Durable</span><strong>{metrics.activeDurables}</strong></article>
    </div>}

    <section className="experience-section">
      <header><div><Users size={17}/><span><strong>协作任务</strong><small>{mode === "advanced" ? "任务边界 · 有效期 · 直接成员 · 不可转发" : "选择哪些 AI 共同参与一项工作"}</small></span></div></header>
      {tasks.length ? <div className="team-task-grid">
        {tasks.map((object) => {
          const task = payload<Task>(object);
          return <button key={object.object_id} className={`team-task-card ${object.status}`} onClick={() => void openTask(object.object_id)}>
            <header><span>{object.status.toUpperCase()}</span><Clock3 size={14}/></header>
            <h3>{task.title}</h3>
            <p>{task.member_agent_ids.length} 个直接成员 · 到期 {stamp(task.expires_at)}</p>
            <footer><code>{task.task_id}</code><span>{object.current_revision > 1 ? `R${object.current_revision}` : "R1"}</span></footer>
          </button>;
        })}
      </div> : <EmptyTeam icon="task" title="还没有协作任务" text="请先在“模型与 Agent”创建至少一个可参与协作的 AI，再选择直接参与者。"/>}
    </section>

    <section className="experience-section">
      <header><div><AlertTriangle size={17}/><span><strong>需要你处理的冲突</strong><small>{mode === "advanced" ? "同一主张的冲突不会用最后写入或多数票覆盖。" : "不同 AI 意见不一致时，由你决定采用哪一项。"}</small></span></div>{onOpenReview && <button className="button light" onClick={onOpenReview}>打开待我审核</button>}</header>
      {conflicts.length ? <div className="team-conflict-list">
        {conflicts.map((object) => {
          const item = payload<Conflict>(object);
          return <article key={object.object_id} className="team-conflict-card">
            <div><AlertTriangle size={15}/><strong>{item.claim_key}</strong></div>
            <p>{item.topic}</p>
            <small>{item.entry_ids.length} 条冲突贡献 · {item.agent_ids.length} 个 Agent · review {conflictReviews[object.object_id] || "missing"} · epistemic {item.status}</small>
          </article>;
        })}
      </div> : <EmptyTeam icon="conflict" title="没有待处理冲突" text="如果多个 Agent 对同一 claim 给出不同值，这里会出现 protected Conflict 与 pending Review。"/>}
    </section>

    {mode === "advanced" && <section className="experience-section">
      <header><div><GitMerge size={17}/><span><strong>Project Durable Promotion</strong><small>Task close → Owner selects entries → Candidate → Review → Active</small></span></div>{onOpenReview && <button className="button light" onClick={onOpenReview}>打开待我审核</button>}</header>
      {durables.length ? <div className="team-durable-list">
        {durables.map((object) => {
          const item = payload<Durable>(object);
          return <button key={object.object_id} onClick={() => setSelectedDurable(object)}>
            <span className={`eval-chip ${object.status === "active" ? "pass" : ""}`}>{object.status}</span>
            <main><strong>{item.title}</strong><small>{item.summary || item.body.slice(0, 90)}</small></main>
            <aside><b>{item.entry_ids.length}</b><span>entries</span><b>{item.source_agent_ids.length}</b><span>agents</span></aside>
          </button>;
        })}
      </div> : <EmptyTeam icon="durable" title="还没有 Project Durable" text="任务关闭后，Owner 选择没有未解决冲突的 Blackboard 条目，先形成 Candidate，再经 Review 激活。"/>}
    </section>}

    {selectedTask && <TaskDrawer api={api} detail={selectedTask} close={() => setSelectedTask(undefined)} changed={refreshTask} openDurable={() => setDurableModal(true)} setNotice={setNotice}/>}
    {selectedDurable && <DurableDrawer api={api} object={selectedDurable} close={() => setSelectedDurable(undefined)} changed={async () => { setSelectedDurable(undefined); await refreshAll("Project Durable 已提交到 Owner Review；审核前不会进入长期检索。"); }} onOpenReview={onOpenReview}/>}
    {taskModal && <TaskModal api={api} projectID={projectID} agents={projectAgents} close={() => setTaskModal(false)} changed={async () => { setTaskModal(false); await refreshAll("Team Task 已创建；Private / Blackboard 共享范围已固定到直接成员。"); }}/>}
    {durableModal && selectedTask && <DurableModal api={api} detail={selectedTask} close={() => setDurableModal(false)} changed={async () => { setDurableModal(false); await refreshTask(); await refreshAll("已形成 Project Durable Candidate；仍需提交激活 Review。"); }}/>}
  </div>;
}

function EmptyTeam({ icon, title, text }: { icon: string; title: string; text: string }) {
  return <div className="empty"><span>{icon === "conflict" ? <AlertTriangle size={20}/> : icon === "durable" ? <GitMerge size={20}/> : <Users size={20}/>}</span><h3>{title}</h3><p>{text}</p></div>;
}

function TaskDrawer({ api, detail, close, changed, openDurable, setNotice }: {
  api: APIClient; detail: TaskDetail; close: () => void; changed: () => Promise<void>;
  openDurable: () => void; setNotice: (value: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const task = detail.task;
  const blackboard = detail.blackboard || [];
  async function closeTask() {
    setBusy(true); setError("");
    try {
      await api.post(`/v1/team/tasks/${encodeURIComponent(detail.object.object_id)}/close-proposal`, {
        expected_revision: detail.object.current_revision,
        edit_reason: "Owner closes the team task and freezes the promotion boundary.",
        idempotency_key: `team-close-${detail.object.object_id}-${detail.object.current_revision}`,
      });
      setNotice("任务关闭已提交 Owner Review；审核前 Task 仍保持 active。");
      await changed();
    } catch (reason) { setError(errText(reason)); }
    finally { setBusy(false); }
  }
  return <div className="drawer-backdrop" onMouseDown={close}>
    <aside className="drawer experience-drawer team-task-drawer" onMouseDown={(event) => event.stopPropagation()}>
      <header><div><p className="micro">TEAM TASK · OWNER VIEW</p><h2>{task.title}</h2></div><button className="button light" onClick={close}>关闭</button></header>
      <p className="experience-boundary"><LockKeyhole size={15}/><span>Owner 页面不会加载或展示 Private Scratch。下面只显示当前任务的共享 Blackboard、成员、TTL 和治理状态。</span></p>
      {error && <p className="form-error">{error}</p>}
      <div className="experience-proof-grid">
        <article><small>STATUS</small><strong>{task.status}</strong><span>R{detail.object.current_revision}</span></article>
        <article><small>MEMBERS</small><strong>{task.member_agent_ids.length}</strong><span>direct only</span></article>
        <article><small>BLACKBOARD</small><strong>{blackboard.length}</strong><span>shared entries</span></article>
        <article><small>EXPIRES</small><strong>{stamp(task.expires_at).split(" ")[0]}</strong><span>{stamp(task.expires_at)}</span></article>
      </div>
      <section className="team-drawer-section">
        <h3>Shared Blackboard</h3>
        {blackboard.length ? <div className="team-blackboard-list">{blackboard.map((object) => {
          const entry = payload<Blackboard>(object);
          return <article key={object.object_id}>
            <header><div><strong>{entry.topic}</strong><code>{entry.claim_key}</code></div><span className="eval-chip">{entry.meta.epistemic_status}</span></header>
            <p><b>{entry.claim_value}</b> · {entry.content}</p>
            <footer><span>Agent {entry.meta.agent_id}</span><span>direct-share {entry.direct_share_agent_ids.length}</span><span>TTL {stamp(entry.meta.expires_at)}</span></footer>
          </article>;
        })}</div> : <div className="empty compact"><p>还没有共享 Blackboard 条目。</p></div>}
      </section>
      <footer className="drawer-actions">
        {task.status === "active" && <button className="button" disabled={busy} onClick={() => void closeTask()}>提交任务关闭 Review</button>}
        {task.status === "closed" && blackboard.length > 0 && <button className="button primary" onClick={openDurable}>选择性沉淀 Project Durable</button>}
      </footer>
    </aside>
  </div>;
}
function DurableDrawer({ api, object, close, changed, onOpenReview }: {
  api: APIClient; object: HarnessObject; close: () => void; changed: () => Promise<void>; onOpenReview?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const item = payload<Durable>(object);
  async function propose() {
    setBusy(true); setError("");
    try {
      await api.post(`/v1/team/durables/${encodeURIComponent(object.object_id)}/activation-proposal`, {
        expected_revision: object.current_revision,
        edit_reason: "Owner selected this closed-task summary for governed project memory.",
        idempotency_key: `team-durable-activate-${object.object_id}-${object.current_revision}`,
      });
      await changed();
    } catch (reason) { setError(errText(reason)); }
    finally { setBusy(false); }
  }
  return <div className="drawer-backdrop" onMouseDown={close}>
    <aside className="drawer experience-drawer" onMouseDown={(event) => event.stopPropagation()}>
      <header><div><p className="micro">PROJECT DURABLE · GOVERNED</p><h2>{item.title}</h2></div><button className="button light" onClick={close}>关闭</button></header>
      <p className="experience-boundary"><ShieldCheck size={15}/><span>Candidate 不进入默认项目召回。只有 Owner Review 批准为 active 后，Portfolio Index 才会把它作为长期项目记忆暴露。</span></p>
      {error && <p className="form-error">{error}</p>}
      <div className="experience-proof-grid">
        <article><small>STATUS</small><strong>{object.status}</strong><span>R{object.current_revision}</span></article>
        <article><small>ENTRIES</small><strong>{item.entry_ids.length}</strong><span>owner selected</span></article>
        <article><small>AGENTS</small><strong>{item.source_agent_ids.length}</strong><span>provenance kept</span></article>
        <article><small>EPISTEMIC</small><strong>{item.epistemic_status}</strong><span>not majority truth</span></article>
      </div>
      <section className="team-durable-body"><h3>{item.summary || "Summary"}</h3><pre>{item.body}</pre></section>
      <footer className="drawer-actions">
        {object.status === "candidate" && <button className="button primary" disabled={busy} onClick={() => void propose()}>提交激活 Review</button>}
        {onOpenReview && <button className="button light" onClick={onOpenReview}>打开待我审核</button>}
      </footer>
    </aside>
  </div>;
}
function TaskModal({ api, projectID, agents, close, changed }: {
  api: APIClient; projectID: string; agents: Agent[]; close: () => void; changed: () => Promise<void>;
}) {
  const [members, setMembers] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    try {
      if (!members.length) throw new Error("至少选择一个直接成员 Agent");
      await api.post("/v1/team/tasks", {
        project_id: projectID,
        title: String(form.get("title") || "").trim(),
        member_agent_ids: members,
        ttl_seconds: Number(form.get("ttl") || 3600),
        idempotency_key: `owner-team-task-${projectID}-${Date.now()}`,
      });
      await changed();
    } catch (reason) { setError(errText(reason)); setBusy(false); }
  }
  function toggle(id: string, checked: boolean) {
    setMembers((current) => checked ? Array.from(new Set([...current, id])) : current.filter((value) => value !== id));
  }
  return <div className="modal-backdrop" onMouseDown={close}>
    <form className="modal wide" onSubmit={submit} onMouseDown={(event) => event.stopPropagation()}>
      <p className="micro">TEAM TASK · DIRECT MEMBERSHIP</p>
      <h2>创建多 Agent 任务边界</h2>
      <p className="token-warning">成员关系不具有传递性。加入同一 Task 不代表 Agent 自动获得彼此 Private Scratch，也不代表可以转授权 Blackboard 分享。</p>
      <label>任务名称<input name="title" required placeholder="例如：跨 Agent 架构复核"/></label>
      <label>任务 TTL（秒，60–604800）<input name="ttl" type="number" min={60} max={604800} defaultValue={3600} required/></label>
      <fieldset><legend>直接成员 Agent</legend><div className="experience-case-picks">
        {agents.map((agent) => <label key={agent.agent_id}><input type="checkbox" checked={members.includes(agent.agent_id)} onChange={(event) => toggle(agent.agent_id, event.target.checked)}/><span><strong>{agent.name}</strong><small>{agent.agent_id}</small></span></label>)}
      </div></fieldset>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions"><button className="button" type="button" onClick={close}>取消</button><button className="button primary" disabled={busy}>创建 Team Task</button></div>
    </form>
  </div>;
}
function DurableModal({ api, detail, close, changed }: {
  api: APIClient; detail: TaskDetail; close: () => void; changed: () => Promise<void>;
}) {
  const [entries, setEntries] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const blackboard = detail.blackboard || [];
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    try {
      if (!entries.length) throw new Error("至少选择一条没有未解决冲突的 Blackboard 条目");
      await api.post("/v1/team/durables", {
        task_id: detail.task.task_id,
        entry_ids: entries,
        title: String(form.get("title") || "").trim(),
        summary: String(form.get("summary") || "").trim(),
        body: String(form.get("body") || "").trim(),
        idempotency_key: `owner-team-durable-${detail.task.task_id}-${Date.now()}`,
      });
      await changed();
    } catch (reason) { setError(errText(reason)); setBusy(false); }
  }
  function toggle(id: string, checked: boolean) {
    setEntries((current) => checked ? Array.from(new Set([...current, id])) : current.filter((value) => value !== id));
  }
  return <div className="modal-backdrop" onMouseDown={close}>
    <form className="modal wide" onSubmit={submit} onMouseDown={(event) => event.stopPropagation()}>
      <p className="micro">PROJECT DURABLE · OWNER SELECTION</p>
      <h2>从已关闭任务选择长期沉淀</h2>
      <p className="token-warning">系统不会自动把 Blackboard 变成长期事实；同一 claim_key 的不同值不能一起沉淀，冲突必须先在 Review Center 处理。</p>
      <fieldset><legend>选择 Blackboard 来源</legend><div className="experience-case-picks">
        {blackboard.map((object) => { const entry = payload<Blackboard>(object); return <label key={object.object_id}><input type="checkbox" checked={entries.includes(object.object_id)} onChange={(event) => toggle(object.object_id, event.target.checked)}/><span><strong>{entry.claim_key} = {entry.claim_value}</strong><small>{entry.topic} · Agent {entry.meta.agent_id}</small></span></label>; })}
      </div></fieldset>
      <label>长期记忆标题<input name="title" required defaultValue={detail.task.title + " · Durable"}/></label>
      <label>摘要<input name="summary" placeholder="这组共享信息为什么值得进入项目长期记忆"/></label>
      <label>正文<textarea name="body" rows={6} required placeholder="由 Owner 整理的长期项目记忆正文；保留来源条目与 Agent provenance。"/></label>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions"><button className="button" type="button" onClick={close}>取消</button><button className="button primary" disabled={busy}>创建 Durable Candidate</button></div>
    </form>
  </div>;
}
