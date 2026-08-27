import {
  FormEvent,
  lazy,
  ReactNode,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  Activity,
	AlertTriangle,
  ArrowRight,
  Blocks,
  BookOpen,
  Bot,
  Box,
  BrainCircuit,
  CalendarClock,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleDollarSign,
  Copy,
  Database,
  Download,
  FileInput,
  FileSearch,
  Fingerprint,
  FolderPlus,
  GitBranch,
  HeartPulse,
  HelpCircle,
  Home,
  KeyRound,
  Layers3,
  Library,
  LockKeyhole,
  Menu,
  Network,
  PackageCheck,
  PanelLeftClose,
  Pin,
  PinOff,
  Play,
  PlugZap,
  Plus,
  RefreshCw,
  Route,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Upload,
  UserPlus,
  XCircle,
} from "lucide-react";
import {
  APIClient,
  connect,
  ConnectionState,
  HarnessObject,
  HarnessRun,
  MemoryType,
  ModelUsageDashboard,
  ModelUsageSummary,
  PipelineVersion,
  PluginVersion,
  Project,
  RunDetail,
} from "./api";
import { ImportCenter } from "./ImportCenter";
import { ProjectWorkspace } from "./ProjectWorkspace";
import {
  CorrectionImpact,
  KnowledgeUnit,
  KnowledgeUnitCorrectionModal,
  KnowledgeUnitDrawer,
  SemanticGraph,
  SemanticGraphData,
} from "./MemorySemantics";
import { EntityCorrectionModal } from "./EntityCorrection";
import { MemoryGraphData, MemoryLineage } from "./MemoryLineage";
import { TemporalTimeline } from "./TemporalTimeline";
import { AssetGovernanceDrawer } from "./AssetGovernance";
import { AssetFoundry } from "./AssetFoundry";
import { KnowledgeProducts } from "./KnowledgeProducts";
import { ContextExchangeInspector } from "./ContextExchangeInspector";
import {
  ContextBridgeCapabilities,
  ContextBridgeManifest,
} from "./ContextBridgeCapabilities";
import { ProfileCenter } from "./ProfileCenter";
import { HelpCenter } from "./HelpCenter";
import { UIMode } from "./ui";
import { ActivityCalendar, ActivityCalendarData } from "./ActivityCalendar";
import { ReadableMarkdown, ReadableObject } from "./ReadableContent";

const TeamMemoryCenter = lazy(() => import("./TeamMemoryCenter").then((module) => ({ default: module.TeamMemoryCenter })));
const ExperienceBank = lazy(() => import("./ExperienceBank").then((module) => ({ default: module.ExperienceBank })));
const AdaptationLab = lazy(() => import("./AdaptationLab").then((module) => ({ default: module.AdaptationLab })));
const PortableBundleCenter = lazy(() => import("./PortableBundleCenter").then((module) => ({ default: module.PortableBundleCenter })));
const StrategyWorkbench = lazy(() => import("./StrategyWorkbench").then((module) => ({ default: module.StrategyWorkbench })));
const PipelineStudio = lazy(() => import("./PipelineStudio").then((module) => ({ default: module.PipelineStudio })));
const PluginCenter = lazy(() => import("./PluginCenter").then((module) => ({ default: module.PluginCenter })));

function DeferredPage({ children }: { children: ReactNode }) {
  return <Suspense fallback={<div className="loading-state"><span /><p>正在按需加载工作区…</p></div>}>{children}</Suspense>;
}

type PageID =
  | "home"
  | "import"
  | "projects"
  | "search"
  | "memory"
  | "profiles"
  | "team"
  | "products"
  | "experience"
  | "adaptation"
  | "portable"
  | "assets"
  | "timeline"
  | "runs"
  | "review"
  | "strategy"
  | "studio"
  | "plugins"
  | "connections"
  | "health"
  | "help";
type ProjectSummary = {
  project: Project;
  metrics: Record<string, number>;
  finance?: Record<string, unknown>;
};
type HomeTask = {
  task_id: string;
  title: string;
  description?: string;
  status: "suggested" | "todo" | "in_progress" | "done" | "dismissed";
  priority: number;
  due_at?: string;
  source_kind: "manual" | "ai_suggestion";
};

const navigation: Array<{
  group: string;
  advanced?: boolean;
  items: Array<{ id: PageID; label: string; hint: string; icon: typeof Home }>;
}> = [
  {
    group: "开始使用",
    items: [
      { id: "home", label: "记忆总览", hint: "Home", icon: Home },
      { id: "import", label: "导入记忆", hint: "Import", icon: FileInput },
      { id: "search", label: "检索", hint: "Recall", icon: Search },
    ],
  },
  {
    group: "记忆",
    items: [
      {
        id: "memory",
        label: "记忆库",
        hint: "Memory Library",
        icon: BrainCircuit,
      },
      {
        id: "profiles",
        label: "上下文画像",
        hint: "Profile Compiler",
        icon: Fingerprint,
      },
      {
        id: "team",
        label: "多 AI 协作",
        hint: "AI Collaboration",
        icon: UserPlus,
      },
      {
        id: "products",
        label: "知识产品",
        hint: "Reports & Briefs",
        icon: BookOpen,
      },
      {
        id: "assets",
        label: "能力资产工坊",
        hint: "Asset Foundry",
        icon: PackageCheck,
      },
      {
        id: "timeline",
        label: "时间脉络",
        hint: "Temporal Context",
        icon: CalendarClock,
      },
      { id: "projects", label: "项目工作台", hint: "Projects", icon: Library },
      { id: "review", label: "待我审核", hint: "Review", icon: ShieldCheck },
      { id: "runs", label: "运行记录", hint: "Runs", icon: Route },
    ],
  },
  {
    group: "高级构建",
    advanced: true,
    items: [
      {
        id: "experience",
        label: "经验评估",
        hint: "Experience Bank",
        icon: Activity,
      },
      {
        id: "adaptation",
        label: "适配实验室",
        hint: "Governed Adaptation",
        icon: Sparkles,
      },
      {
        id: "portable",
        label: "迁移与携带",
        hint: "Portable Bundle",
        icon: Download,
      },
      {
        id: "strategy",
        label: "策略工坊",
        hint: "Blueprint",
        icon: SlidersHorizontal,
      },
      { id: "studio", label: "流程编辑器", hint: "Pipelines", icon: GitBranch },
      { id: "plugins", label: "插件中心", hint: "Plugins", icon: Blocks },
    ],
  },
  {
    group: "设置",
    items: [
      {
        id: "connections",
        label: "模型与 Agent",
        hint: "Connections",
        icon: PlugZap,
      },
      { id: "health", label: "健康与恢复", hint: "Health", icon: HeartPulse },
      { id: "help", label: "操作手册", hint: "健康与恢复 / 使用指南", icon: BookOpen },
    ],
  },
];

const pageCopy: Record<
  PageID,
  { eyebrow: string; title: string; description: string }
> = {
  home: {
    eyebrow: "MEMORY HOME",
    title: "今天要做什么，记忆发生了什么",
    description:
      "先处理待办和审核，再查看当前空间的记忆状态；设置与运行细节按需打开。",
  },
  import: {
    eyebrow: "MEMORY IMPORT CENTER",
    title: "先把真实内容带进来",
    description:
      "文件、笔记和 AI 历史会话都会先保留原文，再只整理本次新增内容。",
  },
  projects: {
    eyebrow: "PROJECT OPERATING MEMORY",
    title: "先处理待办，再查看项目记忆",
    description: "今天到期、逾期、正在处理、AI 建议和待审核事项集中在一个工作台。",
  },
  search: {
    eyebrow: "SCOPED RECALL",
    title: "检索，并且知道为什么命中",
    description: "默认只在当前项目中召回；每个结果保留类型、来源和时间范围。",
  },
  memory: {
    eyebrow: "MEMORY LIBRARY",
    title: "查看记忆内容，也能追溯它从哪里来",
    description:
      "基础模式查看原材料、关键信息和项目记忆；高级模式再查看完整结构与技术信息。",
  },
  profiles: {
    eyebrow: "PROFILE COMPILER",
    title: "把长期记忆编译成按任务可用的上下文画像",
    description:
      "稳定身份、项目动态与会话恢复彼此分离；人工锁定内容不会被后续自动重建静默覆盖。",
  },
  team: {
    eyebrow: "MULTI-AI COLLABORATION",
    title: "让不同 AI 在同一任务中协作，又保留各自的私密草稿",
    description:
      "Memory Harness 管理参与者、共享对象、冲突和长期保存；不会自动启动或调度外部 AI。",
  },
  experience: {
    eyebrow: "EXPERIENCE BANK",
    title: "从真实运行形成可反证的经验，而不是自动学习",
    description:
      "Context Receipt、Outcome 与独立 Evaluation 分层记录；Case 与 Pattern 经过审核后才进入受治理经验库。",
  },
  adaptation: {
    eyebrow: "GOVERNED ADAPTATION LAB",
    title: "从失败经验提出局部改进，但不自动重写全局 Harness",
    description:
      "Change Proposal、独立 Evaluation、Owner Review、Case Overlay、Canary 与 Rollback 保持一条可审计链；Global Blueprint 默认不动。",
  },
  portable: {
    eyebrow: "PORTABLE MEMORY BUNDLE",
    title: "选择性携带记忆，并且知道目标 Harness 会丢失什么",
    description:
      "迁移保留 Object Revision、Evidence、来源 DAG 与能力标签；目标端先 Preflight，外部内容只进入 quarantine / Candidate。",
  },
  products: {
    eyebrow: "REVISIONED KNOWLEDGE PRODUCTS",
    title: "把长期记忆整理成可读、可改、可持续生长的知识产品",
    description:
      "项目简报自动更新；报告、日记、个人能力与档案保留 Evidence、Revision 和人工锁定字段。",
  },
  assets: {
    eyebrow: "GOVERNED ASSET FOUNDRY",
    title: "把可复用记忆变成真正受治理的 Agent 能力",
    description:
      "分类、编辑、Diff、服务端验证、Owner 审核、激活与回滚统一在不可变 Revision 链上。",
  },
  timeline: {
    eyebrow: "TEMPORAL MEMORY",
    title: "按时间理解记忆，而不只是按更新时间排序",
    description:
      "事件时间、有效时间、记录时间、截止时间和 Run 轨迹统一到一个可查询的项目时间坐标。",
  },
  runs: {
    eyebrow: "RUN EXPLORER",
    title: "每次沉淀都有完整轨迹",
    description:
      "Graph、Timeline 与副作用回执共同回答：系统做了什么，为什么，以及是否真的完成。",
  },
  review: {
    eyebrow: "HUMAN REVIEW GATE",
    title: "关键变化不会静默生效",
    description: "身份、程序、商机、资金和 Agent 资产在影响行为前停在这里。",
  },
  strategy: {
    eyebrow: "MEMORY BLUEPRINT WORKBENCH",
    title: "把整套记忆方式装配给当前项目",
    description:
      "生长层、空间结构与召回成本彼此独立；替换插件策略、增加层级、调整参数，再发布项目版本。",
  },
  studio: {
    eyebrow: "PIPELINE EDITOR",
    title: "编辑一条具体的处理流程",
    description: "克隆流程模板、调整步骤和审核门，再发布一个不可变版本。",
  },
  plugins: {
    eyebrow: "PLUGIN FOUNDATION",
    title: "基座稳定，策略可以持续进化",
    description:
      "记忆层、流程和业务模块通过受信插件加入；撤销信任不会抹掉历史。",
  },
  connections: {
    eyebrow: "GOVERNED CONNECTIONS",
    title: "让不同 AI 只获得需要的记忆",
    description:
      "Codex、ChatGPT、DeepSeek Harness 与模型使用不同凭据、项目范围和权限。",
  },
  health: {
    eyebrow: "INTEGRITY & RECOVERY",
    title: "知道系统现在是否值得信任",
    description:
      "数据库、Evidence、索引、密钥存储、插件和运行状态都有可核验结果。",
  },
  help: {
    eyebrow: "OPERATION MANUAL",
    title: "操作手册：每个功能能做什么、怎么操作",
    description: "从第一次导入到连接 Codex，逐项说明操作方法、系统自动行为和安全边界。",
  },
};

function pageFromHash(): PageID {
  const raw = window.location.hash.replace(/^#\/?/, "");
  const legacyAliases: Record<string, PageID> = {
    dashboard: "home",
    today: "home",
    project: "projects",
    sources: "import",
    episodes: "memory",
    records: "memory",
    assets: "assets",
    portfolio: "projects",
  };
  const value = (legacyAliases[raw] || raw) as PageID;
  return Object.hasOwn(pageCopy, value) ? value : "home";
}

function useAsync<T>(
  loader: (() => Promise<T>) | null,
  dependencies: unknown[],
) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(Boolean(loader));
  const reload = useCallback(() => {
    if (!loader) return;
    setLoading(true);
    setError("");
    loader()
      .then(setData)
      .catch((reason: unknown) =>
        setError(reason instanceof Error ? reason.message : String(reason)),
      )
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, dependencies);
  useEffect(reload, [reload]);
  return { data, error, loading, reload };
}

function Status({ value }: { value?: string }) {
  const normalized = (value || "unknown").toLowerCase();
  const good = [
    "ok",
    "pass",
    "active",
    "enabled",
    "completed",
    "published",
    "verified",
    "bundled",
    "healthy",
  ].includes(normalized);
  const danger = [
    "failed",
    "denied",
    "quarantined",
    "revoked",
    "degraded",
  ].includes(normalized);
  return (
    <span className={`status ${good ? "good" : danger ? "danger" : "warn"}`}>
      <i />
      {normalized.replaceAll("_", " ")}
    </span>
  );
}

function Empty({
  icon: Icon = Box,
  title,
  children,
}: {
  icon?: typeof Box;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="empty">
      <span>
        <Icon size={20} />
      </span>
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}

function LoadBoundary({
  loading,
  error,
  children,
}: {
  loading: boolean;
  error: string;
  children: ReactNode;
}) {
  if (loading)
    return (
      <div className="loading-state">
        <span />
        <p>正在读取真实数据…</p>
      </div>
    );
  if (error)
    return (
      <Empty icon={XCircle} title="这部分暂时无法读取">
        {error}
      </Empty>
    );
  return <>{children}</>;
}

function Metric({
  label,
  value,
  detail,
  tone = "",
}: {
  label: string;
  value: ReactNode;
  detail: string;
  tone?: string;
}) {
  return (
    <article className={`metric ${tone}`}>
      <small>{label}</small>
      <strong>{value}</strong>
      <p>{detail}</p>
    </article>
  );
}

function Section({
  title,
  detail,
  action,
  children,
  className = "",
}: {
  title: string;
  detail?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`section ${className}`}>
      <header>
        <div>
          <h2>{title}</h2>
          {detail && <p>{detail}</p>}
        </div>
        {action}
      </header>
      {children}
    </section>
  );
}

function time(value?: string) {
  if (!value) return "尚未记录";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? value
    : new Intl.DateTimeFormat("zh-CN", {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      }).format(parsed);
}

function Locked({
  connection,
  retry,
}: {
  connection: Extract<ConnectionState, { mode: "locked" }>;
  retry: () => void;
}) {
  return (
    <main className="locked-screen">
      <div className="locked-mark">
        <Fingerprint size={40} />
      </div>
      <p className="micro">OWNER CONTROL PLANE</p>
      <h1>
        内核可以被看见，
        <br />
        管理权只属于桌面应用。
      </h1>
      <p className="locked-copy">{connection.reason}</p>
      <div className="locked-facts">
        <div>
          <span>内核地址</span>
          <code>{connection.endpoint}</code>
        </div>
        <div>
          <span>公开健康检查</span>
          <Status value={String(connection.health?.status || "offline")} />
        </div>
        <div>
          <span>Owner API</span>
          <Status value="locked" />
        </div>
      </div>
      <button className="button primary" onClick={retry}>
        <RefreshCw size={15} />
        重新检测
      </button>
      <p className="locked-note">
        <LockKeyhole size={14} /> 普通浏览器、Agent token 和插件都不能创建
        Agent、修改模型密钥或批准受保护记忆。
      </p>
    </main>
  );
}

export function HomePage({
  api,
  project,
  summary,
  onNavigate,
  onCreateSpace,
}: {
  api: APIClient;
  project?: Project;
  summary?: ProjectSummary;
  onNavigate: (target: PageID) => void;
  onCreateSpace: () => void;
}) {
  const projectID = project?.project_id || "";
  const snapshot = useAsync(async () => {
    const [
      operations,
      episodes,
      tasks,
      activity,
      sources,
      pinned,
    ] = await Promise.all([
      api.get<{ operations: Array<Record<string, unknown>> }>(
        "/v1/operations?status=review_required&limit=8",
      ),
      projectID
        ? api.get<{ episodes: Array<Record<string, unknown>> }>(
            `/v1/episodes?project_id=${encodeURIComponent(projectID)}&limit=3`,
          )
        : Promise.resolve({ episodes: [] }),
      projectID
        ? api.get<{ tasks: HomeTask[] }>(
            `/v1/project-tasks?project_id=${encodeURIComponent(projectID)}`,
          )
        : Promise.resolve({ tasks: [] }),
      projectID
        ? api.get<ActivityCalendarData>(
            `/v1/projects/${encodeURIComponent(projectID)}/activity-calendar?days=28`,
          ).catch(() => undefined)
        : Promise.resolve(undefined),
      projectID
        ? api.get<{ sources: ProcessSource[] }>(
            `/v1/process/sources?project_id=${encodeURIComponent(projectID)}`,
          )
        : Promise.resolve({ sources: [] }),
      projectID
        ? api.get<{ memories: Array<Record<string, unknown>>; total: number }>(
            `/v1/memory-pins?project_id=${encodeURIComponent(projectID)}&limit=3`,
          )
        : Promise.resolve({ memories: [], total: 0 }),
    ]);
    return {
      operations: operations.operations,
      episodes: episodes.episodes,
      tasks: tasks.tasks,
      activity,
      sources: sources.sources,
      pinned: pinned.memories,
    };
  }, [api, projectID]);
  const projectEvidence = Number(summary?.metrics.evidence || 0);
  const latestDay = snapshot.data?.activity?.days[snapshot.data.activity.days.length - 1];
  const currentDay = latestDay?.date || new Intl.DateTimeFormat("sv-SE", { year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
  const dayOf = (value?: string) => {
    if (!value) return "";
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? value.slice(0, 10) : new Intl.DateTimeFormat("sv-SE", { year: "numeric", month: "2-digit", day: "2-digit" }).format(parsed);
  };
  const tasks = (snapshot.data?.tasks || []).filter((task) => ["suggested", "todo", "in_progress"].includes(task.status));
  const taskRank = (task: HomeTask) => {
    const due = dayOf(task.due_at);
    if (due && due < currentDay) return 0;
    if (due === currentDay) return 1;
    if (task.status === "in_progress") return 2;
    if (task.status === "suggested") return 3;
    return 4;
  };
  const activeTasks = [...tasks].sort((left, right) => taskRank(left) - taskRank(right) || left.priority - right.priority).slice(0, 7);
  const overdueCount = tasks.filter((task) => dayOf(task.due_at) && dayOf(task.due_at) < currentDay).length;
  const dueTodayCount = tasks.filter((task) => dayOf(task.due_at) === currentDay).length;
  const pendingSources = (snapshot.data?.sources || []).filter((source) => ["pending", "changed"].includes(source.status)).length;
  const failedSources = (snapshot.data?.sources || []).filter((source) => source.status === "failed").length;
  const reviewCount = snapshot.data?.operations.length || 0;
  const [pinError, setPinError] = useState("");
  async function unpin(memoryID: string) {
    setPinError("");
    try {
      await api.put(`/v1/memories/${encodeURIComponent(memoryID)}/pin`, { project_id: projectID, pinned: false });
      snapshot.reload();
    } catch (reason) {
      setPinError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <LoadBoundary loading={snapshot.loading} error={snapshot.error}>
      <section className="home-focus" aria-labelledby="home-today-title">
        <header className="home-focus-header">
          <div>
            <p>今天</p>
            <h2 id="home-today-title">先把真正需要你处理的事情放在这里</h2>
            <span>{project?.name || "当前记忆空间"} · 其余配置和运行细节已收进对应页面</span>
          </div>
          <div>
            <button className="button surface" onClick={() => onNavigate("import")}><FileInput size={14}/>导入原材料</button>
            <button className="button surface" onClick={onCreateSpace}><FolderPlus size={14}/>新建记忆空间</button>
          </div>
        </header>
        <div className="home-focus-grid">
          <section className="today-panel">
            <header><div><strong>今天要处理</strong><span>{overdueCount} 项逾期 · {dueTodayCount} 项今天到期 · {reviewCount} 项待审核</span></div><button className="text-button" onClick={() => onNavigate("projects")}>全部待办 <ArrowRight size={13}/></button></header>
            {reviewCount > 0 && <button className="today-action review" onClick={() => onNavigate("review")}><span><ShieldCheck size={16}/></span><div><strong>{reviewCount} 项变化需要你确认</strong><small>打开审核页查看内容、来源和影响后再决定</small></div><ArrowRight size={14}/></button>}
            {activeTasks.length ? <div className="today-action-list">{activeTasks.map((task) => {
              const due = dayOf(task.due_at);
              const overdue = Boolean(due && due < currentDay);
              const label = overdue ? "已逾期" : due === currentDay ? "今天到期" : task.status === "in_progress" ? "正在处理" : task.status === "suggested" ? "AI 建议" : "待开始";
              return <button className={`today-action${overdue ? " overdue" : ""}`} key={task.task_id} onClick={() => onNavigate("projects")}><span><CalendarClock size={16}/></span><div><strong>{task.title}</strong><small>{label}{task.due_at ? ` · ${time(task.due_at)}` : ""}</small></div><ArrowRight size={14}/></button>;
            })}</div> : reviewCount === 0 && projectEvidence > 0 ? <div className="today-clear"><CheckCircle2 size={20}/><div><strong>今天没有需要处理的事项</strong><span>你可以继续导入，或去检索已有记忆。</span></div></div> : null}
            {projectEvidence === 0 && <button className="today-first-step" onClick={() => onNavigate("import")}><FileInput size={20}/><div><strong>导入第一份原材料</strong><span>文件、笔记或 AI 会话均可；系统会保留原文并只整理本次新增内容。</span></div><ArrowRight size={15}/></button>}
          </section>
          <aside className="memory-pulse" aria-label="今日记忆状态">
            <header><div><strong>记忆状态</strong><span>只显示当前空间最有用的运行结果</span></div><button className="text-button" onClick={() => onNavigate("memory")}>打开记忆库 <ArrowRight size={13}/></button></header>
            <div className="memory-pulse-metrics">
              <article><strong>{latestDay?.evidence || 0}</strong><span>今天新增原材料</span></article>
              <article><strong>{latestDay?.knowledge_units || 0}</strong><span>今天生成关键信息</span></article>
              <article><strong>{latestDay?.memories || 0}</strong><span>今天形成项目记忆</span></article>
              <article className={failedSources ? "needs-attention" : ""}><strong>{pendingSources + failedSources}</strong><span>仍需整理或重试</span></article>
            </div>
            <div className="memory-pulse-note">
              <Sparkles size={15}/>
              <span>{failedSources ? `${failedSources} 份原材料处理失败，打开记忆库即可重新处理。` : pendingSources ? `${pendingSources} 份新增或变化的原材料等待整理。` : "当前没有失败或等待整理的原材料。"}</span>
            </div>
          </aside>
        </div>
      </section>
      <ActivityCalendar data={snapshot.data?.activity} />
      {pinError && <p className="form-error inline-error">{pinError}</p>}
      <div className="home-memory-summary">
        <section className="home-simple-section">
          <header><div><p>已固定的记忆</p><span>只有你主动固定的内容才会出现在首页</span></div><button className="text-button" onClick={() => onNavigate("memory")}>管理记忆 <ArrowRight size={13}/></button></header>
          {snapshot.data?.pinned.length ? <div className="pinned-memory-list">{snapshot.data.pinned.map((item, index) => <article key={String(item.memory_id || index)}><button className="pinned-memory-main" onClick={() => onNavigate("memory")}><Pin size={14}/><div><strong>{String(item.summary || "未命名记忆")}</strong><span>{String(item.body || "")}</span><small>{Array.isArray(item.source_evidence_ids) ? `${item.source_evidence_ids.length} 条来源` : "来源可追溯"} · 固定于 {time(String(item.pinned_at || ""))}</small></div></button><button className="pin-remove" aria-label={`取消固定 ${String(item.summary || "未命名记忆")}`} onClick={() => void unpin(String(item.memory_id))}><PinOff size={14}/></button></article>)}</div> : <div className="simple-empty"><Pin size={18}/><div><strong>还没有固定记忆</strong><span>到记忆库找到需要长期关注的内容，点击“固定到首页”。AI 不会替你决定什么最重要。</span></div></div>}
        </section>
        <section className="home-simple-section">
          <header><div><p>最近复盘</p><span>刚刚从原材料整理出的经历与结果</span></div><button className="text-button" onClick={() => onNavigate("memory")}>查看全部 <ArrowRight size={13}/></button></header>
          {snapshot.data?.episodes.length ? <div className="recent-episode-list">{snapshot.data.episodes.map((episode, index) => <button key={String(episode.episode_id || index)} onClick={() => onNavigate("memory")}><Route size={15}/><div><strong>{String(episode.title || "会话复盘")}</strong><span>{String(episode.summary || "")}</span><small>{time(String(episode.updated_at || episode.ended_at || ""))}</small></div><ArrowRight size={13}/></button>)}</div> : <div className="simple-empty"><Route size={18}/><div><strong>还没有复盘</strong><span>第一份原材料完成整理后会出现在这里。</span></div></div>}
        </section>
      </div>
    </LoadBoundary>
  );
}

function SearchPage({ api, projectID }: { api: APIClient; projectID: string }) {
  const [query, setQuery] = useState("");
  const [allProjects, setAllProjects] = useState(false);
  const [asOf, setAsOf] = useState("");
  const [includeHistory, setIncludeHistory] = useState(false);
  const [result, setResult] = useState<{
    hits?: Array<Record<string, unknown>>;
    context_id?: string;
    backend?: string;
    candidate_count?: number;
    took_ms?: number;
  }>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    const anchor = asOf ? new Date(asOf) : undefined;
    if (anchor && Number.isNaN(anchor.getTime())) {
      setError("时间锚点格式无效");
      setLoading(false);
      return;
    }
    try {
      setResult(
        await api.post("/v1/search/unified", {
          query,
          project_id: allProjects ? "" : projectID,
          all_projects: allProjects,
          limit: 30,
          as_of: anchor?.toISOString() || "",
          include_history: includeHistory,
        }),
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setLoading(false);
    }
  }
  return (
    <>
      <form className="search-panel temporal-search" onSubmit={submit}>
        <Search size={25} />
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="检索事实、决定、人员、商机或一段过去的讨论…"
          autoFocus
        />
        <button className="button primary" disabled={!query.trim() || loading}>
          {loading ? "检索中" : "开始召回"}
        </button>
        <div className="search-options">
          <label>
            <input
              type="checkbox"
              checked={allProjects}
              onChange={(event) => setAllProjects(event.target.checked)}
            />
            明确检索全部项目
          </label>
          <label>
            <CalendarClock size={13} />
            时间锚点
            <input
              type="datetime-local"
              value={asOf}
              onChange={(event) => setAsOf(event.target.value)}
            />
          </label>
          <label>
            <input
              type="checkbox"
              checked={includeHistory}
              onChange={(event) => setIncludeHistory(event.target.checked)}
            />
            包含已被取代/过期历史
          </label>
          {asOf && (
            <button type="button" onClick={() => setAsOf("")}>
              恢复现在
            </button>
          )}
        </div>
      </form>
      {error && (
        <Empty icon={XCircle} title="检索失败">
          {error}
        </Empty>
      )}
      {result && (
        <Section
          title={`${result.hits?.length || 0} 个真实结果`}
          detail={`Context · ${result.context_id || "未生成"} · ${result.backend || "retrieval"} · 候选 ${result.candidate_count ?? result.hits?.length ?? 0} · ${result.took_ms ?? 0}ms`}
        >
          <div className="search-results">
            {result.hits?.map((hit, index) => (
              <article key={String(hit.result_id || hit.id || index)}>
                <div>
                  <Status value={String(hit.kind || "record")} />
                  <small>{String(hit.project_id || "")}</small>
                </div>
                <h3>
                  {String(
                    hit.title || hit.summary || hit.statement || "未命名记录",
                  )}
                </h3>
                <p>{String(hit.snippet || hit.body || hit.summary || "")}</p>
                <div className="search-temporal-meta">
                  <span>{String(hit.temporal_relation || "unknown")}</span>
                  <span>
                    时间相关{" "}
                    {Math.round(Number(hit.temporal_relevance || 0) * 100)}%
                  </span>
                  {Boolean(hit.valid_from) && (
                    <span>有效自 {time(String(hit.valid_from))}</span>
                  )}
                  {Boolean(hit.valid_until) && (
                    <span>至 {time(String(hit.valid_until))}</span>
                  )}
                </div>
                <footer>
                  <span>
                    记录/观察{" "}
                    {time(String(hit.observed_at || hit.updated_at || ""))}
                  </span>
                  <button className="text-button">
                    查看来源 <ArrowRight size={13} />
                  </button>
                </footer>
              </article>
            ))}
          </div>
        </Section>
      )}
      {!result && !error && (
        <Empty icon={FileSearch} title="从一个真实问题开始">
          既可以问“现在什么最相关”，也可以指定过去某个时间点，系统会按当时的有效事实和时间距离重新排序。
        </Empty>
      )}
    </>
  );
}

const legacyLayers = [
  ["01", "Evidence", "不可变证据", "所有内容先保留原始来源"],
  ["02", "Knowledge Unit", "知识点", "从来源抽样出的最小事实与意图"],
  ["03", "Episode", "会话复盘", "一次经历中的目标、行动与结果"],
  ["04", "Memory", "长期记忆", "语义、情景、程序与身份"],
  ["05", "Living Asset", "生长知识", "随证据持续修订的文档"],
  ["06", "Agent Asset", "能力资产", "经验证后可供 Agent 使用"],
];

const memoryPageSize = 40;
const assetTypeCopy: Record<string, string> = {
  prompt: "提示资产",
  skill: "技能",
  rule: "规则",
  constraint: "强约束",
  procedure: "流程",
  tool_recipe: "工具配方",
  mcp: "MCP 合同",
  unclassified: "待人工分类",
};

type ProcessSource = {
  session_id: string;
  project_id: string;
  title: string;
  source_system: string;
  evidence_count: number;
  imported_at: string;
  last_processed_at?: string;
  status: "pending" | "processing" | "completed" | "changed" | "failed";
  status_detail: string;
};

const processStatusCopy: Record<ProcessSource["status"], string> = {
  pending: "待处理",
  processing: "处理中",
  completed: "已完成",
  changed: "内容有变化",
  failed: "失败",
};

function MemoryPager({
  page,
  total,
  onPage,
}: {
  page: number;
  total: number;
  onPage: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / memoryPageSize));
  if (total <= memoryPageSize) return null;
  return (
    <nav className="memory-pager" aria-label="记忆分页">
      <span>
        第 {page + 1} / {pages} 页 · 共 {total} 条
      </span>
      <div>
        <button disabled={page === 0} onClick={() => onPage(page - 1)}>
          上一页
        </button>
        <button disabled={page + 1 >= pages} onClick={() => onPage(page + 1)}>
          下一页
        </button>
      </div>
    </nav>
  );
}

export function MemoryPage({
  api,
  projectID,
  openRun,
  mode = "advanced",
}: {
  api: APIClient;
  projectID: string;
  openRun: (id: string) => void;
  mode?: UIMode;
}) {
  const [view, setView] = useState<
    | "overview"
    | "knowledge"
    | "episodes"
    | "memories"
    | "living"
    | "assets"
    | "graph"
    | "objects"
  >("overview");
  const [selected, setSelected] = useState<HarnessObject>();
  const [selectedLiving, setSelectedLiving] = useState("");
  const [selectedAsset, setSelectedAsset] = useState("");
  const [assetTypeFilter, setAssetTypeFilter] = useState("");
  const [selectedUnit, setSelectedUnit] = useState<KnowledgeUnit>();
  const [unitCorrection, setUnitCorrection] = useState<{
    unit: KnowledgeUnit;
    expectedRevision: number;
    impact?: CorrectionImpact;
  }>();
  const [entityCorrection, setEntityCorrection] = useState<{
    id: string;
    label: string;
    entityType: string;
    impact: CorrectionImpact;
  }>();
  const [memoryCorrection, setMemoryCorrection] = useState<{
    memory: Record<string, unknown>;
    expectedRevision: number;
  }>();
  const [graphMode, setGraphMode] = useState<"semantic" | "lineage">(
    "semantic",
  );
  const [processing, setProcessing] = useState("");
  const [processNotice, setProcessNotice] = useState("");
  const [sourcePicker, setSourcePicker] = useState(false);
  const [selectedSources, setSelectedSources] = useState<string[]>([]);
  const [knowledgePage, setKnowledgePage] = useState(0);
  const [memoryPage, setMemoryPage] = useState(0);
  const [participantEditor, setParticipantEditor] = useState<{
    sessionID: string;
    title: string;
  }>();
  const [firstPersonSpeaker, setFirstPersonSpeaker] = useState("");
  const [otherParticipants, setOtherParticipants] = useState("");
  const [participantError, setParticipantError] = useState("");
  const [participantSaving, setParticipantSaving] = useState(false);
  useEffect(() => {
    setKnowledgePage(0);
    setMemoryPage(0);
    setView("overview");
  }, [projectID]);
  const summary = useAsync(
    () =>
      api.get<{ layers: Array<Record<string, unknown>>; needs_review: number }>(
        `/v1/layers?project_id=${encodeURIComponent(projectID)}`,
      ),
    [api, projectID],
  );
  const overview = useAsync(
    () =>
      Promise.all([
        api.get<{ memories: Array<Record<string, unknown>>; total: number }>(
          `/v1/memories?project_id=${encodeURIComponent(projectID)}&limit=8`,
        ),
        api.get<{ episodes: Array<Record<string, unknown>>; total: number }>(
          `/v1/episodes?project_id=${encodeURIComponent(projectID)}&limit=8`,
        ),
      ]).then(([memories, episodes]) => ({
        memories: memories.memories,
        episodes: episodes.episodes,
      })),
    [api, projectID],
  );
  const sourceState = useAsync(
    () => api.get<{ sources: ProcessSource[] }>(`/v1/process/sources?project_id=${encodeURIComponent(projectID)}`),
    [api, projectID],
  );
  const units = useAsync(
    view === "knowledge"
      ? () =>
          api.get<{ knowledge_units: KnowledgeUnit[]; total: number }>(
            `/v1/knowledge-units?project_id=${encodeURIComponent(projectID)}&limit=${memoryPageSize}&offset=${knowledgePage * memoryPageSize}`,
          )
      : null,
    [api, projectID, view, knowledgePage],
  );
  const episodes = useAsync(
    view === "episodes"
      ? () =>
          api.get<{ episodes: Array<Record<string, unknown>>; total: number }>(
            `/v1/episodes?project_id=${encodeURIComponent(projectID)}&limit=${memoryPageSize}`,
          )
      : null,
    [api, projectID, view],
  );
  const memories = useAsync(
    view === "memories"
      ? () =>
          api.get<{ memories: Array<Record<string, unknown>>; total: number }>(
            `/v1/memories?project_id=${encodeURIComponent(projectID)}&limit=${memoryPageSize}&offset=${memoryPage * memoryPageSize}`,
          )
      : null,
    [api, projectID, view, memoryPage],
  );
  const pins = useAsync(
    view === "memories"
      ? () =>
          api.get<{ memories: Array<Record<string, unknown>>; total: number }>(
            `/v1/memory-pins?project_id=${encodeURIComponent(projectID)}&limit=100`,
          )
      : null,
    [api, projectID, view],
  );
  const living = useAsync(
    view === "living"
      ? () =>
          api.get<{ views: Array<Record<string, unknown>>; total: number }>(
            `/v1/living?project_id=${encodeURIComponent(projectID)}`,
          )
      : null,
    [api, projectID, view],
  );
  const assets = useAsync(
    view === "assets"
      ? () =>
          api.get<{ assets: Array<Record<string, unknown>>; total: number }>(
            `/v1/assets?project_id=${encodeURIComponent(projectID)}${assetTypeFilter ? `&type=${encodeURIComponent(assetTypeFilter)}` : ""}`,
          )
      : null,
    [api, projectID, view, assetTypeFilter],
  );
  const graph = useAsync<MemoryGraphData>(
    view === "graph"
      ? () =>
          api.get(
            `/v1/graph?project_id=${encodeURIComponent(projectID)}&limit=60`,
          )
      : null,
    [api, projectID, view],
  );
  const semanticGraph = useAsync<SemanticGraphData>(
    view === "graph"
      ? () =>
          api.get(
            `/v1/graph/semantic?project_id=${encodeURIComponent(projectID)}&limit=100`,
          )
      : null,
    [api, projectID, view],
  );
  const livingDetail = useAsync<Record<string, unknown>>(
    selectedLiving
      ? () => api.get(`/v1/living/${encodeURIComponent(selectedLiving)}`)
      : null,
    [api, selectedLiving],
  );
  const extensions = useAsync(
    view === "objects"
      ? async () => {
          const [types, objects] = await Promise.all([
            api.get<{ types: MemoryType[] }>("/v1/harness/types"),
            api.get<{ objects: HarnessObject[] }>(
              `/v1/harness/objects?project_id=${encodeURIComponent(projectID)}&limit=100`,
            ),
          ]);
          return { types: types.types, objects: objects.objects };
        }
      : null,
    [api, projectID, view],
  );
  const types = extensions.data?.types || [];
  const objects = extensions.data?.objects || [];
  const layers = summary.data?.layers || [];
  const layerCount = (index: number) => Number(layers[index]?.count || 0);
  async function openParticipantEditor(item: Record<string, unknown>) {
    const sessionID = String(item.session_id || "");
    setParticipantEditor({ sessionID, title: String(item.title || "会话") });
    setParticipantError("");
    setFirstPersonSpeaker("");
    setOtherParticipants("");
    try {
      const data = await api.get<{
        participants: Array<{ display_name: string; role: string }>;
      }>(`/v1/sessions/${encodeURIComponent(sessionID)}/participants`);
      setFirstPersonSpeaker(
        data.participants.find((item) => item.role === "first_person_speaker")
          ?.display_name || "",
      );
      setOtherParticipants(
        data.participants
          .filter((item) => item.role !== "first_person_speaker")
          .map((item) => item.display_name)
          .join("\n"),
      );
    } catch (reason) {
      setParticipantError(
        reason instanceof Error ? reason.message : String(reason),
      );
    }
  }
  async function saveParticipantContext(andReprocess: boolean) {
    if (!participantEditor) return;
    setParticipantSaving(true);
    setParticipantError("");
    const participants: Array<{
      display_name: string;
      role: string;
      aliases?: string[];
    }> = [];
    if (firstPersonSpeaker.trim())
      participants.push({
        display_name: firstPersonSpeaker.trim(),
        role: "first_person_speaker",
        aliases: ["我", "本人"],
      });
    for (const name of otherParticipants
      .split(/[，,\n]/)
      .map((value) => value.trim())
      .filter(Boolean))
      participants.push({ display_name: name, role: "participant" });
    try {
      await api.put(
        `/v1/sessions/${encodeURIComponent(participantEditor.sessionID)}/participants`,
        { participants },
      );
      const sessionID = participantEditor.sessionID;
      setParticipantEditor(undefined);
      setProcessNotice(
        `已保存 ${participants.length} 位会话参与者；${andReprocess ? "正在用这些身份重新沉淀。" : "重新沉淀后才会更新主体与知识图谱。"}`,
      );
      if (andReprocess) await reprocessEpisode(sessionID);
    } catch (reason) {
      setParticipantError(
        reason instanceof Error ? reason.message : String(reason),
      );
    } finally {
      setParticipantSaving(false);
    }
  }
  async function reprocessEpisode(sessionID: string) {
    setProcessing(sessionID);
    setProcessNotice(
      "正在用当前模型和高精度质量门重新沉淀；原始 Evidence 不会改变。",
    );
    try {
      const result = await api.post<{
        results: Array<{
          knowledge_units: number;
          compiler: string;
          quality_status: string;
        }>;
      }>("/v1/process", { session_id: sessionID, force: true });
      const compiled = result.results[0];
      setProcessNotice(
        `重新沉淀完成：${compiled?.compiler || "当前模型"} 生成 ${compiled?.knowledge_units || 0} 条知识，质量状态 ${compiled?.quality_status || "unknown"}；项目投影与运行轨迹已刷新。`,
      );
      summary.reload();
      overview.reload();
      episodes.reload();
      memories.reload();
    } catch (reason) {
      setProcessNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    } finally {
      setProcessing("");
    }
  }
  async function openKnowledgeCorrection(unit: KnowledgeUnit) {
    setProcessNotice(
      "正在读取这条知识的当前 Revision 与下游影响；这里只读取，不会改变记忆。",
    );
    try {
      const [detail, correction] = await Promise.all([
        api.get<{
          knowledge_unit: KnowledgeUnit;
          governance?: HarnessObject | null;
        }>(
          `/v1/knowledge-units/${encodeURIComponent(unit.unit_id)}?project_id=${encodeURIComponent(projectID)}`,
        ),
        api.get<{ impact: CorrectionImpact }>(
          `/v1/corrections/impact?project_id=${encodeURIComponent(projectID)}&kind=knowledge_unit&id=${encodeURIComponent(unit.unit_id)}`,
        ),
      ]);
      setSelectedUnit(detail.knowledge_unit);
      setUnitCorrection({
        unit: detail.knowledge_unit,
        expectedRevision: detail.governance?.current_revision || 0,
        impact: correction.impact,
      });
      setProcessNotice("");
    } catch (reason) {
      setProcessNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    }
  }
  async function openEntityCorrection(entity: {
    id: string;
    label: string;
    entityType: string;
  }) {
    setProcessNotice(
      `正在计算实体“${entity.label}”的真实影响范围；不会修改任何关系。`,
    );
    try {
      const correction = await api.get<{ impact: CorrectionImpact }>(
        `/v1/corrections/impact?project_id=${encodeURIComponent(projectID)}&kind=entity&id=${encodeURIComponent(entity.id)}`,
      );
      setEntityCorrection({ ...entity, impact: correction.impact });
      setProcessNotice("");
    } catch (reason) {
      setProcessNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    }
  }
  async function submitKnowledgeCorrection(
    draft: KnowledgeUnit,
    editReason: string,
  ) {
    if (!unitCorrection) return;
    const key = `owner-ku-${draft.unit_id}-${unitCorrection.expectedRevision}-${Date.now()}`;
    await api.post(
      `/v1/knowledge-units/${encodeURIComponent(draft.unit_id)}/revision-proposals`,
      {
        project_id: projectID,
        expected_revision: unitCorrection.expectedRevision,
        edit_reason: editReason,
        idempotency_key: key,
        knowledge_unit: draft,
      },
    );
    setUnitCorrection(undefined);
    setSelectedUnit(undefined);
    setProcessNotice(
      "人工修正已提交为待审核 Revision；当前版本尚未改变。请到“待我审核”查看 Diff 并决定是否生效。",
    );
    units.reload();
  }
  async function openMemoryCorrection(item: Record<string, unknown>) {
    const memoryID = String(item.memory_id || "");
    setProcessNotice(
      "正在读取这条长期记忆的当前治理 Revision；这里只读取，不会改变内容。",
    );
    try {
      const detail = await api.get<{
        memory: Record<string, unknown>;
        governance?: HarnessObject | null;
      }>(
        `/v1/memories/${encodeURIComponent(memoryID)}/governance?project_id=${encodeURIComponent(projectID)}`,
      );
      setMemoryCorrection({
        memory: detail.memory,
        expectedRevision: detail.governance?.current_revision || 0,
      });
      setProcessNotice("");
    } catch (reason) {
      setProcessNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    }
  }
  async function submitMemoryCorrection(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!memoryCorrection) return;
    const form = new FormData(event.currentTarget);
    const memoryID = String(memoryCorrection.memory.memory_id || "");
    const draft = {
      ...memoryCorrection.memory,
      tier: String(form.get("tier") || memoryCorrection.memory.tier || ""),
      asset_form: String(
        form.get("asset_form") || memoryCorrection.memory.asset_form || "",
      ),
      domain: String(
        form.get("domain") || memoryCorrection.memory.domain || "",
      ),
      summary: String(form.get("summary") || ""),
      body: String(form.get("body") || ""),
      confidence: Number(form.get("confidence")),
      importance: Number(form.get("importance")),
    };
    await api.post(
      `/v1/memories/${encodeURIComponent(memoryID)}/revision-proposals`,
      {
        project_id: projectID,
        expected_revision: memoryCorrection.expectedRevision,
        edit_reason: String(form.get("edit_reason") || "").trim(),
        idempotency_key: `owner-memory-${memoryID}-${memoryCorrection.expectedRevision}-${Date.now()}`,
        memory: draft,
      },
    );
    setMemoryCorrection(undefined);
    setProcessNotice(
      "长期记忆修正已提交为待审核 Revision；当前版本尚未改变。请到“待我审核”检查 Diff 后再激活。",
    );
    memories.reload();
    overview.reload();
  }
  async function toggleMemoryPin(item: Record<string, unknown>) {
    const memoryID = String(item.memory_id || "");
    const pinned = (pins.data?.memories || []).some((memory) => String(memory.memory_id) === memoryID);
    setProcessNotice(pinned ? "正在取消首页固定…" : "正在固定到首页…");
    try {
      await api.put(`/v1/memories/${encodeURIComponent(memoryID)}/pin`, {
        project_id: projectID,
        pinned: !pinned,
      });
      setProcessNotice(pinned ? "已从首页移除；记忆内容和来源没有改变。" : "已固定到首页；这是你的选择，不使用 AI 重要度排序。");
      pins.reload();
    } catch (reason) {
      setProcessNotice(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function processSelected(sessionIDs: string[], processMode: "incremental" | "force") {
    if (!sessionIDs.length) {
      setProcessNotice("没有符合条件的原材料需要处理。你也可以点击“选择原材料”手动选择。");
      return;
    }
    setProcessing("sources");
    setProcessNotice(processMode === "force" ? "正在强制重新整理选中的原材料；原文和 Evidence 不会改变。" : "正在整理选中的新增、变化或失败原材料；已经完成且未变化的内容会自动跳过。");
    try {
      const result = await api.post<{
        results: Array<{ knowledge_units: number }>;
        total: number;
        succeeded: number;
        failed: number;
        skipped: number;
      }>("/v1/process", { project_id: projectID, session_ids: sessionIDs, mode: processMode });
      const knowledge = result.results.reduce(
        (total, item) => total + Number(item.knowledge_units || 0),
        0,
      );
      setProcessNotice(
        `处理完成：成功 ${result.succeeded || 0} 份，跳过 ${result.skipped || 0} 份，失败 ${result.failed || 0} 份，生成 ${knowledge} 条关键信息。每份材料的结果已分别保留。`,
      );
      summary.reload();
      overview.reload();
      units.reload();
      episodes.reload();
      memories.reload();
      living.reload();
      assets.reload();
      graph.reload();
      semanticGraph.reload();
      extensions.reload();
      sourceState.reload();
      setSourcePicker(false);
      setSelectedSources([]);
    } catch (reason) {
      setProcessNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    } finally {
      setProcessing("");
    }
  }
  function processNewSources() {
    const ids = (sourceState.data?.sources || [])
      .filter((item) => ["pending", "changed", "failed"].includes(item.status))
      .map((item) => item.session_id);
    void processSelected(ids, "incremental");
  }
  function forceAllSources() {
    const items = sourceState.data?.sources || [];
    if (!items.length) return;
    if (!window.confirm(`将重新处理当前空间内全部 ${items.length} 份原材料。原文不会改变，但派生结果会产生新版本。确定继续吗？`)) return;
    void processSelected(items.map((item) => item.session_id), "force");
  }
  return (
    <LoadBoundary loading={summary.loading} error={summary.error}>
      <div className="memory-library-head">
        <div className={`layer-ribbon ${mode === "basic" ? "basic-layers" : ""}`}>
          {(mode === "basic" ? [["01", "Source Material", "原材料", "导入后保持不变的对话或文件"], ["02", "Key Information", "关键信息", "从原材料中识别出的事实、目标或决定"], ["03", "Project Memory", "项目记忆", "确认后可长期使用的项目内容"]] : legacyLayers).map(([number, english, chinese, detail], index) => (
            <button
              key={english}
              onClick={() =>
                setView(
                  index === 1
                    ? "knowledge"
                    : mode === "advanced" && index === 2
                      ? "episodes"
                      : (mode === "basic" && index === 2) || index === 3
                        ? "memories"
                        : index === 4
                          ? "living"
                          : index === 5
                            ? "assets"
                            : "overview",
                )
              }
              style={{ "--delay": `${index * 55}ms` } as React.CSSProperties}
            >
              <span>{number}</span>
              <small>{english}</small>
              <h3>{chinese}</h3>
              <p>{detail}</p>
              <b>{layerCount(index)}</b>
              {index < (mode === "basic" ? 2 : legacyLayers.length - 1) && <ArrowRight size={14} />}
            </button>
          ))}
        </div>
        <p>
          <ShieldCheck size={14} />
          每一项都能回到原始材料；自动整理不会改写或删除原文。
        </p>
      </div>
      <div className="memory-library-actions">
        <div>
          <strong>默认只整理新导入的原材料</strong>
          <span>已经成功且未变化的旧材料不会重复处理。</span>
        </div>
        <div className="memory-process-actions">
        <button className="button" disabled={processing === "sources" || sourceState.loading} onClick={() => { setSelectedSources([]); setSourcePicker(true); }}>选择原材料</button>
        <button className="button primary" disabled={processing === "sources" || sourceState.loading} onClick={processNewSources}>
          {processing === "sources" ? (
            <RefreshCw size={14} />
          ) : (
            <Play size={14} />
          )}
          {processing === "sources" ? "正在处理…" : "处理新增或失败的原材料"}
        </button>
        {mode === "advanced" && <button className="button danger" disabled={processing === "sources" || sourceState.loading} onClick={forceAllSources}>强制重新处理全部</button>}
        </div>
      </div>
      <div className="memory-view-tabs">
        <button
          className={view === "overview" ? "active" : ""}
          onClick={() => setView("overview")}
        >
          总览
        </button>
        <button
          className={view === "knowledge" ? "active" : ""}
          onClick={() => setView("knowledge")}
        >
          {mode === "basic" ? "关键信息" : "知识点"} <em>{layerCount(1)}</em>
        </button>
        {mode === "advanced" && <button
          className={view === "episodes" ? "active" : ""}
          onClick={() => setView("episodes")}
        >
          会话复盘 <em>{layerCount(2)}</em>
        </button>}
        <button
          className={view === "memories" ? "active" : ""}
          onClick={() => setView("memories")}
        >
          {mode === "basic" ? "项目记忆" : "长期记忆"} <em>{layerCount(3)}</em>
        </button>
        {mode === "advanced" && <button
          className={view === "living" ? "active" : ""}
          onClick={() => setView("living")}
        >
          生长知识 <em>{layerCount(4)}</em>
        </button>}
        {mode === "advanced" && <button
          className={view === "assets" ? "active" : ""}
          onClick={() => setView("assets")}
        >
          能力资产 <em>{layerCount(5)}</em>
        </button>}
        {mode === "advanced" && <button
          className={view === "graph" ? "active" : ""}
          onClick={() => setView("graph")}
        >
          知识图谱
        </button>}
        {mode === "advanced" && <button
          className={view === "objects" ? "active" : ""}
          onClick={() => setView("objects")}
        >
          插件对象
        </button>}
      </div>
      {processNotice && (
        <p className={`memory-process-notice ${processing ? "running" : ""}`}>
          {processing && <RefreshCw size={14} />}
          {processNotice}
        </p>
      )}
      {view === "overview" && (
        <LoadBoundary loading={overview.loading} error={overview.error}>
          <div className="memory-overview-grid">
            <Section
              title="最近长期记忆"
              detail={`当前项目共 ${layerCount(3)} 条；这里只加载最近 8 条。`}
            >
              {overview.data?.memories.length ? (
                <div className="memory-card-list">
                  {overview.data.memories.map((item, index) => (
                    <article key={String(item.memory_id || index)}>
                      <div>
                        <Status
                          value={String(item.status || item.tier || "memory")}
                        />
                        <span>{String(item.tier || "memory")}</span>
                      </div>
                      <h3>{String(item.summary || "未命名记忆")}</h3>
                      <p>{String(item.body || "")}</p>
                      <footer>
                        {Array.isArray(item.source_evidence_ids)
                          ? `${item.source_evidence_ids.length} 条 Evidence`
                          : "来源可追溯"}{" "}
                        · {time(String(item.updated_at || ""))}
                      </footer>
                    </article>
                  ))}
                </div>
              ) : (
                <Empty icon={BrainCircuit} title="还没有长期记忆">
                  先从“导入记忆”加入一份真实内容。
                </Empty>
              )}
            </Section>
            <Section title="最近会话复盘" detail="每次导入怎样变成知识。">
              {overview.data?.episodes.length ? (
                <div className="episode-list">
                  {overview.data.episodes.map((episode, index) => (
                    <article key={String(episode.episode_id || index)}>
                      <span>
                        {String(episode.source_system || "source")
                          .slice(0, 2)
                          .toUpperCase()}
                      </span>
                      <div>
                        <strong>{String(episode.title || "会话复盘")}</strong>
                        <small>{String(episode.summary || "")}</small>
                        <em>
                          {time(
                            String(
                              episode.updated_at || episode.created_at || "",
                            ),
                          )}
                        </em>
                      </div>
                      <b>{String(episode.units || 0)}</b>
                    </article>
                  ))}
                </div>
              ) : (
                <Empty icon={Route} title="还没有会话复盘">
                  导入 AI 会话或一组相关文档后会自动出现。
                </Empty>
              )}
            </Section>
          </div>
        </LoadBoundary>
      )}
      {view === "knowledge" && (
        <LoadBoundary loading={units.loading} error={units.error}>
          <Section
            title="结构化知识点"
            detail={`共 ${units.data?.total || layerCount(1)} 条；点击任意知识点检查主体、人物、地点、时间、双向关系与原文依据。`}
          >
            {units.data?.knowledge_units.length ? (
              <>
                <div className="knowledge-table">
                  {units.data.knowledge_units.map((item, index) => (
                    <button
                      key={String(item.unit_id || index)}
                      onClick={() => setSelectedUnit(item)}
                    >
                      <Status value={String(item.unit_type || "knowledge")} />
                      <div>
                        <strong>{String(item.statement || "")}</strong>
                        <small>
                          {item.structure?.attribution?.resolution ===
                          "resolved"
                            ? `主体：${item.structure.frame?.subject?.canonical_name || item.structure.attribution.subject_surface || "已解析"}`
                            : "主体待确认，不会默认映射为 Owner"}{" "}
                          · {String(item.evidence_id || "")}
                        </small>
                      </div>
                      <span>
                        {Math.round(Number(item.confidence || 0) * 100)}%
                      </span>
                      <time>{time(String(item.observed_at || ""))}</time>
                      <ChevronRight size={15} />
                    </button>
                  ))}
                </div>
                <MemoryPager
                  page={knowledgePage}
                  total={units.data.total}
                  onPage={setKnowledgePage}
                />
              </>
            ) : (
              <Empty icon={Database} title="还没有知识点">
                导入后，当前沉淀引擎会在保留来源的基础上生成候选知识。
              </Empty>
            )}
          </Section>
        </LoadBoundary>
      )}
      {view === "episodes" && (
        <LoadBoundary loading={episodes.loading} error={episodes.error}>
          <Section
            title="会话复盘"
            detail="一次会话或一批文档形成一个可以检查的 Episode；多人录音可先补充说话人与参与者，再重新沉淀。"
          >
            {episodes.data?.episodes.length ? (
              <div className="episode-grid">
                {episodes.data.episodes.map((item, index) => (
                  <article key={String(item.episode_id || index)}>
                    <div>
                      <Status value={String(item.status || "episode")} />
                      <code>{String(item.compiler || "")}</code>
                    </div>
                    <h3>{String(item.title || "会话复盘")}</h3>
                    <p>{String(item.summary || "")}</p>
                    <dl>
                      <div>
                        <dt>Evidence</dt>
                        <dd>
                          {Array.isArray(item.evidence_ids)
                            ? item.evidence_ids.length
                            : 0}
                        </dd>
                      </div>
                      <div>
                        <dt>知识点</dt>
                        <dd>{String(item.units || 0)}</dd>
                      </div>
                    </dl>
                    <footer>
                      <span>
                        {String(item.source_system || "")} ·{" "}
                        {time(String(item.updated_at || ""))}
                      </span>
                      <div className="episode-actions">
                        <button
                          className="text-button"
                          onClick={() => openParticipantEditor(item)}
                        >
                          <UserPlus size={12} />
                          说话人与人物
                        </button>
                        <button
                          className="text-button"
                          disabled={processing === String(item.session_id)}
                          onClick={() =>
                            reprocessEpisode(String(item.session_id))
                          }
                        >
                          {processing === String(item.session_id)
                            ? "正在重新沉淀…"
                            : "用当前模型重新沉淀"}
                        </button>
                      </div>
                    </footer>
                  </article>
                ))}
              </div>
            ) : (
              <Empty icon={Route} title="还没有会话复盘">
                完成一次导入后会出现。
              </Empty>
            )}
          </Section>
        </LoadBoundary>
      )}
      {view === "memories" && (
        <LoadBoundary loading={memories.loading} error={memories.error}>
          <Section
            title="长期记忆"
            detail={`共 ${memories.data?.total || layerCount(3)} 条；候选与冲突记忆会明确标注。人工修正只创建待审核 Revision，不会覆盖当前版本。`}
          >
            {memories.data?.memories.length ? (
              <>
                <div className="memory-card-list full">
                  {memories.data.memories.map((item, index) => (
                    <article className={(pins.data?.memories || []).some((memory) => String(memory.memory_id) === String(item.memory_id)) ? "pinned" : ""} key={String(item.memory_id || index)}>
                      <div>
                        <Status value={String(item.status || "memory")} />
                        <span>{(pins.data?.memories || []).some((memory) => String(memory.memory_id) === String(item.memory_id)) ? "已固定到首页" : String(item.tier || "memory")}</span>
                      </div>
                      <h3>{String(item.summary || "未命名记忆")}</h3>
                      <p>{String(item.body || "")}</p>
                      <footer>
                        <span>
                          {Array.isArray(item.source_evidence_ids)
                            ? `${item.source_evidence_ids.length} 条 Evidence`
                            : "来源可追溯"}{" "}
                          · 置信度{" "}
                          {Math.round(Number(item.confidence || 0) * 100)}%
                        </span>
                        <div className="memory-card-actions">
                          <button className="text-button" onClick={() => void toggleMemoryPin(item)}>
                            {(pins.data?.memories || []).some((memory) => String(memory.memory_id) === String(item.memory_id)) ? <PinOff size={12}/> : <Pin size={12}/>}
                            {(pins.data?.memories || []).some((memory) => String(memory.memory_id) === String(item.memory_id)) ? "取消固定" : "固定到首页"}
                          </button>
                          <button className="text-button" onClick={() => void openMemoryCorrection(item)}>人工修正</button>
                        </div>
                      </footer>
                    </article>
                  ))}
                </div>
                <MemoryPager
                  page={memoryPage}
                  total={memories.data.total}
                  onPage={setMemoryPage}
                />
              </>
            ) : (
              <Empty icon={BrainCircuit} title="还没有长期记忆">
                系统不会用演示数据填满页面。
              </Empty>
            )}
          </Section>
        </LoadBoundary>
      )}
      {view === "living" && (
        <LoadBoundary loading={living.loading} error={living.error}>
          <Section
            title="生长知识"
            detail="这些是由长期记忆持续重建的可读文档；点击即可查看正文和来源记忆。"
          >
            {living.data?.views.length ? (
              <div className="living-grid">
                {living.data.views.map((item, index) => (
                  <button
                    key={String(item.view_id || index)}
                    onClick={() => setSelectedLiving(String(item.view_id))}
                  >
                    <span className="living-number">0{index + 1}</span>
                    <div>
                      <Status value={String(item.status || "active")} />
                      <h3>{String(item.title || "生长知识")}</h3>
                      <p>{String(item.summary || "")}</p>
                      <small>
                        {Array.isArray(item.source_memory_ids)
                          ? `${item.source_memory_ids.length} 条来源记忆`
                          : "来源可追溯"}{" "}
                        · {time(String(item.updated_at || ""))}
                      </small>
                    </div>
                    <ChevronRight size={18} />
                  </button>
                ))}
              </div>
            ) : (
              <Empty icon={FileSearch} title="还没有生长知识">
                高质量长期记忆形成后会自动生成。
              </Empty>
            )}
          </Section>
        </LoadBoundary>
      )}
      {view === "assets" && (
        <LoadBoundary loading={assets.loading} error={assets.error}>
          <Section
            title="能力资产"
            detail="Prompt、Skill、Rule、Constraint、Procedure、Tool Recipe、MCP 进入统一受治理资产链；候选不会自动激活。"
          >
            <div className="asset-type-filter">
              <button
                className={!assetTypeFilter ? "active" : ""}
                onClick={() => setAssetTypeFilter("")}
              >
                全部
              </button>
              {Object.entries(assetTypeCopy).map(([type, label]) => (
                <button
                  key={type}
                  className={assetTypeFilter === type ? "active" : ""}
                  onClick={() => setAssetTypeFilter(type)}
                >
                  {label}
                  <small>{type}</small>
                </button>
              ))}
            </div>
            {assets.data?.assets.length ? (
              <div className="asset-browser-grid">
                {assets.data.assets.map((item, index) => (
                  <button
                    key={String(item.asset_id || index)}
                    onClick={() => setSelectedAsset(String(item.asset_id))}
                  >
                    <div>
                      <Status value={String(item.status || "candidate")} />
                      <code>
                        {assetTypeCopy[String(item.asset_type)] ||
                          String(item.asset_type || "asset")}
                      </code>
                    </div>
                    <h3>{String(item.title || "能力资产")}</h3>
                    <p>{String(item.summary || "")}</p>
                    <footer>
                      <span>
                        {String(item.classification_status || "legacy")} ·{" "}
                        {String(item.validation_status || "not_run")}
                      </span>
                      <ChevronRight size={16} />
                    </footer>
                  </button>
                ))}
              </div>
            ) : (
              <Empty icon={PackageCheck} title="当前类型还没有能力资产">
                只有可复用且需要治理的程序知识才会进入资产层；模糊类型不会被强行激活。
              </Empty>
            )}
          </Section>
        </LoadBoundary>
      )}
      {view === "graph" && (
        <Section
          title="可追溯知识图谱"
          detail="语义图先展示实体关系；属性进入详情。来源链先展示六层总览，再按对象展开真实邻域。"
        >
          <div className="graph-mode-switch">
            <button
              className={graphMode === "semantic" ? "active" : ""}
              onClick={() => setGraphMode("semantic")}
            >
              语义关系网络
            </button>
            <button
              className={graphMode === "lineage" ? "active" : ""}
              onClick={() => setGraphMode("lineage")}
            >
              六层来源链
            </button>
          </div>
          {graphMode === "semantic" ? (
            <LoadBoundary
              loading={semanticGraph.loading}
              error={semanticGraph.error}
            >
              {semanticGraph.data?.nodes.length ? (
                <SemanticGraph
                  graph={semanticGraph.data}
                  onCorrectEntity={(entity) =>
                    void openEntityCorrection(entity)
                  }
                />
              ) : (
                <Empty icon={Network} title="还没有可靠语义关系">
                  未解析主体不会被强行画入图谱。请配置模型后重新沉淀，或从知识点详情确认主体。
                </Empty>
              )}
            </LoadBoundary>
          ) : (
            <LoadBoundary loading={graph.loading} error={graph.error}>
              {graph.data?.nodes.length ? (
                <MemoryLineage graph={graph.data} />
              ) : (
                <Empty icon={Network} title="还没有来源链路">
                  完成一次高质量沉淀后会自动形成网络。
                </Empty>
              )}
            </LoadBoundary>
          )}
        </Section>
      )}
      {view === "objects" && (
        <LoadBoundary loading={extensions.loading} error={extensions.error}>
          <>
            <Section
              title="可扩展记忆类型"
              detail="这些类型来自内置或外部插件；新增一层不需要改内核。"
            >
              <div className="type-grid">
                {types.map((type) => {
                  const count = objects.filter(
                    (item) => item.type_id === type.type_id,
                  ).length;
                  return (
                    <button
                      key={type.type_id}
                      onClick={() =>
                        setSelected(
                          objects.find((item) => item.type_id === type.type_id),
                        )
                      }
                    >
                      <div>
                        <Status value={type.status} />
                        <span>
                          {type.protection_class === "protected" ? (
                            <LockKeyhole size={14} />
                          ) : (
                            <Database size={14} />
                          )}
                        </span>
                      </div>
                      <h3>{type.display_name}</h3>
                      <code>{type.type_id}</code>
                      <footer>
                        <strong>{count}</strong>
                        <span>当前项目对象</span>
                        <ChevronRight size={15} />
                      </footer>
                    </button>
                  );
                })}
              </div>
            </Section>
            <Section
              title="对象浏览器"
              detail="每个对象都有生命周期、修订、来源和产生它的 Run。"
            >
              {objects.length ? (
                <div className="object-table">
                  {objects.map((item) => (
                    <button
                      key={item.object_id}
                      onClick={() => setSelected(item)}
                    >
                      <Status value={item.status} />
                      <div>
                        <strong>
                          {String(
                            Object.values(item.revision.payload)[0] ||
                              item.type_id,
                          )}
                        </strong>
                        <code>{item.type_id}</code>
                      </div>
                      <span>v{item.current_revision}</span>
                      <span>{Math.round(item.revision.confidence * 100)}%</span>
                      <time>{time(item.updated_at)}</time>
                      <ChevronRight size={15} />
                    </button>
                  ))}
                </div>
              ) : (
                <Empty icon={Layers3} title="这个项目还没有插件对象">
                  默认六层记忆仍然可以在前面的标签页查看；插件 Pipeline
                  写入后会在这里出现。
                </Empty>
              )}
            </Section>
          </>
        </LoadBoundary>
      )}
      {selectedUnit && (
        <KnowledgeUnitDrawer
          unit={selectedUnit}
          onClose={() => setSelectedUnit(undefined)}
          onCorrect={() => void openKnowledgeCorrection(selectedUnit)}
        />
      )}
      {unitCorrection && (
        <KnowledgeUnitCorrectionModal
          unit={unitCorrection.unit}
          expectedRevision={unitCorrection.expectedRevision}
          impact={unitCorrection.impact}
          onClose={() => setUnitCorrection(undefined)}
          onSubmit={submitKnowledgeCorrection}
        />
      )}
      {entityCorrection && (
        <EntityCorrectionModal
          api={api}
          projectID={projectID}
          target={entityCorrection}
          graph={semanticGraph.data}
          onClose={() => setEntityCorrection(undefined)}
          onSubmitted={(message) => {
            setProcessNotice(message);
            semanticGraph.reload();
            units.reload();
          }}
        />
      )}
      {memoryCorrection && (
        <div
          className="modal-backdrop"
          onMouseDown={() => setMemoryCorrection(undefined)}
        >
          <form
            className="modal wide"
            onMouseDown={(event) => event.stopPropagation()}
            onSubmit={(event) => void submitMemoryCorrection(event)}
          >
            <p className="micro">
              OWNER MEMORY REVISION · v{memoryCorrection.expectedRevision}
            </p>
            <h2>人工修正长期记忆</h2>
            <p className="drawer-lead">
              这里不会直接改数据库旧行。提交后只会生成新的
              Revision，并进入“待我审核”；批准后才成为检索与读取的权威版本。
            </p>
            <div className="form-grid">
              <label>
                层级
                <select
                  name="tier"
                  defaultValue={String(
                    memoryCorrection.memory.tier || "semantic",
                  )}
                >
                  <option value="episodic">episodic</option>
                  <option value="semantic">semantic</option>
                  <option value="procedural">procedural</option>
                  <option value="identity_core">identity_core</option>
                </select>
              </label>
              <label>
                资产形式
                <input
                  name="asset_form"
                  defaultValue={String(
                    memoryCorrection.memory.asset_form || "",
                  )}
                />
              </label>
              <label>
                领域
                <input
                  name="domain"
                  defaultValue={String(memoryCorrection.memory.domain || "")}
                />
              </label>
              <label>
                置信度
                <input
                  name="confidence"
                  type="number"
                  min="0"
                  max="1"
                  step="0.01"
                  defaultValue={String(memoryCorrection.memory.confidence ?? 0)}
                />
              </label>
              <label>
                重要度
                <input
                  name="importance"
                  type="number"
                  min="0"
                  max="1"
                  step="0.01"
                  defaultValue={String(memoryCorrection.memory.importance ?? 0)}
                />
              </label>
            </div>
            <label>
              摘要
              <input
                name="summary"
                required
                maxLength={1200}
                defaultValue={String(memoryCorrection.memory.summary || "")}
              />
            </label>
            <label>
              正文
              <textarea
                name="body"
                required
                rows={8}
                maxLength={12000}
                defaultValue={String(memoryCorrection.memory.body || "")}
              />
            </label>
            <label>
              修正理由
              <textarea
                name="edit_reason"
                required
                rows={3}
                placeholder="说明为什么当前长期记忆需要人工修正；审核页会保留这条理由。"
              />
            </label>
            <p className="token-warning">
              Evidence、Episode、Memory ID
              和原始观察时间不可在这里改写；如需纠正来源，必须追加新的
              Evidence。
            </p>
            <div className="modal-actions">
              <button
                type="button"
                className="button"
                onClick={() => setMemoryCorrection(undefined)}
              >
                取消
              </button>
              <button className="button primary">提交 Revision 等待审核</button>
            </div>
          </form>
        </div>
      )}
      {participantEditor && (
        <div
          className="modal-backdrop"
          onMouseDown={() =>
            !participantSaving && setParticipantEditor(undefined)
          }
        >
          <form
            className="modal"
            onMouseDown={(event) => event.stopPropagation()}
            onSubmit={(event) => {
              event.preventDefault();
              void saveParticipantContext(true);
            }}
          >
            <p className="micro">PARTICIPANT CONTEXT</p>
            <h2>告诉系统这次录音里有谁</h2>
            <p className="drawer-lead">
              身份资料只帮助模型解析“谁做了什么”，不会修改原始录音。多人对话没有声道标签时，系统仍会把无法确定的“我/你/他”交给你确认。
            </p>
            <label>
              整份语料中的“我”是谁（可不填）
              <input
                value={firstPersonSpeaker}
                onChange={(event) => setFirstPersonSpeaker(event.target.value)}
                placeholder="仅当全文第一人称始终属于同一人时填写"
              />
            </label>
            <small className="token-warning">
              多人录音中若不同人都说过“我”，请留空，避免把他人的经历写到错误主体。
            </small>
            <label>
              其他参与人或重要人物
              <textarea
                rows={5}
                value={otherParticipants}
                onChange={(event) => setOtherParticipants(event.target.value)}
                placeholder={"每行一个名字，例如：\n王芳\n李明"}
              />
            </label>
            {participantError && (
              <p className="form-error">{participantError}</p>
            )}
            <div className="modal-actions">
              <button
                type="button"
                className="button"
                disabled={participantSaving}
                onClick={() => setParticipantEditor(undefined)}
              >
                取消
              </button>
              <button
                type="button"
                className="button"
                disabled={participantSaving}
                onClick={() => saveParticipantContext(false)}
              >
                只保存资料
              </button>
              <button
                type="submit"
                className="button primary"
                disabled={participantSaving}
              >
                {participantSaving ? "正在保存…" : "保存并重新沉淀"}
              </button>
            </div>
          </form>
        </div>
      )}
      {selected && (
        <div
          className="drawer-backdrop"
          onMouseDown={() => setSelected(undefined)}
        >
          <aside
            className="drawer"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button
              className="drawer-close"
              onClick={() => setSelected(undefined)}
              aria-label="关闭"
            >
              ×
            </button>
            <p className="micro">GENERIC MEMORY OBJECT</p>
            <h2>
              {String(
                Object.values(selected.revision.payload)[0] || selected.type_id,
              )}
            </h2>
            <div className="drawer-status">
              <Status value={selected.status} />
              <span>修订 v{selected.current_revision}</span>
              <span>
                置信度 {Math.round(selected.revision.confidence * 100)}%
              </span>
            </div>
            <ReadableObject value={selected.revision.payload} />
            <details className="technical-details">
              <summary>查看版本和来源信息</summary>
            <dl>
              <div>
                <dt>类型</dt>
                <dd>{selected.type_id}</dd>
              </div>
              <div>
                <dt>插件</dt>
                <dd>
                  {selected.revision.plugin_id}@
                  {selected.revision.plugin_version}
                </dd>
              </div>
              <div>
                <dt>内容哈希</dt>
                <dd>
                  <code>{selected.revision.content_hash}</code>
                </dd>
              </div>
              <div>
                <dt>来源 Evidence</dt>
                <dd>
                  {selected.revision.source_evidence_ids.length || "无直接引用"}
                </dd>
              </div>
            </dl>
            </details>
            {selected.revision.run_id && (
              <button
                className="button primary"
                onClick={() => openRun(selected.revision.run_id!)}
              >
                打开产生它的运行 <ArrowRight size={14} />
              </button>
            )}
          </aside>
        </div>
      )}
      {selectedLiving && (
        <div
          className="drawer-backdrop"
          onMouseDown={() => setSelectedLiving("")}
        >
          <aside
            className="drawer living-drawer"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button
              className="drawer-close"
              onClick={() => setSelectedLiving("")}
              aria-label="关闭"
            >
              ×
            </button>
            <LoadBoundary
              loading={livingDetail.loading}
              error={livingDetail.error}
            >
              {livingDetail.data && (
                <>
                  <p className="micro">LIVING KNOWLEDGE</p>
                  <h2>
                    {String(
                      (livingDetail.data.view as Record<string, unknown>)
                        ?.title || "生长知识",
                    )}
                  </h2>
                  <p className="drawer-lead">
                    {String(
                      (livingDetail.data.view as Record<string, unknown>)
                        ?.summary || "",
                    )}
                  </p>
                  <ReadableMarkdown
                    content={String(livingDetail.data.content || "")}
                    className="living-readable-content"
                  />
                  <small>
                    {Array.isArray(livingDetail.data.memories)
                      ? `${livingDetail.data.memories.length} 条来源记忆`
                      : "来源可追溯"}
                  </small>
                </>
              )}
            </LoadBoundary>
          </aside>
        </div>
      )}
      {sourcePicker && (
        <div className="modal-backdrop" onMouseDown={() => setSourcePicker(false)}>
          <div className="modal wide source-picker" role="dialog" aria-modal="true" aria-labelledby="source-picker-title" onMouseDown={(event) => event.stopPropagation()}>
            <p className="micro">SELECT SOURCE MATERIAL</p>
            <h2 id="source-picker-title">选择要整理的原材料</h2>
            <p className="modal-intro">每一项是一整份对话或文件。基础模式不会拆成单条消息，以免丢失上下文。</p>
            <div className="source-picker-tools">
              <button className="button" type="button" onClick={() => setSelectedSources((sourceState.data?.sources || []).filter((item) => ["pending", "changed", "failed"].includes(item.status)).map((item) => item.session_id))}>选择需要处理的</button>
              <button className="button" type="button" onClick={() => setSelectedSources([])}>清空选择</button>
              <span>已选 {selectedSources.length} / {sourceState.data?.sources.length || 0}</span>
            </div>
            <div className="source-picker-list">
              {(sourceState.data?.sources || []).map((source) => <label key={source.session_id} className={`source-picker-row ${source.status}`}>
                <input type="checkbox" checked={selectedSources.includes(source.session_id)} onChange={(event) => setSelectedSources((current) => event.target.checked ? [...new Set([...current, source.session_id])] : current.filter((id) => id !== source.session_id))}/>
                <div><strong>{source.title || source.session_id}</strong><small>{source.source_system} · {source.evidence_count} 条内容 · 导入 {time(source.imported_at)}</small></div>
                <span className={`source-state ${source.status}`}>{processStatusCopy[source.status]}</span>
                <small>{source.last_processed_at ? `上次处理 ${time(source.last_processed_at)}` : "尚未处理"}</small>
              </label>)}
              {!sourceState.loading && !(sourceState.data?.sources.length) && <Empty icon={FileInput} title="还没有原材料">先从“导入记忆”加入一份对话或文件。</Empty>}
            </div>
            <div className="modal-actions">
              <button type="button" className="button" onClick={() => setSourcePicker(false)}>取消</button>
              {mode === "advanced" && <button type="button" className="button danger" disabled={!selectedSources.length || processing === "sources"} onClick={() => void processSelected(selectedSources, "force")}>强制重跑选中项</button>}
              <button type="button" className="button primary" disabled={!selectedSources.length || processing === "sources"} onClick={() => void processSelected(selectedSources, "incremental")}>处理选中原材料</button>
            </div>
          </div>
        </div>
      )}
      {selectedAsset && (
        <AssetGovernanceDrawer
          api={api}
          assetID={selectedAsset}
          onClose={() => setSelectedAsset("")}
          onChanged={assets.reload}
        />
      )}
    </LoadBoundary>
  );
}

function RunRow({ run, onClick }: { run: HarnessRun; onClick?: () => void }) {
  return (
    <button onClick={onClick}>
      <Status value={run.status} />
      <div>
        <strong>{run.pipeline_id}</strong>
        <small>
          {run.caller_type} · {run.channel} · {time(run.created_at)}
        </small>
      </div>
      <code>v{run.pipeline_version}</code>
      <ChevronRight size={15} />
    </button>
  );
}

type RunPageResponse = {
  runs: HarnessRun[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

const RUN_PAGE_SIZE = 50;

export function RunsPage({
  api,
  projectID,
  requestedRun,
}: {
  api: APIClient;
  projectID: string;
  requestedRun: string;
}) {
  const [runItems, setRunItems] = useState<HarnessRun[]>([]);
  const [runTotal, setRunTotal] = useState(0);
  const [runHasMore, setRunHasMore] = useState(false);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState("");
  const fetchRuns = useCallback(async (offset: number, replace: boolean) => {
    if (!projectID) return;
    setRunsLoading(true);
    setRunsError("");
    try {
      const result = await api.get<RunPageResponse>(
        `/v1/harness/runs?project_id=${encodeURIComponent(projectID)}&limit=${RUN_PAGE_SIZE}&offset=${offset}`,
      );
      setRunTotal(result.total || 0);
      setRunHasMore(Boolean(result.has_more));
      setRunItems((current) => {
        if (replace) return result.runs || [];
        const byID = new Map(current.map((item) => [item.run_id, item]));
        for (const item of result.runs || []) byID.set(item.run_id, item);
        return Array.from(byID.values());
      });
    } catch (reason) {
      setRunsError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setRunsLoading(false);
    }
  }, [api, projectID]);
  useEffect(() => {
    setRunItems([]);
    setRunTotal(0);
    setRunHasMore(false);
    void fetchRuns(0, true);
  }, [fetchRuns]);
  const reloadRuns = useCallback(() => fetchRuns(0, true), [fetchRuns]);
  const loadMoreRuns = useCallback(() => fetchRuns(runItems.length, false), [fetchRuns, runItems.length]);
  const [selectedID, setSelectedID] = useState(requestedRun);
  useEffect(() => {
    if (requestedRun) setSelectedID(requestedRun);
  }, [requestedRun]);
  const detail = useAsync<RunDetail>(
    selectedID
      ? () => api.get(`/v1/harness/runs/${encodeURIComponent(selectedID)}`)
      : null,
    [api, selectedID],
  );
  const [actionError, setActionError] = useState("");
  async function cancelRun() {
    try {
      await api.post(
        `/v1/harness/runs/${encodeURIComponent(selectedID)}/cancel`,
        { reason: "Owner 从运行检查器取消" },
      );
      await reloadRuns();
      detail.reload();
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function retryRun() {
    try {
      const result = await api.post<{ run_id: string }>(
        `/v1/harness/runs/${encodeURIComponent(selectedID)}/retry`,
        {},
      );
      setSelectedID(result.run_id);
      await reloadRuns();
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function forkRun(nodeID: string) {
    try {
      const result = await api.post<{ run_id: string }>(
        `/v1/harness/runs/${encodeURIComponent(selectedID)}/fork`,
        { node_id: nodeID },
      );
      setSelectedID(result.run_id);
      await reloadRuns();
    } catch (reason) {
      setActionError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <LoadBoundary loading={runsLoading && runItems.length === 0} error={runsError && runItems.length === 0 ? runsError : ""}>
      <div className="runs-layout">
        <aside className="run-index">
          <div className="run-filter">
            <Route size={16} />
            <strong>{runItems.length} / {runTotal} 次运行</strong>
            <button onClick={() => void reloadRuns()} disabled={runsLoading}>
              <RefreshCw size={14} />
            </button>
          </div>
          {runsError && runItems.length > 0 && <p className="form-error inline-error">{runsError}</p>}
          {runItems.length ? (
            <>
              <div className="run-list">
                {runItems.map((run) => (
                  <RunRow
                    key={run.run_id}
                    run={run}
                    onClick={() => setSelectedID(run.run_id)}
                  />
                ))}
              </div>
              {runHasMore && (
                <button className="run-load-more" disabled={runsLoading} onClick={() => void loadMoreRuns()}>
                  {runsLoading ? "正在加载…" : `加载更多运行记录（${runItems.length} / ${runTotal}）`}
                </button>
              )}
            </>
          ) : (
            <Empty icon={Route} title="暂无 Run">
              从流程工坊启动一次流程。
            </Empty>
          )}
        </aside>
        <main className="run-inspector">
          {actionError && (
            <p className="form-error inline-error">{actionError}</p>
          )}
          {selectedID ? (
            <LoadBoundary loading={detail.loading} error={detail.error}>
              {detail.data && (
                <RunInspector
                  key={detail.data.run.run_id}
                  api={api}
                  detail={detail.data}
                  onCancel={cancelRun}
                  onRetry={retryRun}
                  onFork={forkRun}
                />
              )}
            </LoadBoundary>
          ) : (
            <Empty icon={PanelLeftClose} title="选择一次运行">
              右侧会显示 Graph、Timeline、对象和副作用回执。
            </Empty>
          )}
        </main>
      </div>
    </LoadBoundary>
  );
}

const stageCopy: Record<string, { title: string; purpose: string }> = {
  "trigger.manual": {
    title: "接收真实输入",
    purpose: "冻结本次运行的项目、会话与 Evidence 范围",
  },
  "memory.compile": {
    title: "提炼知识",
    purpose: "识别主体、关系、时间与原文依据，并合并长期记忆",
  },
  "project.derive": {
    title: "归入项目",
    purpose: "更新项目上下文、目标、决定、风险和商机投影",
  },
  "memory.semantic_graph": {
    title: "建立语义与时间关系",
    purpose: "只把已解析实体写入可追溯双向图和时间事实",
  },
  "memory.materialize": {
    title: "生成可扩展对象",
    purpose: "把六层产物映射为插件可读取的类型化对象",
  },
  "search.refresh": {
    title: "刷新检索",
    purpose: "让本次新增内容可以按项目边界被召回",
  },
};

function spanResult(span: RunDetail["spans"][number]) {
  const detail =
    span.detail && typeof span.detail === "object"
      ? (span.detail as Record<string, unknown>)
      : {};
  return detail.result && typeof detail.result === "object"
    ? (detail.result as Record<string, unknown>)
    : {};
}

function modelCostText(health?: ModelUsageSummary) {
  if (!health || health.cost_status === "unavailable") return "未配置定价";
  if (health.cost_status === "mixed_currency") return "多币种，未合计";
  const amount = `${((health.estimated_cost_microminor || 0) / 1_000_000).toFixed(4)} ${health.currency || ""} minor`;
  return health.cost_status === "partial_estimate" ? `${amount} · 部分估算` : amount;
}

function visibleMetrics(result: Record<string, unknown>) {
  return Object.entries(result)
    .filter(
      ([key, value]) =>
        key !== "operation" &&
        ["string", "number", "boolean"].includes(typeof value),
    )
    .slice(0, 5);
}

function RunInspector({
  api,
  detail,
  onCancel,
  onRetry,
  onFork,
}: {
  api: APIClient;
  detail: RunDetail;
  onCancel: () => void;
  onRetry: () => void;
  onFork: (nodeID: string) => void;
}) {
  const contextRun =
    detail.run.pipeline_id === "builtin.context-bridge.exchange";
  const [view, setView] = useState<
    "context" | "graph" | "timeline" | "effects" | "models"
  >(contextRun ? "context" : "graph");
  const modelCalls = detail.model_calls || [];
  const modelHealth = detail.model_health;
  const cancellable = [
    "queued",
    "running",
    "paused",
    "waiting_review",
  ].includes(detail.run.status);
  const retryable = ["failed", "cancelled", "denied"].includes(
    detail.run.status,
  );
  const completedStages = detail.spans.filter(
    (span) => span.status === "completed",
  ).length;
  const failedStages = detail.spans.filter(
    (span) => span.status === "failed",
  ).length;
	const modelFallbacks = detail.spans
		.map((span) => ({ nodeID: span.node_id, result: spanResult(span) }))
		.filter(({ result }) => typeof result.model_error === "string" && result.model_error.trim());
	const fallbackReason = modelFallbacks.length
		? String(modelFallbacks[0].result.model_error)
		: "";
	const fallbackReasonCopy = fallbackReason.includes("model did not return one bounded JSON value")
		? "模型没有返回符合模板的标准结果，系统因此拒绝激活这批内容。"
		: fallbackReason
			? "模型调用没有产生可安全使用的结果，系统已保留原内容并阻止自动激活。"
			: "至少一个步骤需要继续处理；请查看下方步骤和事件记录。";
  const reviewEvents = detail.events.filter((event) =>
    event.event_type.includes("review"),
  ).length;
  const uncertainEffects = detail.effects.filter(
    (effect) => effect.outcome === "unknown" || effect.status === "dispatched",
  ).length;
  const started = new Date(
    detail.run.started_at || detail.run.created_at,
  ).getTime();
  const ended = detail.run.ended_at
    ? new Date(detail.run.ended_at).getTime()
    : Date.now();
  const durationSeconds =
    Number.isFinite(started) && Number.isFinite(ended)
      ? Math.max(0, Math.round((ended - started) / 1000))
      : 0;
  const outcomeFacts = detail.spans
    .flatMap((span) =>
      visibleMetrics(spanResult(span)).map(([key, value]) => ({
        key,
        value,
        node: span.node_id,
      })),
    )
    .filter(
      (item, index, all) =>
        all.findIndex(
          (other) =>
            other.key === item.key &&
            String(other.value) === String(item.value),
        ) === index,
    )
    .slice(-8);
  const replayableNodes = new Set(
    (detail.stage_outputs || []).map((item) => item.node_id),
  );
  const terminal = [
    "completed",
    "completed_with_warnings",
    "failed",
    "cancelled",
    "denied",
  ].includes(detail.run.status);
  return (
    <>
      <div className="run-head">
        <div>
          <p className="micro">{detail.run.run_id}</p>
          <h2>{detail.run.pipeline_id}</h2>
          <div>
            <Status value={detail.run.status} />
            <span>
              {detail.run.caller_type} / {detail.run.channel}
            </span>
            <span>{time(detail.run.started_at || detail.run.created_at)}</span>
          </div>
        </div>
        <div className="run-actions">
          <code>{detail.run.pipeline_hash}</code>
          {detail.run.retry_of_run_id && (
            <span className="run-parent">
              Retry · {detail.run.retry_of_run_id}
            </span>
          )}
          {detail.run.forked_from_run_id && (
            <span className="run-parent">
              Fork · {detail.run.forked_from_run_id}
            </span>
          )}
          {cancellable && (
            <button className="button" onClick={onCancel}>
              取消运行
            </button>
          )}
          {retryable && (
            <button className="button primary" onClick={onRetry}>
              <RefreshCw size={13} />
              从快照重试
            </button>
          )}
        </div>
      </div>
		{detail.run.status === "completed_with_warnings" && (
			<div className="run-warning-explanation" role="alert">
				<AlertTriangle size={20} />
				<div>
					<strong>这次运行完成了流程，但结果没有全部成功</strong>
					<p>{modelFallbacks.length ? `${modelFallbacks.length} 个批次只保存了待补齐骨架，二次沉淀尚未完成；修复模型设置后可回到“能力资产工坊”再次运行。` : fallbackReasonCopy}</p>
					{modelFallbacks.length > 0 && <small>原因：{fallbackReasonCopy}</small>}
					{fallbackReason && <details><summary>查看技术记录</summary><code>{fallbackReason}</code></details>}
				</div>
			</div>
		)}
      <div className="run-result-first">
        <article>
          <small>处理步骤</small>
          <strong>
            {completedStages}/{detail.spans.length}
          </strong>
          <span>{failedStages ? `${failedStages} 个失败` : "已完成阶段"}</span>
        </article>
        <article>
          <small>运行耗时</small>
          <strong>{durationSeconds}s</strong>
          <span>{detail.run.ended_at ? "终态耗时" : "当前已运行"}</span>
        </article>
        <article>
          <small>审核信号</small>
          <strong>{reviewEvents}</strong>
          <span>
            {detail.run.status === "waiting_review"
              ? "正在等待 Owner"
              : "轨迹中的审核事件"}
          </span>
        </article>
        <article>
          <small>外部效果</small>
          <strong>{detail.effects.length}</strong>
          <span>
            {uncertainEffects
              ? `${uncertainEffects} 项结果未确认`
              : "无未知外部结果"}
          </span>
        </article>
      </div>
      {modelHealth && modelHealth.calls > 0 && (
        <div className="run-model-summary">
          <article><small>模型调用</small><strong>{modelHealth.calls}</strong><span>{modelHealth.failed_calls ? `${modelHealth.failed_calls} 次失败` : "全部调用成功"}</span></article>
          <article><small>Token</small><strong>{modelHealth.total_tokens.toLocaleString()}</strong><span>{modelHealth.prompt_tokens.toLocaleString()} in · {modelHealth.completion_tokens.toLocaleString()} out</span></article>
          <article><small>最大延迟</small><strong>{modelHealth.max_latency_ms} ms</strong><span>{modelHealth.provider_reported_calls}/{modelHealth.calls} 次含 provider usage</span></article>
          <article><small>估算费用</small><strong>{modelCostText(modelHealth)}</strong><span>{modelHealth.cost_status === "estimated" ? "全部调用按 Owner 单价估算" : modelHealth.cost_status === "partial_estimate" ? `${modelHealth.priced_calls || 0}/${modelHealth.calls} 次可计价` : "不会自动猜测价格"}</span></article>
        </div>
      )}
      {outcomeFacts.length > 0 && (
        <div className="run-outcome-facts">
          <span>本次实际产物</span>
          {outcomeFacts.map((item) => (
            <article key={`${item.node}:${item.key}:${String(item.value)}`}>
              <small>{item.key.replaceAll("_", " ")}</small>
              <strong>{String(item.value)}</strong>
              <em>{item.node}</em>
            </article>
          ))}
        </div>
      )}
      <div className="tabbar">
        {contextRun && (
          <button
            className={view === "context" ? "active" : ""}
            onClick={() => setView("context")}
          >
            上下文送达
          </button>
        )}
        <button
          className={view === "graph" ? "active" : ""}
          onClick={() => setView("graph")}
        >
          处理步骤
        </button>
        <button
          className={view === "timeline" ? "active" : ""}
          onClick={() => setView("timeline")}
        >
          事件时间线
        </button>
        <button
          className={view === "effects" ? "active" : ""}
          onClick={() => setView("effects")}
        >
          外部副作用 <em>{detail.effects.length}</em>
        </button>
        {modelCalls.length > 0 && (
          <button className={view === "models" ? "active" : ""} onClick={() => setView("models")}>
            模型调用 <em>{modelCalls.length}</em>
          </button>
        )}
      </div>
      {view === "context" && contextRun && (
        <ContextExchangeInspector api={api} runID={detail.run.run_id} />
      )}
      {view === "graph" && (
        <div className="run-graph">
          <div className="graph-line" />
          {detail.spans.map((span, index) => {
            const copy = stageCopy[span.stage_type] || {
              title: span.node_id,
              purpose: span.stage_type,
            };
            const result = spanResult(span);
            return (
              <article
                key={span.span_id}
                style={{ "--column": index + 1 } as React.CSSProperties}
              >
                <span>{index + 1}</span>
                <small>
                  {span.node_id} · {span.stage_type}
                </small>
                <h3>{copy.title}</h3>
                <p>{String(result.operation || copy.purpose)}</p>
                <div className="run-stage-metrics">
                  {visibleMetrics(result).map(([key, value]) => (
                    <span key={key}>
                      <b>{key.replaceAll("_", " ")}</b>
                      <em>{String(value)}</em>
                    </span>
                  ))}
                </div>
                <footer>
                  <Status value={span.status} />
                  <code>{span.span_id}</code>
                  {terminal &&
                    (index === 0 ||
                      detail.spans
                        .slice(0, index)
                        .every(
                          (item) =>
                            item.status === "completed" &&
                            replayableNodes.has(item.node_id),
                        )) && (
                      <button
                        className="run-fork-node"
                        onClick={() => onFork(span.node_id)}
                      >
                        从此阶段分支
                      </button>
                    )}
                </footer>
              </article>
            );
          })}
        </div>
      )}
      {view === "timeline" && (
        <div className="event-timeline">
          {detail.events.map((event) => (
            <article key={event.sequence}>
              <span>{String(event.sequence).padStart(2, "0")}</span>
              <i />
              <div>
                <strong>{event.event_type.replaceAll(".", " · ")}</strong>
                <small>
                  {event.producer} · {time(event.created_at)}
                </small>
                <div className="event-facts">
                  {Object.entries(event.data || {})
                    .slice(0, 6)
                    .map(([key, value]) => (
                      <span key={key}>
                        <b>{key.replaceAll("_", " ")}</b>
                        <em>
                          {typeof value === "object"
                            ? JSON.stringify(value)
                            : String(value)}
                        </em>
                      </span>
                    ))}
                </div>
              </div>
            </article>
          ))}
        </div>
      )}
      {view === "models" && (
        <div className="model-call-list">
          {modelCalls.map((call) => (
            <article key={call.call_id}>
              <div><Status value={call.status} /><strong>{call.provider} / {call.model}</strong><code>{call.node_id || "unbound"}</code></div>
              <p>{call.total_tokens.toLocaleString()} tokens · {call.prompt_tokens.toLocaleString()} in · {call.completion_tokens.toLocaleString()} out · {call.latency_ms} ms</p>
              <small>{call.usage_source === "provider_reported" ? `Provider usage · cached ${call.cached_prompt_tokens || 0} · reasoning ${call.reasoning_tokens || 0}` : "Provider 未返回 usage"}</small>
              <span>{call.pricing_source === "provider_config" ? `${((call.estimated_cost_microminor || 0) / 1_000_000).toFixed(4)} ${call.currency || ""} minor` : "费用未估算"}</span>
              {call.error_code && <em>{call.error_code}</em>}
            </article>
          ))}
        </div>
      )}
      {view === "effects" &&
        (detail.effects.length ? (
          <div className="effect-list">
            {detail.effects.map((effect) => (
              <article key={`${effect.node_id}:${effect.effect_key}`}>
                <div>
                  <CircleDollarSign size={16} />
                  <strong>{effect.effect_key}</strong>
                </div>
                <Status value={effect.status} />
                <p>Outcome · {effect.outcome}</p>
                <code>{effect.node_id}</code>
                <pre>{JSON.stringify(effect.receipt || {}, null, 2)}</pre>
              </article>
            ))}
          </div>
        ) : (
          <Empty icon={ShieldCheck} title="这次运行没有外部副作用">
            纯本地转换不需要外部回执；所有本地写入仍记录在上方步骤结果中。
          </Empty>
        ))}
    </>
  );
}

type LegacyReviewDetail = {
  operation: Record<string, unknown>;
  proposed_memory?: Record<string, unknown>;
  knowledge_unit?: Record<string, unknown>;
  episode?: Record<string, unknown>;
  evidence: Array<Record<string, unknown>>;
};

function operationName(value: unknown) {
  return (
    (
      {
        CREATE: "新增长期记忆",
        UPDATE: "修订长期记忆",
        CORRECT: "纠正长期记忆",
        MERGE: "合并重复记忆",
        SUPERSEDE: "用新记忆取代旧记忆",
        CONFLICT: "处理冲突记忆",
      } as Record<string, string>
    )[String(value).toUpperCase()] || String(value || "受保护记忆变化")
  );
}

function riskName(value: unknown) {
  const risk = String(value || "C").toUpperCase();
  return (
    (
      {
        A: "A级 · 自动处理",
        B: "B级 · 可追溯",
        C: "C级 · 受保护",
        D: "D级 · 高度敏感",
      } as Record<string, string>
    )[risk] || `${risk}级风险`
  );
}

function reviewPatch(item: Record<string, unknown>) {
  if (item.patch && typeof item.patch === "object")
    return item.patch as Record<string, unknown>;
  try {
    return JSON.parse(String(item.patch_json || "{}")) as Record<
      string,
      unknown
    >;
  } catch {
    return {};
  }
}

function RevisionReviewRow({
  item,
  decide,
}: {
  item: Record<string, unknown>;
  decide: (id: string, decision: "approve" | "reject") => void;
}) {
  const diff =
    item.diff && typeof item.diff === "object"
      ? (item.diff as Record<string, unknown>)
      : {};
  const validation =
    item.validation && typeof item.validation === "object"
      ? (item.validation as Record<string, unknown>)
      : {};
  const changed = Array.isArray(diff.changed_fields)
    ? diff.changed_fields.join(" · ")
    : "内容变化";
  const id = String(item.review_id || "");
  return (
    <article>
      <div>
        <div className="review-meta">
          <Status value="asset revision" />
          <code>
            R{String(item.revision)} ← R{String(item.base_revision)}
          </code>
        </div>
        <h3>
          {item.rollback_from_revision
            ? `回滚受治理对象到历史内容 R${String(item.rollback_from_revision)}`
            : "受治理对象新修订等待激活"}
        </h3>
        <p className="review-preview">变更字段：{changed}</p>
        <p className="review-reasons">
          验证：{String(validation.status || "not_run")} · 目标状态：
          {String(item.target_status || "")}
        </p>
        <small>
          {time(String(item.created_at || ""))} · {String(item.object_id || "")}
        </small>
      </div>
      <div>
        <button className="button" onClick={() => decide(id, "reject")}>
          拒绝
        </button>
        <button
          className="button primary"
          onClick={() => decide(id, "approve")}
        >
          批准并激活
        </button>
      </div>
    </article>
  );
}

export function ReviewPage({
  api,
  openRun,
}: {
  api: APIClient;
  openRun: (id: string) => void;
}) {
  const queue = useAsync(async () => {
    const [legacy, harness, revisions] = await Promise.all([
      api.get<{ operations: Array<Record<string, unknown>> }>(
        "/v1/operations?status=review_required&limit=100",
      ),
      api.get<{ reviews: Array<Record<string, unknown>> }>(
        "/v1/pipelines/reviews?status=pending&limit=100",
      ),
      api.get<{ reviews: Array<Record<string, unknown>> }>(
        "/v1/harness/revision-reviews?status=pending&limit=100",
      ),
    ]);
    return {
      operations: legacy.operations,
      reviews: harness.reviews,
      revisionReviews: revisions.reviews,
    };
  }, [api]);
  const [error, setError] = useState("");
  const [selectedLegacy, setSelectedLegacy] = useState("");
  const detail = useAsync<LegacyReviewDetail>(
    selectedLegacy
      ? () => api.get(`/v1/operations/${encodeURIComponent(selectedLegacy)}`)
      : null,
    [api, selectedLegacy],
  );
  async function decideLegacy(id: string, decision: "approve" | "reject") {
    try {
      await api.post(`/v1/operations/${encodeURIComponent(id)}/review`, {
        decision,
      });
      setSelectedLegacy("");
      queue.reload();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function decideRun(id: string, decision: "approve" | "reject") {
    try {
      await api.post(
        `/v1/pipelines/reviews/${encodeURIComponent(id)}/decision`,
        { decision, note: "Owner 在桌面审核中心确认" },
      );
      queue.reload();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function decideRevision(id: string, decision: "approve" | "reject") {
    try {
      await api.post(
        `/v1/harness/revision-reviews/${encodeURIComponent(id)}/decision`,
        {
          decision,
          note:
            decision === "approve"
              ? "Owner 已检查 Diff 与验证结果"
              : "Owner 拒绝该受治理修订",
        },
      );
      queue.reload();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }
  const total =
    (queue.data?.operations.length || 0) +
    (queue.data?.reviews.length || 0) +
    (queue.data?.revisionReviews.length || 0);
  return (
    <LoadBoundary loading={queue.loading} error={queue.error}>
      <div className="review-banner">
        <strong>{total}</strong>
        <div>
          <h2>项变化等待你的判断</h2>
          <p>Agent、模型和插件不能批准自己的受保护写入。</p>
        </div>
        <ShieldCheck size={42} />
      </div>
      {error && <p className="form-error inline-error">{error}</p>}
      {total ? (
        <div className="review-list">
          {queue.data?.reviews.map((item, index) => (
            <article key={String(item.review_id || index)}>
              <div>
                <div className="review-meta">
                  <Status value="pipeline gate" />
                  <code>{String(item.pipeline_id)}</code>
                </div>
                <h3>{String(item.reason || "流程等待 Owner 审核")}</h3>
                <p>
                  Run · {String(item.run_id)} · Node · {String(item.node_id)}
                </p>
                <pre>{JSON.stringify(item.request, null, 2)}</pre>
                <small>{time(String(item.created_at || ""))}</small>
              </div>
              <div>
                <button
                  className="text-button"
                  onClick={() => openRun(String(item.run_id))}
                >
                  查看轨迹
                </button>
                <button
                  className="button"
                  onClick={() => decideRun(String(item.review_id), "reject")}
                >
                  拒绝
                </button>
                <button
                  className="button primary"
                  onClick={() => decideRun(String(item.review_id), "approve")}
                >
                  批准并继续
                </button>
              </div>
            </article>
          ))}
          {queue.data?.revisionReviews.map((item, index) => (
            <RevisionReviewRow
              key={String(item.review_id || index)}
              item={item}
              decide={decideRevision}
            />
          ))}
          {queue.data?.operations.map((item, index) => {
            const patch = reviewPatch(item);
            const id = String(item.operation_id || index);
            const summary = String(item.summary || patch.summary || "");
            return (
              <article key={id}>
                <div>
                  <div className="review-meta">
                    <span className="review-risk">
                      {riskName(item.risk_tier)}
                    </span>
                    <code>{String(item.type || "protected change")}</code>
                  </div>
                  <h3>{operationName(item.type)}</h3>
                  <p className="review-preview">
                    {summary ||
                      "没有可显示的候选正文，请打开详情检查来源后再决定。"}
                  </p>
                  <p className="review-reasons">
                    {Array.isArray(item.reason_codes) &&
                    item.reason_codes.length
                      ? item.reason_codes.join(" · ")
                      : "需要检查候选内容、来源和影响范围。"}
                  </p>
                  <small>
                    置信度 {Math.round(Number(item.confidence || 0) * 100)}% ·{" "}
                    {time(String(item.created_at || ""))}
                  </small>
                </div>
                <div>
                  <button
                    className="text-button"
                    onClick={() => setSelectedLegacy(id)}
                  >
                    查看来源与全文
                  </button>
                  <button
                    className="button"
                    onClick={() => decideLegacy(id, "reject")}
                  >
                    拒绝
                  </button>
                  <button
                    className="button primary"
                    onClick={() => decideLegacy(id, "approve")}
                  >
                    接受
                  </button>
                </div>
              </article>
            );
          })}
        </div>
      ) : (
        <Empty icon={CheckCircle2} title="审核队列为空">
          受保护操作出现时，它会带着来源、理由和产生它的运行来到这里。
        </Empty>
      )}
      {selectedLegacy && (
        <div
          className="drawer-backdrop"
          onMouseDown={() => setSelectedLegacy("")}
        >
          <aside
            className="drawer review-drawer"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button
              className="drawer-close"
              onClick={() => setSelectedLegacy("")}
              aria-label="关闭"
            >
              ×
            </button>
            <LoadBoundary loading={detail.loading} error={detail.error}>
              {detail.data && (
                <>
                  <p className="micro">PROTECTED MEMORY REVIEW</p>
                  <h2>
                    {String(
                      detail.data.proposed_memory?.summary ||
                        detail.data.knowledge_unit?.statement ||
                        operationName(detail.data.operation.type),
                    )}
                  </h2>
                  <div className="drawer-status">
                    <span className="review-risk">
                      {riskName(detail.data.operation.risk_tier)}
                    </span>
                    <span>{operationName(detail.data.operation.type)}</span>
                    <span>
                      置信度{" "}
                      {Math.round(
                        Number(detail.data.operation.confidence || 0) * 100,
                      )}
                      %
                    </span>
                  </div>
                  <section className="review-proposal">
                    <small>准备写入的内容</small>
                    <p>
                      {String(
                        detail.data.proposed_memory?.body ||
                          detail.data.knowledge_unit?.statement ||
                          "这项变化没有可显示的正文，请拒绝并检查产生它的流程。",
                      )}
                    </p>
                    {detail.data.proposed_memory && (
                      <div>
                        <span>
                          层级{" "}
                          {String(detail.data.proposed_memory.tier || "未标注")}
                        </span>
                        <span>
                          状态{" "}
                          {String(
                            detail.data.proposed_memory.status || "candidate",
                          )}
                        </span>
                      </div>
                    )}
                  </section>
                  {detail.data.knowledge_unit && (
                    <section className="review-source-block">
                      <small>抽取依据</small>
                      <blockquote>
                        {String(detail.data.knowledge_unit.statement || "")}
                      </blockquote>
                      <code>
                        {String(detail.data.knowledge_unit.unit_id || "")}
                      </code>
                    </section>
                  )}
                  {detail.data.episode && (
                    <section className="review-source-block">
                      <small>所属会话复盘</small>
                      <strong>
                        {String(detail.data.episode.title || "会话复盘")}
                      </strong>
                      <p>{String(detail.data.episode.summary || "")}</p>
                    </section>
                  )}
                  <section className="review-evidence">
                    <small>
                      原始 Evidence · {detail.data.evidence.length} 条
                    </small>
                    {detail.data.evidence.length ? (
                      detail.data.evidence.map((item, index) => (
                        <article key={String(item.evidence_id || index)}>
                          <p>{String(item.preview || "")}</p>
                          <footer>
                            <code>{String(item.evidence_id || "")}</code>
                            <span>
                              {String(item.source_system || "")} ·{" "}
                              {time(String(item.observed_at || ""))}
                            </span>
                          </footer>
                        </article>
                      ))
                    ) : (
                      <p>没有找到原始 Evidence，建议拒绝并检查导入链路。</p>
                    )}
                  </section>
                  <dl>
                    <div>
                      <dt>操作编号</dt>
                      <dd>
                        <code>
                          {String(detail.data.operation.operation_id || "")}
                        </code>
                      </dd>
                    </div>
                    <div>
                      <dt>审核原因</dt>
                      <dd>
                        {Array.isArray(detail.data.operation.reason_codes)
                          ? detail.data.operation.reason_codes.join(" · ")
                          : "受保护层写入"}
                      </dd>
                    </div>
                  </dl>
                  <div className="review-drawer-actions">
                    <button
                      className="button"
                      onClick={() => decideLegacy(selectedLegacy, "reject")}
                    >
                      拒绝并保留审计
                    </button>
                    <button
                      className="button primary"
                      onClick={() => decideLegacy(selectedLegacy, "approve")}
                    >
                      确认接受
                    </button>
                  </div>
                </>
              )}
            </LoadBoundary>
          </aside>
        </div>
      )}
    </LoadBoundary>
  );
}

export function LegacyStudioPage({
  api,
  projectID,
  openRun,
}: {
  api: APIClient;
  projectID: string;
  openRun: (id: string) => void;
}) {
  const catalog = useAsync(async () => {
    const [pipelines, stages] = await Promise.all([
      api.get<{ pipelines: PipelineVersion[] }>("/v1/pipelines"),
      api.get<{ stages: Array<Record<string, unknown>> }>(
        "/v1/pipelines/stages",
      ),
    ]);
    return { pipelines: pipelines.pipelines, stages: stages.stages };
  }, [api]);
  const [selected, setSelected] = useState<PipelineVersion>();
  const [input, setInput] = useState(
    '{\n  "title": "新商机候选",\n  "next_action": "安排需求确认"\n}',
  );
  const [result, setResult] = useState("");
  async function runPipeline() {
    if (!selected) return;
    try {
      const response = await api.post<{ run_id: string; status: string }>(
        "/v1/pipelines/execute",
        {
          project_id: projectID,
          pipeline_id: selected.pipeline_id,
          pipeline_version: selected.version,
          idempotency_key: `studio-${Date.now()}`,
          input: JSON.parse(input),
        },
      );
      setResult(`${response.status} · ${response.run_id}`);
      openRun(response.run_id);
    } catch (reason) {
      setResult(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <LoadBoundary loading={catalog.loading} error={catalog.error}>
      <div className="studio-layout">
        <aside>
          <p className="micro">PIPELINE CATALOG</p>
          <h2>已发布模板</h2>
          {catalog.data?.pipelines.map((item) => (
            <button
              key={`${item.pipeline_id}:${item.version}`}
              className={
                selected?.pipeline_id === item.pipeline_id ? "active" : ""
              }
              onClick={() => setSelected(item)}
            >
              <GitBranch size={17} />
              <div>
                <strong>{item.name}</strong>
                <small>
                  {item.plugin_id} · v{item.version}
                </small>
              </div>
              <ChevronRight size={15} />
            </button>
          ))}
        </aside>
        <main>
          {selected ? (
            <>
              <div className="studio-head">
                <div>
                  <Status value={selected.status} />
                  <h2>{selected.name}</h2>
                  <p>{selected.definition.intent}</p>
                </div>
                <button className="button primary" onClick={runPipeline}>
                  <Play size={14} />
                  运行此版本
                </button>
              </div>
              <div className="pipeline-canvas">
                <div className="canvas-grid" />
                {selected.definition.nodes.map((node, index) => (
                  <article
                    key={node.id}
                    style={
                      {
                        "--x": `${9 + index * Math.min(27, 76 / Math.max(1, selected.definition.nodes.length - 1))}%`,
                        "--y": `${index % 2 ? 55 : 22}%`,
                      } as React.CSSProperties
                    }
                  >
                    <span>{index + 1}</span>
                    <small>{node.stage_type}</small>
                    <h3>{node.id}</h3>
                    <code>v{node.stage_version}</code>
                    {index < selected.definition.nodes.length - 1 && (
                      <ArrowRight size={18} />
                    )}
                  </article>
                ))}
              </div>
              <div className="studio-bottom">
                <label>
                  本地运行输入
                  <textarea
                    value={input}
                    onChange={(event) => setInput(event.target.value)}
                    rows={7}
                    spellCheck={false}
                  />
                </label>
                <div>
                  <p className="micro">VALIDATION</p>
                  <p>
                    <CheckCircle2 size={15} /> 已发布版本不可变
                  </p>
                  <p>
                    <CheckCircle2 size={15} />{" "}
                    {selected.definition.nodes.length} 个节点，无环
                  </p>
                  <p>
                    <CheckCircle2 size={15} /> 能力：
                    {selected.definition.required_capabilities.join(", ") ||
                      "纯本地"}
                  </p>
                  {result && <code>{result}</code>}
                </div>
              </div>
            </>
          ) : (
            <Empty icon={GitBranch} title="选择一个流程模板">
              这里会显示节点、依赖、版本与本地预览输入。
            </Empty>
          )}
        </main>
      </div>
    </LoadBoundary>
  );
}

export function LegacyPluginsPage({ api }: { api: APIClient }) {
  const list = useAsync(
    () => api.get<{ plugins: PluginVersion[] }>("/v1/plugins"),
    [api],
  );
  const [notice, setNotice] = useState("");
  async function install() {
    try {
      const method = window.go?.main?.DesktopBridge?.InstallPluginPackage;
      if (!method) throw new Error("插件安装只在桌面应用中开放");
      const item = await method("", [], false);
      if (item?.plugin_id)
        setNotice(
          `已安装 ${item.plugin_id}@${item.version}；启用前仍需为项目授予能力。`,
        );
      list.reload();
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <LoadBoundary loading={list.loading} error={list.error}>
      <div className="plugin-intro">
        <div>
          <PackageCheck size={25} />
          <p className="micro">TRUSTED EXTENSION BASE</p>
          <h2>
            插件贡献类型和流程，
            <br />
            不能取得 Owner 权限。
          </h2>
          <button className="button light" onClick={install}>
            <Upload size={14} />
            安装签名插件
          </button>
        </div>
        <dl>
          <div>
            <dt>已登记</dt>
            <dd>{list.data?.plugins.length || 0}</dd>
          </div>
          <div>
            <dt>外部签名</dt>
            <dd>
              {list.data?.plugins.filter(
                (item) => item.signature_status === "verified",
              ).length || 0}
            </dd>
          </div>
          <div>
            <dt>隔离</dt>
            <dd>
              {list.data?.plugins.filter(
                (item) => item.status === "quarantined",
              ).length || 0}
            </dd>
          </div>
        </dl>
      </div>
      {notice && <p className="surface-notice">{notice}</p>}
      <div className="plugin-grid">
        {list.data?.plugins.map((item) => (
          <article key={`${item.plugin_id}:${item.version}`}>
            <div className="plugin-top">
              <span>
                {item.signature_status === "bundled" ? (
                  <Sparkles size={16} />
                ) : (
                  <KeyRound size={16} />
                )}
              </span>
              <Status value={item.status} />
            </div>
            <h3>{item.name}</h3>
            <code>
              {item.plugin_id}@{item.version}
            </code>
            <p>
              {item.permissions.length
                ? item.permissions.join(" · ")
                : "无需额外能力"}
            </p>
            <div className="plugin-contrib">
              <span>
                <strong>{item.contributions.memory_types?.length || 0}</strong>{" "}
                记忆类型
              </span>
              <span>
                <strong>{item.contributions.pipelines?.length || 0}</strong>{" "}
                流程
              </span>
              <span>
                <strong>{item.contributions.stages?.length || 0}</strong> 阶段
              </span>
            </div>
            <footer>
              <Status value={item.signature_status} />
              <span>{item.publisher}</span>
            </footer>
          </article>
        ))}
      </div>
    </LoadBoundary>
  );
}

export function ConnectionsPage({
  api,
  projects,
  mode = "advanced",
}: {
  api: APIClient;
  projects: ProjectSummary[];
  mode?: UIMode;
}) {
  const state = useAsync(async () => {
    const [models, agents, capabilities, sources] = await Promise.all([
      api.get<Record<string, unknown>>("/v1/model/config"),
      api.get<{
        agents: Array<Record<string, unknown>>;
        allowed_permissions: string[];
      }>("/v1/agents"),
      api.get<{
        tools: Array<Record<string, unknown>>;
        command: string;
        context_bridge?: ContextBridgeManifest;
      }>("/v1/integrations/capabilities"),
      api.get<{ sources: Array<Record<string, unknown>> }>("/v1/sources"),
    ]);
    return {
      models,
      agents: agents.agents,
      allowedPermissions: agents.allowed_permissions,
      capabilities,
      sources: sources.sources,
    };
  }, [api]);
  const [providerModal, setProviderModal] = useState(false);
  const [providerDraft, setProviderDraft] = useState<Record<string, unknown> | null>(null);
	const [providerForm, setProviderForm] = useState({name:"",kind:"openai",protocol:"openai_responses",baseURL:"https://api.openai.com/v1",model:"gpt-5.6"});
  const [agentModal, setAgentModal] = useState(false);
  const [oneTimeToken, setOneTimeToken] = useState("");
  const [notice, setNotice] = useState("");
  const providers = (state.data?.models.providers || []) as Array<
    Record<string, unknown>
  >;
	const presets = (state.data?.models.presets || []) as Array<Record<string, unknown>>;
	const modelCatalog = (state.data?.models.model_catalog || []) as Array<Record<string, unknown>>;
	const availableModels = modelCatalog.filter(item=>String(item.provider_kind)===providerForm.kind);
	const selectedModelKnowledge = availableModels.find(item=>String(item.model_id)===providerForm.model);
  const runtime = (state.data?.models.runtime || {}) as Record<string, unknown>;
  const usage = (state.data?.models.usage || { window_hours: 24, generated_at: "", health: { calls: 0, successful_calls: 0, failed_calls: 0, provider_reported_calls: 0, prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, reasoning_tokens: 0, cached_prompt_tokens: 0, total_latency_ms: 0, max_latency_ms: 0, estimated_cost_microminor: 0, cost_status: "unavailable" }, providers: [] }) as ModelUsageDashboard;
  const usageByProvider = new Map<string, ModelUsageDashboard["providers"][number]>();
  for (const item of usage.providers) {
    if (!usageByProvider.has(item.provider_id)) usageByProvider.set(item.provider_id, item);
  }
  const providerDraftPricing = (providerDraft?.pricing || {}) as Record<string, unknown>;
	function protocolLabel(protocol: unknown) {
		return ({openai_chat:"Chat Completions",openai_responses:"Responses",anthropic_messages:"Anthropic Messages"} as Record<string,string>)[String(protocol)] || String(protocol || "未指定");
	}
	function openProviderEditor(provider: Record<string, unknown> | null) {
		setProviderDraft(provider);
		setProviderForm(provider ? {
			name:String(provider.name||""),kind:String(provider.kind||"openai"),protocol:String(provider.protocol||"openai_chat"),baseURL:String(provider.base_url||""),model:String(provider.model||""),
		} : {name:"OpenAI",kind:"openai",protocol:"openai_responses",baseURL:"https://api.openai.com/v1",model:"gpt-5.6"});
		setProviderModal(true);
	}
	function applyProviderPreset(preset: Record<string, unknown>) {
		setProviderForm({name:String(preset.name||""),kind:String(preset.kind||"openai_compatible"),protocol:String(preset.protocol||"openai_chat"),baseURL:String(preset.base_url||""),model:String(preset.example_model||"")});
	}
  async function saveProvider(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setNotice("");
    const form = new FormData(event.currentTarget);
    try {
      const payload = {
		name: providerForm.name,
		kind: providerForm.kind,
		protocol: providerForm.protocol,
		base_url: providerForm.baseURL,
		model: providerForm.model,
        api_key: form.get("api_key"),
        enabled: providerDraft ? Boolean(providerDraft.enabled) : true,
        pricing: {
          currency: String(form.get("pricing_currency") || ""),
          input_per_million_minor: Number(form.get("input_per_million_minor") || 0),
          output_per_million_minor: Number(form.get("output_per_million_minor") || 0),
        },
      };
      if (providerDraft?.provider_id) {
        await api.put(`/v1/model/providers/${encodeURIComponent(String(providerDraft.provider_id))}`, payload);
      } else {
        await api.post("/v1/model/providers", payload);
      }
      setProviderModal(false);
      setProviderDraft(null);
      setNotice(providerDraft ? "模型提供方与定价已更新。" : "模型提供方已保存。请先测试连接，再启用 Agent 模式。");
      state.reload();
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function testProvider(id: string) {
    try {
      const result = await api.post<{models?: string[]; selected_model_found?: boolean}>(`/v1/model/providers/${encodeURIComponent(id)}/test`, {});
      const count = Array.isArray(result.models) ? result.models.length : 0;
      setNotice(count > 0
        ? `连接检查通过；当前账号返回 ${count} 个模型，${result.selected_model_found ? "已找到所选模型" : "但未找到所选模型"}。这不等同于沉淀质量验收。`
        : "连接检查通过；这不等同于沉淀质量验收。");
      state.reload();
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : String(reason));
      state.reload();
    }
  }
  async function setRuntime(mode: "rules" | "agent", providerID = "") {
    try {
      await api.put("/v1/model/runtime", {
        mode,
        active_provider_id: providerID,
        fallback_to_rules: true,
      });
      setNotice(
        mode === "agent"
          ? "Agent 模式已启用；失败时安全回退到本地规则。"
          : "已切换为完全本地规则模式。",
      );
      state.reload();
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : String(reason));
    }
  }
  async function saveAgent(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setNotice("");
    const form = new FormData(event.currentTarget);
    try {
      const permissions = form.getAll("permissions").map(String);
      if (form.get("team_bundle") === "on") permissions.push("team.private", "team.blackboard.read", "team.blackboard.write", "team.blackboard.share");
      const response = await api.post<{ token: string }>("/v1/agents", {
        name: form.get("name"),
        kind: form.get("kind"),
        permissions: [...new Set(permissions)],
        project_ids: form.getAll("projects"),
        all_projects: form.get("all_projects") === "on",
      });
      setOneTimeToken(response.token);
      state.reload();
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : String(reason));
    }
  }
  return (
    <LoadBoundary loading={state.loading} error={state.error}>
      <div className="connection-hero">
        <div>
          <Bot size={25} />
          <p className="micro">每个 AI 都有独立身份</p>
          <h2>每个 AI 都有自己的门和钥匙。</h2>
          <p>选择它能访问哪些记忆空间、能读什么、能写什么；它不能超出这些范围。</p>
        </div>
        {mode === "advanced" && <div className="access-formula">
          <span>Caller</span>
          <i>∩</i>
          <span>Project</span>
          <i>∩</i>
          <span>Plugin</span>
          <i>∩</i>
          <span>Stage</span>
        </div>}
      </div>
      {notice && <p className="surface-notice">{notice}</p>}
      <div className="two-column connections">
        <Section
          title="模型配置中心"
          detail="连接成功与沉淀质量是两项不同检查。"
          action={
            <button className="button" onClick={() => openProviderEditor(null)}>
              <Plus size={13} />
              添加模型
            </button>
          }
        >
          <div className="model-runtime">
            <div>
              <span>当前模式</span>
              <strong>{String(runtime.mode || "rules")}</strong>
            </div>
            <div className="runtime-actions">
              <Status value={providers.length ? "configured" : "local rules"} />
              {runtime.mode === "agent" && (
                <button
                  className="text-button"
                  onClick={() => setRuntime("rules")}
                >
                  改用本地规则
                </button>
              )}
            </div>
            <p>{String(state.data?.models.privacy_notice || "")}</p>
            <small>
              密钥存储 · {String(state.data?.models.secret_store || "unknown")}{" "}
              · {state.data?.models.secret_persistent ? "持久" : "非持久"}
            </small>
			{state.data?.models.model_catalog_notice ? <small>模型知识更新于 {String(state.data.models.model_catalog_updated_at || "未知")} · {String(state.data.models.model_catalog_notice)}</small> : null}
          </div>
          <div className="model-health-strip">
            <article><small>最近 {usage.window_hours}h 调用</small><strong>{usage.health.calls}</strong><span>{usage.health.failed_calls ? `${usage.health.failed_calls} 次失败` : "无失败调用"}</span></article>
            <article><small>Provider Token</small><strong>{usage.health.total_tokens.toLocaleString()}</strong><span>{usage.health.provider_reported_calls}/{usage.health.calls} 次含 usage</span></article>
			<article><small>最大延迟</small><strong>{usage.health.max_latency_ms} ms</strong><span>以单次模型请求计</span></article>
            <article><small>估算费用</small><strong>{modelCostText(usage.health)}</strong><span>{usage.health.cost_status === "estimated" ? "全部调用仅使用 Owner 单价" : usage.health.cost_status === "partial_estimate" ? `${usage.health.priced_calls || 0}/${usage.health.calls} 次可计价` : "未自动猜测价格"}</span></article>
          </div>
          {providers.map((provider) => {
            const providerID = String(provider.provider_id);
            const providerUsage = usageByProvider.get(providerID);
            const pricing = (provider.pricing || {}) as Record<string, unknown>;
            return <div className="provider-row" key={providerID}>
              <Network size={16} />
              <div>
                <strong>{String(provider.name)}</strong>
                <small>
				  {String(provider.model)} · {protocolLabel(provider.protocol)} · {String(provider.base_url)}
                </small>
                <small className="provider-health-line">
                  {providerUsage ? `${providerUsage.health.calls} calls · ${providerUsage.health.total_tokens.toLocaleString()} tokens · max ${providerUsage.health.max_latency_ms} ms · ${modelCostText(providerUsage.health)}` : "最近 24h 暂无模型调用"}
                </small>
                <small className="provider-pricing-line">
                  {pricing.configured ? `定价 ${String(pricing.currency)} · 输入 ${String(pricing.input_per_million_minor)} / 输出 ${String(pricing.output_per_million_minor)} minor / 1M tokens` : "未配置定价 · 不估算费用"}
                </small>
              </div>
              <div className="row-actions">
                <Status value={String(provider.status)} />
				<button className="text-button" onClick={() => openProviderEditor(provider)}>编辑</button>
                <button
                  className="text-button"
                  onClick={() => testProvider(String(provider.provider_id))}
                >
                  测试
                </button>
                <button
                  className="text-button"
                  onClick={() =>
                    setRuntime("agent", String(provider.provider_id))
                  }
                >
                  启用
                </button>
              </div>
            </div>;
          })}
        </Section>
        <Section
          title="Agent 权限主体"
          detail="令牌只显示一次，数据库只保存哈希。"
          action={
            <button
              className="button"
              onClick={() => {
                setOneTimeToken("");
                setAgentModal(true);
              }}
            >
              <UserPlus size={13} />
              创建 Agent
            </button>
          }
        >
          {state.data?.agents.length ? (
            <div className="agent-list">
              {state.data.agents.map((agent) => (
                <div key={String(agent.agent_id)}>
                  <Bot size={17} />
                  <div>
                    <strong>{String(agent.name)}</strong>
                    <small>
                      {String(agent.kind)} ·{" "}
                      {Array.isArray(agent.permissions)
                        ? agent.permissions.join(", ")
                        : ""}
                    </small>
                  </div>
                  <Status value={String(agent.status)} />
                </div>
              ))}
            </div>
          ) : (
            <Empty icon={Bot} title="还没有外部 Agent">
              创建后可为 Codex、ChatGPT-compatible 或 DSH
              分配不同项目与读写权限。
            </Empty>
          )}
        </Section>
      </div>
      {mode === "advanced" && <Section
        title="MCP 能力表面"
        detail="实际工具列表由身份和授权过滤；不是所有 Agent 都看到同一套能力。"
      >
        <div className="tool-grid">
          {state.data?.capabilities.tools.map((tool, index) => (
            <article key={String(tool.name || index)}>
              <div>
                <Status value={String(tool.mode)} />
                <code>{String(tool.permission)}</code>
              </div>
              <h3>{String(tool.name)}</h3>
              <p>{String(tool.summary)}</p>
              <small>{String(tool.scope)}</small>
            </article>
          ))}
        </div>
      </Section>}
      {mode === "advanced" && <ContextBridgeCapabilities
        manifest={state.data?.capabilities.context_bridge}
      />}
      {mode === "advanced" && <div className="dsh-banner">
        <div>
          <span>EXPERIMENTAL ADAPTER</span>
          <h3>DeepSeek Harness Bridge</h3>
          <p>
            已建立独立插件边界；只有完成版本契约验证后才会标记为兼容。Memory
            Harness 不依赖 DSH 才能运行。
          </p>
        </div>
        <Status value="experimental" />
      </div>}
      {providerModal && (
        <div
          className="modal-backdrop"
          onMouseDown={() => { setProviderModal(false); setProviderDraft(null); }}
        >
          <form
            className="modal"
            onSubmit={saveProvider}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <p className="micro">MODEL PROVIDER</p>
            <h2>{providerDraft ? "编辑模型提供方" : "添加模型提供方"}</h2>
			{!providerDraft && presets.length>0 && <div className="model-preset-grid"><span>常用接入</span>{presets.map(preset=><button type="button" key={String(preset.preset_id)} onClick={()=>applyProviderPreset(preset)}><strong>{String(preset.name)}</strong><small>{String(preset.description||"")}</small></button>)}</div>}
            <label>
              显示名称
			  <input name="name" required maxLength={120} value={providerForm.name} onChange={event=>setProviderForm(current=>({...current,name:event.target.value}))} />
            </label>
            <label>
              类型
			  <select name="kind" value={providerForm.kind} onChange={event=>{const kind=event.target.value;setProviderForm(current=>({...current,kind,protocol:kind==='anthropic'?'anthropic_messages':kind==='openai'||kind==='opencode_go'?'openai_responses':'openai_chat'}))}}>
                <option value="openai">OpenAI</option>
				<option value="anthropic">Anthropic / Claude</option>
				<option value="opencode_go">OpenCode Go</option>
                <option value="deepseek">DeepSeek</option>
                <option value="openai_compatible">
                  OpenAI Compatible / Local
                </option>
              </select>
            </label>
			<label>
			  调用方式
			  <select name="protocol" aria-label="调用方式" value={providerForm.protocol} onChange={event=>setProviderForm(current=>({...current,protocol:event.target.value}))}>
				<option value="openai_responses">Responses（OpenAI 新接口）</option>
				<option value="openai_chat">Chat Completions（兼容接口）</option>
				<option value="anthropic_messages">Anthropic Messages</option>
			  </select>
			  <small>OpenCode Go 的不同模型使用不同调用方式；从下方已知模型选择时会自动匹配。</small>
			</label>
            <label>
              API 地址
              <input
                name="base_url"
                required
				value={providerForm.baseURL}
				onChange={event=>setProviderForm(current=>({...current,baseURL:event.target.value}))}
              />
            </label>
            <label>
              模型
			  <input name="model" aria-label="模型" list="known-provider-models" required value={providerForm.model} onChange={event=>{const model=event.target.value;const known=modelCatalog.find(item=>String(item.provider_kind)===providerForm.kind&&String(item.model_id)===model);setProviderForm(current=>({...current,model,protocol:known?String(known.protocol):current.protocol}))}} />
			  <datalist id="known-provider-models">{availableModels.map(item=><option key={String(item.model_id)} value={String(item.model_id)}>{String(item.name)}</option>)}</datalist>
            </label>
			{selectedModelKnowledge&&<div className="model-knowledge-card"><strong>{String(selectedModelKnowledge.name)}</strong><span>{protocolLabel(selectedModelKnowledge.protocol)} · {Array.isArray(selectedModelKnowledge.input)?selectedModelKnowledge.input.join(' + '):'text'}</span><p>{String(selectedModelKnowledge.best_for)}</p><small>目录来源：{String(selectedModelKnowledge.source)}。实际可用性仍以“测试连接”返回为准。</small></div>}
            <fieldset className="model-pricing-fields">
              <legend>可选费用估算</legend>
              <p>单价由 Owner 明确填写；系统不会联网猜测价格。单位是“每 100 万 token 的最小货币单位”，例如 USD cents / CNY fen。输入与输出都为 0 时关闭费用估算。</p>
              <div className="form-grid">
                <label>币种
                  <input name="pricing_currency" defaultValue={String(providerDraftPricing.currency || "USD")} maxLength={3} pattern="[A-Za-z]{3}" />
                </label>
                <label>输入单价 / 1M tokens
                  <input name="input_per_million_minor" type="number" min="0" step="1" defaultValue={String(providerDraftPricing.input_per_million_minor || 0)} />
                </label>
                <label>输出单价 / 1M tokens
                  <input name="output_per_million_minor" type="number" min="0" step="1" defaultValue={String(providerDraftPricing.output_per_million_minor || 0)} />
                </label>
              </div>
            </fieldset>
            <label>
              {providerDraft ? "API Key（留空则保留现有密钥）" : "API Key（仅存入系统钥匙串）"}
              <input
                name="api_key"
                type="password"
                autoComplete="new-password"
              />
            </label>
            <div className="modal-actions">
              <button
                type="button"
                className="button"
                onClick={() => { setProviderModal(false); setProviderDraft(null); }}
              >
                取消
              </button>
              <button className="button primary">{providerDraft ? "保存修改" : "保存提供方"}</button>
            </div>
          </form>
        </div>
      )}
      {agentModal && (
        <div
          className="modal-backdrop"
          onMouseDown={() => setAgentModal(false)}
        >
          <form
            className="modal wide"
            onSubmit={saveAgent}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <p className="micro">AGENT PRINCIPAL</p>
            <h2>
              {oneTimeToken ? "立即保存一次性令牌" : "创建最小权限 Agent"}
            </h2>
            {oneTimeToken ? (
              <>
                <p className="token-warning">
                  关闭后不能再次查看；服务端只保存不可逆哈希。
                </p>
                <textarea
                  className="token-output"
                  readOnly
                  value={oneTimeToken}
                  rows={4}
                />
                <div className="modal-actions">
                  <button
                    type="button"
                    className="button"
                    onClick={() => navigator.clipboard.writeText(oneTimeToken)}
                  >
                    <Copy size={13} />
                    复制令牌
                  </button>
                  <button
                    type="button"
                    className="button primary"
                    onClick={() => setAgentModal(false)}
                  >
                    我已安全保存
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className="form-grid">
                  <label>
                    名称
                    <input
                      name="name"
                      required
                      maxLength={120}
                      placeholder="Codex Research"
                    />
                  </label>
                  <label>
                    客户端类型
                    <select name="kind" defaultValue="codex">
                      <option value="codex">Codex</option>
                      <option value="chatgpt">ChatGPT</option>
                      <option value="dsh">DeepSeek Harness</option>
                      <option value="custom">其他 MCP 客户端</option>
                    </select>
                  </label>
                </div>
                {mode === "basic" ? <fieldset>
                  <legend>这个 AI 可以做什么</legend>
                  <input type="hidden" name="permissions" value="memory.read" />
                  <input type="hidden" name="permissions" value="project.read" />
                  <div className="agent-permission-presets">
                    <label><input type="checkbox" name="permissions" value="memory.capture"/><span><strong>可以写入原材料</strong><small>允许它把明确内容保存到选中的记忆空间。</small></span></label>
                    <label><input type="checkbox" name="permissions" value="memory.propose"/><span><strong>可以提出记忆建议</strong><small>只能提交建议，仍需你审核后才生效。</small></span></label>
                    <label><input type="checkbox" name="team_bundle"/><span><strong>参与多 AI 协作</strong><small>可写私密草稿、读取直接共享内容，并主动提交或调整自己的共享内容。</small></span></label>
                  </div>
                  <p className="field-help">默认已允许读取选中空间的项目上下文和记忆。</p>
                </fieldset> : <fieldset>
                  <legend>逐项权限</legend>
                  <div className="check-grid">
                    {state.data?.allowedPermissions.map((permission) => (
                      <label key={permission}>
                        <input
                          type="checkbox"
                          name="permissions"
                          value={permission}
                          defaultChecked={
                            permission === "memory.read" ||
                            permission === "project.read"
                          }
                        />
                        {permission}
                      </label>
                    ))}
                  </div>
                </fieldset>}
                <fieldset>
                  <legend>可以访问哪些记忆空间</legend>
                  {mode === "advanced" && <label className="check-line">
                    <input name="all_projects" type="checkbox" />
                    允许全部项目（需明确选择）
                  </label>}
                  <div className="check-grid">
                    {projects.map((item) => (
                      <label key={item.project.project_id}>
                        <input
                          type="checkbox"
                          name="projects"
                          value={item.project.project_id}
                        />
                        {item.project.name}
                      </label>
                    ))}
                  </div>
                </fieldset>
                <div className="modal-actions">
                  <button
                    type="button"
                    className="button"
                    onClick={() => setAgentModal(false)}
                  >
                    取消
                  </button>
                  <button className="button primary">创建并生成令牌</button>
                </div>
              </>
            )}
          </form>
        </div>
      )}
    </LoadBoundary>
  );
}

export function HealthPage({ api, onOpenManual }: { api: APIClient; onOpenManual: () => void }) {
  const health = useAsync(
    () => api.get<Record<string, unknown>>("/v1/health/detail"),
    [api],
  );
  const [backupNotice, setBackupNotice] = useState("");
  const doctor = (health.data?.doctor || {}) as {
    status?: string;
    checks?: Array<Record<string, unknown>>;
  };
  const search = (health.data?.search || {}) as Record<string, unknown>;
  async function exportBackup() {
    try {
      const method = window.go?.main?.DesktopBridge?.ExportBackup;
      if (!method) throw new Error("备份导出只在桌面应用中开放");
      const path = await method();
      if (path) setBackupNotice(`备份已写入：${path}`);
    } catch (reason) {
      setBackupNotice(
        reason instanceof Error ? reason.message : String(reason),
      );
    }
  }
  return (
    <LoadBoundary loading={health.loading} error={health.error}>
      <div className="health-hero">
        <div>
          <HeartPulse size={28} />
          <p className="micro">SYSTEM INTEGRITY</p>
          <h2>
            {health.data?.status === "ok"
              ? "当前内核通过完整性检查"
              : "系统有需要处理的健康问题"}
          </h2>
          <p>{String(health.data?.home || "")}</p>
          <button className="button light" onClick={exportBackup}>
            <Download size={14} />
            导出完整备份
          </button>
        </div>
        <div className="health-ring">
          <strong>{doctor.status === "pass" ? "PASS" : "CHECK"}</strong>
          <span>Doctor</span>
        </div>
      </div>
      {backupNotice && <p className="surface-notice">{backupNotice}</p>}
      <section className="health-manual-entry" aria-label="操作手册入口">
        <span><BookOpen size={24} /></span>
        <div>
          <p className="micro">OPERATION MANUAL</p>
          <h2>操作手册</h2>
          <p>从第一次导入，到选择原材料、处理待办、连接 Codex 和多 AI 协作，每个功能都按“怎么操作、系统会做什么、不会做什么”说明。</p>
        </div>
        <button className="button primary" onClick={onOpenManual}>
          打开操作手册
          <ArrowRight size={14} />
        </button>
      </section>
      <div className="metric-row">
        <Metric
          label="CORE VERSION"
          value={String(health.data?.version || "—")}
          detail="当前运行版本"
          tone="ink"
        />
        <Metric
          label="UPTIME"
          value={`${Math.floor(Number(health.data?.uptime_seconds || 0) / 60)}m`}
          detail={`启动于 ${time(String(health.data?.started_at || ""))}`}
        />
        <Metric
          label="SEARCH"
          value={search.consistent ? "一致" : "检查"}
          detail={`${String(search.indexed || search.indexed_evidence || 0)} 条索引`}
        />
        <Metric
          label="OWNER SESSION"
          value="Active"
          detail="仅存在于桌面进程内存"
          tone="sage"
        />
      </div>
      <Section
        title="Doctor 检查项"
        detail="失败、跳过或超时都不会被写成 PASS。"
      >
        <div className="health-list">
          {doctor.checks?.map((check, index) => (
            <div key={String(check.name || index)}>
              <span>
                {String(check.name || check.check || `check-${index + 1}`)}
              </span>
              <code>{String(check.detail || check.message || "")}</code>
              <Status value={String(check.status || "unknown")} />
            </div>
          )) || (
            <Empty icon={Activity} title="未返回细分检查">
              请查看 Doctor 汇总状态。
            </Empty>
          )}
        </div>
      </Section>
    </LoadBoundary>
  );
}

function AppShell({
  session,
}: {
  session: Extract<ConnectionState, { mode: "desktop" }>["session"];
}) {
  const api = useMemo(() => new APIClient(session), [session]);
  const [page, setPage] = useState<PageID>(pageFromHash);
  const [mode, setMode] = useState<UIMode>(() =>
    window.localStorage.getItem("memory-harness.ui-mode") === "advanced" ? "advanced" : "basic",
  );
  const [collapsed, setCollapsed] = useState(false);
  const [builderOpen, setBuilderOpen] = useState(() =>
    ["experience", "adaptation", "portable", "strategy", "studio", "plugins"].includes(pageFromHash()),
  );
  const [requestedRun, setRequestedRun] = useState("");
  const [newSpaceOpen, setNewSpaceOpen] = useState(false);
  const [newSpaceError, setNewSpaceError] = useState("");
  const [creatingSpace, setCreatingSpace] = useState(false);
  const registry = useAsync(
    () => api.get<{ projects: ProjectSummary[] }>("/v1/projects"),
    [api],
  );
  const [projectID, setProjectID] = useState("");
  useEffect(() => window.localStorage.setItem("memory-harness.ui-mode", mode), [mode]);
  useEffect(() => {
    if (projectID || !registry.data?.projects.length) return;
    const remembered = window.localStorage.getItem(
      "memory-harness.current-project",
    );
    const preferred =
      registry.data.projects.find(
        (item) => item.project.project_id === remembered,
      ) ||
      registry.data.projects.find((item) => item.project.slug === "personal") ||
      registry.data.projects[0];
    setProjectID(preferred.project.project_id);
  }, [projectID, registry.data]);
  useEffect(() => {
    if (projectID)
      window.localStorage.setItem("memory-harness.current-project", projectID);
  }, [projectID]);
  useEffect(() => {
    const handler = () => {
      const target = pageFromHash();
      setPage(target);
      setBuilderOpen(
        ["experience", "adaptation", "portable", "strategy", "studio", "plugins"].includes(target),
      );
    };
    window.addEventListener("hashchange", handler);
    return () => window.removeEventListener("hashchange", handler);
  }, []);
  const navigate = (target: PageID) => {
    setBuilderOpen(
      ["experience", "adaptation", "portable", "strategy", "studio", "plugins"].includes(target),
    );
    window.location.hash = target;
    setPage(target);
  };
  const openHelp = (section: PageID | "start" = page) => {
    window.sessionStorage.setItem("memory-harness.help-section", section);
    navigate("help");
  };
  async function createSpace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreatingSpace(true);
    setNewSpaceError("");
    const form = new FormData(event.currentTarget);
    try {
      const created = await api.post<Project>("/v1/projects", {
        name: String(form.get("name") || "").trim(),
        description: String(form.get("description") || "").trim(),
        slug: mode === "advanced" ? String(form.get("slug") || "").trim() : "",
        default_currency: mode === "advanced" ? String(form.get("currency") || "CNY") : "CNY",
        budget_minor: mode === "advanced" ? Math.round(Number(form.get("budget") || 0) * 100) : 0,
      });
      await registry.reload();
      setProjectID(created.project_id);
      setNewSpaceOpen(false);
      navigate("import");
    } catch (reason) {
      setNewSpaceError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setCreatingSpace(false);
    }
  }
  const openRun = (id: string) => {
    setRequestedRun(id);
    navigate("runs");
  };
  const currentSummary = registry.data?.projects.find(
    (item) => item.project.project_id === projectID,
  );
  const currentProject = currentSummary?.project;
  const copy = pageCopy[page];
  return (
    <div className={`app-shell ${collapsed ? "collapsed" : ""}`}>
      <aside className="sidebar">
        <div className="brand">
          <span>
            <BrainCircuit size={22} />
          </span>
          <div>
            <strong>Memory Harness</strong>
            <small>你的可编程长期记忆</small>
          </div>
          <button
            onClick={() => setCollapsed((value) => !value)}
            aria-label="折叠导航"
          >
            {collapsed ? <Menu size={17} /> : <PanelLeftClose size={17} />}
          </button>
        </div>
        <nav>
          {navigation.map((group) => {
            if (mode === "basic" && group.advanced) return null;
            const basicPages = new Set<PageID>(["home", "import", "search", "memory", "team", "projects", "review", "connections", "health", "help"]);
            const visibleItems = mode === "basic" ? group.items.filter((item) => basicPages.has(item.id)) : group.items;
            if (!visibleItems.length) return null;
            return (
            <div
              key={group.group}
              className={group.advanced ? "advanced-nav" : ""}
            >
              {group.advanced ? (
                <button
                  className={`builder-toggle ${builderOpen ? "open" : ""}`}
                  onClick={() => setBuilderOpen((value) => !value)}
                >
                  <span>
                    <small>按需打开</small>
                    {group.group}
                  </span>
                  <ChevronDown size={14} />
                </button>
              ) : (
                <p>{group.group}</p>
              )}
              {(!group.advanced || builderOpen) &&
                visibleItems.map((item) => {
                  const Icon = item.icon;
                  return (
                    <button
                      key={item.id}
                      className={page === item.id ? "active" : ""}
                      onClick={() => navigate(item.id)}
                      title={item.label}
                    >
                      <Icon size={17} />
                      <span>
                        {item.label}
                        <small>{item.hint}</small>
                      </span>
                      {page === item.id && <i />}
                    </button>
                  );
                })}
            </div>
          );})}
        </nav>
        <footer>
          <span className="live-dot" />
          <div>
            <strong>本地记忆已连接</strong>
            <small>{session.version}</small>
          </div>
        </footer>
      </aside>
      <main className="workspace">
        <div className="scopebar">
          <div>
            <span>当前记忆边界</span>
            <select
              value={projectID}
              onChange={(event) => setProjectID(event.target.value)}
            >
              {registry.data?.projects.map((item) => (
                <option
                  value={item.project.project_id}
                  key={item.project.project_id}
                >
                  {item.project.name}
                </option>
              ))}
            </select>
          </div>
          <button className="scope-create" onClick={() => setNewSpaceOpen(true)}>
            <Plus size={15} />
            新建记忆空间
          </button>
          <button className="scope-search" onClick={() => navigate("search")}>
            <Search size={15} />
            检索当前项目 <kbd>⌘ K</kbd>
          </button>
          <div className="mode-switch" role="group" aria-label="界面模式">
            <button className={mode === "basic" ? "active" : ""} onClick={() => setMode("basic")}>基础</button>
            <button className={mode === "advanced" ? "active" : ""} onClick={() => setMode("advanced")}>高级</button>
          </div>
          <span className="owner-chip">
            <Fingerprint size={14} />
            本地 Owner
          </span>
        </div>
        <header className={`page-header ${page === "home" ? "compact" : ""}`}>
          <div>
            <p>{copy.eyebrow}</p>
            <h1>{copy.title}</h1>
            <span>{copy.description}</span>
          </div>
          <div>
            {page !== "help" && <button className="page-help" onClick={() => openHelp()}><HelpCircle size={14}/>查看本页用法</button>}
            <small>{currentProject?.name || "正在读取项目"}</small>
            <Status value="owner active" />
          </div>
        </header>
        <div className="page-content">
          {page === "home" && (
            <HomePage
              api={api}
              project={currentProject}
              summary={currentSummary}
              onNavigate={navigate}
              onCreateSpace={() => setNewSpaceOpen(true)}
            />
          )}
          {page === "import" && (
            <ImportCenter
              api={api}
              project={currentProject}
              onNavigate={navigate}
            />
          )}
          {page === "projects" && (
            <ProjectWorkspace
              api={api}
              projectID={projectID}
              projects={registry.data?.projects || []}
              select={setProjectID}
              onNavigate={navigate}
              mode={mode}
            />
          )}
          {page === "search" && <SearchPage api={api} projectID={projectID} />}
          {page === "memory" && (
            <MemoryPage api={api} projectID={projectID} openRun={openRun} mode={mode} />
          )}
          {page === "profiles" && (
            <ProfileCenter api={api} projectID={projectID} />
          )}
          {page === "team" && (
            <DeferredPage><TeamMemoryCenter api={api} projectID={projectID} mode={mode} onOpenReview={() => navigate("review")} /></DeferredPage>
          )}
          {page === "experience" && (
            <DeferredPage><ExperienceBank api={api} projectID={projectID} /></DeferredPage>
          )}
          {page === "adaptation" && (
            <DeferredPage><AdaptationLab api={api} projectID={projectID} /></DeferredPage>
          )}
          {page === "portable" && (
            <DeferredPage><PortableBundleCenter api={api} projectID={projectID} /></DeferredPage>
          )}
          {page === "products" && (
            <KnowledgeProducts api={api} projectID={projectID} />
          )}
          {page === "assets" && (
            <AssetFoundry api={api} projectID={projectID} />
          )}
          {page === "timeline" && (
            <TemporalTimeline api={api} projectID={projectID} />
          )}
          {page === "runs" && (
            <RunsPage
              api={api}
              projectID={projectID}
              requestedRun={requestedRun}
            />
          )}
          {page === "review" && <ReviewPage api={api} openRun={openRun} />}
          {page === "strategy" && (
            <DeferredPage><StrategyWorkbench
              api={api}
              projectID={projectID}
              projects={registry.data?.projects || []}
            /></DeferredPage>
          )}
          {page === "studio" && (
            <DeferredPage><PipelineStudio api={api} projectID={projectID} openRun={openRun} /></DeferredPage>
          )}
          {page === "plugins" && (
            <DeferredPage><PluginCenter
              api={api}
              projectID={projectID}
              projects={registry.data?.projects || []}
            /></DeferredPage>
          )}
          {page === "connections" && (
            <ConnectionsPage
              api={api}
              projects={registry.data?.projects || []}
              mode={mode}
            />
          )}
          {page === "health" && <HealthPage api={api} onOpenManual={() => openHelp("start")} />}
          {page === "help" && <HelpCenter />}
        </div>
      </main>
      {newSpaceOpen && <div className="modal-backdrop" onMouseDown={() => setNewSpaceOpen(false)}>
        <form className="modal" onSubmit={createSpace} onMouseDown={(event) => event.stopPropagation()}>
          <p className="micro">NEW MEMORY SPACE</p>
          <h2>新建记忆空间</h2>
          <p className="modal-intro">每个空间都有独立的原材料、记忆、待办和 Agent 权限。</p>
          <label>名称<input name="name" required maxLength={120} autoFocus placeholder="例如：新产品发布" /></label>
          <label>说明（可选）<textarea name="description" rows={4} placeholder="这个空间主要记录什么？" /></label>
          {mode === "advanced" && <>
            <label>短标识（可留空自动生成）<input name="slug" pattern="[a-z0-9][a-z0-9-]*" placeholder="new-product" /></label>
            <div className="form-grid"><label>币种<input name="currency" defaultValue="CNY" maxLength={3}/></label><label>可选预算<input name="budget" type="number" min="0" step="0.01" defaultValue="0"/></label></div>
          </>}
          {newSpaceError && <p className="form-error">{newSpaceError}</p>}
          <div className="modal-actions"><button type="button" className="button" onClick={() => setNewSpaceOpen(false)}>取消</button><button className="button primary" disabled={creatingSpace}>{creatingSpace ? "创建中…" : "创建并开始导入"}</button></div>
        </form>
      </div>}
    </div>
  );
}

export default function App() {
  const [connection, setConnection] = useState<ConnectionState>();
  const [error, setError] = useState("");
  const initialise = useCallback(() => {
    setError("");
    connect()
      .then(setConnection)
      .catch((reason) =>
        setError(reason instanceof Error ? reason.message : String(reason)),
      );
  }, []);
  useEffect(initialise, [initialise]);
  if (error)
    return (
      <main className="locked-screen">
        <XCircle size={42} />
        <h1>桌面会话启动失败</h1>
        <p>{error}</p>
        <button className="button primary" onClick={initialise}>
          重新尝试
        </button>
      </main>
    );
  if (!connection)
    return (
      <main className="boot-screen">
        <BrainCircuit size={31} />
        <span />
        <p>正在建立本地 Owner 会话…</p>
      </main>
    );
  if (connection.mode === "locked")
    return <Locked connection={connection} retry={initialise} />;
  return <AppShell session={connection.session} />;
}
