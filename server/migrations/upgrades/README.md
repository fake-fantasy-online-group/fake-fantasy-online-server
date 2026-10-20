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

## 000089_20261020_status_visuals.sql

Creates the optional `game_status_visuals` table and fills 27 source-confirmed
ID/name mappings: 22 approved missing visuals and five existing, live client
aliases. `000000_init-89.sql` contains the identical schema and seed body for
empty-database initialization. No previous migration is changed.

Run the incremental file only, with a separately authorized maintenance process,
verified backup, and the intended database:

```sh
psql -v ON_ERROR_STOP=1 -f 000089_20261020_status_visuals.sql
```

The upgrade owns one transaction, with a 5-second lock timeout and a 60-second
statement timeout. It does not migrate on server startup, restart a process,
change a published rift rule, or modify status mechanics or player data. An
absent/empty visual table leaves the server's legacy presentation unchanged.

Identity preconditions:

- Native sources use the same rank-specific name expression as `LoadStatuses`,
  and require the exact approved set of positive-duration ranks. A native
  source replaced by a custom row does not silently pass this check.
- Custom sources require exact IDs, names and rank sets, positive duration and
  the same native-replacement relationship required by `LoadStatuses`.
- Runtime 金身 (1900101) and 敏捷 (1900102) are rank 1 in
  `applyRiftEliteBuff`. Their names are checked against exactly one matching
  mechanism, ability ID and trigger in each published rift version 6 and 7.
- Missing, renamed, added or removed approved ranks abort the transaction.
  Investigate the source version instead of manufacturing status records or
  weakening the guards.

Preservation and replay:

- The mapping key is `(status_id, expected_name)`, not a global name alias.
  Other names at the same ID remain untouched, including 1154/8 金刚罗汉,
  1178/3–6, and 1036/7–8.
- Only an absent row or a wholly neutral row (`effect_name=''`,
  `use_effect_name_for_self=false`, `position_mode=-1`) is filled. The effect,
  self-title choice and position are written atomically, never merged field by
  field. Nonempty and partially configured mappings are preserved exactly.
- Each row has a postcondition: a fill must match the approved mapping, while a
  previously configured row must remain identical. Source/mapping locks keep
  those checks stable throughout the transaction.
- Reapplying without intervening changes fills zero rows: the first run makes
  each eligible row nonempty, so the next compare-and-set cannot update it.
  Replay can refill subsequently removed/fully reset mappings; do not use
  deletion or a neutral row as a persistent opt-out.
- The five live aliases retain their original self titles and inherit legacy
  anchors. In particular, 春风 remains on its original orbit anchor. Newly
  mapped effects use explicit anchors matching their native contributors where
  available; no global position normalization is introduced.

The blue glyph choice for 破防之枪 is 衰弱, never 防御下降. The approved
missing mappings use the existing effect key as the legacy self title because
that protocol field also selects the self aura. Icons, descriptions, real
status IDs, ranks, mechanics and remaining lifetimes stay independent.

No mapping is seeded for 1032 隐身, 1157 状态抗性 or 1173 伤害反射.
The 13 obsolete global aliases without an exact runtime status source
(魔镜术 and the twelve 符 names listed in the SQL) are not assigned invented
IDs or used to rename native statuses. This upgrade does not create artwork,
change tint/alpha/looping, or enable missing resistance/reflection mechanics.
