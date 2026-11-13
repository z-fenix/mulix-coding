#!/usr/bin/env python3
"""Build/verify/install mulix bundles. Checksums ensure integrity, not publisher identity."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import struct
import subprocess
import sys
import tarfile
import tempfile
import zipfile

# Only this trusted repository directory is importable, even with Python -I.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from quality_policy import GateError, require, source_snapshot, strict_json
from quality_process import run as run_process

TARGETS = ("linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64", "windows_amd64", "windows_arm64")
MAX_ARCHIVE = 64 * 1024 * 1024
MAX_BINARY = 128 * 1024 * 1024


def sha(data):
    return hashlib.sha256(data).hexdigest()


def valid_version(version):
    require(isinstance(version, str) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,99}", version), "invalid version")
    return version


def native_target():
    system = {"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}.get(platform.system())
    machine = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine().lower())
    target = str(system) + "_" + str(machine)
    require(target in TARGETS, "unsupported native platform: " + target)
    return target


def binary_name(target):
    return "mulix.exe" if target.startswith("windows_") else "mulix"


def archive_name(version, target):
    return "mulix_" + version + "_" + target + (".zip" if target.startswith("windows_") else ".tar.gz")


def command(argv, cwd, env=None):
    result = run_process(argv, cwd=cwd, env=env, capture_output=True, text=True, timeout=600)
    require(result.returncode == 0, "command failed: " + repr(argv) + "\n" + result.stdout + result.stderr)
    return result.stdout.strip()


def check_binary(data, target):
    os_name, arch = target.split("_")
    machine = None
    if os_name == "linux" and len(data) >= 20 and data[:6] == b"\x7fELF\x02\x01":
        machine = {62: "amd64", 183: "arm64"}.get(struct.unpack_from("<H", data, 18)[0])
    elif os_name == "darwin" and len(data) >= 8 and struct.unpack_from("<I", data)[0] == 0xfeedfacf:
        machine = {0x01000007: "amd64", 0x0100000c: "arm64"}.get(struct.unpack_from("<I", data, 4)[0])
    elif os_name == "windows" and len(data) >= 64 and data[:2] == b"MZ":
        offset = struct.unpack_from("<I", data, 60)[0]
        if offset + 6 <= len(data) and data[offset:offset + 4] == b"PE\0\0":
            machine = {0x8664: "amd64", 0xaa64: "arm64"}.get(struct.unpack_from("<H", data, offset + 4)[0])
    require(machine == arch, "binary format/architecture mismatch: " + target)


def regular_bytes(path, limit):
    require(not path.is_symlink() and path.is_file(), "missing/non-regular bundle file: " + path.name)
    require(path.stat().st_size <= limit, "bundle file too large: " + path.name)
    with path.open("rb") as stream:
        data = stream.read(limit + 1)
    require(len(data) <= limit, "bundle file grew beyond limit")
    return data


def binary_bytes(bundle, manifest, target):
    require(target in TARGETS, "unsupported target")
    rows = [r for r in manifest["artifacts"] if r["target"] == target]
    require(len(rows) == 1, "missing/duplicate artifact target")
    row = rows[0]
    require(row.get("archive") == archive_name(manifest["version"], target), "invalid archive name")
    size = row.get("binary_size")
    require(type(size) is int and 0 < size <= MAX_BINARY, "invalid binary size")
    archive = regular_bytes(Path(bundle) / row["archive"], MAX_ARCHIVE)
    require(sha(archive) == row.get("archive_sha256"), "archive checksum mismatch: " + target)
    name = binary_name(target)
    try:
        if target.startswith("windows_"):
            with zipfile.ZipFile(io.BytesIO(archive)) as zf:
                members = zf.infolist()
                require(len(members) == 1, "archive must contain exactly one member")
                item = members[0]
                require(item.filename == name and not item.is_dir() and stat.S_ISREG(item.external_attr >> 16), "invalid/non-regular zip member")
                require(item.file_size == size and not item.flag_bits & 1, "invalid zip member size/encryption")
                with zf.open(item) as stream:
                    data = stream.read(size + 1)
        else:
            # Read a single fixed-size header ourselves: TarFile.next() processes
            # unbounded PAX/GNU metadata before returning an apparent member.
            with gzip.GzipFile(fileobj=io.BytesIO(archive), mode="rb") as stream:
                item = tarfile.TarInfo.frombuf(stream.read(512), "ascii", "strict")
                require(item.name == name and item.type in (tarfile.REGTYPE, tarfile.AREGTYPE) and not item.linkname and item.size == size, "invalid/non-regular or extended tar member")
                data = stream.read(size)
                padding = (-size) % 512
                require(stream.read(padding) == b"\0" * padding, "invalid tar member padding")
                require(stream.read(1024) == b"\0" * 1024, "missing tar end markers")
                # A bounded read through gzip EOF validates its CRC/size trailer;
                # even compressed zero padding cannot consume unbounded space.
                tail = stream.read(tarfile.RECORDSIZE + 1)
                require(len(tail) <= tarfile.RECORDSIZE and not tail.strip(b"\0"), "extra member/excessive tar padding")
    except (tarfile.TarError, zipfile.BadZipFile, EOFError, OSError, RuntimeError) as error:
        raise GateError("invalid/truncated archive: " + str(error)) from error
    require(len(data) == size and sha(data) == row.get("binary_sha256"), "binary size/checksum mismatch")
    check_binary(data, target)
    return data


def verify_bundle(bundle):
    bundle = Path(bundle)
    raw = regular_bytes(bundle / "manifest.json", 2 * 1024 * 1024)
    manifest = strict_json(raw)
    require(isinstance(manifest, dict) and manifest.get("schema") == "mulix.release.v1", "invalid release manifest")
    valid_version(manifest.get("version"))
    source = manifest.get("source")
    require(isinstance(source, dict) and re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", source.get("commit", "")) and re.fullmatch(r"[0-9a-f]{64}", source.get("fingerprint", "")) and type(source.get("dirty")) is bool, "invalid source binding")
    rows = manifest.get("artifacts")
    require(isinstance(rows, list) and len(rows) == len(TARGETS) and all(isinstance(r, dict) for r in rows), "incomplete artifact matrix")
    require({r.get("target") for r in rows} == set(TARGETS), "missing/duplicate artifact targets")
    names = {"manifest.json", "SHA256SUMS"} | {archive_name(manifest["version"], target) for target in TARGETS}
    require({p.name for p in bundle.iterdir()} == names, "unexpected/missing bundle filesystem entries")
    expected = {"manifest.json": sha(raw)}
    for row in rows:
        binary_bytes(bundle, manifest, row["target"])
        expected[row["archive"]] = row["archive_sha256"]
    actual = {}
    for line in regular_bytes(bundle / "SHA256SUMS", 4096).decode("ascii").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._+-]+)", line)
        require(match is not None, "invalid checksum line")
        checksum, name = match.groups()
        require(name not in actual, "duplicate checksum entry")
        actual[name] = checksum
    require(actual == expected, "checksum inventory mismatch")
    return manifest


def bundle_snapshot(bundle):
    manifest = verify_bundle(bundle)
    names = ["manifest.json", "SHA256SUMS"] + [r["archive"] for r in manifest["artifacts"]]
    return {name: sha(regular_bytes(Path(bundle) / name, MAX_ARCHIVE)) for name in names}


def source_staging(source, prefix):
    """Keep build products outside source, including with an in-checkout TMPDIR."""
    parent = Path(tempfile.gettempdir()).resolve()
    if parent.is_relative_to(source):
        parent = source.parent
    return tempfile.TemporaryDirectory(prefix=prefix, dir=parent)


def build_bundle(source, output, version):
    valid_version(version)
    source, output = Path(source).resolve(), Path(output).absolute()
    require(not output.exists() and not output.is_symlink(), "output already exists: " + str(output))
    before = source_snapshot(source)
    build_env = dict(os.environ, CGO_ENABLED="0", GOFLAGS="", GOENV="off", GOWORK="off", GOTOOLCHAIN="local", GOEXPERIMENT="", GOAMD64="v1", GOARM64="v8.0")
    tool = command(["go", "version"], source, build_env)
    output.parent.mkdir(parents=True, exist_ok=True)
    with source_staging(source, ".mulix-release-") as temporary:
        stage = Path(temporary)
        payload = stage / "bundle"
        payload.mkdir()
        rows = []
        for target in TARGETS:
            binary = stage / target / binary_name(target)
            binary.parent.mkdir()
            os_name, arch = target.split("_")
            env = dict(build_env, GOOS=os_name, GOARCH=arch)
            command(["go", "build", "-trimpath", "-ldflags", "-X main.version=" + version + " -s -w", "-o", str(binary), "./cmd/mulix"], source, env)
            data = binary.read_bytes()
            check_binary(data, target)
            archive = payload / archive_name(version, target)
            if os_name == "windows":
                with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
                    item = zipfile.ZipInfo(binary.name)
                    item.create_system = 3
                    item.external_attr = 0o100755 << 16
                    item.compress_type = zipfile.ZIP_DEFLATED
                    zf.writestr(item, data)
            else:
                with tarfile.open(archive, "w:gz") as tf:
                    item = tarfile.TarInfo(binary.name)
                    item.size, item.mode, item.mtime = len(data), 0o755, 0
                    tf.addfile(item, io.BytesIO(data))
            rows.append({"target": target, "archive": archive.name, "archive_sha256": sha(archive.read_bytes()), "binary_sha256": sha(data), "binary_size": len(data)})
        after = source_snapshot(source)
        require(before["fingerprint"] == after["fingerprint"], "source changed during build")
        manifest = {"schema": "mulix.release.v1", "version": version, "source": {k: before[k] for k in ("commit", "fingerprint", "dirty")}, "go_version": tool, "artifacts": rows}
        (payload / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        (payload / "SHA256SUMS").write_text("".join(sha(p.read_bytes()) + "  " + p.name + "\n" for p in sorted(payload.iterdir())), encoding="ascii")
        verify_bundle(payload)
        # Exclusive reservation: never rename over an existing output, even an empty one.
        output.mkdir()
        for path in sorted(payload.iterdir(), key=lambda p: (p.name in ("manifest.json", "SHA256SUMS"), p.name)):
            with (output / path.name).open("xb") as stream:
                stream.write(path.read_bytes())
    return verify_bundle(output)


def atomic_install(data, prefix, version, validate):
    """The only publication is os.replace; every fallible validation precedes it."""
    prefix = Path(prefix).absolute()
    require(not prefix.is_symlink(), "installation prefix cannot be a symlink")
    prefix.mkdir(parents=True, exist_ok=True)
    destination = prefix / binary_name(native_target())
    require(not destination.is_symlink() and (not destination.exists() or destination.is_file()), "installed target must be an ordinary file")
    fd, name = tempfile.mkstemp(prefix=".mulix-install-", suffix=".exe" if os.name == "nt" else ".tmp", dir=prefix)
    staged = Path(name)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        staged.chmod(0o755)
        observed = command([str(staged), "version"], prefix)
        require(observed == "mulix " + version, "staged version differs from manifest/request: " + observed)
        validate(staged)
        os.replace(staged, destination)
        return destination
    finally:
        staged.unlink(missing_ok=True)  # Only this invocation's unique staging file.


def install_bundle(bundle, prefix, expected_version=None):
    manifest = verify_bundle(bundle)
    if expected_version is not None:
        require(valid_version(expected_version) == manifest["version"], "requested version differs from bundle")
    before = bundle_snapshot(bundle)
    data = binary_bytes(bundle, manifest, native_target())
    def stable(_staged):
        require(before == bundle_snapshot(bundle), "bundle changed during installation")
    return atomic_install(data, prefix, manifest["version"], stable)


def install_source(source, prefix, version=None):
    source = Path(source).resolve()
    stamp = valid_version(version if version is not None else default_version(source))
    before = source_snapshot(source)
    with source_staging(source, "mulix-source-install-") as temporary:
        binary = Path(temporary) / binary_name(native_target())
        os_name, arch = native_target().split("_")
        env = dict(os.environ, CGO_ENABLED="0", GOOS=os_name, GOARCH=arch, GOFLAGS="", GOENV="off", GOWORK="off", GOTOOLCHAIN="local")
        command(["go", "build", "-trimpath", "-ldflags", "-X main.version=" + stamp + " -s -w", "-o", str(binary), "./cmd/mulix"], source, env)
        def stable(staged):
            require(before["fingerprint"] == source_snapshot(source, transient_files=(staged,))["fingerprint"], "source changed during installation")
        return atomic_install(binary.read_bytes(), prefix, stamp, stable)


def default_version(source):
    if os.environ.get("MULIX_VERSION"):
        return os.environ["MULIX_VERSION"]
    result = subprocess.run(["git", "-C", str(source), "describe", "--tags"], capture_output=True, text=True)
    return result.stdout.strip() if result.returncode == 0 else "dev"


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    build = commands.add_parser("build")
    build.add_argument("--source", type=Path, default=Path(__file__).resolve().parent.parent)
    build.add_argument("--output", type=Path)
    build.add_argument("--version")
    build.add_argument("--dry-run", action="store_true")
    verify = commands.add_parser("verify")
    verify.add_argument("--bundle", type=Path, required=True)
    install = commands.add_parser("install")
    origin = install.add_mutually_exclusive_group()
    origin.add_argument("--bundle", type=Path)
    origin.add_argument("--source", type=Path)
    install.add_argument("--prefix", type=Path, default=Path.home() / ".local/bin")
    install.add_argument("--version")
    args = parser.parse_args(argv)
    try:
        if args.command == "build":
            version = valid_version(args.version if args.version is not None else default_version(args.source))
            output = args.output or args.source / "dist" / ("mulix_" + version)
            if args.dry_run:
                result = {"dry_run": True, "qualified": False, "version": version, "output": str(output), "targets": list(TARGETS)}
            else:
                result = build_bundle(args.source, output, version)
        elif args.command == "verify":
            result = verify_bundle(args.bundle)
        else:
            if args.bundle is not None:
                binary = install_bundle(args.bundle, args.prefix, args.version)
            elif args.source is not None:
                binary = install_source(args.source, args.prefix, args.version)
            else:
                with tempfile.TemporaryDirectory(prefix="mulix-install-source-") as temporary:
                    source = Path(temporary) / "source"
                    ref = valid_version(args.version) if args.version is not None else "main"
                    command(["git", "clone", "--depth", "1", "--branch", ref, "https://github.com/z-fenix/mulix-coding", str(source)], Path(temporary))
                    binary = install_source(source, args.prefix, args.version)
            result = {"installed": str(binary)}
    except (GateError, OSError, ValueError, subprocess.SubprocessError) as error:
        print("DELIVERY_BLOCKED: " + str(error), file=sys.stderr)
        return 1
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
