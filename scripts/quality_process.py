"""Owned command-tree cancellation, including nested runners in new sessions.

A private registry is inherited only by this invocation's children. Each worker
registers before exec, so cancellation cannot miss a not-yet-registered command.
This contains cooperative framework subprocesses, not deliberately escaping code.
"""
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import uuid

ROOT_KEY = "_MULIX_PROCESS_TREE_ROOT"
NODE_KEY = "_MULIX_PROCESS_TREE_NODE"
TOKEN_KEY = "_MULIX_PROCESS_TREE_TOKEN"


def identity(pid):
    if sys.platform.startswith("linux"):
        try:
            return Path("/proc", str(pid), "stat").read_text().rsplit(")", 1)[1].split()[19]
        except (FileNotFoundError, ProcessLookupError):
            return None
    if os.name == "nt":
        import ctypes
        from ctypes import wintypes
        kernel = ctypes.WinDLL("kernel32", use_last_error=True)
        kernel.OpenProcess.argtypes = (wintypes.DWORD, wintypes.BOOL, wintypes.DWORD)
        kernel.OpenProcess.restype = wintypes.HANDLE
        kernel.GetProcessTimes.argtypes = (wintypes.HANDLE,) + (ctypes.POINTER(wintypes.FILETIME),) * 4
        kernel.GetProcessTimes.restype = wintypes.BOOL
        kernel.GetExitCodeProcess.argtypes = (wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD))
        kernel.GetExitCodeProcess.restype = wintypes.BOOL
        kernel.CloseHandle.argtypes = (wintypes.HANDLE,)
        handle = kernel.OpenProcess(0x1000, False, pid)
        if not handle:
            return None
        try:
            status = wintypes.DWORD()
            if not kernel.GetExitCodeProcess(handle, ctypes.byref(status)):
                raise ctypes.WinError(ctypes.get_last_error())
            if status.value != 259:  # STILL_ACTIVE
                return None
            times = [wintypes.FILETIME() for _ in range(4)]
            if not kernel.GetProcessTimes(handle, *(ctypes.byref(t) for t in times)):
                raise ctypes.WinError(ctypes.get_last_error())
            return str((times[0].dwHighDateTime << 32) | times[0].dwLowDateTime)
        finally:
            kernel.CloseHandle(handle)
    result = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "lstart="], capture_output=True, text=True, env=dict(os.environ, LC_ALL="C"), timeout=10)
    return result.stdout.strip() or None


def cancelled(node, root):
    while True:
        if not node.is_dir() or (node / "cancelled").exists():
            return True
        if node == root:
            return False
        node = node.parent


def cancel_tree(node):
    try:
        (node / "cancelled").write_bytes(b"cancelled\n")
    except FileNotFoundError:
        return  # An owning ancestor already cancelled and removed this branch.
    for path in sorted(node.rglob("pid.json"), key=lambda p: len(p.parts), reverse=True):
        try:
            record = json.loads(path.read_text(encoding="utf-8"))
        except FileNotFoundError:
            continue
        pid = record["pid"]
        if type(pid) is not int or pid <= 0 or pid == os.getpid():
            raise RuntimeError("invalid owned process record")
        current = identity(pid)
        if current is not None and current != record["identity"]:
            continue  # The PID has been reused; it is no longer ours.
        if os.name == "nt":
            if current is not None:
                taskkill = str(Path(os.environ.get("SystemRoot", r"C:\Windows")) / "System32/taskkill.exe")
                result = subprocess.run([taskkill, "/PID", str(pid), "/T", "/F"], capture_output=True, timeout=10)
                if result.returncode and identity(pid) is not None:
                    raise RuntimeError("could not cancel owned Windows process tree")
        else:
            try:
                os.killpg(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass


def run(argv, *, cwd=None, env=None, stdout=None, stderr=None, capture_output=False, text=False, timeout=600):
    argv = [str(a) for a in argv]
    env = dict(os.environ if env is None else env)
    temporary = None
    if ROOT_KEY in env or NODE_KEY in env:
        root, parent = Path(env[ROOT_KEY]), Path(env[NODE_KEY])
        if not parent.is_relative_to(root) or json.loads((root / "owner.json").read_text(encoding="utf-8"))["token"] != env[TOKEN_KEY] or cancelled(parent, root):
            raise RuntimeError("invalid/cancelled inherited process ownership")
    else:
        temporary = tempfile.TemporaryDirectory(prefix="mulix-process-tree-")
        root = parent = Path(temporary.name)
        env[TOKEN_KEY] = uuid.uuid4().hex
        (root / "owner.json").write_text(json.dumps({"token": env[TOKEN_KEY]}), encoding="utf-8")
    node = Path(tempfile.mkdtemp(prefix="command-", dir=parent))
    env.update({ROOT_KEY: str(root), NODE_KEY: str(node)})
    if capture_output:
        if stdout is not None or stderr is not None:
            raise ValueError("capture_output conflicts with redirected logs")
        stdout = stderr = subprocess.PIPE
    proc = None
    try:
        proc = subprocess.Popen([sys.executable, "-I", "-B", str(Path(__file__).resolve()), "worker", str(node), *argv], cwd=cwd, env=env, stdout=stdout, stderr=stderr, start_new_session=os.name != "nt", text=text, encoding="utf-8" if text else None, errors="replace" if text else None)
        try:
            if capture_output:
                out, err = proc.communicate(timeout=timeout)
            else:
                proc.wait(timeout=timeout)
                out = err = None
        except subprocess.TimeoutExpired as error:
            cancel_tree(node)
            proc.communicate(timeout=10)
            error.returncode = proc.returncode
            raise
        return subprocess.CompletedProcess(argv, proc.returncode, out, err)
    finally:
        try:
            cancel_tree(node)
            if proc is not None:
                proc.wait(timeout=10)
        finally:
            if temporary is not None:
                temporary.cleanup()
            else:
                shutil.rmtree(node, ignore_errors=True)


def worker(node, argv):
    root = Path(os.environ[ROOT_KEY])
    if not node.is_relative_to(root) or cancelled(node, root):
        return 125
    pid = os.getpid()
    if os.name != "nt" and os.getpgrp() != pid:
        raise RuntimeError("worker does not own its process group")
    pending = node / "pid.pending"
    pending.write_text(json.dumps({"pid": pid, "identity": identity(pid)}), encoding="utf-8")
    pending.replace(node / "pid.json")
    if cancelled(node, root):
        return 125
    if os.name != "nt":
        os.execvpe(argv[0], argv, os.environ)
    # Windows exec emulation can change PID: keep this registered parent alive
    # so taskkill /T reaches the actual command and all of its descendants.
    return subprocess.call(argv)


if __name__ == "__main__":
    if len(sys.argv) < 4 or sys.argv[1] != "worker":
        raise SystemExit("internal command worker only")
    raise SystemExit(worker(Path(sys.argv[2]), sys.argv[3:]))
