import { useCallback, useEffect, useMemo, useState } from "react";
import { Archive, CheckCircle2, Download, FileCheck2, RefreshCw, ShieldAlert, Upload } from "lucide-react";
import { APIClient, HarnessObject } from "./api";

type Manifest = {
  bundle_id: string; bundle_hash: string; source_project_id?: string;
  object_count: number; evidence_count: number; required_capabilities: string[];
  signature: { status: string; algorithm: string };
};
type Finding = { severity: string; code: string; subject: string; detail: string };
type Report = {
  compatible: boolean; blocked: boolean; missing_capabilities: string[];
  unmapped_object_types: string[]; findings: Finding[]; degradations: string[];
  permission_delta: string[]; presentation_fallback: boolean; import_mode: string;
};
type Preflight = { path: string; manifest: Manifest; report: Report };
type ExportResult = { path: string; manifest: Manifest };
type ImportResult = {
  bundle_id: string; target_project_id: string; evidence_imported: number;
  evidence_duplicates: number; candidate_object_ids: string[]; no_direct_activation: boolean;
};

const short = (value?: string) => value ? `${value.slice(0, 18)}…${value.slice(-8)}` : "—";
const errText = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

export function PortableBundleCenter({ api, projectID }: { api: APIClient; projectID: string }) {
  const [objects, setObjects] = useState<HarnessObject[]>([]);
  const [objectTotal, setObjectTotal] = useState(0);
  const [objectHasMore, setObjectHasMore] = useState(false);
  const [objectsLoading, setObjectsLoading] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [includeDependencies, setIncludeDependencies] = useState(true);
  const [preflight, setPreflight] = useState<Preflight>();
  const [lastExport, setLastExport] = useState<ExportResult>();
  const [lastImport, setLastImport] = useState<ImportResult>();
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = useCallback(async (offset = 0, replace = true) => {
    if (!projectID) return;
    setObjectsLoading(true);
    try {
      const result = await api.get<{ objects: HarnessObject[]; total?: number; has_more?: boolean }>(
        `/v1/harness/objects?project_id=${encodeURIComponent(projectID)}&limit=100&offset=${offset}`,
      );
      const incoming = result.objects || [];
      setObjectTotal(result.total ?? incoming.length);
      setObjectHasMore(Boolean(result.has_more));
      setObjects((current) => {
        if (replace) return incoming;
        const byID = new Map(current.map((item) => [item.object_id, item]));
        for (const item of incoming) byID.set(item.object_id, item);
        return Array.from(byID.values());
      });
    } finally {
      setObjectsLoading(false);
    }
  }, [api, projectID]);
  useEffect(() => { void load(0, true).catch((reason) => setError(errText(reason))); }, [load]);
  useEffect(() => {
    setSelected([]); setPreflight(undefined); setLastImport(undefined); setConfirmed(false);
    setObjects([]); setObjectTotal(0); setObjectHasMore(false);
  }, [projectID]);

  const candidates = useMemo(() => objects.filter((item) => item.type_id === "builtin.portable-bundle.import-candidate.v1"), [objects]);
  const selectable = useMemo(() => objects.filter((item) => item.type_id !== "builtin.portable-bundle.import-candidate.v1"), [objects]);
  async function exportBundle() {
    setBusy(true); setError(""); setNotice("");
    try {
      const method = window.go?.main?.DesktopBridge?.ExportPortableBundle;
      if (!method) throw new Error("Portable Bundle 导出只在桌面应用中开放");
      if (!selected.length) throw new Error("至少选择一个根对象");
      const result = await method(projectID, selected, includeDependencies) as ExportResult;
      setLastExport(result);
      setNotice(`已导出 ${result.manifest.object_count} 个对象 / ${result.manifest.evidence_count} 条 Evidence。`);
    } catch (reason) { setError(errText(reason)); }
    finally { setBusy(false); }
  }

  async function inspectBundle() {
    setBusy(true); setError(""); setNotice(""); setConfirmed(false); setLastImport(undefined);
    try {
      const method = window.go?.main?.DesktopBridge?.PreflightPortableBundle;
      if (!method) throw new Error("Portable Bundle Preflight 只在桌面应用中开放");
      setPreflight(await method() as Preflight);
    } catch (reason) { setError(errText(reason)); }
    finally { setBusy(false); }
  }

  async function importBundle() {
    if (!preflight || preflight.report.blocked || !confirmed) return;
    setBusy(true); setError(""); setNotice("");
    try {
      const method = window.go?.main?.DesktopBridge?.ImportPortableBundle;
      if (!method) throw new Error("Portable Bundle 导入只在桌面应用中开放");
      const result = await method(
        projectID, preflight.path, preflight.manifest.bundle_id,
        preflight.manifest.bundle_hash, `owner-portable-${Date.now()}`,
      ) as ImportResult;
      setLastImport(result);
      setNotice(`导入完成：${result.evidence_imported} 条新 Evidence，${result.candidate_object_ids.length} 个受保护 Candidate。`);
      await load(0, true);
    } catch (reason) { setError(errText(reason)); }
    finally { setBusy(false); }
  }

  function toggle(id: string, checked: boolean) {
    setSelected((current) => checked ? Array.from(new Set([...current, id])) : current.filter((item) => item !== id));
  }

  const status = preflight?.report.blocked ? "BLOCKED" : preflight?.report.compatible ? "COMPATIBLE" : preflight ? "DEGRADED" : "NOT CHECKED";
  return <div className="experience-bank portable-bundle-center">
    <section className="experience-hero">
      <div><p>PORTABLE MEMORY BUNDLE · SELECTIVE MIGRATION</p>
        <h2>迁移的是对象、来源和治理语义，不是把另一台 Harness 的数据库整库搬进来。</h2>
        <span>导出保留 Revision / Hash / Source DAG / Capability。导入先 Preflight；外部 Evidence 进入 quarantine，Object 只生成 protected Candidate，永远不会直接 Active。</span>
      </div>
      <div className="experience-actions">
        <button className="button light" disabled={busy || objectsLoading} onClick={() => void load(0, true)}><RefreshCw size={14}/>刷新对象</button>
        <button className="button" disabled={busy} onClick={() => void inspectBundle()}><FileCheck2 size={14}/>选择 Bundle 并 Preflight</button>
      </div>
    </section>
    {notice && <p className="surface-notice"><CheckCircle2 size={14}/>{notice}</p>}
    {error && <p className="form-error inline-error">{error}</p>}
    <div className="experience-metrics">
      <article><span>已加载 / 总 Object</span><strong>{objects.length} / {objectTotal}</strong></article>
      <article><span>已选根对象</span><strong>{selected.length}</strong></article>
      <article><span>已隔离导入 Candidate</span><strong>{candidates.length}</strong></article>
      <article className={preflight?.report.blocked ? "danger" : ""}><span>Preflight</span><strong>{status}</strong></article>
    </div>
    <section className="experience-section">
      <header><div><Download size={17}/><span><strong>选择性导出</strong><small>选择根对象；Source Object / Evidence DAG 可随包带出。</small></span></div></header>
      <label className="portable-dependency-toggle"><input type="checkbox" checked={includeDependencies} onChange={(event) => setIncludeDependencies(event.target.checked)}/>包含来源依赖与 Evidence</label>
      <div className="portable-object-list">
        {selectable.map((object) => <label key={object.object_id} className="portable-object-row">
          <input type="checkbox" checked={selected.includes(object.object_id)} onChange={(event) => toggle(object.object_id, event.target.checked)}/>
          <span><strong>{object.type_id}</strong><small>{object.object_id} · {object.status} · R{object.current_revision} · {short(object.revision.content_hash)}</small></span>
        </label>)}
      </div>
      {objectHasMore && <button className="run-load-more" disabled={objectsLoading} onClick={() => void load(objects.length, false)}>{objectsLoading ? "正在加载…" : `加载更多 Object（${objects.length} / ${objectTotal}）`}</button>}
      {!selectable.length && <div className="empty"><span><Archive size={20}/></span><h3>当前项目还没有可迁移 Object</h3><p>先形成受治理 Object，再进行选择性 Portable Export。</p></div>}
      <div className="experience-actions portable-actions">
        <button className="button primary" disabled={busy || !selected.length} onClick={() => void exportBundle()}><Download size={14}/>导出所选 Bundle</button>
      </div>
      {lastExport && <div className="portable-result"><strong>{lastExport.manifest.bundle_id}</strong><span>{lastExport.path}</span><small>{lastExport.manifest.object_count} objects · {lastExport.manifest.evidence_count} evidence · {short(lastExport.manifest.bundle_hash)}</small></div>}
    </section>
    <section className="experience-section">
      <header><div><ShieldAlert size={17}/><span><strong>Import Preflight</strong><small>完整性、Schema、来源、能力、类型映射与指令注入扫描。</small></span></div></header>
      {!preflight ? <div className="empty"><span><FileCheck2 size={20}/></span><h3>还没有检查迁移包</h3><p>先选择本地 .mhbundle.tar.gz；Preflight 不会写入任何 Evidence 或 Object。</p></div> : <>
        <div className="experience-proof-grid">
          <article><small>BUNDLE</small><strong>{short(preflight.manifest.bundle_id)}</strong><span>{preflight.manifest.object_count} objects / {preflight.manifest.evidence_count} evidence</span></article>
          <article className={preflight.report.blocked ? "danger" : ""}><small>COMPATIBILITY</small><strong>{status}</strong><span>{preflight.report.import_mode}</span></article>
          <article><small>SIGNATURE</small><strong>{preflight.manifest.signature.status.toUpperCase()}</strong><span>{preflight.manifest.signature.algorithm}</span></article>
        </div>
        <div className="portable-report-grid">
          <ReportList title="缺失能力" values={preflight.report.missing_capabilities}/>
          <ReportList title="未映射 Object Type" values={preflight.report.unmapped_object_types}/>
          <ReportList title="显式降级" values={preflight.report.degradations}/>
          <ReportList title="权限增量" values={preflight.report.permission_delta}/>
        </div>
        <FindingList findings={preflight.report.findings}/>
      </>}
    </section>
    {preflight && <section className="experience-section portable-import-zone">
      <header><div><Upload size={17}/><span><strong>隔离导入当前项目</strong><small>兼容性不足可以降级保存，但 blocked Bundle 永远不能导入。</small></span></div></header>
      <label className="portable-confirm"><input type="checkbox" checked={confirmed} disabled={preflight.report.blocked} onChange={(event) => setConfirmed(event.target.checked)}/>
        <span><strong>我确认导入只产生 quarantine Evidence 与 protected Candidate。</strong><small>不会直接 Active，不会扩大权限，不会移动 Global Blueprint，也不会把外部指令当系统指令。</small></span>
      </label>
      <div className="experience-actions portable-actions">
        <button className="button primary" disabled={busy || preflight.report.blocked || !confirmed} onClick={() => void importBundle()}><Upload size={14}/>确认隔离导入</button>
      </div>
      {lastImport && <div className="portable-result"><strong>NO DIRECT ACTIVATION: {lastImport.no_direct_activation ? "YES" : "NO"}</strong><span>{lastImport.candidate_object_ids.length} candidates · {lastImport.evidence_imported} new evidence · {lastImport.evidence_duplicates} duplicates</span></div>}
    </section>}
  </div>;
}

function ReportList({ title, values }: { title: string; values: string[] }) {
  return <article><strong>{title}</strong>{values.length ? <ul>{values.map((value) => <li key={value}>{value}</li>)}</ul> : <span>无</span>}</article>;
}

function FindingList({ findings }: { findings: Finding[] }) {
  if (!findings.length) return <p className="surface-notice"><CheckCircle2 size={14}/>未发现注入或结构完整性风险信号。</p>;
  return <div className="portable-findings">{findings.map((item, index) => <article className={item.severity === "blocked" ? "danger" : ""} key={`${item.code}-${index}`}>
    <strong>{item.severity.toUpperCase()} · {item.code}</strong><span>{item.subject}</span><p>{item.detail}</p>
  </article>)}</div>;
}
