import { useEffect } from "react";
import {
  Bot,
  CheckCircle2,
  FileInput,
  FolderPlus,
  ListChecks,
  Sparkles,
} from "lucide-react";

const steps = [
  { icon: FolderPlus, title: "1. 新建记忆空间", text: "在“记忆总览”最上方点击“新建记忆空间”，填写名称和可选说明。系统会自动准备其他设置。" },
  { icon: FileInput, title: "2. 导入原材料", text: "导入一整份对话或文件。原文会保持不变，不会因为整理或重跑被覆盖。" },
  { icon: Sparkles, title: "3. 整理本次新增内容", text: "默认只处理新导入、内容有变化或上次失败的原材料；已经成功且未变化的内容不会重复处理。" },
  { icon: CheckCircle2, title: "4. 确认 AI 建议", text: "AI 识别的身份、程序、长期记忆和待办建议，需要你确认后才会正式生效。" },
  { icon: ListChecks, title: "5. 处理项目待办", text: "人工创建的待办直接进入清单；AI 只能提出建议，不能替你确认。" },
  { icon: Bot, title: "6. 按需连接 Codex", text: "创建 Codex 身份后，选择允许访问指定空间还是全部空间，再把只显示一次的令牌安全配置到 MCP 客户端。" },
];

const pages = [
  {
    id: "home", title: "记忆总览", level: "基础",
    can: "先看今天要处理的待办和审核，再了解最近四周的记忆产出、自己固定的记忆和最近复盘。也可以直接新建记忆空间。",
    how: "点击待办或审核提示会直接进入对应处理页面；日历最右侧始终是今天。在记忆库中点击“固定到首页”，可以把你确认有用的项目记忆放到这里。需要另开主题时，点击最上方的“新建记忆空间”。",
    automatic: "总览只汇总已经存在的数据，不会因为打开页面而重新整理原材料，也不会由 AI 擅自决定哪些内容必须固定。",
    boundary: "不同记忆空间互相隔离；新建空间不会复制当前空间的内容。首页固定只改变展示位置，不会改写记忆正文、来源或重要程度。",
  },
  {
    id: "import", title: "导入记忆", level: "基础",
    can: "导入文件、粘贴文本或带入完整历史会话。",
    how: "选择一种导入方式，确认当前空间，再提交；一份文件或一整段会话会作为一份原材料保存。",
    automatic: "默认只整理本次新导入的内容；如果高级设置改成手动，就只保存原文。",
    boundary: "不会改写原文件，也不会把不同文件自动合并成一份来源。",
  },
  {
    id: "search", title: "检索", level: "基础",
    can: "在当前空间查找原材料、关键信息、项目记忆和相关记录。",
    how: "输入自然语言问题，先看结果摘要，再打开来源确认原文和时间。",
    automatic: "系统会在当前空间中排序相关结果，并保留命中原因和来源。",
    boundary: "基础检索不会跨空间；没有来源支撑的内容不会伪装成已确认事实。",
  },
  {
    id: "memory", title: "记忆库", level: "基础",
    can: "查看“原材料 → 关键信息 → 项目记忆”，并选择要整理的整份对话或文件。",
    how: "点击“处理新增或失败的原材料”处理默认范围；要自己选择时，点击“选择原材料”。在“项目记忆”卡片上点击“固定到首页”，可以把你确认有用的内容放到总览。",
    automatic: "已经成功且内容未变化的材料会跳过；每份材料分别显示成功或失败。",
    boundary: "原材料永远不改写；“强制重新处理全部”只在高级模式出现，并需要再次确认。",
  },
  {
    id: "projects", title: "项目工作台", level: "基础",
    can: "集中处理今天到期、已逾期、进行中、AI 建议和需要审核的事项。",
    how: "人工待办可直接开始或完成；AI 建议要先点“加入待办”，才会变成正式任务。",
    automatic: "系统会把已识别的行动项放进建议区，并保留它来自哪份原材料。",
    boundary: "AI 和 MCP 不能绕过你直接建立正式待办；长上下文默认只显示摘要。",
  },
  {
    id: "review", title: "待我审核", level: "基础",
    can: "确认或拒绝会影响长期记忆、身份、程序和能力资产的建议。",
    how: "先打开全文和来源，再选择接受或拒绝；不要只根据短摘要判断。",
    automatic: "系统会把高风险或有冲突的变化停在这里等待。",
    boundary: "未审核的建议不会静默进入长期记忆，也不会自动覆盖已有内容。",
  },
  {
    id: "team", title: "多 AI 协作", level: "基础",
    can: "让多个 AI 参与同一任务，同时保留每个 AI 自己的私密草稿。",
    how: "先创建 AI 身份，再建立协作任务、选择成员；需要共享时，由 AI 明确提交给指定对象。",
    automatic: "系统只管理成员、共享范围、有效期、冲突和审核记录。",
    boundary: "不会替你启动或调度外部 AI；私密草稿不互通，接收者不能转发别人的内容。",
  },
  {
    id: "connections", title: "模型与 Agent", level: "基础",
	can: "配置整理内容所用的模型，并为 Codex 或其他 AI 创建独立身份。支持 OpenAI Responses、Chat Completions、Anthropic Messages 和 OpenCode Go 混合模型。",
    how: "添加整理模型时先选常用接入，再选模型；系统会填入正确调用方式。保存后先点“测试”，再启用。创建 Agent 时另行选择权限和空间范围。",
    automatic: "选择已知模型时会自动匹配调用方式；连接测试会读取提供商此刻返回的模型列表。每次 Agent 调用仍会检查身份和空间范围。",
    boundary: "内置模型目录不是可用性承诺；只有连接测试通过才代表当前账号可调用。创建身份也不等于启动外部 AI。",
  },
  {
    id: "health", title: "健康与恢复", level: "基础",
    can: "查看数据库、原材料、索引和运行环境是否健康，并导出完整备份。",
    how: "先看总体状态，再查看失败项；需要了解功能时，点击本页的“打开操作手册”。",
    automatic: "检查失败、跳过或超时都不会被写成通过。",
    boundary: "健康检查只诊断，不会自动删除数据、重跑正式原材料或替你恢复备份。",
  },
  {
    id: "profiles", title: "上下文画像", level: "高级",
    can: "把长期记忆整理成某项任务可直接使用的背景资料。",
    how: "选择画像后查看稳定信息、项目动态和会话恢复内容；需要时再调整读取额度。",
    automatic: "系统按任务选择相关内容，并标注更新时间和来源。",
    boundary: "人工锁定的内容不会被自动重建静默覆盖。",
  },
  {
    id: "products", title: "知识产品", level: "高级",
    can: "把记忆整理成项目简报、报告、日记或档案。",
    how: "项目简报点击“手动生成 / 刷新”；新建报告、复盘、档案、决策日志或风险报告时先选模板，再填写内容。",
    automatic: "新材料完成项目沉淀时，项目简报会读取已确认记忆并安全更新；你也可以手动立即刷新，人工锁定段落会保留。",
    boundary: "不会用自动结果覆盖人工锁定内容，也不会删除旧版本。",
  },
  {
    id: "assets", title: "能力资产工坊", level: "高级",
    can: "把可复用经验整理成提示词、规则、步骤、工具配方或 MCP 能力。",
    how: "先查看七类标准模板，勾选一周或当前筛选中的候选，点击“按模板二次沉淀”；补齐缺失字段后查看差异和验证，再由 Owner 批准激活。",
    automatic: "增量模式只处理新增或来源有变化的候选；系统按资产类型检查必填字段、来源、权限和冲突。",
    boundary: "模型不可用时只生成不能激活的待补齐骨架；候选不会自动激活，原材料也不会被重跑或反向修改。",
  },
  {
    id: "timeline", title: "时间脉络", level: "高级",
    can: "按事件时间、有效时间、记录时间和截止时间查看项目变化。",
    how: "选择时间范围和类型，再打开记录查看来源。",
    automatic: "系统会把同一项目已有的时间记录放到同一时间轴；点击刷新只是重新读取当前视图。",
    boundary: "时间脉络是实时查询，不需要二次沉淀。刷新不会重跑原材料；推测时间也不会伪装成精确日期。",
  },
  {
    id: "runs", title: "运行记录", level: "高级",
    can: "查看一次整理具体执行了哪些步骤、产生了什么结果和错误。",
    how: "先看结果，再按需打开步骤、时间线、模型调用和影响记录。",
    automatic: "每次执行会保留独立运行记录和阶段输出。",
    boundary: "失败或超时不会写成成功；查看记录本身不会重新执行。",
  },
  {
    id: "strategy", title: "自动整理方式", level: "高级",
    can: "自定义什么时候整理、处理哪些材料以及哪些结果需要确认。",
    how: "先复制或修改方案，验证无误后再用于项目；普通用户保持默认即可。",
    automatic: "项目会按当前启用的方案处理新材料。",
    boundary: "修改方案不会自动重跑旧原材料；强制重跑仍需单独选择。",
  },
  {
    id: "studio", title: "流程编辑器", level: "高级",
    can: "用可视步骤调整一份自动整理流程。",
    how: "添加步骤、连接顺序、填写参数，先验证再发布。",
    automatic: "系统检查缺失连接、权限和无效参数。",
    boundary: "未发布流程不会影响正式项目；验证不会调用未授权外部工具。",
  },
  {
    id: "plugins", title: "插件中心", level: "高级",
    can: "为项目增加新的整理步骤、检索方式或工具。",
    how: "先查看发布者、权限和兼容性，再按项目启用所需能力。",
    automatic: "系统检查插件声明、版本、权限和兼容性。",
    boundary: "插件只能得到你明确授予的能力；停用不会删除历史记录。",
  },
  {
    id: "experience", title: "经验评估", level: "高级",
    can: "从真实运行结果总结哪些做法在什么条件下有效。",
    how: "查看运行、结果和独立评估，再决定是否形成可复用经验。",
    automatic: "系统关联运行条件、结果和评估证据。",
    boundary: "一次成功不会自动变成通用经验。",
  },
  {
    id: "adaptation", title: "改进实验", level: "高级",
    can: "针对失败经验提出局部改进，并做小范围验证和回滚。",
    how: "创建改进建议、独立评估、人工审核，再做小范围试用。",
    automatic: "系统保留建议、验证和回滚链路。",
    boundary: "不会因为一次失败自动重写整个整理方式。",
  },
  {
    id: "portable", title: "迁移与便携包", level: "高级",
    can: "选择性导出或导入记忆，同时检查目标环境是否兼容。",
    how: "先选择范围并预检，确认缺失能力和冲突后再导入。",
    automatic: "系统保留版本、来源和能力标签，并报告可能丢失的内容。",
    boundary: "外部内容只进入隔离或候选状态，不会直接成为正式长期记忆。",
  },
];

export function HelpCenter() {
  useEffect(() => {
    const section = window.sessionStorage.getItem("memory-harness.help-section");
    window.sessionStorage.removeItem("memory-harness.help-section");
    if (section) window.setTimeout(() => document.getElementById(`help-${section}`)?.scrollIntoView({ behavior: "smooth", block: "start" }), 80);
  }, []);

  return (
    <div className="help-center">
      <section className="help-start" id="help-start">
        <p>第一次使用</p>
        <h2>操作手册：六步完成从原材料到可用记忆</h2>
        <span>基础模式只展示完成这条流程所需的操作；需要调整规则、权限或运行细节时，再切换高级模式。</span>
        <div>{steps.map(({ icon: Icon, title, text }) => <article key={title}><Icon size={20}/><h3>{title}</h3><p>{text}</p></article>)}</div>
      </section>

      <section className="help-section" id="help-pages">
        <header><p>每个功能怎么用</p><h2>操作、自动行为和边界都写清楚</h2></header>
        <div className="help-page-grid">{pages.map((item) => <article id={`help-${item.id}`} key={item.id}>
          <header><h3>{item.title}</h3><span className={item.level === "高级" ? "advanced" : ""}>{item.level}功能</span></header>
          <dl>
            <div><dt>能做什么</dt><dd>{item.can}</dd></div>
            <div><dt>怎么操作</dt><dd>{item.how}</dd></div>
            <div><dt>系统会自动做什么</dt><dd>{item.automatic}</dd></div>
            <div><dt>系统不会做什么</dt><dd>{item.boundary}</dd></div>
          </dl>
        </article>)}</div>
      </section>

      <section className="help-section" id="help-team">
        <header><p>多 AI 协作</p><h2>它管理共享边界，不负责替你启动 AI</h2></header>
        <ol className="help-flow">
          <li><b>1</b><span><strong>创建并连接不同 AI</strong><small>每个 AI 都有独立身份、记忆空间和权限。</small></span></li>
          <li><b>2</b><span><strong>选择参与同一任务的 AI</strong><small>只有直接加入的成员才能参与，成员关系不会自动扩散。</small></span></li>
          <li><b>3</b><span><strong>AI 主动提交要共享的内容</strong><small>私密草稿只对作者可见；接收者不能转发别人的内容。</small></span></li>
          <li><b>4</b><span><strong>你处理冲突并决定长期保存</strong><small>冲突不会自动覆盖；未审核结果不能进入长期记忆。</small></span></li>
        </ol>
      </section>

      <section className="help-section" id="help-models">
        <header><p>整理模型怎么选</p><h2>先选提供商和模型，调用方式由系统匹配</h2></header>
        <div className="help-page-grid">
          <article><header><h3>OpenAI</h3><span>推荐 Responses</span></header><p>新建配置时默认使用 Responses。只有旧模型、旧网关或明确要求兼容接口时，才改为 Chat Completions。</p></article>
          <article><header><h3>Anthropic / Claude</h3><span>Messages</span></header><p>选择 Claude 预设后会使用 Anthropic Messages，不需要伪装成 OpenAI 接口。</p></article>
          <article><header><h3>OpenCode Go</h3><span>自动匹配</span></header><p>它的一组模型并不共用同一种调用方式。选择目录中的模型后，系统会自动匹配 Responses、Chat Completions 或 Messages。</p></article>
          <article><header><h3>其他服务和本地模型</h3><span>可自定义</span></header><p>选择兼容服务，再按服务说明选择调用方式。可以直接填写未收录的模型名；保存后必须先做连接测试。</p></article>
        </div>
      </section>

      <section className="help-section" id="help-manual-runs">
        <header><p>什么时候需要手动运行</p><h2>会“生成新版本”的功能才需要明确触发</h2></header>
        <div className="help-page-grid">
          <article><header><h3>能力资产二次沉淀</h3><span className="advanced">手动</span></header><p>适合每周或按项目筛选候选后集中整理。默认只处理新增或有变化的候选；“重新检查当前范围”才会再次调用模型。</p></article>
          <article><header><h3>项目简报</h3><span className="advanced">自动 + 手动</span></header><p>新材料完成沉淀时会安全更新；点击“手动生成 / 刷新项目简报”可以立即检查并刷新。报告、复盘等由你按模板新建。</p></article>
          <article><header><h3>上下文画像与经验评估</h3><span className="advanced">手动</span></header><p>需要重新投影或重新发现经验时，在对应页面点击重建。人工锁定内容和旧版本仍然保留。</p></article>
          <article><header><h3>时间脉络、项目工作台与首页</h3><span>实时视图</span></header><p>这些页面直接读取当前记录。刷新只重新加载显示，不会重跑原材料，也不需要建立重复版本。</p></article>
        </div>
      </section>

      <section className="help-section" id="help-glossary">
        <header><p>名词对照</p><h2>界面中的普通说法与高级名称</h2></header>
        <dl className="help-glossary">
          <div><dt>原材料</dt><dd>Evidence：导入后保持不变的原文与来源</dd></div>
          <div><dt>关键信息</dt><dd>Knowledge Unit：从原材料中识别出的事实、目标或决定</dd></div>
          <div><dt>多 AI 协作</dt><dd>Team Memory：不同 AI 在受控任务中共享内容</dd></div>
          <div><dt>自动整理方式</dt><dd>Blueprint / Pipeline：系统按什么步骤整理材料</dd></div>
          <div><dt>AI 最多读取多少内容</dt><dd>Context Budget：一次提供给 AI 的最大内容额度</dd></div>
          <div><dt>外接工具连接</dt><dd>MCP：让 Codex 等外部 AI 按授权调用本应用功能的连接方式</dd></div>
		  <div><dt>模型调用方式</dt><dd>Protocol：应用向模型发送请求时使用的接口格式；它不是模型名称</dd></div>
        </dl>
      </section>
    </div>
  );
}
