# mulix-coding 实施计划

移植来源与对应关系:

| 来源 | 移植内容 | 落地位置 |
|---|---|---|
| spec-kit (本仓库) | constitution/specify/clarify/plan/tasks/analyze/implement 流程与模板体系、CLI 交互模式 | `internal/scaffold`、`assets/templates`、`internal/cliutil`(命令面) |
| comet | phase 状态机、guard 前置检查、PreToolUse hook 强制拦截 | `internal/flow`、`internal/guard`、`internal/hook` |
| superpowers | brainstorming 三档流程(spike/bounded/architectural)、TDD 红绿重构铁律 + 验证 checklist | `assets/skills/mulix-specify/SKILL.md`(change 粒度分类)与 `assets/skills/mulix-build/SKILL.md`(task 粒度分类 + TDD 铁律)正文,`internal/guard` 的 `tdd-evidence-present` 检查项。原 `mulix-brainstorm` 独立技能与 `brainstorm-approved` 检查项已在 Phase 8 移除,详见该节 |
| caveman | SKILL.md description 只写触发条件不写摘要的纪律;探索委托给廉价子代理、只回传引用的原则 | `assets/skills/*.md` 撰写规范(原则性移植,不移植其压缩引擎/BM25/浏览器代理等实际基础设施) |

第一阶段范围:仅适配 Claude Code(`.claude/skills` + `.claude/settings.json` 的 PreToolUse/SessionStart hook)。不做其他 agent host、不做 caveman 的压缩引擎/多 profile、不做 comet 的 Native 工作流/dashboard/worktree supervisor、不做 spec-kit 的 GitHub issue 转换/自更新/preset 系统。

## Phase 0 — 项目骨架(已完成)

- [x] `go.mod`(module `github.com/mulix-dev/mulix-coding`)
- [x] 目录骨架:`cmd/mulix`、`internal/{flow,state,guard,hook,scaffold,cliutil}`、`assets/{skills,templates}`

## Phase 1 — 核心状态机与强流程控制(代码已完成,待接入 CLI)

> **注**:本节的 9 阶段描述(含 brainstorm 独立阶段)是 Phase 1 完成时的历史记录。
> Phase 8 移除了 brainstorm 阶段,当前实际为 8 阶段。见 Phase 8。

- [x] `internal/flow`:9 阶段(brainstorm→specify→clarify→plan→tasks→analyze→build→verify→archive)+ 转移表 `Table` + 纯函数 `Apply`。单测覆盖合法转移、非法转移报错、verify-fail 计数、verify-pass 置位 archive-pending、build-complete 重置 verify_result。
- [x] `internal/state`:`.mulix/state/<change>.yaml` 严格 YAML 解码(未知字段/schema 不符即报错,对齐 comet 的 fail-loud 校验),`Save`/`Load`/`Exists`。`internal/state/root.go`:`FindRoot`(向上查找 `.mulix/` 目录,类似 git)、`SetActive`/`Active`(当前激活 change,对齐 comet 的 current-selection 文件)。单测覆盖往返、未知字段拒绝、schema 不符拒绝。
- [x] `internal/guard`:按 event 注册的前置检查表(`checksByEvent`),每项检查返回 pass/fail + 修复提示(`Next`),不做状态变更。已实现:brainstorm-approved、spec/plan/tasks/analyze 产物存在且非空、clarify 记录或显式跳过确认、tasks 全勾选、verify 结果一致性、archive 显式确认。单测覆盖每类检查的通过/失败路径。
- [x] `internal/hook`:Claude Code PreToolUse 请求解析(`ParseRequest`)、按 phase 白名单的写权限判定(`Decide`)、响应渲染(`Render`,`hookSpecificOutput.permissionDecision`)与退出码(`ExitCode`:allow=0/deny=2)。规则:
  - 非 Write/Edit 工具永远放行(不拦截探索/验证类操作)。
  - `.mulix/state/**` 任何阶段都不可直写(必须走 `mulix state transition`,防止绕过状态机直接改分数板)。
  - brainstorm 阶段只能写 change 目录 + `docs/`(design doc);specify/clarify/plan/tasks/verify 阶段只能写 change 目录;build 阶段不限制(源码/测试在这里写);archive 阶段禁止一切写入。
  - 路径匹配是分段感知的(`specs/1-other` 不会误匹配前缀 `specs/1`),支持绝对路径转相对根目录。
  单测覆盖以上每条规则 + 响应/退出码渲染。

**Phase 1 收尾(已完成)**:
- [x] `internal/state`:`List`(枚举 `.mulix/state/*.yaml` 下所有 change,排序返回)供 `mulix status` 使用;`root_test.go` 覆盖 `FindRoot`(向上查找/未找到报错)、`SetActive`/`Active`(往返/未设置报错)、`List`(空/多个排序)。
- [x] `internal/flow`:`NextEvents(phase)` 返回该 phase 下所有合法事件,供 CLI 提示下一步可做什么,不用在 CLI 里重复 phase 逻辑。单测覆盖 clarify 分支(complete/skip 两个出口)和 archive 的自循环。

Phase 1 全部完成,`go build ./...` 和 `go test ./...` 全绿(4 个包,共 31 个测试)。

## Phase 2 — CLI 命令面(`cmd/mulix` + `internal/cliutil`)(已完成)

使用 Cobra。已实现命令:

- [x] `mulix init [--dir .] [--force]` — 写入 `.mulix/state/`、`.mulix/memory/constitution.md`、`.mulix/templates/*`(从 `assets` embed 拷贝)、`.claude/skills/mulix-*/SKILL.md`、合并(非覆盖)`.claude/settings.json` 的 `PreToolUse` hook 配置。未加 `--force` 时已存在文件跳过并报告;幂等(二次运行只有 `.mulix/state/` 会重复出现在 written 里)。
- [x] `mulix new <title>` — 用 `scaffold.CreateChangeDir` 生成 `specs/NNN-slug/` 目录、`flow.New` 初始状态(brainstorm 阶段)、`state.SetActive`。`--branch` 可选记录分支名(不代为建分支)。
- [x] `mulix state show [--change x]` — 打印当前 phase 及关键字段 + `flow.NextEvents` 提示。
- [x] `mulix state set <field> <value> [--change x]` — 白名单字段写入(`spec_path`/`plan_path`/`tasks_path`/`analyze_path`/`verify_report`/`brainstorm_track`/`clarify_skipped`/`analyze_skipped`/`verify_result`/`archive_confirmation`),只改字段不改 phase。
- [x] `mulix state transition <event> [--change x] [--force]` — 先 `guard.Run` 打印报告,不过全部通过则报错退出非零;通过才 `flow.Apply` + `state.Save`。`--force` 可强行跳过(技能文本明确警告不推荐)。
- [x] `mulix state select <change>` — 切换 active change。
- [x] `mulix guard <event> [--change x]` — 只读跑检查,不转移状态。
- [x] `mulix hook`(Hidden 命令,实际 PreToolUse 入口):读 stdin JSON → `hook.ParseRequest` → 定位 root/active change/state → `hook.Decide` → `hook.Render` + 对应 exit code。任何环节出错(找不到 root/active change/state 损坏)都 **fail open**(放行 + stderr 警告),避免配置问题把项目所有写操作锁死。
- [x] `mulix status` — 列出所有 change 及 phase,`*` 标记当前 active。
- [x] `mulix version`

## Phase 3 — 内容资产(`assets/skills`, `assets/templates`)(已完成)

- [x] `assets/templates/`:`constitution-template.md`、`spec-template.md`、`plan-template.md`、`tasks-template.md`、`analyze-template.md`。
- [x] `assets/skills/mulix-using-mulix/SKILL.md`:元技能,说明双层强制机制(hook 拦截 + guard 检查),要求先跑 `mulix state show` 而非凭对话记忆判断 phase。
- [x] `assets/skills/mulix-brainstorm/SKILL.md`:移植 superpowers 三档流程(spike/bounded/architectural)+ ratchet 单向升级规则 + hard gate。
- [x] `assets/skills/mulix-specify/mulix-clarify/mulix-plan/mulix-tasks/mulix-analyze/SKILL.md`:对齐 spec-kit 对应阶段的产物要求,改为调用 `mulix` CLI。
- [x] `assets/skills/mulix-build/SKILL.md`:移植 superpowers TDD 铁律(红-绿-重构 + verify 步骤 + checklist)+ caveman 探索委托原则。
- [x] `assets/skills/mulix-verify/mulix-archive/SKILL.md`:verify 阶段记录构建/测试结果;archive 阶段呈现 merge/PR/keep/discard 菜单,要求显式确认。
- [x] 所有 `description` 字段只写触发条件,不写流程摘要(caveman/superpowers 纪律)。

## Phase 4 — 打包与验证(已完成)

- [x] `assets/assets.go` 用 `go:embed` 把 `skills/`、`templates/` 编译进二进制;`internal/scaffold/init.go` 从 embed FS 读取并写入目标项目,`init` 完全离线可用。
- [x] `internal/scaffold/init_test.go`:6 个集成测试,覆盖文件写入、hook 注册到 settings.json、保留用户已有的其他设置、无 `--force` 时跳过已存在文件、`--force` 覆盖、幂等性。
- [x] `go build ./...`、`go vet ./...`、`go test ./...` 全绿;`gofmt -l .` 无输出。
- [x] 手动端到端烟测(临时目录):`init` → `new` → `state transition`(guard 失败阻断 → 补字段后成功)→ 模拟 Claude Code 发来的 `PreToolUse` JSON 验证 hook 正确 allow/deny(包括 change 目录内允许写、目录外拒绝写、`.mulix/state/**` 任何情况下都拒绝直写)。
- [x] `README.md`:项目定位、四个来源的移植对应关系、workflow 图、安装/常用命令、开发命令。

## 完成状态

Phase 0-7 全部完成。8 个包(`flow`/`state`/`guard`/`hook`/`scaffold`/`preset`/`cliutil`/`cmd/mulix`)共 85 个单测全部通过(`cmd/mulix` 本身仍无独立单测,靠 `cliutil` 的端到端 cobra 测试 + 手动烟测覆盖)。`go build ./...`、`go vet ./...`、`gofmt -l .` 全部无输出。核心交付:

- 强流程状态机(9 阶段,`internal/flow`)+ 严格状态持久化(`internal/state`)+ 前置检查(`internal/guard`)+ Claude Code PreToolUse 强制拦截(`internal/hook`)全部落地并联调通过。
- `mulix init` 可离线在任意项目安装 Claude Code 技能包 + hook 配置,合并而非覆盖用户已有 `settings.json`。
- 10 个 SKILL.md(含 brainstorm 三档流程、build 阶段 TDD 铁律、阶段无关的 `mulix-taskstoissues`)全部移植完成并遵守 description-only-trigger 纪律。
- preset 系统(`internal/preset`):manifest/registry/三层覆盖解析/本地目录与 HTTPS zip URL 安装/内置+可配置 catalog,以及 `mulix preset` 全套子命令,端口自 spec-kit 的 `PresetResolver`/`PresetRegistry`/`common.sh`。
- `mulix-taskstoissues`:基于 `gh` CLI(非 GitHub MCP,按用户选择保持零外部依赖定位)把 `tasks.md` 转换为去重后的 GitHub issue。
- `assets/templates/*.md` 五个产物模板与 spec-kit 的 `templates/*.md` 几乎逐字对齐(章节结构、字段命名、`FR-###`/`SC-###` 编号、User Story 分阶段等),同时保留 `internal/guard` 依赖的精确锚点(`## Clarifications`、`- [ ]`)不变。
- 8 个 SKILL.md(specify/clarify/plan/tasks/analyze/build/verify/taskstoissues)与 spec-kit `templates/commands/*.md` 对应命令的执行步骤、检查清单、格式规则几乎逐字对齐,不引入 mulix 没有的 extensions.yml 钩子/多脚本变体/handoffs 机制。

未做(按计划明确排除):其他 agent host 适配、caveman 压缩引擎/BM25/browse 基础设施、comet Native 工作流/dashboard、spec-kit 的自更新、`checklist-template.md`(mulix 无对应的 `/checklist` 阶段或命令)、`constitution`/`converge` 对应的独立阶段技能(前者由 `mulix init` scaffold 直接处理,后者是 mulix 当前 9 阶段流程之外的新功能)。

## Phase 5 — preset 系统 + taskstoissues(扩展需求,已完成)

来源:spec-kit `src/specify_cli/presets/__init__.py`(`PresetResolver`/`PresetRegistry`)、`scripts/bash/common.sh` 的 `resolve_template()`/`resolve_template_content()`、`presets/catalog.json`+`catalog.community.json`、`templates/commands/taskstoissues.md`。

设计决策(已与用户确认):
- **issue 转换**:走 `gh` CLI(Bash 调用),不依赖 GitHub MCP server —— 对齐 mulix 当前“零外部依赖、离线可用”的定位,用户只需装好 `gh` 并登录即可用,不需要额外配置 MCP。落地为新技能 `assets/skills/mulix-taskstoissues/SKILL.md`,阶段无关(tasks.md 存在后随时可用,不接入 `internal/flow` 状态机,理由与 spec-kit 一致:这是任务列表的衍生操作,不是阶段推进)。
- **preset 范围**:做满“本地安装 + URL 下载 + 内置 catalog”,不做自更新。即:
  - `internal/preset` 包:manifest 结构体(`preset.yml` 等价)、registry(`.mulix/presets/.registry` JSON)、覆盖解析(项目覆盖 → presets 按 priority → core bundled,三层,mulix 无 extensions 层所以比 spec-kit 少一层)、组合策略(`replace`/`prepend`/`append`/`wrap`,`{CORE_TEMPLATE}` 占位符)。
  - 分发格式:GitHub 风格的 zip 包(`archive/zip` 标准库直接支持,不引入第三方解压依赖),`mulix preset add <url>` 下载并解压到 `.mulix/presets/<id>/`;`mulix preset add <本地目录>` 直接拷贝。可选 `--sha256` 校验。
  - 内置 catalog:`assets/presets/catalog.json`(embed 进二进制,对齐 spec-kit 的 `bundled: true` 概念,但 mulix 第一阶段的 catalog 只做 id→{name,description,tags},不打包实际 preset 内容——因为 mulix 目前没有 lean/constitution-sync 等价的官方 preset 可供 bundled;catalog 主要用于 `preset search`/`preset catalog list` 的元数据展示与 `preset add <id>` 时解析 `download_url`)。
  - CLI:`mulix preset list/add/remove/enable/disable/set-priority/info/resolve/search`,`mulix preset catalog list/add/remove`。

### 任务拆解

- [x] `internal/preset/manifest.go`:`Manifest`(`SchemaVersion`, `PresetInfo{ID,Name,Version,Description,Author,Repository,License}`, `Requires{MulixVersion}`, `Provides{Templates []TemplateEntry}`, `Tags`)、`TemplateEntry{Type,Name,File,Description,Strategy,Replaces}` + `EffectiveStrategy()`,`LoadManifest(path)`(严格 YAML 解码 + `Validate()`:schema 版本、必填字段、至少一个模板、strategy 合法性)。
- [x] `internal/preset/registry.go`:`.mulix/presets/.registry` JSON 读写(`Entry{Version,Source,ManifestHash,Enabled,Priority,InstalledAt}`),`LoadRegistry/Save/Add/Remove/SetPriority/SetEnabled/ListByPriority`(按 priority 升序 + id 字母序排列,对齐 spec-kit `list_by_priority`)。
- [x] `internal/preset/resolve.go`:`ResolveTemplateContent(root, name string, coreFS fs.FS) (string, error)` —— 端口 `resolve_template_content` 的三层栈(project overrides → presets by priority → core),组合策略实现(replace 短路并跳出循环、prepend/append 用 `"\n\n"` 拼接、wrap 用 `strings.ReplaceAll` 替换 `{CORE_TEMPLATE}`),无 replace 基底时报错(对齐 common.sh 的 "no replace base" 错误)。
- [x] `internal/preset/install.go`:`InstallFromDir(root, srcDir string) (InstallResult, error)`(本地目录拷贝 + manifest 校验 + 写 registry,复装保留已有 priority/enabled);`InstallFromURL(root, url, sha256Hex string) (InstallResult, error)`(仅允许 HTTPS/localhost,标准库 `archive/zip` 解压,兼容 GitHub 归档的顶层包装目录,zip-slip 防护,可选 sha256 校验)。
- [x] `internal/preset/catalog.go` + `internal/preset/catalogs.go`:`CatalogEntry{ID,Name,Description,Tags,DownloadURL,SHA256}`,`LoadBundledCatalog(fs.FS)`(从 `assets/presets/catalog.json` embed 读取)、`ParseCatalogJSON`、`(Catalog) Search(query)`;`CatalogRef` + `AddCatalogRef/RemoveCatalogRef/LoadCatalogRefs`(存于 `.mulix/presets/catalogs.json`,`default` 名保留给内置 catalog)、`FetchCatalog`(HTTP GET 自定义 catalog URL)。
- [x] `assets/presets/catalog.json`(目前为空 catalog,mulix 还没有官方 bundled preset)+ `assets/assets.go` 增加 `//go:embed presets` → `Presets embed.FS`。
- [x] `internal/cliutil/preset_cmd.go`:`mulix preset list [--all]/add <source> [--sha256]/remove <id> [--purge]/enable <id>/disable <id>/set-priority <id> <n>/info <id>/resolve <template-name>/search <query>`,`mulix preset catalog list/add/remove`。`add` 的 `<source>` 依次尝试:HTTPS/HTTP URL → 本地目录 → 内置/已配置 catalog 里的 preset id(查到后取其 `download_url`)。
- [x] 单测:`internal/preset` 下 5 个测试文件 —— manifest 解码(合法/未知字段/错误 schema/无模板/非法 strategy 全部拒绝)、registry 往返与排序(含 disabled 过滤)、三层覆盖解析(replace/prepend/append/wrap 各一个用例 + "高 priority replace 短路低 priority" 用例 + wrap 缺占位符报错)、本地目录安装(含复装保留 priority/enabled)、zip URL 安装(`httptest.Server` 起本地 zip 响应,含 GitHub 顶层包装目录、sha256 校验成功/失败、拒绝非 HTTPS)、catalog 解析与搜索、catalog ref 增删查(含 `FetchCatalog`)。`internal/cliutil/preset_cmd_test.go`:通过 `NewRootCmd()` + 重定向 `os.Stdout` 端到端跑 `preset add/list/info/resolve/enable/disable/set-priority/remove/catalog *` 全部子命令。
- [x] `assets/skills/mulix-taskstoissues/SKILL.md`:description 只写触发条件("tasks.md 存在且需要转换为 GitHub issue 时使用,不限阶段");正文移植 spec-kit 的去重规则(扫描 issue 标题里的 `\bT\d{3,}\b` 模式,用 `gh issue list --state all --json number,title --limit 100` 分页)、创建规则(`gh issue create --title "T001: ..."`)、remote 必须是 GitHub 的前置检查(`git config --get remote.origin.url` 校验)、"仅在匹配的仓库里创建" 的硬性红线、`gh auth status` 失败即停止而非猜测凭证。**设计决策**:用 `gh` CLI 而非 GitHub MCP server(用户已确认),对齐 mulix 零外部依赖定位;该技能不接入 `internal/flow` 状态机,是阶段无关的工具技能(对齐 spec-kit 本身把 taskstoissues 做成独立 slash command 而非流程步骤的设计)。
- [x] `README.md`:补充 Presets 与 Converting tasks to GitHub issues 两节说明与命令示例。
- [x] 验证:`go build ./...`、`go vet ./...`、`gofmt -l .`、`go test ./...`(`-count=1`)全绿,7 个包(新增 `preset`,`cliutil` 从零测试变为有测试)。手动 `.exe` 烟测被 Windows Defender 拦截(`go build` 产出的临时 `a.out.exe` 被誤报为病毒,与本次改动无关),改用 `go run ./cmd/mulix init --dir <tmp>` 验证 `mulix-taskstoissues` 技能随 `init` 正常安装;`preset add/list/resolve` 等子命令的端到端行为由上面新增的 `internal/cliutil/preset_cmd_test.go` 覆盖,弥补了手动烟测被 AV 拦截的验证缺口。

## Phase 6 — 产物模板与 spec-kit 对齐(扩展需求,已完成)

用户要求 `assets/templates/` 下的产物模板尽量和 spec-kit 的 `templates/*.md` 保持一致。原 mulix 模板是几十行的精简骨架(配合 `internal/guard` 只做“文件存在且非空”“包含 `## Clarifications`”“`- [ ]` 计数”等字符串级检查),spec-kit 的模板则是数百行、user-story 驱动、带大量占位符和示例注释的重量级模板。经询问用户,确认对齐程度为**几乎逐字对齐**:移植 spec-kit 模板的章节结构、字段命名、占位符和示例注释,仅替换 spec-kit 专属的机制性内容(`__SPECKIT_COMMAND_*__` 占位符、`[###-feature-name]` 分支约定等)为 mulix 的等价物(`specs/<change>/`、指向对应 SKILL.md),并保留 mulix guard 依赖的精确锚点不变。

### 逐文件对应

- `spec-template.md`:完整移植 spec-kit 版本(User Scenarios & Testing、按 P1/P2/P3 分优先级的 User Story 区块、Edge Cases、Functional Requirements 用 `FR-###` 编号、Key Entities、Success Criteria 用 `SC-###` 编号、Assumptions),额外保留 mulix 原有的 `## Out of Scope` 和 `## Clarifications`(guard 精确匹配 `"## Clarifications"` 字符串,一字不改)。
- `plan-template.md`:移植 spec-kit 的 Technical Context(Language/Version、Primary Dependencies 等字段表)、Constitution Check、Project Structure(Documentation + Source Code 两个代码块,含 Option 1/2 可选布局)、Complexity Tracking 表;保留 mulix 原有的 Approach/Design Notes/Risks/Test Strategy 章节(呼应 `mulix-plan` SKILL.md 里“按 brainstorm_track 成比例”的指导,而不是强制要求 spec-kit 的 research.md/data-model.md/quickstart.md 独立文件,因为 mulix state 里没有对应的独立字段)。
- `tasks-template.md`:移植 spec-kit 的 Setup → Foundational → User Story N(可选 Tests 小节)→ Polish 分阶段结构、`[P]`/`[Story]` 标记语义、Dependencies & Execution Order、Implementation Strategy(MVP First / Incremental Delivery);checkbox 语法保持精确的 `"- [ ]"`(guard 用 `strings.Count` 字面匹配,已用 `grep -c` 验证新模板里仍是标准语法未被破坏)。
- `constitution-template.md`:移植 spec-kit 的 `[PRINCIPLE_N_NAME]`/`[PRINCIPLE_N_DESCRIPTION]` 五原则占位结构 + 示例注释、`[SECTION_2/3_NAME]`、Governance、版本/日期落款行;保留 mulix 原有的 Non-Negotiables 和 Amendment History(spec-kit 没有独立的 amendment history 章节,是 mulix 原创,予以保留)。
- `analyze-template.md`:spec-kit 没有与 mulix `analyze` 阶段直接对应的模板文件(它的等价物是 `templates/commands/analyze.md` 这个命令定义,不是产物模板,还依赖 mulix 没有的 `.specify/extensions.yml` hook 机制),因此移植的是该命令**第 6 步定义的报告输出结构**(Findings 表格用 category-initial + 序号的稳定 ID、Severity 分级 CRITICAL/HIGH/MEDIUM/LOW、Coverage Summary 表、Metrics 小节),而不是命令本身的执行步骤。
- `checklist-template.md`:**未移植**。mulix 的 9 阶段流程里没有对应的 `/checklist` 命令或 SKILL.md,移植一个没有消费者的模板文件属于范围外的空转产物,故不引入。已在 README 里注明这一点,避免看起来像遗漏。

### 验证

- [x] `go build ./...`、`go vet ./...`、`gofmt -l .`、`go test ./... -count=1` 全绿(8 个包,84 个单测,与 Phase 5 完成时一致 —— 本阶段只改动模板内容,不涉及 Go 代码)。
- [x] 确认 guard 依赖的精确锚点未被破坏:`grep -n "## Clarifications" spec-template.md` 命中;`grep -c "^\- \[ \]" tasks-template.md` 返回非零且语法标准。
- [x] `internal/preset`/`internal/cliutil` 里引用 `"spec-template"` 等名字的测试全部使用合成/临时 fixture 内容(`fakeCoreFS`、`httptest` 临时文件),不读取 `assets/templates/` 真实文件,因此不受模板内容变化影响,测试全部保持通过。
- [x] `go run ./cmd/mulix init --dir <tmp>` 手动烟测:新模板正确写入 `.mulix/templates/`,行数从原先每个几十行涨到 49~206 行,符合“接近 spec-kit 体量”的预期。
- [x] `README.md`:新增 `## Templates` 一节,说明对齐程度、guard 依赖的两个精确锚点、以及未移植 `checklist-template.md` 的原因。

## Phase 7 — SKILL.md 与 spec-kit `templates/commands/*.md` 对齐(扩展需求,已完成)

用户要求 `assets/skills` 下的 SKILL.md 按 Phase 6 模板对齐时“几乎逐字对齐”的同等力度,同步对齐 spec-kit `templates/commands/*.md` 里对应斜杠命令的完整提示词定义(`analyze.md`/`checklist.md`/`clarify.md`/`constitution.md`/`converge.md`/`implement.md`/`plan.md`/`specify.md`/`tasks.md`/`taskstoissues.md`)。这批命令文件比 Phase 6 的模板更长,结构上包含 spec-kit 专属的 `.specify/extensions.yml` 扩展钩子(Pre/Post-Execution Hooks)、多脚本变体(bash/powershell/python)、`handoffs` 字段等 mulix 没有对应基础设施的机制性内容——对齐时移植的是每个命令的**执行步骤、检查清单、格式规则、严重度/分类标准**等实质内容,不引入 mulix 不存在的钩子/多脚本/handoffs 机制,这与 Phase 6 排除 `__SPECKIT_COMMAND_*__`占位符机制的处理方式一致。

### 逐文件对应

- `mulix-specify/SKILL.md` ← `specify.md`:移植 Quick Guidelines(WHAT/WHY 而非 HOW、写给非技术读者)、执行流程(生成短名 → 提取概念 → 标记 NEEDS CLARIFICATION 的三条件判断 → 填 User Scenarios → 生成可测试的 Functional Requirements → 定义 Success Criteria → 识别 Key Entities)、Success Criteria 的好/坏示例对比、"合理默认值不必询问"清单。不移植 spec-kit 自动生成 `checklists/requirements.md` 质量清单的步骤(mulix 无 `/checklist` 消费者,与 Phase 6 排除 `checklist-template.md` 的理由一致)。
- `mulix-clarify/SKILL.md` ← `clarify.md`:移植完整的模糊性扫描分类法(Functional Scope/Domain & Data Model/Interaction & UX/Non-Functional/Integration/Edge Cases/Constraints/Terminology/Completion Signals/Placeholders 十类,每类标 Clear/Partial/Missing)、最多 5 问的排队与逐题询问规则(每题给推荐选项、接受后立即写回 `## Clarifications` 并同步修改对应章节,不是先记录问答再批量改)、停止条件。
- `mulix-plan/SKILL.md` ← `plan.md`:移植 Technical Context 字段填写要求、Phase 0(消解 NEEDS CLARIFICATION,但要求 mulix 版本必须当场解决而非留给人类,因为 mulix state 没有 spec-kit 的 research.md 中间产物)、Phase 1(实体/接口契约抽取,按项目是否有对外接口决定是否需要),以及 Constitution Check 门禁。
- `mulix-tasks/SKILL.md` ← `tasks.md`:移植严格 checklist 格式定义(`- [ ] T001 [P] [US1] 描述`五要素及正确/错误示例)、Setup → Foundational → User Story N → Polish 的阶段结构与归类规则(实体/契约映射到对应 story,跨 story 共享的放 Setup)。
- `mulix-analyze/SKILL.md` ← `analyze.md`:移植六类检测(Duplication/Ambiguity/Underspecification/Constitution Alignment/Coverage Gaps/Inconsistency)、四级严重度启发式(CRITICAL/HIGH/MEDIUM/LOW 的判定标准)、报告结构(Findings 表 + Coverage Summary 表 + Metrics)。
- `mulix-build/SKILL.md` ← `implement.md`:在已移植 superpowers TDD 铁律的基础上,补充 implement.md 的"先读 tasks.md+plan.md+spec.md 再动手"、按 tasks.md 阶段顺序逐阶段执行(`[P]` 任务才并行)、进度汇报与失败处理规则(非并行任务失败即停,`[P]` 任务失败只报告不阻塞其他并行任务)。
- `mulix-verify/SKILL.md` ← `implement.md` 第 9 步(spec-kit 没有独立 verify 命令,验证步骤内嵌在 implement.md 末尾):补充"确认实现真正匹配 spec.md 需求和 plan.md 方案,而非只看 tasks.md 是否勾完"这一条,呼应 spec-kit"勾选是声明,不是证据"的验证精神。
- `mulix-taskstoissues/SKILL.md` ← `taskstoissues.md`:已有结构与 spec-kit 高度一致(去重正则、创建规则、GitHub remote 前置检查均已对齐),本次补充 spec-kit 原文里的强调格式(`> [!CAUTION]` 风格的 STOP/UNDER NO CIRCUMSTANCES 警示块),原有正文内容不变。
- `mulix-brainstorm`/`mulix-archive`/`mulix-using-mulix`:**未改动**。这三个是 mulix 原创(brainstorm 三档流程移植自 superpowers、archive 的显式确认菜单和 using-mulix 元技能均无 spec-kit 对应物——spec-kit 没有 brainstorm/archive 阶段,也没有跨阶段的元技能命令),`grep -l -i "archive\|brainstorm" templates/commands/*.md` 确认零匹配。
- `checklist`/`constitution`/`converge` 对应的斜杠命令:**未移植为新 skill**。mulix 没有 `/checklist` 阶段(同 Phase 6);`constitution` 由 `mulix init` 的 scaffold 直接写入 `.mulix/memory/constitution.md`,不是一个独立阶段技能;`converge`(codebase-vs-spec 差距扫描并追加任务)是 spec-kit 较新的命令,mulix 当前 9 阶段流程里没有对应的"事后补任务"步骤,引入属于范围外的新功能而非对齐现有产物,故不做。

### 验证

- [x] 逐条核对 8 个改写的 SKILL.md 里引用的 `mulix state transition <event>` 事件名(`spec-complete`/`clarify-complete`/`clarify-skipped`/`plan-complete`/`tasks-complete`/`analyze-complete`/`analyze-skipped`/`build-complete`/`verify-pass`/`verify-fail`/`archived`/`brainstorm-approved`)与 `internal/flow/flow.go` 里的 `Event` 常量逐一比对,全部一致,未引入不存在的事件名。
- [x] `go build ./...`、`go test ./... -count=1` 全绿(8 个包,85 个单测,与 Phase 6 完成时一致——本阶段只改动 skill 文档正文,不涉及 Go 代码)。
- [x] `grep -rln` 确认 `internal/scaffold/init_test.go` 只断言 SKILL.md 的路径/存在性,不断言正文内容,不受本次文字改动影响。

## Phase 8 — 移除 brainstorm 独立阶段,技巧下沉到 specify/build(已完成)

用户澄清:comet/spec-kit/superpowers 三个来源项目不应被 mulix 直接依赖或在正文里点名引用,产物和阶段数量不新增;把 superpowers 的 brainstorm+TDD 完整嵌入 `mulix-build`,`mulix-brainstorm` 本身移除,其技巧(spike/bounded/architectural 三档分类 + 单向升级的 ratchet 规则)改为下沉到 `mulix-specify`(整个 change 粒度,写 spec.md 前)和 `mulix-build`(单个 task 粒度,写第一个测试前)两处。用户明确这是**真正从流程中删除 brainstorm 阶段**,包括 Go 状态机代码和测试,不是只删 SKILL.md 文件。

设计决策(已与用户确认):
- **命名限制范围**:仅 `assets/skills/**/SKILL.md` 正文、Go 代码注释禁止出现 "spec-kit"/"comet"/"superpowers" 等来源项目名;`docs/PLAN.md`、`README.md` 属于面向开发者/维护者的内部文档,可以保留来源归属说明(本节和 README 的"发想来源"部分因此保留点名,SKILL.md 和 Go 注释则已清理)。
- **build 阶段的 brainstorm 技巧粒度**:比 change 级别的三档分类更细,是**按 task 分档**——每个任务开工前先判断是 spike-sized(直接写测试)、bounded(先看一眼相关代码定approach)还是 architectural(涉及 plan.md 没定的设计决策,需要先摆出选项和理由,必要时报告人类),同样遵循单向升级的 ratchet 规则。

### 改动范围

**`internal/flow`**(状态机层面的真删除):
- `flow.go`:`Phase` 常量表去掉 `PhaseBrainstorm`;`Phases` 切片从 9 个减到 8 个;`Event` 常量表去掉 `EventBrainstormApproved`;`Table` 去掉 brainstorm→specify 的转移项;包级注释里去掉"comet 的 classic-state/classic-transitions 移植"措辞。
- `state.go`:`State` 结构体去掉 `BrainstormTrack`/`BrainstormNote` 字段(没有阶段消费,保留是死字段);`New()` 的初始阶段从 `PhaseBrainstorm` 改为 `PhaseSpecify`。
- `flow_test.go`:`TestApply_LegalTransitionAdvancesPhase` 改为测 `spec-complete`(specify→clarify)而不是 `brainstorm-approved`;`TestFind_UnknownEventFromPhase` 改用 `EventSpecComplete` 作为"archive 阶段查不到的事件"探针。

**`internal/guard`**:去掉 `checkBrainstormApproved` 及其在 `checksByEvent` 里的注册项;包级注释去掉"comet 的 classic-guard.ts 移植"措辞;`Result.Next` 字段注释去掉"mirroring comet's Next hint"措辞。

**`internal/hook`**:`allowedPrefixes` 去掉 `PhaseBrainstorm` 分支,原本"brainstorm 阶段可写 docs/"的设计文档例外挪到 `PhaseSpecify`(因为架构级设计文档现在是 specify 阶段三档分类里 architectural 档的产物);包级注释和 `changeDirFor` 的注释去掉"comet 的 classic-hook-guard.ts 移植"/"superpowers convention" 措辞。

**`internal/cliutil`**:`new.go` 的命令简介从"starts in the brainstorm phase"改为"starts in the specify phase";`state_cmd.go` 去掉 `brainstorm_track` 字段的 `set` 支持(合法值列表 `spike|bounded|architectural`)及其在 `Long` 帮助文本里的列举——三档分类现在是 specify/build 两个 skill 正文里的思考纪律,不是状态机字段。

**`internal/hook/hook_test.go`**:`TestDecide_BrainstormPhaseBlocksSourceWrite`/`TestDecide_BrainstormPhaseAllowsSpecDirWrite` 改名并改用 `PhaseSpecify`;其余用到 `PhaseBrainstorm` 的测试同步改为 `PhaseSpecify`。

**`internal/state/state_test.go`**:两个 YAML fixture 里的 `phase: brainstorm` 改为 `phase: specify`(这两个测试本身只断言"未知字段/schema 不符即拒绝",phase 取值不影响断言,改动只是为了不再引用已删除的阶段名)。

**技能层**:
- 删除 `assets/skills/mulix-brainstorm/` 整个目录。
- `mulix-specify/SKILL.md`:开头新增"先分类再动手"章节,移植原 `mulix-brainstorm` 的三档分类定义(spike/bounded/architectural)+ ratchet 单向升级规则,分类结果不再写入 state 字段,只是流程纪律;写 spec.md 的动作移到分类+审批之后。description 同步改为反映新职责。
- `mulix-build/SKILL.md`:Part 1 新增"按 task 分档"小节(spike-sized/bounded/architectural,同一套 ratchet 规则但粒度是单个 task);Part 1/Part 2 的小标题去掉"(spec-kit implement)"/"(superpowers test-driven-development)" 来源括注。
- `mulix-plan/SKILL.md`:两处引用已删除的 `brainstorm_track` 字段改为"how the change was classified during specify"这类不点名字段的措辞;"design doc written during brainstorm"改为"written during specify"。
- `mulix-using-mulix/SKILL.md`:阶段计数从"nine-phase"改为"eight-phase",流程图去掉 `brainstorm →`;技能清单去掉 `mulix-brainstorm`;"starts in the brainstorm phase"改为"starts in the specify phase";红旗清单里"brainstorm and archive both require explicit confirmation"改为不点名阶段的措辞。
- `mulix-taskstoissues/SKILL.md`:去掉"This mirrors spec-kit's taskstoissues command"这句点名引用,改为直接描述该技能是阶段无关的独立工具。
- `assets/templates/spec-template.md`/`plan-template.md`/`constitution-template.md`:去掉对 `brainstorm_track` 字段和 "written during brainstorm" 的引用(改为"分类结果见 specify 阶段"一类措辞);`plan-template.md` 里 `docs/superpowers/specs/` 这个直接点名来源项目路径的引用改为 `docs/`。

**文档层**(README.md,保留来源归属,仅更新工作流描述使其与代码一致):
- 顶部发想来源列表:superpowers 一行改为反映"三档分类和 TDD 现在都直接嵌进 `mulix-specify`/`mulix-build`,不再是独立的 `mulix-brainstorm` 技能"。
- `## The workflow` 代码块从 9 阶段改为 8 阶段(去掉 `brainstorm →`),新增一段说明"没有独立 brainstorm 阶段,分类纪律嵌在 specify 开头和 build 每个 task 开工前两处"。
- `mulix new` 命令示例注释"starts in the brainstorm phase"改为"starts in the specify phase"。
- `## Templates` 一节里关于 SKILL.md 对应关系的说明,`mulix-build` 单独列出"合并 spec-kit implement + superpowers TDD + 按 task 分档"的说明,`mulix-brainstorm` 从"无 spec-kit 对应物,未改动"的列举里去掉(因为它已不存在)。

### 验证

- [x] `go build ./...`、`go vet ./...`、`gofmt -l .`、`go test ./... -count=1` 全绿。
- [x] `grep -rniI "spec-kit|superpower|\bcomet\b" assets/skills/*/SKILL.md assets/templates/*.md` 零匹配,确认来源项目名已从 agent 可见的技能/模板正文清空。
- [x] `grep -rn "mulix-brainstorm" --include="*.go" .` 零匹配,确认 Go 代码里没有硬编码引用已删除的技能名(技能目录是按目录扫描安装的,不需要额外改注册表)。

未做(明确排除,与用户澄清一致):不引入新的产物文件或新的阶段节点;不把 `brainstorm_track` 之类字段保留为"没有阶段消费的死字段"——分类结果只作为对话过程中的思考纪律,不落盘。

## Phase 9 — 产物目录拆分为 docs/specs + changes,状态迁移到按变更 .runtime(已完成)

用户报告两个问题:1) "产物结果目录"需要调整(经追问澄清为具体的目录拆分方案);2) 归档后 `.mulix/active` 未被清除,导致 `mulix state show`(不带 `--change`)在变更归档后仍然静默指向一个已经结束的变更,而不是报错提示需要选择新变更。

### 问题 2:归档未清理 active 标记

`internal/flow` 的 `EventArchived` 转移本身只置位 `Archived = true`,从未涉及 active-change 追踪;`internal/cliutil/new.go` 里 `mulix new` 会调用 `state.SetActive`,但没有任何代码路径在归档时反向调用清除。修复:

- `internal/state/root.go` 新增 `ClearActive(root)`(删除 `.mulix/active`,文件不存在不算错误)和 `ActiveIs(root, change)`(判断某 change 是否是当前 active,未设置时返回 false 而非报错)。
- `internal/cliutil/state_cmd.go` 的 `newStateTransitionCmd`:转移成功后,如果新状态是 `phase == archive && Archived == true` 且被转移的 change 正好是当前 active change,调用 `ClearActive` 并打印提示。只在"确实是 active 才清"是为了不影响"转移一个非 active 的 change"这种通过 `--change` 显式指定的场景。
- 新增测试:`internal/state/root_test.go` 覆盖 `ClearActive`/`ActiveIs`;`internal/cliutil/state_cmd_test.go`(新文件)通过完整 CLI 命令路径覆盖"归档 active change 后标记被清除"和"归档非 active change 不影响现有 active 标记"两种场景。

### 问题 1:产物目录拆分

经多轮追问确认最终方案——**spec.md(需求文档)与其余产物(过程文档)分属两个目录**,理由是 spec 意在归档后仍长期作为需求参考,其余是执行过程中的工作产物,语义上不同:

```
docs/specs/<NNN-slug>/
└── spec.md                  # 需求文档(specify/clarify 阶段读写)

docs/changes/<NNN-slug>/
├── plan.md                  # plan 阶段
├── tasks.md                 # tasks 阶段
├── analyze.md                # analyze 阶段
├── report.md                # verify 阶段(原 verify-report.md,同步改名)
└── .runtime/
    └── state.yaml            # 该 change 的运行时状态(原 .mulix/state/<change>.yaml)

.mulix/
├── active                    # 项目级共享:当前激活 change(未拆分,用户明确选择保留共享)
├── templates/                # 项目级共享:模板
└── memory/                   # 项目级共享:constitution.md
```

用户明确排除的两个选项:1) 把 `.mulix` 整体改名为 `.runtime`;2) 把 `active`/`templates`/`memory` 也拆到每个 change 目录下——只有 state 按 change 拆分,其余共享内容仍在项目根 `.mulix/` 下,命名保持不变。

> **注**:实现过程中最初把 `changes/<change>/` 放在项目根下(与 `docs/specs/` 平级),用户在实现中途指出应统一放在 `docs/` 下,遂改为 `docs/changes/<change>/`,即最终方案里写的路径。下文的路径引用已按最终方案改写,不再是最初的中间版本。

`verify_report`/`VerifyReport` 字段同步改名为 `report_path`/`ReportPath`(用户确认文件名和字段名一并改,不只改目录里的文件名),因为改名后 `verify_report` 会变成全字段里唯一一个名字不跟文件名对齐的(其余都是 `<name>_path`)。

### 改动范围

**`internal/scaffold`**:
- `newchange.go`:`SpecsDir` 常量值从 `"specs"` 改为 `"docs/specs"`;新增 `ChangesDir = "docs/changes"` 和 `RuntimeDirName = ".runtime"`。`NextNumber` 改为扫描 `ChangesDir` 而不是 `SpecsDir`(change 目录从 `mulix new` 那一刻就存在,spec 目录要等 spec.md 写完才有内容意义上的"存在",用 docs/changes/ 编号更稳)。`CreateChangeDir` 改为同时创建两个目录;`NewChange` 结构体的 `Dir`/`AbsDir` 拆成 `SpecDir`/`SpecAbsDir` + `ChangeDir`/`ChangeAbsDir`。
- `init.go`:去掉之前 `Init` 里无条件创建 `.mulix/state/` 目录并计入 `Written` 的那段——state 目录现在是按 change 分散在 `docs/changes/<change>/.runtime/` 下,由 `state.Save` 按需创建,不再有一个项目级共享的 state 目录需要 `init` 预建。

**`internal/state`**:
- `state.go`:去掉 `Dir` 常量(不再是扁平目录);`PathFor` 改为拼 `docs/changes/<change>/.runtime/state.yaml`。为避免 `state`↔`scaffold` 循环 import,`changesDir`/`runtimeDirName` 在这里各自重复定义了一份字符串常量,而不是 import scaffold 的对应常量。
- `root.go`:`List` 原来是扫 `.mulix/state/*.yaml` 拿文件名当 change id,现在改为扫 `docs/changes/` 下的目录、检查每个目录里是否存在 `.runtime/state.yaml`。

**`internal/flow`**:`state.go` 的 `VerifyReport string \`yaml:"verify_report,omitempty"\`` 改名为 `ReportPath string \`yaml:"report_path,omitempty"\``;`State` 结构体的路径注释同步更新为 `docs/changes/<change-id>/.runtime/state.yaml`。

**`internal/guard`**:`checksByEvent` 里 `EventVerifyPass` 对应的 `checkArtifactPresent` 闭包从读 `s.VerifyReport` 改为读 `s.ReportPath`,检查名 `"verification-report-present"` 不变(它是检查项名字,不是文件名,不受改名影响)。

**`internal/hook`**:这是本次改动里语义最复杂的一块,因为原来"一个阶段对应一个可写目录"的模型建立在 spec.md 和 plan.md/tasks.md 是同一目录下兄�手文件的假设上,拆分后不再成立:
- `stateGuardPrefix`(原来的固定字符串 `.mulix/state`)改为按 change 动态拼 `docs/changes/<change>/.runtime`。
- `allowedPrefixes` 按阶段区分该写哪个目录:specify 阶段写 `docs/specs/<change>`(spec.md 还没写完时)+ `docs`(架构级设计文档);clarify 阶段只写 `docs/specs/<change>`(clarify 直接改 spec.md);plan/tasks/analyze/verify 阶段写 `docs/changes/<change>`;build 不受限;archive 禁止一切写入。
- 原来的单一 `changeDirFor` 拆成两个函数:`specDirFor`(优先读 `s.SpecPath` 所在目录,没有则 fallback 到 `docs/specs/<change>`)和 `changeDirFor`(优先读 `s.PlanPath` 所在目录,没有则 fallback 到 `docs/changes/<change>`)。
- 发现并修复一个由拆分引入的权限漏洞:specify 阶段的 `docs` 允许项(为架构级设计文档开的口子)如果不做排除,会连带放行 `docs/specs/<其他 change>/`,因为它也在 `docs` 前缀之下——这正好是拆分前 `docs/` 和 `specs/` 是两个不相交顶层目录时不会出现的问题。修复:遍历 `allowed` 列表时,`docs` 这一项特殊处理,排除掉 `docs/specs` 子树(设计文档应该直接放在 `docs/` 下,不应该嵌进 `docs/specs/`)。

**`internal/cliutil`**:
- `new.go`:命令简介和创建成功后的打印语句改为同时报告 `SpecDir` 和 `ChangeDir` 两个路径。
- `state_cmd.go`:见"问题 2"一节的 `ClearActive` 接线;另外 `state show` 的输出增加 `report_path:` 一行,`set` 命令的字段允许列表和 `case` 分支把 `verify_report` 改成 `report_path`。

**测试**:`internal/scaffold/newchange_test.go`、`internal/scaffold/init_test.go`、`internal/state/state_test.go`、`internal/state/root_test.go`(新增)、`internal/guard/guard_test.go`、`internal/hook/hook_test.go`(含新增的 plan/clarify 阶段可写目录测试)、`internal/cliutil/state_cmd_test.go`(新文件)按上述改动逐一同步。

**技能层**(`assets/skills/mulix-specify|plan|tasks|analyze|verify|taskstoissues|using-mulix/SKILL.md`):写入路径从 `specs/<change>/` 改为按产物类型分别指向 `docs/specs/<change>/spec.md` 或 `docs/changes/<change>/{plan,tasks,analyze,report}.md`;`verify_report` 字段名同步改为 `report_path`,文件名从 `verify-report.md` 改为 `report.md`;`.mulix/state/<change>.yaml` 的引用改为 `docs/changes/<change>/.runtime/state.yaml`。

**模板层**(`assets/templates/plan-template.md`/`tasks-template.md`):输入来源路径和"本变更的文档产物"目录树同步改为两目录结构。

**文档层**(README.md,保留来源归属不变,仅更新与代码一致的目录描述):`## The workflow` 之后新增一段关于目录拆分的说明;`mulix init` 一节去掉"写入 state 目录"的描述,补充"不创建 docs/specs 或 changes,那些由 mulix new 按需创建"。

### 验证

- [x] `go build ./...`、`go vet ./...`、`gofmt -l .`、`go test ./... -count=1` 全绿。
- [x] `grep -rn '\`specs/\|"specs/\| specs/<\|specs/\[###'` 排除 `docs/specs` 后,`assets/` 下零残留旧路径引用。
- [x] 手动验证:在 `examples/todo-cli` 里移除历史遗留的 `.mulix/active`(该 change 早已 `archived: true` 但 active 标记未清除,是本次修复动机的真实复现),确认 `mulix state show`(不带 `--change`)之后正确报错提示"no active change set",带 `--change` 显式指定仍正常工作。

未做:没有重新生成 `examples/todo-cli` 里已完成的历史产物文件（`specs/001-...` 目录及其内容）来匹配新目录结构——旧例子是在旧目录约定下跑完的，视为该次运行的历史记录，不做迁移；后续如果需要在新目录约定下重新验证完整流程，需要重新跑一遍 example（本 Phase 未包含在范围内，用户未提出该要求）。
