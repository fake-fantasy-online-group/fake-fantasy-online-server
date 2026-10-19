#!/usr/bin/env python3
"""Build unpublished, binary-only server bundles from a clean Git checkout.

This creates local files only. It never uploads, deploys, executes SQL, or runs a
server. The sole executable invocation is Linux binaries' side-effect-free
--version path. Go 1.21 remains the module language baseline; the CI toolchain is
pinned separately in .github/go-version.
"""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
MODULE = "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server"
PROGRAMS = ("gameserver", "dispatch", "migrate")
TARGETS = ("linux", "windows")
README = """UNRELEASED SERVER BUILD

This is a development build targeting 1.0.0, not a production release, deployment
approval, protocol compatibility certification, or database baseline record.

Contents: gameserver, dispatch, migrate, binary build identity, checksums, and
license notices. There is no Fyne controller, game asset bundle, source checkout,
private key, environment configuration, SQL seed, player database, or backup.
The dispatch binary retains its existing embedded web UI.

migrate IS ONLY AN EMPTY DEVELOPMENT DATABASE INITIALIZER. It is NOT a production
incremental migration tool. Do not run it against an existing/player database.
Its required init SQL is deliberately not bundled. None of these programs are
automatically run by this bundle. Runtime configuration and database validation
require a separate, explicitly approved process.

SHA256SUMS lists every other file in this bundle. The outer SHA256SUMS verifies
the ZIP files. These hashes detect byte changes; they are not a signed release.
All three programs support --version without opening databases or services.
"""


def run(*args, cwd=ROOT, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True).strip()


def require_clean_checkout():
    if run("git", "status", "--porcelain", "--untracked-files=all"):
        raise ValueError("refusing to package a dirty checkout (including untracked files)")
    commit = run("git", "rev-parse", "HEAD")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("expected a full 40-character Git commit")
    return commit


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def checksum_text(paths):
    return "".join(f"{sha256(p)}  {p.name}\n" for p in sorted(paths))


def bundle_names(goos):
    suffix = ".exe" if goos == "windows" else ""
    return {p + suffix for p in PROGRAMS} | {
        "BUILD-INFO.json", "README.txt", "LICENSE", "NOTICE.md", "SHA256SUMS"
    }


def create_bundle(stage, output, goos):
    # Exact allowlist: never archive the checkout or recursively include files.
    expected = bundle_names(goos)
    if {p.name for p in stage.iterdir()} != expected or any(
        not p.is_file() or p.is_symlink() for p in stage.iterdir()
    ):
        raise ValueError("unexpected bundle contents")
    with zipfile.ZipFile(output, "x", compression=zipfile.ZIP_DEFLATED) as archive:
        for name in sorted(expected):
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            mode = 0o755 if name in PROGRAMS or name.endswith(".exe") else 0o644
            info.external_attr = (0o100000 | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, (stage / name).read_bytes())
    verify_bundle(output, goos)


def verify_bundle(path, goos):
    with zipfile.ZipFile(path) as archive:
        names = archive.namelist()
        if len(names) != len(set(names)) or set(names) != bundle_names(goos):
            raise ValueError("archive allowlist mismatch")
        manifest = archive.read("SHA256SUMS").decode("ascii").splitlines()
        listed = set()
        for line in manifest:
            digest, name = line.split("  ", 1)
            if name in listed or name == "SHA256SUMS" or name not in names:
                raise ValueError("invalid checksum entry")
            if hashlib.sha256(archive.read(name)).hexdigest() != digest:
                raise ValueError(f"checksum mismatch: {name}")
            listed.add(name)
        if listed != set(names) - {"SHA256SUMS"}:
            raise ValueError("incomplete checksum manifest")


def build(output, go):
    commit = require_clean_checkout()
    toolchain = (ROOT / ".github/go-version").read_text().strip()
    if not re.fullmatch(r"1\.[0-9]+\.[0-9]+", toolchain):
        raise ValueError("Go toolchain must be pinned to an exact patch version")
    env = os.environ.copy()
    env.update(CGO_ENABLED="0", GOTOOLCHAIN="local", GOWORK="off", GOFLAGS="-mod=readonly",
               GOARCH="amd64", GOAMD64="v1", GOEXPERIMENT="", GOENV="off")
    if run(go, "env", "GOVERSION", env=env) != "go" + toolchain:
        raise ValueError(f"use the pinned Go {toolchain} toolchain")
    if run(go, "env", "GOHOSTOS", "GOHOSTARCH", env=env).splitlines() != ["linux", "amd64"]:
        raise ValueError("artifact verification requires a Linux amd64 build host")
    output = output.resolve()
    if output == ROOT or ROOT in output.parents:
        raise ValueError("artifact output must be outside the source checkout")
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        raise ValueError("artifact output directory must be empty")
    archives = []
    with tempfile.TemporaryDirectory(prefix="server-build-") as temp:
        # Build precisely the named commit, excluding ignored local keys, config,
        # and generated files that could otherwise influence Go/embed inputs.
        source = Path(temp) / "source"
        source.mkdir()
        snapshot = subprocess.check_output(
            ["git", "archive", "--format=tar", commit, "server", "LICENSE", "NOTICE.md"],
            cwd=ROOT,
        )
        with tarfile.open(fileobj=io.BytesIO(snapshot)) as archive:
            archive.extractall(source, filter="data")
        run(go, "mod", "download", cwd=source / "server", env=env)
        run(go, "mod", "verify", cwd=source / "server", env=env)
        for goos in TARGETS:
            stage = Path(temp) / goos
            stage.mkdir()
            env["GOOS"] = goos
            components = []
            for program in PROGRAMS:
                suffix = ".exe" if goos == "windows" else ""
                binary = stage / (program + suffix)
                subprocess.run(
                    [go, "build", "-trimpath", "-buildvcs=false", "-ldflags",
                     f"-X {MODULE}/internal/buildinfo.commit={commit}",
                     "-o", str(binary), f"./cmd/{program}"],
                    cwd=source / "server", env=env, check=True,
                )
                settings = run(go, "version", "-m", str(binary), env=env)
                if f"{MODULE}/internal/buildinfo.commit={commit}" not in settings:
                    raise ValueError("binary is missing the full source commit")
                for value in ("CGO_ENABLED=0", f"GOOS={goos}", "GOARCH=amd64", "GOAMD64=v1"):
                    if value not in settings.split():
                        raise ValueError(f"missing binary build setting: {value}")
                if goos == "linux":
                    # Invalid runtime paths ensure --version is handled before startup.
                    smoke_env = dict(env, DATABASE_URL="not-a-database-connection")
                    identity = json.loads(subprocess.check_output(
                        [str(binary), "--version"], env=smoke_env, cwd=temp,
                        text=True, timeout=10,
                    ))
                    if (identity["commit"] != commit or identity["status"] != "unreleased"
                            or identity["program"] != program or identity["version"] != "1.0.0"
                            or identity["go_version"] != "go" + toolchain
                            or identity["goos"] != goos or identity["goarch"] != "amd64"
                            or identity["build_version"] != "1.0.0-unreleased+sha." + commit):
                        raise ValueError("binary identity mismatch")
                    if program == "migrate" and identity["purpose"] != "empty-development-database-init-only":
                        raise ValueError("initializer purpose is missing")
                components.append({"program": program, "file": binary.name,
                                   "purpose": "empty-development-database-init-only" if program == "migrate"
                                   else "server-component"})
            metadata = {
                "version": "1.0.0", "status": "unreleased",
                "build_version": "1.0.0-unreleased+sha." + commit,
                "commit": commit, "source_state": "clean", "go_version": "go" + toolchain,
                "goos": goos, "goarch": "amd64", "goamd64": "v1", "cgo_enabled": False,
                "components": components,
            }
            (stage / "BUILD-INFO.json").write_text(json.dumps(metadata, indent=2) + "\n")
            (stage / "README.txt").write_text(README)
            for name in ("LICENSE", "NOTICE.md"):
                (stage / name).write_bytes((source / name).read_bytes())
            (stage / "SHA256SUMS").write_text(checksum_text(list(stage.iterdir())))
            archive = output / f"server-1.0.0-unreleased-{commit}-{goos}-amd64.zip"
            create_bundle(stage, archive, goos)
            archives.append(archive)
    if require_clean_checkout() != commit:
        raise ValueError("source changed during build; do not use these artifacts")
    (output / "SHA256SUMS").write_text(checksum_text(archives))
    print(f"Verified binary-only, unpublished artifacts: {output}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--go", default="go", help="path to the pinned Go executable")
    args = parser.parse_args()
    build(args.output, args.go)
