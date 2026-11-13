# P1：持续质量与最终交付物验收

日期：2026-11-12。用户已要求按先前方案实施 P1 并完整测试，不再进行需求访谈。以下工程细节由实现者选择，不写作用户指定。

## 目的与边界

P0 已将单个变更的任务执行、测试回执、最终验证与归档绑定。P1 将仓库本身的必测策略和最终交付物接入可重复的持续验收，覆盖：必测策略、CI、原生平台矩阵、产物 E2E、安装升级验收。

验收对象是 mulix 工具，不是下游业务、模型效果或真实宿主付费评测。保留 P0 schema、receipts、质量门禁及项目自有 formal adapter，不编辑此前同步的 13 个资源。本轮不提交、不推送、不发布、不改变远端设置、不向实际用户目录安装。

## 实现者选择

- 沿用 Go、Bash；新增 Python 标准库脚本，使用 `python3 -I -B`，不引入 pip/npm 依赖。
- 工具链固定为本地已执行版本：Go 1.27.3、Python 3.14.8、Node 26.13.0；Bash/Git 由原生 runner 提供并记录版本。Actions 依赖使用已查询的完整 commit SHA。
- 平台为既有六个发行目标：linux/darwin/windows × amd64/arm64。Linux 两个架构执行 race；所有平台执行根模块/示例 full、vet、Python、自有脚本一致性、最终产物安装升级和三个 smoke。工具或 symlink 能力不足即失败，不静默 skip。
- 原生 runner：ubuntu-24.04、ubuntu-24.04-arm、macos-15-intel、macos-15、windows-2025、windows-11-arm。尚未远端执行，不能提前声称这些环境通过。
- 安装/升级基准是已验证且固定的 P0 commit `c50621e0191c51c38573e183c81ab9a193bceb7f`，不是虚构的已发布历史版本。用这个 commit 的真实源码构建旧 bundle，真实初始化旧项目，再用候选 bundle 升级。

## 组成与接口

### 1. 仓库必测策略与执行

`scripts/quality-policy.json` 是平台、工具、覆盖率下限、关键必测标识、许可 skip 原因及升级基准的权威配置。`scripts/quality.py` 提供 native 执行、汇总和 extended 子命令；`scripts/tests/test_quality.py` 验证失败关闭行为。

Go 测试使用 `-count=1 -json`；JSON 按 ASCII LF 分帧，日志固定 UTF-8 解码。源码清单与各模块、六目标 Go 实际编译/嵌入输入核对，拒绝被忽略但实际参与构建的文件以及隐藏修改的 assume-unchanged/skip-worktree index 标记。驱动保留 stdout/stderr、argv、退出码和覆盖率 profile，检查包与测试终态、失败、缓存、重复/不完整事件和非空执行；必测测试缺失或跳过不能通过。许可 skip 只能是逐个测试、逐个平台、已知原因：非 Windows 上的五个 Windows 专属测试；Windows 的 Unix 权限/TMPDIR 专属测试；需要外部上游源码的可选再生测试。其他 skip 均阻断；原生 Windows 必须执行其五个路径身份测试。不能把某个 package 没有测试文件与整套零执行混为一谈。

覆盖率以精确 covered/total 比较，不用四舍五入的显示百分数做门禁。Linux amd64 固定下限采用当前真实基线；各平台另外与同平台、同工具链、固定 base revision 的实际覆盖率比较，候选 root/example 均不退化。PR 使用 PR base SHA；非 PR 使用确定的历史 SHA。基准覆盖率运行发生在私有本地 clone，不能修改实际 checkout。

现有 scaffold TestMain 单独构建的 CLI 必须在外层 `go test -race` 时显式 `go build -race`，用 build-tag 常量及二进制 build info 回归验证。驱动为 race 作业设置独立诊断日志，任何子进程 race 日志均使作业失败，避免负向命令吞掉诊断。子进程 test executable 已继承 race 的路径保持不变。

### 2. 最终 bundle 与安全安装

`scripts/delivery.py` 实现 build、verify、install；`scripts/qualify_delivery.py` 执行最终产物场景，二者与 native 驱动共用 `scripts/quality_run.py` 的显式 argv/超时/持久日志，以及 `scripts/quality_process.py` 的调用私有进程树登记和子树取消（exec 前登记，覆盖嵌套新 session）。`scripts/release.sh`/`scripts/install.sh` 保留 Bash 入口。新文件均有行为测试 `scripts/tests/test_delivery.py`。

Bundle 是新建且不覆盖既存目录的输出：六个单文件归档（Unix tar.gz，Windows zip）、`manifest.json`、`SHA256SUMS`。manifest schema 为 `mulix.release.v1`，包含版本、源码 commit、源码清单指纹/dirty 状态、Go 版本及六个目标各自的归档名、归档 hash、二进制 hash/大小。校验器核对精确目标集合、严格相对文件名、普通文件、归档成员与大小、二进制目标架构、所有 checksum，拒绝缺失/多义 JSON、路径穿越和 symlink/link 成员。

构建只使用实际 checkout 或明确指定的私有历史 checkout。工作区存在未提交修改时 manifest 如实记录；本地测试允许测试该候选，但不将其称为干净发行。CI 必须核对 manifest commit 与预期 SHA、源码无修改及整个运行前后指纹一致。

不再 `rm -rf dist`。默认产物放在 `dist/mulix_<version>`；重用已有输出路径报错并保留全部内容；`--output` 可指定全新目录。`--dry-run` 只输出计划，不能成为构建或执行证据。

artifact 安装接受 `--bundle` 和临时或用户显式的 `--prefix`，不需要 Git/Go。先验证 bundle，将本机二进制写到 prefix 内同文件系统的唯一暂存文件，核对实际 `version` 输出后原子替换。校验、版本或替换失败保留原二进制及用户数据，清理仅限本次拥有的暂存文件。拒绝不支持平台、非法参数和错误版本。源码安装入口仍可用，同样在暂存/版本核对后替换；用户未明确选择的实际用户目录不会用于测试。

归档哈希是完整性机制，不是发行者身份认证；不声称防御拥有同一工作区写权限的攻击者或提供文件系统沙箱。

### 3. 原生最终产物与升级验收

CI 仅在 bundle job 构建候选及固定旧基准；六个 native job 下载这同一份不可变上传物，验收期间不重建候选 CLI。原生 job 将本机环境与声明的目标核对，验证 bundle，安装并执行版本，所有 smoke 必须显式传入同一个安装后二进制；记录安装前后 hash 并复核输入 bundle 未漂移。

- **DELIVER-01**：最终产物安装 → 初始化 → 多进程完整开发/验证/归档链；实际运行现有 e2e、sessions、update 三项 smoke，检查退出码及既有行为断言。不能以源码重建替代最终产物。
- **DELIVER-02**：真实旧 commit bundle 安装和初始化；保留用户数据及本地修改；损坏候选安装失败必须保持旧二进制与项目；正确候选升级，实际发生模板 upstream/local 冲突，拒绝非法接受路径，保留本地修改/sidecar，显式接受目标 upstream，重试成功并保持其他用户数据。旧/新源码 revision、版本及最终 hash 均记录。
- 负向单元/组件测试覆盖 malformed/truncated bundle、错误 hash/平台/版本、已有输出、不完整构建、原子替换失败、临时产物清理与数据不变。注入故障只在私有 fixture 内，不能写作真实业务验收。

### 4. CI 门禁与延长测试

新增 `.github/workflows/quality.yml`：PR、main push、手动和定时触发；contents 只读、checkout 不保留凭据、超时上限、完整 SHA action pins。无路径过滤。`bundles` 产出候选/历史基准；`native` 为六目标；固定 `P1 required` 汇总 job 使用 `always()`，只有依赖成功且六份完整 PASS 原生报告与当前候选 manifest hash、commit、目标匹配才成功。缺失、重复、失败、取消、旧产物证据不可被 success/skip 掩盖；重试选取同目标最新 attempt，候选改变后需全部重验。

定时/手动作业运行有界跨进程并发重复、恢复/故障测试和 Go fuzz；新增针对质量 Markdown/contract 的 fuzz seeds 与不崩溃/合法拒绝属性，不把短期 fuzz 写作漏洞穷尽证明。该作业在定时/手动事件为必需，PR 事件允许它未调度；native 从不因此可选。

远端保护需要用户另行授权后检查/启用，仓库内写 YAML 不等于 required check 已启用。本机无 `gh`，当前远端保护状态未知；文档记录待办和读操作，不通过新增凭据或改权限绕过。

## 验收与证据

依次：当前 checkout 的新鲜基线 → RED/GREEN → 每任务 suite → 一次独立整分支审查及必要的 RED/GREEN 修复 → 新鲜全量验收。保留本计划 `.superpowers/sdd/2026-11-12-p1-continuous-qualification/` 的 ledger、命令、原始日志、源码前后清单、bundle、覆盖率和报告，不删除之前的 P0 证据或已有 bin/dist。

最终分别报告：Linux 本地执行结果；六目标 cross-build 完整性；未运行的 Windows/macOS/ARM 原生 CI；尚未核实的远端保护。不能将前三者混成跨平台发行通过。

## 资料

Runner 标签选择依据 GitHub 官方文档（2026-11-12 查询）：
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job
