#!/usr/bin/env python3
"""Execute real final-bundle lifecycle tests in owned directories; no business sign-off."""
import argparse
import json
from pathlib import Path
import shutil
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
import delivery as d
from quality_policy import GateError, require, strict_json
from quality_run import Runner

ROOT = Path(__file__).resolve().parent.parent


def qualify(bundle, baseline, output, target):
    require(target == d.native_target(), "native target differs from requested qualification target")
    bundle, baseline, output = Path(bundle).resolve(), Path(baseline).resolve(), Path(output).absolute()
    current, previous = d.verify_bundle(bundle), d.verify_bundle(baseline)
    policy = strict_json((ROOT / "scripts/quality-policy.json").read_bytes())
    require(previous["source"]["commit"] == policy["upgrade_revision"] and previous["source"]["dirty"] is False, "historical upgrade baseline is not the pinned clean commit")
    before, baseline_before = d.bundle_snapshot(bundle), d.bundle_snapshot(baseline)
    output.mkdir(parents=True)
    runner = Runner(output / "commands", ROOT)
    report = {"schema": "mulix.delivery-tests.v1", "status": "FAIL", "target": target, "candidate_source": current["source"], "baseline_source": previous["source"], "candidate_version": current["version"], "baseline_version": previous["version"], "cases": {}}
    prefix = output / "private prefix 中文"
    cli = prefix / d.binary_name(target)
    python = sys.executable
    install = [python, "-I", "-B", str(ROOT / "scripts/delivery.py"), "install"]
    try:
        runner.run("install-baseline", [*install, "--bundle", baseline, "--prefix", prefix, "--version", previous["version"]])
        old_hash = d.sha(cli.read_bytes())
        version = runner.run("baseline-version", [cli, "version"])
        require(version.stdout.strip() == "mulix " + previous["version"], "historical installed version mismatch")
        project, fresh = output / "old project", output / "fresh project"
        runner.run("init-old-project", [cli, "init", "--dir", project, "--host", "dsh"])
        relative = ".mulix/templates/spec-template.md"
        local, base = project / relative, project / ".mulix/.installed" / relative
        old_template = local.read_bytes()
        require(base.read_bytes() == old_template, "old initialization did not record authentic baseline")
        user = project / "user-data.txt"
        user.write_bytes(b"DELIVER-02 user data must survive\n")
        customized = b"# User-owned spec customization\nUSER_SPEC_P1\n"
        local.write_bytes(customized)
        broken = output / "damaged-bundle-fixture"
        broken.mkdir()
        # Copy only the verified flat inventory, never arbitrary trees/links.
        for name in before:
            (broken / name).write_bytes(d.regular_bytes(bundle / name, d.MAX_ARCHIVE))
        native_row = next(r for r in current["artifacts"] if r["target"] == target)
        (broken / native_row["archive"]).write_bytes(b"INTERRUPTED DOWNLOAD FIXTURE")
        runner.run("reject-damaged-install", [*install, "--bundle", broken, "--prefix", prefix], expected="nonzero")
        require(d.sha(cli.read_bytes()) == old_hash and local.read_bytes() == customized and user.read_bytes() == b"DELIVER-02 user data must survive\n", "failed install changed old executable/project/data")
        runner.run("install-candidate", [*install, "--bundle", bundle, "--prefix", prefix, "--version", current["version"]])
        require(d.sha(cli.read_bytes()) == native_row["binary_sha256"], "installed binary differs from final artifact")
        version = runner.run("candidate-version", [cli, "version"])
        require(version.stdout.strip() == "mulix " + current["version"], "final installed version mismatch")
        runner.run("init-current-project", [cli, "init", "--dir", fresh, "--host", "dsh"])
        incoming = (fresh / relative).read_bytes()
        require(incoming != old_template, "upgrade fixture has no actual old/current upstream difference")
        conflict = runner.run("upgrade-conflict", [cli, "update"], cwd=project, expected="nonzero")
        require("conflicted" in conflict.stdout + conflict.stderr, "upgrade failed for a reason other than expected real conflict")
        require(local.read_bytes() == customized and base.read_bytes() == old_template, "conflict overwrote local or authentic old baseline")
        state = strict_json((project / ".mulix/install-state.json").read_bytes())
        pending = state.get("pending", {}).get(relative)
        require(isinstance(pending, dict), "real conflict did not produce pending recovery metadata")
        candidate = project / pending["candidate"]
        require(b"<<<<<<< incoming (mulix)" in candidate.read_bytes() and (candidate.parent / "incoming.md").read_bytes() == incoming, "missing conflict sidecar/incoming recovery material")
        runner.run("reject-invalid-accept", [cli, "update", "--accept", "../user-data.txt"], cwd=project, expected="nonzero")
        require(local.read_bytes() == customized and base.read_bytes() == old_template, "invalid accept changed selected local/baseline")
        # A fixture's explicit resolution, not authenticated human consent.
        resolved = incoming + b"\nUSER_SPEC_P1\n"
        local.write_bytes(resolved)
        runner.run("accept-resolution", [cli, "update", "--accept", relative], cwd=project)
        runner.run("retry-upgrade", [cli, "update"], cwd=project)
        require(local.read_bytes() == resolved and base.read_bytes() == incoming and user.read_bytes() == b"DELIVER-02 user data must survive\n", "accepted upgrade/retry lost resolution, baseline or user data")
        require(not strict_json((project / ".mulix/install-state.json").read_bytes()).get("pending"), "upgrade still has unresolved pending entries")
        require(not (project / ".claude").exists(), "DSH-only historical upgrade installed an unintended host")
        report["cases"]["DELIVER-02"] = {"status": "PASS", "old_binary_sha256": old_hash, "candidate_binary_sha256": d.sha(cli.read_bytes()), "actual_historical_revision": previous["source"]["commit"], "user_data_preserved": True, "resolution": "explicit test fixture; not human sign-off"}
        for name in ("e2e", "sessions", "update"):
            # Forward-slash relative script path is portable to native Git Bash.
            runner.run("smoke-" + name, ["bash", "scripts/smoke-" + name + ".sh", cli.as_posix()], timeout=600)
        require(d.sha(cli.read_bytes()) == native_row["binary_sha256"], "qualification replaced the final installed artifact")
        require(before == d.bundle_snapshot(bundle) and baseline_before == d.bundle_snapshot(baseline), "input bundle changed during qualification")
        report["cases"]["DELIVER-01"] = {"status": "PASS", "binary_sha256": native_row["binary_sha256"], "smokes": ["e2e", "sessions", "update"], "rebuilt_candidate": False}
        report["status"] = "PASS"
    except Exception as error:
        report["error"] = str(error)
        raise
    finally:
        report["commands"] = runner.records
        (output / "delivery-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("--baseline-bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--target", required=True)
    args = parser.parse_args()
    try:
        result = qualify(args.bundle, args.baseline_bundle, args.output, args.target)
    except (GateError, OSError, ValueError) as error:
        print("DELIVERY_TESTS_BLOCKED: " + str(error), file=sys.stderr)
        return 1
    print(json.dumps({"status": result["status"], "target": result["target"], "cases": result["cases"]}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
