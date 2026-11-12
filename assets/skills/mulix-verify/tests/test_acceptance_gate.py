"""验收门禁工具测试；所有夹具为合成数据，不代表产品用例执行证据。"""

import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


GATE = Path(__file__).resolve().parents[1] / "scripts" / "check-acceptance.py"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class AcceptanceGateTest(unittest.TestCase):
    """移除任一失败关闭条件，都应被相应的非法输入测试发现。"""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.ledger = Path(self.temp.name) / "ledger"
        self.project = self.ledger / "biz-finance"
        self.workbench = self.ledger / "test"
        self.project.mkdir(parents=True)
        self.workbench.mkdir()
        self.revision = "a" * 40
        self.change = "2099-01-01-acceptance-test"
        self.feature = "requirements/case/示例/payment.feature"
        self.write_text(self.ledger / "pm/Product-Spec.md", "# 合成产品规则\n重复提交不重复建单。\n")
        self.write_text(self.workbench / "完整测试点与测试用例.md", "# 合成正式来源\n| AUT-001 | 重复提交只创建一张付款申请 |\n")
        self.write_text(self.project / f"docs/specs/{self.change}/spec.md", "- **FR-001**: 不重复建单。\n- **SC-001**: 只存在一张付款申请。\n")
        self.write_text(self.workbench / self.feature, "@live\nFeature: 合成门禁夹具\nScenario: 重复提交只建一单\n* match observed.count == 1\n")
        self.inventory = self.workbench / "requirements/case/示例/cases.json"
        self.context_path = self.workbench / "fixtures/acceptance-context.json"
        self.write_json(self.context_path, {"case": "AUT-001", "fixture": "synthetic-isolated-account"})
        self.write_json(self.inventory, [{"id": "AUT-001", "admission": "formal", "expected": "只创建一张付款申请。"}])
        self.run_path = self.workbench / "reports/case-live/run-001.json"
        self.run = {
            "status": "FINALIZED", "evidence_finalized": True, "run_id": "run-001",
            "implementation_revision": self.revision, "environment": "isolated-synthetic-pg18",
            "executed_at": "2099-01-01T11:30:00+00:00",
            "case_context_sha256": {"AUT-001": digest(self.context_path)},
            "scope": "SAMPLED_PARTIAL_ASSERTIONS", "passed": 1, "failed": 0,
            "input_sha256": {self.feature: digest(self.workbench / self.feature)},
            "verdicts": [{"path": self.feature, "name": "重复提交只建一单", "passed": True, "skipped": False}],
        }
        self.write_json(self.run_path, self.run)
        self.contract_path = self.project / f"docs/changes/{self.change}/acceptance-contract.json"
        self.results_path = self.project / f"docs/changes/{self.change}/acceptance-results.json"
        self.approval_path = self.workbench / "traceability/acceptance-scope-review.json"
        self.contract = {
            "schema_version": 1, "change": self.change, "acceptance_level": "FORMAL",
            "baseline": {
                "implementation_revision": self.revision, "environment": "isolated-synthetic-pg18",
                "spec": self.ref(self.project / f"docs/specs/{self.change}/spec.md"),
                "product": self.ref(self.ledger / "pm/Product-Spec.md"),
                "formal_cases": self.ref(self.workbench / "完整测试点与测试用例.md"),
            },
            "case_sources": [self.ref(self.inventory)],
            "requirements": [{"id": "FR-001", "case_ids": ["AUT-001"]}, {"id": "SC-001", "case_ids": ["AUT-001"]}],
            "exclusions": [], "blocking_items": [],
            "cases": [{"id": "AUT-001", "coverage": "FULL", "remaining": [], "context": self.ref(self.context_path), "checkpoints": [{
                "id": "CP-001", "expected": "只创建一张付款申请。", "method": "AUTOMATED",
                "feature": self.feature, "scenario": "重复提交只建一单",
            }]}],
        }
        self.results = {
            "schema_version": 1, "change": self.change,
            "implementation_revision": self.revision, "environment": "isolated-synthetic-pg18",
            "executed_at": "2099-01-01T12:00:00+00:00", "blockers": [],
            "cases": [{"id": "AUT-001", "checkpoints": [{
                "id": "CP-001", "status": "PASS", "actual": "观察到一张合成付款申请。",
                "run_id": "run-001", "evidence": self.ref(self.run_path),
            }]}],
        }
        self.save()

    @staticmethod
    def write_text(path, text):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def write_json(self, path, data):
        self.write_text(path, json.dumps(data, ensure_ascii=False, indent=2) + "\n")

    def ref(self, path):
        relative = path.relative_to(self.project) if path.is_relative_to(self.project) else Path("..") / path.relative_to(self.ledger)
        return {"path": str(relative), "sha256": digest(path)}

    def save(self):
        self.write_json(self.contract_path, self.contract)
        contract_hash = digest(self.contract_path)
        self.write_json(self.approval_path, {
            "status": "APPROVED", "contract_sha256": contract_hash,
            "reviewed_by": "synthetic-reviewer", "reviewed_at": "2099-01-01T11:00:00+00:00",
        })
        self.results["contract_sha256"] = contract_hash
        self.results["scope_approval"] = self.ref(self.approval_path)
        self.write_json(self.results_path, self.results)

    def check(self):
        return subprocess.run([
            sys.executable, "-I", "-B", str(GATE), "--project-root", str(self.project),
            "--workbench-root", str(self.workbench), "--contract", str(self.contract_path),
            "--results", str(self.results_path), "--implementation-revision", self.revision,
        ], capture_output=True, text=True, timeout=10)

    def assertBlocked(self, expected):
        self.save()
        result = self.check()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(expected, result.stdout + result.stderr)

    def test_complete_reviewed_checkpoint_evidence_passes_without_writing_inputs(self):
        before = {p: p.read_bytes() for p in self.ledger.rglob("*") if p.is_file()}
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("EVIDENCE_CHECK_PASS", result.stdout)
        self.assertEqual(before, {p: p.read_bytes() for p in self.ledger.rglob("*") if p.is_file()})

    def test_candidate_case_cannot_be_promoted_by_result_flags(self):
        self.write_json(self.inventory, [{"id": "AUT-001", "admission": "candidate", "expected": "只创建一张付款申请。"}])
        self.contract["case_sources"] = [self.ref(self.inventory)]
        self.assertBlocked("formal")

    def test_each_nonpassing_checkpoint_status_blocks(self):
        for status in ("PARTIAL", "FAIL", "BLOCKED", "NOT_RUN", "SKIPPED_BY_USER"):
            with self.subTest(status=status):
                self.results["cases"][0]["checkpoints"][0]["status"] = status
                self.assertBlocked("PASS")

    def test_missing_checkpoint_blocks_even_when_other_checkpoint_passes(self):
        self.contract["cases"][0]["checkpoints"].append({
            "id": "CP-002", "expected": "命令回执不重复。", "method": "AUTOMATED",
            "feature": self.feature, "scenario": "重复提交只建一单",
        })
        self.assertBlocked("checkpoint")

    def test_empty_case_set_is_not_vacuous_success(self):
        self.contract["cases"] = []
        self.results["cases"] = []
        self.assertBlocked("cases")

    def test_implementation_revision_must_match_verified_target(self):
        self.results["implementation_revision"] = "b" * 40
        self.assertBlocked("revision")

    def test_old_native_run_cannot_be_relabelled_with_new_results_revision(self):
        self.run["implementation_revision"] = "b" * 40
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run revision")

    def test_native_run_without_deployment_provenance_remains_blocked(self):
        del self.run["implementation_revision"]
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run revision")

    def test_unmapped_success_criterion_blocks(self):
        self.contract["requirements"] = self.contract["requirements"][:1]
        self.assertBlocked("SC-001")

    def test_markdown_requirement_forms_cannot_shrink_scope(self):
        spec_path = self.project / f"docs/specs/{self.change}/spec.md"
        original = spec_path.read_text(encoding="utf-8")
        covered = list(self.contract["requirements"])
        forms = (
            "- **{id}**: 必须拒绝未授权请求。",
            "* **{id}**: 必须拒绝未授权请求。",
            "+ **{id}**: 必须拒绝未授权请求。",
            "1. **{id}**: 必须拒绝未授权请求。",
            "**{id}**: 必须拒绝未授权请求。",
            "- {id}: 必须拒绝未授权请求。",
            "| {id} | 必须拒绝未授权请求。 |",
            "验收要求{id}：必须拒绝未授权请求。",
            "**{id}**：必须拒绝未授权请求。",
            "## {id} 必须拒绝未授权请求。",
        )
        for ident in ("FR-002", "SC-002"):
            for form in forms:
                with self.subTest(requirement=ident, form=form):
                    self.write_text(spec_path, original + form.format(id=ident) + "\n")
                    self.contract["baseline"]["spec"] = self.ref(spec_path)
                    self.contract["requirements"] = list(covered)
                    self.assertBlocked("unmapped requirements: " + ident)
                    self.contract["requirements"].append({"id": ident, "case_ids": ["AUT-001"]})
                    self.save()
                    result = self.check()
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn("EVIDENCE_CHECK_PASS", result.stdout)

    def test_explicit_scope_exclusion_requires_bound_review_record(self):
        self.contract["requirements"] = self.contract["requirements"][:1]
        self.contract["exclusions"] = [{"id": "SC-001", "reason": "仅为校验显式范围审核的合成输入"}]
        self.save()
        self.write_json(self.approval_path, {"status": "PENDING", "contract_sha256": digest(self.contract_path)})
        self.results["scope_approval"] = self.ref(self.approval_path)
        self.write_json(self.results_path, self.results)
        result = self.check()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("APPROVED", result.stdout + result.stderr)

    def test_known_blocker_cannot_be_hidden_by_passing_results(self):
        self.contract["blocking_items"] = [{"id": "REQ-027", "status": "SKIPPED_BY_USER"}]
        self.assertBlocked("blocking_items")

    def test_partial_case_design_cannot_claim_full_case_acceptance(self):
        self.contract["cases"][0]["remaining"] = ["尚缺真实业务原单观察"]
        self.assertBlocked("remaining")

    def test_source_drift_invalidates_historical_pass(self):
        self.write_text(self.ledger / "pm/Product-Spec.md", "产品来源已变化\n")
        self.assertBlocked("sha256")

    def test_unfinalized_or_dry_run_native_evidence_blocks(self):
        for field, value in (("status", "RUNNING"), ("dry_run", True), ("evidence_finalized", False), ("scope", "ORACLE_ONLY")):
            with self.subTest(field=field):
                changed = dict(self.run)
                changed[field] = value
                self.write_json(self.run_path, changed)
                self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
                self.assertBlocked("run")

    def test_skipped_native_scenario_blocks_even_if_top_level_counts_are_green(self):
        self.run["verdicts"][0]["skipped"] = True
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run")

    def test_zero_scenario_run_is_not_execution(self):
        self.run["passed"] = 0
        self.run["verdicts"] = []
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run")

    def test_authored_observation_json_is_not_native_run_evidence(self):
        self.write_json(self.run_path, {"count": 1, "status": "PASS"})
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run")

    def test_feature_drift_invalidates_finalized_run(self):
        self.write_text(self.workbench / self.feature, "Feature: 修改后的特性\n")
        self.assertBlocked("input_sha256")

    def test_duplicate_result_checkpoint_cannot_replace_missing_coverage(self):
        self.results["cases"][0]["checkpoints"] *= 2
        self.assertBlocked("duplicate")

    def test_evidence_outside_workbench_is_rejected(self):
        outside = Path(self.temp.name) / "outside.json"
        self.write_json(outside, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = {"path": str(outside), "sha256": digest(outside)}
        self.assertBlocked("workbench")

    def prepare_manual(self):
        expected = self.contract["cases"][0]["checkpoints"][0]
        expected["method"] = "MANUAL"
        expected.pop("feature")
        expected.pop("scenario")
        self.manual_path = self.workbench / "reports/manual/manual-001.json"
        self.manual = {
            "kind": "MANUAL_ACCEPTANCE", "implementation_revision": self.revision,
            "environment": "isolated-synthetic-pg18", "case_id": "AUT-001", "checkpoint_id": "CP-001",
            "context_sha256": digest(self.context_path),
            "status": "PASS", "expected": expected["expected"], "actual": "人工观察到一张合成付款申请。",
            "executed_by": "synthetic-reviewer", "executed_at": "2099-01-01T12:00:00+00:00",
        }
        self.write_json(self.manual_path, self.manual)
        point = self.results["cases"][0]["checkpoints"][0]
        point.pop("run_id")
        point["actual"] = self.manual["actual"]
        point["evidence"] = self.ref(self.manual_path)
        self.save()

    def test_manual_checkpoint_requires_bound_expected_actual_and_executor(self):
        self.prepare_manual()
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_manual_record_from_another_checkpoint_is_rejected(self):
        self.prepare_manual()
        self.manual["checkpoint_id"] = "CP-OTHER"
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("manual evidence case/checkpoint")

    def test_manual_record_with_changed_expected_is_rejected(self):
        self.prepare_manual()
        self.manual["expected"] = "弱化后的预期"
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("manual evidence")

    def test_manual_record_without_executor_is_rejected(self):
        self.prepare_manual()
        self.manual["executed_by"] = ""
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("manual executed_by")

    def test_source_document_cannot_be_replaced_with_an_unrelated_file(self):
        replacement = self.workbench / "unrelated.md"
        self.write_text(replacement, "AUT-001\n")
        self.contract["baseline"]["formal_cases"] = self.ref(replacement)
        self.assertBlocked("formal_cases authority")

    def test_case_outside_original_corpus_is_not_formal_authority(self):
        self.write_text(self.workbench / "完整测试点与测试用例.md", "# 没有本用例\n")
        self.contract["baseline"]["formal_cases"] = self.ref(self.workbench / "完整测试点与测试用例.md")
        self.assertBlocked("original case source")

    def test_approval_must_match_current_contract_hash(self):
        self.write_json(self.approval_path, {"status": "APPROVED", "contract_sha256": "0" * 64, "reviewed_by": "reviewer", "reviewed_at": "2099-01-01T11:00:00+00:00"})
        self.results["scope_approval"] = self.ref(self.approval_path)
        self.write_json(self.results_path, self.results)
        result = self.check()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("scope approval", result.stdout + result.stderr)

    def test_malformed_json_fails_closed_without_traceback(self):
        self.write_text(self.results_path, "{broken json")
        result = self.check()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("EVIDENCE_CHECK_BLOCKED", result.stderr)
        self.assertNotIn("Traceback", result.stderr)

    def test_duplicate_json_keys_cannot_hide_blockers(self):
        self.write_text(self.results_path, '{"blockers":["failed"],"blockers":[]}')
        result = self.check()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("duplicate JSON key", result.stdout + result.stderr)

    def test_native_input_symlink_cannot_escape_workbench(self):
        outside = Path(self.temp.name) / "outside-input.txt"
        self.write_text(outside, "not a workbench fixture")
        link = self.workbench / "fixtures/escaped.txt"
        link.parent.mkdir(parents=True, exist_ok=True)
        link.symlink_to(outside)
        self.run["input_sha256"]["fixtures/escaped.txt"] = digest(outside)
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("workbench")

    def test_native_run_from_another_fixture_context_cannot_be_combined(self):
        self.run["case_context_sha256"] = {"AUT-001": "b" * 64}
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("context")

    def test_manual_record_from_another_fixture_context_cannot_be_combined(self):
        self.prepare_manual()
        self.manual["context_sha256"] = "b" * 64
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("context")

    def test_native_execution_before_scope_review_is_rejected(self):
        self.run["executed_at"] = "2099-01-01T10:00:00+00:00"
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run execution time")

    def test_native_execution_after_result_summary_is_rejected(self):
        self.run["executed_at"] = "2099-01-01T13:00:00+00:00"
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run execution time")

    def test_native_run_without_execution_time_is_rejected(self):
        del self.run["executed_at"]
        self.write_json(self.run_path, self.run)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.run_path)
        self.assertBlocked("run execution time")

    def test_manual_execution_before_scope_review_is_rejected(self):
        self.prepare_manual()
        self.manual["executed_at"] = "2099-01-01T10:00:00+00:00"
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("manual execution time")

    def test_manual_execution_after_result_summary_is_rejected(self):
        self.prepare_manual()
        self.manual["executed_at"] = "2099-01-01T13:00:00+00:00"
        self.write_json(self.manual_path, self.manual)
        self.results["cases"][0]["checkpoints"][0]["evidence"] = self.ref(self.manual_path)
        self.assertBlocked("manual execution time")

    def test_lower_evidence_level_cannot_pass_formal_gate(self):
        self.contract["acceptance_level"] = "KERNEL"
        self.assertBlocked("FORMAL")


if __name__ == "__main__":
    unittest.main()
