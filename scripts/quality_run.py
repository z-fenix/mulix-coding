"""Durable explicit-argv execution shared by native and delivery test drivers."""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parent))
from quality_policy import GateError, require
from quality_process import run as run_process


def reject_race_logs(directory):
    directory = Path(directory)
    require(directory.is_dir(), "race diagnostic directory is missing")
    files = [str(p) for p in directory.rglob("*") if p.is_file() or p.is_symlink()]
    require(not files, "subprocess race diagnostics found: " + repr(files))


class Runner:
    def __init__(self, output, cwd, env=None):
        self.output, self.cwd = Path(output).absolute(), Path(cwd).resolve()
        self.output.mkdir(parents=True)  # Existing evidence is never overwritten.
        self.env = dict(os.environ) if env is None else dict(env)
        self.records = []

    def run(self, name, argv, cwd=None, env=None, timeout=600, expected=0):
        require(re.fullmatch(r"[A-Za-z0-9_-]+", name), "invalid command label")
        require(not any(r["name"] == name for r in self.records), "duplicate command label")
        argv = [str(a) for a in argv]
        stdout, stderr = self.output / (name + ".stdout.log"), self.output / (name + ".stderr.log")
        record = {"name": name, "argv": argv, "cwd": str(cwd or self.cwd), "started_at": datetime.now(timezone.utc).isoformat(), "expected_exit": expected, "exit_code": None, "timed_out": False, "status": "FAIL", "stdout": stdout.name, "stderr": stderr.name}
        start = time.monotonic()
        with stdout.open("xb") as out, stderr.open("xb") as err:
            try:
                try:
                    result = run_process(argv, cwd=cwd or self.cwd, env=self.env if env is None else env, stdout=out, stderr=err, timeout=timeout)
                    record["exit_code"] = result.returncode
                except subprocess.TimeoutExpired as error:
                    record["timed_out"] = True
                    record["exit_code"] = getattr(error, "returncode", None)
            except (OSError, RuntimeError, ValueError) as error:
                record["error"] = str(error)
        record["seconds"] = time.monotonic() - start
        code = record["exit_code"]
        ok = code is not None and not record["timed_out"] and (code != 0 if expected == "nonzero" else code == expected)
        record["status"] = "PASS" if ok else "FAIL"
        self.records.append(record)
        (self.output / "commands.json").write_text(json.dumps(self.records, indent=2) + "\n", encoding="utf-8")
        text_out, text_err = stdout.read_text(encoding="utf-8", errors="replace"), stderr.read_text(encoding="utf-8", errors="replace")
        require(ok, "command failed/timed out: " + name + " exit=" + str(code) + "\n" + text_out[-1500:] + text_err[-1500:])
        return subprocess.CompletedProcess(argv, code, text_out, text_err)
