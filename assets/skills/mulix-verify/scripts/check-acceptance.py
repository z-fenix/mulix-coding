#!/usr/bin/env python3
"""只读核对正式验收证据的完整性与一致性；不认证人工同意或执行真实性。"""

import argparse
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re
import sys


class GateError(ValueError):
    """输入缺失、漂移或证据不足，必须保持验收阻断。"""


def require(condition, message):
    if not condition:
        raise GateError(message)


def text(value, label):
    require(isinstance(value, str) and value.strip(), label + " must be nonempty")
    return value


def rows(value, label, nonempty=False):
    require(isinstance(value, list), label + " must be an array")
    require(not nonempty or bool(value), label + " must not be empty")
    return value


def indexed(value, label, nonempty=True):
    result = {}
    for row in rows(value, label, nonempty):
        require(isinstance(row, dict), label + " contains a non-object")
        ident = text(row.get("id"), label + " id")
        require(ident not in result, label + " duplicate id: " + ident)
        result[ident] = row
    return result


def unique_json(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key: " + key)
        result[key] = value
    return result


def timestamp(value, label):
    parsed = datetime.fromisoformat(text(value, label).replace("Z", "+00:00"))
    require(parsed.tzinfo is not None, label + " requires a timezone")
    return parsed


def execution_time(value, window, label):
    earliest, latest = window
    observed_at = timestamp(value, label)
    require(earliest <= observed_at <= latest, label + " must be between scope approval and result summary")


class Sources:
    """同次检查缓存输入，并在结束前复核，避免并发修改后仍返回通过。"""

    def __init__(self, project, workbench):
        self.project = project.resolve(strict=True)
        self.workbench = workbench.resolve(strict=True)
        self.snapshots = {}

    def read(self, path):
        path = path.resolve(strict=True)
        if path not in self.snapshots:
            self.snapshots[path] = path.read_bytes()
        return self.snapshots[path]

    def document(self, data):
        return json.loads(data.decode("utf-8"), object_pairs_hook=unique_json)

    def load(self, path):
        return self.document(self.read(path))

    def ref(self, ref, roots, label):
        require(isinstance(ref, dict), label + " needs path and sha256")
        raw = Path(text(ref.get("path"), label + " path"))
        path = (self.project / raw).resolve(strict=True)
        require(any(path.is_relative_to(root) for root in roots), label + " outside allowed workbench/source root")
        expected = text(ref.get("sha256"), label + " sha256")
        require(re.fullmatch(r"[0-9a-f]{64}", expected), label + " invalid sha256")
        data = self.read(path)
        require(hashlib.sha256(data).hexdigest() == expected, label + " sha256 drift: " + str(raw))
        return path, data

    def stable(self):
        for path, original in self.snapshots.items():
            require(path.read_bytes() == original, "source changed during validation: " + str(path))


def native_run(sources, evidence, point, expected, case_id, baseline, context_hash, window):
    path, data = sources.ref(evidence, [sources.workbench / "reports", sources.workbench / "runs"], "run evidence")
    run = sources.document(data)
    require(isinstance(run, dict), "run must be a native case-live object")
    require(run.get("status") == "FINALIZED" and run.get("evidence_finalized") is True, "run must be FINALIZED with evidence_finalized=true")
    require(run.get("dry_run", False) is False, "run dry-run is not execution")
    require(run.get("scope") in ("SAMPLED_PARTIAL_ASSERTIONS", "FULL_CASES"), "run scope cannot be oracle/diagnostic/unknown")
    require(run.get("implementation_revision") == baseline["implementation_revision"], "run revision missing or different from deployment")
    require(run.get("environment") == baseline["environment"], "run environment missing or different from deployment")
    execution_time(run.get("executed_at"), window, "run execution time")
    contexts = run.get("case_context_sha256")
    require(isinstance(contexts, dict) and contexts.get(case_id) == context_hash, "run case context missing or different from approved fixture")
    require(text(point.get("run_id"), "run_id") == run.get("run_id"), "run_id differs from native evidence")
    require(type(run.get("failed")) is int and run["failed"] == 0, "run has failures or missing counts")
    verdicts = rows(run.get("verdicts"), "run verdicts", True)
    require(type(run.get("passed")) is int and run["passed"] == len(verdicts), "run passed count must equal nonzero native verdict count")
    for verdict in verdicts:
        require(isinstance(verdict, dict) and verdict.get("passed") is True and verdict.get("skipped") is False and not verdict.get("error"), "run has failed/skipped/malformed verdicts")
    inputs = run.get("input_sha256")
    require(isinstance(inputs, dict) and bool(inputs), "run input_sha256 must freeze executed sources")
    for name, fingerprint in inputs.items():
        require(isinstance(name, str) and not Path(name).is_absolute(), "run input_sha256 path must be relative to workbench")
        sources.ref({"path": str(sources.workbench / name), "sha256": fingerprint}, [sources.workbench], "run input_sha256")
    feature = text(expected.get("feature"), "checkpoint feature")
    scenario = text(expected.get("scenario"), "checkpoint scenario")
    require(feature in inputs, "run input_sha256 missing bound feature")
    matches = [v for v in verdicts if v.get("path") == feature and v.get("name") == scenario]
    require(len(matches) == 1, "run must uniquely contain bound checkpoint scenario for " + case_id)
    return path


def manual_record(sources, evidence, point, expected, case_id, baseline, context_hash, window):
    _, data = sources.ref(evidence, [sources.workbench / "reports", sources.workbench / "runs"], "manual evidence")
    record = sources.document(data)
    require(isinstance(record, dict) and record.get("kind") == "MANUAL_ACCEPTANCE", "manual evidence requires MANUAL_ACCEPTANCE")
    require(record.get("case_id") == case_id and record.get("checkpoint_id") == expected["id"], "manual evidence case/checkpoint mismatch")
    require(record.get("context_sha256") == context_hash, "manual evidence context differs from approved fixture")
    require(record.get("implementation_revision") == baseline["implementation_revision"] and record.get("environment") == baseline["environment"], "manual evidence revision/environment mismatch")
    require(record.get("status") == "PASS" and record.get("expected") == expected["expected"], "manual evidence must PASS the approved expected result")
    text(record.get("executed_by"), "manual executed_by")
    execution_time(record.get("executed_at"), window, "manual execution time")
    require(text(record.get("actual"), "manual actual") == point["actual"], "manual evidence actual result mismatch")


def check_acceptance(project, workbench, contract_path, results_path, revision):
    sources = Sources(project, workbench)
    contract_data = sources.read(contract_path)
    contract = sources.document(contract_data)
    results = sources.load(results_path)
    require(isinstance(contract, dict) and isinstance(results, dict), "contract/results must be JSON objects")
    require(type(contract.get("schema_version")) is int and contract["schema_version"] == 1 and type(results.get("schema_version")) is int and results["schema_version"] == 1, "unsupported schema_version")
    change = text(contract.get("change"), "change")
    require(re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9._-]*", change) and change not in (".", ".."), "invalid change")
    require(results.get("change") == change, "results change mismatch")
    require(contract.get("acceptance_level") == "FORMAL", "FORMAL evidence required; kernel/integration completion is separate")
    contract_hash = hashlib.sha256(contract_data).hexdigest()
    require(results.get("contract_sha256") == contract_hash, "results contract_sha256 drift")
    baseline = contract.get("baseline")
    require(isinstance(baseline, dict), "baseline required")
    require(re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", revision), "target implementation revision must be a full commit hash")
    require(baseline.get("implementation_revision") == revision and results.get("implementation_revision") == revision, "implementation revision mismatch")
    environment = text(baseline.get("environment"), "environment")
    require(results.get("environment") == environment, "results environment mismatch")
    executed_at = timestamp(results.get("executed_at"), "results executed_at")
    for owner, field in ((contract, "blocking_items"), (results, "blockers")):
        require(not rows(owner.get(field), field), field + " still contains unresolved acceptance blockers")

    spec_path, spec_data = sources.ref(baseline.get("spec"), [sources.project / "docs/specs" / change], "spec")
    product_root = (sources.project.parent / "pm").resolve()
    product_path, _ = sources.ref(baseline.get("product"), [product_root], "product")
    formal_path, formal_data = sources.ref(baseline.get("formal_cases"), [sources.workbench], "formal_cases")
    require(spec_path == sources.project / "docs/specs" / change / "spec.md", "spec authority must be the change spec.md")
    require(product_path == product_root / "Product-Spec.md", "product authority must be pm/Product-Spec.md")
    require(formal_path == sources.workbench / "完整测试点与测试用例.md", "formal_cases authority must be the original workbench case document")
    formal_text = formal_data.decode("utf-8")
    # Native P0 tracks IDs independently of Markdown declaration formatting.
    # ASCII boundaries also retain IDs adjacent to Chinese prose.
    source_ids = set(re.findall(r"\b(?:FR|SC)-[0-9]+\b", spec_data.decode("utf-8"), re.ASCII))
    require(bool(source_ids), "spec must contain nonempty FR/SC acceptance scope")
    requirements = indexed(contract.get("requirements"), "requirements")
    exclusions = indexed(contract.get("exclusions"), "exclusions", False)
    require(not set(requirements) & set(exclusions), "requirement cannot be both covered and excluded")
    missing = source_ids - set(requirements) - set(exclusions)
    require(not missing, "unmapped requirements: " + ", ".join(sorted(missing)))
    require(set(requirements) | set(exclusions) == source_ids, "scope contains unknown requirement ids")
    for item in exclusions.values():
        text(item.get("reason"), "scope exclusion reason")

    _, approval_data = sources.ref(results.get("scope_approval"), [sources.workbench / "traceability", sources.project / "docs/changes" / change], "scope approval")
    approval = sources.document(approval_data)
    require(isinstance(approval, dict) and approval.get("status") == "APPROVED", "scope review must be APPROVED, not inferred")
    require(approval.get("contract_sha256") == contract_hash, "scope approval is not bound to current contract")
    text(approval.get("reviewed_by"), "scope reviewed_by")
    reviewed_at = timestamp(approval.get("reviewed_at"), "scope reviewed_at")
    require(reviewed_at <= executed_at, "scope approval must precede execution")
    window = reviewed_at, executed_at

    inventory = {}
    for ref in rows(contract.get("case_sources"), "case_sources", True):
        path, data = sources.ref(ref, [sources.workbench / "requirements/case"], "case source")
        require(path.name == "cases.json", "case source must be workbench cases.json, not a second inventory")
        for row in rows(sources.document(data), "case source entries", True):
            require(isinstance(row, dict), "malformed case source entry")
            ident = text(row.get("id"), "case source id")
            require(ident not in inventory, "duplicate formal case source id: " + ident)
            inventory[ident] = row
    cases = indexed(contract.get("cases"), "cases")
    actual_cases = indexed(results.get("cases"), "result cases")
    require(set(cases) == set(actual_cases), "result cases do not match approved case scope")
    referenced = set()
    for item in requirements.values():
        ids = rows(item.get("case_ids"), "requirement case_ids", True)
        require(all(isinstance(ident, str) for ident in ids) and len(set(ids)) == len(ids), "requirement case_ids must be unique strings")
        referenced.update(ids)
    require(referenced == set(cases), "requirements have missing/orphan case bindings")
    count = 0
    for case_id, case in cases.items():
        require(case_id in inventory and inventory[case_id].get("admission") == "formal", "case is not formal: " + case_id)
        require(re.search(r"(?<![\w-])" + re.escape(case_id) + r"(?![\w-])", formal_text), "formal case id absent from original case source: " + case_id)
        require(case.get("coverage") == "FULL", "case coverage is not FULL: " + case_id)
        require(not rows(case.get("remaining"), "case remaining"), "case remaining obligations are not empty: " + case_id)
        _, context_data = sources.ref(case.get("context"), [sources.workbench], "case context")
        context_hash = hashlib.sha256(context_data).hexdigest()
        checkpoints = indexed(case.get("checkpoints"), "contract checkpoints")
        actual_points = indexed(actual_cases[case_id].get("checkpoints"), "result checkpoints")
        require(set(checkpoints) == set(actual_points), "result checkpoint scope differs from contract: " + case_id)
        for point_id, expected in checkpoints.items():
            point = actual_points[point_id]
            text(expected.get("expected"), "checkpoint expected")
            require(point.get("status") == "PASS", "checkpoint must PASS: " + case_id + "/" + point_id)
            text(point.get("actual"), "checkpoint actual")
            method = expected.get("method")
            require(method in ("AUTOMATED", "MANUAL"), "unknown checkpoint execution method")
            if method == "AUTOMATED":
                native_run(sources, point.get("evidence"), point, expected, case_id, baseline, context_hash, window)
            else:
                manual_record(sources, point.get("evidence"), point, expected, case_id, baseline, context_hash, window)
            count += 1
    sources.stable()
    return len(cases), count


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", type=Path, default=Path.cwd())
    parser.add_argument("--workbench-root", type=Path, help="defaults to the project's sibling test directory")
    parser.add_argument("--contract", type=Path, required=True)
    parser.add_argument("--results", type=Path, required=True)
    parser.add_argument("--implementation-revision", required=True, help="full revision independently verified against the deployed target")
    args = parser.parse_args(argv)
    project = args.project_root.resolve()
    workbench = args.workbench_root or project.parent / "test"
    try:
        cases, checkpoints = check_acceptance(project, workbench, args.contract, args.results, args.implementation_revision)
    except (GateError, OSError, ValueError, TypeError, KeyError, AttributeError) as error:
        print("EVIDENCE_CHECK_BLOCKED: " + str(error), file=sys.stderr)
        return 1
    print(f"EVIDENCE_CHECK_PASS: {cases} formal cases, {checkpoints} checkpoints; integrity only, not business sign-off or authenticated human consent.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
