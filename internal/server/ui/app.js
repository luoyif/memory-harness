const $ = (selector) => document.querySelector(selector);
const content = $('#content');
const title = $('#title');
const eyebrow = $('#eyebrow');
const pageIntro = $('#page-intro');
const updated = $('#updated');
const coreDot = $('#core-dot');
const coreLabel = $('#core-label');
const coreVersion = $('#core-version');
const reviewCount = $('#review-count');
const navButtons = [...document.querySelectorAll('[data-page]')];
const projectSelect = $('#project-select');
const drawer = $('#detail-drawer');
const drawerContent = $('#drawer-content');
const drawerScrim = $('#drawer-scrim');
const projectDialog = $('#project-dialog');
const recordDialog = $('#record-dialog');
const importDialog = $('#import-dialog');
const conversationDialog = $('#conversation-dialog');
const searchDialog = $('#search-dialog');
const providerDialog = $('#provider-dialog');
const agentDialog = $('#agent-dialog');
const tokenDialog = $('#token-dialog');
const toastBox = $('#toast');

const pageMeta = {
  portfolio: ['全局经营', 'PORTFOLIO MEMORY', '在项目、记忆和资金之间建立一条可追溯的经营线。'],
  project: ['项目工作台', 'PROJECT OPERATING SPACE', '把目标、时间事实、决策、风险、资金和项目记忆放在同一张工作台上。'],
  search: ['统一检索', 'HYBRID RECALL', '在 Evidence、复盘、记忆、时间事实、决策和经营记录中检索，并保留来源。'],
  timeline: ['时间记忆', 'BI-TEMPORAL MEMORY', '查看现在有效的事实，也可以回到某个历史时点检查当时知道什么。'],
  overview: ['记忆生长', 'MEMORY GROWTH MAP', '从不可变证据到可验证能力，查看六层记忆如何形成。'],
  episodes: ['会话复盘', 'EPISODIC COMPILER', '每次对话如何被压缩成可追溯的目标、行动与结果。'],
  memory: ['长期记忆', 'GOVERNED MEMORY', '按情景、语义、程序与身份层查看跨会话沉淀。'],
  assets: ['能力资产', 'PROTECTED EVOLUTION', 'Procedure、Skill 与 Rule 候选不会在未复核时静默生效。'],
  review: ['待你复核', 'HUMAN REVIEW GATE', '身份、程序和纠正操作必须经过你的确认。'],
  sources: ['导入与连接', 'PROVENANCE ADAPTERS', '所有导入先成为 Evidence；原始导出留在本地，可重新解析。'],
  control: ['AI 控制中心', 'GOVERNED AGENT ACCESS', '配置记忆模型，并为 Codex、Claude Code、DeepSeek 或其他 Agent 分配最小权限。'],
  health: ['系统健康', 'INTEGRITY & RECOVERY', '检查 Ledger、统一索引、项目范围、任务和派生记忆的一致性。'],
};

let currentPage = 'portfolio';
let projectSummaries = [];
let selectedProjectID = '';
let toastTimer;
let renderSequence = 0;
let timelineHistory = false;
let timelineAsOf = '';
let searchAllProjects = false;
let lastSearchQuery = '';
let conversationPayload = null;
let activeProjectData = null;
let editingProviderID = '';
let editingAgentID = '';
let controlSnapshot = null;

function node(tag, className = '', text = '') {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== '') el.textContent = String(text);
  return el;
}

function clear() { content.replaceChildren(); }

function showToast(message, tone = 'good') {
  window.clearTimeout(toastTimer);
  toastBox.textContent = message;
  toastBox.className = `toast show ${tone}`;
  toastTimer = window.setTimeout(() => { toastBox.className = 'toast'; }, 3600);
}

function sectionHeading(name, detail = '', action = null) {
  const row = node('div', 'section-heading');
  const copy = node('div');
  copy.append(node('h3', '', name));
  if (detail) copy.append(node('p', '', detail));
  row.append(copy);
  if (action) row.append(action);
  return row;
}

function badge(value, tone = '') {
  const normalized = String(value || 'unknown').toLowerCase();
  const good = ['ok', 'pass', 'healthy', 'active', 'applied', 'completed', 'corroborated', 'canonical', 'compiled', 'posted'].includes(normalized);
  const warn = ['review_required', 'candidate', 'conflict', 'protected', 'degraded', 'failed', 'open'].includes(normalized);
  return node('span', `badge ${tone || (good ? 'good' : warn ? 'warn' : 'quiet')}`, normalized.replaceAll('_', ' '));
}

function metricCard(name, value, detail, tone = '') {
  const card = node('article', `metric-card ${tone}`.trim());
  card.append(node('p', 'metric-label', name), node('strong', 'metric-value', value), node('p', 'metric-detail', detail));
  return card;
}

function emptyState(name, detail, action = null) {
  const box = node('div', 'empty-state');
  box.append(node('span', 'empty-glyph', '○'), node('strong', '', name), node('p', '', detail));
  if (action) box.append(action);
  return box;
}

function formatTime(value, dateOnly = false) {
  if (!value) return '暂无记录';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', dateOnly ? { year: 'numeric', month: 'short', day: 'numeric' } : { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date);
}

function currencyDigits(currency) { return currency === 'JPY' ? 0 : 2; }
function minorFromMajor(value, currency) { return Math.round(Number(value || 0) * (10 ** currencyDigits(currency))); }
function money(minor, currency = 'CNY') {
  return new Intl.NumberFormat('zh-CN', { style: 'currency', currency, maximumFractionDigits: currencyDigits(currency) }).format(Number(minor || 0) / (10 ** currencyDigits(currency)));
}
function percent(value) { return `${Math.round(Number(value || 0) * 100)}%`; }

function statusButton(label, endpoint, id, status) {
  const button = node('button', 'text-button', label); button.type = 'button';
  button.addEventListener('click', async () => {
    button.disabled = true;
    try { await requestJSON(`${endpoint}/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ status }) }); showToast(`已更新为“${label}”。`); await loadProjectRegistry(selectedProjectID); await render('project'); }
    catch (error) { showToast(error.message, 'bad'); button.disabled = false; }
  });
  return button;
}

async function requestJSON(path, options = {}) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 60000);
  try {
    const response = await fetch(path, {
      ...options,
      headers: { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...(options.headers || {}) },
      signal: controller.signal,
    });
    const payload = await response.json();
    if (!response.ok) throw new Error(payload?.error?.message || `请求失败 (${response.status})`);
    return payload;
  } finally { window.clearTimeout(timer); }
}

function projectRecord() { return projectSummaries.find((item) => item.project.project_id === selectedProjectID); }
function selectedProject() { return projectRecord()?.project || null; }

function populateProjectSelect(target, includePortfolio = false) {
  target.replaceChildren();
  if (includePortfolio) {
    const all = node('option', '', '全部项目'); all.value = ''; target.append(all);
  }
  projectSummaries.forEach((summary) => {
    const option = node('option', '', summary.project.name);
    option.value = summary.project.project_id;
    target.append(option);
  });
  target.value = selectedProjectID;
}

async function loadProjectRegistry(preferred = '') {
  const data = await requestJSON('/v1/projects');
  projectSummaries = data.projects || [];
  const stored = preferred || localStorage.getItem('memoryos.project');
  const exists = projectSummaries.some((item) => item.project.project_id === stored);
  const firstReal = projectSummaries.find((item) => !['project-inbox', 'project-personal'].includes(item.project.project_id));
  selectedProjectID = exists ? stored : (firstReal?.project.project_id || projectSummaries.find((item) => item.project.project_id === 'project-personal')?.project.project_id || projectSummaries[0]?.project.project_id || '');
  localStorage.setItem('memoryos.project', selectedProjectID);
  populateProjectSelect(projectSelect);
  populateProjectSelect($('#import-project'));
  populateProjectSelect($('#conversation-project'));
}

async function refreshChrome() {
  try {
    const [health, layers] = await Promise.all([requestJSON('/health'), requestJSON('/v1/layers')]);
    coreDot.classList.add('online');
    coreLabel.textContent = 'Memory Core 在线';
    coreVersion.textContent = `${health.version} · ${layers.compiler}`;
    reviewCount.textContent = layers.needs_review || '';
    reviewCount.classList.toggle('visible', layers.needs_review > 0);
  } catch (_) {
    coreDot.classList.remove('online'); coreLabel.textContent = 'Memory Core 不可用'; coreVersion.textContent = '检查守护进程';
  }
}

function renderPortfolio(data, layers) {
  const projects = data.projects || [];
  const aggregate = projects.reduce((out, item) => {
    out.evidence += item.metrics.evidence; out.goals += item.metrics.open_goals; out.risks += item.metrics.open_risks; out.review += item.metrics.pending_review;
    return out;
  }, { evidence: 0, goals: 0, risks: 0, review: 0 });
  const hero = node('div', 'portfolio-hero');
  const copy = node('div'); copy.append(node('p', 'kicker', 'YOUR OPERATING MEMORY'), node('h3', '', '项目在推进，记忆也在复利。'), node('p', '', '这里不把项目做成孤立文件夹：每个项目有自己的事实、目标与账本，同时保留明确的跨项目观察。'));
  const pulse = node('div', 'portfolio-pulse'); pulse.append(node('span', '', 'ACTIVE SPACES'), node('strong', '', projects.filter((item) => item.project.status === 'active').length), node('small', '', `${aggregate.evidence} 份证据进入经营上下文`));
  hero.append(copy, pulse); content.append(hero);

  const metrics = node('div', 'metric-grid');
  metrics.append(metricCard('项目空间', projects.length, '含收件箱与个人空间', 'ink'), metricCard('推进中目标', aggregate.goals, '所有项目当前目标'), metricCard('开放风险', aggregate.risks, '按项目隔离管理', aggregate.risks ? 'amber' : 'sage'), metricCard('等待复核', aggregate.review, '受保护记忆操作', aggregate.review ? 'amber' : 'sage'));
  content.append(metrics, sectionHeading('项目组合', '点击项目进入独立工作台'));
  const grid = node('div', 'project-grid');
  projects.forEach((summary) => {
    const project = summary.project;
    const card = node('button', 'project-card'); card.type = 'button'; card.style.setProperty('--project-color', project.color);
    const top = node('div', 'project-card-top'); top.append(badge(project.status), node('span', '', project.slug));
    card.append(top, node('h3', '', project.name), node('p', '', project.description || '尚未填写项目说明。'));
    const stats = node('div', 'project-card-stats'); stats.append(node('span', '', `${summary.metrics.memories} 记忆`), node('span', '', `${summary.metrics.open_goals} 目标`), node('span', '', `${summary.metrics.open_risks} 风险`)); card.append(stats);
    const finance = summary.finance.currencies?.find((item) => item.currency === project.default_currency);
    card.append(node('div', 'project-budget', finance ? `预算余量 ${money(finance.remaining_minor, finance.currency)}` : `预算 ${money(project.budget_minor, project.default_currency)}`));
    card.addEventListener('click', () => { selectedProjectID = project.project_id; projectSelect.value = selectedProjectID; localStorage.setItem('memoryos.project', selectedProjectID); render('project', true); });
    grid.append(card);
  });
  content.append(grid);

  const currencyTotals = new Map();
  projects.forEach((item) => (item.finance.currencies || []).forEach((line) => {
    const current = currencyTotals.get(line.currency) || { income: 0, expense: 0, net: 0 };
    current.income += line.income_minor; current.expense += line.expense_minor; current.net += line.net_minor; currencyTotals.set(line.currency, current);
  }));
  content.append(sectionHeading('跨项目资金观察', '币种分别统计，不做隐式汇率换算'));
  const financeGrid = node('div', 'finance-strip');
  if (!currencyTotals.size) financeGrid.append(emptyState('还没有资金记录', '在项目工作台新增收入或支出后，会按币种出现在这里。'));
  currencyTotals.forEach((value, currency) => {
    const card = node('article', 'currency-card'); card.append(node('span', '', currency), node('strong', '', money(value.net, currency)), node('small', '', `收入 ${money(value.income, currency)} · 支出 ${money(value.expense, currency)}`)); financeGrid.append(card);
  });
  content.append(financeGrid);

  const layerRail = node('div', 'layer-rail');
  (layers.layers || []).forEach((layer) => { const item = node('div'); item.append(node('b', '', String(layer.ordinal).padStart(2, '0')), node('span', '', layer.chinese_name), node('strong', '', layer.count)); layerRail.append(item); });
  content.append(sectionHeading('六层记忆系统', '全局真实对象计数'), layerRail);
}

function renderProject(data, financeEntries, connectors) {
  activeProjectData = data;
  const { project, metrics, finance } = data.summary;
  const hero = node('div', 'project-hero'); hero.style.setProperty('--project-color', project.color);
  const copy = node('div'); copy.append(node('p', 'kicker', project.slug.toUpperCase()), node('h3', '', project.name), node('p', '', project.description || '这个项目还没有说明。'));
  const budget = finance.currencies?.find((item) => item.currency === project.default_currency);
  const budgetBox = node('div', 'budget-box'); budgetBox.append(node('span', '', 'PROJECT BUDGET'), node('strong', '', money(budget?.remaining_minor ?? project.budget_minor, project.default_currency)), node('small', '', `已支出 ${money(budget?.expense_minor || 0, project.default_currency)} / ${money(project.budget_minor, project.default_currency)}`));
  hero.append(copy, budgetBox); content.append(hero);

  const grid = node('div', 'metric-grid'); grid.append(metricCard('Evidence', metrics.evidence, `${metrics.episodes} 次会话复盘`, 'ink'), metricCard('长期记忆', metrics.memories, `${metrics.facts} 条当前事实`), metricCard('推进中目标', metrics.open_goals, `${data.milestones.length} 个里程碑`, 'sage'), metricCard('开放风险', metrics.open_risks, `${metrics.pending_review} 项记忆待复核`, metrics.open_risks ? 'amber' : 'sage')); content.append(grid);

  content.append(sectionHeading('核心上下文', '像 Letta Memory Blocks 一样有明确额度，但每一块都带来源'));
  const blockGrid = node('div', 'context-grid');
  (data.context_blocks || []).forEach((block) => { const card = node('article', 'context-card'); card.append(node('span', '', block.label), node('p', '', block.content), node('small', '', `${block.content.length} / ${block.budget_chars} 字 · ${block.source_refs.length} 个来源`)); blockGrid.append(card); });
  if (!data.context_blocks?.length) blockGrid.append(emptyState('还没有核心上下文块', '可新增“项目现状”“边界条件”或“长期方向”，并控制它占用的上下文额度。'));
  content.append(blockGrid);

  const columns = node('div', 'project-columns');
  const goals = node('section', 'project-panel'); goals.append(sectionHeading('目标与里程碑', `${data.goals.length} 个目标`));
  if (!data.goals.length) goals.append(emptyState('尚未设定项目目标', '新增目标后，系统会把它与项目记忆分开治理。'));
  data.goals.forEach((goal) => { const row = node('article', 'work-row'); const body = node('div'); body.append(node('h4', '', goal.title), node('p', '', goal.description || '无补充说明'), node('small', '', goal.target_at ? `目标日期 ${formatTime(goal.target_at, true)}` : `优先级 ${goal.priority}`)); const action = goal.status === 'active' ? statusButton('完成', '/v1/goals', goal.goal_id, 'completed') : goal.status === 'paused' ? statusButton('继续', '/v1/goals', goal.goal_id, 'active') : null; row.append(badge(goal.status), body); if (action) row.append(action); goals.append(row); });
  const milestones = node('div', 'milestone-list'); data.milestones.forEach((item) => { const row = node('div'); row.append(node('i', item.status === 'completed' ? 'done' : ''), node('span', '', item.title), item.status === 'completed' ? node('small', '', formatTime(item.completed_at, true)) : statusButton('完成', '/v1/milestones', item.milestone_id, 'completed')); milestones.append(row); }); goals.append(milestones);

  const riskPanel = node('section', 'project-panel'); riskPanel.append(sectionHeading('风险雷达', `${data.risks.length} 项`));
  if (!data.risks.length) riskPanel.append(emptyState('暂未登记项目风险', '风险记录不等于悲观，它让影响与应对策略可见。'));
  data.risks.forEach((risk) => { const row = node('article', 'risk-row'); const score = node('strong', `risk-score level-${Math.ceil(risk.score / 5)}`, risk.score); const body = node('div'); body.append(node('h4', '', risk.title), node('p', '', risk.mitigation || risk.description || '尚未填写应对策略'), node('small', '', `概率 ${risk.probability}/5 · 影响 ${risk.impact}/5`)); const action = ['open', 'monitoring'].includes(risk.status) ? statusButton('已缓解', '/v1/risks', risk.risk_id, 'mitigated') : badge(risk.status); row.append(score, body, action); riskPanel.append(row); });
  columns.append(goals, riskPanel); content.append(columns);

  content.append(sectionHeading('决策日志', '旧决策可以被新决策明确取代，但不会从历史中消失'));
  const decisions = node('div', 'decision-timeline');
  if (!data.decisions.length) decisions.append(emptyState('还没有正式决策', '把关键取舍记录成决策，而不是让它埋在聊天里。'));
  data.decisions.forEach((decision) => { const row = node('article'); row.append(node('time', '', formatTime(decision.decided_at, true))); const body = node('div'); body.append(node('h4', '', decision.title), node('p', '', decision.decision), node('small', '', decision.rationale || `${decision.source_evidence_ids.length} 个来源`)); row.append(body, badge(decision.status)); decisions.append(row); }); content.append(decisions);

  content.append(sectionHeading('项目资金', `金额按最小货币单位保存；${data.finance_accounts?.length || 0} 个独立账户，不同币种不混算`));
  const financeGrid = node('div', 'finance-summary-grid');
  (finance.currencies || []).forEach((line) => { financeGrid.append(metricCard(`${line.currency} 净额`, money(line.net_minor, line.currency), `收入 ${money(line.income_minor, line.currency)} · 支出 ${money(line.expense_minor, line.currency)}`, line.remaining_minor < 0 ? 'amber' : 'sage')); });
  content.append(financeGrid);
  const ledger = node('div', 'ledger-list');
  (financeEntries.entries || []).slice(0, 20).forEach((entry) => { const row = node('div', 'ledger-row'); row.append(node('time', '', formatTime(entry.occurred_at, true)), node('span', '', entry.description), badge(entry.status === 'void' ? 'void' : entry.category || entry.entry_type, 'quiet'), node('strong', entry.amount_minor < 0 ? 'negative' : 'positive', money(entry.amount_minor, entry.currency))); if (entry.status !== 'void') row.append(statusButton('作废', '/v1/finance/entries', entry.entry_id, 'void')); ledger.append(row); });
  if (!financeEntries.entries?.length) ledger.append(emptyState('还没有项目收支', '新增一笔计划或已入账的收入、支出，项目资金视图会自动更新。'));
  content.append(ledger);

  const connectorLine = node('div', 'connector-line');
  (connectors.connectors || []).forEach((item) => { const chip = node('span'); chip.append(badge(item.status), document.createTextNode(`${item.name} · ${item.kind}`)); connectorLine.append(chip); });
  content.append(sectionHeading('项目连接器', `${connectors.total || 0} 个`), connectorLine);
}

function searchForm() {
  const form = node('form', 'search-form');
  const input = node('input'); input.type = 'search'; input.placeholder = '检索事实、决策、会话、长期记忆或资金说明…'; input.value = lastSearchQuery; input.autocomplete = 'off'; input.id = 'search-query';
  const scope = node('label', 'scope-toggle'); const checkbox = node('input'); checkbox.type = 'checkbox'; checkbox.checked = searchAllProjects; scope.append(checkbox, node('span', '', '明确检索全部项目'));
  const submit = node('button', 'primary-button', '开始检索'); submit.type = 'submit';
  form.append(input, scope, submit);
  form.addEventListener('submit', (event) => { event.preventDefault(); lastSearchQuery = input.value.trim(); searchAllProjects = checkbox.checked; runSearch(lastSearchQuery); });
  return form;
}

function renderSearchLanding() {
  const hero = node('div', 'search-hero'); hero.append(node('p', 'kicker', 'ONE QUERY · EVERY LAYER'), node('h3', '', '找到答案，也找到它为什么可信。'), node('p', '', '默认只检索当前项目。勾选“全部项目”才会执行跨项目召回，结果始终显示层级、项目、有效时间和来源。'), searchForm()); content.append(hero);
  const kinds = node('div', 'search-kind-grid'); [['evidence', '原始证据'], ['episode', '会话复盘'], ['memory', '长期记忆'], ['fact', '时间事实'], ['decision', '项目决策'], ['goal', '目标'], ['risk', '风险'], ['finance', '资金']].forEach(([kind, label]) => { const item = node('div'); item.append(node('span', '', kind), node('strong', '', label)); kinds.append(item); }); content.append(kinds);
}

async function runSearch(query) {
  if (!query) { showToast('请输入检索内容。', 'bad'); return; }
  clear(); content.append(node('div', 'loading', '正在检索所有可用层…'));
  try {
    const result = await requestJSON('/v1/search/unified', { method: 'POST', body: JSON.stringify({ query, project_id: selectedProjectID, all_projects: searchAllProjects, limit: 30 }) });
    clear(); content.append(searchForm());
    const scopeName = searchAllProjects ? '全部项目' : (selectedProject()?.name || '当前项目');
    content.append(sectionHeading(`“${query}” 的结果`, `${result.hits.length} 条 · ${scopeName} · ${result.backend}`));
    if (!result.hits.length) { content.append(emptyState('没有找到匹配记录', '可以换一个表达，或明确启用全部项目检索。')); return; }
    const list = node('div', 'search-results');
    result.hits.forEach((hit) => {
      const card = node('article', 'search-result-card');
      const head = node('div'); head.append(badge(hit.kind), badge(hit.status), node('span', '', projectSummaries.find((p) => p.project.project_id === hit.project_id)?.project.name || hit.project_id), node('time', '', formatTime(hit.observed_at)));
      card.append(head, node('h3', '', hit.title), node('p', '', hit.snippet));
      const foot = node('div', 'result-foot'); foot.append(node('code', '', hit.source_id), node('small', '', `相关度 ${hit.lexical_rank} · 时间序 ${hit.recency_rank}`));
      const useful = node('button', 'text-button', '有帮助'); useful.type = 'button'; useful.addEventListener('click', () => sendFeedback(result.context_id, hit.result_id, 'helpful'));
      const irrelevant = node('button', 'text-button muted-action', '不相关'); irrelevant.type = 'button'; irrelevant.addEventListener('click', () => sendFeedback(result.context_id, hit.result_id, 'irrelevant'));
      foot.append(useful, irrelevant); card.append(foot); list.append(card);
    }); content.append(list); updated.textContent = `${result.took_ms} ms · ${result.candidate_count} 个候选`;
  } catch (error) { clear(); content.append(searchForm(), emptyState('检索失败', error.message)); }
}

async function sendFeedback(contextID, resultID, rating) {
  try { await requestJSON('/v1/recall/feedback', { method: 'POST', body: JSON.stringify({ project_id: selectedProjectID, context_id: contextID, result_id: resultID, rating }) }); showToast('反馈已记录，会用于调整后续召回。'); }
  catch (error) { showToast(error.message, 'bad'); }
}

function renderTimeline(data) {
  const controls = node('div', 'timeline-controls');
  const date = node('input'); date.type = 'datetime-local'; if (timelineAsOf) date.value = timelineAsOf.slice(0, 16);
  const history = node('label', 'scope-toggle'); const checkbox = node('input'); checkbox.type = 'checkbox'; checkbox.checked = timelineHistory; history.append(checkbox, node('span', '', '显示全部历史与已取代事实'));
  const apply = node('button', 'ghost-button', '查看这个时点'); apply.type = 'button'; apply.addEventListener('click', () => { timelineAsOf = date.value ? new Date(date.value).toISOString() : ''; timelineHistory = checkbox.checked; render('timeline'); });
  const now = node('button', 'text-button', '回到现在'); now.type = 'button'; now.addEventListener('click', () => { timelineAsOf = ''; timelineHistory = false; render('timeline'); });
  controls.append(node('span', '', '有效时间'), date, history, apply, now); content.append(controls);
  content.append(sectionHeading(timelineAsOf ? `截至 ${formatTime(timelineAsOf)}` : '当前有效事实', `${data.total} 条 · 系统记录时间与现实有效时间分开保存`));
  if (!data.facts.length) { content.append(emptyState('这个时点没有事实', '新增项目状态、关系或约束后，可以沿时间回看它们何时成立、何时被取代。')); return; }
  const groups = new Map(); data.facts.forEach((fact) => { const key = fact.subject; if (!groups.has(key)) groups.set(key, []); groups.get(key).push(fact); });
  const timeline = node('div', 'fact-groups');
  groups.forEach((facts, subject) => { const group = node('section', 'fact-group'); group.append(node('h3', '', subject)); facts.forEach((fact) => { const row = node('article', 'fact-row'); const line = node('div'); line.append(node('strong', '', fact.predicate), node('span', '', fact.object)); const time = node('small', '', `${formatTime(fact.valid_from, true)} → ${fact.valid_until ? formatTime(fact.valid_until, true) : '现在'} · 记录于 ${formatTime(fact.recorded_at)}`); row.append(line, time, badge(fact.status)); group.append(row); }); timeline.append(group); }); content.append(timeline);
}

function renderOverview(data, dashboard, living) {
  const hero = node('div', 'growth-hero'); const copy = node('div'); copy.append(node('p', 'kicker', 'THE VERIFIABLE PATH'), node('h3', '', '一条记忆，必须知道自己从哪里来。'), node('p', '', `当前由 ${data.compiler} 编译、${data.policy} 治理。每一层都可以回到原始 Evidence。`)); const seal = node('div', 'hero-seal'); seal.append(node('strong', '', data.layers.reduce((sum, layer) => sum + layer.count, 0)), node('span', '', '可检查对象')); hero.append(copy, seal); content.append(hero);
  const flow = node('ol', 'layer-flow'); data.layers.forEach((layer, index) => { const item = node('li', `layer-card layer-${layer.id}`); item.style.setProperty('--delay', `${index * 55}ms`); const head = node('div', 'layer-head'); head.append(node('span', 'layer-number', String(layer.ordinal).padStart(2, '0')), badge(layer.status)); item.append(head, node('p', 'layer-en', layer.name), node('h4', '', layer.chinese_name), node('strong', 'layer-count', layer.count), node('p', 'layer-description', layer.description)); flow.append(item); }); content.append(flow);
  const grid = node('div', 'metric-grid'); grid.append(metricCard('原始 Evidence', dashboard.totals.evidence, `${dashboard.totals.sessions} 个会话 · ${dashboard.totals.sources} 个来源`, 'ink'), metricCard('今日捕获', dashboard.today.evidence, `${dashboard.today.sessions} 个会话进入 Ledger`), metricCard('已完成整理', data.completed_jobs, `${data.layers[2].count} 个 Episode`, 'sage'), metricCard('需要复核', data.needs_review, '身份、程序或纠正候选', data.needs_review ? 'amber' : 'sage')); content.append(sectionHeading('今日生长', '真实计数，不包含演示数据'), grid);
  content.append(sectionHeading('活知识投影', '索引与上下文均可由 Memory Records 重建')); const views = node('div', 'living-grid'); (living.views || []).forEach((view) => { const card = node('article', 'living-card'); card.append(node('span', 'living-mark', view.view_type === 'hot' ? '↗' : view.view_type === 'context' ? '◎' : '≡')); const body = node('div'); body.append(node('h4', '', view.title), node('p', '', view.summary), node('code', '', view.canonical_path)); card.append(body); views.append(card); }); if (!living.views?.length) views.append(emptyState('尚未生成活知识', '导入或捕获 Evidence 后，系统会生成 Memory Index、Hot Index 和 Active Context。')); content.append(views);
}

function renderEpisodes(data) {
  content.append(sectionHeading('Episode 时间线', `${data.total} 次可追溯复盘`));
  if (!data.episodes.length) { const button = node('button', 'ghost-button', '导入第一份内容'); button.addEventListener('click', () => importDialog.showModal()); content.append(emptyState('还没有 Episode', 'Evidence 保存后会按会话形成一次复盘。', button)); return; }
  const timeline = node('div', 'episode-timeline'); data.episodes.forEach((episode, index) => { const card = node('article', 'episode-card'); const marker = node('div', 'timeline-marker', String(data.total - index).padStart(2, '0')); const body = node('div', 'episode-body'); const meta = node('div', 'episode-meta'); meta.append(badge(episode.status), node('span', '', episode.source_system), node('time', '', formatTime(episode.ended_at))); body.append(meta, node('h3', '', episode.title), node('p', '', episode.summary)); const stats = node('div', 'episode-stats'); stats.append(node('span', '', `${episode.evidence_ids.length} Evidence`), node('span', '', `${episode.units} 知识点`), node('span', '', `revision ${episode.revision}`)); const inspect = node('button', 'text-button', '查看抽取结果 →'); inspect.type = 'button'; inspect.addEventListener('click', () => openEpisode(episode.episode_id)); body.append(stats, inspect); card.append(marker, body); timeline.append(card); }); content.append(timeline);
}

const tierLabels = { episodic: '情景', semantic: '语义', procedural: '程序', identity_core: '身份核心', working: '工作' };
function renderMemories(data, activeTier = '') {
  const filters = node('div', 'filter-row'); [['', '全部'], ['episodic', '情景'], ['semantic', '语义'], ['procedural', '程序'], ['identity_core', '身份核心']].forEach(([value, label]) => { const button = node('button', value === activeTier ? 'active' : '', label); button.type = 'button'; button.addEventListener('click', () => loadMemoryTier(value)); filters.append(button); }); content.append(filters);
  if (!data.memories.length) { content.append(emptyState('这一层还没有记忆', '系统不会用占位数据伪造记忆。导入真实内容并整理后会出现在这里。')); return; }
  const grid = node('div', 'memory-grid'); data.memories.forEach((memory) => { const card = node('article', `memory-card tier-${memory.tier}`); const top = node('div', 'memory-top'); top.append(node('span', 'tier-label', tierLabels[memory.tier] || memory.tier), badge(memory.status)); card.append(top, node('h3', '', memory.summary), node('p', '', memory.body)); const provenance = node('div', 'provenance-row'); provenance.append(node('span', '', `${memory.source_evidence_ids.length} 份证据`), node('span', '', `${memory.source_episode_ids.length} 次复盘`), node('span', '', `强度 ${memory.strength}`)); const inspect = node('button', 'trace-button', '追溯来源'); inspect.type = 'button'; inspect.addEventListener('click', () => openTrace(memory.memory_id)); card.append(provenance, inspect); grid.append(card); }); content.append(grid);
}

function renderAssets(data) {
  const policy = node('div', 'policy-banner'); policy.append(node('span', 'policy-icon', '⛉')); const copy = node('div'); copy.append(node('strong', '', '受保护的演化边界'), node('p', '', '程序记忆可以提出能力资产候选，但验证、批准和激活是不同步骤；当前不会自动改写任何 Agent 配置。')); policy.append(copy, badge(data.activation || 'manual_review_only', 'warn')); content.append(policy, sectionHeading('Agent Asset Registry', `${data.total} 个真实候选或已批准资产`));
  if (!data.assets.length) { content.append(emptyState('还没有能力资产候选', '当系统识别到可复用流程时，会先生成 Procedure 候选并进入复核。')); return; }
  const list = node('div', 'asset-list'); data.assets.forEach((asset) => { const card = node('article', 'asset-card'); const version = node('div', 'asset-version'); version.append(node('small', '', 'VERSION'), node('strong', '', asset.version)); const body = node('div'); body.append(node('p', 'asset-type', asset.asset_type), node('h3', '', asset.title), node('p', '', asset.summary)); const state = node('div', 'asset-state'); state.append(badge(asset.status), node('small', '', `验证：${asset.validation_status}`)); card.append(version, body, state); list.append(card); }); content.append(list);
}

function renderReview(data) {
  const summary = node('div', 'review-summary'); summary.append(node('strong', '', data.total), node('div', '', '项等待你的判断'), node('p', '', '批准会让候选记忆进入可用状态；拒绝会保留审计记录并停止提升。')); content.append(summary);
  if (!data.operations.length) { content.append(emptyState('复核箱已经清空', '低风险语义记忆自动沉淀；受保护层仍会在这里等待确认。')); return; }
  const list = node('div', 'review-list'); data.operations.forEach((operation) => { const card = node('article', 'review-card'); const copy = node('div'); const top = node('div', 'review-top'); top.append(badge(operation.type, 'quiet'), badge(`risk ${operation.risk_tier}`, 'warn')); copy.append(top, node('h3', '', operation.type === 'CORRECT' ? '检测到显式纠正' : '受保护记忆候选'), node('p', '', operation.reason_codes.join(' · ')), node('code', '', operation.target_memory_id || operation.operation_id)); const actions = node('div', 'review-actions'); const reject = node('button', 'ghost-button', '拒绝'); reject.type = 'button'; reject.addEventListener('click', () => decide(operation.operation_id, 'reject')); const approve = node('button', 'primary-button', '批准'); approve.type = 'button'; approve.addEventListener('click', () => decide(operation.operation_id, 'approve')); actions.append(reject, approve); card.append(copy, actions); list.append(card); }); content.append(list);
}

function renderSources(sourceData, connectorData) {
  const actions = node('div', 'inline-actions'); const textImport = node('button', 'ghost-button', '导入文本 / Markdown'); textImport.type = 'button'; textImport.addEventListener('click', () => { $('#import-project').value = selectedProjectID; importDialog.showModal(); }); const historyImport = node('button', 'primary-button', '导入 AI 历史会话'); historyImport.type = 'button'; historyImport.addEventListener('click', () => { $('#conversation-project').value = selectedProjectID; conversationDialog.showModal(); }); actions.append(textImport, historyImport);
  content.append(sectionHeading('来源适配器', '解析能力与 Memory 语义分离；原始导出始终保留', actions));
  const adapters = node('div', 'adapter-grid'); [['Text / Markdown', 'ready', '网页导入 · SHA-256 幂等 · 自动沉淀'], ['ChatGPT', 'ready', '导出 JSON · mapping DAG · active branch'], ['Claude', 'ready', '导出 JSON · content blocks · 隐藏思考不进入沉淀'], ['DeepSeek', 'ready', '导出 JSON · REQUEST / RESPONSE / SEARCH'], ['Evidence API', 'ready', '单条或批量写入 · 项目自动路由'], ['MCP Agents', 'governed', 'Codex / Claude Code / OpenClaw · 令牌与项目权限'], ['LLM Wiki', 'adapter boundary', '继续复用外部多格式解析，不直接写其受管 Wiki']].forEach(([name, state, detail]) => { const card = node('article', 'adapter-card'); card.append(badge(state), node('h3', '', name), node('p', '', detail)); adapters.append(card); }); content.append(adapters);
  content.append(sectionHeading('当前项目连接器', `${connectorData.total || 0} 个`)); const connectorGrid = node('div', 'connector-grid'); (connectorData.connectors || []).forEach((item) => { const card = node('article'); card.append(badge(item.status), node('h4', '', item.name), node('p', '', item.kind), node('small', '', item.last_sync_at ? `最近同步 ${formatTime(item.last_sync_at)}` : '等待首次同步')); connectorGrid.append(card); }); if (!connectorData.connectors?.length) connectorGrid.append(emptyState('还没有连接器登记', '直接导入仍然可用；连接器用于持续同步和游标管理。')); content.append(connectorGrid);
  content.append(sectionHeading('已捕获来源', `${sourceData.total} 个含真实 Evidence 的来源`)); if (!sourceData.sources.length) { content.append(emptyState('尚未写入来源数据', '导入内容后，来源、会话数和最近捕获时间会自动出现。')); return; }
  const table = node('div', 'data-table'); const header = node('div', 'table-row table-header'); ['来源', 'Evidence', '会话', '最近捕获'].forEach((item) => header.append(node('span', '', item))); table.append(header); sourceData.sources.forEach((source) => { const row = node('div', 'table-row'); row.append(node('strong', '', source.name), node('span', '', source.evidence), node('span', '', source.sessions), node('span', '', formatTime(source.last_captured_at))); table.append(row); }); content.append(table);
}

const permissionLabels = {
  'memory.read': '读取记忆', 'memory.capture': '写入 Evidence', 'project.read': '读取项目', 'project.write': '写目标 / 决策 / 风险', 'finance.read': '读取资金', 'finance.write': '写入资金',
};

function projectName(projectID) { return projectSummaries.find((item) => item.project.project_id === projectID)?.project.name || projectID; }

function openProviderEditor(provider = null, preset = null) {
  editingProviderID = provider?.provider_id || '';
  $('#provider-dialog-title').textContent = provider ? '编辑记忆模型' : '配置记忆模型';
  $('#provider-kind').value = provider?.kind || preset?.kind || 'openai';
  $('#provider-name').value = provider?.name || preset?.name || '';
  $('#provider-base-url').value = provider?.base_url || preset?.base_url || '';
  $('#provider-model').value = provider?.model || preset?.example_model || '';
  $('#provider-api-key').value = '';
  $('#provider-enabled').checked = provider ? provider.enabled : true;
  $('#provider-clear-key').checked = false;
  providerDialog.showModal();
}

function fillAgentOptions(agent = null, allowedPermissions = Object.keys(permissionLabels)) {
  const permissionTarget = $('#agent-permissions'); permissionTarget.replaceChildren();
  allowedPermissions.forEach((permission) => { const label = node('label'); const input = node('input'); input.type = 'checkbox'; input.name = 'agent-permission'; input.value = permission; input.checked = agent ? agent.permissions.includes(permission) : ['memory.read', 'memory.capture', 'project.read'].includes(permission); label.append(input, document.createTextNode(permissionLabels[permission] || permission)); permissionTarget.append(label); });
  const projectTarget = $('#agent-project-grants'); projectTarget.replaceChildren();
  projectSummaries.forEach((summary) => { const label = node('label'); const input = node('input'); input.type = 'checkbox'; input.name = 'agent-project'; input.value = summary.project.project_id; input.checked = agent ? agent.project_ids.includes(summary.project.project_id) : summary.project.project_id === selectedProjectID; label.append(input, document.createTextNode(summary.project.name)); projectTarget.append(label); });
  const allProjects = $('#agent-all-projects'); allProjects.checked = Boolean(agent?.all_projects); projectTarget.classList.toggle('disabled', allProjects.checked); projectTarget.querySelectorAll('input').forEach((input) => { input.disabled = allProjects.checked; });
}

function openAgentEditor(agent = null) {
  editingAgentID = agent?.agent_id || '';
  $('#agent-dialog-title').textContent = agent ? '调整 Agent 权限' : '接入一个 AI Agent';
  $('#agent-name').value = agent?.name || '';
  $('#agent-kind').value = agent?.kind || 'codex';
  $('#agent-name').disabled = Boolean(agent);
  $('#agent-kind').disabled = Boolean(agent);
  fillAgentOptions(agent, controlSnapshot?.agents?.allowed_permissions);
  agentDialog.showModal();
}

function showAgentToken(token) {
  $('#agent-token-value').textContent = token;
  tokenDialog.showModal();
}

async function setModelRuntime(mode, providerID = '') {
  try { await requestJSON('/v1/model/runtime', { method: 'PUT', body: JSON.stringify({ mode, active_provider_id: providerID, fallback_to_rules: true }) }); showToast(mode === 'agent' ? 'Memory Agent 已启用；规则回退保持开启。' : '已切换为完全本地的规则模式。'); await render('control'); }
  catch (error) { showToast(error.message, 'bad'); }
}

async function testProvider(providerID, button) {
  const old = button.textContent; button.disabled = true; button.textContent = '检测中…';
  try { const result = await requestJSON(`/v1/model/providers/${encodeURIComponent(providerID)}/test`, { method: 'POST' }); showToast(result.selected_model_found ? '模型服务连通，已找到所选模型。' : '服务连通，但模型列表中没有所选名称。', result.selected_model_found ? 'good' : 'bad'); await render('control'); }
  catch (error) { showToast(error.message, 'bad'); button.disabled = false; button.textContent = old; }
}

async function changeAgentStatus(agent, status) {
  try { await requestJSON(`/v1/agents/${encodeURIComponent(agent.agent_id)}`, { method: 'PATCH', body: JSON.stringify({ status, permissions: agent.permissions, all_projects: agent.all_projects, project_ids: agent.project_ids }) }); showToast(status === 'active' ? 'Agent 已启用。' : 'Agent 已停用，令牌立即失效。'); await render('control'); }
  catch (error) { showToast(error.message, 'bad'); }
}

async function rotateAgentToken(agent) {
  if (!window.confirm(`轮换“${agent.name}”的令牌？旧令牌会立即失效。`)) return;
  try { const result = await requestJSON(`/v1/agents/${encodeURIComponent(agent.agent_id)}/rotate-token`, { method: 'POST' }); showAgentToken(result.token); await render('control'); }
  catch (error) { showToast(error.message, 'bad'); }
}

function renderControl(modelData, agentData, auditData, capabilities) {
  controlSnapshot = { model: modelData, agents: agentData, audit: auditData, capabilities };
  const runtime = modelData.runtime;
  const activeProvider = modelData.providers.find((item) => item.provider_id === runtime.active_provider_id);
  const hero = node('div', 'control-hero'); const heroCopy = node('div'); heroCopy.append(node('p', 'kicker', 'LOCAL OWNER · EXPLICIT GRANTS'), node('h3', '', runtime.mode === 'agent' ? '模型负责理解，规则负责守门。' : '当前记忆沉淀完全在本地运行。'), node('p', '', 'Agent 的模型调用、项目范围和每一次读写都可见、可撤销、可审计。身份与程序记忆仍须由你复核。'));
  const modeSeal = node('div', 'control-mode-seal'); modeSeal.append(node('span', '', 'DISTILLATION'), node('strong', '', runtime.mode === 'agent' ? 'AGENT' : 'RULES'), node('small', '', activeProvider ? activeProvider.model : '无外发数据'));
  hero.append(heroCopy, modeSeal); content.append(hero);

  const safety = node('div', 'safety-grid'); [['数据外发', runtime.mode === 'agent' ? `仅发送新会话给 ${activeProvider?.name || '所选模型'}` : '关闭'], ['故障回退', '始终启用 rules-v1'], ['密钥保存', modelData.secret_store], ['保护边界', 'Agent 无权批准高风险记忆']].forEach(([name, value]) => { const item = node('div'); item.append(node('span', '', name), node('strong', '', value)); safety.append(item); }); content.append(safety);

  const providerAction = node('button', 'primary-button', '＋ 添加模型'); providerAction.type = 'button'; providerAction.addEventListener('click', () => openProviderEditor(null, modelData.presets[0])); content.append(sectionHeading('模型配置中心', '默认规则模式不联网；启用 Agent 模式后仅发送待沉淀的新会话', providerAction));
  const runtimeBar = node('div', 'runtime-bar'); const rulesButton = node('button', runtime.mode === 'rules' ? 'active' : '', '本地规则'); rulesButton.type = 'button'; rulesButton.addEventListener('click', () => setModelRuntime('rules')); runtimeBar.append(rulesButton); modelData.providers.filter((item) => item.enabled).forEach((provider) => { const button = node('button', runtime.active_provider_id === provider.provider_id ? 'active' : '', provider.name); button.type = 'button'; button.addEventListener('click', () => setModelRuntime('agent', provider.provider_id)); runtimeBar.append(button); }); content.append(runtimeBar);
  const providerGrid = node('div', 'provider-grid'); modelData.providers.forEach((provider) => { const card = node('article', 'provider-card'); const top = node('div'); top.append(badge(provider.enabled ? provider.status : 'disabled'), node('span', '', provider.kind)); card.append(top, node('h4', '', provider.name), node('code', '', provider.model), node('p', '', provider.base_url)); const meta = node('div', 'provider-meta'); meta.append(node('span', '', provider.has_secret ? '密钥已保存' : '无密钥'), node('span', '', provider.last_test_at ? `检测 ${formatTime(provider.last_test_at)}` : '尚未检测')); const actions = node('div', 'card-actions'); const test = node('button', 'ghost-button', '检测连接'); test.type = 'button'; test.addEventListener('click', () => testProvider(provider.provider_id, test)); const edit = node('button', 'text-button', '编辑'); edit.type = 'button'; edit.addEventListener('click', () => openProviderEditor(provider)); actions.append(test, edit); card.append(meta, actions); providerGrid.append(card); });
  if (!modelData.providers.length) { const choices = node('div', 'preset-row'); modelData.presets.forEach((preset) => { const button = node('button', 'preset-card'); button.type = 'button'; button.append(node('strong', '', preset.name), node('small', '', preset.example_model)); button.addEventListener('click', () => openProviderEditor(null, preset)); choices.append(button); }); providerGrid.append(emptyState('尚未配置大模型', '选择一个预设开始；保持规则模式也可以完整运行。'), choices); }
  content.append(providerGrid);

  const agentAction = node('button', 'primary-button', '＋ 接入 Agent'); agentAction.type = 'button'; agentAction.addEventListener('click', () => openAgentEditor()); content.append(sectionHeading('Agent 权限矩阵', `${agentData.total} 个独立身份 · 令牌仅显示一次`, agentAction));
  if (!agentData.agents.length) content.append(emptyState('还没有外部 Agent', '创建后再把一次性令牌放入对应 MCP 客户端的秘密环境变量。'));
  const agents = node('div', 'agent-list'); agentData.agents.forEach((agent) => { const row = node('article', `agent-card ${agent.status}`); const identity = node('div', 'agent-identity'); identity.append(node('span', 'agent-monogram', agent.name.slice(0, 1).toUpperCase()), node('div')); identity.lastChild.append(node('h4', '', agent.name), node('p', '', `${agent.kind} · ${agent.agent_id}`)); const grants = node('div', 'agent-grants'); agent.permissions.forEach((permission) => grants.append(node('span', '', permissionLabels[permission] || permission))); const projects = node('div', 'agent-projects', agent.all_projects ? '全部项目（含未来项目）' : agent.project_ids.map(projectName).join(' · ')); const status = node('div', 'agent-status'); status.append(badge(agent.status), node('small', '', agent.last_used_at ? `最近使用 ${formatTime(agent.last_used_at)}` : '尚未使用')); const actions = node('div', 'card-actions'); const edit = node('button', 'text-button', '权限'); edit.type = 'button'; edit.addEventListener('click', () => openAgentEditor(agent)); const rotate = node('button', 'text-button', '轮换令牌'); rotate.type = 'button'; rotate.addEventListener('click', () => rotateAgentToken(agent)); const toggle = node('button', 'ghost-button', agent.status === 'active' ? '停用' : '启用'); toggle.type = 'button'; toggle.addEventListener('click', () => changeAgentStatus(agent, agent.status === 'active' ? 'disabled' : 'active')); actions.append(edit, rotate, toggle); row.append(identity, grants, projects, status, actions); agents.append(row); }); content.append(agents);

  content.append(sectionHeading('MCP 能力清单', '读取、写入和经营操作均使用相同 Agent 令牌与项目授权'));
  const command = node('div', 'mcp-command'); command.append(node('span', '', '启动命令'), node('code', '', capabilities.command), node('small', '', `令牌环境变量：${capabilities.token_env}`)); content.append(command);
  const capabilityGrid = node('div', 'capability-grid'); capabilities.tools.forEach((tool) => { const card = node('article'); card.append(badge(tool.mode, tool.mode === 'write' ? 'warn' : 'good'), node('code', '', tool.name), node('h4', '', tool.summary), node('p', '', `${tool.permission} · ${tool.scope}`)); capabilityGrid.append(card); }); content.append(capabilityGrid);

  content.append(sectionHeading('最近访问审计', `${auditData.total} 条受控 Agent 事件`)); const auditList = node('div', 'audit-list'); (auditData.events || []).slice(0, 12).forEach((event) => { const row = node('div', 'audit-row'); row.append(node('time', '', formatTime(event.created_at)), node('code', '', event.agent_id), node('strong', '', event.action), node('span', '', event.project_id ? projectName(event.project_id) : event.resource_type), badge(event.status)); auditList.append(row); }); if (!auditData.events?.length) auditList.append(emptyState('暂无访问事件', 'Agent 第一次读取或写入后会在这里留下可追溯记录。')); content.append(auditList);
}

function renderHealth(data) {
  const grid = node('div', 'metric-grid'); grid.append(metricCard('Core', data.status, data.version, data.status === 'ok' ? 'sage' : 'amber'), metricCard('Canonical Ledger', data.doctor.ledger_records, '不可变 Evidence'), metricCard('统一检索', data.doctor.unified_documents, '跨层可重建文档', 'sage'), metricCard('Uptime', `${data.uptime_seconds}s`, '本次运行时间')); content.append(grid, sectionHeading('完整性检查', '所有检查均读取真实文件和数据库'));
  const checks = node('div', 'check-list'); [['Doctor', data.doctor.status], ['项目 / 时间事实', `${data.doctor.projects} / ${data.doctor.temporal_facts}`], ['Evidence / Search turns', `${data.doctor.ledger_records} / ${data.doctor.search_turns}`], ['Episodes / Units', `${data.doctor.episodes} / ${data.doctor.knowledge_units}`], ['Memory / Operations', `${data.doctor.memory_records} / ${data.doctor.memory_operations}`], ['统一 FTS 双索引', `${data.doctor.unified_unicode_rows} / ${data.doctor.unified_trigram_rows}`], ['目标 / 风险 / 资金', `${data.doctor.goals} / ${data.doctor.risks} / ${data.doctor.finance_entries}`], ['Agent / 审计 / 模型', `${data.doctor.agent_principals} / ${data.doctor.agent_audit_events} / ${data.doctor.model_providers}`], ['Data root', data.home]].forEach(([name, value]) => { const row = node('div', 'check-row'); row.append(node('span', '', name), name === 'Doctor' ? badge(value) : node('code', '', value)); checks.append(row); }); content.append(checks);
  const jobGrid = node('div', 'metric-grid small'); jobGrid.append(metricCard('Pending', data.jobs.pending, '等待处理'), metricCard('Running', data.jobs.running, '正在处理'), metricCard('Completed', data.jobs.completed, '已完成'), metricCard('Failed', data.jobs.failed, '需要处理', data.jobs.failed ? 'amber' : '')); content.append(sectionHeading('可恢复任务'), jobGrid);
}

async function loadMemoryTier(tier) { clear(); content.append(node('div', 'loading', '正在切换记忆层…')); const data = await requestJSON(`/v1/memories?limit=200${tier ? `&tier=${encodeURIComponent(tier)}` : ''}`); clear(); renderMemories(data, tier); }

function openDrawer() { drawerContent.replaceChildren(); drawer.classList.add('open'); drawerScrim.classList.add('open'); drawer.setAttribute('aria-hidden', 'false'); }
function closeDrawer() { drawer.classList.remove('open'); drawerScrim.classList.remove('open'); drawer.setAttribute('aria-hidden', 'true'); }

async function openEpisode(id) {
  try { const data = await requestJSON(`/v1/episodes/${encodeURIComponent(id)}`); openDrawer(); drawerContent.append(node('p', 'eyebrow', 'EPISODE TRACE'), node('h2', '', data.episode.title), node('p', 'drawer-lead', data.episode.summary)); const facts = node('div', 'drawer-facts'); [['来源', data.episode.source_system], ['Evidence', data.episode.evidence_ids.length], ['知识点', data.knowledge_units.length], ['编译器', data.episode.compiler]].forEach(([name, value]) => { const item = node('div'); item.append(node('small', '', name), node('strong', '', value)); facts.append(item); }); drawerContent.append(facts, sectionHeading('原子知识点', '每一点都保留 Evidence ID')); data.knowledge_units.forEach((unit) => { const card = node('article', 'trace-unit'); const top = node('div'); top.append(badge(unit.unit_type), badge(unit.status)); card.append(top, node('p', '', unit.statement), node('code', '', unit.evidence_id)); drawerContent.append(card); }); } catch (error) { showToast(error.message, 'bad'); }
}

async function openTrace(id) {
  try { const data = await requestJSON(`/v1/memories/${encodeURIComponent(id)}/trace`); openDrawer(); drawerContent.append(node('p', 'eyebrow', 'FULL PROVENANCE'), node('h2', '', data.memory.summary), badge(data.memory.status)); const strength = node('div', 'trace-strength'); strength.append(node('span', '', '记忆强度'), node('strong', '', data.memory.strength), node('small', '', `置信度 ${percent(data.memory.confidence)}`)); drawerContent.append(strength); [['Memory Operations', data.operations, (item) => `${item.type} · ${item.status}`], ['Knowledge Units', data.knowledge_units, (item) => item.statement], ['Episodes', data.episodes, (item) => item.title], ['Canonical Evidence', data.evidence, (item) => item.preview || item.evidence_id]].forEach(([name, items, formatter]) => { drawerContent.append(sectionHeading(name, `${items.length} 项`)); items.forEach((item) => { const card = node('article', 'trace-step'); card.append(node('span', 'trace-dot'), node('p', '', formatter(item))); const idValue = item.operation_id || item.unit_id || item.episode_id || item.evidence_id; if (idValue) card.append(node('code', '', idValue)); drawerContent.append(card); }); }); } catch (error) { showToast(error.message, 'bad'); }
}

async function decide(id, decision) { try { await requestJSON(`/v1/operations/${encodeURIComponent(id)}/review`, { method: 'POST', body: JSON.stringify({ decision, reviewer: 'local-user' }) }); showToast(decision === 'approve' ? '已批准，记忆状态已经更新。' : '已拒绝，审计记录已保留。'); await render('review'); } catch (error) { showToast(error.message, 'bad'); } }

async function processAll() {
  const button = $('#process-all'); const old = button.textContent; button.disabled = true; button.textContent = '正在整理…';
  try { const data = await requestJSON('/v1/process', { method: 'POST', body: '{}' }); showToast(`整理完成：${data.total} 个会话。`); await loadProjectRegistry(selectedProjectID); await render(currentPage); }
  catch (error) { showToast(error.message, 'bad'); } finally { button.disabled = false; button.textContent = old; }
}

async function render(page, pushHash = false) {
  const sequence = ++renderSequence; currentPage = pageMeta[page] ? page : 'portfolio'; const [pageTitle, pageEyebrow, intro] = pageMeta[currentPage]; title.textContent = pageTitle; eyebrow.textContent = pageEyebrow; pageIntro.textContent = intro;
  navButtons.forEach((button) => { const active = button.dataset.page === currentPage; button.classList.toggle('active', active); active ? button.setAttribute('aria-current', 'page') : button.removeAttribute('aria-current'); });
  if (pushHash && window.location.hash !== `#${currentPage}`) window.location.hash = currentPage;
  clear(); content.append(node('div', 'loading', '正在读取真实数据…'));
  try {
    if (currentPage === 'portfolio') {
      const [projects, layers] = await Promise.all([requestJSON('/v1/projects'), requestJSON('/v1/layers')]); if (sequence !== renderSequence) return; projectSummaries = projects.projects || []; clear(); renderPortfolio(projects, layers); updated.textContent = `${projects.total} 个项目空间`;
    } else if (currentPage === 'project') {
      const [data, entries, connectors] = await Promise.all([requestJSON(`/v1/projects/${encodeURIComponent(selectedProjectID)}`), requestJSON(`/v1/finance/entries?project_id=${encodeURIComponent(selectedProjectID)}&limit=100`), requestJSON(`/v1/connectors?project_id=${encodeURIComponent(selectedProjectID)}`)]); if (sequence !== renderSequence) return; clear(); renderProject(data, entries, connectors); updated.textContent = data.summary.project.status;
    } else if (currentPage === 'search') {
      if (sequence !== renderSequence) return; clear(); if (lastSearchQuery) await runSearch(lastSearchQuery); else renderSearchLanding(); updated.textContent = searchAllProjects ? '全部项目' : selectedProject()?.name || '';
    } else if (currentPage === 'timeline') {
      const query = new URLSearchParams({ project_id: selectedProjectID, limit: '500' }); if (timelineAsOf) query.set('as_of', timelineAsOf); if (timelineHistory) query.set('include_history', '1'); const data = await requestJSON(`/v1/facts?${query}`); if (sequence !== renderSequence) return; clear(); renderTimeline(data); updated.textContent = selectedProject()?.name || '';
    } else if (currentPage === 'overview') {
      const [layers, dashboard, living] = await Promise.all([requestJSON('/v1/layers'), requestJSON('/v1/dashboard'), requestJSON('/v1/living')]); if (sequence !== renderSequence) return; clear(); renderOverview(layers, dashboard, living); updated.textContent = `更新于 ${formatTime(layers.generated_at)}`;
    } else if (currentPage === 'episodes') {
      const data = await requestJSON('/v1/episodes?limit=100'); if (sequence !== renderSequence) return; clear(); renderEpisodes(data); updated.textContent = `${data.total} 次复盘`;
    } else if (currentPage === 'memory') {
      const data = await requestJSON('/v1/memories?limit=200'); if (sequence !== renderSequence) return; clear(); renderMemories(data); updated.textContent = `${data.total} 条记忆`;
    } else if (currentPage === 'assets') {
      const data = await requestJSON('/v1/assets'); if (sequence !== renderSequence) return; clear(); renderAssets(data); updated.textContent = '受保护注册表';
    } else if (currentPage === 'review') {
      const data = await requestJSON('/v1/operations?status=review_required&limit=100'); if (sequence !== renderSequence) return; clear(); renderReview(data); updated.textContent = `${data.total} 项待处理`;
    } else if (currentPage === 'sources') {
      const [sources, connectors] = await Promise.all([requestJSON('/v1/sources'), requestJSON(`/v1/connectors?project_id=${encodeURIComponent(selectedProjectID)}`)]); if (sequence !== renderSequence) return; clear(); renderSources(sources, connectors); updated.textContent = selectedProject()?.name || '';
    } else if (currentPage === 'control') {
      const [models, agents, audit, capabilities] = await Promise.all([requestJSON('/v1/model/config'), requestJSON('/v1/agents'), requestJSON('/v1/agent-audit'), requestJSON('/v1/integrations/capabilities')]); if (sequence !== renderSequence) return; clear(); renderControl(models, agents, audit, capabilities); updated.textContent = models.runtime.mode === 'agent' ? 'Memory Agent 已启用' : '本地规则模式';
    } else {
      const data = await requestJSON('/v1/health/detail'); if (sequence !== renderSequence) return; clear(); renderHealth(data); updated.textContent = `启动于 ${formatTime(data.started_at)}`;
    }
  } catch (error) { if (sequence !== renderSequence) return; clear(); content.append(emptyState('无法加载这个页面', error.name === 'AbortError' ? '请求超时，请检查 Memory Core。' : error.message)); }
  refreshChrome();
}

function field(label, id, type = 'text', value = '') { const wrap = node('label'); wrap.append(document.createTextNode(label)); const input = node('input'); input.id = id; input.type = type; input.value = value; if (type === 'number') input.step = 'any'; wrap.append(input); return wrap; }
function area(label, id, rows = 4) { const wrap = node('label'); wrap.append(document.createTextNode(label)); const input = node('textarea'); input.id = id; input.rows = rows; wrap.append(input); return wrap; }
function selectField(label, id, options) { const wrap = node('label'); wrap.append(document.createTextNode(label)); const select = node('select'); select.id = id; options.forEach(([value, name]) => { const option = node('option', '', name); option.value = value; select.append(option); }); wrap.append(select); return wrap; }

function renderRecordFields() {
  const target = $('#record-fields'); const kind = $('#record-kind').value; target.replaceChildren(); const grid = node('div', 'form-grid');
  if (kind === 'goal') { grid.append(field('目标名称', 'record-title'), field('目标日期', 'record-date', 'date'), field('优先级（0-9）', 'record-priority', 'number', '5')); target.append(grid, area('目标说明', 'record-description'));
  } else if (kind === 'milestone') { const goals = [['', '不绑定目标'], ...((activeProjectData?.goals || []).map((item) => [item.goal_id, item.title]))]; grid.append(field('里程碑名称', 'record-title'), field('计划日期', 'record-date', 'date'), selectField('所属目标', 'record-goal', goals)); target.append(grid);
  } else if (kind === 'decision') { grid.append(field('决策标题', 'record-title'), field('决定日期', 'record-date', 'date', new Date().toISOString().slice(0, 10))); target.append(grid, area('最终决定', 'record-body'), area('原因与取舍', 'record-rationale', 3));
  } else if (kind === 'risk') { grid.append(field('风险名称', 'record-title'), field('发生概率（1-5）', 'record-probability', 'number', '3'), field('影响程度（1-5）', 'record-impact', 'number', '3')); target.append(grid, area('风险说明', 'record-description'), area('应对策略', 'record-mitigation', 3));
  } else if (kind === 'fact') { grid.append(field('主体', 'record-subject'), field('关系 / 属性', 'record-predicate'), field('开始生效日期', 'record-date', 'date', new Date().toISOString().slice(0, 10))); target.append(grid, area('事实内容', 'record-object', 3));
  } else if (kind === 'finance') { const currency = selectedProject()?.default_currency || 'CNY'; const currencies = [...new Set([currency, 'CNY', 'USD', 'HKD', 'EUR', 'JPY'])].map((item) => [item, item]); const accounts = [['', '不指定账户'], ...((activeProjectData?.finance_accounts || []).map((item) => [item.account_id, `${item.name} · ${item.currency}`]))]; grid.append(selectField('类型', 'record-finance-type', [['expense', '支出'], ['income', '收入'], ['adjustment', '调整']]), field('金额', 'record-amount', 'number'), selectField('币种', 'record-currency', currencies), selectField('资金账户', 'record-account', accounts), field('发生日期', 'record-date', 'date', new Date().toISOString().slice(0, 10))); target.append(grid, field('分类', 'record-category'), area('说明', 'record-description', 3));
  } else if (kind === 'account') { const currency = selectedProject()?.default_currency || 'CNY'; grid.append(field('账户名称', 'record-title'), selectField('账户类型', 'record-account-type', [['cash', '现金'], ['bank', '银行'], ['receivable', '应收'], ['payable', '应付'], ['virtual', '虚拟账户']]), selectField('币种', 'record-currency', [[currency, currency], ...(['CNY', 'USD', 'HKD', 'EUR', 'JPY'].filter((item) => item !== currency).map((item) => [item, item]))]), field('期初余额', 'record-amount', 'number', '0')); target.append(grid);
  } else if (kind === 'connector') { grid.append(field('连接器名称', 'record-title'), selectField('来源类型', 'record-connector-kind', [['chatgpt', 'ChatGPT'], ['claude', 'Claude'], ['deepseek', 'DeepSeek'], ['files', '本地文件'], ['browser', '浏览器'], ['api', 'Evidence API']])); target.append(grid);
  } else { grid.append(field('区块名称', 'record-label'), field('字符额度', 'record-budget', 'number', '4000')); target.append(grid, area('核心上下文内容', 'record-body', 7)); }
}

async function submitRecord() {
  const kind = $('#record-kind').value; let endpoint; let payload = { project_id: selectedProjectID };
  if (kind === 'goal') { endpoint = '/v1/goals'; payload = { ...payload, title: $('#record-title').value.trim(), description: $('#record-description').value.trim(), priority: Number($('#record-priority').value || 0), target_at: $('#record-date').value ? new Date(`${$('#record-date').value}T00:00:00`).toISOString() : '' };
  } else if (kind === 'milestone') { endpoint = '/v1/milestones'; payload = { ...payload, goal_id: $('#record-goal').value, title: $('#record-title').value.trim(), due_at: $('#record-date').value ? new Date(`${$('#record-date').value}T00:00:00`).toISOString() : '' };
  } else if (kind === 'decision') { endpoint = '/v1/decisions'; payload = { ...payload, title: $('#record-title').value.trim(), decision: $('#record-body').value.trim(), rationale: $('#record-rationale').value.trim(), decided_at: new Date(`${$('#record-date').value || new Date().toISOString().slice(0, 10)}T00:00:00`).toISOString() };
  } else if (kind === 'risk') { endpoint = '/v1/risks'; payload = { ...payload, title: $('#record-title').value.trim(), description: $('#record-description').value.trim(), probability: Number($('#record-probability').value), impact: Number($('#record-impact').value), mitigation: $('#record-mitigation').value.trim() };
  } else if (kind === 'fact') { endpoint = '/v1/facts'; payload = { ...payload, subject: $('#record-subject').value.trim(), predicate: $('#record-predicate').value.trim(), object: $('#record-object').value.trim(), valid_from: new Date(`${$('#record-date').value || new Date().toISOString().slice(0, 10)}T00:00:00`).toISOString(), confidence: 1 };
  } else if (kind === 'finance') { endpoint = '/v1/finance/entries'; const entryType = $('#record-finance-type').value; const currency = $('#record-currency').value; let amount = Math.abs(minorFromMajor($('#record-amount').value, currency)); if (entryType === 'expense') amount = -amount; payload = { ...payload, account_id: $('#record-account').value, entry_type: entryType, category: $('#record-category').value.trim(), description: $('#record-description').value.trim(), amount_minor: amount, currency, occurred_at: new Date(`${$('#record-date').value || new Date().toISOString().slice(0, 10)}T00:00:00`).toISOString(), status: 'posted', idempotency_key: `ui-${Date.now()}` };
  } else if (kind === 'account') { endpoint = '/v1/finance/accounts'; const currency = $('#record-currency').value; payload = { ...payload, name: $('#record-title').value.trim(), account_type: $('#record-account-type').value, currency, opening_balance_minor: minorFromMajor($('#record-amount').value, currency) };
  } else if (kind === 'connector') { endpoint = '/v1/connectors'; payload = { ...payload, name: $('#record-title').value.trim(), kind: $('#record-connector-kind').value };
  } else { endpoint = '/v1/context-blocks'; payload = { ...payload, label: $('#record-label').value.trim(), content: $('#record-body').value.trim(), budget_chars: Number($('#record-budget').value || 4000), source_refs: [] }; }
  await requestJSON(endpoint, { method: 'POST', body: JSON.stringify(payload) }); recordDialog.close(); await loadProjectRegistry(selectedProjectID); showToast('项目记录已经保存。'); await render(kind === 'fact' ? 'timeline' : 'project', true);
}

async function previewConversation() {
  const file = $('#conversation-file').files[0]; if (!file) { showToast('请先选择 JSON 文件。', 'bad'); return; } if (file.size > 60 * 1024 * 1024) { showToast('文件超过 60 MiB，请先拆分导出。', 'bad'); return; }
  try { conversationPayload = JSON.parse(await file.text()); const data = await requestJSON('/v1/import/conversations', { method: 'POST', body: JSON.stringify({ format: $('#conversation-format').value, project_id: $('#conversation-project').value, dry_run: true, payload: conversationPayload }) }); $('#conversation-preview').textContent = `${data.preview.conversations} 个会话 · ${data.preview.messages} 条消息 · ${data.preview.date_from ? `${formatTime(data.preview.date_from, true)} 至 ${formatTime(data.preview.date_to, true)}` : '无时间信息'} · ${data.preview.warnings} 个解析提示`; $('#submit-conversation').disabled = false; showToast('预检完成，没有写入数据。'); }
  catch (error) { conversationPayload = null; $('#submit-conversation').disabled = true; $('#conversation-preview').textContent = error.message; showToast(error.message, 'bad'); }
}

navButtons.forEach((button) => button.addEventListener('click', () => render(button.dataset.page, true)));
projectSelect.addEventListener('change', () => { selectedProjectID = projectSelect.value; localStorage.setItem('memoryos.project', selectedProjectID); $('#import-project').value = selectedProjectID; $('#conversation-project').value = selectedProjectID; render(currentPage === 'portfolio' ? 'project' : currentPage, currentPage === 'portfolio'); });
$('#refresh').addEventListener('click', async () => { await loadProjectRegistry(selectedProjectID); render(currentPage); });
$('#process-all').addEventListener('click', processAll);
$('#new-project').addEventListener('click', () => projectDialog.showModal());
$('#quick-add').addEventListener('click', () => { if (currentPage === 'control') { openAgentEditor(); return; } renderRecordFields(); recordDialog.showModal(); });
$('#record-kind').addEventListener('change', renderRecordFields);
$('#drawer-close').addEventListener('click', closeDrawer); drawerScrim.addEventListener('click', closeDrawer);
document.querySelectorAll('[data-close-dialog]').forEach((button) => button.addEventListener('click', () => button.closest('dialog').close()));
window.addEventListener('hashchange', () => render(window.location.hash.slice(1) || 'portfolio'));
window.addEventListener('keydown', (event) => { if (event.key === 'Escape') closeDrawer(); if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); searchDialog.showModal(); $('#command-query').focus(); } });
$('#search-jump').addEventListener('click', () => { searchDialog.showModal(); $('#command-query').focus(); });
$('#search-command-form').addEventListener('submit', (event) => { event.preventDefault(); lastSearchQuery = $('#command-query').value.trim(); searchDialog.close(); render('search', true); });

$('#project-form').addEventListener('submit', async (event) => { event.preventDefault(); try { const currency = $('#project-currency').value; const project = await requestJSON('/v1/projects', { method: 'POST', body: JSON.stringify({ name: $('#project-name').value.trim(), slug: $('#project-slug').value.trim().toLowerCase(), description: $('#project-description').value.trim(), default_currency: currency, budget_minor: minorFromMajor($('#project-budget').value, currency), aliases: $('#project-aliases').value.split(',').map((item) => item.trim()).filter(Boolean) }) }); projectDialog.close(); event.target.reset(); await loadProjectRegistry(project.project_id); showToast('新项目空间已经建立。'); render('project', true); } catch (error) { showToast(error.message, 'bad'); } });
$('#record-form').addEventListener('submit', async (event) => { event.preventDefault(); try { await submitRecord(); } catch (error) { showToast(error.message, 'bad'); } });

$('#import-form').addEventListener('submit', async (event) => { event.preventDefault(); const submit = $('#submit-import'); submit.disabled = true; submit.textContent = '正在保存…'; const payload = { source_system: $('#import-source').value.trim(), session_id: $('#import-session').value.trim(), project_id: $('#import-project').value, documents: [{ title: $('#import-title').value.trim(), text: $('#import-text').value }] }; try { const result = await requestJSON('/v1/import/text', { method: 'POST', body: JSON.stringify(payload) }); importDialog.close(); $('#import-text').value = ''; $('#import-title').value = ''; await loadProjectRegistry(selectedProjectID); showToast(`导入完成：${result.pipeline?.knowledge_units || 0} 个知识点。`); await render('project', true); } catch (error) { showToast(error.message, 'bad'); } finally { submit.disabled = false; submit.textContent = '保存并沉淀'; } });

$('#preview-conversation').addEventListener('click', previewConversation);
$('#conversation-file').addEventListener('change', () => { conversationPayload = null; $('#submit-conversation').disabled = true; $('#conversation-preview').textContent = '文件已选择，请先预检。'; });
$('#conversation-form').addEventListener('submit', async (event) => { event.preventDefault(); if (!conversationPayload) return; const submit = $('#submit-conversation'); submit.disabled = true; submit.textContent = '正在导入…'; try { const data = await requestJSON('/v1/import/conversations', { method: 'POST', body: JSON.stringify({ format: $('#conversation-format').value, project_id: $('#conversation-project').value, payload: conversationPayload }) }); conversationDialog.close(); conversationPayload = null; event.target.reset(); await loadProjectRegistry(selectedProjectID); showToast(`导入完成：${data.preview.conversations} 个会话，${data.preview.messages} 条消息。`); render('project', true); } catch (error) { showToast(error.message, 'bad'); } finally { submit.disabled = false; submit.textContent = '确认导入'; } });

$('#provider-form').addEventListener('submit', async (event) => { event.preventDefault(); const payload = { name: $('#provider-name').value.trim(), kind: $('#provider-kind').value, base_url: $('#provider-base-url').value.trim(), model: $('#provider-model').value.trim(), api_key: $('#provider-api-key').value.trim(), clear_api_key: $('#provider-clear-key').checked, enabled: $('#provider-enabled').checked }; const path = editingProviderID ? `/v1/model/providers/${encodeURIComponent(editingProviderID)}` : '/v1/model/providers'; try { await requestJSON(path, { method: editingProviderID ? 'PATCH' : 'POST', body: JSON.stringify(payload) }); providerDialog.close(); showToast('模型配置已安全保存。'); await render('control'); } catch (error) { showToast(error.message, 'bad'); } });

$('#agent-all-projects').addEventListener('change', () => { const disabled = $('#agent-all-projects').checked; $('#agent-project-grants').classList.toggle('disabled', disabled); document.querySelectorAll('[name="agent-project"]').forEach((input) => { input.disabled = disabled; }); });
$('#agent-form').addEventListener('submit', async (event) => { event.preventDefault(); const permissions = [...document.querySelectorAll('[name="agent-permission"]:checked')].map((input) => input.value); const allProjects = $('#agent-all-projects').checked; const projectIDs = allProjects ? [] : [...document.querySelectorAll('[name="agent-project"]:checked')].map((input) => input.value); const existing = controlSnapshot?.agents?.agents.find((item) => item.agent_id === editingAgentID); const payload = editingAgentID ? { status: existing?.status || 'active', permissions, all_projects: allProjects, project_ids: projectIDs } : { name: $('#agent-name').value.trim(), kind: $('#agent-kind').value, permissions, all_projects: allProjects, project_ids: projectIDs }; try { const result = await requestJSON(editingAgentID ? `/v1/agents/${encodeURIComponent(editingAgentID)}` : '/v1/agents', { method: editingAgentID ? 'PATCH' : 'POST', body: JSON.stringify(payload) }); agentDialog.close(); await render('control'); if (result.token) showAgentToken(result.token); else showToast('Agent 权限已经更新。'); } catch (error) { showToast(error.message, 'bad'); } });
$('#copy-agent-token').addEventListener('click', async () => { const value = $('#agent-token-value').textContent; try { await navigator.clipboard.writeText(value); showToast('令牌已复制，请放入 MCP 客户端的秘密配置。'); } catch (_) { showToast('自动复制失败，请手动选择令牌。', 'bad'); } });
tokenDialog.addEventListener('close', () => { $('#agent-token-value').textContent = ''; });

async function bootstrap() { try { await loadProjectRegistry(); await render(window.location.hash.slice(1) || 'portfolio'); } catch (error) { clear(); content.append(emptyState('MemoryOS 无法启动界面', error.message)); refreshChrome(); } }
bootstrap();
