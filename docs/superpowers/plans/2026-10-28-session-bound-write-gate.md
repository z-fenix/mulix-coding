# Session-Bound Write Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 同目录多会话各自绑定 change；未绑定时动态使用全局 active，绑定失效时明确失败、不回退。

**Architecture:** 新增 `sessionctx` 统一解析、存储与选择上下文，保留每个 change 的既有阶段状态。短上下文锁与每 change 锁分离，CLI、hook、运行产物脚本共用解析结果；宿主自动身份传播以真实契约验证为前置条件。

**Tech Stack:** Go 1.27.0、现有 Cobra/yaml.v3、标准库 JSON/SHA-256/文件操作、Windows LockFileEx 与 Unix flock、Bash/Git Bash；本计划默认不新增 Go 模块依赖。

**Spec:** [已批准设计](../specs/2026-10-28-session-bound-write-gate-design.md)，2026-10-28 获用户批准。

## Global Constraints

- “阶段仍属于 change，不复制为会话级状态。”不改变 `mulix.state.v2`、七阶段或 guard 的证据含义。
- “未绑定会话动态回退到项目全局 `.mulix/active`；不在首次读取或写入时自动生成绑定。”
- “只有绑定记录不存在才回退；绑定损坏、读取失败、目标不存在、状态无法加载或目标已归档时，不改用其他 change。”
- “文件不存在等同 `legacy`；新 init 和已有项目升级都不自动启用 session。”配置存在但无效不能降为 legacy。
- “CLI 使用显式 `--session-id`，其次使用专用环境变量 `MULIX_SESSION_ID`。”hook 身份只来自本次请求。
- “不允许持有上下文锁等待 change 锁。”同时需要时顺序为 change → context；每次调用固定一个上下文快照。
- “普通 `mulix update` 继续不编辑 `.claude/settings.json`”；通过显式 init 安装新增 hook，DSH-only 不新增 Claude 配置。
- “SessionEnd 不自动删除，首版不做 TTL 自动清理。”归档保留绑定，解绑不删除 change。
- “不把 Bash、MCP、NotebookEdit 等工具加入本轮自动写入检查。”不是安全沙箱，不承诺源码并行编辑安全。
- Windows 与 Unix 均需跨进程互斥、进程退出释放、有限等待；不得 TTL 删锁、先删旧文件或无锁降级。
- 保护控制路径、保留合法旧 `001-*` ID；examples 的历史安装副本及历史开发记录不改写。
- 计划编写阶段只修改文档；此后用户已授权 Native 实现。提交、推送及真实宿主配置/实测仍需另行授权。

## Review Focus

1. 显式空 `--session-id` / 已设置但为空的环境变量不能伪装“没提供身份”；绑定故障不能让显式修改参数绕过隔离。由 Task 3/4 的身份与访问策略测试覆盖。
2. 锁拥有者被终止、解锁后留下锁文件、Windows 保留名或父目录链接不能导致锁分裂或越界。由 Task 2/3 的子进程与控制路径测试覆盖。
3. `phase=archive, archived=false` 仍是合法归档流程；只有真正已归档的目标才使默认会话上下文失效。由 Task 3/5 的归档边界测试覆盖。
4. hook 的 cwd 与进程目录不同、嵌套项目、相同 session ID 跨项目出现时，不能从父项目或继承环境猜上下文。由 Task 3/6 测试覆盖。
5. 脚本测试原来只写 active、不安装可执行 mulix 或 state.yaml；改造后必须修正 fixture，而不是用空结果回退掩盖错误。由 Task 8 的真实 CLI fixture、失败不落盘与冻结目录测试覆盖。

---

## 执行边界与文件地图

这是一个特性的依赖链，不拆成独立子项目。用户已选择 Native：在当前会话顺序实现，并完成一次独立整分支审阅；提交、推送及真实宿主配置变更仍需另行授权。

| 单元 | 新增或修改文件 | 职责 |
| --- | --- | --- |
| 契约证据 | 新增 `docs/verification/session-hook-contract.md` | 官方契约、宿主/版本/实测范围，不保存会话秘密 |
| 路径与锁 | 新增 `internal/fileutil/controlpath.go`、`controlpath_test.go`；新增 `internal/lockfile/{lock.go,lock_windows.go,lock_unix.go,lock_unsupported.go,lock_test.go}` | 受控路径检查、原生跨进程锁及超时 |
| 上下文 | 新增 `internal/sessionctx/{types.go,store.go,resolve.go,select.go,validate.go,store_test.go,resolve_test.go,select_test.go}`；修改 `internal/layout/layout.go` | 模式、绑定、解析、恢复与路径常量 |
| CLI 上下文 | 新增 `internal/cliutil/{context_cmd.go,context_cmd_test.go,session_context.go,session_context_test.go}`；修改 `root.go`、`new.go`、`new_test.go`、`state_cmd.go` | 身份读取、context 子命令、new/select 选择语义 |
| 状态更新 | 新增 `internal/state/{mutation.go,mutation_test.go}`、`internal/cliutil/state_session_test.go`；修改 `state_cmd.go`、`guard_cmd.go`、`status_cmd.go` | 加锁读改写、归档部分失败、只读诊断 |
| 写入 hook | 修改 `internal/hook/hook.go`、`hook_test.go`；新增 `internal/cliutil/{hook_eval.go,hook_eval_test.go}`；修改 `hook_cmd.go` | 会话请求、根定位、分类型失败与控制文件保护 |
| 启动/安装 | 新增 `internal/hook/{session_start.go,session_start_test.go}`、`internal/scaffold/{contextignore.go,contextignore_test.go}`；修改 `scaffold/init.go`、`init_test.go`、`cliutil/hook_cmd.go`、`init_cmd.go`、`update_cmd.go` 及对应测试 | SessionStart、幂等安装、忽略规则与升级提示 |
| 产物/指导 | 修改两个 vendored 脚本、`scripts/sync-superpowers.sh`、`internal/scaffold/ledger_test.go`；新增 `internal/scaffold/session_scripts_test.go`；修改 README、自有技能及 `internal/scaffold/agentsmd.go` | 全入口产物归属与宿主指导一致 |
| 综合回归 | 新增 `scripts/smoke-sessions.sh`、`internal/cliutil/session_concurrency_test.go`；修改 `scripts/smoke-e2e.sh` | 多会话 smoke、多进程集成与既有七阶段回归 |

测试需要私有 helper 时在所属 `_test.go` 中定义，不创建新的生产测试框架。现有 `runPresetCLI` 和 `chdirTemp` 改动进程 stdout/cwd，相关测试不得 `t.Parallel()`；并发证明使用真正的子进程。

### 依赖与提交约定

Task 1 为宿主契约前置调查；Task 2 → 3 → 4 → 5 → 6，Task 7 依赖 1/3/6，Task 8 依赖 4/7，Task 9 最后组合验收。宿主不可用时，2–6 的核心和显式参数测试可继续，7 的自动集成与 C19 不能冒充通过。

每个任务末尾是审阅/提交检查点。只有未来获准实现并且用户另行允许提交时，才按该任务 Files 精确暂存并提交；不要 `git add .`。在 main 上开始提交前先创建功能分支。本文列出的提交消息是候选，不是当前提交授权。

## Task 1: 核对宿主契约，固定可验证的集成边界

**Files:** Create `docs/verification/session-hook-contract.md`；本任务不创建产品代码或虚构宿主 fixture。

**Interfaces:** Produces 契约表：PreToolUse 的 `session_id`/`cwd`、SessionStart 输入、环境文件支持、stdout additionalContext 格式、startup/resume/clear/compact matcher、子代理身份与 CLI/桌面差异。后续 Task 7 只能使用表中有来源或实测证据的字段。

- [ ] **Step 1: 核对官方 hook 文档并记录来源、查阅日期和版本。** 从 `https://code.claude.com/docs/en/hooks` 开始；字段缺失写“未证实”，不据记忆填“支持”。只摘录完成本特性所需契约，不收集对话或凭证。
- [ ] **Step 2: 在获得宿主实测授权的临时项目验证身份传播。** 分别记录 CLI、桌面、恢复会话、子代理的实际结果；不改真实项目/全局 settings，不自动发起未经批准的付费模型调用。无法实测的行保留“未验证”，不是 PASS。
- [X] **Step 3: 固定双路径适配决策。** 经验证能用环境文件时只导出 `MULIX_SESSION_ID`；无此能力时 additionalContext 提示显式 `--session-id`，并明确未获得自动隔离。两种路径都不缓存 change、不新建绑定。契约不满足时不得悄悄改核心回退规则。
- [X] **Step 4: 检查证据表可供 Task 7 直接取用。** 每个自动集成字段有链接或实测证据，未知项标注影响；确认没有会话 token、完整环境或 transcript。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`docs: record session hook integration contract`。纯调查任务不伪造 RED/GREEN。

## Task 2: 受控文件路径与跨进程锁

**Files:** Create `internal/fileutil/controlpath.go`、`controlpath_test.go`；Create `internal/lockfile/lock.go`、`lock_windows.go`、`lock_unix.go`、`lock_unsupported.go`、`lock_test.go`。

**Interfaces:**

```go
// fileutil：root 是规范化项目根；rel 只能是项目内受控相对路径。
func ControlPath(root, rel string, createParents bool) (string, error)

// lockfile：fn 不得递归取得同一 key；解锁后不删除锁文件。
const ContextKey = "context"
func ChangeKey(change string) string
func With(ctx context.Context, root, key string, fn func() error) error
var ErrTimeout error
var ErrUnsupported error
```

`ChangeKey` 使用 change 的 SHA-256，实际文件位于 `.mulix/.locks/<key>.lock`。默认最多等待 **5 秒**，每 **20 毫秒**尝试一次，调用方更早的 context deadline 优先；超时返回可 `errors.Is` 判断的错误。该默认等待也适用于 hook，宿主超时支持在 Task 1 核对并在安装/说明中匹配，不能出现宿主先超时 fail-open 却宣称锁超时阻断。

- [X] **Step 1: 写失败测试。** `TestControlPath_RejectsTraversalSymlinkAndNonRegularTarget` 断言 `../x`、绝对路径、链接父目录/目标、目录目标不能读写项目外文件；链接不可创建时记录跳过，另保留可注入错误的确定性覆盖。`TestLock_MutualExclusionAcrossProcesses` 断言 A 持锁时 B 不能进入 fn；`TestLock_TimeoutDoesNotRunCallback` 断言 deadline 后 callback 次数为 0；`TestLock_KilledOwnerReleasesAndFileRemainsReusable` 终止 A 后 B 可进入，旧锁文件可保留。
  `TestLock_TimeoutDoesNotRunCallback` 的核心断言：

```go
if !errors.Is(err, ErrTimeout) || calls != 0 {
    t.Fatalf("timeout must not enter callback: calls=%d err=%v", calls, err)
}
```

- [X] **Step 2: 运行 RED。** `go test ./internal/fileutil ./internal/lockfile -run 'TestControlPath|TestLock' -count=1 -v`；新 API/包未实现导致失败，记录输出，不把环境故障当 RED。
- [X] **Step 3: 实现路径和平台锁。** `ControlPath` 用 Lstat 检查受控路径及已存在父级，不跟随链接；只创建明确的缺失父目录，复用已有安全写入。Windows 使用 LockFileEx/UnlockFileEx 锁偏移 0 的 1 字节；Unix 使用非阻塞排他 flock。`lock_unix.go` build tags 覆盖 Linux/macOS 及支持 syscall.Flock 的 BSD，其他目标明确返回 ErrUnsupported，不退回进程内锁。句柄关闭/进程退出释放锁，永不 unlink 锁文件。平台调用限定在两个小文件，不新增模块依赖。
- [ ] **Step 4: 运行 GREEN 与进程同步检查。** 同 Step 2；测试用管道/文件握手确认 A 已持锁，不以固定 sleep 猜时序。Windows 和 Linux/macOS 都要运行；交叉编译不等于跨平台运行验证。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: add bounded cross-process project locks`。

## Task 3: 会话存储、解析和目标选择服务

**Files:** Create `internal/sessionctx/{types.go,store.go,resolve.go,select.go,validate.go,store_test.go,resolve_test.go,select_test.go}`；Modify `internal/layout/layout.go`。

**Consumes:** Task 2 的 ControlPath、ContextKey、ChangeKey、With；既有 `state.FindRoot/Load/Save/Active/SetActive/ClearActive`、`fileutil.WriteFile`、`flow.State`。旧 state 全局文件接口保持低层能力，其生产调用需要本任务的锁包装；不在它们内部再隐式重入 context 锁。

**Produces:** 下列接口归 `sessionctx`；方法统一接受标准库 context，不与包名混淆。

```go
type Mode string // Legacy="legacy", Session="session"
type Source string // SessionBinding, GlobalFallback, Global, ExplicitChange，值与设计一致
type Access int // Inspect, Mutate
type Identity struct { Value string; Present bool }
type ResolveRequest struct { Identity Identity; ExplicitChange string; Access Access }
type Resolved struct {
    Root string `json:"root"`; Mode Mode `json:"mode"`
    SessionID string `json:"session_id"`; Change string `json:"change"`
    Source Source `json:"source"`; Phase flow.Phase `json:"phase"`
    RuntimeDir string `json:"runtime_dir"`; State *flow.State `json:"-"`
}
type SelectionTarget struct { Mode Mode; Identity Identity; Global bool }
type Selection struct { Mode Mode; SessionID string; Change string; Scope string }
type Store struct { /* 项目根及可测试的私有 I/O 依赖 */ }
func Open(start string) (*Store, error)
func (s *Store) Root() string
func (s *Store) Mode(ctx context.Context) (Mode, error)
func (s *Store) SetMode(ctx context.Context, mode Mode) error
func (s *Store) Resolve(ctx context.Context, req ResolveRequest) (Resolved, error)
func (s *Store) PrepareSelection(ctx context.Context, id Identity, global bool) (SelectionTarget, error)
func (s *Store) Select(ctx context.Context, target SelectionTarget, change string) (Selection, error)
func (s *Store) Unbind(ctx context.Context, id Identity) (bool, error)
func (s *Store) ClearActiveIf(ctx context.Context, change string) (bool, error)
func ValidateIdentity(id Identity) error
func ValidateChange(change string) error
```

Selection.Scope 仅为 `"session"` 或 `"global"`，表示本次修改范围，不一定等于会话的有效上下文来源。错误使用 `*ResolveError` 包装 Kind/Path/Err；Kind 至少有 `config-invalid`、`identity-invalid`、`binding-invalid`、`active-invalid`、`state-invalid`，支持 errors.As/Unwrap。Resolve 失败时保留已知 mode/source/change，hook 不靠匹配报错字符串决定放行。

- [X] **Step 1: 写解析与存储失败测试。** `TestResolve_BoundAndUnboundAreIndependent` 在 `resolve_test.go` 建 fixture：X/Y 有合法状态，session-A 绑定 X，B 无绑定，active=X；切 active=Y 后断言 `A.Change=="X" && A.Source==SessionBinding`、`B.Change=="Y" && B.Source==GlobalFallback`，B 绑定文件仍不存在。表测坏 JSON/未知字段/目录/读取错误/ID 不匹配/失效 state 不回退；active 真不存在返回空 change 和绝对 `_shared`，空文件不是不存在。`TestResolve_ArchivePhaseIsNotArchived` 断言 PhaseArchive/Archived=false 可解析，Archived=true 默认解析错误，显式 Inspect 可诊断它。

```go
if gotA.Change != "X" || gotA.Source != SessionBinding || gotB.Change != "Y" || gotB.Source != GlobalFallback {
    t.Fatalf("binding/fallback crossed: A=%+v B=%+v", gotA, gotB)
}
if _, err := os.Lstat(bindingPathB); !errors.Is(err, fs.ErrNotExist) {
    t.Fatalf("fallback must not create a binding: %v", err)
}
```
- [X] **Step 2: 增加身份、路径和恢复测试后运行 RED。** 最大 ID **512 UTF-8 字节**，拒绝非 UTF-8、控制字符、空/全空白、首尾空白，内部空格/单引号作为不透明内容保留；不存在身份用 Present=false。ValidateChange 保留日期/001/普通旧 slug，拒绝分隔符、冒号、`.`/`..`、`_shared`、Windows 设备名与尾随点/空白。测试重复 ID 在两个项目不同绑定、嵌套根优先、绑定/配置链接拒绝；SetMode 可显式修复普通坏配置文件，Unbind 不要求旧记录可解码。运行 `go test ./internal/sessionctx -count=1 -v`，确认新契约失败。
- [X] **Step 3: 实现存储和解析。** context/session JSON schema 严格解码，拒绝尾随第二个 JSON 值；绑定名为原始 ID 的 SHA-256，UTC bound_at。只在缺文件时回退，legacy 不读绑定；普通显式查询不读失效绑定，带身份的 session 修改必须解析并匹配有效上下文。模式/绑定/active 的元信息在短 context 锁内取快照，释放后再取 change 锁并加载 state；不得 context→change 嵌套。session 目标必须核对 State.Change 且未 Archived；legacy 保持旧默认解析。PrepareSelection 固定本次模式；Select 对目标加 change 锁验证，再在 context 锁内安全替换指定会话/全局记录，忽略旧绑定内容以便修复。
- [X] **Step 4: 测试锁序和持久化故障后运行 GREEN。** store 的私有 read/write/remove/load 函数可在同包测试注入 fs.ErrPermission/替换失败；断言坏旧绑定替换失败仍保留字节、ClearActiveIf 不删除已切换目标、切 legacy 再切 session 不丢绑定，Unbind 只移除受控普通文件。运行 Task 3 全测试及 `go test ./internal/state ./internal/fileutil ./internal/lockfile -count=1`。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: resolve change context from session bindings`。

## Task 4: CLI 身份、上下文管理及 new/select

**Files:** Create `internal/cliutil/{session_context.go,session_context_test.go,context_cmd.go,context_cmd_test.go}`；Modify `root.go`、`new.go`、`new_test.go`、`state_cmd.go`。

**Interfaces:** Consumes Task 3 的全部 Store 契约。Produces `sessionIdentity(cmd *cobra.Command, lookupEnv func(string) (string, bool)) (sessionctx.Identity, error)`、`resolveCommandContext(cmd *cobra.Command, explicit string, access sessionctx.Access) (*sessionctx.Store, sessionctx.Resolved, error)`、`newContextCmd() *cobra.Command`。后续 state/guard/status 用同一 adapter，不再自己读取 active。root 上只登记 persistent `--session-id`，不全局提前验证/加载绑定，避免坏会话阻断 init/update/version/preset。

- [X] **Step 1: 写失败测试。** `TestSessionIdentity_ExplicitFlagOverridesEnv`：flag=A/env=B 得 A；flag 显式空值、已设置但空 env 都错误，env 真未设置才是 Present=false。`TestContextCLI_ShowFormatsAndEmptyContext`：JSON 字段与 Resolved 一致；`--field runtime-dir` stdout 恰好一个绝对目录和换行；`--json` 与 `--field` 同时出现拒绝；空上下文 context show 成功。`TestContextCLI_UnbindReportsRemovalBeforeFallbackError`：旧绑定确实移除后全局失效，输出部分结果且返回非零。
- [X] **Step 2: 写选择矩阵测试并运行 RED。** `TestNewSelect_SessionLegacyAndGlobalMatrix` 覆盖设计 6.2 的六格：session+身份默认只写绑定、--global 只写 active；无身份/legacy 仍写 active；其他会话文件逐字节不变。失效旧绑定可被 select 替换；new 绑定写失败时新 state 仍存在，输出已创建 ID，不递归清理。Selection.Scope 固定为 `"session"` 或 `"global"`。核心断言如下，随后运行 `go test ./internal/cliutil -run 'TestSessionIdentity|TestContextCLI|TestNewSelect' -count=1 -v`。

```go
if stdout != wantRuntimeDir+"\n" { // TestContextCLI_ShowFormatsAndEmptyContext 的单字段分支
    t.Fatalf("machine output contains extra text: %q", stdout)
}
if got.Scope != "session" || globalAfter != globalBefore { // session 默认选择分支
    t.Fatalf("selection unexpectedly changed global: %+v", got)
}
```
- [X] **Step 3: 实现 CLI 接口。** context show 支持 text/json/单 runtime-dir 字段，诊断走 stderr；mode 显式设置不碰 active/绑定/settings。new 先 PrepareSelection，再保留既有分配逻辑创建目录，在 change 锁内保存新 state，最后以同一 SelectionTarget 调用 Select；重取同一 change 锁只能在前一锁已释放后进行，不重解析模式。select 直接 PrepareSelection→Select，不解析当前坏绑定。两个命令增加局部 `--global`；输出 Scope 是修改的目标范围，不把 --global 的结果冒充本会话有效 change。
- [X] **Step 4: 运行 GREEN 和旧 new 回归。** 同 Step 2，另运行 `go test ./internal/cliutil -run 'TestNew_|TestContextCLI|TestSessionIdentity' -count=1`；检查 `init/update/version` 在坏绑定和空 session env 下仍可进入自己的职责，context mode/unbind 可以修复自己的控制状态。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: add session-aware context selection commands`。

## Task 5: 加锁状态修改、归档和只读输出

**Files:** Create `internal/state/{mutation.go,mutation_test.go}`、`internal/cliutil/state_session_test.go`；Modify `state_cmd.go`、`guard_cmd.go`、`status_cmd.go`、`state_cmd_test.go`。

**Interfaces:**

```go
// state：afterSave 在状态已落盘且仍持有 change 锁时调用，不得再取同一 change 锁。
type UpdateResult struct { State flow.State; Saved bool }
func Update(ctx context.Context, root, change string,
    mutate func(*flow.State) error,
    afterSave func(flow.State) error) (UpdateResult, error)
```

Consumes Task 4 的 resolveCommandContext、Task 3 的 ClearActiveIf。`Update` 内重新 Load 后执行 mutate、Save，再执行可空 afterSave；Saved=true 且 error 非空表示后处理失败，不能报未保存。低层 Save 仍可供 fixture 和已加锁的新建路径使用。

- [X] **Step 1: 写失败测试。** `TestUpdate_SerializesReadModifyWriteAcrossProcesses` 用测试子进程同时修改不同字段，最终两个字段都在；`TestUpdate_ReloadsPhaseBeforeTransition` 两个进程提交同一迁移，恰好一个成功、另一个看到新阶段而非法。`TestUpdate_DifferentChangesDoNotShareLongGuardLock` 用握手阻塞 A 的 mutate，B 不等 A 释放即可完成。`TestUpdate_AfterSaveFailureReportsSavedState` 断言 result.Saved=true，落盘状态不回滚。
  `TestUpdate_AfterSaveFailureReportsSavedState` 的断言（next 是回调拟保存的合法状态）：

```go
if err == nil || !result.Saved || result.State.Change != next.Change {
    t.Fatalf("lost partial-success state: result=%+v err=%v", result, err)
}
if loaded.Archived != next.Archived {
    t.Fatalf("afterSave failure rolled back saved state: %+v", loaded)
}
```

- [X] **Step 2: 写 CLI 边界并运行 RED。** `TestStateMutation_ExplicitChangeCannotBypassBindingOrFallback`：绑定 A/回退 A 时指定 B 修改均失败，Inspect B 可用；坏绑定不能用 --change 绕过。`TestArchive_PreservesBindingAndConditionallyClearsGlobal`：归档目标绑定保留；afterSave 前把全局切到别的目标不会被清除；Archived=false 的 archive 阶段仍可执行 archived 事件。运行 `go test ./internal/state ./internal/cliutil -run 'TestUpdate_|TestStateMutation_|TestArchive_' -count=1 -v`。
- [X] **Step 3: 实现 Update 及命令迁移。** set/transition 先固定 Resolved.Change，再进入 Update；Update 重新加载时校验 State.Change 等于锁定目标，mutate 后不能改变 Schema/Change。guard.Run、合法事件检查和 flow.Apply 都在新加载状态上执行，保留 --force 只跳 readiness 不跳合法迁移。归档 afterSave 调 ClearActiveIf，绝不删除绑定。state show/guard 用 Inspect，status 列出全部目标且区分当前/全局标记，上下文故障显示并返回非零。删除旧无锁 ActiveIs→ClearActive 的生产组合；无 change 的 state show 仍报未选。
- [X] **Step 4: 运行 GREEN 与既有状态回归。** 同 Step 2，再运行 `go test ./internal/state ./internal/guard ./internal/cliutil -count=1`；不得给现有全局 cwd/stdout helper 加 t.Parallel 来伪造跨进程证明。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`fix: serialize state mutations and preserve session archive context`。

## Task 6: PreToolUse 接入会话解析及失败策略

**Files:** Modify `internal/hook/hook.go`、`hook_test.go`、`internal/cliutil/hook_cmd.go`；Create `internal/cliutil/hook_eval.go`、`hook_eval_test.go`。现有 `hook.Decide`/Render/ExitCode 保持可复用，不重写阶段规则。

**Interfaces:** `hook.Request` 新增 `SessionID *string`，nil 表示字段未提供；显式空指针值非法。Produces `hook.DecideContext(root string, resolved sessionctx.Resolved, req hook.Request) hook.Decision`；CLI 私有 `evaluatePreToolUse(ctx context.Context, processCWD string, payload []byte) hookOutcome`，`hookOutcome` 包含 Decision、Resolved、Cause。分离计算与 os.Exit，单测不执行真实退出；进程出口仍满足允许=0、阻断=2。

- [X] **Step 1: 写失败矩阵。** `TestHookEval_SessionBindingAndDynamicFallback` 构造 A/design、B/build，同一源码路径分别 deny/allow；切 active 只影响未绑定 C。`TestHookEval_InvalidBindingNeverFallsBack` 覆盖坏记录、丢 state、已归档、锁超时；断言拒绝及 source/change 恢复提示。`TestHookEval_NoActiveAndLegacyErrors` 覆盖无 active 放行、legacy 原有错误告警放行、session 坏 active 拒绝、坏 context.json 拒绝。
  `TestHookEval_InvalidBindingNeverFallsBack` 给绑定记录写入坏 JSON，同时全局指向 build，断言仍阻断：

```go
if outcome.Decision.Allow || outcome.Cause == nil {
    t.Fatalf("invalid binding fell back to a writable change: %+v", outcome)
}
if hook.ExitCode(outcome.Decision) != 2 {
    t.Fatal("session context failure must block the real tool call")
}
```

- [X] **Step 2: 写根定位/控制保护后运行 RED。** `TestHookEval_RequestCWDWinsAndMissingCWDUsesProcess` 使用嵌套项目和不同 processCWD；请求 cwd 失效不悄悄改用父根；继承 env ID 不覆盖请求 ID。`TestDecideContext_ProtectsControlPathsBeforeBuildAllowance` 在 build 与无当前 change 下拒绝四类控制文件/目录本身及子路径；另外在坏绑定场景验证非目标工具、缺 file_path 仍按旧约定放行，legacy 不额外保护这些新控制路径。运行 `go test ./internal/hook ./internal/cliutil -run 'TestHookEval_|TestDecideContext_' -count=1 -v`。
- [X] **Step 3: 实现入口与错误分类。** ParseRequest 成功后，非目标工具/缺 file_path 直接按旧约定放行，即使绑定失效也不扩大检查范围；有效 Write/Edit 才进入上下文解析。ResolveRequest 用 Inspect 且不带 explicit change；默认绑定归档仍按 Task 3 的默认规则失败。非法请求先尝试从进程 cwd 确认模式，已知 session 项目拒绝；无管理根告警放行。config-invalid 不能因 mode 尚未知而 fail-open。DecideContext 在普通阶段检查前保护 `.mulix/context.json`、active、sessions、.locks。拒绝信息含可得的 mode/change/phase/source，不能要求改全局来修复坏绑定。
- [X] **Step 4: 注入输出失败并运行 GREEN。** 把 hook 输出收敛为私有 `emitHook(out io.Writer, decision hook.Decision, strict bool, render func(hook.Decision) ([]byte, error)) int`；`TestEmitHook_DenyRenderFailureStillBlocks` 注入编码或 writer 失败，需阻断时 exit=2/stderr 有原因，不进入通用 failOpen。运行本任务矩阵、既有 hook 全测试，并用临时构建的真实二进制验证 exit 0/2，不能在 runPresetCLI 内触发 os.Exit。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: enforce session context in write hooks`。

## Task 7: SessionStart、幂等安装与本地忽略

**Files:** Create `internal/hook/{session_start.go,session_start_test.go}`、`internal/scaffold/{contextignore.go,contextignore_test.go}`；Modify `internal/scaffold/init.go`、`init_test.go`、`internal/cliutil/hook_cmd.go`、`init_cmd.go`、`update_cmd.go`、`update_cmd_test.go`。

**Interfaces:** Consumes Task 1 的已核对契约、Task 3 的身份与 Store。Produces `hook.RenderSessionStart(additionalContext string) ([]byte, error)`、`hook.SessionEnvLine(id sessionctx.Identity) (string, error)`、`scaffold.ensureContextIgnore(root string) (changed bool, err error)`；SessionStart 入口保留 `mulix hook session-start`，PreToolUse 仍为 `mulix hook`。精确 JSON shape/matcher 从 Task 1 证据表复制，不在缺证据时猜结构。

- [X] **Step 1: 写失败测试。** `TestSessionStart_OnlyExportsIdentityAndDoesNotBind`：只有宿主明确环境文件存在时追加一条正确转义的 export，仅 MULIX_SESSION_ID，不含 change/runtime，不建立绑定、不切模式。`TestSessionEnvLine_QuotesOpaqueIDWithoutExecuting` 用含内部单引号/空格的合法 ID，在临时 Bash source 后原样读回，攻击标记文件不存在。`TestSessionStart_NoEnvFileHasExplicitParameterGuidance`：不猜全局 shell 文件，有符合契约的显式参数提示。
  `TestSessionEnvLine_QuotesOpaqueIDWithoutExecuting` 的最终断言（roundTrip 来自独立 Bash 子进程）：

```go
if roundTrip != id.Value || markerExists {
    t.Fatalf("opaque ID was changed or executed: got=%q marker=%v", roundTrip, markerExists)
}
```

- [X] **Step 2: 写安装/忽略/升级测试并运行 RED。** `TestInit_SessionStartMergeIsIdempotentAndPreservesUserHooks`：两次 init 只有一条 mulix SessionStart，原用户 hook/settings 和 PreToolUse 保留；DSH-only 无 Claude。`TestContextIgnore_PreservesCustomRulesAndLocalData`：`.mulix/.gitignore` 只幂等补充 `/context.json`、`/sessions/`、`/.locks/`，不覆盖原内容、不创建上下文配置；坏/链接忽略文件报错。`TestUpdate_ReportsMissingSessionIntegrationWithoutEditingSettings`：CLI 提示显式 init，普通/force update 前后 settings、context、bindings 字节不变。运行 `go test ./internal/hook ./internal/scaffold ./internal/cliutil -run 'TestSessionStart_|TestSessionEnvLine_|TestInit_SessionStart|TestContextIgnore_|TestUpdate_ReportsMissing' -count=1 -v`。
- [X] **Step 3: 实现经核对的集成。** SessionStart 只读取宿主身份及可诊断上下文；身份合法但绑定失效时输出修复提示，不回退/不阻断宿主启动。legacy 不因坏绑定失败。init 复用 mergeHookEntry，必要时拆小 helper 但不重构更新算法；普通 update 的缺集成提示放 CLI 层，只对已安装 Claude 项目，检查失败明确告警，不写 settings。忽略文件为本地一次性补齐，不能进入 vendor baseline/sweep，force 同样不重置。
- [ ] **Step 4: 运行 GREEN 并核对真实宿主证据。** 同 Step 2，加现有 init/update 全测试。只对 Task 1 已证实的平台宣称自动注入；不能从 fixture PASS 推导桌面/子代理实测通过。若 hook 默认超时短于锁上限，在本任务按核对契约设置合适 hook timeout 并测试，或明确调整内部等待预算，仍须保证有限等待与阻断语义。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: propagate session identity without changing global defaults`。

## Task 8: 运行产物脚本、vendor 生成规则和使用指导

**Files:** Modify `assets/superpowers/skills/subagent-driven-development/scripts/sdd-workspace`、`assets/superpowers/skills/brainstorming/scripts/start-server.sh`、`scripts/sync-superpowers.sh`、`internal/scaffold/ledger_test.go`、`README.md`、`assets/skills/mulix-using/SKILL.md`、`assets/skills/mulix-build/SKILL.md`、`assets/skills/mulix-archive/SKILL.md`、`internal/scaffold/agentsmd.go` 及 `agentsmd_test.go`；Create `internal/scaffold/session_scripts_test.go`。

**Interfaces:** 脚本在已定位的项目根运行 `mulix context show --field runtime-dir`，使用继承的 MULIX_SESSION_ID；stdout 是单个绝对目录，非零立即退出，不用 jq/Python 解析。脑暴自建实例变量改为 `BRAINSTORM_INSTANCE_ID`，不再与宿主 ID 混用。Task 4 的 CLI 是唯一运行目录来源。

- [X] **Step 1: 写真实 CLI fixture 和失败测试。** `session_scripts_test.go` 中 `TestMain` 一次构建项目二进制到测试临时目录，清理后退出；`ledgerCommand` 仅对子进程把临时 binary 目录加 PATH，保留其他 env。ledgerFixture 用 state.Save 创建合法 001-ledger 状态，不再只有 active。`TestSessionScripts_TwoBindingsProduceSeparateRuntimeDirs` 断言同名 tasks plan 的 workspace 分别归 A/B；`TestSessionScripts_BadBindingFailsWithoutCreatingSharedArtifacts` 比较运行产物目录调用前后的文件清单（不包含正常创建的锁文件），坏绑定非零且无 _shared/错误目录产物。
  `TestSessionScripts_BadBindingFailsWithoutCreatingSharedArtifacts` 的断言（before/after 是运行产物目录中排序后的相对文件清单，不含锁文件）：

```go
if runErr == nil || !slices.Equal(before, after) {
    t.Fatalf("script hid resolution failure or wrote artifacts: err=%v", runErr)
}
```

- [X] **Step 2: 加冻结目录/vendor 回归并运行 RED。** `TestSessionScripts_GlobalSwitchOnlyAffectsNewInvocation`：已启动脑暴服务固定原运行目录，新调用跟随新全局，端口/token 不迁移；测试进程用可控前台启动和显式清理，不依赖 Windows nohup。`TestSessionScripts_NestedRootAndSpaceInPath`：嵌套 mulix 根/含空格路径仍正确。`TestVendorPatches_ResolveContextInsteadOfReadingActive` 检查同步补丁与安装后脚本的上下文调用一致、无活跃 change 的直接读取。运行 `go test ./internal/scaffold -run 'TestSessionScripts_|TestVendorPatches_|TestLedger|TestTaskDone' -count=1 -v`。
- [X] **Step 3: 修改两个脚本和同步补丁。** 使用单次目录解析快照；失败发生在 mkdir/服务器 token 写入前，禁止 `|| true`→_shared；既有 plan-path 碰撞、证据目录、脚本权限保持。同步脚本仍精确匹配补丁且上游漂移报错。允许单独运行未安装 mulix 的脑暴脚本时，只在确实没有 mulix 项目的原有非项目分支使用临时目录；有 mulix 项目却找不到可执行文件必须报错，不伪装无当前 change。
- [ ] **Step 4: 同步指导并运行 GREEN。** README/自有技能/DSH bootstrap 明确 `context mode session`、new/select 默认绑定、--global、解绑、回退来源、失效绑定修复、宿主未注入时显式参数、归档保留绑定。legacy 的旧失败行为与 session 的收紧边界分别描述，不再一概说全部错误 fail-open；外部写工具与同源文件冲突边界不变。修改技能/AGENTS 引导时按执行阶段加载相应 writing-skills/writing-for-agents 指导。运行 Step 2 及现有 scaffold 全测试；Unix、Windows/Git Bash 分别记录。另将同步脚本复制到临时仓库的 scripts/，使用与 vendored VERSION 匹配的上游源只读输入，在临时 assets/ 中执行真实重新生成，确认上下文改写仍在；不能在本仓库直接运行会清空 assets 的脚本。没有匹配上游源时，明确记录 C24 的真实重新生成未验证，静态补丁检查不能冒充该证据。
- [ ] **Step 5: 审阅检查点。** 候选提交消息：`feat: scope runtime artifacts to resolved session context`。

## Task 9: 多会话综合验收与发布前证据

**Files:** Create `scripts/smoke-sessions.sh`、`internal/cliutil/session_concurrency_test.go`；Modify `scripts/smoke-e2e.sh`、`docs/verification/session-hook-contract.md`。不得改 examples 的已安装副本来让测试通过。

**Interfaces:** smoke-sessions 接受可选二进制路径，与 smoke-e2e 一样只在临时 Git 项目操作；子进程集成分别设置 cwd/env/stdin，捕获真实退出码与 stdout/stderr，不用全局 chdir 并发。Task 5 的 Update 与 Task 6 hook 是最终测试入口。

- [X] **Step 1: 写失败的综合回归。** `TestSessionCLI_ArchiveRaceDoesNotClearNewActive`、`TestSessionCLI_ConcurrentFieldUpdatesPreserveBoth`、`TestSessionCLI_FailedBindingWriteReportsCreatedChange` 使用独立进程和确定性握手/故障注入，断言磁盘内容而非只查输出。smoke-sessions 至少包含 A/design、B/build、C/fallback、全局切换、坏绑定拒绝、解绑恢复、无 active、控制文件阻断；旧 smoke 里硬编码脚本文本的 active 断言改为实际行为验证，不削减既有七阶段证据。
  `TestSessionCLI_ConcurrentFieldUpdatesPreserveBoth` 在两个进程成功退出后重新加载同一状态：

```go
if loaded.SpecPath != wantSpecPath || loaded.TasksPath != wantTasksPath {
    t.Fatalf("concurrent CLI updates lost a field: %+v", loaded)
}
```

- [X] **Step 2: 运行 RED 并核对它检测到真实缺陷。** `go test ./internal/cliutil -run '^TestSessionCLI_' -count=1 -v`，以及新 smoke。若组合实现已自然满足测试，在临时改动中只撤去对应选择/锁/拒绝分支证明测试会失败，随后立即恢复；不能把网络、缺 Bash 或工具链错误当作功能 RED。
- [X] **Step 3: 修复组合缝隙，不扩大设计。** 重点核对生产调用没有遗留无锁 active 写入、没有先 Load 再在锁外 Save、没有脚本自己读取 active、没有误用 `phase=archive` 代替 Archived。保持 error 与部分成功输出一致；不添加源码锁/自动 Worktree/自动绑定作为“顺手优化”。
- [ ] **Step 4: 执行完整验收并记录证据。** 运行下列命令，各自记录退出码和平台。先构建供两个 smoke 使用的临时二进制，Windows 路径可用 `/tmp/mulix-session-verify.exe`，不是仓库 bin/dist 产物。

```bash
go test ./... -count=1
```

```bash
go vet ./...
```

```bash
go test -race ./internal/lockfile ./internal/sessionctx ./internal/state ./internal/cliutil -count=1
```

```bash
go build -o /tmp/mulix-session-verify.exe ./cmd/mulix
```

```bash
bash scripts/smoke-e2e.sh /tmp/mulix-session-verify.exe
```

```bash
bash scripts/smoke-sessions.sh /tmp/mulix-session-verify.exe
```

```bash
bash scripts/smoke-update.sh /tmp/mulix-session-verify.exe
```

race 若受 Windows C 编译器/宿主限制，明确列为未验证并在支持平台补跑；不能静默换成普通测试再称 race 通过。Unix 和 Windows/Git Bash 的锁与脚本都须运行，交叉编译仅补充构建证据。真实宿主 C19 单独写证据，不以二进制 mock 代替。

- [ ] **Step 5: 自查验收矩阵、完成审阅检查点。** 按下表逐项链接到实际测试/日志，列出所有未验证项、宿主限制与部分失败；加载 verification-before-completion，并在获准实现的执行方式要求下完成整分支独立审阅。候选提交消息：`test: verify session binding compatibility and concurrency`。集成、push、PR、merge 均不包含在本计划的自动授权中。

## 设计覆盖与自审清单

| 设计要求 | 实施任务与证明 |
| --- | --- |
| §1–2 绑定/回退契约及非目标 | Task 3/4/6；所有任务 Global Constraints |
| §3 既有 CLI、hook、脚本接缝 | Task 4–8 文件地图；Task 9 组合 |
| §4 模式、schema、控制路径、生命周期 | Task 2/3/7；C01、C05–08、C20、C22–23 |
| §5 身份、来源、根定位、动态回退 | Task 3/4/6；C02–04、C10、C12–13、C18 |
| §6 上下文命令/new/select/--change | Task 4/5；C07–11 |
| §7 hook 与错误边界 | Task 6；C04–06、C12–13、C18、C21 |
| §8 宿主传播/升级/忽略规则 | Task 1/7；C19–20 |
| §9 产物与调用快照 | Task 8；C03、C17、C24 |
| §10 并发/归档/部分失败 | Task 2/3/5/9；C08、C14–16、C23 |
| §11 来源、恢复与输出通道 | Task 4–7；text/JSON/field 及错误测试 |
| §12 全部 C01–C24 | 上述测试 + Task 9 七阶段/更新/多会话 smoke，真实宿主另列 |
| §13 留给实施者的决策 | 本计划选择原生 OS 锁、5s/20ms、512 字节身份、明确接口；宿主格式按 Task 1 证据决策 |
| §14 授权边界 | 当前只生成文档；实现/提交/宿主实测在后续授权后进行 |

- [X] 每个任务先有可失败的契约测试，再实现与验证；调查任务不伪装 TDD。
- [X] 核对 Store/Resolved/Identity/SelectionTarget/UpdateResult 名称、签名和错误 Kind 在所有后续任务一致。
- [X] 五项 Review Focus 各有具名测试；C01–C24 都有对应证明，未实测项明确列出。
- [X] 没有把宿主能力、功能测试或 RED/GREEN 的预期结果写成当前已验证事实。
- [X] 用户审阅计划并选择 Native 后才开始 Task 1；未完成的跨平台、真实宿主及未授权的提交检查点保持未勾选。

## 当前交付状态

2026-10-28：用户批准正式设计及 Native 实施。功能代码已实现，独立整分支审阅发现的三项问题已修复并通过回归；Windows 本机全量、竞态、静态检查和三组 smoke 已通过。真实 CLI 2.1.312 的临时项目实测仅证实 SessionStart；独立 CLI 未登录，PreToolUse 与自动环境注入未触发，Unix 原生运行仍未验证；对应步骤及未授权的提交检查点保持未勾选。未提交、推送或修改真实宿主配置。
