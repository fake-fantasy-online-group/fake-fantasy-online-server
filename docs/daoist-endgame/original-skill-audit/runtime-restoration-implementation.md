# Verified native status restoration

Date: 2026-10-12. This is an additive runtime correction layer, not a claim that
all original five-profession behavior has been restored or historically certified.
The seven unresolved entries in `runtime-restoration-gaps.json` remain unresolved.

## Source boundary

The immutable `../v5/skill-original-ranks.json` and `../v5/skill-status-anchors.json`
are prior read-only loader snapshots. They are not rewritten by this change.
`runtime-restoration-gaps.json` records the source evidence and limitations.
`server/internal/data/native_skill_restoration.go` contains the explicit correction
layer. Existing migrations and databases are unchanged.

## Implemented original ranks 1–5

| Skill | Runtime correction | Original duration |
| --- | --- | --- |
| 10009 破防之枪 | Defense −30/40/50/60/70%; skill-local status 10009 | 15s |
| 10103 幻影剑 | Accuracy −30/35/40/45/50%; status 1008/11–15 | 15s |
| 10109 仙风云体 | Status 1019/16–20 gains movement-only restriction; existing defense retained | 10s |
| 10111 金缚剑 | Movement-only restriction; skill-local status 10111 | 6/7/8/9/10s |
| 10113 封法剑 | Silence via status 1003/1–5 with source duration | 6/7/8/9/10s |
| 10122 混沌剑 | Reject beneficial states via status 1029/1–5 | 10/11/12/13/14s |
| 10124 山字诀 | Status 1019/11–15 gains original attack cost −10/13/16/19/22%; defense retained | 30s |
| 10219 击晕 | Stun via existing status 1002/16,11,12,13,14 | 5/6/7/8/9s |
| 10221 狙击 | Movement-only restriction via 10111/1 with fixed source duration | 5s |
| 10224 烟雾弹 | Remove only the unsupported magic-defense modifier from status 1007/11–15; accuracy/magic-attack loss retained | Existing 60s |
| 10325 枷锁术 | Status 1009/11–15 gains attack-speed reduction 30/35/40/45/50% | 30s |

Attack speed has an important internal sign convention: positive `AttrAtkSpeed`
subtracts from player speed. 枷锁 uses positive 30–50 values and an explicit
`AttackSpeedIsFrequency` marker. Only these marked state rows invert the monster
attack interval as `ceil(baseMS × 100 / max(50, 100 − slowPct))`. A 1,000ms base
becomes 1,429/1,539/1,667/1,819/2,000ms. The old `baseMS × (1 + slowPct)` would
only reduce frequency about 23–33%, despite text promising 30–50%, and is not
used for 枷锁. Existing unmarked equipment/buff interval semantics stay unchanged.
High ranks retain the existing player 50% minimum speed, avoiding zero division;
repeated refreshes rebuild from template stats rather than compounding. Player
and monster admission keep their existing frame-quantization rules.

The two skill-local status IDs are absent from all 1,890 original state rows.
The loader rejects any future collision at any rank before adding any of its ten
new rows. Status totals therefore become 1,900.

## Movement and high ranks

`StatusDef.BlockMove` is independent of `ControlStun` and `ControlSilence`. Player
voluntary movement, monster chasing, roaming-path emission, and ordinary pet
movement consult it. Attacks and ordinary skill casts remain available, subject to
their existing admission rules. The buff snapshot exposes the existing immobile
bit. Expiration, dispel, and source reconciliation read the active status set;
there is no separate sticky movement flag. Existing forced server displacement and
teleport bypasses remain unchanged.

The existing continuation engine consumes the restored original first/fifth
values. No replacement compression or damage formula is introduced. Movement
control uses the existing 15-second control boundary, then resistance pressure;
rank-20 金缚 is 15 seconds with pressure 10. Fixed-duration 狙击 remains 5 seconds
with no invented pressure. Self-restricting 仙风云体 remains 10 seconds. Categorical
harmful-state immunity still runs before resistance.

## Verification

Passed:

- All 55 restored original skill ranks: exact duration, attribute magnitude,
  movement/silence/stun/beneficial-block distinction, retained smoke payload,
  unchanged non-smoke magic-defense penalties, attack-speed sign and real stats.
- Ranks 6, 20, 21, 100, 256, 65,536, 1,000,000: continued original slopes,
  control-duration boundaries, fixed-duration preservation, resistance pressure.
- Synthetic-ID collision rejection, including collisions outside ranks 1–5.
- `TestNativeMovement*`: actual 金缚/狙击 at ranks 1/5/20 block player
  `MoveTo` while real basic attacks and accepted skill casts still deal damage;
  monster locomotion/roaming emit no false movement path; root and silence
  compose independently; removal, expiration and cleansing restore movement.
- Independent scene acceptance exercises all 55 restored original ranks, real
  player/monster 枷锁 attack admission (including high ranks and repeat refresh),
  and downward-only root source refresh with release when the source is lost.
- Targeted race-enabled data/domain/scene regressions for restoration,
  original controls, persistent states and movement pass.
- Full `go test ./internal/data ./internal/domain ./internal/game/scene -count=1`.
  Existing network-PostgreSQL tests skip when no DSN is available.
- Actual PostgreSQL 17.11 socket-free read-only `LoadSkills` and `LoadStatuses`:
  `TestNativeRestorationActualReadOnlyLoader`. All 55 rows and referenced statuses
  exactly match the correction-layer scene acceptance fixture; the same original
  and high-rank assertions pass against actual loaded data.
- `TestV5SkillCurvesActualReadOnlyLoader` also passes all 149 battle skills,
  750 loaded profession rows, 1,900 status rows, and its wide rank sweep.

`restored-runtime-fixtures.json` is explicitly an immutable source snapshot plus
this runtime layer, not a replacement original-source export. Reproduce it with:

```sh
FANTASY_NATIVE_RESTORATION_FIXTURE="$PWD/../docs/daoist-endgame/original-skill-audit/restored-runtime-fixtures.json" \
  go test ./internal/data -run '^TestNativeRestorationExportAcceptanceFixture$' -count=1
```

Run actual-loader verification only against an idle disposable test cluster, using
the repository's trusted PostgreSQL bootstrap and single-user seed tools:

```sh
FANTASY_V5_SKILL_PG_DATA=<idle-disposable-cluster> \
FANTASY_V5_SKILL_PG_DATABASE=fresh_test PG_BIN=<trusted-postgres-bin> \
  go test ./internal/data -run '^TestNativeRestorationActualReadOnlyLoader$' -count=1 -v
```

No production database, migration rewrite, commit, push, or deployment is included.
