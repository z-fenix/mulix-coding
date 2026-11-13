#!/usr/bin/env python3
"""Run required native checks, final-artifact tests and a fail-closed CI aggregate."""
import argparse
from contextlib import ExitStack
import json
import os
from pathlib import Path
import re
import shutil
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import delivery as d
from quality_policy import GateError, aggregate, check_coverage, coverage, evaluate_go, require, source_snapshot, strict_json
from quality_run import Runner, reject_race_logs

ROOT = Path(__file__).resolve().parent.parent
MANDATORY = ("tools", "root-full", "example-full", "root-vet", "example-vet", "python", "coverage", "shell", "parity", "diff", "delivery", "source-stable")


def write_json(path, value):
    Path(path).write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def validate_policy(policy):
    require(isinstance(policy, dict) and policy.get("schema") == "mulix.repository-quality.v1", "invalid repository policy")
    require(isinstance(policy.get("targets"), list) and len(policy["targets"]) == len(d.TARGETS) and set(policy["targets"]) == set(d.TARGETS), "policy must include all six native targets")
    checks = policy.get("required_checks")
    require(isinstance(checks, list) and len(checks) == len(MANDATORY) and set(checks) == set(MANDATORY), "policy must retain every mandatory check")
    require(all(isinstance(policy.get("tools", {}).get(tool), str) and re.fullmatch(r"\d+\.\d+\.\d+", policy["tools"][tool]) for tool in ("go", "python", "node")), "policy tool versions must be exact")
    require(bool(policy.get("required_tests", {}).get("all")), "empty required test inventory")
    for key in ("upgrade_revision", "default_base_revision"):
        require(re.fullmatch(r"[0-9a-f]{40}", policy.get(key, "")), "policy requires full historical commit")
    require(type(policy.get("extended_iterations", 5)) is int and 1 <= policy.get("extended_iterations", 5) <= 20, "invalid stress iteration budget")
    require(type(policy.get("fuzz_seconds", 20)) is int and 10 <= policy.get("fuzz_seconds", 20) <= 300, "invalid fuzz budget")
    return policy


def load_policy():
    return validate_policy(strict_json((ROOT / "scripts/quality-policy.json").read_bytes()))


def require_tool(name):
    result = shutil.which(name)
    require(result is not None, "required tool unavailable: " + name)
    return result


def validate_context(source, manifest, target, ci, env, observed_target):
    require(target in d.TARGETS and target == observed_target, "actual native platform differs from declared target")
    require(source["commit"] == manifest["source"]["commit"], "candidate belongs to another source commit")
    if ci:
        require(env.get("GITHUB_ACTIONS") == "true" and env.get("GITHUB_SHA") == source["commit"] and re.fullmatch(r"[0-9]+", env.get("GITHUB_RUN_ID", "")) and re.fullmatch(r"[1-9][0-9]*", env.get("GITHUB_RUN_ATTEMPT", "")), "missing/mismatched CI run context")
        require(source["dirty"] is False and manifest["source"]["dirty"] is False, "dirty source/artifact cannot qualify CI")
    elif source["dirty"] or manifest["source"]["dirty"]:
        require(source["dirty"] == manifest["source"]["dirty"] and source["fingerprint"] == manifest["source"]["fingerprint"], "dirty local candidate is stale; rebuild after source changes")


def check_stability(before, after, bundle_before, bundle_after):
    require(before["fingerprint"] == after["fingerprint"] and before["dirty"] == after["dirty"], "source/index/head/branch changed during qualification")
    require(bundle_before == bundle_after, "bundle changed during qualification")


def native_env(target):
    os_name, arch = target.split("_")
    return dict(os.environ, GOOS=os_name, GOARCH=arch, GOFLAGS="", GOENV="off", GOWORK="off", GOTOOLCHAIN="local", CGO_ENABLED="1" if os_name == "linux" else "0", PYTHON=sys.executable)


def tools(runner, policy, target):
    for tool in ("go", "git", "bash", "node"):
        require_tool(tool)
    versions = {}
    for name, argv in (("go", ["go", "version"]), ("python", [sys.executable, "-I", "-B", "--version"]), ("node", ["node", "--version"]), ("git", ["git", "--version"]), ("bash", ["bash", "--version"])):
        result = runner.run("tool-" + name, argv)
        versions[name] = (result.stdout or result.stderr).strip().splitlines()[0]
    require(versions["go"].split()[2] == "go" + policy["tools"]["go"], "Go version differs from policy")
    require(versions["python"] == "Python " + policy["tools"]["python"], "Python version differs from policy")
    require(versions["node"] == "v" + policy["tools"]["node"], "Node version differs from policy")
    host = runner.run("go-native-host", ["go", "env", "GOHOSTOS", "GOHOSTARCH"]).stdout.splitlines()
    require("_".join(host) == target, "Go toolchain is not native to declared target")
    if target.startswith("linux_"):
        cc = runner.run("go-cc", ["go", "env", "CC"]).stdout.strip().split()[0]
        require_tool(cc)
    with tempfile.TemporaryDirectory(prefix="symlink-probe-", dir=runner.output.parent) as temporary:
        path = Path(temporary)
        (path / "target").write_bytes(b"capability")
        try:
            (path / "link").symlink_to("target")
            require((path / "link").read_bytes() == b"capability", "native symlink capability is not functional")
        except OSError as error:
            raise GateError("native symlink capability required; platform tests cannot silently skip: " + str(error)) from error
    return versions


def packages(runner, label, cwd):
    rows = runner.run(label + "-packages", ["go", "list", "-f", "{{.ImportPath}} {{len .TestGoFiles}} {{len .XTestGoFiles}}", "./..."], cwd=cwd).stdout.splitlines()
    result = {}
    for line in rows:
        package, local, external = line.split()
        require(package not in result, "duplicate package inventory")
        result[package] = bool(int(local) + int(external))
    return result


def go_tests(runner, label, cwd, target, policy, profile=None, race=False, required=None, env=None):
    inventory = packages(runner, label, cwd)
    argv = ["go", "test", "-count=1", "-json"]
    if profile is not None:
        argv += ["-coverprofile=" + str(profile)]
        if Path(cwd).name == "todo-cli":
            argv += ["-coverpkg=./..."]
    if race:
        argv += ["-race"]
    result = runner.run(label, argv + ["./..."], cwd=cwd, env=env)
    return evaluate_go(result.stdout, inventory, target, policy, required=required)


def clone_at(runner, label, revision, path):
    require(re.fullmatch(r"[0-9a-f]{40}", revision) and revision != "0" * 40, "base must be full nonzero commit SHA")
    actual = runner.run(label + "-revision", ["git", "rev-parse", "--verify", revision + "^{commit}"]).stdout.strip()
    require(actual == revision, "base is not the exact requested commit")
    runner.run(label + "-clone", ["git", "clone", "--local", "--no-hardlinks", "--no-checkout", ROOT, path])
    runner.run(label + "-line-endings", ["git", "-C", path, "config", "core.autocrlf", "false"])
    runner.run(label + "-checkout", ["git", "-C", path, "checkout", "--detach", revision])


def python_tests(output):
    output = Path(output)
    require(not output.exists(), "Python test report already exists")
    scopes = {}
    for relative in ("scripts/tests", "assets/skills/mulix-verify/tests"):
        path = ROOT / relative
        suite = unittest.TestLoader().discover(str(path), top_level_dir=str(path))
        result = unittest.TextTestRunner(verbosity=2).run(suite)
        scopes[relative] = {"run": result.testsRun, "failures": len(result.failures), "errors": len(result.errors), "skipped": len(result.skipped), "expected_failures": len(result.expectedFailures), "unexpected_successes": len(result.unexpectedSuccesses)}
    stats = {key: sum(row[key] for row in scopes.values()) for key in next(iter(scopes.values()))}
    stats["scopes"] = scopes
    write_json(output, stats)
    require(all(row["run"] > 0 and all(row[key] == 0 for key in ("failures", "errors", "skipped", "expected_failures", "unexpected_successes")) for row in scopes.values()), "Python mandatory scope failed/skipped/expected-failed/empty")
    return stats


def bundles(args, policy):
    output = args.output.absolute()
    output.mkdir(parents=True)
    runner = Runner(output / "commands", ROOT)
    before = source_snapshot(ROOT)
    if args.ci:
        require(not before["dirty"] and os.environ.get("GITHUB_SHA") == before["commit"] and os.environ.get("GITHUB_ACTIONS") == "true", "bundle CI source must be clean/current")
    version = args.version or (os.environ.get("GITHUB_REF_NAME") if os.environ.get("GITHUB_REF_TYPE") == "tag" else "ci-" + before["commit"][:12])
    runner.run("candidate-build", [sys.executable, "-I", "-B", ROOT / "scripts/delivery.py", "build", "--source", ROOT, "--output", output / "candidate", "--version", version])
    with tempfile.TemporaryDirectory(prefix="mulix-historical-source-") as temporary:
        old = Path(temporary) / "source"
        clone_at(runner, "historical", policy["upgrade_revision"], old)
        runner.run("historical-build", [sys.executable, "-I", "-B", ROOT / "scripts/delivery.py", "build", "--source", old, "--output", output / "baseline", "--version", "p1-baseline-" + policy["upgrade_revision"][:7]])
    current = d.verify_bundle(output / "candidate")
    baseline = d.verify_bundle(output / "baseline")
    check_stability(before, source_snapshot(ROOT), current, d.verify_bundle(output / "candidate"))
    result = {"candidate_manifest_sha256": d.sha((output / "candidate/manifest.json").read_bytes()), "commit": before["commit"], "dirty": before["dirty"], "baseline_commit": baseline["source"]["commit"]}
    write_json(output / "bundle-report.json", result)
    if args.ci:
        with Path(os.environ["GITHUB_OUTPUT"]).open("a", encoding="utf-8") as stream:
            stream.write("manifest_hash=" + result["candidate_manifest_sha256"] + "\n")
    return result


def native(args, policy):
    output = args.output.absolute()
    output.mkdir(parents=True)
    target = args.target
    runner = Runner(output / "commands", ROOT, native_env(target))
    report = {"schema": "mulix.qualification.v1", "status": "FAIL", "ci": args.ci, "target": target, "attempt": int(os.environ.get("GITHUB_RUN_ATTEMPT", "1")), "run_id": os.environ.get("GITHUB_RUN_ID", "local"), "checks": {}, "source_unchanged": False}
    race_path = None
    with ExitStack() as stack:
        try:
            before = source_snapshot(ROOT)
            write_json(output / "source-before.json", before)
            manifest = d.verify_bundle(args.bundle)
            bundle_before = d.bundle_snapshot(args.bundle)
            validate_context(before, manifest, target, args.ci, os.environ, d.native_target())
            report.update(commit=before["commit"], dirty=before["dirty"], bundle_manifest_sha256=d.sha((args.bundle / "manifest.json").read_bytes()))
            report["tools"] = tools(runner, policy, target)
            report["checks"]["tools"] = "PASS"
            report["tests"] = {}
            for name, cwd in (("root", ROOT), ("example", ROOT / "examples/todo-cli")):
                profile = output / (name + ".cover")
                required = None if name == "root" else []
                report["tests"][name] = go_tests(runner, name + "-full", cwd, target, policy, profile=profile, required=required)
                report["checks"][name + "-full"] = "PASS"
                runner.run(name + "-vet", ["go", "vet", "./..."], cwd=cwd)
                report["checks"][name + "-vet"] = "PASS"
            if target.startswith("linux_"):
                # POSIX race runtime flags cannot safely encode whitespace in log_path.
                race_path = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="mulix-race-", dir="/tmp")))
                env = dict(runner.env, GORACE="halt_on_error=1 exitcode=66 atexit_sleep_ms=0 log_path=" + str(race_path / "race"))
                for name, cwd in (("root", ROOT), ("example", ROOT / "examples/todo-cli")):
                    report["tests"][name + "-race"] = go_tests(runner, name + "-race", cwd, target, policy, race=True, required=None if name == "root" else [], env=env)
                    reject_race_logs(race_path)
                    report["checks"][name + "-race"] = "PASS"
            runner.run("python", [sys.executable, "-I", "-B", ROOT / "scripts/quality.py", "python-tests", "--output", output / "python-tests.json"])
            report["python"] = strict_json((output / "python-tests.json").read_bytes())
            report["checks"]["python"] = "PASS"
            base_ref = args.base_ref or policy["default_base_revision"]
            report["coverage"] = {"base_commit": base_ref}
            with tempfile.TemporaryDirectory(prefix="mulix-coverage-base-") as temporary:
                base = Path(temporary) / "source"
                clone_at(runner, "coverage-base", base_ref, base)
                for name, cwd in (("root", base), ("example", base / "examples/todo-cli")):
                    base_profile = output / (name + "-base.cover")
                    go_tests(runner, name + "-base", cwd, target, policy, profile=base_profile, required=[])
                    actual, reference = coverage(output / (name + ".cover")), coverage(base_profile)
                    check_coverage(actual, reference, policy.get("coverage_floors", {}).get(target, {}).get(name))
                    report["coverage"][name] = {"candidate": actual, "base": reference}
            report["checks"]["coverage"] = "PASS"
            for path in sorted((ROOT / "scripts").glob("*.sh")):
                runner.run("syntax-" + path.stem, ["bash", "-n", path.relative_to(ROOT).as_posix()])
            for relative in ("executing-plans/scripts/task-done", "subagent-driven-development/progress-format.md"):
                require((ROOT / "scripts/superpowers-overrides" / relative).read_bytes() == (ROOT / "assets/superpowers/skills" / relative).read_bytes(), "embedded override drift: " + relative)
            report["checks"].update(shell="PASS", parity="PASS")
            runner.run("diff", ["git", "diff", "--check", base_ref, "--"])
            report["checks"]["diff"] = "PASS"
            runner.run("delivery", [sys.executable, "-I", "-B", ROOT / "scripts/qualify_delivery.py", "--bundle", args.bundle.absolute(), "--baseline-bundle", args.baseline_bundle.absolute(), "--output", output / "delivery", "--target", target])
            report["delivery"] = strict_json((output / "delivery/delivery-report.json").read_bytes())
            require(report["delivery"]["status"] == "PASS", "delivery report not PASS")
            report["checks"]["delivery"] = "PASS"
            after = source_snapshot(ROOT)
            write_json(output / "source-after.json", after)
            check_stability(before, after, bundle_before, d.bundle_snapshot(args.bundle))
            if race_path is not None:
                reject_race_logs(race_path)
            report["source_unchanged"] = True
            report["checks"]["source-stable"] = "PASS"
            report["status"] = "PASS"
            report["release_certified"] = False  # Only the complete clean native CI matrix can qualify a release candidate.
        except Exception as error:
            report["error"] = str(error)
            raise
        finally:
            if race_path is not None:
                shutil.copytree(race_path, output / "race-diagnostics")
            report["commands"] = runner.records
            write_json(output / "native-report.json", report)
    return report


def fuzz_corpus_snapshot(directory):
    if not directory.exists():
        return {}
    require(directory.is_dir() and not directory.is_symlink(), "invalid fuzz corpus directory")
    return {path.name: d.sha(d.regular_bytes(path, 8 * 1024 * 1024)) for path in sorted(directory.iterdir())}


def preserve_fuzz_reproducers(directory, output, before):
    saved = []
    for name, digest in fuzz_corpus_snapshot(directory).items():
        if before.get(name) == digest:
            continue
        destination = output / "fuzz-reproducers" / directory.name / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("xb") as stream:
            stream.write(d.regular_bytes(directory / name, 8 * 1024 * 1024))
        saved.append(destination.relative_to(output).as_posix())
    return saved


def extended(args, policy):
    target = d.native_target()
    require(target.startswith("linux_"), "extended race/fuzz job requires native Linux")
    output = args.output.absolute()
    output.mkdir(parents=True)
    runner = Runner(output / "commands", ROOT, native_env(target))
    before = source_snapshot(ROOT)
    corpus = ROOT / "internal/quality/testdata/fuzz/FuzzAcceptanceBlock"
    corpus_before = fuzz_corpus_snapshot(corpus)
    report = {"status": "FAIL", "target": target, "iterations": policy.get("extended_iterations", 5), "fuzz_seconds": policy.get("fuzz_seconds", 20)}
    with tempfile.TemporaryDirectory(prefix="mulix-race-", dir="/tmp") as temporary:
        race_path = Path(temporary)
        try:
            tools(runner, policy, target)
            env = dict(runner.env, GORACE="halt_on_error=1 exitcode=66 atexit_sleep_ms=0 log_path=" + str(race_path / "race"))
            expected = ["github.com/z-fenix/mulix-coding/internal/lockfile#TestLock_MutualExclusionAcrossProcesses", "github.com/z-fenix/mulix-coding/internal/lockfile#TestLock_KilledOwnerReleasesAndFileRemainsReusable", "github.com/z-fenix/mulix-coding/internal/cliutil#TestQualityCLIInterruptedOrChangedExecutionNeverLeavesPass", "github.com/z-fenix/mulix-coding/internal/scaffold#TestUpdateReview_SameFilePartialWriteOutcomes"]
            selected = ["./internal/lockfile", "./internal/cliutil", "./internal/scaffold"]
            listing = packages(runner, "stress", ROOT)
            inventory = {k: v for k, v in listing.items() if k in {x.split("#")[0] for x in expected}}
            selector = "^(" + "|".join(x.split("#")[1] for x in expected) + ")$"
            for index in range(report["iterations"]):
                result = runner.run("stress-" + str(index + 1), ["go", "test", "-race", "-count=1", "-json", "-run", selector, *selected], env=env)
                evaluate_go(result.stdout, inventory, target, policy, required=expected)
                reject_race_logs(race_path)
            runner.run("fuzz", ["go", "test", "-count=1", "-run", "^FuzzAcceptanceBlock", "-fuzz", "^FuzzAcceptanceBlock$", "-fuzztime=" + str(report["fuzz_seconds"]) + "s", "-parallel=2", "./internal/quality"], env=env, timeout=report["fuzz_seconds"] + 120)
            reject_race_logs(race_path)
            require(source_snapshot(ROOT)["fingerprint"] == before["fingerprint"], "extended run changed source")
            report["status"] = "PASS"
        except Exception as error:
            report["error"] = str(error)
            raise
        finally:
            try:
                report["fuzz_reproducers"] = preserve_fuzz_reproducers(corpus, output, corpus_before)
            except Exception as error:
                report.update(status="FAIL", evidence_error=str(error))
                raise
            finally:
                shutil.copytree(race_path, output / "race-diagnostics")
                report["commands"] = runner.records
                write_json(output / "extended-report.json", report)
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    build = sub.add_parser("bundles")
    build.add_argument("--output", type=Path, required=True)
    build.add_argument("--version")
    build.add_argument("--ci", action="store_true")
    run = sub.add_parser("native")
    run.add_argument("--output", type=Path, required=True)
    run.add_argument("--bundle", type=Path, required=True)
    run.add_argument("--baseline-bundle", type=Path, required=True)
    run.add_argument("--target", choices=d.TARGETS, required=True)
    run.add_argument("--base-ref")
    run.add_argument("--ci", action="store_true")
    extra = sub.add_parser("extended")
    extra.add_argument("--output", type=Path, required=True)
    py = sub.add_parser("python-tests")
    py.add_argument("--output", type=Path, required=True)
    gate = sub.add_parser("aggregate")
    gate.add_argument("--reports", type=Path, required=True)
    gate.add_argument("--commit", required=True)
    gate.add_argument("--bundle-hash", required=True)
    gate.add_argument("--event", required=True)
    args = parser.parse_args(argv)
    try:
        policy = load_policy()
        if args.command == "native":
            result = native(args, policy)
        elif args.command == "bundles":
            result = bundles(args, policy)
        elif args.command == "extended":
            result = extended(args, policy)
        elif args.command == "python-tests":
            result = python_tests(args.output)
        else:
            needs = strict_json(os.environ.get("QUALITY_NEEDS", "{}"))
            require(os.environ.get("GITHUB_ACTIONS") == "true" and args.commit == os.environ.get("GITHUB_SHA"), "aggregate requires current CI context")
            needs["_run_id"] = os.environ["GITHUB_RUN_ID"]
            reports = [strict_json(p.read_bytes()) for p in args.reports.rglob("native-report.json")]
            result = aggregate(reports, args.commit, args.bundle_hash, policy, needs, args.event)
        print(json.dumps({key: value for key, value in result.items() if key not in ("tests", "commands", "delivery")}, indent=2, sort_keys=True))
    except (GateError, OSError, ValueError, TypeError) as error:
        print("QUALIFICATION_BLOCKED: " + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
