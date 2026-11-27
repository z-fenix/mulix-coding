# 通用技能、Superpowers 升级与 Karate 测试接入：可行性方案

**日期：2026-11-27 · 本文记录设计基线和实施前的 JAR 实测；实施见[开发计划](../plans/2026-11-27-generic-skills-karate-verification.md)。**

**结论：可行。** 从默认安装中移出项目专属工作台适配，保留原生验收门禁；将内嵌 Superpowers 升至官方 `v7.0.2`；在每个变更下增加 `.mulix/.runtime/<change>/tests/`，只保存用例和自动化代码。**任务阶段只做定向验证，最终验收在当前版本上全量执行。** 首版用指定 JAR 的 `run` 执行 `.feature`，以逐场景 JSONL 接入 `mulix test run` 和 `mulix verify`。报告和滚动日志分别写入会话临时目录；JAR 不放进 `tests/`，也不打进 Go 二进制。

## 1. 技能审核与去项目化

已审阅 `assets/skills/` 下全部 **9** 个 `SKILL.md` 的 frontmatter 和正文，以及 Verify 随附的参考、脚本、测试和相关默认模板。9 个 `description` 都没有写死项目路径，保留 `name` 和触发条件，不把目录布局、执行步骤或工具版本写进 `description`。

| 技能 | 发现与拟调整 |
|---|---|
| [`mulix-verify`](../../../assets/skills/mulix-verify/SKILL.md#L56) | 默认正文仍示范 `--workbench-root ../test`，并随包安装特定证据协议。移除该命令和引用；改为：若项目宪法声明附加验收门禁，按项目声明执行，未完成项进入 `mulix-review.blocking`。默认只使用原生验收链，不猜测外部目录。 |
| [`mulix-archive`](../../../assets/skills/mulix-archive/SKILL.md#L8) | 将对 Verify 内置工作台补丁的依赖，改为复核项目明确声明的门禁，以及当前原生验证回执。保留 `tests/` 源文件，不补造历史证据。 |
| [`mulix-specify`](../../../assets/skills/mulix-specify/SKILL.md#L37) | 默认采用本变更的 FR → AC；规格记录适用的验收基线、范围和阻断。项目有独立验收来源时引用其原始 ID，不把正式准入、索引或工作台作为通用前置条件。 |
| [`mulix-tasks`](../../../assets/skills/mulix-tasks/SKILL.md#L35) | 默认完成 AC → TC → suite / test / file 映射。只在项目实际要求时附原始用例和检查点；保留覆盖要求、任务承接、实现与验收状态区分。 |
| [`mulix-using`](../../../assets/skills/mulix-using/SKILL.md#L80)、[`mulix-build`](../../../assets/skills/mulix-build/SKILL.md#L66) | 未发现这些项目专属引用。同步新增 `tests/` 的归属、写入阶段和运行方式；现有 SDD 台账、TDD 和证据规则保留。 |
| [`mulix-design`](../../../assets/skills/mulix-design/SKILL.md)、[`mulix-clarify`](../../../assets/skills/mulix-clarify/SKILL.md)、[`mulix-taskstoissues`](../../../assets/skills/mulix-taskstoissues/SKILL.md) | 未发现这些项目专属引用，保持不变。 |

**不是只改文案。** 原有 [`acceptance-gate.md`](../../../compat/formal-workbench/references/acceptance-gate.md#L5) 记载了这套专用布局；[`check-acceptance.py`](../../../compat/formal-workbench/scripts/check-acceptance.py#L169) 实际校验同级 `../pm/Product-Spec.md`、`完整测试点与测试用例.md`、`requirements/case/**/cases.json` 和工作台 `reports/` / `runs/`。默认安装会递归打包技能目录与模板（[`managed.go`](../../../internal/scaffold/managed.go#L35)）。实施时一并移出默认包：

- Verify 的专用参考、校验器及测试，以及 `acceptance-contract-template.json`、`acceptance-results-template.json`。有真实依赖的项目，把这些文件迁为**项目自管**，通过项目宪法显式引用；既不在通用包里改名重发，也不取消这些项目的门禁。
- 同步泛化 `constitution-template.md`、`spec-template.md`、`tasks-template.md` 的验收来源和示例；保留规范、测试和审阅的唯一 `mulix-acceptance`、`mulix-tests`、`mulix-review` 协议，以及真实执行、基线、覆盖和阻断要求。
- 更新[安装资产断言](../../../internal/scaffold/formal_acceptance_assets_test.go#L13)、[P1 必跑项](../../../scripts/quality-policy.json)和 [Python 测试发现范围](../../../scripts/quality.py#L129)。默认资产不再安装专用文件，不等于可以删除测试范围后把空跑算成功；仓库继续维护的迁移校验应在非内嵌测试目录运行。

## 2. 内嵌 Superpowers 升级

- **已核实版本**：实施前的仓库内嵌版本为 `7.0.0`；2026-11-27 [上游官方最新发布](https://github.com/obra/superpowers/releases/latest) 重定向到 [`v7.0.2`](https://github.com/obra/superpowers/releases/tag/v7.0.2)，标签解引用提交为 `22f0b463e7a5e579904dd6211a520321035526f7`。核对过该提交的源码，仍有 15 个技能。
- **已复现阻断**：取上述提交，在临时目录运行现有同步脚本。第一个 `brainstorming/scripts/start-server.sh` 精确补丁报 `patch matched 0 times (want 1)`。上游在创建会话目录前新增了 `--project-dir` 的创建和规范化逻辑，旧的整段匹配不再成立。未在工作区运行同步脚本，也未验证后续补丁全部可用。
- **升级做法**：固定标签、提交和许可证。让 [`sync-superpowers.sh`](../../../scripts/sync-superpowers.sh#L33) 先在独立暂存目录生成候选，逐个重定位补丁并保持唯一匹配；失败不碰现有 `assets/superpowers/`。不靠调宽匹配或只改 `VERSION` 通过。重复生成应得到相同文件清单和内容，确认后再替换内嵌资源、重建 CLI、走安全 `mulix update`。
- **必须保留的适配**：两种宿主仍安装不带命名空间的技能；设计和任务阶段在 Mulix 审批处结束；计划仍为 `tasks.md` / `## Task N`；两种执行方式保留 SDD 台账和状态同步；临时输出仍经会话解析进入 scratch。验证新版不重新引入 `.superpowers` 项目路径、自动执行交接或工作区删除。

## 3. Karate 测试目录、运行器与证据

### 目录与生命周期

```text
.mulix/.runtime/<change>/
├── state.yaml             # CLI 写入状态和执行回执
├── specs/                 # 已批准的设计（按现有规则创建）
├── tests/                 # 仅版本化的用例和自动化代码
│   ├── features/          # 可执行 .feature 入口
│   │   ├── task-<N>/      # 只归属任务 N；最终验收也会重跑
│   │   └── final/         # 仅最终验收执行的跨任务用例（按需）
│   └── helpers/           # 复用 .feature / .js，不作为入口
└── sdd/                   # 现有原生执行记录，规则不变
```

在 [`layout`](../../../internal/layout/layout.go#L36) 定义 `TestsDir`，由 [`mulix new`](../../../internal/scaffold/newchange.go#L54) 创建所属变更的 `tests/`；旧变更在 build 采用时按需创建，不批量迁移或改 ID。`tasks` 只批准用例路径、预期和配置；`build` 写用例及自动化代码；`verify` 和 `archive` 只读。扩展 [`runtimeWritable`](../../../internal/hook/hook.go#L71) 只放行 build 下本变更的测试源码，不给整个 `.runtime` 开写权限，并沿用对其他变更、链接及归档变更的检查。这个钩子仍不是文件系统沙箱。

`tests/` 源码可随变更入库，`init/update` 不把它当受管资源覆盖。JAR、机器环境配置值、凭证、运行临时文件、原始事件、HTML/XML、日志和截图均不放进 `tests/`。适配器用当前会话的 `<scratch-dir>/verification/<run-id>/` 保存私有暂存副本和运行输出；`state.yaml` 和既有任务/最终报告只留回执、脱敏摘要与哈希。不改现有 SDD 原生日志的保留规则，也不新建一套永久报告目录。同步更新 [README 的目录与权限约定](../../../README.md#L49)和 [`quality-template.md`](../../../assets/templates/quality-template.md)。

### 已核实的 JAR 能力与边界

指定产物 `karate-agent-2.1.5.RC1.jar` 的 Maven 标识为 `io.karatelabs.ext:karate-agent:2.1.5.RC1`，大小 `64,056,608` 字节（约 61.1 MiB），SHA-256 为 `9e5fc19c560fa6824aae07a52ec8ca11542ecf6e93bd9f823b039477525cf9e3`。基础 CLI 探测覆盖 Linux 上的 Java 21.0.12.1 和 Java 27；后续 Mock、API、编排和日志实测使用 Linux / Java 21.0.12.1，仅连接隔离环境的回环地址，未访问外网或真实业务服务。

| 探测 | 实测结果 / 接入约束 |
|---|---|
| `run` + `mock` | 同一 JAR 启动本地 Mock，执行 2 个 `.feature`，3 个顶层场景全部通过。覆盖 HTTP 200/404、JSON 断言、`Scenario Outline` 展开、`karate-config.js` 环境配置、`.feature` / `.js` 助手复用，以及中文场景名和带空格路径。 |
| 机器可读报告 | 同次运行生成 HTML、JUnit XML、Cucumber JSON 和 Karate JSONL。JSONL 含 `SCENARIO_ENTER`、`SCENARIO_EXIT` 和 `SUITE_EXIT`；顶层与被调用的助手分别记录为 `data.callDepth=0` 和大于 `0`。断言失败的样例退出 `1` 并产生 `ERROR.data.type=AssertionError`。 |
| dry-run / 零场景 / 配置错误 | dry-run 和零场景都可能退出 `0`；dry-run 同时有 `passed=true`、`skipped=true`，JUnit XML 样例甚至显示 `skipped=0`。配置失败可能产生 Cucumber 失败项，但没有场景进入/退出事件。三者都不能作通过证据，也不是行为 RED。 |
| 工作目录与日志 | 仅设 `-w` 而进程 cwd 不同，相对入口可能选中零场景；cwd 与 `-w` 对齐并提供绝对入口已验证。`-o` 只控制报告：默认仍在 cwd 下生成 `target/karate-agent.log`。只改变 `KARATE_LOG_FILE` 指向临时目录后，API 套件仍是 3/3 通过，用例目录无新增文件，源码哈希不变。 |
| `.karate.js` 编排 | 用 `run` 传入带故意抛错的脚本，实测退出 `0` 且零场景。用 `launch` 则执行脚本，可调用 `Runner.run`；子场景失败但脚本不抛错时仍退出 `0`，脚本传播失败才退出 `1`。仅证明指定制品可运行该编排，不代表其他发行包的授权条件。 |
| 其他入口 | 顶层帮助列出 `run`、`mock`、`clean`、`serve`、`launch`。`serve` 的 `/api/eval` 和 `/api/mcp` 仅从命令帮助确认，未联调；浏览器 UI 和清理命令未实跑。帮助未暴露独立的 `mcp` 或 `case-live` 子命令。 |

### 任务定向验证与最终全量验收

| 环节 | 执行范围 | 放行依据 |
|---|---|---|
| 任务编排（`tasks`） | 批准任务用例、最终专跑用例和套件配置；不在本阶段执行。 | 同一 `mulix-tests` 矩阵和 `quality.yaml`，不新增第二套验收计划。 |
| 任务定向验证（`build`） | `mulix test run --task N` 只选择任务 N 映射的套件；Karate 只把 N 的 `.feature` 入口传给 JAR。代码任务执行 `--stage red` 再执行 `--stage green`，非代码任务用有明确理由的 `--stage check`；未映射 Karate 的任务不启动 JAR。 | 当前任务的执行回执及完成台账，不要求任务级全量验收或单独签批。 |
| 最终全量验收（`verify`） | 在最终受审版本上统一由 `mulix verify` 跑**所有必跑套件**。Karate 用同一个固定 JAR 执行全部 `features/task-<N>/`、`features/final/` 入口；已有 Go/JSON 套件照常执行。 | 全部必跑场景及套件通过、审阅通过，并生成绑定当前实现和报告的最终回执。任务 GREEN 不可替代这一轮。 |
| 阶段切换与归档 | `build-complete`、`verify-pass` 和归档只核验相应回执，不额外启动测试。最终回执因源码、测试、工具、配置或报告等已绑定输入变化而失效时，按现有流程重新 `mulix verify`。 | 相应回执仍有效；没有有效回执就不放行，不复用过期通过结果。 |

- **按文件划分任务范围。** `features/task-<N>/` 中的入口只归属任务 N，不在同一个 `.feature` 混放多个任务。通用动作和数据准备放 `helpers/`，不作为入口或独立计数。任务验证从批准的用例映射提取该任务的入口列表，不先跑全量再过滤结果。现有 Go/JSON runner 仍按任务映射的套件执行；需要更窄范围时配置对应套件，而不是为本次接入修改其执行协议。
- **跨任务必跑场景必须明确列出。** 现有 `cases` 继续用 `task` 关联任务。最终专跑用例建议用同一 `mulix-tests` 块内的可选 `verify_cases` 清单登记 TC ID、关联 AC、suite、file 和精确场景名；不分配任务、不参与任务级选择，但最终必须有实际通过的回执。它只补充回归，不能替代现有任务和 AC 的可执行映射。未采用该清单的旧计划仍可读。
- **关闭任务不重复执行。** 当前 [`task-done`](../../../scripts/superpowers-overrides/executing-plans/scripts/task-done#L29) 会执行传入的命令。无论采用 inline 还是 subagent-driven，同一任务在输入不变且已有有效回执时不再跑 GREEN/CHECK。inline 可由 `task-done` 触发唯一一次 GREEN/CHECK；若前一步已经执行，收尾应只核验回执并记录到台账。RED 仍在实现前独立执行。更新任务模板及执行器适配，避免把执行和收尾误写成两次。整体审阅也不另起一次最终全量执行。

任务通过后在最终版本重跑同一用例是为捕捉后续改动引入的回归。重复的是在相同输入下为同一任务再跑一次 GREEN/CHECK，或在最终验收前后机械追加另一轮全量。

### 推荐接入方式

**首版用 `run` + Karate JSONL 接入现有 quality runner。** 只把 `.feature` 作为用例入口，普通 `.js` 和可复用 `.feature` 放在 `helpers/`。不依赖 `launch`、`serve`、MCP 或旧工作台。`mock` 可用于隔离回归，不是生产环境验收的替代。

| 路线 | 取舍 |
|---|---|
| **在现有 quality runner 增加 Karate 适配器（推荐）** | 统一校验会话、路径、输入指纹和回执；无需另建工作台或报告流程。 |
| 直接在 `quality.yaml` 配 `java -jar`，或先加项目脚本 | 原生 runner 只解析 `go-json` 或逐行 `name/status` JSON，不读取 Karate 文件报告；当前指纹与合同也排除 `.runtime`。包装脚本可用于验证接口，不能独立满足目录和验收要求。 |
| JAR 随 Go 二进制或发布包安装 | 方便离线取用，但会扩大单文件包或改变六平台交付清单；需要另行确认 JAR 再分发许可，且仍需 Java。不是首版前提。 |

**接口草案（仅供实施评审）**：项目在现有 `.mulix/quality.yaml` 显式声明一个 `format: karate-jsonl` suite，配置固定版本、预期 SHA-256 和超时。CLI 从本地预置路径（例如 `MULIX_KARATE_JAR` 指向的绝对路径）取 JAR；缺 Java、缺 JAR 或哈希不符明确失败。普通项目不默认添加该 suite，也无需 Java；声明后仍是必跑，不因空目录隐式跳过。CI 从受控制品来源预置同一校验值的文件，运行时不自动联网下载或升级。

1. 适配器在启动 JAR 前根据调用阶段生成入口清单：任务模式从 N 对应的 Karate `cases` 取去重后的文件；最终模式枚举两个入口分组并覆盖 Karate 的 `cases` 与 `verify_cases`。缺失入口、归属不符或把助手当入口都拒绝，实际执行范围写入回执。`file` 存项目相对路径，`test` 用 `该路径#精确场景名`；Scenario Outline 使用展开后唯一名称，不以行号或共享标签替代身份。
2. 适配器从当前有效会话固定变更和输入，校验真实路径及文件类型，把用例、助手和已核验的 JAR 暂存到本次私有执行目录。进程 cwd 和 Karate `-w` 都指向测试副本；使用绝对入口路径、受控配置目录、`--no-pom` 和 `-f ~html,karate:jsonl`。`-o` 指向本次 scratch 的报告子目录，另固定 `KARATE_LOG_FILE` 为 `<scratch-dir>/verification/<run-id>/karate-agent.log`。使用参数数组，不继承覆盖执行选项或日志路径的环境变量，不自动启动 `serve` 或连接外部报告服务。
3. **用本次 `karate-json/karate-events.jsonl` 的明细判定。** 要求完整 `SUITE_ENTER` / `SUITE_EXIT` 与场景进入/退出配对；只把 `data.callDepth=0` 的场景按规范化文件路径和精确名称匹配已批准 TC。助手事件不单独计数，但影响顶层场景的失败仍计入。先判 `skipped`，再看 `passed`；核对汇总时考虑 Karate 的 pass 可能包含 skip。入口无有效场景、事件缺失/重复/截断、映射场景缺失/跳过、执行异常均阻断。RED 只认可已执行的映射断言失败及一致的非零退出；配置失败、超时或未知错误类型不计 RED。GREEN/verify 要求退出 `0`、suite 有实际通过且无失败、所有映射场景均实际通过。控制台汇总、展示型报告和退出码均不能代替逐场景检查。
4. **输入要独立绑定**：[`LoadContract`](../../../internal/quality/contract.go#L264) 和 [`Fingerprint`](../../../internal/quality/fingerprint.go#L159) 当前都拒绝映射到 `.runtime`。只为当前变更的合法 `tests/` 源文件增加例外，不把 `state.yaml`、`specs/`、`sdd/` 或其他变更纳入。任务 RED/GREEN 绑定同一用例及依赖、政策、JAR 和执行环境身份，仍要求实际实现变化；全量 verify 绑定全部入口及可执行助手、JAR/JVM 身份、实际项目输入和最终报告。在运行前、运行后/落盘、放行时复核相应输入。保留 CLI 对失败或中断重试使旧回执失效的机制。

## 4. 实施顺序、验收与回退

| 批次 | 实施范围 | 放行条件 |
|---|---|---|
| 1. 去项目化 | 先写默认安装回归，再迁出专用资产并改通用技能、模板；给依赖方迁移说明 | Claude Code、DeepSeek Harness 的全新安装均无专用路径/文件；普通项目只靠原生 AC/TC 可完成流程。旧项目的定制和既有证据保留。 |
| 2. 刷新内嵌资源 | 重新定位 `v7.0.2` 补丁，校对生成物，验证两种宿主和旧资源更新 | 固定上游提交可重现，资源/覆盖层一致，阶段审批、会话 scratch、SDD 恢复及升级冲突流程通过。 |
| 3. 接入并资格验证 | `layout` / `new` / hook → 合同、作用范围和指纹 → Karate 适配器及任务收尾 → CLI 端到端与发布门禁 | 任务做定向 RED→GREEN/CHECK，最终用同一固定 JAR 全量验收。阶段切换及归档不重复跑。P1 用实际候选二进制跑支持平台的原生矩阵，不以 Linux 本机探测替代。 |

**Karate 接入验收**：

- **证明任务定向与最终全量。** 构造两个 Karate 任务各有独立入口，另有一个仅最终运行的跨任务用例；用本次 CLI/JAR 调用和场景事件证明每个任务只执行自己的入口，未映射 Karate 的任务不启动 JAR，最终验收覆盖两组任务入口和 `final/`。让跨任务用例失败，确认即使任务均通过也不能归档。
- **证明收尾不重跑。** 任务已有有效 GREEN/CHECK 回执时，记录完成、阶段切换和归档不能新增测试执行；无有效回执不得放行。更改已绑定的最终输入后旧全量回执失效，重新验收才可放行。
- **场景映射**：用隔离 Mock 重跑 2 个入口、3 个顶层场景的 API 样例，含 Outline、中文名称和带空格路径。`.feature` 助手的嵌套事件不计为第 4 个用例；助手代码仍参与输入指纹。
- **拒绝误放行**：实际映射断言失败可记 RED；配置失败、超时、零用例、dry-run、重复/截断事件、缺失或跳过的映射用例不能伪造通过或 RED。入口校验拒绝 `.karate.js` 编排脚本；`launch` 的退出码不作通过证据。
- **隔离和新鲜度**：执行前后比较 `tests/` 文件清单及源码哈希；`-o` 的报告和 `KARATE_LOG_FILE` 的滚动日志都在本次 scratch，源码目录不落 `target/karate-agent.log`。验证换 JAR、改用例或助手后旧回执失效，跨变更或会话以及路径越界均不放行。

**兼容及回退**：已批准的旧变更不自动迁用例或重写计划/回执。`quality.yaml` 是项目级配置，待活动变更交付后再为新变更启用 Karate；若要对活动变更启用，先重新审阅相关测试矩阵并取证。依赖旧工作台的项目先迁出规则和校验器再更新；现有 [`update` 策略](../../../internal/scaffold/update.go#L312) 只自动移除有可信基线且未改的退役文件并留备份，定制/未知文件须人工核对。分批交付保留上一版 CLI、资源基线和项目迁移备份；回退不删除 `tests/` 或历史记录，不把新旧输入不一致的回执用作放行。

**正式使用前置**：确定 CI 的 JAR 预置来源、Java 版本和支持平台；真实服务用例由项目提供受控环境、数据和执行授权。本文的 JAR 探测属于实施前可行性证据，不构成业务验收或跨平台发布资格；复验方法见[仓库质量指南](../../quality.md)。
