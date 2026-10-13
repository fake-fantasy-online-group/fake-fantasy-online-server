# Persisted skill configuration (migration 62)

## Authority and loading

Migration `000000_init-62.sql` replaces the former Go identity catalogue,
per-skill growth-policy map, and `native_skill_restoration.go` post-load patches.
There is no production embedded catalogue, process-global installation, or
missing-migration fallback.

`LoadSkills` keeps its existing signature. It first loads the ordinary native
skill rows and additive status applications, then calls
`LoadSkillProgressionCatalog(ctx, querier, skills)` and
`InstallV5SkillProgression(skills, catalogue)`. Installation validates every
original rank 1–5 anchor and representative continued ranks before attaching
one immutable snapshot. `SkillTable.ProgressionCatalog()` exposes that snapshot
for equipment validation and scene callers. `SkillDef.Progression` and
`StatusApplication.Progression` are runtime-only (`json:"-"`); copies retain the
pointer and all mutable catalogue slices are copied at API boundaries.

Startup order: ordinary skills → skill configuration validation/attachment →
equipment/endgame catalogue using the same skill snapshot. Scene formulae and
status scaling consume the already attached snapshot. ID-only domain APIs take
a required catalogue argument; invalid/missing startup configuration fails.

## Persisted fields

- `game_skill_progression`: 149 combat-skill memberships, branch, and whether to
  read native prerequisites. Name, profession and unlock level are taken from
  the existing first-rank native skill row rather than duplicated columns.
- `game_skill_prerequisite_overrides`: one explicit historical restoration,
  skill 10311 → 10321. The other 148 prerequisite lists use the original loader.
- `game_skill_growth_policy`: one bounded typed row per member; attribute/caps
  and exact affected attack IDs. No formula/script text is accepted.
- `game_skill_synergies`: 128 ordered source/target/rank-mode/effect/coefficient
  edges. Runtime validation checks references, profession boundaries, modes,
  supported effect names, coefficients and maximum source cardinality. Native
  (`effective_raw`) edges must use the original payload with coefficient zero.
- Existing `game_skill_statuses`: 35 previously code-authored applications.
- Existing `game_custom_statuses` and `game_custom_status_affixes`: 30 status
  rows (10 new rows and 20 explicit native replacements), including movement
  restriction and attack-frequency semantics. Generic typed loading replaces
  native rows only when `replace_native` explicitly requests it.
- `game_skill_configuration_manifest`: expected coverage for membership,
  synergies, prerequisite overrides and migration-62 status/application rows.
  Accidentally deleting a member and its policy cannot silently send that
  original skill back to legacy rank handling. Intentional catalogue changes
  must update their manifest in the same reviewed transaction.

The migration seed retains original identity evidence for regression auditing;
production SQL only persists the fields above and the loader does not read the
seed text. Test snapshots live in `internal/testfixture/catalog`, imported only
by tests. Domain test helpers are `_test.go` files.

## Compatibility and rollback

Original `gamedata.ov_skilldesc`, `ov_exceptdesc` and `ov_exceptdetail` rows are
unchanged, including all rank 1–5 values. Mathematical continuation remains Go.
The migration does not write characters, skill allocations, points, equipment
instances or player saves. Existing IDs and original state semantics remain.

Replaying migration 62 does not overwrite administrator policy values or restore
deleted synergy/affix children. Missing required coverage fails loading rather
than repopulating from Go. SQL checks reject negative caps, and nonzero
continuation caps below the original rank-5 anchor (the intentional attribute
cap at rank 4 remains valid).

Deploy migration 62 before this server binary. A database-only rollback is not
compatible with this binary: it deliberately refuses missing configuration.
A previous binary cannot simply be restarted against all migration-62 rows:
its old custom-status loader treats the 20 explicit native replacements as
collisions. Restore the pre-migration configuration backup together with the
matching binary, or prepare a reviewed transactional reverse migration that
removes only migration-62 application/status/affix rows (identified by
`configuration_version=62`) before starting that older binary. Child affixes
must be removed before their status parents. New unused catalogue tables and
columns can remain, but their presence is not a complete rollback. No automatic
SQL down migration is supplied. Do not delete character skills or reset player
skill points to roll back this configuration migration. Removing new status
rows while this binary is running would invalidate status references.

## Verification

- `tests/sql/validate_skill_configuration.py` creates a brand-new isolated
  PostgreSQL cluster, loads migrations 0–59 then 62, and tests first/repeated
  application, exact original/player-row preservation, expected coverage,
  administrator configuration preservation and cap constraints. It never
  targets a running or existing user database.
- `TestSkillConfigurationDatabaseGolden` uses actual PostgreSQL query results
  from `LoadSkills` and `LoadStatuses`. A snapshot captured before refactoring
  verifies all 149 identities and policies, 128 edges, and 1,490 effective
  skill/status payloads: every skill at ranks 1–5, 20, 21, 60, 255 and 65,536.
  All numerical values and resolved effects match. Only legacy omitted-certain
  versus explicitly certain status chance is normalized in this comparison.
- `TestSkillConfigurationDatabaseRejectsMissingAndUsesConfiguredPolicy` verifies
  absent manifest/member/policy/synergy/restoration and invalid cap rejection,
  and proves that a changed SQL policy creates a new independent snapshot.
- Constructor tests cover incomplete inputs, malformed references, cycles,
  invalid caps/attributes, defensive copying and JSON exclusion.
- `tests/original_skill_source_audit.py` reads migration 62 rather than a removed
  Go catalogue and retains its 750 raw-row hashes, 745 original-rank identities,
  and 58 exact native passive relationships.

Commands (trusted local PostgreSQL tools must be configured):

```sh
python3 tests/sql/validate_skill_configuration.py --work-dir /tmp/skill-config-new-check
FANTASY_SKILL_CONFIG_PG_DATA=/tmp/skill-config-new-check/data \
  go test ./internal/data -run '^TestSkillConfigurationDatabase' -v
go test ./internal/domain ./internal/data ./internal/game/scene ./cmd/gameserver
python3 tests/original_skill_source_audit.py
```
