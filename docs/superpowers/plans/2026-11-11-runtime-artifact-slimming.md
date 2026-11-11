# Runtime Artifact Slimming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将持久运行产物收敛为每个 change 下的 `specs/`、`sdd/`，将一次性材料移到系统临时目录，不损坏状态、会话隔离或构建证据。

**Architecture:** 保留 `.mulix/.runtime/<change>/state.yaml` 和现有 SDD 消费接口。增加由有效上下文推导、不会自行创建文件的 `scratch-dir` 查询，脚本与 hook 共享此路径，避免全局临时目录放行。通过现有 superpowers 同步脚本维护嵌入脚本和文档改写，不引入新依赖、状态迁移或清理后台任务。

**Tech Stack:** Go、现有 Cobra CLI、Bash、Node.js visual companion、Go 测试及现有 smoke 脚本。

**Spec:** `docs/superpowers/specs/2026-11-11-runtime-artifact-slimming-scope.md`（已在对话批准的范围记录）。

## Global Constraints

- 保留按 change 隔离的 `.mulix/.runtime/<change>/`。
- 每个 change 只保留 `specs/`、`sdd/` 两类运行产物目录。
- `state.yaml` 是必要控制状态，保留原位，不计作日志；根级 `.runtime/.gitignore` 也保留。
- 最终验证报告仍位于 `docs/changes/<change>/report.md`。
- 保持现有进度格式、历史兼容、RED/GREEN 报告要求、构建门禁和 SDD 工作区保留行为，不照搬上游结束后删除整个工作区的动作。
- 不在初始化或更新时静默删除旧项目的历史文件；保留防止旧 brainstorm token 等被意外提交的忽略规则。
- 不扩大 hook 的工具覆盖范围：仍仅约束 Write/Edit，不把它描述为安全沙箱。
- 未获授权前不提交、不推送、不删除现有运行记录；不处理已有未跟踪的 `dist.zip`。

## Review Focus

1. 损坏绑定、缺失状态和无当前 change 必须是三种不同结果，不能因错误创建临时回退产物；Task 1、2 添加对应测试。
2. 多个项目、多个会话绑定同一 change、全局切换以及不同 change 的临时材料不能串用；Task 1 路径隔离测试、Task 2 服务探针测试覆盖。
3. 自定义临时目录含空格、Windows/Git Bash 路径形式和目录前缀碰撞不得扩大写入范围；Task 1 hook 表驱动测试、Task 2 脚本测试覆盖。
4. Visual companion 的 PID、token、端口和 mockup 必须共同落入私有临时路径；既有符号链接不得被当作新 scratch 目录写入；Task 2 覆盖。
5. 升级旧项目不能删除历史材料、暴露旧 token、改写旧 ledger 或重跑已完成任务；Task 3 以历史快照和现有恢复测试覆盖。

## 文件与职责

| 文件组 | 职责 |
| --- | --- |
| `internal/sessionctx/{scratch.go,types.go,resolve.go}` | 派生临时路径，并与一次成功的上下文解析绑定 |
| `internal/cliutil/context_cmd.go` | 输出 `scratch-dir`；无 change 不再返回 `_shared` runtime |
| `internal/hook/{hook.go,context.go}`、`internal/layout/layout.go` | 将持久目录和临时写入权限分离，保留旧路径忽略/保留名用途 |
| `scripts/sync-superpowers.sh`、相应 `assets/superpowers/skills/` 文件 | 可重放的脚本和诊断产物路径改写 |
| `assets/skills/mulix-{using,design,build,verify,archive}/SKILL.md`、`README.md` | 精简后的产物合同与使用说明 |
| 对应 Go 测试、`scripts/smoke-{e2e,sessions}.sh` | 上下文、阶段、脚本、安装和历史兼容回归 |

## 实施选择（待计划审阅确认）

- 新增 `Resolved.ScratchDir string`，JSON 名为 `scratch_dir`；新增 `mulix context show --field scratch-dir`。不新增持久控制文件。
- 内部函数 `scratchDir(root, change, sessionID string) string` 返回 `filepath.Join(os.TempDir(), "mulix-scratch-" + digest)`；digest 为清理后的绝对项目根、change、会话 ID 以 NUL 分隔后所得完整 SHA-256 小写十六进制值。Windows 根路径按现有大小写不敏感规则规范化。未提供会话 ID 时使用空串。
- `RuntimeDir` 仅在有 change 时有值。无 change 的 `context show --json` 成功并输出 `runtime_dir: ""` 和有效 `scratch_dir`；单独查询 `--field runtime-dir` 返回非零并提示先创建/选择 change。错误上下文不返回可用 scratch 路径。
- 有状态时，持久 runtime 仅允许 design 写 `specs/`、build 写 `sdd/`。Scratch 中 design 可写 `brainstorm/`，build/verify 可写 `verification/`、`migrations/`；`diagnosing-superpowers/` 各阶段可写，承接旧 `_shared` 的临时诊断能力，但仅限当前上下文的 scratch 根。
- 无状态时保持普通项目写入的既有允许行为，但拒绝 Write/Edit 写 `.mulix/.runtime/`；CLI 的初始化和状态维护不受这个工具门禁影响。错误处理仍遵循现有 legacy/session 模式规则。
- 新 scratch 路径不会在解析/查询时创建；消费脚本以私有权限按需创建。保留旧 runtime 忽略规则及 `_shared` 保留名，不因停止生成而放开旧敏感文件或把 `_shared` 当成 change。
- 不新增 SDD 文件名级 hook 过滤器，避免冻结上游协议；由执行脚本的标准输出合同、技能指令和安装回归约束额外留档。

---

### Task 1: 派生隔离的 scratch 上下文并收敛门禁

**Files:**
- Create: `internal/sessionctx/scratch.go`, `internal/sessionctx/scratch_test.go`
- Modify: `internal/sessionctx/types.go`, `internal/sessionctx/resolve.go`, `internal/sessionctx/resolve_test.go`
- Modify: `internal/cliutil/context_cmd.go`, `internal/cliutil/context_cmd_test.go`
- Modify: `internal/hook/hook.go`, `internal/hook/context.go`, `internal/hook/hook_test.go`, `internal/hook/context_test.go`
- Modify: `internal/layout/layout.go`（更新常量含义，不移除历史忽略/保留名所需常量）

**Interfaces:**
- Consumes: `Store.Resolve(context.Context, ResolveRequest) (Resolved, error)` 的既有根、change、有效 Identity 和错误语义。
- Produces: `Resolved.ScratchDir string`、`scratchDir(root, change, sessionID string) string`、CLI `--field scratch-dir`，以及上述持久/临时权限矩阵。后续脚本只消费 CLI，不自行重算 hash。

- [X] **Step 1: 添加失败测试**

新增 `TestScratchDir_IsStableAndContextScoped`，断言结果绝对、同输入稳定、切换根/change/session 任意一项均不同、位于测试设置的系统临时根，且查询后路径仍不存在。新增 Windows 规范化用例，仅在 Windows 检查项目根大小写变化结果相同。

更新 `TestContextCLI_ShowFormatsAndEmptyContext` 和现有无 change Resolve 断言：

```go
// 有效空上下文的 JSON/Resolve 合同；字段查询另外断言非零及无路径 stdout。
if got.RuntimeDir != "" || got.State != nil || got.ScratchDir == "" {
    t.Fatalf("unexpected empty context: %+v", got)
}
```

新增 `TestDecideContext_ScratchAndNoChangePolicy`，表驱动精确覆盖：design/specs 允许、design/旧 brainstorm 拒绝、build/sdd 允许、build 与 verify 的旧 verification/migrations 拒绝；当前 scratch 的对应临时目录允许；其他 change/session 的 scratch、同名前缀兄弟目录及错阶段目录拒绝；诊断目录各阶段允许；无状态时 runtime 拒绝而普通源码允许。保留 state.yaml、跨 change 和 archived 写入拒绝测试。

- [X] **Step 2: 运行确认 RED**

Run: `go test ./internal/sessionctx ./internal/hook ./internal/cliutil`
Expected: 新字段/函数未定义或旧 `_shared`/旧目录仍允许导致明确失败；不以无关环境错误作为 RED。

- [X] **Step 3: 实现接口和权限矩阵**

按实施选择添加派生函数；仅成功解析后设置 ScratchDir；清空无 change 的 RuntimeDir。CLI 增加 scratch 字段并对缺 change 的 runtime 字段报错。`DecideContext` 在无状态普通放行前保护 runtime，并在有状态时验证当前 scratch 根及阶段子目录；复用既有路径规范化和分段边界规则，不放行整个系统临时目录。必须在 build 的普通路径无限制分支之前识别系统临时根下的保留命名空间 `mulix-scratch-<64位小写十六进制>/`：非当前上下文目录直接拒绝，不能将其当普通项目外路径放行。移除 `decideRuntime` 的 `_shared` 特权并更新提示。不要改状态读写、锁、选择或归档流程。

- [X] **Step 4: 运行确认 GREEN**

Run: `go test ./internal/sessionctx ./internal/hook ./internal/cliutil`
Expected: 全部 PASS，包括坏绑定不回退、Windows 别名和 legacy 错误行为的原有测试。

- [X] **Step 5: 检查变更边界**

Run: `git diff --check`
Expected: 无空白错误；未提交，状态/会话存储布局未变。

### Task 2: 将可视化和诊断材料改为临时输出

**Files:**
- Modify: `scripts/sync-superpowers.sh`
- Modify: `assets/superpowers/skills/brainstorming/scripts/{start-server.sh,stop-server.sh}`
- Modify: `assets/superpowers/skills/brainstorming/visual-companion.md`
- Modify: `assets/superpowers/skills/diagnosing-superpowers/{SKILL.md,templates/case.md,templates/report.md}`
- Modify: `internal/scaffold/session_scripts_test.go`

**Interfaces:**
- Consumes: Task 1 的 `mulix context show --field scratch-dir`，成功时输出绝对路径，坏上下文时非零且无落盘。
- Produces: 服务启动 JSON 的 screen/state 路径、`BRAINSTORM_DIR`、`BRAINSTORM_PORT_FILE`、`BRAINSTORM_TOKEN_FILE` 均在当前 scratch 的 `brainstorm/` 下；诊断材料在 `scratch-dir/diagnosing-superpowers/<session-id>/`。

- [X] **Step 1: 添加失败的脚本探针测试**

将 `TestSessionScripts_GlobalSwitchOnlyAffectsNewInvocation` 改为使用真实 Resolve 返回的 ScratchDir 断言：首个服务仍指向 X 的原 scratch，切换到 Y 仅影响下一次调用，绑定 A 使用其自己的 scratch。

新增 `TestSessionScripts_BrainstormUsesPrivateScratchOnly` 和 `TestSessionScripts_BrainstormRejectsSymlinkScratch`：设置含空格的临时根；检查 Dir/Port/Token 均在预期 `brainstorm/` 下、不同实例会话目录不同、runtimeFiles 前后相同；POSIX 检查目录 owner-only；POSIX symlink 用例须拒绝且不向链接目标写文件，无法创建 symlink 的平台明确 Skip。保留坏绑定不创建任何产物用例，并检查 scratch 也没有新增文件。无 change 的有效上下文可启动临时 companion。

- [X] **Step 2: 运行确认 RED**

Run: `go test ./internal/scaffold -run '^TestSessionScripts_' -count=1`
Expected: 现有服务仍写 runtime/brainstorm 或查询 runtime-dir 失败，使新断言 FAIL。

- [X] **Step 3: 修改可重放的同步改写和嵌入产物**

修改 sync 的 exact-match patch：`--project-dir` 先成功查询 scratch-dir，再创建私有 `brainstorm/` 父目录，以 `mktemp -d` 创建实例目录；保留 `umask 077`，复用目录时拒绝符号链接和非当前用户所有的目录并确保私有权限。保留父目录中的端口/token 复用和逐实例 PID/server-id 行为。Git Bash 继续通过 cygpath 转换查询路径，不回退到项目 runtime。无 project 参数使用同样的系统临时原则。

将原 `~/.superpowers/` 诊断路径改写为 `<scratch-dir>/`，在诊断技能说明如何查询真实绝对路径、私有创建目录以及查询失败即停。替换 visual-companion 的 runtime 持久化说明和查找方式，以启动 JSON/已捕获路径为准。stop 脚本不再声称新会话持久存于 runtime；保持现有 PID 身份校验，不在本任务扩大递归删除范围或删除历史持久目录。

所有嵌入修改必须由同步脚本重放，保留现有 mulix progress overlays；不要只改 assets 留待下次同步丢失。

- [X] **Step 4: 运行确认 GREEN 并检查同步**

Run: `go test ./internal/scaffold -run '^TestSessionScripts_' -count=1`
Expected: 全部可运行测试 PASS；Node/Bash 不可用时明确记录 Skip，不当作服务已验证。

若本机原版 superpowers 7.0.0 可用，运行 `bash scripts/sync-superpowers.sh /home/zhang/.claude/plugins/cache/claude-plugins-official/superpowers/7.0.0`，再次运行相同测试，并比较同步前后 assets 内容摘要，要求无新增差异；不可用则记录这项未执行，不联网获取源码。

- [X] **Step 5: 检查变更边界**

Run: `git diff --check`
Expected: 无空白错误；没有新增 `.runtime` 目录、没有删除旧产物、没有修改 progress overlays 或提交。

### Task 3: 固化 SDD 产物合同、更新说明并全量回归

**Files:**
- Create: `internal/scaffold/runtime_artifacts_test.go`
- Modify: `assets/skills/mulix-{using,design,build,verify,archive}/SKILL.md`
- Modify: `README.md`, `scripts/smoke-e2e.sh`, `scripts/smoke-sessions.sh`
- Modify as needed: `internal/scaffold/{init_test.go,update_test.go,ledger_test.go}`（只增加此变更相关回归）

**Interfaces:**
- Consumes: 前两任务的 runtime/scratch 合同，以及原有 `sdd-workspace`、`task-brief`、`task-start`、`task-done`、`review-package` 脚本和 ledger fixture。
- Produces: 两种 host 安装的技能遵守同一产物规则；新执行只留下标准 SDD 文件，旧数据和历史 ledger 不被重写。

- [X] **Step 1: 添加产物策略与兼容回归测试**

对 HostClaude、HostDSH 使用下面的真实安装/脚本行为测试覆盖相同合同，不增加只检查 SKILL.md 源码字符串的测试。技能引用行为先用六场景只读消费者记录旧路径选择作为 RED，再验证更新后的消费者会选择查询 scratch、保留标准 SDD 记录并在坏绑定时停止；文档整体在最终独立审查中复核。

新增 `TestRuntimeArtifactPolicy_StandardSddFilesOnly`：在临时 Git/CLI fixture 中运行真实执行辅助脚本及 review-package，断言工作区文件集合恰为 `plan-path`、`progress.md`、`task-1-brief.md`、`task-1-report.md`（由 fixture 按真实报告合同写入）、`task-1-tests.log` 和脚本实际打印的一个 `review-*.diff`；`.gitignore` 位于 sdd 层。当前 change 顶层不出现 specs/sdd 之外的新目录，state.yaml 不变。不把额外的测试辅助输出写进工作区。

新增 `TestRuntimeArtifactPolicy_UpdatePreservesLegacyRuntime`：预置旧 brainstorm token、verification/migrations 文件、_shared 记录和旧 ledger，Init/Update 后逐字节不变；旧 token 仍被原 ignore 规则匹配。保留并运行 ledger 身份、完成记录、RED/GREEN 门禁及续跑测试。

- [X] **Step 2: 运行确认 RED**

Run: `go test ./internal/scaffold -run '^TestRuntimeArtifactPolicy_' -count=1`
Expected: 标准脚本合同和历史保护属于既有行为的保护性回归，应通过。新文档行为的 RED 来自改写前的消费者：其选择了 runtime/brainstorm、runtime/verification 和 `_shared`；不以会随措辞改变的源码字符串断言制造失败。

- [X] **Step 3: 更新技能、说明和 smoke 合同**

在 mulix-using 与 mulix-build 写入相同标题的精简产物规则；固定日志只由现有标准脚本按原命名生成，其余 RED/调试/重跑输出放 scratch/verification，不额外生成 evidence JSON、二进制或自定义日志目录；将必要命令结果摘要写回现有 progress/report，不能虚构或丢失证据。design 指向临时 companion，verify 使用 scratch/verification 并保持正式报告路径，archive 不再要求保留临时 companion。

README 更新工作流、目录职责、context 字段/no-change 行为、临时材料和历史兼容说明；明确不自动清理旧目录。保留 runtimeGitignore 的旧敏感目录规则和 seeded-once/update 行为。smoke-e2e 改为拒绝新 runtime/brainstorm 写入，旧 token 的忽略检查标为历史兼容；smoke-sessions 验证无 change 的 JSON 成功、runtime 字段查询失败、scratch 字段成功，不再期待 `_shared`。

- [X] **Step 4: 全量验证**

Run: `go test ./...`
Expected: 全部 PASS。

Run: `go vet ./...`
Expected: 无诊断。

Run: `bash scripts/smoke-e2e.sh && bash scripts/smoke-sessions.sh && bash scripts/smoke-update.sh`
Expected: 三项成功；如环境缺少所需程序，说明具体阻塞，不把未运行结果记为通过。

Run: `git diff --check`
Expected: 无空白错误。精确检查 SDD 原始文件种类、progress 格式和门禁未退化，不为了消除路径词汇而移除历史保护。

- [X] **Step 5: 收尾检查，不自动提交**

检查 diff 仅含计划范围内代码、测试、说明及范围/计划文档；`dist.zip` 保持原样。汇报测试、环境 Skip、临时路径接口、兼容边界和实际偏离。执行完成后按 verification-before-completion 和所选执行技能进行最终独立审查；未获用户授权前不提交、推送或清除历史文件。
