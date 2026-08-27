import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Beaker, GitCompare, Plus, RefreshCw, RotateCcw, ShieldCheck } from "lucide-react";
import { APIClient, HarnessObject } from "./api";

type Proposal = {
  proposal_id: string; source_case_ids: string[]; base_blueprint_hash: string; effective_blueprint_hash: string;
  patch: { role: string; config: Record<string, unknown> }; predicted_fix: string; predicted_regressions: string[];
  evaluation_suite: string[]; minimum_sample: number; stop_conditions: { max_regression_rate: number; stop_on_safety_failure: boolean };
  proposer_id: string; verifier_id?: string; evaluation_object_ids: string[]; canary_scope: string; overlay_ttl_seconds: number;
};

type Overlay = {
  overlay_id: string; proposal_id: string; base_blueprint_hash: string; effective_blueprint_hash: string;
  patch: { role: string; config: Record<string, unknown> }; permission_delta: string[]; ttl_seconds: number; expires_at: string;
};

type Evaluation = {
  evaluation_id: string; evaluator_id: string; verdict: "pass" | "fail" | "unknown"; sample_size: number;
  baseline_ref: string; challenger_ref: string; dimensions: Array<{ name: string; verdict: string; note?: string }>;
};

type ProposalDetail = { object: HarnessObject; proposal: Proposal; evaluations: Evaluation[] };
type OverlayDetail = { object: HarnessObject; overlay: Overlay };
type DryRun = { base_blueprint_hash: string; effective_blueprint_hash: string; target_role: string; base_config: Record<string, unknown>; effective_config: Record<string, unknown>; permission_delta: string[]; no_writes_performed: boolean };

const payload = <T,>(object: HarnessObject) => object.revision.payload as T;
const short = (value?: string) => value ? `${value.slice(0, 18)}…${value.slice(-8)}` : "—";

type FailedCase = {
  case_id: string;
  source_run_id: string;
  result: "pass" | "fail" | "unknown";
  primary_failure_dimension?: string;
  diagnosis?: string;
};

type CanaryResult = {
  run_id: string;
  status: string;
  samples: number;
  improved_samples: number;
  regressed_samples: number;
  regression_rate: number;
  safety_failure: boolean;
  global_blueprint_unchanged: boolean;
};

const stamp = (value?: string) => value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
const errorText = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);
const evaluationHasRegression = (value: Evaluation) => value.dimensions.some((item) => item.name.toLowerCase() === "regression");

export function AdaptationLab({ api, projectID }: { api: APIClient; projectID: string }) {
  const [proposals, setProposals] = useState<HarnessObject[]>([]);
  const [overlays, setOverlays] = useState<HarnessObject[]>([]);
  const [cases, setCases] = useState<HarnessObject[]>([]);
  const [selectedProposal, setSelectedProposal] = useState<ProposalDetail>();
  const [selectedOverlay, setSelectedOverlay] = useState<OverlayDetail>();
  const [overlayProposal, setOverlayProposal] = useState<ProposalDetail>();
  const [proposalOpen, setProposalOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [lastCanary, setLastCanary] = useState<CanaryResult>();

  const load = useCallback(async () => {
    if (!projectID) return;
    setLoading(true);
    setError("");
    try {
      const [proposalResult, overlayResult, caseResult] = await Promise.all([
        api.get<{ proposals: HarnessObject[] }>(`/v1/adaptation/proposals?project_id=${encodeURIComponent(projectID)}&limit=200`),
        api.get<{ overlays: HarnessObject[] }>(`/v1/adaptation/overlays?project_id=${encodeURIComponent(projectID)}&limit=200`),
        api.get<{ cases: HarnessObject[] }>(`/v1/experience/cases?project_id=${encodeURIComponent(projectID)}&status=active&limit=200`),
      ]);
      setProposals(proposalResult.proposals || []);
      setOverlays(overlayResult.overlays || []);
      setCases((caseResult.cases || []).filter((item) => payload<FailedCase>(item).result === "fail"));
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setLoading(false);
    }
  }, [api, projectID]);

  useEffect(() => { void load(); }, [load]);

  const counts = useMemo(() => ({
    candidate: proposals.filter((item) => item.status === "candidate").length,
    approved: proposals.filter((item) => item.status === "active").length,
    overlays: overlays.filter((item) => item.status === "active").length,
    failures: cases.length,
  }), [proposals, overlays, cases]);

  async function openProposal(id: string) {
    try {
      setSelectedProposal(await api.get<ProposalDetail>(`/v1/adaptation/proposals/${encodeURIComponent(id)}`));
    } catch (reason) {
      setError(errorText(reason));
    }
  }

  async function openOverlay(id: string) {
    try {
      const detail = await api.get<OverlayDetail>(`/v1/adaptation/overlays/${encodeURIComponent(id)}`);
      const proposal = await api.get<ProposalDetail>(`/v1/adaptation/proposals/${encodeURIComponent(detail.overlay.proposal_id)}`);
      setSelectedOverlay(detail);
      setOverlayProposal(proposal);
    } catch (reason) {
      setError(errorText(reason));
    }
  }

  async function refreshSelectedProposal() {
    if (!selectedProposal) return;
    await openProposal(selectedProposal.object.object_id);
    await load();
  }

  async function refreshSelectedOverlay() {
    if (!selectedOverlay) return;
    await openOverlay(selectedOverlay.object.object_id);
    await load();
  }

  return (
    <div className="experience-bank adaptation-lab">
      <section className="experience-hero">
        <div>
          <p>GOVERNED ADAPTATION · CASE OVERLAY ONLY</p>
          <h2>从失败经验提出可回滚改动，但永远不让经验直接重写 Global Blueprint。</h2>
          <span>
            Proposal、Evaluator、Verifier 三权分离。Dry Run 零写入；Overlay 有 TTL；Canary 超过回归阈值时回退 Base，历史 Run 仍保留完整解释链。
          </span>
        </div>
        <div className="experience-actions">
          <button className="button light" disabled={loading} onClick={() => void load()}>
            <RefreshCw size={14} />刷新
          </button>
          <button className="button" disabled={!cases.length} onClick={() => setProposalOpen(true)}>
            <Plus size={14} />新建 Change Proposal
          </button>
        </div>
      </section>
      {notice && <p className="surface-notice"><ShieldCheck size={14} />{notice}</p>}
      {error && <p className="form-error inline-error">{error}</p>}
      <div className="experience-metrics">
        <article><span>可复现失败 Case</span><strong>{counts.failures}</strong></article>
        <article><span>Proposal Candidate</span><strong>{counts.candidate}</strong></article>
        <article><span>已批准 Proposal</span><strong>{counts.approved}</strong></article>
        <article><span>Active Overlay</span><strong>{counts.overlays}</strong></article>
      </div>

      <section className="experience-section">
        <header><div><GitCompare size={17} /><span><strong>Change Proposals</strong><small>失败 Case → Dry Run → 独立 Evaluation → Owner Review</small></span></div></header>
        {proposals.length ? <div className="experience-case-grid">
          {proposals.map((object) => <ProposalCard key={object.object_id} object={object} onOpen={() => void openProposal(object.object_id)} />)}
        </div> : <EmptyAdaptation title="还没有 Change Proposal" text="先在 Experience Bank 保留一个 active 失败 Case，再从这里提出局部、可验证的上下文策略改动。" />}
      </section>
      <section className="experience-section">
        <header><div><Beaker size={17} /><span><strong>Case Overlays</strong><small>只在受限 Case/Canary 中生效；Global Base 指针保持不动。</small></span></div></header>
        {overlays.length ? <div className="experience-pattern-list">
          {overlays.map((object) => <OverlayCard key={object.object_id} object={object} onOpen={() => void openOverlay(object.object_id)} />)}
        </div> : <EmptyAdaptation title="还没有 Case Overlay" text="只有经过独立 Evaluation 和 Owner Review 的 active Proposal 才能生成 Overlay。" />}
      </section>
      {lastCanary && <section className="experience-section adaptation-last-canary">
        <header><div><ShieldCheck size={17} /><span><strong>最近 Canary</strong><small>{lastCanary.run_id}</small></span></div></header>
        <div className="experience-proof-grid">
          <article><small>STATUS</small><strong>{lastCanary.status}</strong><span>{lastCanary.samples} samples</span></article>
          <article className={lastCanary.regressed_samples ? "danger" : ""}><small>REGRESSION</small><strong>{Math.round(lastCanary.regression_rate * 100)}%</strong><span>{lastCanary.regressed_samples} regressed</span></article>
          <article><small>GLOBAL BASE</small><strong>{lastCanary.global_blueprint_unchanged ? "UNCHANGED" : "BLOCKED"}</strong><span>fail closed</span></article>
        </div>
      </section>}
      {selectedProposal && <ProposalDrawer api={api} detail={selectedProposal} close={() => setSelectedProposal(undefined)} changed={refreshSelectedProposal} setNotice={setNotice} />}
      {selectedOverlay && overlayProposal && <OverlayDrawer api={api} detail={selectedOverlay} proposal={overlayProposal} close={() => { setSelectedOverlay(undefined); setOverlayProposal(undefined); }} changed={refreshSelectedOverlay} setNotice={setNotice} setCanary={setLastCanary} />}
      {proposalOpen && <ProposalModal api={api} projectID={projectID} cases={cases} close={() => setProposalOpen(false)} changed={async () => { setProposalOpen(false); await load(); }} setNotice={setNotice} />}
    </div>
  );
}

function EmptyAdaptation({ title, text }: { title: string; text: string }) {
  return <div className="empty"><span><GitCompare size={20} /></span><h3>{title}</h3><p>{text}</p></div>;
}

function ProposalCard({ object, onOpen }: { object: HarnessObject; onOpen: () => void }) {
  const value = payload<Proposal>(object);
  return <button className="experience-case" onClick={onOpen}>
    <header><span className={`eval-chip ${object.status === "active" ? "pass" : "unknown"}`}>{object.status}</span><em>{value.patch.role}</em></header>
    <h3>{value.predicted_fix}</h3>
    <p>{value.predicted_regressions.join(" · ") || "未声明回归风险"}</p>
    <div className="experience-case-proof">
      <span>CASES <b>{value.source_case_ids.length}</b></span>
      <span>EVAL <b>{value.evaluation_object_ids.length}</b></span>
      <span>SAMPLE ≥ <b>{value.minimum_sample}</b></span>
    </div>
    <footer><span>{value.verifier_id ? `Verifier · ${value.verifier_id}` : `Proposer · ${value.proposer_id}`}</span><code>{short(value.base_blueprint_hash)}</code></footer>
  </button>;
}

function OverlayCard({ object, onOpen }: { object: HarnessObject; onOpen: () => void }) {
  const value = payload<Overlay>(object);
  const expired = new Date(value.expires_at).getTime() <= Date.now();
  return <button className="experience-pattern-card" onClick={onOpen}>
    <div><span className={`eval-chip ${expired ? "fail" : object.status === "active" ? "pass" : "unknown"}`}>{expired ? "expired" : object.status}</span><strong>{value.patch.role}</strong></div>
    <p>Base {short(value.base_blueprint_hash)} → Overlay {short(value.effective_blueprint_hash)}</p>
    <footer><span>TTL {value.ttl_seconds}s · expires {stamp(value.expires_at)}</span><span>permission Δ {value.permission_delta.length}</span></footer>
  </button>;
}

function ProposalDrawer({ api, detail, close, changed, setNotice }: {
  api: APIClient; detail: ProposalDetail; close: () => void; changed: () => Promise<void>; setNotice: (value: string) => void;
}) {
  const value = detail.proposal;
  const [evalOpen, setEvalOpen] = useState(false);
  const [approvalOpen, setApprovalOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function createOverlay() {
    setBusy(true); setError("");
    try {
      const overlay = await api.post<HarnessObject>("/v1/adaptation/overlays", {
        proposal_id: value.proposal_id,
        idempotency_key: `owner-overlay-${value.proposal_id}-${detail.object.current_revision}`,
      });
      setNotice(`已生成 Case Overlay ${overlay.object_id}；仍需 Owner Review 才能激活。`);
      await changed();
    } catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  }

  return <div className="drawer-backdrop" onMouseDown={close}>
    <aside className="drawer experience-drawer" onMouseDown={(event) => event.stopPropagation()}>
      <button className="drawer-close" onClick={close}>×</button>
      <p className="micro">HARNESS CHANGE PROPOSAL</p>
      <h2>{value.predicted_fix}</h2>
      <div className="drawer-status"><span>R{detail.object.current_revision}</span><span>{detail.object.status}</span><span>{value.patch.role}</span></div>
      <p className="drawer-lead">只允许低风险 Context 参数 Overlay；不会修改已发布 Global Blueprint。</p>
      <div className="experience-proof-grid">
        <article><small>SOURCE CASES</small><strong>{value.source_case_ids.length}</strong><span>active failures</span></article>
        <article><small>EVALUATIONS</small><strong>{detail.evaluations.length}</strong><span>independent only</span></article>
        <article><small>STOP RATE</small><strong>{Math.round(value.stop_conditions.max_regression_rate * 100)}%</strong><span>safety stop {value.stop_conditions.stop_on_safety_failure ? "on" : "off"}</span></article>
      </div>
      <dl className="experience-lineage">
        <div><dt>Base Blueprint</dt><dd><code>{value.base_blueprint_hash}</code></dd></div>
        <div><dt>Effective Overlay</dt><dd><code>{value.effective_blueprint_hash}</code></dd></div>
        <div><dt>Patch</dt><dd><code>{JSON.stringify(value.patch.config)}</code></dd></div>
        <div><dt>Evaluation suite</dt><dd>{value.evaluation_suite.join(" · ")}</dd></div>
        <div><dt>Predicted regressions</dt><dd>{value.predicted_regressions.join(" · ") || "None declared"}</dd></div>
        <div><dt>Actors</dt><dd>Proposer {value.proposer_id} · Verifier {value.verifier_id || "pending"}</dd></div>
      </dl>
      <p className="experience-boundary"><AlertTriangle size={14} />Proposal 通过只允许进入 scoped Overlay；不会移动项目 Global Blueprint current pointer。</p>
      {error && <p className="form-error inline-error">{error}</p>}
      <div className="drawer-actions">
        <button className="button" onClick={() => setEvalOpen(true)}><Beaker size={14} />添加独立 Evaluation</button>
        {detail.object.status === "candidate" && <button className="button" onClick={() => setApprovalOpen(true)}><ShieldCheck size={14} />提交 Owner Review</button>}
        {detail.object.status === "active" && <button className="button primary" disabled={busy} onClick={() => void createOverlay()}><Plus size={14} />生成 Case Overlay</button>}
      </div>
      {evalOpen && <EvaluationModal api={api} proposal={detail} close={() => setEvalOpen(false)} changed={async () => { setEvalOpen(false); await changed(); }} />}
      {approvalOpen && <ApprovalModal api={api} detail={detail} close={() => setApprovalOpen(false)} changed={async () => { setApprovalOpen(false); await changed(); }} setNotice={setNotice} />}
    </aside>
  </div>;
}

function OverlayDrawer({ api, detail, proposal, close, changed, setNotice, setCanary }: {
  api: APIClient; detail: OverlayDetail; proposal: ProposalDetail; close: () => void; changed: () => Promise<void>;
  setNotice: (value: string) => void; setCanary: (value: CanaryResult) => void;
}) {
  const value = detail.overlay;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [evalOpen, setEvalOpen] = useState(false);
  const canaryEvaluations = proposal.evaluations.filter(evaluationHasRegression);

  async function proposeActivation() {
    setBusy(true); setError("");
    try {
      await api.post(`/v1/adaptation/overlays/${encodeURIComponent(detail.object.object_id)}/activation-proposal`, {
        expected_revision: detail.object.current_revision,
        edit_reason: "Activate bounded Case Overlay for canary only",
        idempotency_key: `owner-overlay-activation-${detail.object.object_id}-${detail.object.current_revision}`,
      });
      setNotice("Overlay 激活请求已进入“待我审核”；尚未生效。");
      await changed();
    } catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  }

  async function runCanary() {
    setBusy(true); setError("");
    try {
      if (!proposal.proposal.verifier_id) throw new Error("Proposal 还没有独立 Verifier");
      if (!canaryEvaluations.length) throw new Error("先添加至少一个包含 regression 维度的 Canary Evaluation");
      const result = await api.post<CanaryResult>(`/v1/adaptation/overlays/${encodeURIComponent(detail.object.object_id)}/canary`, {
        verifier_id: proposal.proposal.verifier_id,
        evaluation_object_ids: canaryEvaluations.map((item) => item.evaluation_id),
        idempotency_key: `owner-canary-${detail.object.object_id}-${canaryEvaluations.map((item) => item.evaluation_id).join("-")}`,
      });
      setCanary(result);
      setNotice(result.status === "stopped_fallback_base" ? "Canary 命中停止条件：已按 Base fallback 解释，不改变 Global Blueprint。" : "Canary 通过；Overlay 仍只在 scoped 范围内有效。" );
      await changed();
    } catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  }
  async function rollback() {
    setBusy(true); setError("");
    try {
      const result = await api.post<CanaryResult>(`/v1/adaptation/overlays/${encodeURIComponent(detail.object.object_id)}/rollback`, {
        idempotency_key: `owner-rollback-${detail.object.object_id}-${Date.now()}`,
      });
      setCanary(result);
      setNotice("已记录显式 rollback：Global Blueprint Base 保持原 hash。Overlay 历史仍可追溯。");
      await changed();
    } catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  }

  const expired = new Date(value.expires_at).getTime() <= Date.now();
  return <div className="drawer-backdrop" onMouseDown={close}>
    <aside className="drawer experience-drawer" onMouseDown={(event) => event.stopPropagation()}>
      <button className="drawer-close" onClick={close}>×</button>
      <p className="micro">CASE OVERLAY</p>
      <h2>{value.patch.role}</h2>
      <div className="drawer-status"><span>R{detail.object.current_revision}</span><span>{detail.object.status}</span><span>{expired ? "expired" : `expires ${stamp(value.expires_at)}`}</span></div>
      <p className="drawer-lead">Overlay 只是一份不可变的有效 Blueprint snapshot；项目 Global Base 从不被替换。</p>
      <div className="experience-proof-grid">
        <article><small>PERMISSION Δ</small><strong>{value.permission_delta.length}</strong><span>must stay zero</span></article>
        <article><small>CANARY EVALS</small><strong>{canaryEvaluations.length}</strong><span>with regression dimension</span></article>
        <article><small>TTL</small><strong>{value.ttl_seconds}s</strong><span>{expired ? "expired" : "bounded"}</span></article>
      </div>
      <dl className="experience-lineage">
        <div><dt>Base</dt><dd><code>{value.base_blueprint_hash}</code></dd></div>
        <div><dt>Effective</dt><dd><code>{value.effective_blueprint_hash}</code></dd></div>
        <div><dt>Proposal</dt><dd>{value.proposal_id}</dd></div>
        <div><dt>Patch</dt><dd><code>{JSON.stringify(value.patch.config)}</code></dd></div>
      </dl>
      <p className="experience-boundary"><AlertTriangle size={14} />Canary 只读取独立 Evaluation；若回归率超阈值或触发 safety stop，结果为 fallback Base，不会写回全局策略。</p>
      {error && <p className="form-error inline-error">{error}</p>}
      <div className="drawer-actions">
        {detail.object.status === "candidate" && !expired && <button className="button" disabled={busy} onClick={() => void proposeActivation()}><ShieldCheck size={14} />提交 Overlay 激活 Review</button>}
        {detail.object.status === "active" && !expired && <button className="button" onClick={() => setEvalOpen(true)}><Beaker size={14} />添加 Canary Evaluation</button>}
        {detail.object.status === "active" && !expired && <button className="button primary" disabled={busy} onClick={() => void runCanary()}><GitCompare size={14} />运行 Canary</button>}
        {detail.object.status === "active" && <button className="button danger" disabled={busy} onClick={() => void rollback()}><RotateCcw size={14} />回退到 Base</button>}
      </div>
      {evalOpen && <EvaluationModal api={api} proposal={proposal} canary close={() => setEvalOpen(false)} changed={async () => { setEvalOpen(false); await changed(); }} />}
    </aside>
  </div>;
}

function EvaluationModal({ api, proposal, canary = false, close, changed }: {
  api: APIClient; proposal: ProposalDetail; canary?: boolean; close: () => void; changed: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    const evaluator = String(form.get("evaluator") || "").trim();
    const verdict = String(form.get("verdict") || "unknown");
    const sampleSize = Number(form.get("sample_size") || 1);
    const dimensions = canary ? [
      { name: "improvement", verdict: String(form.get("improvement") || "unknown"), confidence: 1 },
      { name: "regression", verdict: String(form.get("regression") || "unknown"), confidence: 1 },
      { name: "safety", verdict: String(form.get("safety") || "pass"), confidence: 1 },
    ] : [{ name: "conformance", verdict, confidence: 1 }];
    try {
      await api.post(`/v1/adaptation/proposals/${encodeURIComponent(proposal.object.object_id)}/evaluations`, {
        protocol: canary ? "owner-canary" : "owner-pre-canary",
        evaluator_id: evaluator, evaluator_version: "1", verdict,
        dimensions, confidence: 1, sample_size: sampleSize,
        expected: String(form.get("expected") || ""), observed: String(form.get("observed") || ""),
        baseline_ref: proposal.proposal.base_blueprint_hash,
        challenger_ref: proposal.proposal.effective_blueprint_hash,
        idempotency_key: `${canary ? "canary" : "pre"}-${evaluator}-${proposal.object.object_id}-${Date.now()}`,
      });
      await changed();
    } catch (reason) { setError(errorText(reason)); setBusy(false); }
  }
  return <div className="modal-backdrop" onMouseDown={close}>
    <form className="modal wide" onSubmit={submit} onMouseDown={(event) => event.stopPropagation()}>
      <p className="micro">{canary ? "CANARY EVALUATION" : "INDEPENDENT PRE-CANARY EVALUATION"}</p>
      <h2>{canary ? "记录 Challenger 的改善与回归" : "先证明 Proposal 值得进入受限 Canary"}</h2>
      <p className="token-warning">Evaluator 不能与 Proposer 或 Verifier 相同；Evaluation 只能观察，不能修改 Proposal / Blueprint。</p>
      <div className="form-grid">
        <label>Evaluator ID<input name="evaluator" required placeholder="independent-evaluator" /></label>
        <label>样本数<input name="sample_size" type="number" min="1" defaultValue={canary ? 1 : proposal.proposal.minimum_sample} required /></label>
      </div>
      <label>总 Verdict<select name="verdict" defaultValue="pass"><option value="pass">pass</option><option value="fail">fail</option><option value="unknown">unknown</option></select></label>
      {canary && <div className="form-grid">
        <label>Improvement<select name="improvement" defaultValue="pass"><option value="pass">pass</option><option value="fail">fail</option><option value="unknown">unknown</option></select></label>
        <label>Regression<select name="regression" defaultValue="pass"><option value="pass">pass / 无回归</option><option value="fail">fail / 有回归</option><option value="unknown">unknown</option></select></label>
      </div>}
      {canary && <label>Safety<select name="safety" defaultValue="pass"><option value="pass">pass</option><option value="fail">fail</option><option value="unknown">unknown</option></select></label>}
      <label>Expected<textarea name="expected" rows={2} placeholder="预期结果 / acceptance criterion" /></label>
      <label>Observed<textarea name="observed" rows={2} placeholder="真实观察结果" /></label>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions">
        <button className="button" type="button" onClick={close}>取消</button>
        <button className="button primary" disabled={busy || !proposal.proposal.base_blueprint_hash}>保存 Evaluation</button>
      </div>
    </form>
  </div>;
}

function ApprovalModal({ api, detail, close, changed, setNotice }: {
  api: APIClient; detail: ProposalDetail; close: () => void; changed: () => Promise<void>; setNotice: (value: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    const form = new FormData(event.currentTarget);
    try {
      await api.post(`/v1/adaptation/proposals/${encodeURIComponent(detail.object.object_id)}/approval-proposal`, {
        expected_revision: detail.object.current_revision,
        verifier_id: String(form.get("verifier") || "").trim(),
        edit_reason: String(form.get("reason") || "").trim(),
        idempotency_key: `owner-proposal-approval-${detail.object.object_id}-${detail.object.current_revision}`,
      });
      setNotice("Change Proposal 已进入“待我审核”；Review 决策前不会生成可用 Overlay。");
      await changed();
    } catch (reason) { setError(errorText(reason)); setBusy(false); }
  }
  return <div className="modal-backdrop" onMouseDown={close}>
    <form className="modal" onSubmit={submit} onMouseDown={(event) => event.stopPropagation()}>
      <p className="micro">INDEPENDENT VERIFIER</p>
      <h2>提交 Proposal 到 Owner Review</h2>
      <p className="token-warning">Verifier 必须与 Proposer、所有 Evaluator 不同。这里仅创建待审核 Revision，不会直接激活。</p>
      <label>Verifier ID<input name="verifier" required placeholder="independent-verifier" /></label>
      <label>审核理由<textarea name="reason" required rows={3} defaultValue="Independent pre-canary Evaluation passed; allow scoped Overlay only." /></label>
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions">
        <button className="button" type="button" onClick={close}>取消</button>
        <button className="button primary" disabled={busy}>创建待审核 Revision</button>
      </div>
    </form>
  </div>;
}

function ProposalModal({ api, projectID, cases, close, changed, setNotice }: {
  api: APIClient; projectID: string; cases: HarnessObject[]; close: () => void; changed: () => Promise<void>; setNotice: (value: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [preview, setPreview] = useState<DryRun>();
  const [draft, setDraft] = useState({
    role: "context.presentation-policy",
    config: '{"object":"verbatim"}',
    fix: "减少该失败 Case 中受治理对象的展示损失。",
    regressions: "更大的上下文负载",
    suite: "task-correctness\nregression-check",
    minimum: 2, regressionRate: 0.25, ttl: 3600,
  });
  const [selectedCases, setSelectedCases] = useState<string[]>([]);
  const [proposer, setProposer] = useState("owner-proposer");
  const [scope, setScope] = useState("case-only");
  const [privacy, setPrivacy] = useState("no scope expansion");
  const [cost, setCost] = useState("bounded context increase");
  const [idempotencyKey] = useState(() => `owner-proposal-${Date.now()}-${Math.random().toString(16).slice(2)}`);

  function input() {
    if (!selectedCases.length) throw new Error("至少选择一个 active 失败 Case");
    let config: Record<string, unknown>;
    try { config = JSON.parse(draft.config) as Record<string, unknown>; }
    catch { throw new Error("Patch config 必须是合法 JSON object"); }
    return {
      project_id: projectID, source_case_ids: selectedCases, source_pattern_ids: [],
      patch: { role: draft.role, config }, predicted_fix: draft.fix,
      predicted_regressions: draft.regressions.split("\n").map((item) => item.trim()).filter(Boolean),
      evaluation_suite: draft.suite.split("\n").map((item) => item.trim()).filter(Boolean),
      minimum_sample: draft.minimum,
      stop_conditions: { max_regression_rate: draft.regressionRate, stop_on_safety_failure: true },
      privacy_impact: privacy, cost_impact: cost, proposer_id: proposer,
      canary_scope: scope, overlay_ttl_seconds: draft.ttl, idempotency_key: idempotencyKey,
    };
  }

  async function dryRun() {
    setBusy(true); setError("");
    try { setPreview(await api.post<DryRun>("/v1/adaptation/proposals/dry-run", input())); }
    catch (reason) { setError(errorText(reason)); }
    finally { setBusy(false); }
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    try {
      if (!preview?.no_writes_performed) throw new Error("必须先完成零写入 Dry Run");
      const object = await api.post<HarnessObject>("/v1/adaptation/proposals", input());
      setNotice(`已创建 Change Proposal ${object.object_id}；下一步必须独立 Evaluation。`);
      await changed();
    } catch (reason) { setError(errorText(reason)); setBusy(false); }
  }

  function toggleCase(id: string, checked: boolean) {
    setSelectedCases((current) => checked ? Array.from(new Set([...current, id])) : current.filter((item) => item !== id));
    setPreview(undefined);
  }

  return <div className="modal-backdrop" onMouseDown={close}>
    <form className="modal wide adaptation-proposal-modal" onSubmit={submit} onMouseDown={(event) => event.stopPropagation()}>
      <p className="micro">CHANGE PROPOSAL · NO DIRECT ACTIVATION</p>
      <h2>从失败 Case 提出受限 Context Overlay</h2>
      <p className="token-warning">只允许 presentation / budget / retrieval 三类低风险 Context patch；权限增量必须保持 0。</p>
      <fieldset><legend>来源失败 Case（至少 1 个）</legend><div className="experience-case-picks">
        {cases.map((object) => { const value = payload<FailedCase>(object); return <label key={object.object_id}>
          <input type="checkbox" checked={selectedCases.includes(object.object_id)} onChange={(event) => toggleCase(object.object_id, event.target.checked)} />
          <span><strong>{value.primary_failure_dimension || "失败 Case"}</strong><small>{value.source_run_id} · {value.diagnosis || "保留负结果"}</small></span>
        </label>; })}
      </div></fieldset>
      <div className="form-grid">
        <label>Patch Role<select value={draft.role} onChange={(event) => { setDraft({ ...draft, role: event.target.value }); setPreview(undefined); }}>
          <option value="context.presentation-policy">context.presentation-policy</option>
          <option value="context.budget-policy">context.budget-policy</option>
          <option value="context.retrieval-policy">context.retrieval-policy</option>
        </select></label>
        <label>Patch Config<textarea value={draft.config} onChange={(event) => { setDraft({ ...draft, config: event.target.value }); setPreview(undefined); }} rows={3} /></label>
      </div>
      <label>Predicted Fix<textarea value={draft.fix} onChange={(event) => { setDraft({ ...draft, fix: event.target.value }); setPreview(undefined); }} rows={3} required /></label>
      <div className="form-grid">
        <label>Proposer ID<input value={proposer} onChange={(event) => { setProposer(event.target.value); setPreview(undefined); }} required /></label>
        <label>Canary Scope<input value={scope} onChange={(event) => { setScope(event.target.value); setPreview(undefined); }} required /></label>
      </div>
      <div className="form-grid">
        <label>最小样本<input type="number" min="1" max="10000" value={draft.minimum} onChange={(event) => { setDraft({ ...draft, minimum: Number(event.target.value) }); setPreview(undefined); }} /></label>
        <label>最大回归率<input type="number" min="0" max="1" step="0.05" value={draft.regressionRate} onChange={(event) => { setDraft({ ...draft, regressionRate: Number(event.target.value) }); setPreview(undefined); }} /></label>
        <label>Overlay TTL(s)<input type="number" min="60" max="86400" value={draft.ttl} onChange={(event) => { setDraft({ ...draft, ttl: Number(event.target.value) }); setPreview(undefined); }} /></label>
      </div>
      <div className="form-grid">
        <label>Predicted Regressions<textarea value={draft.regressions} onChange={(event) => { setDraft({ ...draft, regressions: event.target.value }); setPreview(undefined); }} rows={3} /></label>
        <label>Evaluation Suite<textarea value={draft.suite} onChange={(event) => { setDraft({ ...draft, suite: event.target.value }); setPreview(undefined); }} rows={3} /></label>
      </div>
      <div className="form-grid">
        <label>Privacy Impact<input value={privacy} onChange={(event) => { setPrivacy(event.target.value); setPreview(undefined); }} /></label>
        <label>Cost Impact<input value={cost} onChange={(event) => { setCost(event.target.value); setPreview(undefined); }} /></label>
      </div>
      {preview && <div className="experience-proof-grid adaptation-preview">
        <article><small>DRY RUN</small><strong>{preview.no_writes_performed ? "ZERO WRITE" : "BLOCKED"}</strong><span>{preview.target_role}</span></article>
        <article><small>BASE</small><strong>{short(preview.base_blueprint_hash)}</strong><span>{JSON.stringify(preview.base_config)}</span></article>
        <article><small>OVERLAY</small><strong>{short(preview.effective_blueprint_hash)}</strong><span>{JSON.stringify(preview.effective_config)}</span></article>
      </div>}
      {error && <p className="form-error">{error}</p>}
      <div className="modal-actions">
        <button className="button" type="button" onClick={close}>取消</button>
        <button className="button light" type="button" disabled={busy} onClick={() => void dryRun()}><Beaker size={13} />Dry Run（零写入）</button>
        <button className="button primary" disabled={busy || !preview?.no_writes_performed}>创建 Proposal Candidate</button>
      </div>
    </form>
  </div>;
}
