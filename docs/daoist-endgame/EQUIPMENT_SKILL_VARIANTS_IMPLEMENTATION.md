# Equipment skill variants, first release

## Content and execution

V6 equipment carries a typed effect `{ "kind": "skill_variant", "variantId": "..." }`. The ID resolves to a closed, server-owned finite plan. A positive learned hard rank and the variant capability are required; this never grants an unlearned skill. Fixed-rank equipment procs never call the variant resolver.

| ID | Skill | Runtime change |
|---|---|---|
| sword_gale_fan | 10102 | Caster-centered 90° fan, radius 100 before existing compatible area modifiers, at most four targets. Explicit legal primary first, otherwise nearest; first receives full damage/control, others 60% damage without knockback/statuses. |
| warrior_piercing_lance | 10017 | Caster-oriented rectangle length 240, width 48 before compatible modifiers; at most four targets, each 80% damage. |
| assassin_bomb_fuse | 10223 | Positive direct hit detonates the nearest owned 10213 bomb within 160 of the impact position, before the projectile's knockback. Consumes exactly one previously paid bomb at its original location, explosion radius at least 160 and at most six enemies. Uses that trap's original/current lesser payload and offense. No new equipment root execution or recursive trap chain. |
| healer_guarded_mend | 10301 | Single target. Budget is min(current missing HP, resolved original healing). Restores 70% now and 50% after one second, each bounded by real missing health. One deposit per target; a newer cast replaces it. No full-health storage, resurrection, secondary healing modifiers, or heal-trigger loop. |
| warlock_flash_fire | 10401 | Zero preparation, existing MP cost and at least 1200ms cooldown. Existing cooldown debt is never reset by equipment changes. |

None changes an original skill table row, the hard-point cap, or the list/count of training sources. Source skill/rank fractions are resolved once before variant scaling. Standard passive/unique support continues in its existing bounded channels; an area variant becomes an area for scoped support. Same-ID duplicate equipment does not multiply the variant. Different future primary IDs for the same skill fail closed; no such conflicting pair is shipped in the initial closed list.

## Source lifetime

Finite accepted casts keep their copied skill definition, equipment variant and source identity; later wear changes do not add benefits. Subsequent casts resolve the new wear state. Existing functional-wear validation includes breakage, required attributes/class/level and winning formations. V4/V5 catalog entries stay frozen and do not acquire V6 powers.

The healing deposit records source UID and the accepted equipment revision. Source death/disappearance, target death/disappearance, any equipment revision change, or loss of the source's negotiated variant/combat capability cancels the deposit. A subsequent ordinary 10301 heal also replaces the old deposit. Re-equipping the same UID cannot revive it. Crossing scenes drops the scene-local deposit. Release rechecks current friendly targeting; the second stage is a direct bounded restore with no fresh paid cast or equipment proc.

## Negotiated contract

`endgame.skill-variants.v1` requires all of `endgame.v1`, the existing V5 growth capability, and `endgame.skill-behavior.v1`. Without it, equipment variants do not execute and no new field appears in the skill payload. Ordinary equipment stats and base skill behavior remain available.

Negotiated `SkillBehaviorPayload.variant` is an optional object with exactly `id`, `name`, `sourceUID` (decimal JSON string), `item`, `description`, `maxTargets`. Existing `area`, `inputMode`, `mpCost`, `prepareMs`, and `cooldownMs` describe actual execution. The identity contract's kind remains the original skill identity; target input must use the resolved payload. Current/next original-window prose starts with the equipment variant description. Equipment effect prose uses the original special-effect section.

`variantId` itself is catalog input, not a newly exported equipment DTO. Existing skill-rank snapshots include the variant identity and source, so the fingerprint updates on source/behavior change. First-investment preview uses a provisional hard point for variant eligibility without changing real investment.

## Verification boundaries

Focused tests cover all five variants at ranks 1/5/20/21/60/100, immutable original rows, finite geometry and edge control, exact-source wear/break/repair, capability dependencies/revocation, finite healing and dead/full-health cases, bomb owner/one-consumption bounds, and a 60-second paid firebolt script. Real strict-client JSON fixtures can be exported with `SKILL_VARIANTS_FIXTURE_DIR` while running `TestEquipmentSkillVariantsWireFixtures`.

A deterministic stationary rank-five test with 3000 MP produced 55 original casts/2145 MP spent versus 50 quick-fire casts/1950 MP spent in 60 seconds. Quick-fire's benefit is immediate release and movement freedom; it is not advertised as a stationary sustained-DPS multiplier. The pending original projectile at the window boundary is not counted as damage until it lands.

These are source/runtime simulations. They do not replace visual Unity playtests or multiplayer live-map acceptance. No UnityPlayer, production database, external deployment or push is needed for these tests.

### Completed packet upper-bound check

The legal numeric equipment/granted-skill enumerator covered 6,600 combinations, adding the profession's single reviewed variant with maximum-length source UID/item metadata as a conservative payload bound. This injection is deliberately an upper-bound assertion, not a claim that every numeric loadout is a real variant-bearing equipment combination. Largest JSON bytes for warrior/swordsman/assassin/healer/warlock: 56,510 / 56,162 / 56,485 / 57,049 / 56,936. All encoded without truncation below the 61,440-byte bound; minimum headroom is 4,391 bytes. Maximum rows are 31 (healer 30). Exported packets are additionally available to strict-client integration tests.

Frozen V5 regression fixtures for the two exact proc-damage tests and the 10301 high-equipment-rank test explicitly select content version 5; their assertions remain unchanged. V6 behavior is covered independently by real-current-catalog equipment tests.
