# Rift guardian HP, rule 13

New admissions on rule 13 give only the final Rift guardian one tenth of its
previous final HP, using integer division and a minimum of one. The operation
runs after the existing tier, party and guardian-class multipliers, rounding,
and signed-int32 saturation. Damage and all non-guardian stats are unchanged.
Normal monsters, blue/gold elites, trial/dungeon/wilderness bosses, rewards,
ability rules, timing, native templates and player data are not altered.

`GuardianHPDivisor: 10` is frozen with rule 13. Its omitted/zero historical value
preserves rules 1–12, including already admitted runs. A fresh run therefore
needs rule 13; reconnecting an old paid run does not change its difficulty.
Both supported spawn paths use the shared scaling helper and initialize HP,
MaxHP and Stats.MaxHP from its single result. Reconstruction starts from the
original trial/native baselines, not the already reduced entity or overlay.
Invalid nonpositive baselines remain zero; the current seeded guardian path
requires a positive trial baseline rather than creating a boss from invalid data.

## Release scope

Deploy the matching server, then apply migration 109 through the separately
authorized maintenance flow. Existing databases use
`migrations/upgrades/000109_20261024_rift_guardian_hp.sql`, not full init.
The upgrade publishes an immutable exact copy of rule 12 with only version and
HP divisor changed, copies class/ability children, and changes the active
selector only from 12 to 13. Older operator selections and newer versions are
preserved. Replay validates existing configuration and guards; no published
rule is updated. No migration or deployment was run against a live database.

## Focused verification

- All tiers 1–150, party sizes 1–5, all four classes, HP versus attack,
  integer-floor/minimum-one boundaries and int32-saturation order.
- Current approved-native guardian spawn, HP/MaxHP/cached-stat synchronization,
  no other entity changes, normal/elite population parity, legacy spawn,
  repeated population calls, delayed arrival, and persisted-run reconstruction.
- Frozen rules/rewards/loot unchanged, historical snapshot binding, invalid
  divisor/version rejection and native/trial source definitions unchanged.
- `server/tests/rift_guardian_hp_postgres.mjs` executes real PostgreSQL via PGlite
  with production rule schemas/guards and verbatim migration/loader SQL. It
  covers exact clone/child parity, missing/conflicting prerequisites, rollback,
  immutable guards, replay, and selector preservation. Its native prerequisite
  fixture is reduced; it is not full-init, live-database or concurrency coverage.

Run the PostgreSQL check with an installed PGlite module:

`PGLITE_MODULE=/path/to/@electric-sql/pglite node server/tests/rift_guardian_hp_postgres.mjs`
