"""A real minimized Go failure must survive the extended driver's failure path."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]


class FuzzEvidenceTest(unittest.TestCase):
    def test_extended_finally_preserves_new_real_go_reproducer(self):
        spec = importlib.util.spec_from_file_location("fuzz_quality_under_test", SCRIPTS / "quality.py")
        n = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(n)
        with tempfile.TemporaryDirectory(prefix="mulix-fuzz-evidence-") as temporary:
            root = Path(temporary)
            source = root / "source"
            source.mkdir()
            (source / "go.mod").write_text("module fixture.example/fuzz\n\ngo 1.27.3\n", encoding="utf-8")
            (source / "fuzz_test.go").write_text('''package fixture
import "testing"
func FuzzAcceptanceBlock(f *testing.F) {
 f.Add("")
 f.Fuzz(func(t *testing.T, s string) { if len(s) > 0 { t.Fatal("bounded failure fixture") } })
}
''', encoding="utf-8")
            actual_target = n.d.native_target()
            result = subprocess.run(["go", "test", "-run=^FuzzAcceptanceBlock$", "-fuzz=^FuzzAcceptanceBlock$", "-fuzztime=1s", "-parallel=1", "."], cwd=source, env=n.native_env(actual_target), capture_output=True, text=True, timeout=60)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("failing input written to", (result.stdout + result.stderr).lower())
            generated = list((source / "testdata/fuzz/FuzzAcceptanceBlock").iterdir())
            self.assertTrue(generated)
            reproducer = generated[0].read_bytes()
            self.assertTrue(reproducer.startswith(b"go test fuzz v1\n"))
            corpus = source / "internal/quality/testdata/fuzz/FuzzAcceptanceBlock"
            corpus.mkdir(parents=True)
            (corpus / "existing").write_bytes(b'go test fuzz v1\nstring("")\n')
            output = root / "evidence"
            expected = {
                "github.com/z-fenix/mulix-coding/internal/lockfile": ["TestLock_MutualExclusionAcrossProcesses", "TestLock_KilledOwnerReleasesAndFileRemainsReusable"],
                "github.com/z-fenix/mulix-coding/internal/cliutil": ["TestQualityCLIInterruptedOrChangedExecutionNeverLeavesPass"],
                "github.com/z-fenix/mulix-coding/internal/scaffold": ["TestUpdateReview_SameFilePartialWriteOutcomes"],
            }
            events = []
            for package, tests in expected.items():
                events.append({"Action": "start", "Package": package})
                for test in tests:
                    events += [{"Action": "run", "Package": package, "Test": test}, {"Action": "pass", "Package": package, "Test": test}]
                events.append({"Action": "pass", "Package": package})
            class FailingFuzzRunner:
                def __init__(self, directory, cwd, env):
                    self.output, self.records, self.env = Path(directory), [], env
                    self.output.mkdir()
                def run(self, name, argv, **kwargs):
                    if name == "fuzz":
                        (corpus / generated[0].name).write_bytes(reproducer)
                        raise n.GateError("actual failing corpus fixture")
                    return subprocess.CompletedProcess(argv, 0, "\n".join(json.dumps(e) for e in events), "")
            real_temporary = tempfile.TemporaryDirectory
            # Only the stress process boundary/tool preflight are stubbed. The
            # failure bytes above came from an actual native Go fuzz execution.
            with mock.patch.object(n, "ROOT", source), mock.patch.object(n, "Runner", FailingFuzzRunner), mock.patch.object(n, "packages", return_value={p: True for p in expected}), mock.patch.object(n, "tools"), mock.patch.object(n, "source_snapshot", return_value={"fingerprint": "fixture"}), mock.patch.object(n.d, "native_target", return_value="linux_amd64"), mock.patch.object(n.tempfile, "TemporaryDirectory", side_effect=lambda **kw: real_temporary(prefix="race-fixture-", dir=root)):
                with self.assertRaises(n.GateError):
                    n.extended(type("Args", (), {"output": output})(), {"extended_iterations": 1, "fuzz_seconds": 10})
            saved = output / "fuzz-reproducers/FuzzAcceptanceBlock" / generated[0].name
            self.assertTrue(saved.is_file(), "extended failure lost the real minimized reproducer")
            self.assertEqual(saved.read_bytes(), reproducer)
            self.assertFalse((saved.parent / "existing").exists(), "unchanged checked-in seeds are not new failure evidence")
            report = json.loads((output / "extended-report.json").read_text())
            self.assertEqual(report["status"], "FAIL")
            self.assertIn(saved.relative_to(output).as_posix(), report["fuzz_reproducers"])


if __name__ == "__main__":
    unittest.main()
