# V6 authored catalogue

## Scope and content

Exactly the existing 259 native-field-dyeable bases at wear level ≤60 remain eligible. No equipment IDs, unique names, stories, native drops, rune identities, ingredients, class restrictions or level requirements change. Tiers remain 183 transition / 54 high-value / 18 build-defining / 4 godlike. Tier labels do not imply equal combat strength or change drop probability.

All 60 weapons are reviewed. Ignoring names and magnitudes, their attribute channels, conditions, typed effects and skill targets yield 55 mechanical signatures; shared coherent roles remain allowed. This is a structural diversity check, not a claim of 55 equally viable builds.

The reviewed numeric source is `v6/authored_catalog.json`. `compile_v6.py` emits the eight current runtime shards. `v6/review_manifest.json` lists all 259 decisions and the concrete changed channels. Existing transition items that already pass the strong-yellow floor keep their focused role; they are not given every advanced property merely to increase a feature count.

V6 unique effect distribution: 35 MF, 17 elemental damage, 11 penetration, 30 elemental resistance, 7 absorption, 4 elemental mitigation, 10 fixed procs and 5 skill variants. A single item may occupy multiple channels. There are no unique team auras. Non-damage benefits such as MF, resource management and survivability are not added into a fictitious DPS score.

Seven runewords provide focused team auras. Two offense choices are 倒骑观火 (fire damage +10%) and 纸驴驮残夜 (ice damage +15%); each joins the normal capped element-damage bucket and cannot stack with another aura in the same family. Five provide distinct group defenses: 城砖无字 (physical mitigation), 雨洗空符 (dark resistance), 朱鸟衔余烬 (fire resistance), 玄武不出门 (ice resistance), 空瓢济厄 (holy resistance). Their maximum-player, active-pet, team, range, damage-channel and same-family-winner rules are executable metadata. They do not give blanket resistance to every element.

## Four distinct peaks

- 问渊: heavy local weapon white damage, attack interval and basic attacks. Two-handed, no shield; its basic-only specialization does not improve spell damage.
- 泉下欠条: ≤30% health damage and a directional piercing-lance form. It grants no automatic survival or health lock; the acceptance scenario must include incoming damage and death.
- 鳞下不留名: elite hunting and a narrow native attack skill, with attack-speed/critical support. Normal targets lose the elite multiplier; native skill cooldown remains unchanged.
- 无名神席: fireball-specific skill ranks, local magic white damage, fire damage, fire penetration and fireball-only casting tempo / 30% mana-cost reduction. Ice spells and the healer do not inherit its fire specialism. Mana is actually consumed.

Element damage and ordinary conditional damage traits share the existing +100% damage cap; PvE elemental penetration caps at 30 percentage points. Their tooltip percentages must not be multiplied together. A full build should diversify into resource, tempo, skill form or defense once these channels are saturated.

Local enhancement applies only to the item's unrefined intrinsic white attack/defense. It never multiplies other equipment, affixes, cards or refinements. Fixed effects are not rerolled; each item retains at most two explicit variable lines. Skill bonuses only improve learned skills.

## Compatibility

New unique generation and newly completed runewords use content version 6. Version 0 still reads frozen V4 (1,619 unique definitions / 40 recipes). Version 5 reads frozen V5 (259 / 40). No persisted UID, sockets, card snapshot, refinement, roll, durability, lock or ownership field is migrated.

There is one explicit V5 exception: the retired team auras on 伏虎磐 (1107), 炉门客卿 (1111), and 羽端白尘 (2603) now resolve to the same numeric amount of personal physical mitigation or personal corresponding-element resistance. Both gameplay and tooltip consumers use the same `UniqueOf` definition. This is not a silent upgrade to all V6 stats; all their other V5 properties remain frozen. V4 has no typed team auras.

`compile_v5.py --check` now verifies frozen V5 shards and `v5/coverage.json`. `compile_astral.py` verifies current recipes plus both frozen snapshots. The historical V5 showcase manifest is archived under `showcase/v5/` and tested explicitly as historical. The current `showcase/` plan is separately selected and validated against V6; neither plan is permission to grant equipment.

## Verification boundaries

The actual `Compute` comparison in `TestV6UniqueSameBaseStaticBudgetRuntime` evaluates 1,710 legal same-base class/roll scenarios against enumerated compatible maximum-roll yellow lanes: minimum static ratio 1.100000. The report is `../balance/v6_unique_runtime_static.json`. These are static lane comparisons, not DPS multipliers.

Real 60-second command/cast/strike combat acceptance is independently implemented in `equipment_rebalance_v6_acceptance_test.go`. Its final outputs, including shared gear/cards, resource exhaustion, missed hits, conditions and mortality, determine the godlike combat claims. Group damage must not be reported as single-target damage, and an isolated long-cooldown hit must not be described as sustained DPS.

The real PostgreSQL fixture test `TestReviewPostgresActualSkillRuntime` loads the actual skill table with `data.LoadSkills`, then calls `ValidateEquipmentRuntimeV5`; all ten fixed-rank proc entries resolve to an executable damage or healing payload. The V5 suffix is the existing runtime API name, not the active catalogue version.
