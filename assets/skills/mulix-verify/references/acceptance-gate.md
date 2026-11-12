# 正式验收契约与只读证据门禁

## 适用范围

本资源是 project-scoped 正式业务工作台适配，不是通用项目的强制目录约定。仅适用于实际项目宪法要求该正式来源，且采用下述布局的项目：后端根的 `docs/specs/<change>/spec.md`、同级 `../pm/Product-Spec.md` 和工作台的 `完整测试点与测试用例.md`、`requirements/case/**/cases.json`、`reports/` 或 `runs/`。其他项目继续使用原生 AC/TC 与项目自有 quality.yaml，不虚构 pm/test 或正式准入；不同工作台适配另行设计。

适用项目必须先完成本核对，将逐点结果、指纹与未闭合门禁写入最终报告，再进行整体审阅和原生 `mulix verify`；必要但未通过的业务门禁保留在 `mulix-review.blocking`。后续修改报告或源码会使原生回执过期，须按原生流程重验。本脚本的文本输出不是原生 suite 的结构化测试事件，不得把它伪装成 CLI 运行回执，也不能手工设置 verify_result 来代替任一检查。

## 权威与结论边界

以 `.mulix/memory/constitution.md` 和仓库 `CLAUDE.md` 为准，正式用例继续使用父仓 `../test/完整测试点与测试用例.md`、`requirements/case/<module>/cases.json` 及其生成器，不建立第二套用例权威或测试框架。先读工作台 `AGENTS.md` / `README.md`，按已批准范围运行现有来源/绑定校验；静态校验成功不等于用例执行。

本工具只检查**完整性、一致性及证据层级**，不连接业务服务、不修改输入、不改 mulix 状态、不认证人类同意或报告真实性，也不能理解自然语言预期是否足够完整。范围与检查点须由有权人员实际审阅，运行元数据须由受控工作台 producer/CI 产生，真实业务仍需执行。不得自填观察 JSON、批准记录、版本字段或运行结果来使工具通过。

`EVIDENCE_CHECK_PASS` 只表示本次证据结构核对通过；仍须结合全套回归、真实设施专项、逐需求结果和人工裁定，才能考虑 `verify_result=pass`。该项目工作台适配不是原生 CLI/CI 内置 guard；通用 P0 的原生 runner、回执和阶段门禁仍须执行，不能由此脚本替代或绕过。

## 1. 在执行前冻结验收合同

从 `.mulix/templates/acceptance-contract-template.json` 派生 `docs/changes/<change>/acceptance-contract.json`。模板故意包含占位符、PARTIAL 和阻断项，不能直接通过。

- `schema_version=1`，`acceptance_level=FORMAL`。内核/集成/装配完成在任务报告中另列，不使用该门禁升级结论。
- `baseline` 固定实际受测实现完整 revision（40/64 位小写十六进制不可变标识）、环境 ID、change 的 spec.md、父仓 Product-Spec.md、工作台正式用例正文的路径和 SHA-256。实现标识必须独立核验实际部署/冻结构建；不能仅抄当前 HEAD 冒充未部署或不同工作区的版本。
- 所有文件引用为 `{ "path": "相对后端根的路径", "sha256": "64位小写SHA-256" }`；显式 `--workbench-root` 可选择已经核验的独立测试工作树，文件仍按实际路径引用。
- `requirements` 逐行列出规格中每个 `- **FR-nnn**:` / `- **SC-nnn**:` 的 ID 及 `case_ids`；规则/产品 AC 保留在规格和工作台映射中。未映射、重复或未知 ID 阻断。`exclusions` 只能放明确批准的范围排除 `{ "id": "SC-nnn", "reason": "..." }`，并纳入合同整体审核；已知安全阻断不得悄悄改成排除项。
- `case_sources` 指向原工作台各模块的 cases.json；case 必须在原正文存在且索引 `admission=formal`。候选先走原工作台准入，工具不自动转正。
- `cases` 固定 case ID、`coverage=FULL`、`remaining=[]`、冻结前置/受控夹具的 `context` 文件引用，以及全部 `checkpoints`。清单须覆盖正式用例的前置、步骤、全部预期与禁止副作用，不仅是用例标题关联。`blocking_items` 保存所有未闭合门禁；非空时全量正式验收阻断。
- 每个 checkpoint 固定 `id`、非空 `expected` 和 `method=AUTOMATED|MANUAL`；自动场景另固定工作台相对 `feature` 与精确 `scenario` 名。检查点不是新正式用例 ID，不能代替原始 case ID。

内核、事务、并发、生产装配、真实业务旅程使用不同证据。Mock/Fake 调用、H2、目录检查不能证明 PostgreSQL 并发、真实 owner 提交或正式工作台用例通过。

## 2. 审核范围，不推断同意

合同冻结后，在工作台 `traceability/`（或本 change 的 docs/changes 目录）保留实际范围审核记录：

```json
{
  "status": "APPROVED",
  "contract_sha256": "[REVIEWED_CONTRACT_SHA256]",
  "reviewed_by": "[ACTUAL_AUTHORIZED_REVIEWER]",
  "reviewed_at": "[ISO_TIMESTAMP_WITH_TIMEZONE]"
}
```

这是审核记录协议，不是可自行填写的授权。记录必须源于真实确认；程序仅核对字段和合同指纹，不能认证确认者身份。执行结果通过 `scope_approval` 的路径/哈希引用该记录；每条自动/人工原始执行时间必须满足 `reviewed_at ≤ 原始 executed_at ≤ 结果汇总 executed_at`，不能只用汇总时间掩盖批准前的执行。合同变化会使旧批准和结果失效，须重新确认与执行受影响项。

## 3. 真实执行后收集逐检查点结果

从 `.mulix/templates/acceptance-results-template.json` 派生同目录 `acceptance-results.json`。由实际执行/审核流程填入 `change`、`contract_sha256`、同源 revision/environment、带时区的 `executed_at` 和逐点实际结果。结果的 `executed_at` 是本次汇总截止时间，不代替原始执行记录的时间。

- case 与 checkpoint 集合必须与合同完全一致，不能遗漏、重复、添加或靠空数组通过。
- 每点必须 `status=PASS`、非空 `actual`、真实 `evidence` 文件引用。FAIL/BLOCKED/PARTIAL/NOT_RUN/SKIPPED_BY_USER、任一 `blockers` 都阻断；状态只能反映事实，不为通过校验而改写。
- 自动点还须 `run_id`，原始证据必须位于工作台 `reports/` 或 `runs/`。支持现有 case-live 原生报告字段：`status=FINALIZED`、`evidence_finalized=true`、`run_id`、非零 `passed`/零 `failed`、`verdicts[].path/name/passed/skipped/error`、`input_sha256`。全部场景通过且未跳过，绑定场景唯一命中，全部输入哈希仍匹配；dry-run、oracle/诊断或未封存报告不合格。
- **部署/前置来源门禁**：自动报告还必须由真实 producer 在执行开始时捕获 `implementation_revision`、`environment`、带时区的 `executed_at`、`case_context_sha256`（`{ "<caseId>": "<frozen-context-sha256>" }`）。版本、环境和该 case 的 context 必须匹配合同基线，时间须落在批准与汇总之间。当前工作台旧报告若没有这些字段，保持 BLOCKED；需要工作台受控 producer 扩展和重新执行，不能事后往旧 JSON 补字段。本工具不虚构 producer 已接线。
- `SAMPLED_PARTIAL_ASSERTIONS` 报告只支持其中实际执行的检查点，不能单独充当整条用例通过。跨 run 汇总仅在合同已审阅完整、每点均有证据、版本/环境/同 case 夹具上下文一致时成立；有剩余义务仍为 PARTIAL/BLOCKED。

人工点使用工作台 reports/runs 下的真实人工记录 JSON（不是产品 API）：`kind=MANUAL_ACCEPTANCE`、`case_id`、`checkpoint_id`、`implementation_revision`、`environment`、`context_sha256`、`executed_by`、带时区 `executed_at`、`status=PASS`、与合同相同的 `expected`、与逐点结果相同的 `actual`。记录正文可附实际日志/截图/操作记录引用，审核者核对这些观察；人工声明不证明机器执行，也不代替真实二次认证或资金事实。

## 4. 在 verify 放行前执行

从后端根目录，将 `CHANGE` 设置为已审阅的实际变更 ID，以独立核验的受测版本执行：

```bash
: "${CHANGE:?set the reviewed change ID}"
: "${VERIFIED_DEPLOYED_REVISION:?set the independently verified deployed revision}"
python3 -I -B {{SKILLS_DIR}}/mulix-verify/scripts/check-acceptance.py \
  --project-root "$PWD" \
  --workbench-root ../test \
  --contract "docs/changes/$CHANGE/acceptance-contract.json" \
  --results "docs/changes/$CHANGE/acceptance-results.json" \
  --implementation-revision "$VERIFIED_DEPLOYED_REVISION"
```

- exit 0：`EVIDENCE_CHECK_PASS`，仅证据完整性检查通过。
- exit 1：`EVIDENCE_CHECK_BLOCKED`，缺口/漂移/格式/证据层级不合格，保留待办，不设 pass。
- exit 2：CLI 参数错误，不是业务测试失败，也不能算 TDD 行为 RED。

在 `report.md` 记录合同/结果/范围审核的指纹、命令与实际输出、逐 FR/SC/正式 case/checkpoint 结论、真实专项与回归失败类、未执行项、裁定及允许的结论层级。只有所有要求都满足且人工确认真实有效，才按 verify 技能操作。归档前再次核对相同证据；来源漂移不能靠更新哈希“修复”，应审阅影响并重新取证。

## 工具自身验证

```bash
python3 -I -B -m unittest discover -s {{SKILLS_DIR}}/mulix-verify/tests -p 'test_*.py' -v
```

这些临时合成夹具只验证校验器的拒绝/放行行为，不是产品运行、PostgreSQL、正式用例准入、二次认证或 REQ-031 验收证据。测试及完整命令日志留在当前会话 scratch，不混入其他变更的任务台账。
