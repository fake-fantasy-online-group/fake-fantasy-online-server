#!/usr/bin/env bash
# Extract trusted PostgreSQL packages privately; never start a daemon.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-${FFO_PG_TOOLCHAIN_HOME:-$HOME/.cache/ffo/pg-toolchain}}"
for tool in python3 curl dpkg-deb ldd; do command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 1; }; done
[[ "$(uname -s)/$(uname -m)" == Linux/x86_64 ]] || { echo 'This lock targets Debian 13-compatible Linux x86_64.' >&2; exit 1; }
mkdir -p "$ROOT"; ROOT="$(cd "$ROOT" && pwd)"
python3 - "$HERE/postgresql-debian-packages.lock.json" "$ROOT" <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess
import sys
root = Path(sys.argv[2])
cache = root / "downloads"
cache.mkdir(exist_ok=True)
for package in json.loads(Path(sys.argv[1]).read_text()):
    path = cache / Path(package["Filename"]).name
    if not path.exists():
        partial = path.with_suffix(path.suffix + ".partial")
        subprocess.run(["curl", "--fail", "--location", "--silent", "--show-error", "--max-time", "180", "https://deb.debian.org/debian/" + package["Filename"], "-o", str(partial)], check=True)
        if hashlib.sha256(partial.read_bytes()).hexdigest() != package["SHA256"]:
            raise RuntimeError(f"Downloaded package hash mismatch: {partial}")
        partial.replace(path)
    if hashlib.sha256(path.read_bytes()).hexdigest() != package["SHA256"]:
        raise RuntimeError(f"Cached package hash mismatch: {path}")
    subprocess.run(["dpkg-deb", "-x", str(path), str(root / "debian")], check=True)
print("PASS pinned official Debian PostgreSQL package hashes and extraction")
PY
{
 printf '# Private PostgreSQL test tools only. No daemon is started by sourcing this file.\n'
 printf 'export FFO_PG_TOOLCHAIN_HOME=%q\n' "$ROOT"
 cat <<'ENV'
export PG_BIN="$FFO_PG_TOOLCHAIN_HOME/debian/usr/lib/postgresql/17/bin"
export PG_SHARE="$FFO_PG_TOOLCHAIN_HOME/debian/usr/share/postgresql/17"
export LD_LIBRARY_PATH="$FFO_PG_TOOLCHAIN_HOME/debian/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export PATH="$PG_BIN:$PATH"
ENV
} > "$ROOT/env.sh"
source "$ROOT/env.sh"
for binary in postgres initdb psql; do
 if ldd "$PG_BIN/$binary" | grep 'not found'; then
  echo "Missing native dependency for $binary. Restore it from a verified trusted source before continuing." >&2
  exit 1
 fi
 "$PG_BIN/$binary" --version
done
printf 'Ready. Run: source %q\n' "$ROOT/env.sh"
