# 通用技能、Superpowers 与 Karate 两级验证实施计划

## Context

已批准的[可行性方案](../specs/2026-11-27-generic-skills-karate-feasibility.md)要求去除默认包的特定工作台约定、刷新内嵌上游，并在现有质量门禁下实现任务定向验证与最终全量验收。真实 JAR 探测已确认 RC1 的 JSONL、空跑及独立滚动日志行为；本计划不重新选择执行架构。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the plan task-by-task in this session. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 默认安装不带项目专属规则，固定新版 Superpowers，且一个受控 Karate runner 支持任务定向和最终全量。

**Architecture:** 保留 `mulix test run`、`mulix verify`、阶段守卫和状态回执。`internal/quality` 负责测试输入范围、Java 子进程及结构化事件；资源生成仍用现有同步覆盖层；外部工作台迁为可选项目自管资料。

**Tech Stack:** Go 1.27.3、Bash/Perl、Python 3.14.8；Karate JAR 在 Java 21 原生验证。

**Spec:** `docs/superpowers/specs/2026-11-27-generic-skills-karate-feasibility.md`

## Global Constraints

- 固定 `superpowers v7.0.2` / `22f0b463e7a5e579904dd6211a520321035526f7`；保持精确补丁、许可证、双宿主和 SDD 记录约定。
- 本次制品 `karate-agent-2.1.5.RC1.jar`，期望 SHA-256 为 `9e5fc19c560fa6824aae07a52ec8ca11542ecf6e93bd9f823b039477525cf9e3`；JAR 由可信本地路径提供，不编入 CLI、不上传、不自动下载。
- `.mulix/.runtime/<change>/tests/` 只含用例和辅助源码；报告与 `KARATE_LOG_FILE` 使用一次性会话 scratch。原生 AC/TC、P1 和安全更新照常关闭失败。不要提交、推送或发布。

## Review Focus

- 本地定制/未知退役文件不得在默认更新中丢失；双宿主的新安装不可安装兼容适配器（任务 1）。
- 模拟上游补丁失配时现有内嵌内容必须逐字节保留（任务 2）。
- 非拥有变更、非法文件类型或链接不能成为本变更可执行入口；新增 tests 源码必须独立哈希（任务 3）。
- 空跑、dry-run、配置失败及嵌套场景不能成为 GREEN 或行为 RED（任务 4）。
- 任务均绿仍不得掩盖最终专跑用例失败；收尾及归档不重复启动 JAR，变更后拒绝旧回执（任务 5）。

---

### Task 1: 通用默认安装与可选兼容资料

**Files:** `assets/skills/mulix-{specify,tasks,verify,archive}/SKILL.md`，三个默认 Markdown 模板；迁出 Verify 专用资源及两个 JSON 模板到 `compat/formal-workbench/`；`internal/scaffold/formal_acceptance_assets_test.go`、`scripts/quality.py`、`scripts/tests/test_quality.py`、P1 策略及迁移文档。

**Interfaces:** 默认安装仍由 `managedFiles` 递归写资产；旧兼容校验器保持 CLI 输入与证据格式，Python discovery 两个独立非空范围。

- [ ] **Step 1: 写默认安装回归。** 在 `TestInstalledProjectFormalAcceptanceAssets` 真实运行 Init 验证两个宿主均不安装项目专属脚本/模板，没有越界默认工作台路径，仍由 `quality.Block` 解析原生 fences；升级测试验证未改退役文件移除、定制保留。
- [ ] **Step 2: 验证 RED。** `go test ./internal/scaffold -run TestInstalledProjectFormalAcceptanceAssets -count=1` 应因仍安装专属资产失败。
- [ ] **Step 3: 迁出资源并泛化技能和模板。** 兼容目录不参与 Go embed，保留项目需要的校验器和回归；P1 Python scope 改为 `compat/formal-workbench/tests`，测试和说明随之迁移。
- [ ] **Step 4: 验证 GREEN。** 同一 Go 测试及 `python3 -I -B scripts/quality.py python-tests --output <新临时文件>` 通过，两套 Python 测试均非空。

### Task 2: 固定上游并安全生成

**Files:** `scripts/sync-superpowers.sh`、`assets/superpowers/`（生成）、`internal/scaffold/session_scripts_test.go`、README。

**Interfaces:** `bash scripts/sync-superpowers.sh <上游目录>`；脚本先在独立候选目录完成精确补丁、覆盖与漂移扫描，再替换目的地；保留许可证和版本。

- [ ] **Step 1: 写生成与失败保全测试。** 提供 `MULIX_SUPERPOWERS_SOURCE` 指向固定源码；用现有 `TestVendorPatches_ResolveContextInsteadOfReadingActive` 的真实流程证明会话 scratch 和工作区；为二次生成文件一致、故意补丁漂移不改旧输出增加断言。
- [ ] **Step 2: 验证 RED。** 当前脚本对 `v7.0.2` 的精确补丁失败，或者证明失败时存在不完整目的地。
- [ ] **Step 3: 重新定位补丁并生成。** 保留明确的 phase handoff、`tasks.md` / `## Task N`、状态同步与台账。以锁定提交作为发布输入；失败不覆盖旧内容；Linux 实测同源可重复生成。
- [ ] **Step 4: 验证 GREEN。** `MULIX_SUPERPOWERS_SOURCE=<固定源码> go test ./internal/scaffold -run 'TestVendorPatches|Test.*(Scratch|SessionScripts|Ledger|StepCheckbox)' -count=1`，另检查覆盖层字节一致。

### Task 3: 运行目录、用例映射与指纹

**Files:** `internal/layout/layout.go`、`internal/scaffold/newchange.go`、`internal/hook/hook.go`、`internal/quality/{contract,fingerprint}.go`；新增 `internal/quality/karate_scope.go` 及同层测试。

**Interfaces:** `layout.TestsDir`；`Suite.Karate` 持 `version` / `sha256`（可配已审批 `environment`）；`Contract.VerifyCases` 支持同一个 fence 的 `verify_cases`；`SelectedCases(0)` 纳入最终专跑 TC；`karateInputs(p Project,c Contract,s Suite,task int)` 解析严格、物理的入口/助手清单。

- [ ] **Step 1: 写路径和合同测试。** 新 change 创建 tests；只有 owning build 下 `features/task-N`、`features/final` 的 `.feature` 与 `helpers` 下源码可写。任务 1 取本身入口，task 0 取所有入口；旧映射仍有效。拒绝其他变更、错误任务归属、测试以外的 runtime、重复 ID、链接和 Go/JSON 映射到 Karate runtime。
- [ ] **Step 2: 验证 RED。** 跑 `go test ./internal/scaffold ./internal/hook ./internal/quality -run 'Test.*(Karate|RuntimeTests)' -count=1`；旧 Hook/LoadContract/LoadPolicy 应拒绝新合法输入。
- [ ] **Step 3: 实现结构与快照。** 保持 `excluded` 对 workflow Source 的排除；任务快照仅收自身入口及助手；最终覆盖所有入口；SHA-256 包含路径与内容，不把生成物记为实现变化。
- [ ] **Step 4: 验证 GREEN。** 同一命令、`go test ./internal/quality ./internal/guard`，变更助手/最终入口使相应快照失效，新增不相关任务入口不使早期任务的快照失效。

### Task 4: 受控 JAR 执行与 JSONL 门禁

**Files:** `internal/quality/karate_runner.go`、`karate_events.go`、`runner.go`、`fingerprint.go`、`evidence.go` 及 Go 回归测试。

**Interfaces:** `MULIX_KARATE_JAR` 指向绝对本地路径，可用 `MULIX_KARATE_JAVA` 显式选择本地 JVM。原生现有 Go/JSON argv 不变。`SuiteResult` 保留机器验收状态并记录稳定入口和执行器指纹，不用每次临时路径做策略相等比对。

- [ ] **Step 1: 写真实边界测试。** 使用 RC1 对照事件和本地子进程夹具：三顶层场景与嵌套助手、行为断言失败、`passed=true/skipped=true`、零、缺/重复/截断事件、配置失败及超时；记录命令入参、日志路径和报告输出。
- [ ] **Step 2: 验证 RED。** 同一真实 runner 的 `go test ./internal/quality -run Test.*Karate -count=1` 不接受现有未支持格式的期望成功样例。
- [ ] **Step 3: 实现 adapter。** 校验 JAR 内容和 Java 身份，用现有 `exec.CommandContext`、16 MiB 输出限制和 RunRecord 生命周期。在私有 scratch 暂存入口、辅助源码、固定 JAR；明示 `--no-pom`、绝对入口、`-w`、`-o`、`KARATE_LOG_FILE`，禁止继承影响选项/路径的变量。只接受配对后的顶层 JSONL；结构化 `AssertionError` 匹配到映射场景后才可算行为 RED。verify 记录本次最终输入及回执。
- [ ] **Step 4: 验证 GREEN。** 跑本任务测试，检查先验证版本/哈希再执行、JAR 变化或配置漂移使证据失效、原生 Go/JSON 行为不变。

### Task 5: 两级 CLI、无重跑的任务收尾及交付回归

**Files:** `internal/cliutil/quality_cmd.go` 和 CLI 测试、`internal/quality/evidence.go`、`assets/skills/mulix-{build,tasks,verify,archive}/SKILL.md`/模板，`scripts/sync-superpowers.sh` 的 inline 说明补丁及生成物，`scripts/smoke-karate.sh`、README/P1 策略。

**Interfaces:** 用 `resolveCommandContext` 检查归属，新增只读 `mulix test receipt --task N` 验证当前 GREEN/CHECK 及必要的历史 RED；将其交给 inline `task-done`，而不是再启动同一个 `test run`。`build-complete`、`verify-pass`、`archived` 沿用现有守卫。先前各任务源码允许随之后的实现前进，最终仍与当前源/报告等输入绑定。

- [ ] **Step 1: 写 CLI 流程回归。** 任务 1/2 只调用各自入口；非 Karate 任务不启动；跨任务 TC 只在最终执行；有效回执只读不启动程序；全部任务绿、最终失败则不放行；修改 JAR/源码/报告时重新核验证据。
- [ ] **Step 2: 验证 RED。** `go test ./internal/cliutil -run 'TestQualityCLI.*(Karate|Receipt)' -count=1` 应因未实现检查/收尾而失败。
- [ ] **Step 3: 贯通流程并更新文档。** 复用 `ValidateTaskEvidence`、`Run`、`replaceRun` 和 `ValidateVerification`，避免新状态、额外审批或第二套报告；两个执行方式不为收尾重复 GREEN/CHECK。更新生成覆盖，准备使用已安装候选 CLI 和指定 JAR 的隔离烟测。
- [ ] **Step 4: 最终验证。** 跑 `go test -count=1 ./...`、示例模块测试、Python 双范围、`go vet`、Shell 语法/同步与 `git diff --check`。在临时项目用实际构建的候选 CLI、Java 21 和指定 JAR 执行定向 RED→GREEN、最终失败/通过、无重复归档。尽量运行现有 Linux P1 native 资格链；无制品源或无原生宿主的矩阵不可报告为通过，不发布。
