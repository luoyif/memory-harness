import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Beaker,
  CheckCircle2,
  GitBranch,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { APIClient, HarnessObject } from "./api";

type Delivery = {
  total: number;
  delivered: number;
  trimmed: number;
  denied: number;
  failed: number;
  delivery_unverified: number;
  evidence_level?: string;
  completeness?: string;
};
type CaseValue = {
  case_id: string;
  source_run_id: string;
  plan_id?: string;
  receipt_id?: string;
  blueprint_id?: string;
  blueprint_version?: string;
  adapter_id?: string;
  runtime?: string;
  task_features: Record<string, string>;
  delivery: Delivery;
  outcome_run_ids: string[];
  outcome_metrics: Array<{ name: string; value: unknown; confidence: number }>;
  cost: {
    tokens?: number;
    latency_ms?: number;
    money_minor?: number;
    safety_events?: number;
  };
  evaluation_object_ids: string[];
  result: "pass" | "fail" | "unknown";
  primary_failure_dimension?: string;
  secondary_failure_dimensions: string[];
  diagnosis?: string;
  counterfactual_hypothesis?: string;
  transfer_scope: string[];
  expires_at?: string;
  source_artifact_refs: string[];
  generated_at: string;
};
type Evaluation = {
  evaluation_id: string;
  verdict: string;
  protocol: string;
  evaluator_id: string;
  evaluator_version: string;
  dimensions: Array<{
    name: string;
    verdict: string;
    confidence: number;
    note?: string;
  }>;
  expected?: string;
  observed?: string;
  confidence: number;
  evaluated_at: string;
};
type PatternValue = {
  pattern_id: string;
  normalized_pattern: string;
  supporting_case_ids: string[];
  counterexample_case_ids: string[];
  target_components: string[];
  conditions: string[];
  expected_effect: string;
  confidence: number;
  sample_size: number;
  evaluation_object_ids: string[];
  known_regressions: string[];
  negative_domains: string[];
  last_validated?: string;
};
type CaseDetail = {
  object: HarnessObject;
  case: CaseValue;
  evaluations: Evaluation[];
};
type PatternDetail = {
  object: HarnessObject;
  pattern: PatternValue;
  evaluations: Evaluation[];
};

const payload = <T,>(object: HarnessObject) =>
  object.revision.payload as unknown as T;
const verdictLabel: Record<string, string> = {
  pass: "通过",
  fail: "失败",
  unknown: "未知",
};
const stamp = (value?: string) =>
  value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
export function ExperienceBank({
  api,
  projectID,
}: {
  api: APIClient;
  projectID: string;
}) {
  const [cases, setCases] = useState<HarnessObject[]>([]);
  const [patterns, setPatterns] = useState<HarnessObject[]>([]);
  const [selectedCase, setSelectedCase] = useState<CaseDetail>();
  const [selectedPattern, setSelectedPattern] = useState<PatternDetail>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [patternOpen, setPatternOpen] = useState(false);
  const load = useCallback(async () => {
    if (!projectID) return;
    setLoading(true);
    setError("");
    try {
      const [caseResult, patternResult] = await Promise.all([
        api.get<{ cases: HarnessObject[] }>(
          `/v1/experience/cases?project_id=${encodeURIComponent(projectID)}&limit=200`,
        ),
        api.get<{ patterns: HarnessObject[] }>(
          `/v1/experience/patterns?project_id=${encodeURIComponent(projectID)}&limit=100`,
        ),
      ]);
      setCases(caseResult.cases || []);
      setPatterns(patternResult.patterns || []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setLoading(false);
    }
  }, [api, projectID]);
  useEffect(() => {
    void load();
  }, [load]);
  const counts = useMemo(
    () => ({
      unknown: cases.filter((x) => payload<CaseValue>(x).result === "unknown")
        .length,
      fail: cases.filter((x) => payload<CaseValue>(x).result === "fail").length,
      pass: cases.filter((x) => payload<CaseValue>(x).result === "pass").length,
      active: cases.filter((x) => x.status === "active").length,
    }),
    [cases],
  );
  async function rebuild() {
    setLoading(true);
    setError("");
    setNotice("");
    try {
      const result = await api.post<{ total: number }>(
        `/v1/projects/${encodeURIComponent(projectID)}/experience/rebuild`,
        {},
      );
      setNotice(
        `已从可核验 Run 发现 / 重建 ${result.total} 个 Case；原始 Trace 不会自动成为长期记忆。`,
      );
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setLoading(false);
    }
  }
  async function openCase(id: string) {
    try {
      setSelectedCase(
        await api.get<CaseDetail>(
          `/v1/experience/cases/${encodeURIComponent(id)}`,
        ),
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function openPattern(id: string) {
    try {
      setSelectedPattern(
        await api.get<PatternDetail>(
          `/v1/experience/patterns/${encodeURIComponent(id)}`,
        ),
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <div className="experience-bank">
      <section className="experience-hero">
        <div>
          <p>EXPERIENCE BANK · EVALUATION FIRST</p>
          <h2>把真实 Run 变成可反证的经验，而不是把“成功日志”当真理。</h2>
          <span>
            Delivery、Outcome 与 Evaluation 分开记录。Case / Pattern
            只有经过独立 Evaluation 和 Owner Review 才能成为
            active；它们不会进入默认用户事实图。
          </span>
        </div>
        <div className="experience-actions">
          <button
            className="button light"
            disabled={loading}
            onClick={() => void rebuild()}
          >
            <RefreshCw size={14} />
            从真实 Run 发现 Case
          </button>
          <button className="button" onClick={() => setPatternOpen(true)}>
            <GitBranch size={14} />
            提出 Pattern
          </button>
        </div>
      </section>
      {notice && (
        <p className="surface-notice">
          <ShieldCheck size={14} />
          {notice}
        </p>
      )}
      {error && <p className="form-error inline-error">{error}</p>}
      <div className="experience-metrics">
        <article>
          <span>待评价 / Unknown</span>
          <strong>{counts.unknown}</strong>
        </article>
        <article className="danger">
          <span>负结果 / Fail</span>
          <strong>{counts.fail}</strong>
        </article>
        <article>
          <span>通过评价 / Pass</span>
          <strong>{counts.pass}</strong>
        </article>
        <article>
          <span>已治理 Active</span>
          <strong>{counts.active}</strong>
        </article>
      </div>
      <section className="experience-section">
        <header>
          <div>
            <Beaker size={17} />
            <span>
              <strong>Case Experience</strong>
              <small>
                Run + Context Receipt + Outcome → Candidate → Evaluation →
                Review
              </small>
            </span>
          </div>
        </header>
        {loading && !cases.length ? (
          <div className="loading-state">
            <span />
            <p>正在读取 Experience Case…</p>
          </div>
        ) : cases.length ? (
          <div className="experience-case-grid">
            {cases.map((object) => (
              <CaseCard
                key={object.object_id}
                object={object}
                onOpen={() => void openCase(object.object_id)}
              />
            ))}
          </div>
        ) : (
          <div className="empty">
            <span>
              <Beaker size={20} />
            </span>
            <h3>还没有 Experience Case</h3>
            <p>
              点击“从真实 Run 发现 Case”。只有拥有可核验 Context Plan / Receipt
              / Outcome 的终态 Run 才会进入候选。
            </p>
          </div>
        )}
      </section>
      <section className="experience-section">
        <header>
          <div>
            <GitBranch size={17} />
            <span>
              <strong>Diagnostic Pattern</strong>
              <small>至少两个已治理 Case 才能提出；反例永远单独保留。</small>
            </span>
          </div>
        </header>
        {patterns.length ? (
          <div className="experience-pattern-list">
            {patterns.map((object) => (
              <PatternCard
                key={object.object_id}
                object={object}
                onOpen={() => void openPattern(object.object_id)}
              />
            ))}
          </div>
        ) : (
          <div className="empty">
            <span>
              <GitBranch size={20} />
            </span>
            <h3>还没有 Pattern</h3>
            <p>单次成功或失败不会自动形成全局规律；先积累并治理多个 Case。</p>
          </div>
        )}
      </section>
      {selectedCase && (
        <CaseDrawer
          api={api}
          detail={selectedCase}
          close={() => setSelectedCase(undefined)}
          changed={async () => {
            await load();
            await openCase(selectedCase.object.object_id);
          }}
        />
      )}
      {selectedPattern && (
        <PatternDrawer
          api={api}
          detail={selectedPattern}
          close={() => setSelectedPattern(undefined)}
          changed={async () => {
            await load();
            await openPattern(selectedPattern.object.object_id);
          }}
        />
      )}
      {patternOpen && (
        <PatternModal
          api={api}
          projectID={projectID}
          cases={cases.filter((item) => item.status === "active")}
          close={() => setPatternOpen(false)}
          changed={load}
        />
      )}
    </div>
  );
}
function CaseCard({
  object,
  onOpen,
}: {
  object: HarnessObject;
  onOpen: () => void;
}) {
  const value = payload<CaseValue>(object);
  const delivery = value.delivery;
  const complete =
    delivery.total > 0 &&
    delivery.delivered === delivery.total &&
    delivery.failed === 0 &&
    delivery.denied === 0 &&
    delivery.delivery_unverified === 0;
  return (
    <button onClick={onOpen} className={`experience-case ${value.result}`}>
      <header>
        <span className={`eval-chip ${value.result}`}>
          {verdictLabel[value.result] || value.result}
        </span>
        <em>{object.status}</em>
      </header>
      <h3>{value.primary_failure_dimension || "尚未形成失败诊断"}</h3>
      <p>
        {value.diagnosis ||
          "等待独立 Evaluation；Run completed 与 Context delivered 都不是正确性证明。"}
      </p>
      <div className="experience-case-proof">
        <span>
          DELIVERY{" "}
          <b>
            {delivery.delivered}/{delivery.total}
          </b>
        </span>
        <span>
          OUTCOME <b>{value.outcome_run_ids.length}</b>
        </span>
        <span>
          EVAL <b>{value.evaluation_object_ids.length}</b>
        </span>
      </div>
      <footer>
        <span>
          {complete ? "送达已验证" : "送达不完整"} ·{" "}
          {value.runtime || "runtime unknown"}
        </span>
        <code>{value.source_run_id.slice(0, 18)}</code>
      </footer>
    </button>
  );
}

function PatternCard({
  object,
  onOpen,
}: {
  object: HarnessObject;
  onOpen: () => void;
}) {
  const value = payload<PatternValue>(object);
  return (
    <button onClick={onOpen}>
      <div>
        <GitBranch size={15} />
        <span>{object.status}</span>
      </div>
      <main>
        <strong>{value.normalized_pattern}</strong>
        <small>{value.expected_effect}</small>
      </main>
      <aside>
        <b>{value.supporting_case_ids.length}</b>
        <span>支持</span>
        <b>{value.counterexample_case_ids.length}</b>
        <span>反例</span>
      </aside>
    </button>
  );
}

function CaseDrawer({
  api,
  detail,
  close,
  changed,
}: {
  api: APIClient;
  detail: CaseDetail;
  close: () => void;
  changed: () => Promise<void>;
}) {
  const [mode, setMode] = useState<"inspect" | "evaluate">("inspect");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const value = detail.case;
  const object = detail.object;
  async function evaluate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    const form = new FormData(event.currentTarget);
    try {
      const verdict = String(form.get("verdict") || "unknown");
      const dimension = String(
        form.get("dimension") || "task_correctness",
      ).trim();
      await api.post(
        `/v1/experience/cases/${encodeURIComponent(object.object_id)}/evaluations`,
        {
          protocol: "owner-manual",
          evaluator_id: "local-owner",
          evaluator_version: "1.0.0",
          verdict,
          dimensions: [
            {
              name: dimension,
              verdict,
              confidence: 1,
              note: String(form.get("note") || ""),
            },
          ],
          expected: String(form.get("expected") || ""),
          observed: String(form.get("observed") || ""),
          confidence: 1,
          sample_size: 1,
          notes: String(form.get("note") || ""),
          idempotency_key: `owner-eval-${Date.now()}`,
        },
      );
      setNotice("Evaluation 已作为独立受治理对象保存；原始 Run 未被修改。");
      setMode("inspect");
      await changed();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }
  async function proposeActivation() {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const review = await api.post<{ review_id: string }>(
        `/v1/experience/cases/${encodeURIComponent(object.object_id)}/activation-proposal`,
        {
          expected_revision: object.current_revision,
          edit_reason:
            "Owner requests governed Experience activation after Evaluation",
          idempotency_key: `experience-activate-${Date.now()}`,
        },
      );
      setNotice(
        `已创建待审核 Revision ${review.review_id}；current 仍是 candidate，请在“待我审核”中最终决定。`,
      );
      await changed();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="drawer-backdrop" onMouseDown={close}>
      <aside
        className="drawer experience-drawer"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button className="drawer-close" onClick={close}>
          ×
        </button>
        <p className="micro">EXPERIENCE CASE · {value.result.toUpperCase()}</p>
        <h2>{value.primary_failure_dimension || "Evaluation pending"}</h2>
        <div className="drawer-status">
          <span>R{object.current_revision}</span>
          <span>{object.status}</span>
          <span>Evaluation {verdictLabel[value.result]}</span>
          <span>
            {value.delivery.delivered}/{value.delivery.total} delivered
          </span>
        </div>
        {notice && <p className="surface-notice">{notice}</p>}
        {error && <p className="form-error">{error}</p>}
        <div className="governance-tabs">
          <button
            className={mode === "inspect" ? "active" : ""}
            onClick={() => setMode("inspect")}
          >
            证据与诊断
          </button>
          <button
            className={mode === "evaluate" ? "active" : ""}
            onClick={() => setMode("evaluate")}
          >
            独立评价 <em>{detail.evaluations.length}</em>
          </button>
        </div>
        {mode === "inspect" && (
          <>
            <div className="experience-proof-grid">
              <article>
                <small>CONTEXT DELIVERY</small>
                <strong>
                  {value.delivery.delivered}/{value.delivery.total}
                </strong>
                <span>
                  {value.delivery.evidence_level || "unverified"} ·{" "}
                  {value.delivery.completeness || "unknown"}
                </span>
              </article>
              <article>
                <small>RUNTIME OUTCOME</small>
                <strong>{value.outcome_run_ids.length}</strong>
                <span>Observation only</span>
              </article>
              <article className={value.result === "fail" ? "danger" : ""}>
                <small>EVALUATION</small>
                <strong>{verdictLabel[value.result]}</strong>
                <span>{value.evaluation_object_ids.length} object(s)</span>
              </article>
              <article>
                <small>COST</small>
                <strong>{value.cost.tokens || 0}</strong>
                <span>{value.cost.latency_ms || 0} ms</span>
              </article>
            </div>
            <p className="experience-boundary">
              <AlertTriangle size={14} />
              “送达”“回合完成”和“任务正确”是三个不同事实。只有 Evaluation
              可以表达任务结果。
            </p>
            <dl className="experience-lineage">
              <div>
                <dt>Source Run</dt>
                <dd>{value.source_run_id}</dd>
              </div>
              <div>
                <dt>Plan / Receipt</dt>
                <dd>
                  {value.plan_id || "—"} / {value.receipt_id || "—"}
                </dd>
              </div>
              <div>
                <dt>Blueprint</dt>
                <dd>
                  {value.blueprint_id || "—"}@{value.blueprint_version || "—"}
                </dd>
              </div>
              <div>
                <dt>Adapter</dt>
                <dd>
                  {value.adapter_id || "—"} · {value.runtime || "—"}
                </dd>
              </div>
              <div>
                <dt>Diagnosis</dt>
                <dd>{value.diagnosis || "No causal diagnosis."}</dd>
              </div>
            </dl>
            {object.status === "candidate" && detail.evaluations.length > 0 && (
              <button
                className="button primary"
                disabled={busy}
                onClick={() => void proposeActivation()}
              >
                <ShieldCheck size={13} />
                创建待审核激活 Revision
              </button>
            )}
          </>
        )}
        {mode === "evaluate" && (
          <>
            <form className="experience-eval-form" onSubmit={evaluate}>
              <div className="form-grid">
                <label>
                  评价结论
                  <select name="verdict" defaultValue="unknown">
                    <option value="unknown">Unknown / 证据不足</option>
                    <option value="pass">Pass / 达成</option>
                    <option value="fail">Fail / 未达成</option>
                  </select>
                </label>
                <label>
                  评价维度
                  <input
                    name="dimension"
                    defaultValue="task_correctness"
                    required
                  />
                </label>
              </div>
              <label>
                期望结果
                <textarea
                  name="expected"
                  rows={3}
                  placeholder="独立于模型输出定义可验证期望"
                />
              </label>
              <label>
                观察结果
                <textarea
                  name="observed"
                  rows={4}
                  placeholder="例如：NO_CONTEXT；不要粘贴隐藏推理"
                />
              </label>
              <label>
                评价说明
                <textarea name="note" rows={3} />
              </label>
              <p className="token-warning">
                Evaluation 是独立对象；不会改写 Source Run、Receipt 或
                Outcome。无评价或证据不足应保持 Unknown。
              </p>
              <button className="button primary" disabled={busy}>
                <CheckCircle2 size={13} />
                保存独立 Evaluation
              </button>
            </form>
            <div className="experience-evaluation-list">
              {detail.evaluations.map((item) => (
                <article key={item.evaluation_id} className={item.verdict}>
                  <header>
                    <span className={`eval-chip ${item.verdict}`}>
                      {verdictLabel[item.verdict]}
                    </span>
                    <strong>{item.protocol}</strong>
                    <time>{stamp(item.evaluated_at)}</time>
                  </header>
                  <p>
                    {item.dimensions
                      .map((x) => `${x.name}: ${x.verdict}`)
                      .join(" · ")}
                  </p>
                  <small>
                    {item.observed || item.expected || "No text observation"}
                  </small>
                </article>
              ))}
            </div>
          </>
        )}
      </aside>
    </div>
  );
}

function PatternDrawer({ api, detail, close, changed }: { api: APIClient; detail: PatternDetail; close: () => void; changed: () => Promise<void> }) {
  const value = detail.pattern;
  const [mode, setMode] = useState<'inspect' | 'evaluate'>('inspect');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  async function evaluate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(''); setNotice('');
    const form = new FormData(event.currentTarget);
    try {
      const verdict = String(form.get('verdict') || 'unknown');
      await api.post(`/v1/experience/patterns/${encodeURIComponent(detail.object.object_id)}/evaluations`, {
        protocol: 'owner-pattern-review', evaluator_id: 'local-owner', evaluator_version: '1.0.0', verdict,
        dimensions: [{ name: 'evidence_support', verdict, confidence: 1, note: String(form.get('note') || '') }],
        expected: String(form.get('expected') || ''), observed: String(form.get('observed') || ''), confidence: 1,
        sample_size: value.sample_size, idempotency_key: `owner-pattern-eval-${Date.now()}`,
      });
      setNotice('Pattern Evaluation 已独立保存；Pattern 仍不会自动改写任何策略。'); setMode('inspect'); await changed();
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); } finally { setBusy(false); }
  }
  async function activate() {
    setBusy(true); setError(''); setNotice('');
    try {
      const review = await api.post<{ review_id: string }>(`/v1/experience/patterns/${encodeURIComponent(detail.object.object_id)}/activation-proposal`, {
        expected_revision: detail.object.current_revision, edit_reason: 'Owner requests governed diagnostic Pattern activation after Evaluation', idempotency_key: `pattern-activate-${Date.now()}`,
      });
      setNotice(`已创建待审核 Revision ${review.review_id}；不会触发 Blueprint / Prompt / Skill 自动修改。`); await changed();
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); } finally { setBusy(false); }
  }
  return <div className="drawer-backdrop" onMouseDown={close}><aside className="drawer experience-drawer" onMouseDown={event=>event.stopPropagation()}>
    <button className="drawer-close" onClick={close}>×</button><p className="micro">DIAGNOSTIC PATTERN</p><h2>{value.normalized_pattern}</h2>
    <div className="drawer-status"><span>R{detail.object.current_revision}</span><span>{detail.object.status}</span><span>{value.sample_size} cases</span><span>{value.evaluation_object_ids.length} eval</span></div>
    {notice&&<p className="surface-notice">{notice}</p>}{error&&<p className="form-error">{error}</p>}
    <div className="governance-tabs"><button className={mode==='inspect'?'active':''} onClick={()=>setMode('inspect')}>Pattern 证据</button><button className={mode==='evaluate'?'active':''} onClick={()=>setMode('evaluate')}>独立评价 <em>{detail.evaluations.length}</em></button></div>
    {mode==='inspect'&&<><p className="drawer-lead">{value.expected_effect}</p><div className="experience-proof-grid"><article><small>SUPPORT</small><strong>{value.supporting_case_ids.length}</strong><span>governed cases</span></article><article className="danger"><small>COUNTEREXAMPLE</small><strong>{value.counterexample_case_ids.length}</strong><span>retained separately</span></article><article><small>CONFIDENCE</small><strong>{Math.round(value.confidence*100)}%</strong><span>not causal truth</span></article><article><small>EVALUATION</small><strong>{value.evaluation_object_ids.length}</strong><span>independent objects</span></article></div><dl className="experience-lineage"><div><dt>Target components</dt><dd>{value.target_components.join(', ')||'—'}</dd></div><div><dt>Conditions</dt><dd>{value.conditions.join(' · ')||'—'}</dd></div><div><dt>Known regressions</dt><dd>{value.known_regressions.join(' · ')||'None recorded'}</dd></div><div><dt>Negative domains</dt><dd>{value.negative_domains.join(' · ')||'None recorded'}</dd></div><div><dt>Last validated</dt><dd>{stamp(value.last_validated)}</dd></div></dl><p className="experience-boundary"><AlertTriangle size={14}/>Pattern 是诊断候选，不会自动修改 Blueprint、Prompt、Skill 或权限。</p>{detail.object.status==='candidate'&&detail.evaluations.length>0&&<button className="button primary" disabled={busy} onClick={()=>void activate()}><ShieldCheck size={13}/>创建待审核 Pattern Revision</button>}</>}
    {mode==='evaluate'&&<><form className="experience-eval-form" onSubmit={evaluate}><label>评价结论<select name="verdict" defaultValue="unknown"><option value="unknown">Unknown / 证据不足</option><option value="pass">Pass / 支持</option><option value="fail">Fail / 不支持</option></select></label><label>期望证据<textarea name="expected" rows={3}/></label><label>观察证据<textarea name="observed" rows={4}/></label><label>说明<textarea name="note" rows={3}/></label><p className="token-warning">评价 Pattern 只更新 Evaluation 链；不会触发自动策略变更。</p><button className="button primary" disabled={busy}><CheckCircle2 size={13}/>保存 Pattern Evaluation</button></form><div className="experience-evaluation-list">{detail.evaluations.map(item=><article key={item.evaluation_id} className={item.verdict}><header><span className={`eval-chip ${item.verdict}`}>{verdictLabel[item.verdict]}</span><strong>{item.protocol}</strong><time>{stamp(item.evaluated_at)}</time></header><p>{item.dimensions.map(x=>`${x.name}: ${x.verdict}`).join(' · ')}</p></article>)}</div></>}
  </aside></div>;
}
function PatternModal({
  api,
  projectID,
  cases,
  close,
  changed,
}: {
  api: APIClient;
  projectID: string;
  cases: HarnessObject[];
  close: () => void;
  changed: () => Promise<void>;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const form = new FormData(event.currentTarget);
    const support = form.getAll("support").map(String);
    const counter = form.getAll("counter").map(String);
    if (support.length < 2) {
      setError("至少选择两个已治理 active Case 作为支持案例。");
      return;
    }
    setBusy(true);
    try {
      await api.post("/v1/experience/patterns", {
        project_id: projectID,
        normalized_pattern: String(form.get("pattern") || ""),
        supporting_case_ids: support,
        counterexample_case_ids: counter,
        target_components: String(form.get("targets") || "")
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean),
        conditions: String(form.get("conditions") || "")
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
        expected_effect: String(form.get("effect") || ""),
        confidence: Number(form.get("confidence") || 0.5),
        known_regressions: String(form.get("regressions") || "")
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
        negative_domains: String(form.get("negative") || "")
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
        idempotency_key: `owner-pattern-${Date.now()}`,
      });
      close();
      await changed();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="modal-backdrop" onMouseDown={close}>
      <form
        className="modal wide experience-pattern-modal"
        onSubmit={submit}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <p className="micro">PATTERN CANDIDATE</p>
        <h2>从多个已治理 Case 提出诊断 Pattern</h2>
        <p className="token-warning">
          Pattern 不是自动学习结果。至少两个 active Case
          支持；反例必须单列，且不能和支持案例重叠。
        </p>
        {error && <p className="form-error">{error}</p>}
        <label>
          归一化诊断模式
          <textarea
            name="pattern"
            rows={3}
            required
            placeholder="描述跨案例重复出现的诊断模式，不写成因果定律"
          />
        </label>
        <label>
          预期作用
          <textarea
            name="effect"
            rows={3}
            required
            placeholder="若未来验证该模式，可能改善什么"
          />
        </label>
        <div className="form-grid">
          <label>
            目标组件（逗号分隔）
            <input name="targets" placeholder="context.presentation-policy" />
          </label>
          <label>
            置信度
            <input
              name="confidence"
              type="number"
              min="0"
              max="1"
              step="0.05"
              defaultValue="0.5"
            />
          </label>
        </div>
        <label>
          条件（每行一个）
          <textarea name="conditions" rows={3} />
        </label>
        <fieldset>
          <legend>支持案例（至少 2 个）</legend>
          <div className="experience-case-picks">
            {cases.map((object) => {
              const value = payload<CaseValue>(object);
              return (
                <label key={`support-${object.object_id}`}>
                  <input
                    type="checkbox"
                    name="support"
                    value={object.object_id}
                  />
                  <span>
                    <strong>
                      {value.primary_failure_dimension ||
                        verdictLabel[value.result]}
                    </strong>
                    <small>
                      {value.source_run_id.slice(0, 20)} ·{" "}
                      {verdictLabel[value.result]}
                    </small>
                  </span>
                </label>
              );
            })}
          </div>
        </fieldset>
        <fieldset>
          <legend>反例（可选，但会永久单独保留）</legend>
          <div className="experience-case-picks">
            {cases.map((object) => {
              const value = payload<CaseValue>(object);
              return (
                <label key={`counter-${object.object_id}`}>
                  <input
                    type="checkbox"
                    name="counter"
                    value={object.object_id}
                  />
                  <span>
                    <strong>
                      {value.primary_failure_dimension ||
                        verdictLabel[value.result]}
                    </strong>
                    <small>
                      {value.source_run_id.slice(0, 20)} ·{" "}
                      {verdictLabel[value.result]}
                    </small>
                  </span>
                </label>
              );
            })}
          </div>
        </fieldset>
        <div className="form-grid">
          <label>
            已知回归（每行一个）
            <textarea name="regressions" rows={3} />
          </label>
          <label>
            负向领域（每行一个）
            <textarea name="negative" rows={3} />
          </label>
        </div>
        <div className="modal-actions">
          <button type="button" className="button" onClick={close}>
            取消
          </button>
          <button className="button primary" disabled={busy}>
            <GitBranch size={13} />
            创建 Pattern Candidate
          </button>
        </div>
      </form>
    </div>
  );
}
