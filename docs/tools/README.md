# Cloud Go test-toolchain recovery

Restore a private pinned Go SDK and reproducible test cache locations on Linux x86_64:

```sh
bash docs/tools/bootstrap-go-test-toolchain.sh /workspace/shared/server-toolchains
source /workspace/shared/server-toolchains/env.sh
go version
(cd server && go test ./...)
```

The script downloads the official `go1.27.2.linux-amd64.tar.gz` archive from `https://go.dev/dl/`, verifies SHA-256 `ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5`, and extracts it under the requested directory. The checksum was verified against the official Go download JSON on 2026-10-12. It needs Bash, curl, sha256sum, and tar; it does not need sudo or a system package install.

The generated `env.sh` sets:

- `GOROOT`: `<toolchain-root>/go`
- `GOPATH`: `<toolchain-root>/gopath`
- `GOMODCACHE`: `<toolchain-root>/gopath/pkg/mod`
- `GOCACHE`: `<toolchain-root>/gocache`
- `GOTOOLCHAIN=local`, so tests cannot silently download a different Go SDK
- The official Go module proxy and checksum database
- `PATH`, so `go` resolves to this SDK

Always verify `go version`; some cloud images contain an unrelated program named `/usr/bin/go`. A mismatched existing SDK or cached archive stops the script rather than overwriting an unknown installation. This script does not start a server or database, build a desktop controller, change business code, configure credentials, or modify system settings.

Recovery verification on 2026-10-12: `go version go1.27.2 linux/amd64`; `server/` → `go test ./...` passed. Database-backed validation and runtime integration remain separate checks.

## Socket-free PostgreSQL 17 recovery

The separate PostgreSQL bootstrap privately extracts official Debian 13 packages. It does not use apt installation or run package maintainer scripts, modify system settings, register services, or start a TCP/socket daemon.

```sh
bash docs/tools/bootstrap-postgres-test-toolchain.sh /workspace/shared/pg-toolchain
source /workspace/shared/pg-toolchain/env.sh
python3 docs/tools/seed-single-user-skill-cluster.py \
  --work-dir /workspace/shared/pg-toolchain/skill-loader-seed-20261012
```

The work directory must not already exist. Seeding invokes `initdb`, creates `fresh_test`, then loads every contiguous `server/migrations/000000_init-N.sql` fragment in numeric order using `postgres --single -j` and the existing production-aware SQL splitter from `server/tests/sql/validate_daoist_postgres.py`. It records all migration SHA-256 values in a generated source manifest. It never targets an existing user database and never exports or overwrites skill/status anchors.

When the seed process has exited and no `postmaster.pid` is present, make a disposable copy for serial loader tests:

```sh
cp -a /workspace/shared/pg-toolchain/skill-loader-seed-20261012/data \
  /workspace/shared/pg-toolchain/skill-loader-test-data
source /workspace/shared/server-toolchains/env.sh
export FANTASY_V5_SKILL_PG_DATA=/workspace/shared/pg-toolchain/skill-loader-test-data
export FANTASY_V5_SKILL_PG_DATABASE=fresh_test
(cd server && env -u FANTASY_V5_EXPORT_ANCHORS_DIR go test ./internal/data \
  -run '^TestV5SkillCurvesActualReadOnlyLoader$' -count=1 -v)
```

Choose a new, nonexistent destination for each copy. Never run concurrent single-user backends against the same cluster. The loader's queries use `BEGIN READ ONLY; ... ROLLBACK`; PostgreSQL still needs write access to its disposable cluster for normal internal bookkeeping. This is not a production database connection or a network/concurrency validation.

Pinned PostgreSQL packages are `postgresql-17`, `postgresql-client-17`, and `libpq5`, all `17.11-0+deb13u1`. Package paths and SHA-256 hashes are in `postgresql-debian-packages.lock.json`. The lock was derived on 2026-10-12 from the same verified official Debian trixie amd64 Packages index used for the managed-test bootstrap:

- InRelease SHA-256: `0584fba32e13e0ab8285fb16c27adea1ec03a73669c18702821094fd6ca86675`
- Packages.xz SHA-256: `7778d3e3f303b7ddb8ce0fe7c8d57473a076c6bf2e8f241f75421d2396352498`
- InRelease signatures verified against the environment's Debian archive keyring

The bootstrap verifies every archive hash and checks required native libraries with `ldd`. It expects a Debian 13-compatible base image; if native libraries are missing, restore them from verified official packages rather than changing system security/network settings. LLVM/JIT, GUI, networking, service management, and extension installation are outside this source-loader test bootstrap's scope.

Recovery verification on 2026-10-12:

- `postgres`, `initdb`, and `psql`: `17.11 (Debian 17.11-0+deb13u1)`.
- Fresh seed loaded all 56 migration fragments (`0..55`), 8,233 SQL commands including the transaction boundary. Recorded source hashes were rechecked after seeding.
- A copied idle disposable cluster passed `TestV5SkillCurvesActualReadOnlyLoader` with anchor export explicitly disabled. The current loader reported 750 class rows, 149 battle skills, and 1,900 status definitions. These are observed counts for the then-current loader, not a replacement for older anchor manifests or an instruction to overwrite them.
- No daemon or TCP listener was started; no user database, original anchor file, or application source was modified by this recovery.
