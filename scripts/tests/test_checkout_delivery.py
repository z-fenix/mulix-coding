"""In-checkout delivery paths must not confuse owned staging with source edits."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]


class CheckoutDeliveryTest(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("checkout_delivery_under_test", SCRIPTS / "delivery.py")
        self.d = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.d)
        self.temp = tempfile.TemporaryDirectory(prefix="mulix-checkout-delivery-")
        self.addCleanup(self.temp.cleanup)
        self.case = Path(self.temp.name)
        self.source = self.case / "checkout"
        self.source.mkdir()
        (self.source / "go.mod").write_text("module fixture.example/checkout-delivery\n\ngo 1.27.3\n", encoding="utf-8")
        self.main = self.source / "cmd/mulix/main.go"
        self.main.parent.mkdir(parents=True)
        self.main.write_text('''package main
import "fmt"
var version = "dev"
func main() { fmt.Println("mulix " + version) }
''', encoding="utf-8")
        self.git("init", "-q")
        self.git("add", ".")
        self.git("-c", "user.name=P1 fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture")

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.source), *args], stderr=subprocess.STDOUT)

    def version(self, binary, expected):
        result = subprocess.run([str(binary), "version"], capture_output=True, text=True, timeout=30)
        self.assertEqual((result.returncode, result.stdout, result.stderr), (0, "mulix " + expected + "\n", ""))

    def test_release_supports_new_unignored_in_checkout_output(self):
        output = self.source / "deliverables 中文" / "new-parent" / "bundle"
        before = self.d.source_snapshot(self.source)
        original = self.main.read_bytes()
        manifest = self.d.build_bundle(self.source, output, "p1-checkout-output")
        self.assertEqual(manifest["source"], {key: before[key] for key in ("commit", "fingerprint", "dirty")})
        self.assertEqual(self.d.verify_bundle(output), manifest)
        self.assertEqual(len(list(output.iterdir())), 8)
        self.assertFalse(list(output.parent.glob(".mulix-release-*")))
        installed = self.d.install_bundle(output, self.case / "artifact-prefix")
        self.version(installed, "p1-checkout-output")
        self.assertEqual(self.main.read_bytes(), original)
        self.assertEqual(self.git("diff", "--name-only"), b"")
        self.assertTrue(self.git("status", "--porcelain"), "fixture output must really be unignored")

    def test_source_install_supports_fresh_and_existing_unignored_prefixes(self):
        original = self.main.read_bytes()
        for existing in (False, True):
            with self.subTest(existing=existing):
                prefix = self.source / "install paths 中文" / ("existing" if existing else "fresh")
                if existing:
                    prefix.mkdir(parents=True)
                    (prefix / self.d.binary_name(self.d.native_target())).write_bytes(b"OLD-EXECUTABLE")
                    (prefix / "user-data.txt").write_bytes(b"preserve-user-data")
                installed = self.d.install_source(self.source, prefix, "p1-checkout-install")
                self.version(installed, "p1-checkout-install")
                if existing:
                    self.assertEqual((prefix / "user-data.txt").read_bytes(), b"preserve-user-data")
                self.assertFalse(list(prefix.glob(".mulix-install-*")))
        self.assertEqual(self.main.read_bytes(), original)
        self.assertEqual(self.git("diff", "--name-only"), b"")

    def test_release_still_rejects_real_source_changes(self):
        output = self.source / "deliverables" / "blocked-bundle"
        run = self.d.command
        changed = False
        def change_source(argv, cwd, env=None):
            nonlocal changed
            result = run(argv, cwd, env)
            if argv[:2] == ["go", "build"] and not changed:
                self.main.write_bytes(self.main.read_bytes() + b"\n// changed during actual compilation\n")
                changed = True
            return result
        with mock.patch.object(self.d, "command", side_effect=change_source):
            with self.assertRaisesRegex(self.d.GateError, "source changed"):
                self.d.build_bundle(self.source, output, "p1-source-change")
        self.assertTrue(changed)
        self.assertFalse(output.exists(), "changed source must not publish a bundle")

    def test_in_checkout_install_still_rejects_real_source_changes(self):
        prefix = self.source / "install paths 中文" / "rejected"
        prefix.mkdir(parents=True)
        binary = prefix / self.d.binary_name(self.d.native_target())
        binary.write_bytes(b"OLD-EXECUTABLE")
        (prefix / "user-data.txt").write_bytes(b"preserve-user-data")
        run = self.d.command
        changed = False
        def change_source(argv, cwd, env=None):
            nonlocal changed
            result = run(argv, cwd, env)
            if len(argv) == 2 and argv[-1] == "version" and argv[0] != "go":
                self.main.write_bytes(self.main.read_bytes() + b"\n// changed during staged validation\n")
                changed = True
            return result
        with mock.patch.object(self.d, "command", side_effect=change_source):
            with self.assertRaisesRegex(self.d.GateError, "source changed"):
                self.d.install_source(self.source, prefix, "p1-source-change")
        self.assertTrue(changed)
        self.assertEqual(binary.read_bytes(), b"OLD-EXECUTABLE")
        self.assertEqual((prefix / "user-data.txt").read_bytes(), b"preserve-user-data")
        self.assertEqual({path.name for path in prefix.iterdir()}, {binary.name, "user-data.txt"})

    def test_build_staging_stays_outside_source_when_tempdir_is_inside_it(self):
        temporary = self.source / "custom-tmp"
        temporary.mkdir()
        output = self.source / "deliverables" / "with-in-tree-tmp"
        prefix = self.source / "install paths" / "with-in-tree-tmp"
        with mock.patch.object(tempfile, "tempdir", str(temporary)):
            manifest = self.d.build_bundle(self.source, output, "p1-private-staging")
            self.assertEqual(self.d.verify_bundle(output), manifest)
            installed = self.d.install_source(self.source, prefix, "p1-private-staging")
            self.version(installed, "p1-private-staging")
        self.assertEqual(list(temporary.iterdir()), [])
        self.assertFalse(list(self.source.glob(".mulix-release-*")))
        self.assertFalse(list(prefix.glob(".mulix-install-*")))


if __name__ == "__main__":
    unittest.main()
