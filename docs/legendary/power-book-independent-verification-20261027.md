# Independent power-book / class-rework verification — 2026-10-27

## Tested revision

Runtime tree: `acaa476f8d361beefb9b70eae16272584e1f58c7`
(`feaa071e2098274bf820e174689b5b5774d0d450`). Independent tests added in
`3bd68c1f75c7dbb87e9845f75c2ff485af36424b`; final current-loader test accompanies this report.
Final origin/main fetch succeeded: `eeb4956dcc67b932d1ee29f60c30cf71caa97090`, verified ancestor.
Only cloud scratch worktrees / disposable PostgreSQL copies were used; no production
DB access, deployment, remote publication, or client end-to-end claim.

## Results

With the pinned toolchain (`source /workspace/shared/server-toolchains/env.sh`):

- `go test ./... -skip 'Stress|Postgres|WorstCaseLegalPackets' -count=1 -timeout=15m -json`:
  **2,833 top-level tests passed; 14,382 including subtests; zero failed.**
  23 packages passed; 8 had no tests. **66 explicit environment/opt-in skips** remain.
  Scene package 241.618 sec.
- `go test ./internal/game/scene -run '^TestStress' -count=1 -timeout=5m -json`:
  **6 test families / 36 including subtests passed in 63.494 sec**, on the integrated
  runtime (not merely the earlier class-rework baseline). Covers admission/transfer,
  loot contention, 300 targets × 32 DoT states, overlapping/party auras and dense AI.
- New same-UID tests pass: permanent buffs refresh 7500→8750→10000→legacy 0→7500
  without duplicates or logout persistence; source rank retained. An accepted cast
  preserves its original 7500 roll despite a same-UID upgrade; a later cast uses
  10000; legacy 0 accepted sources remain full-strength.

## Real PostgreSQL evidence (not mocks)

Official Debian PostgreSQL **17.11**, verified pinned package hashes, private extracted
toolchain; no system service install. Production SQL migrations 0–114 were applied
in a new disposable cluster. Migration 99's deliberately separate operator activation
from Rift 9 to 10 was reproduced as an explicitly test-only activation before the
later migrations; no production migration/selector was changed.

An idle copy then received migration 115 via the existing SQL-aware single-user splitter.
The new `TestPowerBookCurrentPostgresLoaders` passed in **21.026 sec**:

- Actual `LoadItems`, `LoadSkills`, `LoadEndgameCatalog`, equipment runtime reference
  validation, `LoadCubeRules`, `LoadLootRules`, and `LoadRiftRules`.
- **22,252 items, 62 active cube recipes, 71 read-only SQL queries**.
- Active **Loot 5 / Rift 17**, new numeric-roll gate enabled, historical Loot 1–4 gates off.
- Each loader query ran through real PostgreSQL in `BEGIN READ ONLY; ROLLBACK`.

Other actual-loader tests passed: original/extended skill curves (including shield
30 sec per rank), native restoration, and native status icons. The shield test had a
stale raw-row equality assertion; it now matches the existing approved runtime override
and additionally asserts resolved durations. **No runtime behavior was changed for it.**
The migration worker separately verified migration 115 twice (idempotence), source-drift
and rollback/guard cases; its results are supplementary, not counted above.

## Explicit remaining limits

The three new `TestPowerBookPostgres*` families compile but **were not executed**:
real pgx state/snapshot/receipt fault rollback, lost-ACK recovery/relog/replay/isolation,
and concurrent revision CAS. Normal private Unix-socket startup failed with
`could not create Unix socket ... Operation not permitted`. The approved escalation
route also failed before command execution because the execution environment could not start the requested process. No security settings or access restrictions were changed, and no alternate transport was used to evade the denial.

In-memory atomicity/replay tests did execute; SQL migration/read-only loader tests are
**not substitutes for the unrun pgx transaction tests**. Smallest remaining step:
run the three committed test families on a cloud executor permitting normal connections
to an explicitly disposable `*_test` PostgreSQL database using `fantasy_TEST_DSN`.
Full exhaustive `WorstCaseLegalPackets`, opted-out live/network tests, client UI/game
sessions and production deployment remain outside these claimed results.

## Evidence files

Under the independent worktree's `verification-logs/` (packaged separately):

- `full-integrated.json`
- `final-stress.json`
- `roll-lifecycle.json`
- `actual-pg-curves.json`
- `current-pg-loaders.json`
- `independent-pg115.log`

Earlier restoration/icon loader results are in
`class-power-verification/verification-logs/actual-pg-loaders.json`; its initial stale
shield expectation failure is superseded by `actual-pg-curves.json`, not erased.
