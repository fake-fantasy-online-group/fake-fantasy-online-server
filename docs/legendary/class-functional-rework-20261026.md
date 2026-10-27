# Learned class buffs, functional windows and uncapped area targets

Base: `eeb4956dcc67b932d1ee29f60c30cf71caa97090` (origin/main fetched before work and final validation).

## Scope

32 class powers receive functional changes; all identifiers remain stable.

| Class | Functional powers |
|---|---|
| Warrior | ZS-U01, U03, U04 |
| Swordsman | JK-D06, D07, U01, U04, U05, U06 |
| Assassin | CK-A10, D01, D07, U01, U04, U07, U08 |
| Sorcerer | SS-A30, D10, U02, U03, U04, U05, U06, U07, U08, U10 |
| Healer | YS-A29, A30, D05, U02, U07, U08 |

The authoritative player descriptions are `server/internal/domain/legendary_powers.json`. There is no broad damage doubling, random-roll/book system, new artwork, client change, protocol change, or database migration in this patch.

## Important contracts

- Permanent preparation buffs require invested points in their original skill; their native payload uses the current effective rank. Equipment does not grant a skill or a rank-one fallback.
- Native preparation remains non-consuming only while its eligible equipment is active. Respec, class change, unequip, broken equipment, death and relog are covered. Equipment-owned permanent effects are not persisted as free buffs.
- Trigger windows refresh rather than stack; no extra internal cooldown is introduced. The trigger cast does not retroactively receive its newly earned mana discount.
- SS-U02 uses a target-centered radius of 250, with no target-count limit. Eligible defensive magic is guaranteed dispelled and reconstructed with its full duration and fresh shield. Permanent/non-dispellable statuses remain protected. The same native status is stolen once per cast; the older special-weapon steal path cannot overwrite it with another victim's residual shield.
- SS-U06 is renamed 幻影解缚. A learned 幻影 may be cast under control; only successful actual movement clears control/slow. It adds no immunity window and preserves native success, cost and legal destination checks.
- SS-D05 keeps its per-hit mitigation ceiling (22% maximum HP); it benefits from permanent concentration via SS-U03 without becoming an unbounded additional defense buff.
- YS-A23 covers all eligible nearby enemies. Its multi-primary volley preserves full primary payloads while deduplicating overlapping prism secondaries in the same healing batch; it does not create quadratic repeated hits.

## Target-count removal

All area target selection caps in the affected skill/power paths are removed, including hidden 256-entry cutoffs in native/child target selection, queued damage and root hit recording. Native geometric range, valid enemy/ally/dead-target rules, primary identity, sorting, line-of-sight behavior, source ownership, no-repeat and recursion guards remain. Scope includes class powers, universal powers, special weapons, skill variants, trap explosions and allied auras.

This does not remove damage/heal amount ceilings, movement distances, projectile counts, summon counts or deliberate single-target follow-up attacks. JK-A11 still caps its restoration multiplier at three times the original amount; it no longer describes that numerical ceiling as a target selection limit.

Combat result packets are emitted per target, not as one uint16 target array. Existing slow-client queue/backpressure protection remains; this patch does not claim unlimited network load. Targeted tests cover more than 256 targets, not a full stress run.

## Validation

The task uses the repository's checksum-verified Go 1.27.2 bootstrap. Validation includes all-package compilation (`go test ./... -run '^$'`) and targeted domain/scene regressions, explicitly excluding Stress and PostgreSQL integration suites. New coverage includes permanent learned-state lifecycle and real recasts, resource stability, failed/successful controlled teleport, group fresh-shield deduplication, old special-weapon coexistence, 301-target native/child selection, and multi-primary healing/prism deduplication. See the task handoff for final command outcomes.

Final combined validation (2026-10-26):

```sh
go test ./... -run '^$'
go test ./internal/domain ./internal/game/scene \
  -run '^(TestLegendary|TestCasterIndependent|TestCasterRepair|TestHealerIndependent|TestAssassinFunctional|TestSorcererFunctional|TestTargetCountUnlimited|TestSpecialWeapon|TestEquipmentSkillVariant|TestEquipmentAura|TestAuditAuraAllocationParity|TestV6Review)' \
  -skip 'Stress|Postgres' -count=1
```

Both passed. The combined domain suite completed in 5.926 s and scene suite in 37.944 s. `git diff --check` passed. Final independent review reproduced and fixed the SS-U02 / special-weapon residual-shield overwrite; its regression and failed-teleport negative case passed. No publication/deployment was performed by this implementation task.

## Independent verification (2026-10-27)

Latest `origin/main` was fetched again and remained `eeb4956`; the rework contains it. Verification uses the pinned Go 1.27.2 toolchain.

A new negative regression reproduced a stale permanent-preparation payload when the player equipped or removed the separate named weapon 1326 while retaining YS-U02 and the same learned rank. The permanent-state reconciliation now includes its resolved named-weapon identity and buff modifiers, and the swap regression passes. The full suite also exposed one outdated combined burn-kill assertion (old 6% rather than the approved SS-U05 10% maximum-mana reward); its expectation was corrected without changing production behavior.

Passed checks after these corrections:

- All-package compile check: `go test ./... -run '^$'`.
- Original focused suite: 646 top-level tests passed (before the two follow-up corrections); focused permanent and burn-combination regressions also passed after their fixes.
- Race detector: 25 top-level learned/permanent, functional, escape, group-steal and uncapped-target regressions passed, 5.441 seconds.
- Bounded in-memory stress: all 6 `TestStress*` suites passed, 69.328 seconds. Coverage includes admission/transfer churn, loot bursts, 300 entities with up to 32 periodic statuses each, overlapping and six-party auras, and dense AI.
- Final broader run: `go test ./... -skip 'Stress|Postgres|WorstCaseLegalPackets' -count=1 -timeout=15m -json`; every executed test passed, scene package 241.351 seconds.

Limits: the broader command deliberately excludes the PostgreSQL-named integration suites, opt-in combat stress and the pre-existing exhaustive worst-case skill-packet loadout enumerator. Another 66 top-level tests explicitly skipped because their database/native-resource/benchmark fixtures were unavailable or not opted in. These are not passes. The bounded stress above is not a production network/database load test. No publication or deployment was performed.
