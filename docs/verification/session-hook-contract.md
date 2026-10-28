# 会话 hook 集成契约与验证范围

日期：2026-10-28

## 已知契约与来源

| 项目 | 约定 | 证据与限制 |
| --- | --- | --- |
| PreToolUse | stdin JSON 中 session_id、cwd 标识本次会话/目录 | 当前项目已有 cwd 解析；官方文档检索片段确认通用 session_id。协议测试不等于真实宿主测试。 |
| SessionStart 输出 | hookSpecificOutput 下 hookEventName=SessionStart、additionalContext | hooks reference 检索片段包含此 JSON；保留该最小输出，不依赖其他扩展字段。 |
| 环境注入 | 仅在宿主给出 CLAUDE_ENV_FILE 时追加 shell export MULIX_SESSION_ID | 官方环境变量参考检索片段说明该文件在 Bash 命令前加载；不推断 Terminal/MCP/子代理均继承。 |
| 生命周期 | SessionStart 可用于会话启动/恢复 | 不安装 SessionEnd 清理。新增 hook 使用无 matcher 的通用 SessionStart 注册，避免未经核实的来源枚举限制。 |
| 超时 | 资源锁最多 5 秒；hook 注册明确给出 15 秒 timeout | timeout 为 command hook 配置约定；真实宿主是否执行该值仍需验证。不能将这层超时当作不可绕过的安全边界。 |
| 身份缺失 | 显式 --session-id 是稳定基础接口 | 没有宿主环境文件时，仅提示显式参数；不伪造父会话 ID，不缓存 change。 |

来源：
- [Hooks reference](https://code.claude.com/docs/en/hooks)
- [Environment variables reference（官方）](https://code.claude.com/docs/ko/env-vars)
- [Hooks reference 检索结果入口](https://t.co/PgX6K1xNQc)

直接获取 code.claude.com 文档被当前工具的域名验证/网络限制阻止。上述信息区分官方检索片段与协议约定，不宣称已阅读全文或完成实时契约验证。

## 真实宿主状态

| 环境 | 当前状态 |
| --- | --- |
| Windows 原生 Go CLI / Git Bash 本地协议测试 | 已通过本机协议、跨进程锁、三组 smoke 与隔离 vendor 再生成验证；不等于真实宿主触发验证 |
| Claude Code CLI 的自动环境注入 | 2026-10-28 在隔离临时项目用真实 CLI 2.1.312 实测：SessionStart 触发，未提供 `CLAUDE_ENV_FILE`；模型调用因隔离配置未登录而失败，故环境注入与 PreToolUse 仍未验证 |
| Claude 桌面端 / 恢复会话 | 未实测；不推断与 CLI 一致 |
| 子代理身份/继承 | 未实测；独立 session_id 按独立身份处理 |
| Linux/macOS 原生锁和脚本 | 本机为 Windows，不能冒充已验证 |

SessionStart 适配仅写宿主显式提供的环境文件，并按协议返回上下文。真实项目/全局 settings、本会话当前配置均不改动。测试只在临时项目生成配置和模拟 payload；不启动任何付费模型会话。

## 实施与审阅结果

- 功能分支 `feat/session-bound-write-gate`；改动已提交为 `bd64722`，未推送。未安装到真实项目，未修改真实宿主配置。
- 一次独立整分支审阅覆盖当时 58 个已修改或新增文件，哈希全部匹配；审阅时尚未提交，因此按工作区快照审阅而非提交区间。
- 审阅发现的三项问题均已独立复现、修复并验证：Windows 路径别名绕过控制目录；legacy 模式锁故障时误阻断；绑定有效时 status 隐藏无效全局上下文。第三项原标为 Minor，因违反诊断契约且会掩盖未绑定会话故障，按 Important 修复。
- 另补充绑定/配置文件为符号链接的消费端测试；临时撤去链接保护后测试失败，恢复代码后通过，原文件字节与审阅前一致。
- Windows 会话模式拒绝路径组件末尾的点或空格，并对相对/绝对路径应用相同的物理路径策略；少见扩展路径需使用无歧义拼写。
- 锁失败时，hook 不加锁地严格读取模式文件，仅用于判断已确认的 legacy 是否应 fail-open，不用该提示选择变更。并发切换模式时该提示可能反映较早的策略快照。
- `status` 现在单独验证全局上下文；即使当前绑定有效，全局目标损坏/缺失/归档也会报告错误并返回非零。

## 最终本机验证

| 检查 | 结果 |
| --- | --- |
| `go test ./... -count=1 -json` | 10 个有测试的包，493 个通过的测试/子测试事件，0 失败；Windows 平台相关 2 项跳过：POSIX 文件权限与脚本可执行位。事件计数包含父测试及其子测试，不是 493 个彼此独立的用例。 |
| `go vet ./...` | 通过，输出为空。 |
| `CGO_ENABLED=1 CC=clang go test -race ./internal/lockfile ./internal/sessionctx ./internal/state ./internal/cliutil -count=1` | 四个包通过。默认配置 CGO=0 时不能运行竞态检测；重跑只为该进程临时使用本机已有 clang，未修改用户环境配置。 |
| 七阶段、会话并行、资源更新 smoke | 三组均通过，均在临时项目运行；会话 smoke 的 `--force` 仅构造阶段 fixture，不作为 guard 就绪性的证据。 |
| Vendor 同步 | 用本机匹配的上游 superpowers 7.0.0 在隔离临时目录实际重新生成，随后执行生成的两个脚本，通过。 |
| Linux amd64 / Darwin arm64 | 交叉构建通过；并未在这些系统上运行测试。 |

C01–C24 已分别关联到实际通过的测试；其中 C19 已有真实 CLI 的 SessionStart 证据，**PreToolUse 与自动环境注入没有真实宿主证据**。详细测试名及 RED/GREEN 日志保留在本项目的忽略目录 `.superpowers/sdd/2026-10-28-session-bound-write-gate/`，作为执行记录保留，不删除。

运行产物路径测试执行真实 Bash 脚本和真实 mulix CLI，但用 Node 进程探针替代外部 HTTP 服务器；它验证目录和环境变量归属，不覆盖未修改的 HTTP 服务内部。跨进程测试覆盖确定的竞争交错，但不能证明所有调度情况。

## 验收映射（Windows 本机）

每项至少关联一项实际通过的相关测试；这不代表覆盖所有平台或所有调度。C19 只有协议证据，真实宿主未测。

| 设计编号 | 本机测试或 smoke |
| --- | --- |
| C01 | `TestResolve_LegacyIgnoresBindingsAndModeRoundTripPreservesThem` |
| C02 | `TestResolve_BoundAndUnboundAreIndependent` |
| C03 | `TestResolve_BoundAndUnboundAreIndependent`, `TestSessionScripts_GlobalSwitchOnlyAffectsNewInvocation` |
| C04 | `TestHookEval_SessionBindingAndDynamicFallback` |
| C05 | `TestResolve_InvalidBindingNeverFallsBack`, `TestControlMetadata_RejectsBindingAndConfigSymlinks` |
| C06 | `TestResolve_StateMismatchAndErrorsAreNotFallback`, `TestResolve_ArchivePhaseIsNotArchived` |
| C07 | `TestSelect_UnbindAndFailureRecovery` |
| C08 | `TestContextCLI_UnbindReportsRemovalBeforeFallbackError` |
| C09 | `TestNewSelect_SessionLegacyAndGlobalMatrix` |
| C10 | `TestNew_LegacyNumberedChangeCanStillSelectAndTransition` |
| C11 | `TestResolve_ExplicitChangeCannotBypassMutationContext`, `TestStateMutation_ExplicitChangeCannotBypassBindingOrFallback` |
| C12 | `TestContextCLI_ShowFormatsAndEmptyContext`, `TestHookEval_NoActiveAndLegacyErrors` |
| C13 | `TestResolve_ConfigAndActiveErrorsAreNotMissing`, `TestHookEval_LegacyLockFailureStillFailsOpen`, `TestHookEval_ConfirmedLegacyTimeoutFailsOpen`, `TestStatus_BoundSessionStillReportsInvalidGlobalContext` |
| C14 | `TestUpdate_SerializesReadModifyWriteAcrossProcesses`, `TestUpdate_ReloadsPhaseBeforeTransition` |
| C15 | `TestUpdate_DifferentChangesDoNotShareLongGuardLock` |
| C16 | `TestSessionCLI_ArchiveRaceDoesNotClearNewActive` |
| C17 | `TestSessionScripts_TwoBindingsProduceSeparateRuntimeDirs` |
| C18 | `TestResolve_ProjectAndNestedRootIsolation`, `TestHookEval_RequestCWDWinsAndMissingCWDUsesProcess`, `TestSessionScripts_NestedRootAndSpaceInPath` |
| C19 | `TestSessionStart_OnlyExportsIdentityAndDoesNotBind`, `TestSessionStart_NoEnvFileHasExplicitParameterGuidance`（真实 CLI 仅证实 SessionStart；PreToolUse 与自动环境注入未验证） |
| C20 | `TestInit_SessionStartMergeIsIdempotentAndPreservesUserHooks`, `TestUpdate_ReportsMissingSessionIntegrationWithoutEditingSettings`, `smoke-update.sh` 的 DSH-only 场景 |
| C21 | `TestDecideContext_ProtectsControlPathsBeforeBuildAllowance`, `TestDecideContext_WindowsAliasesAndCaseUseSamePolicy`, `TestDecideContext_WindowsTrailingAliasesNeverReachControlMetadata`, `TestDecideContext_RelativeAliasHasSamePolicyAsAbsolute` |
| C22 | `TestControlPath_RejectsTraversalSymlinkAndNonRegularTarget`, `TestIdentityAndChangeValidation`, `TestControlMetadata_RejectsBindingAndConfigSymlinks` |
| C23 | `TestLock_KilledOwnerReleasesAndFileRemainsReusable`, `TestSelect_UnbindAndFailureRecovery`, `TestUpdate_AfterSaveFailureReportsSavedState` |
| C24 | `TestVendorPatches_ResolveContextInsteadOfReadingActive` |

## 真实 Claude CLI 临时项目实测（2026-10-28）

真实 CLI 版本 2.1.312。测试只在一个新建临时项目、`--restricted` 模式、显式临时 settings 和独立临时配置目录中运行；没有复制凭证，没有安装到现有项目，也没有改动全局设置。

| 观察项 | 结果 |
| --- | --- |
| 两个真实 CLI 会话并发启动 | 是（各自独立 session ID，单次预算上限 $0.25，禁用会话持久化） |
| SessionStart 实际触发 | 两个会话均触发 |
| SessionStart 的 session ID、cwd、绑定来源与阶段 | 两个会话均与各自绑定一致（来源 `session-binding`，阶段分别为 design 与 build） |
| 宿主提供 `CLAUDE_ENV_FILE` | 否，因此无法观察自动环境文件写入 |
| Bash 继承 `MULIX_SESSION_ID` | 未验证 |
| PreToolUse 实际 Allow/Deny | 未触发：模型调用返回 `api_error`（CLI 报告 `total_cost_usd=0`、0 tokens） |
| 原项目工作树与已变更文件指纹 | 前后一致 |
| 指定全局/项目设置与 Git 元数据 | 前后一致 |

失败原因：独立配置目录中的 CLI 登录状态为未登录（`authMethod=none`），桌面应用使用的认证不会被独立 CLI 继承。为避免复制或提取用户凭证，没有为测试另行注入认证。

因此：**SessionStart 集成有真实宿主证据；自动环境注入和 PreToolUse 的真实宿主行为仍未验证。** 2026-10-28 用户已接受这一部分验证结果，明确不以此轮临时测试替代 PreToolUse 与环境注入证据。 两个会话的阶段由真实 `mulix` 构建和真实 `state transition` 守卫构造（未使用 `--force`）。原始 CLI 轨迹只保存在临时测试目录，不进入仓库；临时目录在本轮结束后未删除，可由用户确认后移除。

## 保留边界

- 相同工作目录的源码写入、分支和外部资源不自动隔离；多个会话仍须协调或使用独立工作区。
- 门禁仅覆盖 `Write/Edit`，不是安全沙箱；并发文件替换、硬链接、其他写入工具仍可绕过工作流约束。
- Guard 核查已记录证据的结构，不认证人工批准或实际测试执行；状态、绑定与 active 也不是跨文件事务。
- 已有显式绑定失效不会自动切换全局；宿主未传递会话身份时必须使用明确参数，否则使用全局上下文。
