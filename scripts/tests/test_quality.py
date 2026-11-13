"""Behavioral tests for repository gates; JSON fixtures are not execution evidence."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]


class QualityPolicyTest(unittest.TestCase):
    def setUp(self):
        path = SCRIPTS / "quality_policy.py"
        self.assertTrue(path.is_file(), "repository qualification policy is not implemented")
        spec = importlib.util.spec_from_file_location("quality_policy_under_test", path)
        self.q = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.q)
        self.policy = {
            "targets": ["linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64", "windows_amd64", "windows_arm64"],
            "required_tests": {"all": ["p#TestMust"], "windows": ["p#TestWindows"]},
            "allowed_skips": [{"test": "p#TestWindows", "os": ["linux", "darwin"], "reason": "native Windows only"}],
            "required_checks": ["tools", "root-full", "example-full", "delivery", "source-stable"],
        }

    def events(self, skip=False):
        events = [{"Action": "start", "Package": "p"}, {"Action": "run", "Package": "p", "Test": "TestMust"}]
        if skip:
            events.append({"Action": "output", "Package": "p", "Test": "TestMust", "Output": "missing required tool\n"})
        events += [{"Action": "skip" if skip else "pass", "Package": "p", "Test": "TestMust", "Elapsed": 0.01}, {"Action": "pass", "Package": "p", "Elapsed": 0.02}]
        return events

    def evaluate(self, events, target="linux_amd64"):
        return self.q.evaluate_go("\n".join(json.dumps(e) for e in events), {"p": True}, target, self.policy)

    def test_required_go_missing_skipped_or_incomplete_is_blocked(self):
        result = self.evaluate(self.events())
        self.assertEqual(result["pass"], 1)
        self.assertEqual(result["tests"]["p#TestMust"], "pass")
        for events in (self.events(True), self.events()[:-2], self.events()[:1] + self.events()[-1:], []):
            with self.subTest(events=events), self.assertRaises(self.q.GateError):
                self.evaluate(events)

    def test_go_duplicate_cached_or_failed_events_are_blocked(self):
        good = self.events()
        cases = [good[:3] + [good[2]] + good[3:], good[:-1] + [{"Action": "fail", "Package": "p"}], good[:-1] + [{"Action": "output", "Package": "p", "Output": "ok p (cached)\n"}] + good[-1:], good + [{"Action": "run", "Package": "p", "Test": "TestMust"}]]
        for events in cases:
            with self.subTest(events=events), self.assertRaises(self.q.GateError):
                self.evaluate(events)
        for text in ("{}", "not JSON", "[]"):
            with self.subTest(text=text), self.assertRaises(self.q.GateError):
                self.q.evaluate_go(text, {"p": True}, "linux_amd64", self.policy)

    def test_only_explicit_platform_skip_is_allowed(self):
        events = self.events()[:-1] + [
            {"Action": "run", "Package": "p", "Test": "TestWindows"},
            {"Action": "output", "Package": "p", "Test": "TestWindows", "Output": "    file_test.go:10: native Windows only\n"},
            {"Action": "skip", "Package": "p", "Test": "TestWindows", "Elapsed": 0},
        ] + self.events()[-1:]
        result = self.evaluate(events)
        self.assertEqual(result["skip"], 1)
        with self.assertRaises(self.q.GateError):
            self.evaluate(events, "windows_amd64")
        events[-3]["Output"] = "symlink capability unavailable\n"
        with self.assertRaises(self.q.GateError):
            self.evaluate(events)

    def test_coverage_merges_duplicate_ranges_and_compares_fractions(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "coverage.out"
            p.write_text("mode: atomic\np/a.go:1.1,2.2 2 0\np/a.go:1.1,2.2 2 1\np/b.go:1.1,2.2 3 0\n")
            self.assertEqual(self.q.coverage(p), {"covered": 2, "total": 5})
            self.q.check_coverage({"covered": 50, "total": 100}, {"covered": 80, "total": 200}, None)
            self.q.check_coverage({"covered": 100, "total": 300}, {"covered": 1, "total": 3}, None)
            for actual, base, floor in (({"covered": 49, "total": 100}, {"covered": 1, "total": 2}, None), ({"covered": 0, "total": 0}, {"covered": 1, "total": 2}, None), ({"covered": 1, "total": 2}, {"covered": 1, "total": 3}, {"covered": 3, "total": 4})):
                with self.assertRaises(self.q.GateError):
                    self.q.check_coverage(actual, base, floor)
            p.write_text("mode: set\np/a.go:1.1,2.2 2 1\np/a.go:1.1,2.2 3 0\n")
            with self.assertRaises(self.q.GateError):
                self.q.coverage(p)

    def report(self, target, attempt=1):
        checks = {key: "PASS" for key in self.policy["required_checks"]}
        if target.startswith("linux_"):
            checks.update({"root-race": "PASS", "example-race": "PASS"})
        return {"schema": "mulix.qualification.v1", "status": "PASS", "target": target, "attempt": attempt, "commit": "a" * 40, "bundle_manifest_sha256": "b" * 64, "run_id": "101", "ci": True, "dirty": False, "source_unchanged": True, "checks": checks}

    def test_aggregate_requires_latest_complete_bound_matrix(self):
        reports = [self.report(t) for t in self.policy["targets"]]
        needs = {"_run_id": "101", "bundles": {"result": "success"}, "native": {"result": "success"}, "extended": {"result": "skipped"}}
        def gate(rows=reports, dependencies=needs, event="pull_request"):
            return self.q.aggregate(rows, "a" * 40, "b" * 64, self.policy, dependencies, event)
        self.assertEqual(gate()["status"], "PASS")
        for rows in (reports[:-1], reports + [reports[0]], reports + [dict(reports[0], attempt=2, status="FAIL")], [dict(r, bundle_manifest_sha256="c" * 64) for r in reports], [dict(r, dirty=True) for r in reports], [dict(r, source_unchanged=False) for r in reports], [dict(r, run_id="100") for r in reports]):
            with self.subTest(rows=rows), self.assertRaises(self.q.GateError):
                gate(rows)
        for value in ("skipped", "failure", "cancelled"):
            bad = copy.deepcopy(needs)
            bad["native"]["result"] = value
            with self.assertRaises(self.q.GateError):
                gate(dependencies=bad)
        with self.assertRaises(self.q.GateError):
            gate(event="schedule")
        rerun = reports + [self.report(reports[0]["target"], attempt=2)]
        self.assertEqual(gate(rerun)["status"], "PASS")

    def test_real_go_json_keeps_unicode_line_separators_inside_events(self):
        import os
        import subprocess
        with tempfile.TemporaryDirectory(prefix="mulix-json-protocol-") as temporary:
            root = Path(temporary)
            (root / "go.mod").write_text("module fixture.example/json\n\ngo 1.27.3\n", encoding="utf-8")
            (root / "unicode_test.go").write_text('package fixture\nimport "testing"\nfunc TestUnicode(t *testing.T) { t.Log("before\\u0085after") }\n', encoding="utf-8")
            data = subprocess.check_output(["go", "test", "-count=1", "-json", "."], cwd=root, env=dict(os.environ, GOTOOLCHAIN="local", GOENV="off", GOFLAGS="", GOWORK="off"), text=True, encoding="utf-8", timeout=60)
            self.assertIn("\u0085", data, "fixture must exercise the actual Go wire representation")
            result = self.q.evaluate_go(data, {"fixture.example/json": True}, "linux_amd64", self.policy, required=[])
            self.assertEqual((result["pass"], result["skip"], result["fail"]), (1, 0, 0))

    def test_duplicate_json_keys_are_rejected(self):
        with self.assertRaises(self.q.GateError):
            self.q.strict_json(b'{"status":"FAIL","status":"PASS"}')


class LoggedRunnerTest(unittest.TestCase):
    def setUp(self):
        path = SCRIPTS / "quality_run.py"
        self.assertTrue(path.is_file(), "shared durable command execution is not implemented")
        spec = importlib.util.spec_from_file_location("quality_run_under_test", path)
        self.r = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.r)

    def test_real_exit_and_separate_logs_are_preserved(self):
        import sys
        with tempfile.TemporaryDirectory() as d:
            runner = self.r.Runner(Path(d) / "logs", Path(d))
            result = runner.run("success", [sys.executable, "-I", "-c", "import sys; print('actual stdout'); print('actual stderr', file=sys.stderr)"])
            self.assertEqual(result.stdout, "actual stdout\n")
            self.assertEqual(result.stderr, "actual stderr\n")
            with self.assertRaises(self.r.GateError):
                runner.run("failure", [sys.executable, "-I", "-c", "raise SystemExit(7)"])
            records = json.loads((Path(d) / "logs/commands.json").read_text())
            self.assertEqual([r["exit_code"] for r in records], [0, 7])
            self.assertEqual(records[1]["status"], "FAIL")

    def test_utf8_logs_are_preserved_under_a_non_utf8_parent_locale(self):
        import os
        import subprocess
        import sys
        with tempfile.TemporaryDirectory(prefix="mulix-utf8-log-") as temporary:
            code = "import sys, json; from pathlib import Path; sys.path.insert(0, " + repr(str(SCRIPTS)) + "); from quality_run import Runner; result=Runner(Path(sys.argv[1])/'logs', Path(sys.argv[1])).run('utf8', [sys.executable, '-I', '-B', '-c', \"import os; os.write(1, bytes.fromhex('e4b8ade69687c285'))\"]); print(json.dumps(result.stdout))"
            data = subprocess.check_output([sys.executable, "-I", "-B", "-X", "utf8=0", "-c", code, temporary], env=dict(os.environ, LC_ALL="C"), timeout=30)
            self.assertEqual(json.loads(data), "中文\u0085")

    def test_timeout_is_failed_and_evidence_survives(self):
        import sys
        with tempfile.TemporaryDirectory() as d:
            runner = self.r.Runner(Path(d) / "logs", Path(d))
            with self.assertRaises(self.r.GateError):
                runner.run("timeout", [sys.executable, "-I", "-c", "import threading; threading.Event().wait(30)"], timeout=0.2)
            records = json.loads((Path(d) / "logs/commands.json").read_text())
            self.assertTrue(records[0]["timed_out"])
            self.assertEqual(records[0]["status"], "FAIL")

    def test_child_race_file_blocks_even_when_command_succeeded(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "race"
            path.mkdir()
            self.r.reject_race_logs(path)
            (path / "race.123").write_text("WARNING: DATA RACE\n")
            with self.assertRaises(self.r.GateError):
                self.r.reject_race_logs(path)


class NativeContextTest(unittest.TestCase):
    def setUp(self):
        path = SCRIPTS / "quality.py"
        self.assertTrue(path.is_file(), "native repository qualification is not implemented")
        spec = importlib.util.spec_from_file_location("native_quality_under_test", path)
        self.n = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.n)
        self.source = {"commit": "a" * 40, "fingerprint": "b" * 64, "dirty": False}
        self.manifest = {"source": dict(self.source)}
        self.env = {"GITHUB_ACTIONS": "true", "GITHUB_SHA": "a" * 40, "GITHUB_RUN_ID": "101", "GITHUB_RUN_ATTEMPT": "1"}

    def validate(self, source=None, manifest=None, target="linux_amd64", ci=False, env=None):
        return self.n.validate_context(source or self.source, manifest or self.manifest, target, ci, self.env if env is None else env, "linux_amd64")

    def test_wrong_platform_or_source_and_dirty_ci_cannot_qualify(self):
        self.validate()
        self.validate(ci=True)
        for args in ({"target": "windows_amd64"}, {"manifest": {"source": dict(self.source, commit="c" * 40)}}, {"source": dict(self.source, dirty=True), "ci": True}, {"manifest": {"source": dict(self.source, dirty=True)}, "ci": True}, {"ci": True, "env": {}}, {"ci": True, "env": dict(self.env, GITHUB_SHA="c" * 40)}):
            with self.subTest(args=args), self.assertRaises(self.n.GateError):
                self.validate(**args)

    def test_local_dirty_artifact_is_testable_but_source_drift_is_not(self):
        self.validate(source=dict(self.source, dirty=True), manifest={"source": dict(self.source, dirty=True)})
        with self.assertRaises(self.n.GateError):
            self.validate(source=dict(self.source, dirty=True), manifest={"source": dict(self.source, dirty=True, fingerprint="c" * 64)})

    def test_policy_cannot_drop_targets_or_mandatory_checks(self):
        policy = json.loads((SCRIPTS / "quality-policy.json").read_text())
        self.n.validate_policy(policy)
        for key in ("targets", "required_checks"):
            bad = copy.deepcopy(policy)
            bad[key].pop()
            with self.assertRaises(self.n.GateError):
                self.n.validate_policy(bad)

    def test_each_python_scope_must_execute_nonzero_tests(self):
        from unittest import mock
        import uuid
        scopes = ("scripts/tests", "assets/skills/mulix-verify/tests")
        for empty in scopes:
            with self.subTest(empty=empty), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                for scope in scopes:
                    path = root / scope
                    path.mkdir(parents=True)
                    if scope != empty:
                        (path / ("test_pass_" + uuid.uuid4().hex + ".py")).write_text("import unittest\nclass Case(unittest.TestCase):\n def test_pass(self): self.assertEqual(1, 1)\n", encoding="utf-8")
                with mock.patch.object(self.n, "ROOT", root), self.assertRaises(self.n.GateError):
                    self.n.python_tests(root / "report.json")

    def test_expected_failure_is_not_mandatory_python_success(self):
        from unittest import mock
        import uuid
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for scope in ("scripts/tests", "assets/skills/mulix-verify/tests"):
                path = root / scope
                path.mkdir(parents=True)
                (path / ("test_xfail_" + uuid.uuid4().hex + ".py")).write_text("import unittest\nclass Case(unittest.TestCase):\n @unittest.expectedFailure\n def test_failure(self): self.fail('actual expected failure')\n", encoding="utf-8")
            with mock.patch.object(self.n, "ROOT", root), self.assertRaises(self.n.GateError):
                self.n.python_tests(root / "report.json")

    def test_missing_tool_and_source_or_bundle_drift_are_blocked(self):
        with self.assertRaises(self.n.GateError):
            self.n.require_tool("__mulix_p1_missing_tool__")
        self.n.check_stability(self.source, self.source, {"x": "1"}, {"x": "1"})
        for after, bundle in ((dict(self.source, fingerprint="c" * 64), {"x": "1"}), (self.source, {"x": "2"})):
            with self.assertRaises(self.n.GateError):
                self.n.check_stability(self.source, after, {"x": "1"}, bundle)


if __name__ == "__main__":
    unittest.main()
