# Daoist endgame PostgreSQL validation

Date: 2026-10-11 UTC. Scope: disposable cloud-only test databases; no user/live database, deployment, or repository publication.

## Engine and provenance

The checks use real PostgreSQL **17.11 (Debian 17.11-0+deb13u1)**, extracted into a disposable workspace from the official Debian trixie package archive. No system-wide install or service was performed.

- Release: `https://deb.debian.org/debian/dists/trixie/InRelease`
- Package index: `https://deb.debian.org/debian/dists/trixie/main/binary-amd64/Packages.xz`
- Release signatures verified using the preinstalled Debian archive keyring; the trixie signing-key fingerprint is `04B54C3CDCA79751B16BC6B5225629DF75B188BD`.
- Package-index SHA256: `7778d3e3f303b7ddb8ce0fe7c8d57473a076c6bf2e8f241f75421d2396352498`, verified against the signed release.
- Each package's bytes were SHA256-checked against that package index before extraction:

| Official Debian package | SHA256 |
| --- | --- |
| `postgresql-17_17.11-0+deb13u1_amd64.deb` | `d2ce1ddffafa783f9acda4c92c86fc21e8288bff3782d739294f14fa797d7886` |
| `postgresql-client-17_17.11-0+deb13u1_amd64.deb` | `9d8558f8dd57c8e92e218a20698383575d53742ca3f9e7c2b7fe5f246d5216ae` |
| `libpq5_17.11-0+deb13u1_amd64.deb` | `20a4c9ef58b4baf90deda67cfb2cc062871c062830dddc234d28f5ac7931b86b` |

These packages are in `https://deb.debian.org/debian/pool/main/p/postgresql-17/`.

## Reproduce

Use an official, trusted PostgreSQL 17 installation. From the repository root:

```sh
PG_BIN=/usr/lib/postgresql/17/bin \
  python3 server/tests/sql/validate_daoist_postgres.py \
  --work-dir /tmp/daoist-postgres-validation
```

The destination must **not exist**. The runner creates its own cluster and databases, never accepts a DSN, and never starts a network/listening service. It keeps per-stage logs and the disposable cluster for inspection. If using extracted Debian binaries, set `LD_LIBRARY_PATH` to their matching `usr/lib/x86_64-linux-gnu` directory.

The runner concatenates init fragments in numeric order, matching `cmd/migrate`. Fragments are pieces of a single SQL source and must not be executed individually or sorted lexicographically. Blank lines are converted to whitespace-only lines for the standalone backend's `-j` delimiter; complete SQL/PLpgSQL blocks remain intact. It treats PostgreSQL `ERROR`, `FATAL`, and `PANIC` output as failures even when standalone PostgreSQL exits with status zero.

## Coverage

- Fresh database: all 51 fragments (`000000_init-0.sql` through `000000_init-50.sql`), with full baseline static data, in one transaction.
- Existing database: baseline fragments 0–49, a pre-existing dummy character, then the explicit additive migration.
- Migration applied a second time after data is populated; schema-column fingerprint, native item data, trial reward, overlay, character state, receipt, and run remain unchanged.
- On-disk persistence across separate standalone PostgreSQL processes, covering all new tables and both trial columns.
- UID-linked unique overlay, native refinement/socket preservation, duplicate UID rejection, foreign-key enforcement, and cascade behavior.
- Trial reward range, completion-ID length, overlay revision/unique-ID/roll bounds, receipt key/digest lengths, positive state/run revisions, and JSONB validity.
- Character-state and run compare-and-swap statements: current revision changes one row; stale revision and duplicate initial insert change zero rows.
- Duplicate receipt uniqueness and `ON CONFLICT DO NOTHING` zero-row behavior, corresponding to the store's conflict handling.
- A deliberately rejected final receipt insert rolls back earlier changes to the character state, native equipment, unique overlay, trial reward, and run in the same subtransaction.
- Explicit savepoint rollback and outer transaction rollback leave no test rows behind.

## Verification boundary

The cloud sandbox rejected creation of the local PostgreSQL Unix socket with `Operation not permitted`. That restriction was respected; no alternate listening transport or privilege escalation was used. Actual SQL execution uses PostgreSQL's supported socket-free `postgres --single` mode.

This validates the real SQL schema and transactional data behavior. It does **not** establish a Go/pgx store roundtrip, multi-client locking/concurrency, wire-level failures, or commit-acknowledgment ambiguity. Existing `testStore`-backed Go tests remain unverified here because they require a reachable PostgreSQL server. In particular, the SQL CAS assertions test affected-row behavior sequentially, not concurrent callers. Memory-backed/race Go test results are separate evidence and must not be described as database integration passes.

## Latest result

**PASS** on PostgreSQL 17.11. The reproducible runner exited **0** at 00:35 UTC, with every stage passing:

```text
PASS create-fresh
PASS create-upgrade
PASS fresh-init-0-through-50
PASS fresh-assertions
PASS baseline-init-0-through-49
PASS seed-existing-character
PASS upgrade-first
PASS seed-endgame-persistence
PASS upgrade-second
PASS restart-persistence-idempotence
PASS upgrade-assertions
All SQL checks passed. pgx/network/concurrent-lock tests were NOT run.
```

Validated migration SHA256:
- Explicit upgrade: `637634d7375c0e05a75b7b9dbeb92cfc04e9436c35722fcf3ef402de5b24121a`
- Init fragment 50: `12aab0b6127898b6b69fdc6cdf892275dc5f4947f0c71e1ef0f7c59d8588dbf6`

No runtime code changes were needed for these SQL checks. The initial standalone harness attempt exposed its blank-line delimiter behavior; the final reusable runner handles that correctly, and both clean initialization paths were rerun successfully.

## Ordinary equipment revision repair validation (07:20 UTC)

**PASS** on PostgreSQL 17.11 after repairing ordinary equipment overlay persistence. The clean runner passed fresh initialization/assertions, baseline initialization, original unique-only v1 schema seeding with an existing unique overlay, first upgrade, ordinary/unique persistence seeding, second upgrade, restart/idempotence assertions, and upgrade assertions. The new checks include ordinary revision retention, invalid paired metadata rejection, and ordinary-overlay rollback when final receipt insertion fails.

The original unique overlay retained its UID, revision, identity, and roll. Ordinary equipment retained revision 2 and zero unique metadata across separate standalone backend processes and a repeated migration. Existing native equipment, state/receipt/run CAS, uniqueness, and rollback assertions also pass.

Current validated schema hashes (superseding the pre-repair hashes above):
- Explicit upgrade: `3b9f4fefcc7a00c4bde875ad04c8b1ed960a055db4fa45415f0adac6b6491a4c`
- Init fragment 50: `8a7adc4b7a9bdfddc46b82333279a937d5ac7487d335a49293d027ad0d27eac5`

The same socket-free verification boundary applies: no live data, pgx network roundtrip, or multi-client locking claims. See `ORDINARY_SOCKET_REVISION.md` for the source repair and Go regressions.
