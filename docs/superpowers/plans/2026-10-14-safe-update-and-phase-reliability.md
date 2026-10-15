# mulix 门禁、持久化与安全更新实施计划

> **For agentic workers:** 用户已选择 **Native**。REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in the current session, with a fresh whole-branch reviewer at the end. Steps use checkbox syntax for tracking. 未经用户明确要求不提交、不推送、不创建 PR；各任务的「收尾」记录验证证据而不是自动 commit。

## Context

用户已于 2026-10-14 批准设计文档，要求实现门禁去重、状态安全写入、门禁边界文案、update 正确性修复及 `mulix-using` 改名兼容迁移；之后追加产物命名需求，并明确选择「只改变更目录名」：新变更使用 `YYYY-MM-DD-功能名`，内部保留 spec.md/tasks.md/report.md。用户已选择Native执行。当前源码基线为 `db38da332abd597ec125df6d63952a0527778d82`。已在临时项目复现：同版本第二次更新丢本地修改、冲突重跑丢本地版本、DSH-only 意外新增 Claude；现有全量测试和七阶段 smoke 未覆盖这些场景。

**Goal:** 让门禁检查以单一规则表执行，让资源更新、状态持久化和入口迁移在失败、重试及历史基线不可信时不静默丢失用户内容，并让新变更目录统一为日期型名称。

**Architecture:** 保留七阶段、现有 `managedFiles`、line-level `merge3` 和基线派生 obsolete；新增共享安全 writer、小型逐文件安装记录，以及独立的 backup/conflict/rename 单元。更新先处理 pending 与入口家族，再处理普通资源及 obsolete，最后在新入口可用时刷新 DSH 引导。

**Tech Stack:** Go 1.27、Cobra 1.10.2、yaml.v3、标准库 JSON/SHA-256/文件系统；不新增依赖。当前验证平台为 Windows amd64 / Go 1.27.1 / Git Bash。

**Spec:** `docs/superpowers/specs/2026-10-14-safe-update-and-phase-reliability-design.md`

**Plan location:** 本项目副本已同步获批会话计划，并加入用户随后明确要求的checkbox恢复；正在按已选择的Native方式实施。日期目录与checkbox补充已同步设计附录，未改变其他批准范围。

## 已确认的命名补充设计

新增要求只改变新建变更的目录/ID命名，不改文档文件名，也不将文档平铺：

```text
docs/specs/2026-10-14-add-login/spec.md
docs/changes/2026-10-14-add-login/tasks.md
docs/changes/2026-10-14-add-login/report.md
.mulix/.runtime/2026-10-14-add-login/state.yaml
```

`mulix new` 单次捕获本地日历日期，保留现有Slugify规则。新的change ID与specs/changes/runtime三个目录使用同一日期型ID；同日同名冲突时追加-2、-3，不覆盖已存在目录。旧001-*变更、active、state/guard/hook路径继续支持，不扫描迁移旧目录，不更改state schema。内部产物仍为spec.md/tasks.md/report.md，嵌入writing-plans保存任务文件的路径约定不变。

此补充由用户在布局选择中确认，作为原Spec的命名附录；实施批准后同步设计文档和项目计划副本，再写产品代码。

## Global Constraints

- 本轮保持七阶段流程和 `mulix.state.v2` 不变，不新增阶段，不改变 hook 的工具匹配范围、白名单或 fail-open 策略。
- 不实现多会话锁、并发隔离、全项目事务、CLI self-update、模糊重命名检测、预览命令或通用包管理框架。
- `examples/todo-cli` 及其安装基线、旧 state/技能/运行记录保持不变。
- `docs/PLAN.md` 的历史事实不全局替换；只追加当前变更说明。
- preset resolve 的磁盘/内嵌 core 来源问题已确认，但作为后续独立改动，不混入本轮。
- B 永远不能包含用户定制或冲突标记；成功合并保存 `L=M, B=N`。
- 旧基线逐文件证明，未知内容保留；冲突不写入活动技能；危险覆盖先备份。
- 无法自动完成的更新返回非零并打印已完成部分。
- 不承诺所有 Windows 文件系统绝对原子，不将单文件安全替换描述为跨文件事务或并发隔离。
- 不主动创建 worktree，不安装依赖，不修改用户 Claude 配置，不改内嵌 superpowers 上游内容。
- 新变更目录/ID使用本地日期YYYY-MM-DD-功能slug，内部文件名不改；同日同名用-2/-3，旧编号目录保持原样可用。

## Review Focus

1. 元信息损坏、路径穿越或 baseline digest 不匹配：不能降级成可覆盖旧文件；Task 3 的拒绝/保留测试覆盖。
2. force 备份失败及后续写入中断：不能删除原文件，已经完成的操作必须报告；Task 4/8 的失败注入覆盖。
3. pending 时 L 缺失、N 已换版本、候选被用户编辑：不能通过重复 update 清空问题或覆盖候选；Task 6 覆盖。
4. 旧名辅助文件、多个定制来源、目标碰撞、迁移后清理中断：不能选遍历顺序的赢家或递归删除未知文件；Task 7 覆盖。
5. 元信息/基线缺失的 DSH-only 及新旧入口并存：不能新增 Claude，也不能让 AGENTS.md 指向缺失入口；Task 5/7 覆盖。

## 文件职责与复用边界

| 文件 | 职责 |
| --- | --- |
| `internal/guard/guard.go` | 用 `flow.Find` + 检查名称注册表执行 GuardRefs；复用现有所有检查函数 |
| `internal/fileutil/atomic.go`（新增） | 同目录临时文件、Sync/Close、替换及失败清理 |
| `internal/state/state.go`, `root.go` | Save/SetActive 调用安全 writer；格式、时间戳、ClearActive 不变 |
| `internal/scaffold/newchange.go`, `newchange_test.go`, CLI new测试 | 新建日期型ID和三个关联目录，碰撞分配与旧ID兼容；复用Slugify/layout |
| `internal/scaffold/installstate.go`（新增） | 小型 JSON 记录、路径验证、digest 信任与 hosts |
| `internal/scaffold/legacy.go`（新增） | 冻结两个明确旧入口的已知官方指纹；不猜测其他历史版本 |
| `internal/scaffold/updatefiles.go`（新增） | 普通 L/B/N 分类与可信基线提交 |
| `internal/scaffold/updatebackup.go`（新增） | 懒创建、独立运行目录、覆盖前备份与失败边界 |
| `internal/scaffold/updateconflict.go`（新增） | 旁路候选、pending 和逐文件接受 |
| `internal/scaffold/updaterename.go`（新增） | 明确 alias 家族的迁移、碰撞与旧源清理 |
| `internal/scaffold/update.go` | 对外 Update 编排、结果分类、host 与 obsolete；不继续堆所有细节 |
| `internal/scaffold/init.go` | init 注册实际官方内容和显式 host，不认领跳过异内容 |
| `internal/scaffold/agentsmd.go` | 正式入口文案与引导可用性；复用哨兵合并逻辑 |
| `internal/cliutil/update_cmd.go` | `--accept`、参数约束、部分结果输出与非零退出 |
| `assets/skills/mulix-using/SKILL.md` | 正式入口及准确门禁边界；保留宿主占位符 |
| `README.md`, CLI 帮助/相关注释, `docs/PLAN.md` | 当前契约说明与新增历史记录，不重写历史 |
| `scripts/smoke-e2e.sh`, `scripts/smoke-update.sh`（新增） | 原七阶段 + 临时项目资源更新/迁移/重试 |

复用 `managedFiles`, `managedFile.Mode`, `merge3`, `splitLines`, `baselinePath`, `mergeAgentsBootstrap`, `allHosts`, `Host.skillsDir`；不重新写 merge 算法、preset resolver 或状态机。新文件只按具体职责拆分，不引入插件式 update framework。

## 跨任务接口

下列名字固定，实施时不自行创建第二套等价接口；需要调整先同步计划及调用方。

```go
// internal/fileutil
func WriteFile(path string, data []byte, mode fs.FileMode) error

// internal/scaffold/installstate.go
const installSchema = "mulix.install.v1"
const installStateRel = ".mulix/install-state.json"
type vendorRecord struct { SHA256 string `json:"vendor_sha256"` }
type pendingSource struct {
    Rel string `json:"rel"`
    LocalSHA256 string `json:"local_sha256"`
    BaselineSHA256 string `json:"baseline_sha256,omitempty"`
}
type pendingUpdate struct {
    Stage string `json:"stage"` // "conflict" 或 "accepted-cleanup"
    IncomingSHA256 string `json:"incoming_sha256"`
    Candidate string `json:"candidate,omitempty"`
    LocalSHA256 string `json:"local_sha256"`
    AcceptedSHA256 string `json:"accepted_sha256,omitempty"`
    BackupDir string `json:"backup_dir,omitempty"`
    Sources []pendingSource `json:"sources,omitempty"`
}
type installState struct {
    Schema string `json:"schema"`
    Hosts []Host `json:"hosts"`
    Files map[string]vendorRecord `json:"files"`
    Pending map[string]pendingUpdate `json:"pending,omitempty"`
}
func contentDigest(data []byte) string
func loadInstallState(root string) (*installState, error) // 不存在→空v1记录，不是错误；其他错误→error
func saveInstallState(root string, st *installState) error
func validManagedRel(rel string) bool
func legacyVendorMatch(rel string, data []byte) bool

// internal/scaffold/update.go
// 保留 Root/Force，增加 Accept；结果已有六类保持，新增下面字段。
type UpdateOptions struct { Root string; Force bool; Accept []string }
type UpdateResult struct {
    Written, Updated, Merged, Conflicted, Removed, Skipped []string
    Accepted, Migrated, Blocked, Backups []string
}
func (r UpdateResult) NeedsAttention() bool // len(Conflicted)>0 || len(Blocked)>0

type updateRun struct {
    opts UpdateOptions
    result UpdateResult
    state *installState
    backups *updateBackups
    // 私有测试边界；默认绑定fileutil/os，不暴露给CLI，不使用全局可变钩子。
    write func(string, []byte, fs.FileMode) error
    remove func(string) error
    saveState func(*installState) error
}
func newUpdateRun(opts UpdateOptions) (*updateRun, error)
func (r *updateRun) updateFile(f managedFile) error
func (r *updateRun) commitVendor(rel string, local, incoming []byte, mode fs.FileMode) error
func (r *updateRun) removeObsolete(rel string) error
func (r *updateRun) recordConflict(f managedFile, local, merged []byte, sources []pendingSource) error
func (r *updateRun) acceptPending(rel string, incoming managedFile) error
func (r *updateRun) migrateEntry(h Host, incoming managedFile) (handled bool, ready bool, err error)
func updateHosts(root string, st *installState) ([]Host, error)

type updateBackups struct { root string; dir string; backed map[string]bool }
func (b *updateBackups) preserve(rel string) error // 懒创建运行目录；备份L、旧B与元信息
```

`Update` 仍返回 `(UpdateResult, error)`：操作错误经 error 返回，需人工处理分类留在结果，由 CLI 转为非零；不能为了退出码丢掉部分结果。

---

## Task 1: 让 GuardRefs 驱动实际检查

**Files:** Modify `internal/guard/guard.go`, `internal/guard/guard_test.go`；`internal/flow/flow.go` 只校正注释，不改迁移表语义。

**Consumes:** `flow.Find`, `flow.Table`, 现有 checkFunc / Report / Result。
**Produces:** 同签名 `guard.Run`，唯一的 `checksByName`。

- [X] **Step 1: 加入失败测试，补齐已有 fixture 的真实 phase。**

在 guard 包测试中加入：

```go
func TestRun_UsesDeclaredGuardRefs(t *testing.T) {
    original := flow.Table
    flow.Table = append([]flow.Transition(nil), original...)
    t.Cleanup(func() { flow.Table = original })
    for i := range flow.Table {
        if flow.Table[i].Event == flow.EventSpecComplete {
            flow.Table[i].GuardRefs = []string{"not-registered"}
        }
    }
    report := Run(t.TempDir(), flow.New("001-x", ""), flow.EventSpecComplete)
    if report.Passed() || len(report.Results) != 1 || report.Results[0].Name != "not-registered" {
        t.Fatalf("unregistered declared guard must fail: %+v", report)
    }
}
func TestRun_RejectsIllegalPhaseEvent(t *testing.T) {
    report := Run(t.TempDir(), flow.New("001-x", ""), flow.EventBuildComplete)
    if report.Passed() { t.Fatal("illegal phase/event passed") }
}
```

不在修改 flow.Table 的测试里使用 t.Parallel。现有 Task/Build/Verify/Archive fixture 分别设为相应 phase，使它们仍测试证据而不是非法事件。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/guard -run 'TestRun_(UsesDeclaredGuardRefs|RejectsIllegalPhaseEvent)' -count=1`
Expected: 当前代码不能返回声明的未知名称，两个新测试失败。

- [X] **Step 3: 用名称注册表替代 checksByEvent。**

```go
var checksByName = map[string]checkFunc{
    "spec-artifact-present": checkArtifactPresent("spec-artifact-present", func(s flow.State) string { return s.SpecPath }),
    "clarify-recorded": checkClarifyRecorded,
    "clarify-skip-acknowledged": checkClarifySkipAcknowledged,
    "design-approved": checkDesignApproved,
    "tasks-artifact-present": checkTasksInChangeDir,
    "tasks-have-task-sections": checkTasksHaveSections,
    "execution-method-chosen": checkExecutionMethodChosen,
    "plan-tasks-complete": checkPlanTasksComplete,
    "tdd-evidence-present": checkTddEvidencePresent,
    "verification-report-present": checkArtifactPresent("verification-report-present", func(s flow.State) string { return s.ReportPath }),
    "verify-result-pass": checkVerifyResultPass,
    "verification-failed": checkVerifyResultFail,
    "archive-confirmed": checkArchiveConfirmed,
}
func Run(root string, s flow.State, event flow.Event) Report {
    report := Report{Event: event}
    transition, ok := flow.Find(s.Phase, event)
    if !ok {
        report.Results = append(report.Results, Result{Name: "legal-transition", Next: fmt.Sprintf("Event %q is not legal from phase %q.", event, s.Phase)})
        return report
    }
    for _, name := range transition.GuardRefs {
        check, ok := checksByName[name]
        if !ok {
            report.Results = append(report.Results, Result{Name: name, Next: fmt.Sprintf("Guard %q is not registered; fix the guard registry before transitioning.", name)})
            continue
        }
        report.Results = append(report.Results, check(root, s))
    }
    return report
}
```

增加 Table 全量一致性测试：全部引用已注册、每个 transition 无重复名称、返回结果名称与顺序一致、无孤立注册函数。显式空 GuardRefs 的合法 transition 保持通过，不把未知事件当空 guard。

- [X] **Step 4: 验证 GREEN 与 CLI 回归。**

Run: `go test ./internal/guard ./internal/flow ./internal/cliutil -count=1`
Expected: 全通过，迁移证据要求未改变。

- [X] **Step 5: 记录 RED/GREEN、检查 diff；不自动提交。**

## Task 2: 共享安全 writer 与状态/active 替换

**Files:** Create `internal/fileutil/atomic.go`, `atomic_test.go`；Modify `internal/state/state.go`, `root.go`, `state_test.go`, `root_test.go`。

**Produces:** `fileutil.WriteFile`。helper 按传入 mode 设置临时文件；state/active 替换已有普通文件时沿用原有 permission bits，新建默认 0644。

- [X] **Step 1: 写替换失败仍保留原文件与临时文件清理测试。**

测试内部私有 `writeFileWith` 的失败边界（不公开注入接口）：

```go
func TestWriteFile_ReplaceFailureKeepsOriginal(t *testing.T) {
    dir := t.TempDir()
    target := filepath.Join(dir, "state.yaml")
    if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil { t.Fatal(err) }
    failure := errors.New("replacement failed")
    err := writeFileWith(target, []byte("new\n"), 0o644, writeAndSync,
        func(string, string) error { return failure })
    if !errors.Is(err, failure) { t.Fatalf("error = %v", err) }
    got, err := os.ReadFile(target)
    if err != nil || string(got) != "old\n" { t.Fatalf("original changed: %q, %v", got, err) }
    entries, err := os.ReadDir(dir)
    if err != nil || len(entries) != 1 { t.Fatalf("temporary files leaked: %v, %v", entries, err) }
}
```

增加 write callback 写一部分后报错的测试，断言替换函数未被调用、旧内容不变；新建/替换大于旧文件内容、无多余尾字节、目录目标拒绝、非普通目标拒绝、Unix 权限保留测试。Windows 不使用只读目录作为失败注入。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/fileutil -count=1`
Expected: 缺少 helper 与私有测试边界，编译失败。

- [X] **Step 3: 实现单目录准备后替换，不先删旧文件。**

核心实现：

```go
func WriteFile(path string, data []byte, mode fs.FileMode) error {
    return writeFileWith(path, data, mode, writeAndSync, os.Rename)
}
func writeAndSync(f *os.File, data []byte) error {
    n, err := f.Write(data)
    if err != nil { return err }
    if n != len(data) { return io.ErrShortWrite }
    return f.Sync()
}
func writeFileWith(path string, data []byte, mode fs.FileMode,
    write func(*os.File, []byte) error, replace func(string, string) error) error {
    if info, err := os.Lstat(path); err == nil {
        if !info.Mode().IsRegular() { return fmt.Errorf("write %s: target is not a regular file", path) }
    } else if !os.IsNotExist(err) { return err }
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0o755); err != nil { return err }
    f, err := os.CreateTemp(dir, ".mulix-write-*")
    if err != nil { return err }
    name := f.Name()
    defer os.Remove(name)
    closed := false
    defer func() { if !closed { _ = f.Close() } }()
    if err := f.Chmod(mode.Perm()); err != nil { return err }
    if err := write(f, data); err != nil { return err }
    if err := f.Close(); err != nil { return err }
    closed = true
    return replace(name, path)
}
```

为具体路径包装错误，不能忽略写/Sync/Close/rename 错误。Save/SetActive 只替换最终 os.WriteFile，格式和其他校验不变。读取已有 mode 的错误只有不存在可以回退为 0644。ClearActive 不改。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/fileutil ./internal/state ./internal/cliutil -count=1`
Expected: 全通过；state round-trip 与 active clear/select 兼容。平台注释准确引用非 Unix Rename 保证限制。

- [X] **Step 5: 记录 RED/GREEN、检查 diff；不自动提交。**

## Task 3: 逐文件安装记录与冻结官方指纹

**Files:** Create `internal/scaffold/installstate.go`, `installstate_test.go`, `legacy.go`, `legacy_test.go`, `testdata/legacy/using-mulix.md`, `testdata/legacy/mulix-using-mulix.md`。

**Consumes:** Task 2 writer、existing baselinePath、managedFiles、host placeholder。
**Produces:** 上述 installState 数据结构、digest/验证/load/save/legacyVendorMatch；init 登记在 Task 4 备份设施完成后接入。

- [X] **Step 1: 冻结官方历史证据并写信任测试。**

从尚未改名的 `assets/skills/using-mulix/SKILL.md` 复制真实字节到 testdata；从历史 example 的官方副本读取第二份 fixture，但不修改 example。已从基线实际字节计算出以下指纹，生产 legacy.go 使用这一冻结表（只命中这三个明确路径）：

```go
var legacyVendorDigests = map[string]string{
    ".claude/skills/using-mulix/SKILL.md": "506b636fff71ad1ddd13b98c51a9a6e54016bff04cde17f866c678fe098f8b3c",
    ".dsh/skills/using-mulix/SKILL.md": "08ef0a159bd11f6bbd0caeaad50d9b44eac941b80a73890894ca6f609a006268",
    ".claude/skills/mulix-using-mulix/SKILL.md": "440e312e20ab151b578284bf7953ce58de1a1c1cd41a1f71f8f81f0a47781590",
}
func legacyVendorMatch(rel string, data []byte) bool {
    expected, ok := legacyVendorDigests[rel]
    return ok && contentDigest(data) == expected
}
```

注释记录基线commit和历史example来源；测试按host渲染当前旧名fixture并校验hash。生产不能读取examples/testdata，不能根据未来新技能反推旧正文，不能把CRLF/frontmatter归一化后算命中。

测试主干：

```go
func TestInstallState_DoesNotTrustArbitraryEqualLegacyBytes(t *testing.T) {
    root := t.TempDir()
    rel := ".mulix/templates/spec-template.md"
    writeProjectFile(t, root, rel, "CUSTOM\n")
    writeProjectFile(t, root, ".mulix/.installed/"+rel, "CUSTOM\n")
    st, err := loadInstallState(root)
    if err != nil { t.Fatal(err) }
    if _, ok := st.Files[rel]; ok { t.Fatal("legacy equality silently became trusted") }
}
func TestManagedRel_RejectsEscapesAndOverrides(t *testing.T) {
    for _, rel := range []string{"../outside", "/absolute", `C:\outside`, ".mulix/templates/overrides/custom.md", "AGENTS.md", ".mulix/.backups/x"} {
        if validManagedRel(rel) { t.Fatalf("accepted %q", rel) }
    }
}
```

同时测试损坏 JSON、未知 schema、非法 host/digest/pending path、JSON 后还有其他内容、B digest 变化、meta 文件为目录读取失败。增加现有精确 N 的 proof 单元测试，不在缺元信息时扫描并信任所有 B。新 init 的登记/跳过/host 并集测试归 Task 4，确保它从第一步就使用正式备份实现。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold -run 'Test(InstallState|ManagedRel|LegacyVendor)' -count=1`
Expected: 缺少新数据结构/函数，失败。

- [X] **Step 3: 实现小型记录，不做全库信任标记。**

```go
func contentDigest(data []byte) string {
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:])
}
func validManagedRel(rel string) bool {
    if rel == "" || path.Clean(rel) != rel || strings.ContainsAny(rel, "\\:") || path.IsAbs(rel) { return false }
    if strings.HasPrefix(rel, ".mulix/templates/overrides/") { return false }
    for _, prefix := range []string{".claude/skills/", ".dsh/skills/", ".mulix/templates/", ".mulix/superpowers/"} {
        if strings.HasPrefix(rel, prefix) && len(rel) > len(prefix) { return true }
    }
    return false
}
```

load 使用 Decoder 严格字段/单对象检查、初始化空 maps；missing 返回 schema=v1 空记录；save 验证后 MarshalIndent 并用安全 writer。登记纯 N 必须在 B=N 写入成功后；读取元信息不能返回「missing」吞掉其他错误。

此任务只完成记录和证明设施，不在 loader 中扫描整棵 baseline 自动登记，不为未知来源恢复猜测的官方祖先。init 的资源写入、备份与登记顺序在 Task 4 接入，避免创建以后再替换的第二套临时备份实现。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/scaffold ./internal/cliutil -count=1`
Expected: 原 install/update 行为的测试仍通过，新记录/路径/digest/legacy 拒绝测试通过；冻结 fixture 尚未受名称和文案修改影响。

- [X] **Step 5: 记录冻结指纹来源与 RED/GREEN，不自动提交。**

## Task 4: 保护性备份与 init 可信登记

**Files:** Create `internal/scaffold/updatebackup.go`, `updatebackup_test.go`；Modify `init.go`, `init_test.go`，将 `writeFileMode` 实现切换到 Task 2 writer，保留脚本要求的权限。

**Consumes:** Task 2/3。**Produces:** updateBackups.preserve、init 的可信记录和 hosts 并集。所有更新路径只复用这一个备份入口。

- [X] **Step 1: 写备份失败不覆盖及 init 不认领定制测试。**

```go
func TestInit_ForceStopsWhenBackupCannotBeCreated(t *testing.T) {
    root := t.TempDir()
    if _, err := Init(InitOptions{Root: root}); err != nil { t.Fatal(err) }
    rel := ".mulix/templates/spec-template.md"
    writeProjectFile(t, root, rel, "KEEP LOCAL\n")
    writeProjectFile(t, root, ".mulix/.backups", "not a directory\n")
    if _, err := Init(InitOptions{Root: root, Force: true}); err == nil {
        t.Fatal("force succeeded despite backup failure")
    }
    if got := readProjectFile(t, root, rel); got != "KEEP LOCAL\n" {
        t.Fatalf("local was overwritten: %q", got)
    }
}
func TestInit_SkippedCustomFileIsNotTrusted(t *testing.T) {
    root := t.TempDir()
    rel := ".mulix/templates/spec-template.md"
    writeProjectFile(t, root, rel, "CUSTOM\n")
    if _, err := Init(InitOptions{Root: root}); err != nil { t.Fatal(err) }
    st, err := loadInstallState(root)
    if err != nil { t.Fatal(err) }
    if _, ok := st.Files[rel]; ok { t.Fatal("skipped custom file was trusted") }
}
```

增加已存在精确 N 但旧 B 有定制的测试：校准前旧 B 在备份中；init dsh→claude 合并 hosts；备份内 L/B/原元信息都保留；重复 preserve 不覆盖首份快照；无危险操作不创建备份；备份 base 读取失败不是 missing。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold -run 'Test(Init_ForceStops|Init_SkippedCustom|UpdateBackup|Init_.*Trust|Init_.*Hosts)' -count=1`
Expected: 当前 force 未备份，目标被覆盖或注册断言失败。

- [X] **Step 3: 实现懒创建的运行备份与登记顺序。**

备份布局固定：`<run>/local/<rel>`、`<run>/baseline/<rel>`、`<run>/install-state.json`。运行目录用 `os.MkdirTemp(root/.mulix/.backups, "update-*")`；运行目录已存在时不选固定日期文件覆盖旧备份。首次 preserve 将旧元信息按原字节保存；各 rel 首次保存 L/B，之后同一 run 不覆盖首份快照。

实际复制的读取/写入规则：

```go
func copyBackupFile(src, dst string) error {
    info, err := os.Lstat(src)
    if os.IsNotExist(err) { return nil }
    if err != nil { return err }
    if !info.Mode().IsRegular() { return fmt.Errorf("backup %s: not a regular file", src) }
    data, err := os.ReadFile(src)
    if err != nil { return err }
    return fileutil.WriteFile(dst, data, info.Mode().Perm())
}
```

只有 preserve 全部成功后才能将 rel 加入 backed；任何一个备份失败返回错误。结果 Backups 存实际运行目录，CLI 可输出。为备份/冲突目录创建仅本目录生效的 `.gitignore`（`*\n!.gitignore\n`），不修改项目根 `.gitignore`，避免副本被意外提交。

init 对 managedFile 的明确序列：

1. 加载/验证 install-state，推断已有 host 后与 normalizeHosts(opts.Hosts) 合并，不将空集默认扩成双宿主。
2. 已有 L≠N 且非 force：跳过，保留已有 trust record 但不新增/改变它；使用前仍需 B digest 验证。
3. 已有 L=N：B=N 则登记 N；旧 B 不同且存在则先 preserve，再校准 B=N。
4. 缺 L 或 force 写入：不一致旧 B 或将覆盖已有 L 时先 preserve，再写 L=N、B=N、登记 digest。
5. 每个文件成功后 saveInstallState，完成项加入 InitResult，后续失败不冒充整体成功；seed 文件仍不登记为 updater 文件。`InitResult` 增加 `Backups []string` 供输出；备份器除 validManagedRel 外只接受 init 明确的 constitution/runtime-gitignore 两个 seed 路径，以保护 init --force 的种子覆盖，不能借此扩大 updater sweep。
6. 显式 host 选择在 init 成功安装必要资源后保存；添加另一 host 不丢历史 hosts。原 hook 哨兵合并语义不变。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/fileutil ./internal/scaffold ./internal/cliutil -count=1`
Expected: 新备份和 init 信任测试通过，已有幂等/双宿主/脚本测试通过。

- [X] **Step 5: 记录 RED/GREEN、备份内容路径与 diff；不自动提交。**

## Task 5: 普通 update 的可信 B/L/N 闭环、host 与 obsolete

**Files:** Create `internal/scaffold/updatefiles.go`, `updatefiles_test.go`；Modify `update.go`, `update_test.go`, `internal/cliutil/update_cmd.go`, `update_cmd_test.go`。此任务保留原 CLI 旗标，但立即使 Blocked/Conflicted 有可见汇总和非零结果，避免中间任务绿测依赖旧成功语义；Task 8 完成 accept 参数及完整 partial-error 输出。

**Consumes:** Task 2–4、managedFiles/merge3/splitLines。**Produces:** newUpdateRun、updateFile、commitVendor、NeedsAttention、对称 updateHosts 与可信 obsolete。

- [X] **Step 1: 先锁定连续更新与 DSH-only 的真实回归。**

```go
func TestUpdate_ThreeRunsKeepLocalAndPureBaseline(t *testing.T) {
    root := t.TempDir()
    if _, err := Init(InitOptions{Root: root}); err != nil { t.Fatal(err) }
    rel := ".mulix/templates/spec-template.md"
    vendor := readProjectFile(t, root, rel)
    local := vendor + "\nLOCAL RULE\n"
    writeProjectFile(t, root, rel, local)
    for i := 0; i < 3; i++ {
        if _, err := Update(UpdateOptions{Root: root}); err != nil { t.Fatal(err) }
        if got := readProjectFile(t, root, rel); got != local { t.Fatalf("run %d lost local: %q", i+1, got) }
        if got := readProjectFile(t, root, ".mulix/.installed/"+rel); got != vendor {
            t.Fatalf("run %d polluted baseline", i+1)
        }
    }
}
func TestUpdate_DSHOnlyNeverCreatesClaude(t *testing.T) {
    root := t.TempDir()
    if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil { t.Fatal(err) }
    if _, err := Update(UpdateOptions{Root: root}); err != nil { t.Fatal(err) }
    if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
        t.Fatalf("unexpected Claude tree: %v", err)
    }
}
```

将原 `x.md` 单元 fixture 改为 `.mulix/templates/unit.md`，用私有 run.updateFile(managedFile) 测试新 N。增加跨两个 incoming 版本的独立改动合并、纯 N 基线断言、B=M/L=B 的未知 legacy blocked、record digest mismatch blocked、B=N 可证明/登记后保留 L、L=N 校准旧 B 前备份。

旧 obsolete/DSH 测试直接写旧 B 的 fixture 分为两类：明确 trusted vendor fixture 同步安装记录；未知 legacy fixture 不登记，预期保留/Blocked。不能为了让旧测试通过重新无条件信任 L=B。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold -run 'TestUpdate_(ThreeRuns|DSHOnly|Legacy|Digest|Obsolete)' -count=1`
Expected: 第二次更新丢 LOCAL；DSH-only 新增 Claude；旧 equality fixture 未被保护。

- [X] **Step 3: 实现决策与编排，复用 merge，不写 M 到 B。**

trust 判定优先级固定：record digest 匹配实际 B → B=N 可验证 → B 命中冻结官方 legacy hash；L=N 可直接重建可信 N；其他 B 不可信。digest 不匹配不等于文件未安装，绝不能 fallback L=B 覆盖。

普通文件的动作选择实现为一个纯函数，再由 run.updateFile 执行对应操作；pending 与 alias 家族在 Task6/7 接管前置。缺L分支必须先于N=B，维持仍在发布文件被删除后可恢复的行为：

```go
type fileAction string
const (
    actionCurrent fileAction = "current"
    actionInstall fileAction = "install"
    actionForce fileAction = "force"
    actionBlocked fileAction = "blocked"
    actionUnchanged fileAction = "unchanged"
    actionReplace fileAction = "replace"
    actionMerge fileAction = "merge"
)
func chooseFileAction(exists, trusted, force bool, local, base, incoming []byte) fileAction {
    if exists && bytes.Equal(local, incoming) { return actionCurrent }
    if !exists { return actionInstall }
    if force { return actionForce }
    if !trusted { return actionBlocked }
    if bytes.Equal(base, incoming) { return actionUnchanged }
    if bytes.Equal(local, base) { return actionReplace }
    return actionMerge
}
```

每个动作的具体执行固定：current校准/登记纯N（不同旧B先备份，L不动）；install/force/replace调用commitVendor(rel,N,N,mode)；blocked记录no trusted baseline且L/B不动；unchanged只记Skipped；merge调用现有merge3，conflict保留L/B并分类（Task6增加正式候选与pending），无conflict调用commitVendor(rel,M,N,mode)。分支错误不得掉入force或直接覆盖。

commitVendor 的精确顺序：需要保护旧内容时 preserve → 安全写 L → 安全写纯 N 的 B → 设置 Files[rel].SHA256=contentDigest(N) → 保存元信息 → 分类完成结果。正常 no-op 不写 L、不生成备份；模式适配仅保持脚本可执行，不借 no-op 清空定制。操作失败返回具体 rel/阶段，不能撤掉之前已完成结果。

host 发现：验证 metadata.Hosts 与 baseline 技能足迹按 allHosts 稳定顺序合并；不存在的目录可忽略，其他 WalkDir error 返回；没有 record/baseline 时仅精确旧入口足迹识别 host；无证据则报「run mulix init --host」而不是默认 Claude。只有 top-level Update 需要 host 发现，普通文件单元 run 不依赖人为安装两宿主。

obsolete：仍取 baseline owned paths minus shipped，且排除 overrides/seed/runtime/backups/conflicts；只 trusted B 且 L=B 可默认删除。删除前 preserve，删除 L/B/记录的每一步错误均返回；L 已缺失可清记录，未知 B 或编辑内容列 Blocked 保留。force 只删除受管 baseline 或精确 alias 文件，未知辅助文件不递归删除。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/scaffold ./internal/cliutil -count=1`
Expected: 连续/跨版本与 host 回归通过。同步原无基线CLI测试为「文件保留、输出no trusted baseline、返回error」；原inline-marker测试改为「L/B不变、Conflicted分类」，Task6再加候选/pending断言。不能在Task5完整测试时保留互相矛盾的旧预期。

- [X] **Step 5: 记录两代 N/M/B 的实际字节、RED/GREEN，不自动提交。**

## Task 6: 冲突旁路、pending 与逐文件接受

**Files:** Create `internal/scaffold/updateconflict.go`, `updateconflict_test.go`；Modify `update.go`, `updatefiles.go`, `update_test.go`。CLI `--accept` 正式输出在 Task 8，但 UpdateOptions.Accept 此时可通过 API 测试。

**Consumes:** Task 3 pending schema、Task 4 backup、Task 5 run/commit。**Produces:** recordConflict、acceptPending；合法 stage 为 conflict/accepted-cleanup，后者供 Task 7 rename 恢复使用。

- [X] **Step 1: 写冲突不污染文件与显式接受测试。**

以 owned rel 建立可信 B=`a\nb\nc\n`、L=`a\nlocal\nc\n`、N=`a\nincoming\nc\n`；原内容与 B 在第一次/第二次冲突后必须逐字节相同。通过 run.updateFile 调用，并加载 pending 找候选，不能再断言活动文件包含 marker。

接受测试的关键断言：

```go
// fixture: trusted B, current L, incoming N，recordConflict 已写pending。
writeProjectFile(t, root, rel, "a\nresolved custom\nc\n")
run, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{rel}})
if err != nil { t.Fatal(err) }
if err := run.acceptPending(rel, managedFile{Rel: rel, Data: []byte("a\nincoming\nc\n")}); err != nil { t.Fatal(err) }
if got := readProjectFile(t, root, rel); got != "a\nresolved custom\nc\n" { t.Fatal("accept overwrote resolution") }
if got := readProjectFile(t, root, ".mulix/.installed/"+rel); got != "a\nincoming\nc\n" { t.Fatal("accept did not store pure vendor") }
if err := run.updateFile(managedFile{Rel: rel, Data: []byte("a\nincoming\nc\n")}); err != nil { t.Fatal(err) }
if run.result.NeedsAttention() { t.Fatalf("same N conflicted again: %+v", run.result) }
```

加 pending 但 L 缺失、N 改版本、L 仍含本工具 marker、无 pending 参数、重复 accept rel、候选已被人工编辑、元信息保存失败、下一 N 仍可合并的用例。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold -run 'TestUpdateConflict|TestAcceptPending' -count=1`
Expected: 缺少 pending 闭环或候选/保护行为失败。

- [X] **Step 3: 记录独立候选；accept 是唯一显式推进接口。**

候选目录为 `.mulix/.update-conflicts/<稳定内容标识>/`，保存 `merged.md` 和纯 N 的 `incoming.md`。内容标识对 length-prefixed B/N/L 字节做 SHA-256；现有同内容候选可复用，文件若被用户改动则用唯一新目录，不能覆盖它。保存候选成功后记录 stage=conflict、incoming digest、L digest、候选相对位置，saveInstallState，再把 rel 加 Conflicted。

Update 先校验 Accept 参数互斥/去重与托管路径，再处理 pending，pending 判定先于缺 L 和普通 merge。普通 update 遇 stage=conflict 原样报告；本轮 N 与 pending 版本不同时 Blocked，不生成覆盖旧候选的新祖先。显式force则先备份原L/B/meta，成功安装当前纯N并保存记录后才清pending；rename的force来源清理交Task7，未知辅助文件仍保留。

acceptPending 操作步骤精确如下：

1. 查找该 rel pending，普通接受必须 stage=conflict；current incoming hash 必须相同。Task7另为accepted-cleanup提供安全恢复，不让重复命令提前清除来源记录。
2. 读取目标普通文件 L，拒绝缺失及本工具完整 marker 行 `<<<<<<< incoming (mulix)` / `>>>>>>> local`；仅文本出现一般 `=======` 不应误判普通 Markdown。
3. 无 rename Sources：preserve旧B → 安全写 B=N → Files登记digest → 移除pending → saveInstallState → Accepted。L不重写。
4. 有 rename Sources：交给 Task 7 的统一 accepted-cleanup；不能提前移除pending。
5. 任意 I/O error 带路径返回；metadata写失败的retry需明确保留pending，不把失败当接受成功。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/scaffold -count=1`
Expected: 普通 conflict/accept 闭环、重试不覆盖和跨版本测试通过；未处理完的rename pending不提前清除。

- [X] **Step 5: 记录冲突前后 L/B/pending 字节与 RED/GREEN，不自动提交。**

## Task 7: 正式改名、旧入口迁移与恢复 checkpoint

**Files:** Create `internal/scaffold/updaterename.go`, `updaterename_test.go`；Move `assets/skills/using-mulix/SKILL.md` → `assets/skills/mulix-using/SKILL.md`；Modify `managed.go`（如需集中入口常量）, `update.go`, `agentsmd.go` 及 init/update/CLI 名称断言；所有 legacy fixture 保持 Task 3 冻结字节。

**Consumes:** Task 3 指纹、Task 5/6，host placeholder与现有哨兵合并。**Produces:** migrateEntry、恢复 accepted-cleanup、统一新入口名。

- [X] **Step 1: 写两代 alias 与碰撞/中断回归；编辑技能前加载 writing-skills。**

`superpowers:writing-skills` 仅在实施本 Task、设计和计划都已获批后加载；技能更新主要是名称和事实文案，不改变阶段纪律。基于 frozen fixture 按 host 渲染旧官方 L/B，覆盖以下具体矩阵：

| 来源/目标 | 预期 |
| --- | --- |
| using-mulix pristine，target absent | 新名正确name、旧已知文件移走、备份存在 |
| mulix-using-mulix known pristine，无B | 可验证旧vendor后迁移；不动历史example |
| 单一定制源，可信B，disjoint edit | 定制移到新名，B保存纯N；第二次update保留 |
| B=M污染或未知，无B未知源 | 全部保留、Blocked，不先装默认新入口绕过去 |
| 新目标已有CUSTOM | 保留两边，Blocked |
| 两个旧源均pristine | 统一新名，来源均备份后清理 |
| 一个可合并定制源 + 纯官方源 | 迁移唯一定制来源 |
| 多个定制源/任何未知源 | 不选赢家，不删除 |
| 旧目录附加用户文件 | 附加文件保留，无递归目录删除 |
| 来源冲突，用户accept新名 | 保留人工结果，纯N baseline，备份后清旧源 |
| cleanup第二个source删除失败 | pending=accepted-cleanup，retry按digest续作 |
| retry前新目标或未删旧源被编辑 | Blocked，不继续删除 |
| DSH迁移被阻塞 | AGENTS哨兵段不指向缺失新入口 |

代码断言至少包含：

```go
if got := readProjectFile(t, root, ".dsh/skills/mulix-using/SKILL.md"); !strings.Contains(got, "name: mulix-using\n") {
    t.Fatalf("wrong canonical frontmatter: %s", got)
}
if _, err := os.Stat(filepath.Join(root, ".dsh", "skills", "using-mulix", "SKILL.md")); !os.IsNotExist(err) {
    t.Fatalf("old known entry survived successful migration: %v", err)
}
```

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold ./internal/cliutil -run 'Test.*(Rename|Alias|MulixUsing|EntryMigration)' -count=1`
Expected: 新路径/兼容迁移/恢复 checkpoint 行为尚不存在。

- [X] **Step 3: 先决策整个入口家族，再允许写入与清理。**

集中常量与明确映射：

```go
const entrySkillName = "mulix-using"
var entryAliases = []string{"using-mulix", "mulix-using-mulix"}
func entryRel(h Host, name string) string {
    return path.Join(h.skillsDir(), name, "SKILL.md")
}
```

按批准设计选来源：逐字节 known pristine 或可信 B 的唯一修改源；未知/多定制/目标CUSTOM一律Blocked并将整个family标为handled，不让ordinary循环再创建默认新文件。known hashes只覆盖明确版本，frontmatter/换行不归一化推断vendor。

普通成功迁移和 rename accept 共用同一清理函数 `finishEntryMigration(target string, pending pendingUpdate) error`，该函数在本Task定义。checkpoint顺序：

```text
校验所有来源与目标 → 备份所有来源/已有目标/B/meta
→ 新目标L=M或N → B=N
→ 同次metadata保存：Files[target]=hash(N)，Pending[target]={stage:accepted-cleanup,
   incoming:hash(N), accepted:hash(targetL), backup_dir, sources快照}
→ 按快照删除仍存在且未改的旧L/B/旧记录
→ 移除pending并保存metadata
```

不能先清 pending 再删来源。checkpoint恢复验证新 L digest、B与**pending所存incoming digest**相同且可信、备份路径/内容存在，逐个验证尚存来源 digest；已删来源允许继续，后来修改来源不得自动删除。cleanup已被接受，允许先恢复旧checkpoint，再对当前较新N执行普通更新，不能跳过新版资源刷新。rename accept另验证人工目标frontmatter的name确为mulix-using，否则保留旧源并Blocked。

若 checkpoint 保存前失败，旧源必须尚存并给出人工处理路径，不声称跨文件事务已回滚。对显式force，重新备份当前精确alias与目标、建立当前N和新的来源快照后清理，不能拿陈旧snapshot删除后来改过的内容。

Update 顺序固定：accept/已有checkpoint → 各host migrateEntry → 未handled普通文件 → 排除所有handled alias路径的obsolete → 只有 entry ready 的DSH刷新AGENTS。否则保留当前哨兵段并报告原因。

源目录和frontmatter统一新名；改所有当前测试引用，禁止全局子串替换历史 `mulix-using-mulix`。此Task只切换必要入口引用；完整边界文案在Task11。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/scaffold ./internal/cliutil -count=1`
Expected: alias、定制、碰撞、checkpoint中断、DSH引导测试通过，legacy指纹fixture不变。

- [X] **Step 5: 验证 git diff 不含 examples，记录 RED/GREEN，不自动提交。**

## Task 8: CLI 的 accept、部分结果与诚实退出码

**Files:** Modify `internal/cliutil/update_cmd.go`, `update_cmd_test.go`；适当调整 `internal/cliutil/init_cmd.go` 显示备份路径（若 InitResult 增加 Backups，则与 Task4 同步该字段）；沿用 Cobra command construction和现有runPresetCLI测试入口。

**Consumes:** UpdateOptions.Accept、UpdateResult 新分类。**Produces:** 用户可用的逐文件accept与失败/部分完成报告。

- [X] **Step 1: 更新无可信基线的退出预期，新增冲突及参数测试。**

既有 `TestUpdate_SkipsFileWithoutBaseline` 改为：error非nil、文件字节不变、summary含blocked/no trusted baseline。普通可合并测试建立明确trusted B fixture，不以任意旧B充当vendor。

具体旗标测试：

```go
func TestUpdate_ForceAndAcceptAreMutuallyExclusive(t *testing.T) {
    dir := chdirTemp(t)
    initMulixRoot(t, dir)
    out, err := runPresetCLI(t, "update", "--force", "--accept", ".mulix/templates/spec-template.md")
    if err == nil { t.Fatalf("invalid flag combination succeeded: %s", out) }
}
```

加正常no-op成功、冲突非零、blocked非零、重复accept非零、非托管accept非零、候选人工解决后accept成功、I/O失败前已有written/merged分类仍输出、备份实际目录输出、列表超过5项时有明确总数且关键冲突路径可完整定位的测试。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/cliutil -run 'TestUpdate' -count=1`
Expected: 缺少--accept，旧CLI冲突/blocked返回nil，partial summary遗漏。

- [X] **Step 3: 参数预校验，输出结果后再返回状态。**

```go
var accept []string
cmd.Flags().StringArrayVar(&accept, "accept", nil, "accept a manually resolved pending update for one project-relative file (repeatable)")
// RunE里：
res, updateErr := scaffold.Update(scaffold.UpdateOptions{Root: root, Force: force, Accept: accept})
printUpdateResult(res)
if updateErr != nil { return updateErr }
if res.NeedsAttention() { return fmt.Errorf("update requires manual action; resolve the reported conflicts or blocked files") }
return nil
```

本Task定义 `printUpdateResult(res scaffold.UpdateResult)`，汇总六个既有分类与accepted/migrated/blocked，打印backup目录；不能只显示written计数却省略错误导致的具体完成路径。关键Conflicted/Blocked不按旧max5静默省略，普通already-current列表可继续截断。

CLI帮助说明update是当前二进制资源更新，不是self-update；--force会先备份再丢弃托管定制，--accept只针对pending且不信任未知legacy。

Update在任何写入前拒绝force+accept、重复accept、非法路径；这个约束在API侧也测试，不能只依赖Cobra。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/cliutil ./internal/scaffold -count=1`
Expected: honest summary/exit semantics与API相同，正常no-op/accept成功。

- [X] **Step 5: 记录退出码、部分输出和 RED/GREEN，不自动提交。**

## Task 9: 新建变更目录改为日期型ID

**Files:** Modify `internal/scaffold/newchange.go`, `newchange_test.go`, `internal/cliutil/new.go` 的当前命名注释；Create `internal/cliutil/new_test.go`，复用现有CLI测试fixture。state/guard/hook只增加新旧ID兼容测试，不增加编号/日期格式限制。

**Consumes:** 现有Slugify、layout目录定义、Task2安全state writer。**Produces:** 保持公开 `CreateChangeDir(root,title) (NewChange,error)`，新增私有 `createChangeDirAt(root,title string, now time.Time) (NewChange,error)` 供确定性测试；NewChange三目录/Change均为同一日期型ID。

- [X] **Step 1: 写固定日期、同名碰撞及旧ID兼容测试。**

```go
func TestCreateChangeDirAt_DateIDAndCollision(t *testing.T) {
    root := t.TempDir()
    now := time.Date(2026, 10, 14, 0, 15, 0, 0, time.FixedZone("UTC+8", 8*60*60))
    first, err := createChangeDirAt(root, "Add Login Flow", now)
    if err != nil { t.Fatal(err) }
    if first.Change != "2026-10-14-add-login-flow" { t.Fatalf("id=%q", first.Change) }
    second, err := createChangeDirAt(root, "Add Login Flow", now)
    if err != nil { t.Fatal(err) }
    if second.Change != "2026-10-14-add-login-flow-2" { t.Fatalf("collision id=%q", second.Change) }
    for _, rel := range []string{first.SpecDir, first.ChangeDir, first.RuntimeDir, second.SpecDir, second.ChangeDir, second.RuntimeDir} {
        info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
        if err != nil || !info.IsDir() { t.Fatalf("missing directory %s: %v", rel, err) }
    }
}
```

增加：同日不同slug无全局数字序号；第二天同slug使用第二天日期；只有specs或changes已有同名时不得复用；只有runtime已有同名也不得复用；旧001-*目录不重命名且不影响日期型分配；不可用slug错误；父目录为普通文件时返回error不无穷重试。CLI创建新变更后active、state.Change和三目录一致，仍可select已有001-* v2变更并做合法guard/transition。

- [X] **Step 2: 验证 RED。**

Run: `go test ./internal/scaffold ./internal/cliutil -run 'Test(CreateChangeDir|New|DateChange)' -count=1`
Expected: 旧代码缺createChangeDirAt且仍输出001-*；日期命名与碰撞断言失败。

- [X] **Step 3: 统一日期候选，保留文档文件名。**

```go
func CreateChangeDir(root, title string) (NewChange, error) {
    return createChangeDirAt(root, title, time.Now())
}
func changeCandidate(date, slug string, attempt int) string {
    base := date + "-" + slug
    if attempt == 1 { return base }
    return fmt.Sprintf("%s-%d", base, attempt)
}
```

createChangeDirAt捕获 `now.Format("2006-01-02")`，不用UTC转日，也不在循环中重复取时间；Slugify与空slug拒绝不变。每个候选先检查specs/changes/runtime三个同名路径，任意已存在都换下一候选；读取错误不是不存在。以单次 `os.Mkdir(runtimeAbs,0755)` 预留ID，EEXIST才重试，不用MkdirAll吞掉碰撞；随后创建该次专属spec/change目录。仅清理本调用新建且仍为空的目录，不递归清旧数据，不声称跨目录事务或多会话锁。

已核实NextNumber、旧ChangeID及NewChange.Number只用于本文件/相关旧测试；移除未再使用的编号分配、numberedDirRe、strconv和Number字段，用上述日期/碰撞测试替换旧NextNumber测试。NewChange保留Slug、Change及目录字段，目录注释改为日期示例。

不遍历迁移旧001-*目录，不将所有旧fixture替换成日期；保留旧ID fixture作为兼容测试。内部仍生成空spec/change/runtime目录，spec.md/tasks.md/report.md由原阶段技能按原约定产出。

- [X] **Step 4: 验证 GREEN。**

Run: `go test ./internal/scaffold ./internal/cliutil ./internal/state ./internal/guard ./internal/hook -count=1`
Expected: 日期命名通过，旧v2 change加载/select和所有阶段路径测试仍通过，state schema保持v2。

- [X] **Step 5: 记录本地日期/碰撞/旧ID证据，不自动提交。**

## Task 10: 恢复步骤checkbox与真实完成标记

**Files:** Modify `assets/templates/tasks-template.md`, `assets/skills/mulix-tasks/SKILL.md`, `assets/skills/mulix-build/SKILL.md`；添加安装模板的输出契约与guard解析回归测试。不修改内嵌superpowers（其writing-plans已有checkbox步骤）。

**Consumes:** 原Task N提取/证据检查、用户新增明确格式要求。**Produces:** 未完成 `- [ ] **Step N: ...**`，实际完成 `- [X] **Step N: ...**`；只改标记、不改已批准任务正文的执行纪律。

- [X] **Step 1: 写真实安装输出与解析兼容测试。**

通过Init读取安装后的tasks-template，验证示例Step使用未完成checkbox；guard以含[ ]与[X]步骤、围栏内示例的tasks文档测试TaskNumbers不受状态标记影响。技能消费者评审检查未执行/提交步骤不得被批量标记完成。

- [X] **Step 2: 运行RED。**

Run: `go test ./internal/scaffold ./internal/guard -run 'Test.*(TaskTemplate|StepCheckbox)' -count=1`
Expected: 当前安装模板没有checkbox步骤，新输出契约失败；解析兼容测试可作为已有行为characterization通过，不声称它首次失败。

- [X] **Step 3: 修改模板与执行纪律。**

模板步骤改为 `- [ ] **Step N: ...**`，header说明完成后[X]；mulix-tasks移除plain-bold/no-checkbox覆盖。mulix-build明确每步真实执行和验证后才能把该步改为[X]，允许这种状态标记编辑但禁止改任务编号/定义/批准内容；台账、报告仍保留，不能用checkbox代替证据。本轮实施计划同样在各步骤实际完成后标为大写[X]。

- [X] **Step 4: GREEN。**

Run: `go test ./... -count=1`
Expected: 模板输出通过，TaskNumbers和阶段门禁仍通过；不增加checker或自动全勾脚本。

- [X] **Step 5: 记录证据与diff，不自动提交。**

## Task 11: 边界文案、真实 CLI smoke 与最终验收

**Files:** Modify `README.md`, `internal/cliutil/root.go`, `state_cmd.go` help、`internal/hook/hook.go` 注释、`internal/flow/flow.go` 注释、`internal/state/state.go` 注释、`internal/scaffold/agentsmd.go` 文案、自有入口/design/archive技能的相关事实句、`scripts/smoke-e2e.sh`、`docs/PLAN.md`；Create `scripts/smoke-update.sh`。不改vendor superpowers及examples。

**Consumes:** Task 1–9。**Produces:** 准确当前契约、临时项目集成回归与独立whole-branch review输入。

- [X] **Step 1: 先写集成断言及名称/门禁文案检查。**

smoke-update在独立临时目录构建当前CLI，安装DSH-only，向模板追加唯一LOCAL marker，连续3次update，逐次读取本地/基线断言。使用Git Bash+标准命令（Windows不依赖chmod位）；脚本异常停止且临时目录由trap清理，不修改真实项目，不从untrusted目录执行解释器。

关键可执行断言：

```bash
mulix init --dir "$project" --host dsh >/dev/null
printf '\nSMOKE_LOCAL_CUSTOMIZATION\n' >> "$project/.mulix/templates/spec-template.md"
for n in 1 2 3; do
  (cd "$project" && mulix update >/dev/null)
  grep -q SMOKE_LOCAL_CUSTOMIZATION "$project/.mulix/templates/spec-template.md"
  ! grep -q SMOKE_LOCAL_CUSTOMIZATION "$project/.mulix/.installed/.mulix/templates/spec-template.md"
done
test ! -e "$project/.claude"
test -f "$project/.dsh/skills/mulix-using/SKILL.md"
```

同一脚本添加已知旧入口fixture迁移和再次update、未知旧源保持/非零、旁路冲突/accept。涉及模拟不同N的复杂场景复用Go集成测试中的trusted fixture，不通过修改真实assets临时重建多版本。冻结legacy fixture注入临时项目无需执行其中技能脚本。

- [X] **Step 2: 验证 RED。**

Run: `bash scripts/smoke-e2e.sh && bash scripts/smoke-update.sh`
Expected: 在名称断言或新smoke未接好之前失败；若新smoke首次已绿仍须依赖前面Tasks已记录的功能RED，不伪造失败证据。

- [X] **Step 3: 同步事实文案并完成smoke。**

README新增明确边界段、逐文件legacy保留、冲突候选路径、人工放入目标后--accept步骤、force备份恢复位置、非零含义、Windows非绝对原子/无锁/无事务。AGENTS与自有入口技能表达同一边界：Write/Edit覆盖，Bash/MCP/其他写入未覆盖，异常/缺路径fail-open，DSH纪律+guard，不称安全沙箱。

CLI root Short同时提及支持的agent hosts；state --force仍说明仅绕过readiness、不改变合法阶段。flow.Apply的注释不再说hook负责调用它（实际是CLI），hook注释不再声称agent不能绕过。写文案不改变policy/map代码。

smoke-e2e把现行入口存在断言改为mulix-using；`mulix new`后用 `IFS= read -r c < .mulix/active` 获取实际日期型changeID，断言日期型格式，不再硬编码001-add-greeting。七阶段证据与allow/deny检查原样保留，新增重复标题的-2分配断言放在独立临时变更中，避免打断七阶段active。README示例说明本地日期、同名后缀、旧ID仍可select。docs/PLAN.md追加本轮完成/验证条目，不改旧Phase记录；执行后补充实际结果，不提前写完成。

- [X] **Step 4: 全量验收与重跑原始复现。**

```bash
go test ./... -count=1
go vet ./...
go build ./...
gofmt -l internal cmd
bash scripts/smoke-e2e.sh
bash scripts/smoke-update.sh
git diff --check
git status --short
```

Expected: 所有命令exit0；gofmt无未格式化文件；diff不含examples、vendor superpowers、preset resolver或用户配置。Go tests增加并确认Task1–8关键用例确实执行，不以cached结果替代全量验收。

使用临时项目重跑原始四项探针：本地编辑第2次仍保留、活动文件不含冲突marker且退出非零、DSH-only无Claude、preset resolve仍使用原core（最后一项是明确未改变的范围，不声称修复）。

如IDE已打开本仓库，调用IDE build/lint验证；当前IDE仅识别另一个ledger项目，不能对错误项目构建或伪称IDE验证完成，记录不可用并以上述Go命令验证本仓库。

- [X] **Step 5: 独立整分支review与交付。**

加载 `superpowers:requesting-code-review`，由一名fresh reviewer聚焦B/L/N信任、pending/checkpoint、部分失败、路径范围、未授权删除与spec符合性；确认发现后以失败测试修复并重新全量验证。加载 `superpowers:verification-before-completion`，最终只报告实际执行证据。

交付摘要列出：新名称、legacy被阻塞时处理方法、--accept/--force、备份位置、平台保证边界、测试结果和任何未完成步骤。无需用户要求不提交、不推送、不创建PR。

## 依赖与执行方式

Task1独立；Task2是Task3–8的writer基础；Task3冻结证据必须早于Task7/11技能文案变更；Task4备份必须先于任何不一致旧B替换；Task5→6→7→8共享安装记录和updater接口，按顺序实施，不并行修改共享scaffold文件。Task9实现日期型目录，Task10按用户追加要求恢复步骤checkbox；Task11在全部功能验证后执行。

用户已选择 **Native / 当前会话逐任务实施 + 最后独立review**。补充后共11个任务；各任务记录RED/GREEN并在真实步骤完成后将[ ]改为[X]。不再询问执行方式，不用Workflow工具扩大规模。

## 计划自审结果

- Spec §3→Task1，§4→Task2，§5→Task11，§6→Task3/4，§7→Task4/5/8，§8→Task6/8，§9→Task7，§10→Task11；日期目录补充→Task9/11，步骤checkbox补充→Task10/11。
- 没有模糊legacy猜测、全库trusted marker、额外preset变更或example迁移。
- 接口名一致；`finishEntryMigration`由Task7定义，`printUpdateResult`由Task8定义，其他公共/私有跨任务接口在顶部列出。
- rename checkpoint在metadata中与目标vendor登记同次保存；清旧源完成前不清pending，覆盖了失败后无法重试的风险。
- Review Focus五类都分配了具体测试；错误注入跨平台，不依赖只读目录在Windows上的行为。
- 设计与实施计划已获用户批准，按Native执行；后台agent通知不是新的用户授权。

## 实施验收附录（2026-10-15）

11个Task已完成。一次fresh整工作区review发现5个Important，均在本会话单次RED→GREEN修复，未派发二次review：Windows大小写identity与metadata/override边界、普通canonical pending优先于alias、obsolete force删除后metadata失败的显式续作、同文件部分写入/删除的结果可见性、辅助ignore CRLF兼容。

接口补充：`UpdateResult`增加`Partial []string`，`NeedsAttention`也覆盖Partial；CLI完整打印partial路径、已完成操作及失败stage，不把尚未完成的whole-file commit记为成功。其余批准接口与七阶段/schema不变。当前代码及测试是此补充的实现记录，原任务定义未改写。

最终验收：8个测试包，338个通过test/subtest事件，0失败；go vet/build、两套真实CLI smoke、diff --check和规范化换行后的gofmt通过。原有32个CRLF-only格式化条目不批量改写。无review minor延期项。preset core来源、锁/并发隔离、跨文件事务、安全沙箱及额外平台保证仍按原范围排除。未经请求未commit/push/PR/merge；验证台账、报告及日志保留在ignored执行工作区。
