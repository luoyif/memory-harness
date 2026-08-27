# ADR-010：结构化知识、语义图、时间图与插件统一物化

状态：已接受，第一阶段已实现
日期：2026-08-21
依赖：ADR-009、Plugin Contract、Trace Contract
替代范围：不替代不可变 Evidence；逐步替代“单句 Knowledge Unit + 事后模拟 Run”的默认派生链

## 1. 背景

当前 Knowledge Unit 把所有语义压成一句 statement。这种结构能够展示文本，也能做关键词/向量检索，但无法可靠回答：

- 这句话是谁说的？
- 事实是在说谁？
- 谁对谁做了什么？
- 发生在哪里、什么时候？
- 这是一条事实、一个事件、一个目标还是一个引用观点？
- 新内容是补充、重复、冲突还是取代旧事实？
- 图上的线是来源关系，还是现实世界中的语义关系？

旧版默认导入曾不执行真实 Harness Pipeline，也不物化 Harness Object、Object Relation 或 Temporal Fact。第一阶段重构已经将它们统一到可替换的六阶段 Pipeline；既有真实数据仍由 Owner 手动选择是否重跑。

## 2. 决策

### 2.1 保留 Evidence 为根

所有导入先生成不可变 Evidence：

- 保存原文/原始转写；
- 保存来源系统、会话、角色、导入时间和原始时间；
- 保存内容哈希；
- 保存项目/个人作用域；
- 派生对象只引用 Evidence，不覆盖 Evidence。

模型输出、人工编辑、插件结果都属于派生修订。

### 2.2 引入 Knowledge Unit v2

Knowledge Unit v2 不是一条三元组，也不是一段摘要，而是一个“最小可独立检查的语义断言或事件框架”。

建议内核信封：

```json
{
  "unit_id": "ku_...",
  "schema_version": "memory-harness.knowledge-unit/v2",
  "project_id": "project-personal",
  "kind": "fact|state|event|decision|goal|risk|outcome|procedure|identity|correction",
  "statement": "可独立阅读的规范化句子",
  "language": "zh-CN",
  "attribution": {},
  "frame": {},
  "temporal": {},
  "epistemic": {},
  "provenance": {},
  "status": "candidate|active|review_required|ambiguous|rejected|superseded"
}
```

#### Attribution

```json
{
  "source_speaker_ref": "participant:speaker-a",
  "asserted_by_ref": "participant:speaker-a",
  "subject_ref": "entity:...",
  "subject_surface": "他",
  "resolution": "resolved|unresolved|ambiguous|not_applicable",
  "candidate_refs": ["entity:..."],
  "reason_codes": ["pronoun_cross_chunk"],
  "owner_mapping": "not_assumed"
}
```

严格区分：

| 字段 | 含义 |
| --- | --- |
| source_speaker | 原始记录里哪个声道/角色说了这句话 |
| asserted_by | 谁主张这项信息 |
| subject | 这项事实实际在描述谁/什么 |
| owner | Memory Harness 本地管理者身份 |

默认规则：role=user 只表示来源消息角色，不等于 Owner，也不自动等于事实 subject。

#### Semantic Frame

```json
{
  "subject": {"entity_id": "entity:...", "surface": "张三"},
  "predicate": "plans_to",
  "object": {"kind": "entity|literal|event", "entity_id": "entity:...", "value": null},
  "action": "前往交换学习",
  "participants": [
    {"role": "actor", "entity_id": "entity:..."},
    {"role": "destination", "entity_id": "place:uk"}
  ],
  "locations": [{"entity_id": "place:uk", "role": "destination"}],
  "quantities": [],
  "context": "当前出国交换计划"
}
```

不是每种 Unit 都要求所有字段：

- Fact/State/Identity 需要 subject、predicate、object；
- Event/Outcome 需要 action、participants，可选 location/time；
- Goal/Decision/Risk 需要 owner/subject、content、context；
- Procedure 需要 trigger、steps、inputs、outputs；
- 无法可靠解析时保留 statement，并标记 unresolved，禁止编造实体。

#### Temporal

```json
{
  "observed_at": "系统收到 Evidence 的时间",
  "recorded_at": "派生记录写入时间",
  "event_time_text": "下周或下下周",
  "valid_from": null,
  "valid_until": null,
  "occurred_from": null,
  "occurred_until": null,
  "precision": "exact|day|week|month|range|relative|unknown",
  "resolution": "resolved|relative_pending|ambiguous|not_applicable",
  "anchor_evidence_time": "2026-08-21T..."
}
```

#### Epistemic

```json
{
  "polarity": "positive|negative",
  "modality": "asserted|planned|desired|hypothetical|reported|uncertain",
  "confidence": 0.86,
  "importance": 0.62,
  "novelty": 0.71,
  "quality_flags": [],
  "review_reasons": []
}
```

#### Provenance

```json
{
  "evidence_id": "evidence:...",
  "quote": "边界内的短原文片段",
  "span": {"start": 120, "end": 178, "quote_hash": "sha256:..."},
  "episode_id": "episode:...",
  "run_id": "run:...",
  "span_id": "span:...",
  "extractor_plugin": "builtin.semantic-frame",
  "extractor_version": "1.0.0",
  "model_profile": "configured-default",
  "prompt_hash": "sha256:..."
}
```

### 2.3 结构化抽取拆为可替换插件

默认链不是一个大 Prompt，而是以下可独立替换、测试的阶段：

1. `transcript.normalize`
   - 只清理格式和低价值口头噪声；
   - 不重写语义，不丢原文。

2. `participant.detect`
   - 识别声道、消息角色、显式姓名；
   - 建立会话 Participant Registry；
   - 不自动映射 Owner。

3. `entity.extract`
   - 人、组织、项目、产品、地点、工具、概念；
   - 输出 surface、type、aliases、Evidence span。

4. `entity.resolve`
   - 在项目作用域内做别名、指代和同一实体候选；
   - 低置信度输出 ambiguous，不自动合并。

5. `semantic-frame.extract`
   - 产生 Knowledge Unit v2；
   - 输出实体、动作、关系、参与者和事实类型。

6. `temporal.resolve`
   - 解析绝对/相对时间与有效期；
   - 使用 Evidence 时间作为相对表达锚点。

7. `memory.quality-gate`
   - 过滤寒暄、复述、无主体短句和模型元话语；
   - 保存 rejection reason 和数量；
   - 禁止只返回“过滤后多少条”而没有原因。

8. `memory.reconcile`
   - NEW / CORROBORATE / UPDATE / MERGE / SUPERSEDE / CONFLICT；
   - 受保护对象进入人工门。

每个阶段都固定输入/输出 Schema、版本、模型配置、权限和测试 Fixture。

### 2.4 引入 Entity 与 Assertion

新增通用对象：

#### Entity

- entity_id；
- project_id；
- type：person / organization / project / product / place / tool / concept / event；
- canonical_name；
- aliases；
- properties；
- status；
- confidence；
- source Evidence；
- merge/split history。

同名不自动合并。跨项目实体默认隔离；跨项目关联需要显式授权或全局实体映射。

#### Assertion

- assertion_id；
- project_id；
- subject_entity_id；
- predicate_id；
- object_entity_id 或 literal；
- assertion kind；
- valid_from / valid_until；
- recorded_at；
- confidence；
- status；
- source Unit/Evidence；
- supersedes / conflicts_with；
- plugin/run/span provenance。

Assertion 是语义关系的权威记录。Knowledge Unit 是人可读且可审核的最小记忆对象；一个 Unit 可以产生零到多条 Assertion。

### 2.5 双链的存储与显示

“双链”采用单事实、双向邻接：

- 只存一条权威 Assertion；
- 建立 source 和 target 两端索引；
- 关系类型注册表定义正向标签和反向标签；
- 查询 direction=incoming/outgoing/both；
- UI 从任一实体都能看到对方和原始 Evidence。

示例：

```text
张三 --works_on--> Memory Harness
Memory Harness <--worked_on_by-- 张三
```

这里不是存两条可分别修改的事实，而是同一 Assertion 的两种读取方向，避免冲突。

### 2.6 三图分离

#### Lineage Graph：沉淀链路

回答：

- 这条记忆来自哪段原文？
- 哪个 Run/插件生成了它？
- 它又产生了哪个项目对象或能力资产？

节点：Evidence、Chunk、Knowledge Unit、Episode、Memory、Living Asset、Agent Asset、Run。
边：extracts、consolidates、projects、materializes、supersedes。

#### Semantic Graph：实体关系

回答：

- 谁与谁有什么关系？
- 这个项目有哪些人、工具、地点、决定和事实？
- 这项关系的来源和置信度是什么？

节点：Entity、Event、Concept、Project、Assertion 可视化节点。
边：类型化 Predicate，支持 incoming/outgoing/both、1-hop/2-hop、筛选和来源抽屉。

#### Temporal Graph / Timeline：时间演化

回答：

- 某一时点什么是真的？
- 一项事实何时开始、结束或被取代？
- 事件之间的 before / after / during / overlaps / causes 关系是什么？

节点：Entity、Event、Temporal Fact。
边：validity、supersedes、before、after、during、overlaps、causes。
提供 As-of 查询和时间线，不把墙上显示顺序当成事实顺序。

### 2.7 时间采用双时间模型

至少区分：

- valid time：现实中什么时候成立；
- transaction time：系统什么时候知道/记录；
- Evidence observed time：来源什么时候被捕获；
- event time：描述的事件什么时候发生。

第一阶段本地 SQLite 实现，不引入图数据库硬依赖。现有 `temporal_facts` 表升级后由默认 Pipeline 自动写入，且每条 Fact 引用 Assertion/Unit/Evidence。

### 2.8 统一默认物化链

现有 `recordCompilationRun` 的“事后模拟 Run”停止作为权威执行路径。

新默认导入：

```text
Capture Evidence
  -> Normalize
  -> Detect Participants
  -> Extract + Resolve Entities
  -> Extract Semantic Frames
  -> Resolve Time
  -> Quality Gate
  -> Reconcile Memory
  -> Materialize Objects + Assertions + Temporal Facts
  -> Build Episode
  -> Project to Workspace
  -> Compile Living Assets
  -> Propose Typed Agent Assets
  -> Refresh Search
```

这条链必须真正由 Pipeline Runtime 执行。每个对象的 revision 记录 plugin/run/span。

### 2.9 Object Store 与旧表迁移

迁移期间遵循：

1. Evidence 不动；
2. 为旧 Knowledge Unit/Memory/Asset 生成可重建映射；
3. Shadow Build 新对象，不切换 UI；
4. 对数量、来源、类型、文本和关系做 Diff；
5. Owner 查看迁移报告；
6. 切换读取到 Object Store；
7. 旧表进入兼容只读；
8. 经过一个版本后再讨论删除，当前迭代不删除。

禁止长期双向双写。切换后新对象只由 Object Store 负责 revision/lifecycle；旧 API 使用兼容 Projection。

### 2.10 能力资产工厂

能力资产不是“程序记忆的另一个名称”。引入以下类型和验证器：

| 类型 | 必填核心 | 默认验证 |
| --- | --- | --- |
| Prompt | prompt_text、variables、when_to_use | 变量完整性、示例、注入风险 |
| Skill | inputs、outputs、steps、tools、constraints、success_checks | Fixture、Dry Run、权限 |
| Rule | normative_body、applies_to、exceptions | 冲突检测、作用域 |
| Constraint | requirement、severity、scope | 强制门、冲突检测 |
| Procedure | trigger、steps、owner、outputs | 步骤完整、可执行性 |
| MCP | server/tool manifest、transport、permissions | Schema、连接、授权、危险写入 |
| Tool Recipe | ordered calls、preconditions、verification | 工具存在、参数 Schema、结果检查 |

资产生命周期：

```text
memory candidate
 -> typed asset candidate
 -> compile draft
 -> lint / fixture / dry-run
 -> human review
 -> active version
 -> export/install/bind to Agent
 -> observe outcome
 -> propose next revision
```

模型或插件只能产生 candidate。激活、外部安装、权限扩大和高风险规则变化必须由 Owner 审核。

### 2.11 插件贡献增加 Surface 元数据

每个 Memory Type、Stage、Pipeline、Projection 和 View 增加：

```yaml
surfaces:
  - memory.library
  - memory.semantic-graph
  - memory.temporal
  - project.workspace
  - asset.foundry
  - recall.search
  - run.explorer
inputs: [evidence, knowledge-unit-v2]
outputs: [assertion, temporal-fact]
triggers: [automatic, manual, schedule]
```

前端用它回答：

- 这个插件处理什么？
- 在哪里生效？
- 什么时间运行？
- 会生成什么？
- 当前项目已有多少对象？
- 最近一次运行是否成功？
- 需要哪些权限？

### 2.12 项目工作台 View Contribution

项目工作台不再只有硬编码卡片。插件可声明 View：

- widget ID/version；
- surface=project.workspace；
- 数据 Projection；
- 默认尺寸；
- 可配置项 Schema；
- 读写能力；
- 空状态和权限不足状态；
- 默认可见性。

默认模板仍包含核心上下文、目标、里程碑、决策、风险、资金。用户可以添加商机、客户、实验、论文、时间线、Agent 资产等卡片，并调整顺序/折叠，不需要编辑 JSON。

## 3. API 方向

### 3.1 知识单元

- `GET /v1/knowledge-units/{id}`：返回结构化 Unit、来源片段和解析状态；
- `POST /v1/knowledge-units/{id}/resolve`：人工选择主体/实体/时间候选；
- `POST /v1/knowledge-units/{id}/reprocess`：产生新 Run 和修订，不覆盖历史。

### 3.2 图谱

- `GET /v1/graphs/lineage`；
- `GET /v1/graphs/semantic`；
- `GET /v1/graphs/temporal`；
- `GET /v1/entities/{id}/neighbors?direction=both&depth=1`；
- `GET /v1/assertions/{id}`；
- `POST /v1/entities/merge-proposals`；
- `POST /v1/entities/{id}/aliases`。

### 3.3 沉淀控制

- `POST /v1/runs`：run all；
- `POST /v1/runs/preview`：Dry Run；
- `POST /v1/runs/{id}/fork`：从阶段/检查点分支；
- `POST /v1/objects/{id}/reprocess`；
- `PUT /v1/projects/{id}/automation`：automatic / assisted / manual / schedule。

每次触发都创建新 Run；不复用已完成 Run ID，不篡改历史 Span。

### 3.4 资产

- `GET /v1/assets?type=...`；
- `POST /v1/assets/compile`；
- `POST /v1/assets/{id}/validate`；
- `POST /v1/assets/{id}/review`；
- `POST /v1/assets/{id}/activate`；
- `POST /v1/assets/{id}/export`。

激活与导出是不同动作。导出不自动安装到 Codex、ChatGPT、DSH 或其他 Agent。

## 4. 兼容与拒绝项

### 保留

- 当前 Go Core；
- SQLite 本地优先；
- Evidence JSONL/索引；
- 项目/Agent 权限；
- Harness Object/Revision/Run/Span/Event 基础；
- React/Wails 产品壳；
- MCP 作为外部 Agent 接入通道。

### 拒绝

- 用一个更大的 Prompt 继续输出 statement；
- 把现有来源 DAG 改个配色继续叫知识图谱；
- 同时长期维护旧表和 Object Store 两个可写真相源；
- 为了图谱立即引入必须运行的 Neo4j/Chroma 服务；
- 自动把“用户”“我”“说话者”归为 Owner；
- 自动激活模型生成的 Skill/Rule/Constraint/MCP；
- 把 Pipeline Studio 的空白画布作为新用户入口；
- 把插件 JSON 配置当作普通用户的 DIY 体验。

## 5. 结果

### 正向结果

- 知识点可检查、可纠正、可引用；
- 实体关系、来源链和时间演化不再混图；
- 插件真正产生对象和前端结果；
- 同一份导入能生长为记忆、项目状态和能力资产；
- DIY 能对应到具体产品页面和数据；
- Run 可以解释业务结果而不只是显示节点。

### 成本

- 需要数据库加表/加字段和迁移；
- 需要重新设计默认抽取 Pipeline；
- 需要一套质量评测语料；
- 需要实体合并/拆分和歧义处理 UI；
- 需要把现有 72/64/17 个派生对象在 Shadow 模式下重建对比。

## 6. ADR 验收

ADR 只有在以下测试通过后才算 implemented：

1. 固定会话能正确区分 speaker、subject 和 Owner；
2. 固定语料能抽取人、动作、对象、地点和时间；
3. 不明确的“他/他们/这个”进入 ambiguous，不被强行归属；
4. Semantic Graph 出现真实 Entity 与 Predicate，不包含六列来源扇形；
5. Lineage Graph 仍能回到 Evidence；
6. as-of 查询能返回旧事实，current 查询只返回新事实；
7. incoming/outgoing/both 都返回同一 Assertion 的双向邻接；
8. 默认导入真实产生 Harness Object、Relation 和 Temporal Fact；
9. 资产至少能分出 Prompt、Skill、Rule、Constraint、Procedure，且候选不自动激活；
10. 任一对象可从 UI 追到 Run、Plugin、Unit 和 Evidence。
