"""Fail-closed repository evidence checks; no subprocess execution is attested here."""
import hashlib
import json
from pathlib import Path
import re
import stat
import subprocess
import os


def require_inventoried_go_inputs(root, names):
    """Go ignores no Git patterns: discover actual inputs for every built target."""
    fields = ("GoFiles", "CgoFiles", "CFiles", "CXXFiles", "MFiles", "HFiles", "FFiles", "SFiles", "SwigFiles", "SwigCXXFiles", "SysoFiles", "EmbedFiles", "TestGoFiles", "XTestGoFiles", "TestEmbedFiles", "XTestEmbedFiles")
    decoder = json.JSONDecoder()
    for module in sorted({(root / name).parent for name in names if name == "go.mod" or name.endswith("/go.mod")}):
        for system in ("linux", "darwin", "windows"):
            for arch in ("amd64", "arm64"):
                env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0", GOENV="off", GOFLAGS="", GOWORK="off", GOTOOLCHAIN="local", GOEXPERIMENT="", GOAMD64="v1", GOARM64="v8.0")
                text = subprocess.check_output(["go", "list", "-e", "-mod=readonly", "-json", "./..."], cwd=module, env=env, timeout=120).decode("utf-8")
                while text.strip():
                    row, end = decoder.raw_decode(text.lstrip())
                    text = text.lstrip()[end:]
                    directory = Path(row.get("Dir", module))
                    if not directory.is_relative_to(root):
                        continue
                    require(not row.get("Error"), "cannot inventory Go inputs: " + str(row.get("Error")))
                    for field in fields:
                        for name in row.get(field, []):
                            path = (directory / name).relative_to(root).as_posix()
                            require(path in names, "ignored Go build input must be tracked or unignored: " + path)


class GateError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise GateError(message)


def strict_json(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "duplicate JSON key: " + key)
            result[key] = value
        return result
    def constant(value):
        raise GateError("non-finite JSON number: " + value)
    try:
        return json.loads(data, object_pairs_hook=unique, parse_constant=constant)
    except (UnicodeError, json.JSONDecodeError) as error:
        raise GateError("invalid JSON: " + str(error)) from error


def source_snapshot(root, *, transient_files=()):
    """Inventory source; only callers' untracked, non-build-input staging may be omitted."""
    root = Path(root).resolve()
    def git(*args):
        return subprocess.check_output(["git", "-C", str(root), *args])
    require(Path(git("rev-parse", "--show-toplevel").decode().strip()).resolve() == root, "source must be repository root")
    commit = git("rev-parse", "HEAD").decode().strip()
    for row in git("ls-files", "-v", "-z").split(b"\0"):
        require(not row or (row[:1].isupper() and row[:1] != b"S"), "unsupported hidden index flag (assume-unchanged/skip-worktree)")
    names = {raw.decode("utf-8") for raw in git("ls-files", "--cached", "--others", "--exclude-standard", "-z").split(b"\0") if raw}
    excluded = set()
    for transient in transient_files:
        transient = Path(transient)
        require(not transient.is_symlink() and transient.is_file(), "transient exclusion must be an ordinary file")
        transient = transient.resolve()
        if transient.is_relative_to(root):
            excluded.add(transient.relative_to(root).as_posix())
    if excluded:
        tracked = {raw.decode("utf-8") for raw in git("ls-files", "--cached", "-z").split(b"\0") if raw}
        require(not excluded & tracked, "tracked source cannot be excluded as transient staging")
        names.difference_update(excluded)
    # Even an owned staging file is not exempt if Go actually consumes it.
    require_inventoried_go_inputs(root, names)
    files = []
    for name in sorted(names):
        path = root / name
        if not path.exists() and not path.is_symlink():
            files.append([name, "deleted", None, None])
            continue
        info = path.lstat()
        if stat.S_ISLNK(info.st_mode):
            data, kind = str(path.readlink()).encode(), "symlink"
        else:
            require(stat.S_ISREG(info.st_mode), "unsupported source type: " + name)
            data, kind = path.read_bytes(), "file"
        files.append([name, kind, stat.S_IMODE(info.st_mode), hashlib.sha256(data).hexdigest()])
    snapshot = {"commit": commit, "branch": git("branch", "--show-current").decode().strip(), "index": hashlib.sha256(git("ls-files", "--stage", "-z")).hexdigest(), "files": files}
    fingerprint = hashlib.sha256(json.dumps(snapshot, sort_keys=True, ensure_ascii=True).encode()).hexdigest()
    return dict(snapshot, fingerprint=fingerprint, dirty=bool(git("status", "--porcelain", "--untracked-files=all")))


def evaluate_go(data, packages, target, policy, required=None):
    require(bool(packages), "empty package inventory")
    tests, running, output, started, terminal = {}, set(), {}, set(), {}
    for line in data.split("\n"):
        if not line.strip():
            continue
        event = strict_json(line)
        require(isinstance(event, dict), "Go event must be object")
        action, package, test = event.get("Action"), event.get("Package"), event.get("Test")
        require(action in ("start", "run", "pause", "cont", "output", "pass", "fail", "skip", "bench", "build-output", "build-fail"), "unknown Go event")
        require(action not in ("fail", "build-fail"), "Go test/package/build failure")
        if action == "build-output":
            continue
        require(package in packages, "unexpected/missing Go package: " + str(package))
        if action == "start":
            require(package not in started, "duplicate package start")
            started.add(package)
            continue
        require(package in started and package not in terminal, "event outside running package")
        if action == "output":
            text = event.get("Output")
            require(isinstance(text, str), "missing Go output")
            require(test or not re.search(r"(?m)^ok\s+\S+.*\(cached\)", text), "cached Go run")
            if test:
                output[package + "#" + test] = output.get(package + "#" + test, "") + text
            continue
        if not test:
            require(action in ("pass", "skip"), "invalid package terminal")
            require(not packages[package] or action == "pass", "test package skipped")
            terminal[package] = action
            continue
        require(isinstance(test, str) and bool(test), "missing test identity")
        key = package + "#" + test
        if action == "run":
            require(key not in running and key not in tests, "duplicate test start")
            running.add(key)
        elif action in ("pass", "skip"):
            require(key in running and key not in tests, "duplicate/unstarted test terminal")
            tests[key] = action
            running.remove(key)
        else:
            require(key in running and action in ("pause", "cont", "bench"), "event outside running test")
    require(set(packages) == started == set(terminal), "missing/incomplete package execution")
    require(not running and bool(tests) and any(value == "pass" for value in tests.values()), "zero/all-skipped/incomplete test execution")
    for package, has_tests in packages.items():
        require(not has_tests or any(key.startswith(package + "#") for key in tests), "package did not execute tests: " + package)
    skipped = []
    for key, status in tests.items():
        if status != "skip":
            continue
        reason = output.get(key, "")
        allowed = any(row["test"] == key and target.split("_")[0] in row["os"] and row["reason"] in reason for row in policy.get("allowed_skips", []))
        require(allowed, "unapproved required-test skip: " + key + ": " + reason)
        skipped.append({"test": key, "reason": reason.strip()})
    if required is None:
        groups = policy.get("required_tests", {})
        required = groups.get("all", []) + groups.get("windows" if target.startswith("windows_") else "nonwindows", [])
    for key in required:
        require(key in tests, "required test not executed: " + key)
        # Every skipped result has already matched an exact platform/reason rule.
    return {"pass": sum(v == "pass" for v in tests.values()), "skip": len(skipped), "fail": 0, "tests": tests, "skipped": skipped}


def coverage(path):
    lines = Path(path).read_text().splitlines()
    require(bool(lines) and lines[0] in ("mode: set", "mode: count", "mode: atomic"), "invalid coverage header")
    blocks = {}
    for line in lines[1:]:
        match = re.fullmatch(r"(.+:\d+\.\d+,\d+\.\d+) (\d+) (\d+)", line)
        require(match is not None, "invalid coverage block")
        key, weight, count = match.groups()
        weight, count = int(weight), int(count)
        if key in blocks:
            require(blocks[key][0] == weight, "inconsistent duplicate coverage block")
            blocks[key][1] += count
        else:
            blocks[key] = [weight, count]
    result = {"covered": sum(n for n, c in blocks.values() if c), "total": sum(n for n, c in blocks.values())}
    require(result["total"] > 0, "empty coverage profile")
    return result


def check_coverage(candidate, base, floor):
    def valid(value):
        return isinstance(value, dict) and all(type(value.get(k)) is int for k in ("covered", "total")) and 0 <= value["covered"] <= value["total"] and value["total"] > 0
    require(valid(candidate) and valid(base), "invalid/empty coverage counts")
    for name, reference in (("base", base), ("floor", floor)):
        if reference is not None:
            require(valid(reference), "invalid coverage reference")
            require(candidate["covered"] * reference["total"] >= reference["covered"] * candidate["total"], "coverage regressed below " + name)


def aggregate(reports, commit, bundle_hash, policy, needs, event):
    require(re.fullmatch(r"[0-9a-f]{40}", commit or "") and re.fullmatch(r"[0-9a-f]{64}", bundle_hash or ""), "invalid expected source/artifact binding")
    for job in ("bundles", "native"):
        require(needs.get(job, {}).get("result") == "success", "required dependency did not succeed: " + job)
    expected_extended = "success" if event in ("schedule", "workflow_dispatch") else "skipped"
    require(needs.get("extended", {}).get("result") == expected_extended, "extended dependency missing or not successful for event")
    latest, seen = {}, set()
    for report in reports:
        require(isinstance(report, dict), "malformed native report")
        target, attempt = report.get("target"), report.get("attempt")
        require(target in policy["targets"] and type(attempt) is int and attempt > 0, "unexpected target/attempt")
        require((target, attempt) not in seen, "duplicate native report")
        seen.add((target, attempt))
        if target not in latest or attempt > latest[target]["attempt"]:
            latest[target] = report
    require(set(latest) == set(policy["targets"]), "incomplete native matrix")
    for target, report in latest.items():
        require(report.get("schema") == "mulix.qualification.v1" and report.get("status") == "PASS", "native qualification not PASS: " + target)
        require(report.get("commit") == commit and report.get("bundle_manifest_sha256") == bundle_hash, "stale source/artifact binding: " + target)
        if "_run_id" in needs:
            require(report.get("run_id") == needs["_run_id"], "evidence belongs to another CI run: " + target)
        require(report.get("ci") is True and report.get("dirty") is False and report.get("source_unchanged") is True, "not clean/stable CI evidence: " + target)
        checks = policy["required_checks"] + (["root-race", "example-race"] if target.startswith("linux_") else [])
        require(all(report.get("checks", {}).get(name) == "PASS" for name in checks), "missing mandatory native checks: " + target)
    return {"schema": "mulix.matrix.v1", "status": "PASS", "commit": commit, "bundle_manifest_sha256": bundle_hash, "targets": sorted(latest), "attempts": {t: r["attempt"] for t, r in latest.items()}}
