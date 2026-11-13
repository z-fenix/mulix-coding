"""Real private bundle/filesystem tests; damaged archives are negative fixtures."""
import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import time
import tempfile
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]
ROOT = SCRIPTS.parent
TARGETS = {"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64", "windows_amd64", "windows_arm64"}


def digest(data):
    return hashlib.sha256(data).hexdigest()


class ReleaseSafetyTest(unittest.TestCase):
    def test_shell_wrappers_support_basename_invocation(self):
        for name, args in (("install.sh", ["--help"]), ("release.sh", ["--dry-run", "--version", "p1-basename"])):
            with self.subTest(script=name):
                result = subprocess.run(["bash", name, *args], cwd=SCRIPTS, env=dict(os.environ, PYTHON=sys.executable), capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_failed_release_never_removes_existing_dist(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            (root / "scripts").mkdir()
            for name in ("release.sh", "delivery.py", "quality_policy.py", "quality_process.py", "quality-policy.json"):
                if (SCRIPTS / name).exists():
                    shutil.copyfile(SCRIPTS / name, root / "scripts" / name)
            (root / "dist").mkdir()
            sentinel = root / "dist" / "user-owned.txt"
            sentinel.write_bytes(b"preserve-existing-builds")
            result = subprocess.run(["bash", (root / "scripts/release.sh").as_posix()], env=dict(os.environ, MULIX_VERSION="p1-safety"), capture_output=True, timeout=60)
            self.assertNotEqual(result.returncode, 0, "private incomplete source cannot produce a release")
            self.assertTrue(sentinel.exists(), "failed release deleted the pre-existing dist tree")
            self.assertEqual(sentinel.read_bytes(), b"preserve-existing-builds")
    def test_unknown_installer_option_is_not_success(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            scripts = root / "scripts"
            scripts.mkdir()
            # Exercise argument behavior, not an unrelated CRLF shell parse error.
            for name in ("install.sh", "delivery.py", "quality_policy.py", "quality_process.py", "quality-policy.json"):
                if (SCRIPTS / name).exists():
                    (scripts / name).write_bytes((SCRIPTS / name).read_bytes().replace(b"\r\n", b"\n"))
            entry = ["bash", (scripts / "install.sh").as_posix()]
            healthy = subprocess.run([*entry, "--help"], capture_output=True, timeout=30)
            self.assertEqual(healthy.returncode, 0, healthy.stderr)
            prefix = root / "prefix"
            result = subprocess.run([*entry, "--prefix", str(prefix), "--unknown-option"], capture_output=True, timeout=30)
            self.assertNotEqual(result.returncode, 0, "unknown installer options must not claim success")
            self.assertFalse(prefix.exists())


class DeliveryBundleTest(unittest.TestCase):
    fixture = None

    def setUp(self):
        path = SCRIPTS / "delivery.py"
        self.assertTrue(path.is_file(), "final release bundle tool is not implemented")
        if type(self).fixture is None:
            spec = importlib.util.spec_from_file_location("delivery_under_test", path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            type(self).module = module
            type(self).fixture = tempfile.TemporaryDirectory(prefix="mulix-bundle-tests-")
            type(self).bundle = Path(self.fixture.name) / "original"
            type(self).manifest = module.build_bundle(ROOT, self.bundle, "p1-unit")
        self.d = self.module
        self.temp = tempfile.TemporaryDirectory(prefix="mulix-delivery-case-")
        self.addCleanup(self.temp.cleanup)
        self.case = Path(self.temp.name)

    @classmethod
    def tearDownClass(cls):
        if cls.fixture is not None:
            cls.fixture.cleanup()
            cls.fixture = None

    def damaged(self):
        path = self.case / "damaged"
        shutil.copytree(self.bundle, path)
        return path, copy.deepcopy(self.manifest)

    def rewrite(self, path, manifest):
        (path / "manifest.json").write_text(json.dumps(manifest, sort_keys=True) + "\n")
        self.checksums(path)

    def checksums(self, path):
        files = sorted(p for p in path.iterdir() if p.name != "SHA256SUMS")
        (path / "SHA256SUMS").write_text("".join(digest(p.read_bytes()) + "  " + p.name + "\n" for p in files))

    def archive_fixture(self, member_name="mulix", symlink=False, body=b"not-an-executable"):
        path, manifest = self.damaged()
        row = next(r for r in manifest["artifacts"] if r["target"] == "linux_amd64")
        archive = path / row["archive"]
        with tarfile.open(archive, "w:gz") as tf:
            member = tarfile.TarInfo(member_name)
            member.mode = 0o755
            if symlink:
                member.type, member.linkname = tarfile.SYMTYPE, "../../outside"
            else:
                member.size = len(body)
            tf.addfile(member, None if symlink else io.BytesIO(body))
        row.update(archive_sha256=digest(archive.read_bytes()), binary_sha256=digest(body), binary_size=len(body))
        self.rewrite(path, manifest)
        return path

    def test_complete_real_six_target_bundle_verifies(self):
        manifest = self.d.verify_bundle(self.bundle)
        self.assertEqual(manifest["schema"], "mulix.release.v1")
        self.assertEqual({r["target"] for r in manifest["artifacts"]}, TARGETS)
        self.assertEqual(manifest["version"], "p1-unit")
        data = self.d.binary_bytes(self.bundle, manifest, self.d.native_target())
        row = next(r for r in manifest["artifacts"] if r["target"] == self.d.native_target())
        self.assertEqual(digest(data), row["binary_sha256"])

    def test_existing_output_and_invalid_version_do_not_change_data(self):
        out = self.case / "existing"
        out.mkdir()
        sentinel = out / "notes"
        sentinel.write_bytes(b"user-data")
        with self.assertRaises(self.d.GateError):
            self.d.build_bundle(ROOT, out, "p1-test")
        self.assertEqual(sentinel.read_bytes(), b"user-data")
        for version in ("../escape", "x -X main.version=wrong", "", "x\nwrong"):
            with self.subTest(version=version), self.assertRaises(self.d.GateError):
                self.d.build_bundle(ROOT, self.case / "new", version)
            self.assertFalse((self.case / "new").exists())

    def test_failed_build_does_not_publish_partial_bundle(self):
        run = self.d.command
        def fail_build(argv, cwd, env=None):
            if argv[:2] == ["go", "build"]:
                raise self.d.GateError("injected compiler failure")
            return run(argv, cwd, env)
        with mock.patch.object(self.d, "command", side_effect=fail_build):
            with self.assertRaises(self.d.GateError):
                self.d.build_bundle(ROOT, self.case / "not-published", "p1-failed")
        self.assertFalse((self.case / "not-published").exists())

    def test_tar_extended_metadata_is_rejected_before_payload_processing(self):
        path, manifest = self.damaged()
        row = next(r for r in manifest["artifacts"] if r["target"] == "linux_amd64")
        body = self.d.binary_bytes(self.bundle, self.manifest, row["target"])
        archive = path / row["archive"]
        with tarfile.open(archive, "w:gz", format=tarfile.PAX_FORMAT) as tf:
            item = tarfile.TarInfo("mulix")
            item.size, item.mode = len(body), 0o755
            item.pax_headers = {"comment": "bounded metadata regression " * 4096}
            tf.addfile(item, io.BytesIO(body))
        row["archive_sha256"] = digest(archive.read_bytes())
        self.rewrite(path, manifest)
        with self.assertRaises(self.d.GateError):
            self.d.verify_bundle(path)

    def test_checksum_consistent_gzip_truncation_and_crc_are_rejected(self):
        for kind in ("missing-footer", "bad-crc"):
            with self.subTest(kind=kind):
                path, manifest = self.damaged()
                row = next(r for r in manifest["artifacts"] if r["target"] == "linux_amd64")
                archive = path / row["archive"]
                compressed = bytearray(archive.read_bytes())
                if kind == "missing-footer":
                    del compressed[-8:]
                else:
                    compressed[-8] ^= 1
                archive.write_bytes(compressed)
                row["archive_sha256"] = digest(compressed)
                self.rewrite(path, manifest)
                with self.assertRaises(self.d.GateError):
                    self.d.verify_bundle(path)
            shutil.rmtree(path)

    def test_checksum_and_truncated_archive_are_blocked(self):
        path, manifest = self.damaged()
        row = manifest["artifacts"][0]
        archive = path / row["archive"]
        archive.write_bytes(archive.read_bytes()[:100])
        with self.assertRaises(self.d.GateError):
            self.d.verify_bundle(path)
        row["archive_sha256"] = digest(archive.read_bytes())
        self.rewrite(path, manifest)
        with self.assertRaises(self.d.GateError):
            self.d.verify_bundle(path)

    def test_traversal_symlink_and_wrong_binary_format_are_blocked(self):
        for name, link in (("../../escaped", False), ("mulix", True), ("mulix", False)):
            with self.subTest(name=name, symlink=link):
                path = self.archive_fixture(member_name=name, symlink=link)
                with self.assertRaises(self.d.GateError):
                    self.d.verify_bundle(path)
                shutil.rmtree(path)
        self.assertFalse((self.case / "escaped").exists())

    def test_unlisted_files_directories_and_outside_symlinks_are_rejected(self):
        outside = self.case / "outside-owned-fixture"
        outside.mkdir()
        (outside / "sentinel").write_bytes(b"must not be copied or followed")
        for kind in ("file", "directory", "symlink"):
            with self.subTest(kind=kind):
                path, _ = self.damaged()
                extra = path / "unlisted-entry"
                if kind == "file":
                    extra.write_bytes(b"unlisted")
                elif kind == "directory":
                    extra.mkdir()
                else:
                    extra.symlink_to(outside, target_is_directory=True)
                with self.assertRaises(self.d.GateError):
                    self.d.verify_bundle(path)
                with self.assertRaises(self.d.GateError):
                    self.d.bundle_snapshot(path)
            shutil.rmtree(path)
        self.assertEqual((outside / "sentinel").read_bytes(), b"must not be copied or followed")

    def test_manifest_ambiguity_and_target_omission_are_blocked(self):
        path, manifest = self.damaged()
        manifest["artifacts"].pop()
        self.rewrite(path, manifest)
        with self.assertRaises(self.d.GateError):
            self.d.verify_bundle(path)
        (path / "manifest.json").write_bytes(b'{"schema":"wrong",' + (self.bundle / "manifest.json").read_bytes()[1:])
        self.checksums(path)
        with self.assertRaises(self.d.GateError):
            self.d.verify_bundle(path)

    def test_release_records_toolchain_in_the_controlled_build_environment(self):
        expected = subprocess.check_output(["go", "version"], cwd=ROOT, env=dict(os.environ, GOTOOLCHAIN="local", GOENV="off"), text=True).strip()
        with mock.patch.dict(os.environ, {"GOTOOLCHAIN": "invalid-inherited-toolchain"}):
            manifest = self.d.build_bundle(ROOT, self.case / "controlled-toolchain", "p1-toolchain")
        self.assertEqual(manifest["go_version"], expected)

    def test_release_dry_run_and_unknown_option_have_safe_effects(self):
        out = self.case / "dry"
        args = ["bash", (SCRIPTS / "release.sh").as_posix(), "--dry-run", "--output", str(out)]
        env = dict(os.environ, MULIX_VERSION="p1-dry")
        result = subprocess.run(args, cwd=ROOT, env=env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(out.exists())
        self.assertIn('"dry_run": true', result.stdout)
        # Unknown arguments must be rejected before any real build/output mutation.
        result = subprocess.run([*args, "--unknown-option"], cwd=ROOT, env=env, capture_output=True, timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(out.exists())
    def installer(self):
        self.assertTrue(callable(getattr(self.d, "install_bundle", None)), "atomic final-artifact installation is not implemented")
        return self.d.install_bundle

    def old_prefix(self):
        prefix = self.case / "user prefix 中文"
        prefix.mkdir()
        binary = prefix / ("mulix.exe" if os.name == "nt" else "mulix")
        binary.write_bytes(b"ORIGINAL-EXECUTABLE")
        (prefix / "user-data.txt").write_bytes(b"independent-user-data")
        return prefix, binary

    def test_bundle_install_needs_no_git_or_go(self):
        self.installer()
        prefix = self.case / "prefix with spaces 中文"
        empty_path = self.case / "empty-path"
        empty_path.mkdir()
        env = dict(os.environ, PATH=str(empty_path), PYTHON=sys.executable)
        result = subprocess.run([shutil.which("bash"), (SCRIPTS / "install.sh").as_posix(), "--bundle", str(self.bundle), "--prefix", str(prefix)], env=env, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr)
        binary = prefix / ("mulix.exe" if os.name == "nt" else "mulix")
        result = subprocess.run([str(binary), "version"], env=env, capture_output=True, text=True, timeout=10)
        self.assertEqual((result.returncode, result.stdout, result.stderr), (0, "mulix p1-unit\n", ""))

    def test_corrupt_bundle_keeps_old_binary_and_data(self):
        install = self.installer()
        prefix, binary = self.old_prefix()
        path, manifest = self.damaged()
        (path / manifest["artifacts"][0]["archive"]).write_bytes(b"damaged")
        with self.assertRaises(self.d.GateError):
            install(path, prefix)
        self.assertEqual(binary.read_bytes(), b"ORIGINAL-EXECUTABLE")
        self.assertEqual((prefix / "user-data.txt").read_bytes(), b"independent-user-data")
        self.assertEqual(len(list(prefix.iterdir())), 2)

    def test_staged_actual_version_must_match_manifest(self):
        install = self.installer()
        prefix, binary = self.old_prefix()
        path, manifest = self.damaged()
        manifest["version"] = "p1-relabelled"
        for row in manifest["artifacts"]:
            renamed = row["archive"].replace("p1-unit", "p1-relabelled")
            (path / row["archive"]).rename(path / renamed)
            row["archive"] = renamed
        self.rewrite(path, manifest)
        self.d.verify_bundle(path)  # Complete internally consistent hashes cannot substitute for execution.
        with self.assertRaisesRegex(self.d.GateError, "version"):
            install(path, prefix)
        self.assertEqual(binary.read_bytes(), b"ORIGINAL-EXECUTABLE")
        self.assertEqual(len(list(prefix.iterdir())), 2)

    def test_replace_failure_keeps_original_and_cleans_own_temp(self):
        install = self.installer()
        prefix, binary = self.old_prefix()
        with mock.patch.object(self.d.os, "replace", side_effect=PermissionError("replacement refused")):
            with self.assertRaises((self.d.GateError, OSError)):
                install(self.bundle, prefix)
        self.assertEqual(binary.read_bytes(), b"ORIGINAL-EXECUTABLE")
        self.assertEqual((prefix / "user-data.txt").read_bytes(), b"independent-user-data")
        self.assertEqual(len(list(prefix.iterdir())), 2)

    def test_interrupted_staged_install_preserves_old_and_retries(self):
        install = self.installer()
        prefix, binary = self.old_prefix()
        marker = self.case / "staged-marker"
        # Pause one external syscall in a test-owned worker, after real verified staging.
        # No production hook, fake bundle, child Go process or shared prefix is involved.
        code = '''import sys, time
from pathlib import Path
sys.path.insert(0, sys.argv[1])
import delivery as d
real = d.command
def pause(argv, cwd, env=None):
    if argv[-1] == "version":
        Path(sys.argv[4]).write_text("staged")
        while True: time.sleep(0.01)
    return real(argv, cwd, env)
d.command = pause
d.install_bundle(Path(sys.argv[2]), Path(sys.argv[3]))
'''
        with (self.case / "worker.stdout").open("wb") as out, (self.case / "worker.stderr").open("wb") as err:
            proc = subprocess.Popen([sys.executable, "-I", "-B", "-c", code, str(SCRIPTS), str(self.bundle), str(prefix), str(marker)], stdout=out, stderr=err)
            try:
                deadline = time.monotonic() + 30
                while not marker.exists() and proc.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(marker.exists(), "worker never reached verified staging")
            finally:
                if proc.poll() is None:
                    proc.kill()
                proc.wait(timeout=10)
        self.assertEqual(binary.read_bytes(), b"ORIGINAL-EXECUTABLE")
        orphaned = set(prefix.glob(".mulix-install-*"))
        self.assertTrue(orphaned, "fixture must exercise interruption after staging")
        installed = install(self.bundle, prefix)
        self.assertEqual(subprocess.check_output([str(installed), "version"], text=True), "mulix p1-unit\n")
        self.assertEqual((prefix / "user-data.txt").read_bytes(), b"independent-user-data")
        self.assertTrue(orphaned.issubset(set(prefix.iterdir())), "retry must not sweep unowned prior staging files")

    def test_source_install_remains_available_with_explicit_version(self):
        prefix = self.case / "source-prefix"
        result = subprocess.run(["bash", (SCRIPTS / "install.sh").as_posix(), "--source", str(ROOT), "--prefix", str(prefix), "--version", "p1-source-unit"], capture_output=True, text=True, timeout=120)
        self.assertEqual(result.returncode, 0, result.stderr)
        binary = prefix / ("mulix.exe" if os.name == "nt" else "mulix")
        self.assertEqual(subprocess.check_output([str(binary), "version"], text=True), "mulix p1-source-unit\n")


if __name__ == "__main__":
    unittest.main()
