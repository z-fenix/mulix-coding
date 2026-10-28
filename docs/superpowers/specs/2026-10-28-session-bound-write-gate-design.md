# 写入门禁多会话绑定设计

日期：2026-10-28
状态：设计与实施计划已获批准；Native 实现已完成一次独立整分支审阅并修复发现。本机验证通过；真实 CLI 已证实 SessionStart 与 PreToolUse；自动环境注入未验证，Unix 原生运行未验证；已合并到 `main`（`00376be`）。

## 1. 意图与确认范围

支持同一个工作目录中的多个会话分别处理不同变更，不让一个会话选择、新建或归档变更时改变其他已绑定会话的门禁依据与产物归属。

用户已确认的核心规则：

- 使用“会话 ID → change”显式绑定，阶段仍属于 change，不复制为会话级状态。
- 未绑定会话动态回退到项目全局 `.mulix/active`；不在首次读取或写入时自动生成绑定。
- 只有绑定记录不存在才回退；绑定损坏、读取失败、目标不存在、状态无法加载或目标已归档时，不改用其他 change。
- 已绑定会话不受全局 active 切换影响；解绑后恢复跟随全局 active。
- 设计整理阶段仅编写正式设计，没有修改功能代码；后续用户已单独批准 Native 实现。

本文件中命令接口、配置格式、锁方案和迁移细节是设计默认决策，不冒充用户逐项指定；由用户随正式设计一起审阅。

## 2. 目标、非目标与成功条件

### 2.1 目标

1. hook、CLI、自有技能、SDD 与脑暴脚本用同一套变更解析规则。
2. 保留无会话身份调用和旧项目的使用方式，支持逐步启用会话绑定。
3. 明确区分有效绑定、无绑定回退和上下文故障，不用宽泛异常捕获实现回退。
4. 为绑定、全局 active 和 change 状态的并发修改提供跨进程互斥。
5. 显示上下文来源，让用户能判断当前是在独立绑定还是跟随全局。

### 2.2 非目标与反目标

- 不新增阶段，不改变 `mulix.state.v2`，不改变 guard 的证据含义。
- 不创建或切换 Git 分支、Worktree，不承诺同目录下不同变更的源码无冲突。
- 不增加源码路径认领、文件写入锁或工具执行期间的文件系统事务。
- 不把 Bash、MCP、NotebookEdit 等工具加入本轮自动写入检查。
- 不实现同一 change 的多会话产物分工；多个会话指向同一 change 时共享阶段，源码与执行产物仍可能冲突。
- 不猜测会话身份，不扫描其他会话的对话内容，不按最近活跃时间自动认领 change。
- 不以“提高可用性”为由在显式绑定失效时换用全局 change。
- 不让普通资源更新或 `--force` 清空用户的上下文配置、绑定和运行记录。

成功条件：A、B 分别绑定不同 change 时，阶段判断和运行目录互不串线；未绑定 C 仍能动态跟随全局；绑定故障有明确失败而非隐式降级。

## 3. 当前实现依据

| 位置 | 当前行为 | 本轮变化 |
| --- | --- | --- |
| `internal/state/root.go` | `.mulix/active` 是唯一默认 change 来源 | 保留全局接口，增加会话上下文解析 |
| `internal/cliutil/root.go` | `resolveChange` 仅处理 `--change` 与 active | 区分查询与修改，并复用统一解析器 |
| `internal/cliutil/new.go` | 新建后始终 `SetActive` | 按模式、会话身份和 `--global` 选择写入目标 |
| `internal/cliutil/state_cmd.go` | select 修改全局，状态修改无跨进程锁 | 增加会话选择语义和读改写互斥 |
| `internal/cliutil/hook_cmd.go` | 用进程目录找根，读取全局 active，错误 fail-open | 会话模式使用请求上下文，实施分类型错误策略 |
| `internal/hook/hook.go` | 不解析 session_id；仅检查 Write/Edit；build 对普通源码不限制 | 接收 session_id，保留阶段白名单，增加会话模式控制文件保护 |
| `internal/fileutil/atomic.go` | 安全替换文件，但不是锁或多文件事务 | 复用写入方式，在完整读改写操作外加互斥 |
| `assets/.../scripts/sdd-workspace` | 直接读取 active | 调用统一上下文查询 |
| `assets/.../scripts/start-server.sh` | 直接读取 active；自建 SESSION_ID 作为脑暴实例编号 | 区分宿主会话与服务实例，调用统一上下文查询 |
| `scripts/sync-superpowers.sh` | 生成上述 active 读取补丁 | 同步更新生成规则，防止重新 vendor 后回退 |
| `internal/scaffold/init.go` | 安装 PreToolUse hook | 幂等合并必要的 SessionStart 入口 |
| `internal/scaffold/update.go` | 普通 update 不修改 Claude settings | 保留这一边界，用显式 init 完成新增 hook 安装 |

以上为源码检查，不代表新行为已实现或宿主集成已经实测。

## 4. 项目模式与本地数据

### 4.1 项目模式

新增 `.mulix/context.json`：

```json
{
  "schema": "mulix.context.v1",
  "mode": "session"
}
```

- `mode` 仅允许 `legacy`、`session`。
- 文件不存在等同 `legacy`；新 init 和已有项目升级都不自动启用 session。
- `legacy` 保持现有全局解析、select/new 语义和 hook fail-open 行为；即使宿主提供 session_id，也不读取会话绑定。
- `session` 启用本设计的绑定优先和未绑定回退规则。
- 配置存在但损坏、未知 schema、未知字段或不可读时报告错误，不假装文件不存在后降为 legacy。
- 切回 legacy 不删除绑定；以后重新启用 session 时恢复读取，并重新验证目标。

### 4.2 会话绑定

每个项目保存自己的 `.mulix/sessions/<session-key>.json`，不采用所有会话共写的集中式 JSON。

```json
{
  "schema": "mulix.session.v1",
  "session_id": "host-session-id",
  "change": "2026-10-28-add-login",
  "bound_at": "2026-10-28T08:00:00Z"
}
```

- session-key 为原始会话 ID 的 SHA-256 十六进制值；会话 ID 作为不透明、区分大小写的值，不按路径或 UUID 重新解释。
- 拒绝空白身份、控制字符及不合理长度；具体最大长度由实施者决定并形成测试，不把 ID 直接拼入 shell 或文件路径。
- 读取时验证 schema、字段、原始 ID 与请求一致、change 名称有效，并加载目标状态；状态内 Change 必须等于请求目标。
- change 必须是合法单一路径段，拒绝分隔符、绝对路径、`.`、`..`、保留名 `_shared` 与 Windows 危险路径形式；保留已有日期型和旧 `001-*` ID 的兼容性，不批量重命名。
- 严格区分文件不存在与权限错误、坏 JSON、目录、符号链接等异常；上下文配置和绑定控制文件不跟随链接到任意位置。
- 重复绑定相同目标可幂等成功；更换目标只更新本会话文件。
- 配置、绑定和锁文件均为项目本地数据，纳入 Git 忽略，不含凭证或对话内容。

### 4.3 生命周期

同一会话 ID 恢复时沿用绑定；新的会话 ID 不自动继承，按无绑定规则回退。SessionEnd 不自动删除，首版不做 TTL 自动清理。提供对指定当前会话的显式解绑；批量过期清理留待以后。

归档后保留会话绑定，让后续操作明确报告“绑定目标已归档”。不能因自动删除绑定而触发全局回退。

## 5. 会话身份与上下文来源

### 5.1 身份来源

- hook 的身份以本次请求中的 `session_id` 为准，不允许环境变量覆盖请求身份。
- CLI 使用显式 `--session-id`，其次使用专用环境变量 `MULIX_SESSION_ID`。
- 身份未提供与提供了非法值不同：前者允许兼容全局调用，后者报错，不吞掉后当无身份。
- 项目身份由规范化后的 mulix 项目根目录界定。同一个宿主 session_id 在不同项目的记录互相独立；不在共享 home 目录维护跨项目绑定表。
- hook 优先用本次请求 `cwd` 定位项目，不用待写文件的位置猜 change。缺失 cwd 的兼容请求可用 hook 进程工作目录；提供了 cwd 但解析失败时不得悄悄换到另一个项目。
- Bash、终端或子代理没有可确认的身份时不伪造；若宿主使用不同的子代理 session_id，必须显式绑定或按无绑定规则处理，不自动猜测父会话。

### 5.2 统一解析结果

解析器返回至少：`root`、`mode`、`session_id`（可空）、`change`（可空）、`source`、`phase`（可空）、绝对 `runtime_dir`。

`source` 定义：

- `session-binding`：会话绑定有效。
- `global-fallback`：session 模式且有会话身份，但绑定不存在，读取全局。
- `global`：legacy 或无会话身份，读取全局。
- `explicit-change`：允许的显式 `--change` 查询或管理操作。

无当前 change 时仍保留 global/global-fallback 来源，`change`、`phase` 为空，`runtime_dir` 指向 `_shared`。这是合法的“尚未选择变更”结果，不等同解析故障。

### 5.3 解析表

| 条件 | 结果 |
| --- | --- |
| legacy，配置有效或缺失 | 使用全局，完全不读取绑定 |
| session，有 ID 且绑定有效 | 使用绑定目标，不读取 active 作为备选 |
| session，有 ID 且绑定文件不存在 | 动态读取本次全局 active，不落盘绑定 |
| session，无 ID | 使用全局并标明 source=global |
| session，有绑定但其记录或目标失效 | 明确错误，不读取 active 作为备选 |
| 回退/全局路径下 active 不存在 | 返回无当前 change |
| session 中 active 存在但为空、损坏、不可读或目标失效 | 明确错误，不当作无 active |

动态回退发生在每次新调用的解析时，不是首次使用时的快照绑定。A 绑定 change-A，B 未绑定时，全局从 X 切到 Y，只改变 B 后续调用的上下文。

## 6. 命令契约

以下接口是拟新增行为，不表示当前版本已支持。与变更上下文无关的 init/update/preset/version 不因某个会话绑定失效而无法执行修复操作。

### 6.1 上下文管理

- `mulix context show`：展示有效上下文及来源；无当前 change 也可成功显示空值。
- `mulix context show --json`：稳定 JSON，供诊断和自动化读取。
- `mulix context show --field runtime-dir`：stdout 只输出绝对运行目录一行，供 shell 脚本使用；错误返回非零，诊断写 stderr，不添加 JSON 解析工具依赖。
- `mulix context mode <legacy|session>`：显式修改项目模式，只改 context.json；不重写 active、绑定或宿主 settings。
- `mulix context unbind`：需要明确会话身份，只删除该会话的绑定，不删除 change，不修改 active。允许在绑定损坏或已归档时用于恢复；删除前检查目标为受控普通文件，不跟随链接或递归删目录。

解绑命令的修改结果与后续回退解析结果分别报告：删除成功但全局失效时，明确说明“已解绑；全局上下文无效”，返回非零但不谎称解绑被回滚。

### 6.2 new 与 select

| 模式/身份 | 默认 new/select | 显式 `--global` |
| --- | --- | --- |
| legacy，不论有无会话 ID | 更新全局 active，旧行为不变 | 同默认 |
| session，有会话 ID | 新建/选择后绑定该会话，不改 active | 更新全局 active，不改本会话绑定 |
| session，无会话 ID | 更新全局 active，兼容人工命令 | 同默认 |

- select 只能选择存在、可加载、未归档的目标；可替换本会话的失效绑定，不要求旧绑定可解析。
- new 在创建并保存 change 后才写绑定或 active。后一步失败时，报告已创建 ID 和未完成的绑定，不递归删除产物，也不声称全成功。
- `--global` 仅用于明确的 new/select 目标选择操作，不是跳过阶段或 guard 的通用旗标。
- new/select 不创建或切换 Git 分支；branch 字段仍只是记录。

### 6.3 --change 的查询与修改

- 查询类命令可用 `--change` 查看其他变更，不改变绑定；这也可用于诊断当前失效绑定的目标，必须输出 source=explicit-change。
- session 模式且提供会话身份时，状态修改命令必须先解析当前有效上下文；显式 `--change` 若与有效上下文不同则拒绝。这个检查同样适用于 global-fallback，防止命令修改 B 而 hook 仍按 A 判断。
- session 模式的失效绑定不能被状态修改的 `--change` 绕过；通过 select、unbind 修复，或在无会话身份的人工管理调用中显式指定目标。
- 无会话身份的人工管理命令，以及 legacy 命令，继续允许显式 `--change`；不自动建立绑定。
- `state show` 没有当前 change 时仍返回清晰错误；`context show` 用于查询空上下文。
- `status` 保留列出全部 change 的能力，分别标记当前有效目标和全局 active；个别上下文故障要展示并返回非零，不能用另一标记伪装解析成功。

## 7. Hook 决策与错误边界

### 7.1 正常路径

定位项目与模式 → 获取请求身份 → 统一解析上下文 → 按既有阶段白名单判断。

阶段仍从 `.mulix/.runtime/<change>/state.yaml` 读取。原有 runtime 归属检查、state.yaml 禁止直接编辑、阶段可写子目录及 build 的普通源码放行规则保留。

会话模式在阶段放行前额外保护 `.mulix/context.json`、`.mulix/active`、`.mulix/sessions/`、`.mulix/.locks/`，包括它们自身及子路径，不允许 Write/Edit 修改；统一通过 CLI 管理。legacy 不借此额外收紧原有白名单。

### 7.2 错误策略

| 场景 | legacy | session |
| --- | --- | --- |
| 无 active，且无会话绑定 | 保持现有放行 | 放行，表示没有当前变更 |
| 绑定文件不存在，但全局有效 | 不读取绑定 | 按全局对应阶段判断 |
| 绑定存在但失效 | 不读取绑定 | 拒绝，不回退 |
| 已选定目标的状态加载失败 | 保持现有 fail-open | 拒绝 |
| 全局 active 读取或目标加载异常 | 保持现有 fail-open | 拒绝，不当作 active 缺失 |
| 项目 context.json 存在但无效 | 无法可靠识别 legacy | 拒绝，提示修复配置 |
| 无法解码 hook 请求，但可从进程目录确认 session 项目 | 保持现有 fail-open | 拒绝，不能绕过会话解析 |
| 项目根不存在，不能确认 mulix 管理上下文 | 保持现有告警放行 | 无法识别模式，不声称已应用会话门禁；告警放行 |

非目标工具和缺少 file_path 的处理维持既有约定，本轮不借机扩大工具匹配范围。需要拒绝但响应编码失败时，至少返回阻断退出码并写 stderr，不能再次进入 fail-open。

拒绝信息包含 change（如可得）、phase、source 与恢复建议。当前绑定失效时建议 select/unbind，不建议改全局来修复。成功写入不逐次刷兼容告警。

这是流程辅助，不是身份认证或安全沙箱；手工改控制文件、其他写入工具及旧二进制均可能绕过约束。不能宣称已阻止全部跨变更文件修改。

## 8. 宿主身份传播与安装

### 8.1 SessionStart 适配

拟保留原 `mulix hook` 为 PreToolUse 入口，新增 `mulix hook session-start`，从 SessionStart 请求取得 session_id 与 cwd。

- 支持时，通过宿主提供的环境文件仅导出 `MULIX_SESSION_ID`，不导出固定 change 或 runtime_dir。
- 同时使用宿主支持的会话上下文输出提示身份与当前解析来源，让没有环境注入的调用可显式携带 `--session-id`。
- 不在 SessionStart 建立绑定、更新 active 或自动启用 session 模式。
- shell 值必须正确转义；只使用宿主明确给出的环境文件，不猜用户全局 shell 配置，不把一次 export 误认为会传播到所有独立进程。
- CLI flag 是稳定的基础契约；自动环境注入是经宿主验证后的便利层。CLI 与桌面端、恢复会话与子代理的支持情况必须分别验证并记录，不把一个宿主的结果推广到其他宿主。
- 宿主不支持自动传递时，保留显式参数用法和全局兼容行为；文档明确该宿主未获得自动会话隔离。不能为了通过验收杜撰身份或偷偷固定全局 change。

本轮设计阶段未完成在线官方 hook 契约核对和真实宿主实测；实施计划的前置验证任务必须确认请求字段、环境文件、stdout 上下文格式及各宿主支持范围，再编写集成。若能力不存在，按本节显式参数路径实现和披露，不改变核心绑定/回退契约。

### 8.2 安装与升级

- `mulix init --host claude` 幂等合并新增 SessionStart hook，与原 PreToolUse 并存，不覆盖用户其他 hook；重复 init 不重复注册。
- 旧项目先升级 CLI/托管资源，再显式运行 init 安装新增集成，最后显式开启 session 模式；步骤间不自动迁移 active。
- 普通 `mulix update` 继续不编辑 `.claude/settings.json`，输出新增集成所需的显式 init 提示；不能假装更新技能就已经安装 SessionStart。
- DSH-only 不新增 Claude settings/hook；通过显式 session 参数或专用环境变量使用同一上下文解析器，仍不宣称 DSH 有写入 hook。
- 给当前已启动会话升级时，SessionStart 未重跑就不能假设环境已注入；需恢复/重启适用宿主，或明确传参。
- 模式、绑定和锁文件不是可被 update/force 清理的 vendor 资源。init 需幂等补齐本地忽略规则，不覆盖已有用户忽略内容，不自动取消已经 tracked 的文件。

## 9. 运行产物与长任务

- SDD 与脑暴脚本不再自行 `cat .mulix/active`；在已确定的 mulix 项目根调用 `mulix context show --field runtime-dir`。
- 解析失败即停止，不能用 `|| true` 或空字符串把绑定错误降为 `_shared`；只有解析成功且无当前 change 才使用 `_shared`。
- `start-server.sh` 自建的服务实例 SESSION_ID 与 MULIX_SESSION_ID 分开命名和使用；前者只用于脑暴实例目录，不参与 change 绑定。
- 每次命令、hook、脚本启动时解析一个上下文快照，本次操作固定使用；长任务不在步骤间重新读取 active。
- 服务启动后固定运行目录、端口及 token 文件归属；全局切换或解绑不迁移已启动服务、不移动已生成产物。
- 已放行的工具执行或正在运行的脚本不会因后续 select 而被撤销；同一会话切换后继续向旧目录写入，下一次 Write/Edit 可能按新上下文拒绝，这是预期行为。
- 保留现有按 plan 区分的 SDD workspace 规则，不承诺同一 plan 的多写者协作。
- 同步更新 vendored 脚本和 `scripts/sync-superpowers.sh` 的生成补丁，回归现有 workspace 命名、碰撞处理和运行证据位置。

## 10. 并发、持久化与失败恢复

### 10.1 互斥范围

新增项目本地 `.mulix/.locks/`，实现跨进程互斥，不能只使用 Go 进程内 mutex。

- 短期上下文锁：保护模式修改、绑定读写/删除、全局 active 的读取/替换/条件清除，以及解析快照。
- 每 change 状态锁：覆盖 state set/transition 的完整“重新加载 → 修改或 guard → 应用迁移 → 保存”，防止两个调用以旧状态覆盖彼此更新。不同 change 不因长 guard 共用一把全局锁。
- 安全写入继续复用准备完整文件后替换的方式；锁不能替代旧文件保护，原子替换也不能替代锁。

锁实现由实施者选择，但必须支持 Windows 和 Unix 的跨进程、进程退出后释放、有限等待和明确超时。优先使用经验证的操作系统锁或成熟可移植实现；若需要依赖，实施计划列出选择与理由。不能靠“超过 TTL 就删锁”恢复，也不能失败后无锁继续。

### 10.2 锁顺序与快照

不允许持有上下文锁等待 change 锁。需要两个锁的操作采用 change → context 顺序：

1. 在短上下文锁内解析目标快照，然后释放上下文锁。
2. 状态修改取得目标 change 锁，重新加载状态，再执行 guard 和保存；不会因等待期间 active 切换而改为另一个目标。
3. select/new 在目标 change 锁下验证/保存目标，再取得短上下文锁写绑定或 active。
4. 归档在 change 锁内保存归档状态，再取得上下文锁，只有 active 仍指向该目标才清除。
5. 普通 hook 在上下文快照后读取目标状态时，可按实现需要短暂取得 change 锁；不把锁延伸到宿主随后真正执行 Write/Edit 的整个过程。

项目模式切换也遵循调用快照语义：已经开始的操作按已解析模式完成，之后的新调用使用新模式。本轮不提供模式切换屏障。

### 10.3 多文件失败

- 不宣称绑定、active 和 state 共同事务提交。
- 归档状态保存成功但 active 清除失败时，保留归档事实，报部分完成；active 指向归档目标时会话模式拒绝后续写入。
- 绑定写入失败保留旧绑定，不先删除旧文件。若文件系统不能保证替换原子性，错误不得伪装成功。
- 配置或绑定暂时不可用时，锁等待超时或读取失败都作为错误，不进入回退。
- Windows 的文件替换保证以实际文件系统和 Go 行为为准，不写“所有平台绝对原子”的承诺。
- 并发保证仅覆盖遵守新协议的 mulix 调用；不要同时运行旧二进制或绕过锁手改控制文件。

## 11. 可观察性与恢复路径

`state show`、`context show` 和相关修改命令展示 mode、session（如有）、change、phase、source；不在 JSON/单字段 stdout 混入诊断文字。

- session-binding：提示当前为独立会话绑定。
- global-fallback：提示当前未绑定，后续会跟随全局。
- global：提示无会话身份或处于 legacy。
- `--global`：明确输出更改了全局 active，并说明本会话已有绑定未改变。
- unbind：报告移除的绑定和新的回退结果；无原绑定时幂等，不替用户建立绑定。
- 无效绑定：报告记录位置、目标（如可解码）及 select/unbind 修复方式；不泄漏宿主会话凭证或对话。

## 12. 验收矩阵

以下是实施后的验收要求，不是已经运行的测试结果。

| 编号 | 场景 | 预期 |
| --- | --- | --- |
| C01 | 缺失 context.json，宿主传 session_id | 保持 legacy，不读绑定，不改变旧 select/new |
| C02 | 开启 session，不存在绑定，全局 X 有效 | source=global-fallback，解析 X，不创建绑定 |
| C03 | C02 后全局切到 Y | 下一调用解析 Y，已启动长任务仍用 X |
| C04 | A 绑定 design 的 change-A，B 绑定 build 的 change-B | 各自按对应阶段判断，A 的选择与新建不影响 B |
| C05 | 绑定文件损坏、目录、权限错误、ID 不匹配、未知 schema | 错误，不读全局作为候选，不使用 _shared |
| C06 | 绑定目标缺失、状态错误、状态 Change 不匹配或已归档 | 错误，不回退 |
| C07 | 解绑有效/失效绑定，全局有效 | 删除本会话记录并恢复回退，不改其他会话 |
| C08 | 解绑成功但全局无效 | 明确部分结果，不恢复旧绑定，不声称全成功 |
| C09 | select/new 有会话身份与 --global 的组合 | 严格符合 6.2；--global 不改已有绑定 |
| C10 | 无身份的 CLI，显式 --change，legacy 回归 | 原用法继续工作；显式管理不建立绑定 |
| C11 | session 下状态修改的 --change 与有效目标不一致 | 拒绝；查询可成功，不改变绑定 |
| C12 | 无绑定且 active 不存在 | context show 空上下文；state show 报未选；hook 放行；运行产物可用 _shared |
| C13 | active 存在但损坏、不可读或指向失效目标 | session 错误；legacy 保持既有策略 |
| C14 | 并发 set/transition 同一 change | 无丢失更新、无基于过期阶段迁移；锁超时明确失败 |
| C15 | 不同 change 的长 guard 并发 | 不被一把长持有的项目锁无谓串行化 |
| C16 | 归档与全局 select 并发 | 只条件清除原目标，不删除新 active；绑定保留归档提示 |
| C17 | SDD、脑暴有不同绑定 | 各自运行目录正确，脚本失败不悄悄写 _shared |
| C18 | 宿主 cwd 与 hook 进程目录不同，嵌套 mulix 项目 | 以约定的项目根隔离，不串到父项目 |
| C19 | SessionStart 环境注入、显式参数、恢复和子代理 | 按真实宿主能力验证；不支持的路径明确披露 |
| C20 | init 重跑、已有用户 SessionStart、普通 update、DSH-only | 幂等保留用户设置，update 不改 settings，DSH 不新增 Claude |
| C21 | session 模式直接 Write/Edit 控制文件 | 即使 build 阶段也拒绝；非目标工具边界不变 |
| C22 | 会话 ID 与 change 的路径穿越、控制字符和 Windows 路径形式 | 无越界读写，无 shell 注入；合法旧 ID 可继续选择 |
| C23 | 进程异常退出、锁等待、文件替换失败 | 可恢复或明确报错，不删旧文件、不无锁降级 |
| C24 | 重新同步 vendored superpowers 脚本 | 保留上下文查询逻辑，不重新引入直接读取 active |

实施验证至少包括相关 Go 单测、多进程并发测试、现有完整 Go 测试/vet、七阶段 smoke，以及 Windows/Git Bash 下脚本与锁的验证。未能验证的宿主和平台必须列出，不用模拟 fixture 冒充真实宿主验证。

## 13. 留给实施计划的决策

在不改变本文行为契约的前提下，实施者可决定：

- 上下文解析、存储、锁的具体 Go 包和函数划分。
- 满足第 10 节的平台锁实现、等待上限及可测试的注入点。
- 人类可读输出的排版、稳定错误类型和最大会话 ID 长度。
- 宿主前置验证后的 SessionStart 精确 payload/响应与安装 matcher。
- 本地忽略文件的最小幂等实现与单元/多进程测试组织。

不得自行改为无绑定拒绝、首次使用自动绑定、失效绑定回退、自动切换全局、复制会话阶段，或把源码冲突防护作为已交付能力。若宿主集成限制导致核心会话绑定无法可靠使用，应报告限制，不偷偷缩小验收范围。

## 14. 本轮交付与后续门槛

设计整理阶段只交付本设计及一次独立构建检查后的修订，未提交功能代码或修改真实宿主配置。后续用户已批准 Native 实现；提交、推送及真实宿主配置修改仍需另行授权。该阶段结束后用户已授权 Native 实现与提交（`bd64722`）。

设计经用户审阅批准后，再编写实施计划；编写计划不等于授权直接开始实现。既有源码、examples 中的历史安装副本和历史开发记录不在本轮改写范围。
