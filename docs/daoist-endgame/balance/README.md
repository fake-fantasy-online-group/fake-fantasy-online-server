# Itemization balance research, 2026-10-11

The research phase was read-only. A separately authorized narrow rune28 regeneration repair follows below; no database, published repository, or desktop was changed. Reference checkout HEAD: `3b3999de226a033906e4f1062e164689ba76b70d`. Concurrent authored changes were underway, so baseline unique data are frozen in the research snapshot below.

## Decision summary

The complaint is supported for fresh drops against a well-rolled, coherent yellow. It is not true of converting the identical yellow: uniques are additive overlays retaining ordinary rolls, sockets, refinement and identity. The design must test both cases. Large primary/HP lines already exceed yellow single-line maxima; indiscriminately multiplying all unique values is the wrong correction. Concentrate selected specialists into real output lanes, retain flawed/off-build items, and permit zero special traits.

## Reproduction

From the repository root:

- `python3 docs/daoist-endgame/balance/reconstruct_yellow.py` reads original SQL and rebuilds `yellow_affix_benchmarks.json`. It verifies135 actual resource-tier/type keys and16,583 weighted options, exactly matching migration39. It does not create pools for hypothetical equipment keys.
- `python3 docs/daoist-endgame/balance/compare_baseline_panels.py` reproduces the baseline comparison below. `baseline_fixture_inputs.json` freezes only the eight original base/unique definitions needed, with original source provenance, so future catalogue edits cannot rewrite the baseline.
- `source_parser.py` is an isolated read-only SQL parser. All scripts use Python's standard library and repository-relative paths; no database/network/cache is needed.
- Conditional affix-mode P10/P50/P90, weighted means, ranges, counts and first/last SQL provenance are included.

These are deterministic Python reproductions of inspected formulas, not a claim of Go integration testing or empirical player telemetry. No AGENTS.md was found under /workspace during the audit.

## Yellow gear benchmark

Yellow means five total natural/socket attribute opportunities, not five necessarily valuable affixes. Empty holes count. Natural random affixes never duplicate an attribute ID, so e.g. flat and percentage MAtk cannot coexist as two natural rolls. Counts are weighted 0:60, 1:20, 2:10, 3:5, 4:3, 5:1 for the original elite/BOSS profile before socket reservation. Ordinary monsters use a different capped profile.

The pool is keyed by resource tier and equipment type, not character requirement level. Only 298 of 1,619 eligible unique bases have the legacy pool. Never fabricate a same-level yellow for the other 1,321, or treat tier99 as level99 scaling. Same requirement level can have wildly different intrinsic equipment tiers and special promotional bases.

Per-line values below are conditional on that attribute and mode appearing; P50 is not the median whole yellow item. Percentage values are percentage points; critical values are basis points.

### Physical weapon lines: spear/sword/greatsword/dagger/throw

| Requirement / tier | Attack % range (P50/P90) | Min attack range (P50/P90) | Max attack range (P50/P90) | Primary stat range (P50/P90) | HP/MP range (P50/P90) |
|---|---:|---:|---:|---:|---:|
| Level12 / tier3 |10–18 (15/18)|1–6 (4/6)|3–18 (12/18)|1–3 (2/3)|20–36 (30/36)|
| Level24 / tier5 |10–24 (19/23)|1–10 (7/10)|3–30 (21/30)|1–5 (4/5)|20–48 (38/46)|
| Level30 / tier6 |10–27 (21/26)|1–12 (8/12)|3–36 (24/36)|1–6 (4/6)|20–54 (42/52)|
| Level54 / tier10 |10–39 (28/37)|1–20 (13/19)|3–60 (39/57)|1–10 (7/10)|20–78 (56/74)|

Hit has the same flat range as minimum attack, or percentage range as attack%. Staff adds MAtk flat or percentage with the same 10–27 / 10–39 ranges at tiers6 /10, and INT eligibility. Negative attack-speed affix Mode1 spans -1 to -tier; it raises player speed percentage. It is not precisely the same as raw interval reduction: player interval uses base/(1+speed bonus), then100ms frame quantization.

At level30 the original weapons have intrinsic attack: spear1606 6–100, sword1006 6–81, greatsword1506 12–94, dagger1806 12–38, throw1908 6–44. Staff1306 has MAtk37. At level54 spear1610 is10–174, sword1010 10–140, dagger1810 20–66, staff1310 MAtk64. This intrinsic spread alone makes one universal flat-attack prescription wrong.

### Same-level body / ring lines

Level60 body gear uses tier7/type108: primary1–7 (P50=5/P90=7), HP/MP20–60 (44/58), MDef10–30 (22/29), Def1–14 (9/13) OR10–30% (22/29), physical/magic flat resistance1–12 (8/12). The pool is shared across class bodies, but their intrinsic defenses differ: warrior9901 Def27/MDef31; sword9701 40/35; assassin9501 62/33; healer9801 24/80; warlock9601 24/109 and MAtk66.

Level60 magic ring2560 uses tier10/type107: primary1–10 (7/10), HP/MP20–78 (56/74), MDef10–39 (28/37), Hit10–39% (28/37) or1–20 (13/19), Def1–20 (13/19), flat physical/magic resistance1–12 (8/12). Its intrinsic MAtk is15. The existing unique ring instead prioritizes VIT15/HP223/Hit21, a substantial survivability package but poor magical damage identity.

## Controlled same-base panels

Fixture: actual natural class growth, allocate free points first to meet the weapon requirements and then to STR for martial classes or INT for casters; no other equipment/refinement/buffs. Yellow has five coherent lines at their individual P90. This is an aspirational strong yellow, NOT a median yellow or 90th-percentile whole-item score. Physical yellow: attack%, min attack, max attack, STR, hit%. Caster yellow: MAtk%, INT, SPI, HP, MDef. Unique potency100%, with zero ordinary rolls. Converted yellow retains those same five lines and adds unique affixes. Values are average panel attack / MAtk, not DPS or effective healing; conditional traits, hit chance and crit are excluded here.

| Level/class/base | Plain | Strong yellow | Fresh unique | Converted same yellow |
|---|---:|---:|---:|---:|
|30 warrior1606|117|181|133|201|
|30 swordsman1006|92.5|150|114.5|177.5|
|30 assassin1806|58|106.5|80|134.5|
|30 healer1306 MATK|91|118|107|138|
|30 warlock1306 MATK|101|131|117|151|
|54 warrior1610|204|338|212|348.5|
|54 swordsman1010|160|278|184|310.5|
|54 assassin1810|100|195.5|100|195.5|
|54 healer1310 MATK|160|226|167|235|
|54 warlock1310 MATK|176|247|183|257|

Existing caster1310 instead gives SPI19/MP212, mana cost -15% and cast preparation -12%. Its low MAtk is therefore partly a specialization rather than proof of universal uselessness. Existing assassin1810 has zero attack addition; +2.5% critical and conditional opening bonuses do not bridge the nearly2x attack panel gap against the coherent yellow.

## Rigorous D2 attribute translation

| D2 concept | This game's actual equivalent / boundary |
|---|---|
|Weapon Enhanced Damage|Global Attr47 Mode1 boosts the whole derived attack, not just weapon damage. D2's 200–400% weapon ED values MUST NOT be imported literally. Flat Attr47 adds to both attack bounds;29/31 affect one bound each.|
|Attack Rating|Attr19 Hit. Long-run physical hit chance Hit/(Hit+Def). Magical attacks always hit; physical criticals force a hit. Hit is therefore low/zero offensive value for ordinary spellcasting.|
|Defense|Attr13 affects physical evasion, not damage reduction. Percentage Def scales aggregate character defense.|
|Damage Reduced by X / Magic Damage Reduced by X|Attr69/70 are flat damage subtractions. MDef17 also reduces magic damage but at0.63 damage/point. These are not elemental resistance percentages.|
|Fire/Cold/Lightning/Poison Resist%|No corresponding general four-element percentage-resistance equipment channels. Do not relabel Attr70 as all-resists. D2 default75% resistance cap does not transfer.|
|Deadly/Critical Strike|Attr23/66 store physical/magical critical rate in basis points:100=1%. Crit damage fixed2x. Mode1 is silently ignored for these attrs.|
|IAS|Attr25 negative percentage increases player speed. Basic attack uses1000ms melee or500ms ranged/dagger base, divided by speed factor, quantized to100ms. D2 animation breakpoints do not transfer.|
|Faster Cast Rate|Current swift_cast trait changes PrepareMS only; it does not promise lower cooldown/global delay or instant skills.|
|Life/Mana|Attr34/36 flat or percent. VIT5 gives9HP/point; SPI61 gives9MP plus0.5MDef/point. D2 four-stat STR/DEX/VIT/Energy cannot map1:1 onto six dimensions.|
|Life/Mana stolen per hit|No generic item-stat consumer found. kill_mend/kill_mana are on-monster-kill percentage-of-max restoration, not leech.|
|Replenish Life|Equipment Attr38 accumulates Stats.HPRegen but no runtime consumer of that field was found. Status-periodic Attr38 is a separate pipeline. At audit time rune28/regen recipes lacked an equipment consumer. The narrow rune28 repair below now handles that exact rune; ordinary equipment Attr38 remains unchanged.|
|+All/Class/Tree/Specific Skills|Define separate typed gear-softpoint modifiers for future system. Existing learned skill rows must not be mutated or capped down. See semantic proposal below.|
|Crushing Blow/Open Wounds/elemental penetration/auras/charges/MF/goldfind|Do not claim support from numeric Affix alone. Require distinct server hooks and tests, or explicitly inactive future definitions.|

Existing active unique traits cover conditional damage (HP thresholds, target HP, elite, critical, basic/skill/single/area/invisibility, fire/ice/holy), physical/magic/HP-conditional mitigation, healing/ally/rescue healing, skill mana economy/preparation/range/trap radius, and on-kill HP/MP. Same-kind strongest active instance wins; different kinds sum with current channel caps (damage/healing100%, mitigation45%, cost/cast/range50%, kill restore8%). Trait IDs, caps and effective execution must come from runtime, not tooltip text alone.

## Design recommendation, explicitly proposed rather than measured

Keep broad catalogue uneven. Many items should be modest, niche, inconvenient, useful only during leveling, or have high numbers on the wrong lane for a particular build. Negative affixes are not necessary for every tradeoff: lost DPS/hit/HP/mana/cast preparation or channel opportunity is a real cost. Permit0 traits and variable stat counts. Most identities should be apparent from their fixed stat package.

An additive overlay can be initially reviewed in approximate useful-good-yellow-line equivalents: modest/flawed0.5–1.25, specialists1.5–2.5, a few chase2.5–3.5. This is a review aid, not a scalar power law; flat reduction, global percentages and threshold effects must be checked in combat fixtures. Regular primary stat1–1.5×same-tier P90; selected primary-specialist2–2.5×P90. Do not pay the entire unique budget with broad high-value primary/HP lines.

For meaningful level54 physical specialists, candidate test ranges are +60–100 flat attack OR +30–45% global attack with a smaller+20–40 flat line, trading other lanes. These values are starting candidates only; test at80/100/120% potency and on converted5-line yellows to prevent extreme stacking. A unique without ordinary rolls may beat yellow only in its intended conditions; no requirement that every unique beats yellow.

Runewords require a plain base with no natural affixes, exact socket count/order, and forbid unique bases. A2-rune word therefore forfeits the entire natural5-line yellow possibility, not merely2 natural lines. Compare the ingredient-card effects PLUS the active recipe bonus, and consider the one-active-recipe-per-channel limit. Suggested total intended-build ranges:2-rune2–3 good-line equivalents,3-rune3–4,4-rune4–5,5-rune4.5–6; tune by niche and rune accessibility rather than socket count alone. Current high-tier physical words lack enough offensive value: recipe31 gives only13%attack+6%STR plus its small ingredients, while tier9 yellow can roll attack36%, minimum18, maximum54, STR9, Hit36%.

## Future skill attributes

Represent target scope (all/class/tree/specific), explicit class/skill/tree identity, integer Levels, and availability=`definition_only` until the redesigned skill tree exists. Separate ordinary softpoints from `grants_skill`/oskill: a+class/all modifier should not silently unlock an unlearned skill. Future effective rank = learned hard rank + eligible gear softpoints; hard rank cap20, effective rank may exceed20; prerequisites and synergy contributions should use hard ranks unless individually specified. Support initial all+1 (rare+2), class+1–2, tree+1–3, specific+1–3 (rare narrow+4–5). These are semantic/design starting ranges, not live effects or claims of completed integration. Unknown skill identifiers must fail catalogue validation, not silently disappear.

## Out-of-pool, high-level authoring guidance

Do not label an invented capped-tier proxy a same-level yellow benchmark. For bases without a legacy pool, use their actual intrinsic values and mark the candidate as provisional. The current runtime level cap is 60, so resource entries requiring higher levels do not establish a tested endgame progression curve.

As provisional caster starting points, a narrow specialist can add flat MAtk equal to 15–30% of the actual base's intrinsic MAtk; a rare chase can test 30–45%. These ranges do not authorize also stacking a maximum global MAtk percentage. Pure MP is a clearer off-build utility line than SPI because every SPI point also supplies 0.5 MDef. Illustrative large off-build MP ranges at levels 60/80/100 are 300–600 / 500–900 / 700–1,300; corresponding HP candidates are 200–400 / 300–550 / 400–750. These are design proposals, not measured yellow percentiles or shipping guarantees.

Concrete physical example: item 15043, 紫炎电戟, requires level 108, uses resource tier 29, and has intrinsic attack 590–1,082 (average 836; `server/migrations/000000_init-24.sql:10634`). There is no tier-29 yellow pool. In the controlled warrior growth fixture, allocating all free points to STR produces plain average attack 1,056. An overlay of +160 flat attack and +20 STR adds 170 average attack, reaching 1,226 (+16.1%). Its +300 physical crit adds 3 percentage points of 2× critical chance. This is a meaningful provisional offensive package; the earlier +24 attack/+9 STR added only about 2.7%. If the item has no meaningful offensive trait, +220–260 flat attack is a stronger candidate to test instead. A -15 MDef curse means approximately 9.45 additional incoming magic damage per hit before integer rounding, not 15% vulnerability. Review that flat sacrifice against actual encounter damage rather than its tooltip size.

## Primary D2 sources and lessons

1. https://classic.battle.net/diablo2exp/items/normal/ugloves.shtml — Bloodfist emphasizes life/minimum damage/hit recovery, Magefist fire skills/cast/mana, Frostburn a large mana pool. Their distinct stat priorities support unique identity without a mandatory proc per item.
2. https://classic.battle.net/diablo2exp/items/exceptional/uarmor.shtml — Vipermagi caster package, Shaftstop physical defense package, Guardian Angel paladin/block/resistance-cap package. Similar slots with different recipients and missing strengths.
3. https://classic.battle.net/diablo2exp/items/elite/uwands.shtml — Boneshade emphasizes several specific bone skills; Death's Web emphasizes poison resistance reduction. Large modifiers are useful only to the matching build.
4. https://classic.battle.net/diablo2exp/items/magic/pre.shtml — Affix pools and skill/class/tab distinctions; large weapon ED is on different semantics from this game's global attack percentages.
5. https://classic.battle.net/diablo2exp/items/magic/suf.shtml — Life/mana steal are physical damage-linked, not universal spell sustain; distinguish flat DR/MDR from elemental resists.
6. https://classic.battle.net/diablo2exp/items/runewords-110.shtml — Bramble poison specialization, Lawbringer support without broad weapon ED, Spirit spellcasting, Grief physical attack. Borrow the differentiated priorities rather than directly copying headline numbers. This is historical Arreat Summit data, not a claim to enumerate current D2R recipes.
7. https://classic.battle.net/diablo2exp/skills/basics.shtml — Hard skill cap20; item bonuses can exceed20; hard-point synergy distinction; staffmods vs all/class/tree and oskill semantics.
8. https://classic.battle.net/diablo2exp/basics/resistances.shtml — D2's default75% elemental cap and separate flat-reduction/resistance/absorb stages, which do not match this game's69/70.

## Exact local source entry points

- `server/migrations/000000_init-39.sql:147–299`: table schema, source card pool, lognormal scoring, normalization.
- `server/migrations/000000_init-44.sql`: monster color profiles.
- `server/internal/domain/equipment_roll.go:59–110`: actual roll and no repeated attr.
- `server/internal/domain/equipment_attributes.go`: shared5-line/socket budget.
- `server/internal/game/scene/inventory.go:434–465`: quality colors.
- `server/internal/data/gamedb.go:850–985`: actual intrinsic stats from ov_arm; game_equipment strength/vitality etc are old requirement imports, NOT intrinsic bonuses.
- `server/internal/domain/starting.go`, `level.go`, `stats.go`: class starts/growth, six-dimension derivation.
- `server/internal/domain/equip.go:439–533,562–691`: additive layers and percentage semantics.
- `server/internal/domain/unique_catalog.go`: ApplyUnique retains original state; UniqueAffixes scales the added package.
- `server/internal/domain/unique_effects.go`, `server/internal/game/scene/unique_effects.go`: trait semantics/caps/actual hooks.
- `server/internal/game/combat/strike.go`, `damage.go`, `combat.go`: guaranteed magic/critical hits, fixed magic defense, hit ratio.
- `server/internal/game/entity/entity.go:833–862`: actual player basic attack timing.
- `server/internal/domain/status.go:1070–1088`: status resistance90% cap.
- `server/internal/domain/endgame_runes.go:117–225`: plain-base/exact-order rules and one-winner-per-channel.

## Verified local lore anchors

Prefer original descriptor of each base over generic invented deity ownership. `ov_desc.desc_` has concrete artifact histories; the generated writing_context JSONL files expose this with source lines. Examples include healer15123 and15127, swordsman15006, assassin15073, and the class variants of华天玉袋. New unique stories should be labeled authored extensions when they go beyond those descriptions.

Class mentor identities are supported by original map/NPC rows in `000000_init-0.sql:7259–7263` and class-quest mappings `000000_init-32.sql:50726–50730`. Quest anchors include five medicines407, candles/souls411–414, jade casket419, lost fan420, wine pot421, peony422, painting423, and the three array objects433–435 leading to the five-thousand-year calamity436 (`000000_init-32.sql:51196–51249`). These are grounded themes, not authorization to claim every new object belonged to an immortal. Avoid equating许栖岩 with山石先生 simply from reused NPC identifiers.

## Narrow rune28 regeneration repair

`server/internal/game/scene/endgame_rune_regeneration.go` executes only stored rune28 (`1900028028`) Attr38 Mode0 snapshots as flat HP per two-second online pulse. It does not enable ordinary equipment/card Attr38 or reinterpret it as a maximum-HP percentage. The timing uses the existing two-second recovery cadence; it does not include the sitting-recovery bonus. A full interval is required after entering/equipping/reviving; there is no offline/missed-frame catch-up or resurrection. Valid level/class/sex/stat/durability/slot/UID/snapshot eligibility is checked. Each pulse is bounded to missing HP, aggregate arithmetic saturates, and inventory/UID/card values are untouched. Tests live in `endgame_rune_regeneration_test.go`.

Card and socket tooltips use each stored snapshot's amount and state the two-second cadence. Regeneration remains active when a formation is absent or suppressed, because the rune's standalone effect does not depend on winning a formation channel. The ordinary Attr38 formatter remains unchanged.

The lifecycle regression fix compares exact eligible rune source identities and amounts every world frame. The shared stat-refresh boundary also invalidates a pending clock when eligible sources change, so actual unequip/re-equip, quick swaps, or requirement loss/recovery within one frame cannot inherit the old deadline. Death and both revival paths clear the timer explicitly. Unrelated stat refreshes leave a stable source's cadence intact. Rune20 card/socket tooltips use the same positive player-speed display convention as equipment while preserving the negative stored interval-percent value.

Verification commands, from `server/` with the Go toolchain on `PATH`:

```sh
go test -race ./internal/game/scene -run 'TestAstralRune(Regen|Speed)|TestAstralEveryRuneEquipUseAndRelogRequirements|TestAstralCatalogAndChannelReasonsAreAuthoritative' -count=1
go vet ./internal/game/scene
go test ./internal/game/scene ./internal/domain ./internal/protocol -count=1
```

On 2026-10-11 after the lifecycle fix, the focused rune/equip/relog/catalog race run passed (2.351s), scene vet passed, and the complete scene suite passed (3.822s). Coverage includes actual same-UID unequip/re-equip immediately before a scheduled pulse, same-frame quick swaps, equal-amount source replacement, transient eligibility loss, ordinary and skill revival, stable-refresh cadence, snapshot tooltip values, and rune20's displayed speed sign. The catalogue owner also reported a passing complete server `go test ./...` after all 1,619 authored entries were compiled; the earlier 1806/1810 specialist fixture failures have been resolved. The current converted-yellow matrix limit is documented below, separately from the frozen pre-redesign baseline above.

## V2 executable focused gates and potency matrix

`server/internal/domain/endgame_balance_fixtures_test.go` reads the current explicit authored rows and frozen original SQL fixture inputs. It uses the real Go `StartingBase`, `GrowthAt`, `Compute`, and `EvaluateUniqueTraits`, with free points first meeting source equipment requirements and then concentrated into STR/INT. It separately computes fresh uniques and the identical strong yellow after retaining its affixes and adding the unique overlay.

Designated offensive examples1610/1010/1806/1810/1310 must fulfill their stated specialty against that same-base coherent five-line P90 yellow at100% potency. This gate deliberately does not demand that every niche/support piece beat an offensive yellow. The five-rune physical example31 also includes all ingredient rune effects before comparison.

`v2_specialist_matrix.tsv` records80/100/120% potency, intended and off-condition contexts, condition HP values, effective damage-channel bonus, actual HP/MDef and flat-reduction fields. In the reviewed fixture snapshot the highest converted/yellow ratio is1.927;1806's low-HP channel was reduced25%→20% and1810's elite channel22%→18% while retaining their fresh-drop purpose. There is no hidden per-instance discount, existing-affix deletion, or inventory rewrite.

Reproduce the executable log from `server/`:

```sh
go test ./internal/domain -run 'TestAuthoredOffensive|TestFiveRune' -count=1 -v
```

Limits: these are attack/MAtk-based outcome proxies with explicitly selected conditional multipliers, not full DPS, hit/critical probability, target armor, time at low HP, healing throughput, or full-equipment simulation. The caster off-condition still uses its normal spell, so a general skill-only bonus can remain active. The yellow is a strong coherent reference rather than an ordinary drop percentile. Conversion intentionally remains premium; stacking with a complete loadout and shared channel caps, encounter behavior, and player-driven item choices still require practical gameplay balance testing. Post-60/source-tier99 items cannot claim same-base yellow or live high-level validation where that pool/progression does not exist.

## Audited new-affix consumer boundary

New authored active properties use only server-executed channels: six primary attributes feed real derived attack/defense/resources; attack/MAtk/hit/crit and flat physical/magic reduction settle in `game/combat/strike.go`; MDef is flat magic mitigation; defense participates in avoidance; status resistance is checked in `game/scene/status.go`; movement uses the player's computed move speed. Player attack speed uses Attr25 Mode1 and `Entity.DidBasicAttack`/`Stats.PlayerAttackSpeedPercent`; raw Mode0 milliseconds do not affect player cadence.

Attr38 ordinary equipment regeneration has no general consumer, so it is excluded from new unique/formation affixes. The unchanged轸 rune snapshot gets its own tested two-second scene path, with no global ordinary-regen change. Attr40 MaxWeight is a displayed inventory statistic with no server capacity/encumbrance rule and is also excluded. Crit23/66 percentage mode is ignored by the existing pipeline and rejected. Merely changing a `Stats` field is not sufficient evidence that gameplay consumes it.

Per-skill trait targets include semantic evidence (preparation ranges, shape, healing/trap category), not only valid names. Instant skills cannot advertise cast-time reduction; pure healing cannot advertise damage; trap size requires the actual trap channel; group healing withHurtType0 cannot use magic-only economy. Generic elemental/healing/cast traits also require at least one native path among the item's allowed classes.
