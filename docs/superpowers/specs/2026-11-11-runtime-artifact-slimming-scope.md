# Runtime 产物精简：已批准范围

状态：2026-11-11 在对话中获得用户批准。本文记录该范围，不额外扩大需求。

## 目标

减少 `.mulix/.runtime/` 中持久堆积的中间产物，保留 superpowers 实际执行需要的记录，不改变 Mulix 的阶段状态机和 change 隔离。

## 已批准的行为

- 保留按 change 隔离的 `.mulix/.runtime/<change>/`。
- 每个 change 只保留 `specs/`、`sdd/` 两类运行产物目录。
- `state.yaml` 是必要控制状态，保留原位，不计作日志；根级 `.runtime/.gitignore` 也保留。
- `specs/` 继续保存设计文档。
- `sdd/` 继续按计划分工作区，当前通常是 `sdd/tasks/`。
- SDD 按实际执行方式沿用 superpowers 的产物种类：`progress.md`、`plan-path`、`task-N-brief.md`、`task-N-report.md`、`task-N-tests.log`、`review-<base>..<head>.diff` 以及必要的 `.gitignore`。
- 不在上述记录之外额外叠加自定义日志目录、证据清单或构建二进制留档；一次性测试输出和调试文件使用系统临时目录，必要结果摘要和 RED/GREEN 证据写回已有记录。
- `brainstorm/`、`verification/`、`migrations/`、全局 `_shared/` 不再作为 `.runtime` 下的持久产物落点；临时可视化、诊断和验证材料使用系统临时目录。
- 最终验证报告仍位于 `docs/changes/<change>/report.md`。
- 保持现有进度格式、历史兼容、RED/GREEN 报告要求、构建门禁和 SDD 工作区保留行为，不照搬上游结束后删除整个工作区的动作。
- 不在初始化或更新时静默删除旧项目的历史文件；保留防止旧 brainstorm token 等被意外提交的忽略规则。

## 非目标

- 不迁移状态文件、不扁平化 change 目录、不改会话绑定或归档状态的存储布局。
- 不增加日志系统、自动清理命令或后台清理任务。
- 不扩大 hook 的工具覆盖范围：仍仅约束 Write/Edit，不把它描述为安全沙箱。
- 不取消测试证据或现有质量门禁。
- 未获授权前不提交、不推送、不删除现有运行记录；不处理已有未跟踪的 `dist.zip`。

## 交由实施计划确定

系统临时目录的精确命名、CLI 查询字段、门禁接入和回归测试属于实施选择，需在计划中明确，不能以放行任意临时路径代替现有隔离。
