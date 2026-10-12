#!/usr/bin/env python3
"""Seed an entirely new disposable PostgreSQL cluster; never start a daemon.

Uses the existing production-migration-aware SQL splitter. Refuses an existing
work directory and records exact source hashes. Does not export skill anchors.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys

REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / "server/tests/sql"))
from validate_daoist_postgres import check_splitter, concatenate_sources, write_single_user_commands


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work-dir", type=Path, required=True)
    args = parser.parse_args()
    work = args.work_dir.resolve()
    if work.exists():
        parser.error("--work-dir must not exist; only a brand-new disposable cluster is supported")
    bins = Path(os.environ["PG_BIN"]).resolve()
    version = subprocess.check_output([str(bins / "postgres"), "--version"], text=True).strip()
    if not re.search(r"PostgreSQL\) 17\.", version):
        raise RuntimeError(f"Expected PostgreSQL 17 test tools, got {version}")
    check_splitter()
    sources = sorted((REPO / "server/migrations").glob("000000_init-*.sql"), key=lambda p: int(p.stem.rsplit("-", 1)[1]))
    indexes = [int(path.stem.rsplit("-", 1)[1]) for path in sources]
    if not indexes or indexes != list(range(indexes[-1] + 1)):
        raise RuntimeError("Migration fragments are missing or non-contiguous")
    manifest = {"postgres_version": version, "database": "fresh_test", "migration_files": [{"path": str(path.relative_to(REPO)), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()} for path in sources]}
    work.mkdir(parents=True)
    data = work / "data"
    cmd = [str(bins / "initdb"), "-D", str(data), "--no-locale", "-E", "UTF8", "-U", "validator", "-A", "trust"]
    if os.environ.get("PG_SHARE"):
        cmd += ["-L", os.environ["PG_SHARE"]]
    with (work / "initdb.log").open("w") as log:
        subprocess.run(cmd, check=True, stdout=log, stderr=subprocess.STDOUT)

    def run(database, label, sql=None, paths=None):
        source_path, input_path = work / (label + ".source.sql"), work / (label + ".sql")
        log_path = work / (label + ".log")
        with source_path.open("wb") as out:
            if paths:
                out.write(b"BEGIN;\n")
                concatenate_sources(paths, out)
                out.write(b"\nCOMMIT;\n")
            else:
                out.write(sql.encode("utf-8"))
        with source_path.open(encoding="utf-8", newline="") as source, input_path.open("w", encoding="utf-8", newline="") as out:
            count, largest = write_single_user_commands(source, out)
        source_path.unlink()
        print(f"RUN {label}: {count} statements; largest {largest:,} characters", flush=True)
        with input_path.open(encoding="utf-8") as source, log_path.open("w", encoding="utf-8") as log:
            result = subprocess.run([str(bins / "postgres"), "--single", "-j", "-D", str(data), database], stdin=source, stdout=log, stderr=subprocess.STDOUT)
        with log_path.open(encoding="utf-8") as log:
            errors = any(re.search(r"\b(?:ERROR|FATAL|PANIC):", line) for line in log)
        if result.returncode or errors:
            raise RuntimeError(f"{label} failed; see {log_path}")
        input_path.unlink()
        print(f"PASS {label}", flush=True)

    run("postgres", "create-fresh", sql="CREATE DATABASE fresh_test;")
    run("fresh_test", "fresh-init", paths=sources)
    run("fresh_test", "read-only-smoke", sql="BEGIN READ ONLY; SELECT 'FRESH_SKILL_CLUSTER_READY'; SELECT count(*) AS skill_description_rows FROM gamedata.ov_skilldesc; ROLLBACK;")
    if (data / "postmaster.pid").exists():
        raise RuntimeError("Cluster is not idle; postmaster.pid unexpectedly exists")
    (work / "source-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    (work / "env.sh").write_text("# Use only for serial tests against this idle disposable cluster.\n" + "export FANTASY_V5_SKILL_PG_DATA=" + shlex.quote(str(data)) + "\nexport FANTASY_V5_SKILL_PG_DATABASE=fresh_test\n")
    print(f"READY: PG_BIN={bins}; PG_DATA={data}; database=fresh_test", flush=True)
    print("The cluster is idle. Serialize all single-user tests; no daemon was started.", flush=True)


if __name__ == "__main__":
    main()
