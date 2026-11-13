"""Bounded real process trees must not execute descendants after timeout."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]


def load(name):
    spec = importlib.util.spec_from_file_location(name + "_tree_test", SCRIPTS / (name + ".py"))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


class ProcessTreeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mulix-process-tree-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.ready, self.leaked = self.root / "ready", self.root / "leaked"
        self.leaf = "from pathlib import Path; import os, time; Path(" + repr(str(self.ready)) + ").write_text(str(os.getpid())); time.sleep(2); Path(" + repr(str(self.leaked)) + ").write_text('executed after timeout')"

    def assert_descendant_stopped(self):
        self.assertTrue(self.ready.exists(), "fixture never reached a running descendant")
        deadline = time.monotonic() + 2.3
        while not self.leaked.exists() and time.monotonic() < deadline:
            time.sleep(0.01)
        self.assertFalse(self.leaked.exists(), "descendant executed after its owning command timed out")

    def test_windows_completed_process_is_not_an_active_kill_target(self):
        import ctypes
        module = load("quality_process")
        kernel = mock.Mock()
        kernel.OpenProcess.return_value = 1
        def exited(handle, status):
            status._obj.value = 0
            return True
        kernel.GetExitCodeProcess.side_effect = exited
        kernel.GetProcessTimes.return_value = True
        with mock.patch.object(sys, "platform", "win32"), mock.patch.object(os, "name", "nt"), mock.patch.object(ctypes, "WinDLL", return_value=kernel, create=True):
            self.assertIsNone(module.identity(42))
        kernel.CloseHandle.assert_called_once_with(1)

    def test_outer_runner_timeout_cancels_nested_runner_session(self):
        module = load("quality_run")
        inner = "import sys; from pathlib import Path; sys.path.insert(0, " + repr(str(SCRIPTS)) + "); from quality_run import Runner; Runner(Path(" + repr(str(self.root / "inner")) + "), Path(" + repr(str(self.root)) + ")).run('leaf', [sys.executable, '-I', '-B', '-c', " + repr(self.leaf) + "], timeout=30)"
        runner = module.Runner(self.root / "outer", self.root)
        with self.assertRaises(module.GateError):
            runner.run("nested", [sys.executable, "-I", "-B", "-c", inner], timeout=1)
        self.assert_descendant_stopped()
        records = json.loads((self.root / "outer/commands.json").read_text())
        self.assertTrue(records[0]["timed_out"])

    def test_standalone_delivery_timeout_cancels_direct_descendants(self):
        module = load("delivery")
        parent = "import subprocess, sys, time; subprocess.Popen([sys.executable, '-I', '-B', '-c', " + repr(self.leaf) + "]); time.sleep(30)"
        # Shorten only the real external deadline, for both old subprocess.run
        # and the managed replacement; no fake exit or child behavior.
        communicate = subprocess.Popen.communicate
        def short_deadline(proc, *args, **kwargs):
            if kwargs.get("timeout") == 600:
                kwargs["timeout"] = 1
            return communicate(proc, *args, **kwargs)
        with mock.patch.object(subprocess.Popen, "communicate", short_deadline):
            with self.assertRaises(subprocess.TimeoutExpired):
                module.command([sys.executable, "-I", "-B", "-c", parent], self.root)
        self.assert_descendant_stopped()


if __name__ == "__main__":
    unittest.main()
