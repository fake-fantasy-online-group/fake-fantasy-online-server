# Explicit incremental SQL upgrades

These files are **not** consumed by the empty-database initialization command.
Do not run the complete `000000_init-*` set against an existing database.

## 20261019_monster_behavior.sql

Adds the two monster behavior flags and three configurable escape parameters,
then applies the approved level-1-through-60 combat-monster decisions. The
matching `000000_init-88.sql` tail applies identical data for newly initialized
empty databases. Earlier initialization fragments remain unchanged.

- Scope: positive HP, level 1–60, kind 普通 / 精英 / BOSS.
- Active attack: 68 original `ov_dialog` Scene 2 monster IDs.
- Flee: 15 original `ov_dialog` Scene 7 monster IDs.
- Assistance: 23 dialogue-reviewed, approved IDs; no implied elite inheritance.
- Pickup: 27 IDs across 23 approved names: 21 monkey/ape/rabbit names and the two
  human names 盗墓贼 / 凶暴的盗墓贼. Same-name combat records within scope included.
- Unlisted scoped monsters have these four flags disabled. Existing behavior
  flags on level >60, props, gatherables and zero-level placeholders are untouched.
- Escape defaults: below 20% HP, 3 seconds duration, 10 seconds cooldown, stored as
  milliseconds in `game_monster_ai_templates`. Runtime documentation/tests specify
  the trigger timing. These are tunable gameplay defaults, not recovered original
  resource parameters.
- No detection-of-hidden behavior, player/account updates, or migration-history
  fabrication is included.

Use a separately authorized maintenance process with a verified backup and the
expected database. Apply the incremental file with `psql -v ON_ERROR_STOP=1 -f
20261019_monster_behavior.sql` before starting a binary that reads the new columns.
The file owns its transaction, uses a 5-second lock timeout and a 60-second
statement timeout, and can be reapplied deterministically. Reapplying restores
this approved flag list, so do not use it to overwrite later behavior changes.
It does not restart the server or perform automatic production migration.

Regression tests:

- `go test ./internal/data -run TestMonsterBehavior -count=1`
- For actual PostgreSQL SQL execution, set `FANTASY_MONSTER_BEHAVIOR_TEST_DSN` to
  an isolated test database. The test shadows tables with connection-local
  temporary tables, checks every approved row, verifies out-of-scope preservation,
  and reapplies the upgrade to check idempotence. Without the variable it skips
  explicitly; mocked loading is not PostgreSQL execution evidence.

The transaction also asserts the scoped counts 68 / 23 / 15 / 27 after applying
its ID-and-name decisions. A missing, renamed or out-of-scope approved record
raises an error and rolls back the incremental upgrade. Investigate the data
version rather than changing the expected counts or manufacturing records.
The optional PostgreSQL test includes missing/renamed/level-changed failures
and verifies that flag changes roll back.

Validation on 2026-10-19: static full-list checks and mocked loader tests passed.
Actual PostgreSQL execution was not performed in this environment because no
PostgreSQL test instance was available. Before deployment, run:

```sh
FANTASY_MONSTER_BEHAVIOR_TEST_DSN='<isolated-test-database-DSN>' \
  go test ./internal/data -run TestMonsterBehaviorIncrementalPostgres -count=1 -v
```
