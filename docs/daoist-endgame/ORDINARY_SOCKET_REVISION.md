# Ordinary equipment socket revision repair

Date: 2026-10-11 UTC. Cloud working-tree repair only; no commit, publication, production database, or local-client changes.

## Root cause

`domain.PlanCube` correctly advances `EndgameItemState.Revision` for an ordinary equipment mutation such as `CubeOpenSocket`. However, `store.validateEndgameItem` required ordinary equipment to have revision zero. The first opening therefore failed with `endgame: invalid ordinary metadata` before the durable commit. A validation-only patch would still be wrong: `saveEndgameItem` deleted all ordinary overlays, and the SQL schema prohibited `unique_id = 0`.

## Repair

- Keep the existing revision increment and aggregate compare-and-swap/receipt gate unchanged.
- Permit a nonnegative ordinary equipment revision only on a single valid equipment UID; ordinary unique-roll metadata must remain zero.
- Preserve unique identity and roll-range validation; reject negative revisions, wrong instance kinds, missing UIDs, and stacked equipment carrying metadata.
- Persist any nonzero equipment metadata, including ordinary revisions; only an entirely zero legacy metadata value needs no overlay.
- Support the paired SQL invariant `(unique_id = 0 AND unique_roll_bp = 0)` or `(unique_id > 0 AND unique_roll_bp BETWEEN 8000 AND 12000)`, retaining the revision check, UID foreign key, and cascade behavior.
- Both fresh initialization and the repeatable explicit migration replace the original unique-only checks. Already-installed v1 databases must run the updated `docs/migrations/20261011_daoist_endgame_v1.sql` before using the repaired server. Applying a migration to a real deployment remains a separate operation.

## Regression coverage

`server/internal/store/endgame_ordinary_revision_test.go` reproduces the original failure with the real cube planner and durable memory backend. It now verifies two ordinary openings, retained revisions through reload, exact coin/material persistence, receipt replay without duplicate changes, and stale-CAS rejection. Additional cases cover invalid metadata and the actual overlay INSERT versus DELETE serializer decision.

The SQL runner now checks ordinary revision storage, negative/corrupt metadata rejection, transaction rollback of ordinary overlays, upgrade from the original v1 unique-only schema with existing data, and ordinary/unique revision persistence across backend restarts and repeated migration.

## Verification

- `go test ./... -count=1`: PASS.
- `go test -race ./internal/store -run 'Endgame|OrdinaryCube' -count=1`: PASS.
- `go vet ./internal/store`: PASS.
- `git diff --check`: PASS.
- Six pre-existing PostgreSQL/pgx tests skip because no reachable test database is configured. These results do not claim a network pgx roundtrip or multi-client database concurrency validation.

Real standalone PostgreSQL 17.11 clean run: PASS for fresh initialization, old-v1 upgrade, repeated upgrade, restart persistence, and all integrity/transaction assertions. See `POSTGRES_VALIDATION.md` for schema hashes and coverage.
