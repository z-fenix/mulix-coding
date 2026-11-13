"""Real Go/Git inputs: ignored embeds and hidden index flags cannot look clean."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]


class SourceIdentityTest(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("source_policy_under_test", SCRIPTS / "quality_policy.py")
        self.q = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.q)
        self.temp = tempfile.TemporaryDirectory(prefix="mulix-source-identity-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "go.mod").write_text("module fixture.example/identity\n\ngo 1.27.3\n", encoding="utf-8")
        (self.root / ".gitignore").write_text("*.exe\nbin/\n", encoding="utf-8")
        (self.root / "resources").mkdir()
        (self.root / "resources/base.txt").write_bytes(b"tracked")
        (self.root / "main.go").write_text('''package main
import ("embed"; "fmt")
//go:embed resources
var resources embed.FS
func main() { rows, _ := resources.ReadDir("resources"); for _, row := range rows { fmt.Println(row.Name()) } }
''', encoding="utf-8")
        self.git("init", "-q")
        self.git("add", ".")
        self.git("-c", "user.name=P1 fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture")

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], stderr=subprocess.STDOUT)

    def test_ignored_input_consumed_by_real_go_embed_is_rejected(self):
        clean = self.q.source_snapshot(self.root)
        self.assertFalse(clean["dirty"])
        extra = self.root / "resources/extra.exe"
        extra.write_bytes(b"ignored resource, not executable")
        self.assertEqual(self.git("status", "--porcelain"), b"")
        result = subprocess.check_output(["go", "run", "."], cwd=self.root, env=dict(os.environ, GOTOOLCHAIN="local", GOENV="off", GOFLAGS="", GOWORK="off"), text=True, timeout=60)
        self.assertIn("extra.exe", result, "regression must prove the ignored file is an actual Go build input")
        with self.assertRaisesRegex(self.q.GateError, "ignored.*input"):
            self.q.source_snapshot(self.root)

    def test_input_discovery_uses_the_controlled_build_environment(self):
        from unittest import mock
        before = self.q.source_snapshot(self.root)
        with mock.patch.dict(os.environ, {"GOEXPERIMENT": "invalid-inherited-experiment", "GOAMD64": "v99", "GOARM64": "v99.0"}):
            after = self.q.source_snapshot(self.root)
        self.assertEqual(before, after)

    def test_owned_noninput_transient_exclusion_does_not_hide_other_edits(self):
        before = self.q.source_snapshot(self.root)
        staged = self.root / ".mulix-install-owned.tmp"
        staged.write_bytes(b"private staging bytes")
        self.assertNotEqual(self.q.source_snapshot(self.root)["fingerprint"], before["fingerprint"])
        after = self.q.source_snapshot(self.root, transient_files=(staged,))
        self.assertEqual(after["fingerprint"], before["fingerprint"])
        self.assertTrue(after["dirty"], "exclusion must not invent clean Git status")
        (self.root / "resources/base.txt").write_bytes(b"real changed input")
        self.assertNotEqual(self.q.source_snapshot(self.root, transient_files=(staged,))["fingerprint"], before["fingerprint"])

    def test_transient_exclusions_reject_tracked_files_and_nonregular_paths(self):
        link = self.root / ".mulix-install-link.tmp"
        link.symlink_to(self.root / "resources/base.txt")
        for path in (self.root / "resources/base.txt", self.root / "resources", link):
            with self.subTest(path=path.name), self.assertRaises(self.q.GateError):
                self.q.source_snapshot(self.root, transient_files=(path,))

    def test_transient_exclusions_cannot_hide_actual_embed_inputs(self):
        main = self.root / "main.go"
        original = main.read_bytes()
        for name, all_files in (("owned.tmp", False), (".owned.tmp", True)):
            with self.subTest(name=name):
                main.write_bytes(original.replace(b"go:embed resources", b"go:embed all:resources") if all_files else original)
                staged = self.root / "resources" / name
                staged.write_bytes(b"actual embed input, not exempt staging")
                try:
                    with self.assertRaisesRegex(self.q.GateError, "ignored.*input"):
                        self.q.source_snapshot(self.root, transient_files=(staged,))
                finally:
                    staged.unlink()
                    main.write_bytes(original)

    def test_assume_unchanged_and_skip_worktree_cannot_hide_source_edits(self):
        for flag, undo in (("--assume-unchanged", "--no-assume-unchanged"), ("--skip-worktree", "--no-skip-worktree")):
            with self.subTest(flag=flag):
                self.git("update-index", flag, "resources/base.txt")
                (self.root / "resources/base.txt").write_bytes(b"different actual build bytes")
                self.assertEqual(self.git("status", "--porcelain"), b"")
                with self.assertRaisesRegex(self.q.GateError, "index flag"):
                    self.q.source_snapshot(self.root)
                self.git("update-index", undo, "resources/base.txt")
                (self.root / "resources/base.txt").write_bytes(b"tracked")


if __name__ == "__main__":
    unittest.main()
