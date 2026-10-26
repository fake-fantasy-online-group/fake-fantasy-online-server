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

## 000096_20261021_alternate_set_weapon_drops.sql

Adds 8051 (swordsman alternate) to Pure Yang Tower boss 3060 and 8451
(assassin alternate) to boss 3062. It copies each matching main weapon's two
named drop rows and map-20600 fixed-pool probability/weight. Existing rows,
minimum-one-equipment guarantees, and `no_type_drop` remain unchanged.
The added candidates share the existing guarantee selection; this is not an
additional guaranteed item. Identity/configuration drift aborts atomically;
replaying unchanged data is a no-op. Apply this explicit upgrade, not the full
init, to an existing database; deploy/reload the server's cached drop tables
through the normal release process afterward. This file does not deploy itself.

## 98: dungeon small-drop source correction

Run `000098_20261021_dungeon_drop_source_scope.sql` once for existing databases
(it is idempotent). Retires small-source mappings without normal field spawns and
removes only their provenance-tagged granite drops. Other copied items are retained
for separate review. Do not replay 39; fresh initialization includes correction 98 before later changes.
See `server/docs/dungeon-drop-source-scope-98.md` for baseline scope and tests.

## 99–101: integrated rift, quest and pet changes

Apply the standalone upgrades in numeric order to a database already current
through 98, using a separately authorized maintenance process and verified
backup. Never run the full init against an existing database. Fresh empty-database
initialization includes all three matching bodies in the contiguous init-99,
init-100 and init-101 fragments, with no manual post-init step.

- `000099_20261022_rift_full_coverage.sql` publishes immutable rift rule 10 /
  generator 6, but never updates `game_rift_rule_active`. Deploy the matching
  paging-capable client before any separately authorized activation. Existing
  paid runs retain frozen versions.
- `000100_20261022_quest_special_weapons.sql` extends profession rewards to
  multiple items per route and adds the approved level 3/8/14 quest weapons.
  Deploy with the new list-capable loader. No completed-quest backfill or player
  item conversion is included.
- `000101_20261022_pet_merchant_supplies.sql` appends native 3020–3031 to pet shop
  1003 (seven city aliases inherit it) and beginner shops 1101/1201, without
  widening shared grocery stock 1001. Capture tools, stock/order, prices and
  effects are preserved; identity/source/slot drift aborts atomically.

The upgrade files own their transactions and can be reapplied. Fresh-init bodies
have no transaction control, so the initializer retains one all-or-nothing
transaction. Neither path deploys/restarts a server, activates rift 10, or converts
existing player items. The class-set player-data decision remains a release gate;
merging these files is not authorization to execute them.

See `server/docs/quest-special-weapons-100.md` and
`server/docs/pet-merchant-supplies-101.md` for scope and test details. Actual
PostgreSQL execution/replay has not yet been validated in this cloud environment.

## 109: Rift guardian HP

`000109_20261024_rift_guardian_hp.sql` publishes immutable Rift rule 13 and moves
only an active rule 12 selector to 13. It divides the final scaled guardian HP by
ten for new runs, with minimum one; old paid runs, other monster stats and all
rewards remain frozen. Deploy the matching server before the separately
approved migration. Do not run full init on an existing database.
See `server/docs/endgame/rift-guardian-hp-v13.md` for scope and focused coverage.

## 000110_20261025_cube_wing_progression.sql

Adds active cube-only wing rules, the native UID transform allowlist and three-slot
wing power storage; creates the one-time `幻翼初展` guide and `幻翼域使者` turn-in.
Publishes Rift14 from the existing published Rift12 or Rift13 predecessor without
rewriting old paid runs. The guide is not an admission requirement for rifts.

The JSON policies are checked in under `server/config/`. White feather unit values
are 1/5/25. First milestones 10/30/50/70/90/110/130/150 grant full fixed packages;
repeat clears grant `1 + floor(tier/10)` units, split into existing white feathers.
Repeat clears do not directly roll high feathers; the cube denomination exchanges
allow accumulation into them. A separate approved rare slot rolls one 暗之羽 (11682) at 0.5% once per eligible character and completed run, unaffected by MF or tier. No white/black exchange is added. The independent four-fix migration109 precedes this migration. Rift14 preserves the published Rift13 guardian HP divisor and other predecessor fields.

Run the focused PostgreSQL-engine check with:
`PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/cube_wing_migration_postgres.mjs`.
This checks migration SQL and rollback against reduced prerequisites, not a full
initializer or production deployment. Apply the SQL transaction before starting
the corresponding server; do not apply the fresh-init file to an existing database.

## 000111_20261025_rift_slot_first.sql

Publishes Rift15 as an exact immutable clone of Rift14, changing only its version.
The matching runtime selects a uniformly random nonempty legal equipment part,
then a uniformly random template in that part. One- and two-handed weapons share
one part; legal shields remain separate. This is not a profession-slot bonus.
Only an active Rift14 selector advances to15; older rollback selections and newer
versions remain selected. All prior frozen rules and ready rewards are untouched.
The clone preserves guardian HP divisor10, complete WingRewards (including the
independent 0.5% rare feather), reward quantities, MF and quality policies.

Deploy only as part of the authorized matching-server release. Existing databases
use this transaction after migration110; never rerun fresh init. The matching
`000000_init-111.sql` body is included automatically by numeric fresh-init order.
An operator can roll back new admissions by selecting the published Rift14, but
must keep a server capable of honoring already-admitted Rift15 snapshots; do not
rewrite their rules or rewards.

Focused validation:
`PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rift_slot_first_postgres.mjs`.
This uses a real PostgreSQL engine with reduced prerequisites and production loader
SQL. It covers exact cloning, immutable guards, invalid source/dependency/collision
rejection, transaction rollback, repeat application, old/future selectors and
saved-state preservation. It does not validate a full initializer, concurrent
migration or production deployment.

## 000112_20261026_legendary_power_policy.sql

Publishes loot4 as loot3 plus LegendaryPowerPolicy=1 (class/universal 50:50,
then uniform selection within that eligible pool). Publishes Rift16 as Rift15
with loot_version=4. Historical loot1–3 and Rift1–15 retain policy0 and the
existing mixed-pool RNG; ready grants and owned items are never rewritten.
Only active loot3 / Rift15 selectors advance. Older rollback and future selectors
are preserved. All reward quantities, MF, colour/variant odds, feather rules,
slot-first selection and other rule fields and dependencies are copied intact.

Apply this transaction after111 with the matching server. Never rerun fresh init
against an existing database. Rollback of new admissions can select loot3/Rift15,
but the server must continue to understand already-admitted Rift16 snapshots.

Focused validation:
`PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/legendary_power_policy_postgres.mjs`.
Uses PostgreSQL with reduced prerequisites and production loader SQL; checks
schema/source/guard validation, exact clone, rollback, idempotence, collisions,
old/future selectors and saved-state preservation. Not a live deployment or a
full initializer/concurrent-migration test.
