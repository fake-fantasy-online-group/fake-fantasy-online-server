# Native yellow and card strength anchors

Research date: 2026-10-11. Original SQL and current server consumers were read locally. No database, client files, published repository, or assets were changed. This document follows the latest requirement: every unique should be an upgrade for its intended use, with at least five additional useful, executable stat lines. The earlier README's permission for deliberately underpowered uniques is superseded for this revision.

## Comparison contract

Compare equal refinement and socket investment. The natural-yellow reference is an actual compatible native random-affix pool, with the same base's intrinsic stats and class requirements. Unique power is an additional layer: both a fresh unique without favorable natural rolls and conversion of the identical yellow must be tested at 80%, 100%, and 120% potency.

Card values are useful independent strength anchors. They do not authorize comparing a fresh, empty unique with a yellow holding five supreme cards, nor claiming that every socket permutation is bounded by a natural-roll benchmark. Five total natural/socket opportunities are shared; a natural five-line yellow has no remaining socket capacity. Existing fixed natural lines reduce the random-affix allowance.

## Reproduction and deliverables

From the repository root:

```sh
python3 docs/daoist-endgame/balance/research_card_budgets.py
python3 docs/daoist-endgame/balance/research_card_budgets.py --check
python3 docs/daoist-endgame/balance/reconstruct_yellow.py
```

- `native_card_inventory.json`: all 2,683 physical source cards, exact names, raw effects, level fields, source part flags and SQL line provenance. It distinguishes stat effects with a current general equipment consumer from unsupported/conditional effects.
- `native_card_family_benchmarks.json`: unweighted source ranges by actual named family and named rank, quality or stars. These are not drop percentiles.
- `native_equipment_budget_references.json`: all 1,619 eligible equipment bases, original intrinsic stats, wearer and six-dimensional requirements, slot/type/class/sex, fixed natural affixes and native-pool availability. Exact-level native alternatives and strictly lower-level anchors are separate fields.
- `yellow_affix_benchmarks.json`: existing exact migration-39 conditional affix/mode distributions. Magnitudes must be multiplied by `value_sign`, especially attack speed.

The three new generated JSON files use compact deterministic encoding without dropping source information; the narrative and tables here remain the human-readable review surface.

## Native-yellow coverage and scaling

There are 135 actual tier/type keys, 16,583 weighted options, and 298 eligible bases with a pool. Although migration 39 permits source tiers 1–17, the actual equipment keys use tiers 1–10. Among the 298 bases, 288 have no fixed natural line; ten have one fixed natural line and can roll at most four additional lines.

The wearer-level span is 0–80, not a uniform tier-to-character-level curve. Native weapons stop at requirement 54; bodies reach 80, shoes 70, hats 66, necklaces 62, rings/gloves/shields 60, bags 36, faces 18. The runtime level cap remains a separate constraint on actual play.

Only 50 of the 1,321 out-of-pool bases have any exact wearer-level, matching-type/class native alternative. Another useful metadata field provides a strictly lower-level matching-type/class anchor, but neither creates a missing pool or validates an extrapolated endgame. Never interpret source tier 99 as a smooth character-level scale.

Natural random selection forbids repeated attribute IDs, including alternative modes. Flat and percentage MAtk cannot both be natural rolls. Modes share the attribute's weight rather than competing as separate equally common attributes. P90 values below are conditional on that affix and mode, not the 90th percentile of an entire yellow.

| Source tier | Weapon attack % P90/max | MinAtk P90/max | MaxAtk P90/max | STR/INT P90/max | HP/MP P90/max |
|---|---:|---:|---:|---:|---:|
| 1 | 12/12 | 2/2 | 6/6 | 1/1 | 24/24 |
| 3 | 18/18 | 6/6 | 18/18 | 3/3 | 36/36 |
| 6 | 26/27 | 12/12 | 36/36 | 6/6 | 52/54 |
| 10 | 37/39 | 19/20 | 57/60 | 10/10 | 74/78 |

INT is available on staff and compatible nonweapon pools, not ordinary martial weapon pools. Staff MAtk can be flat or percentage, with the same displayed numeric limits as attack %. Low-level casters can get more output from flat MAtk, so a percentage-only reference is insufficient. Negative percentage attack interval is beneficial; the tier-10 maximum is -10, not +10. Its real value depends on the player's frame-quantized attack interval.

For level-60 body gear, source tier 7 gives primary max 7, HP/MP max 60, MDef max 30, Def flat max 14 or percent max 30, and physical/magic flat reduction max 12. Class bodies have different intrinsic defenses. Level-60 magic ring 2560 instead uses tier 10; its own MAtk 15 is intrinsic, while its random ring pool has no MAtk roll. Role-specific intrinsic values and usable affixes matter more than one scalar level formula.

## Cards: named grades are not character levels

All 2,683 physical `ov_card` rows have `level_need = min_level = max_level = 0`; every corresponding `ov_item.level_need` is also zero. The descriptor resource level is a source category/rank. For example, 魔牙+6 through 魔牙+17 are all descriptor level 18. Reading this as a level-18 equipment restriction is false.

| Actual named grade | Primary, Def or Hit | MinAtk | MaxAtk | MDef | HP/MP | 魔牙 MAtk % |
|---|---:|---:|---:|---:|---:|---:|
| +1 | 1–2 | 1–2 | 3–6 | 10–12 | 20–24 | 10–12 |
| +3 | 5–6 | 5–6 | 15–18 | 16–18 | 32–36 | 16–18 |
| +6 | 11–12 | 11–12 | 33–36 | 25–27 | 50–54 | 25–27 |
| +10 | 19–20 | 19–20 | 57–60 | 37–39 | 74–78 | 37–39 |
| +17 | 33–34 | 33–34 | 99–102 | 58–60 | 116–120 | 58–60 |

Families are 武力/仙魂/根骨/迅捷/巧技/神气 for the six primary stats; 盾 for Def; 精准 for Hit; 锐利/锋刃 for attack bounds; 天护 for MDef; 体/魂 for HP/MP. Source flags restrict different families to different intended slots, recorded per card. Scalar grade-n primary cards provide up to twice a tier-n natural primary roll, whereas HP/MAtk caps closely match the same numerical rank's yellow cap. That makes grade-matched cards a useful explanation for substantial unique primary lines without importing unrelated endgame maxima.

### Higher source families

- 邪月血珠, 残/次/凡/良/精/珍: HP 500/800/1,200/1,600/2,000/2,500. Source flags are hat/bag. 至尊 gives HP 4,000 with different flags: face/necklace/glove/ring/body/shoe.
- 至尊烈阳战珠: physical attack +400, MAtk +500, Def +320, or MDef +320, with source hat/bag flags.
- 幻玉碧沙, six quality grades: movement +15/20/25/30/35/40%, source shoes.
- 鬼魄碧晶 physical critical and 烟袅魂玉 magical critical: supported flat-mode source rows at 100/200/300 basis points, meaning 1/2/3 percentage points. Intermediate quality rows using other semantics are preserved as raw records rather than silently promoted to live effects.
- 高级八卦符卡: HP +300, STR/INT +50, VIT +45, flat physical/magic reduction +60, MAtk +40, or maximum attack +40. Descriptor level 99 does not mean a wearer level 99 gate.
- Orange poker source cards at five stars include HP +2,250, primary +180, MAtk +240, attack +180, flat reduction +240, or 10% attack/MAtk/HP variants. These large source effects are stress anchors, not a normal early-level budget or evidence that players can obtain them.

There are 1,872 physical source cards with at least one supported unconditional general-equipment stat under this audit's conservative definition; 2,655 cards have one nonzero raw effect and 28 have none. Conditional effects and unknown attributes are not guaranteed gameplay effects. Ordinary equipment HP-regeneration and weight lines do not count as meaningful new unique lines; the specific rune-28 regeneration hook is separate.

## Runtime caveats affecting honest comparisons

- `LoadItems` identifies physical cards with `index == item_id`; joining every row with the same `item_id` would erroneously combine affix definitions into a huge card.
- `loadSocketAffixes` snapshots the physical row. `CardInstanceState.StaticAffixes` passes only op 1 / probability 100 effects into equipment computation. Unknown attributes are ignored; critical percent mode is ignored; ordinary HP regen has no general execution path.
- Ordinary `onInlayItem` currently checks item instance, socket/recipe/material/coin conditions, but does not enforce the source card's part flags, min/max level or repeat field. The JSON records source applicability honestly and does not claim these are live enforced restrictions. Repairing this unrelated inlay behavior was not part of this research.
- `gamedb.go` loads intrinsic MinAtk/MaxAtk/Def/MAtk/MDef/Hit/MaxWeight and six primary stats. Raw `ov_arm.p_resist/m_resist` are not included in the current `EquipDef.Bonus` constructor. A balance simulator must not grant them simply because they appear in SQL.

## Actionable authoring and verification rules

1. Use at least five distinct beneficial active stat attributes, preserve coherent item identity, and add additional meaningful lines for selected higher tiers. Negative drawbacks and reserved future skill definitions cannot count toward five.
2. For native pools, choose the strongest feasible maximum-value natural affix package for the item's declared lane, enumerating mode alternatives and the remaining four/five natural-line capacity. This is a deliberate deterministic upper envelope, not a random-drop percentile.
3. Execute actual `Compute` on the original intrinsic stats and legal class-growth fixture. At minimum potency the intended outcome must exceed the yellow lane reference by an explicit margin. Also evaluate 100%/120% and the identical yellow plus the unique overlay. Never use conditional traits to hide an ordinary weak core floor.
4. Physical offense must earn its attack floor; caster offense must earn its MAtk floor. Defensive and resource pieces can earn their advantage through HP, MP or an explicitly modeled incoming-damage scenario. A guard scenario is not universal EHP; MP alone is not evidence of movement speed, healing throughput or damage.
5. Fix irrelevant padding: ordinary Hit/physical crit are not caster damage; magic crit is not martial damage. Primary conversions are STR/INT/AGI/DEX/SPI divided by two for their derived outputs, VIT gives 9 HP, and SPI gives 9 MP plus half a point of MDef. Source values must be valued by these consumers.
6. Use same-type/class exact-level alternatives only when they exist. Record every fallback honestly as a provisional design anchor. For high-level bases combine native envelopes with actual intrinsic equipment scale and appropriate higher-card families; do not invent a measured yellow progression where none exists.

## Source entry points

- `server/migrations/000000_init-39.sql:147–299`: native pool construction, equal attribute/mode allocation, exact option count.
- `server/internal/domain/equipment_roll.go:59–110`: natural roll selection and attribute-ID uniqueness.
- `server/internal/domain/equipment_attributes.go`: fixed/random/socket five-opportunity capacity.
- `server/internal/data/gamedb.go:776–819,961–964,1123–1169`: physical cards, loaded intrinsic stats and card snapshots.
- `server/internal/domain/item.go:531–560`: raw card effects and unconditional affix extraction.
- `server/internal/game/scene/socket.go:153–218`: ordinary inlay checks and stored snapshots.
- `server/internal/domain/equip.go:439–533,562–691`: live attribute consumers, layering and percentage buckets.
- `server/internal/domain/starting.go`, `level.go`, `stats.go`: actual starting stats, class growth and derived coefficients.

Every individual source card and base in the JSON includes its original SQL filename and line number. Source card availability and high-level live play remain explicitly unverified.

## V3 independent runtime verification

`server/internal/domain/endgame_card_budget_integration_test.go` executes the compiled catalogue through `UniqueAffixes` and the actual worn-equipment `Compute` path. It independently enumerates the strongest compatible natural-affix mode combinations for each declared outcome lane. It does not trust a generator's estimated result or recorded ratio. Source fixtures also retain unknown-innate flags and actual `ov_excesuit` membership, so a formation cannot obtain a fictitious plain base by discarding those restrictions.

```sh
cd server
go test ./internal/domain -run '^(TestUniqueAllSourceBudgetLanesThroughWornRuntime|TestFormationAllSourceBudgetLanesThroughActivation)$' -count=1 -v
```

Verified on 2026-10-11:

- All 1,619 uniques passed 7,902 class-by-potency scenarios, covering 80%, 100% and 120%. Of these, 1,896 are native-pool scenarios and 6,006 are explicitly provisional-anchor scenarios. The lowest fresh/benchmark lane ratio was 1.080000.
- Every intended class has at least five beneficial, executable, class-relevant extra lines at minimum potency. Off-route Hit/physical critical/physical attack do not count for a caster route; magic attack/critical do not count for a martial route. The three initial shared-piece padding cases were corrected with an additional useful line.
- Converted yellows retain their original natural benefits. The largest converted/benchmark lane ratio was 3.356368 on low-level base 2863 in the modeled physical-guard lane. This is reported, not hidden behind an arbitrary pass cap. It is a reason for practical gameplay review, and it is not a universal damage multiplier.
- All 40 formations passed actual activation and outcome checks across 100 declared class/base scenarios. The minimum formed/yellow ratio was 1.122172. Checks use the actual maximum of base, recipe and ingredient-rune level requirements; they retain class and below-required-level blocking.
- Formation ingredient snapshots are equipped and computed separately from authored recipe extras. Only the recipe's own beneficial lines count toward five. Initial recipe-22/36 references to shield 1109 were rejected because its fixed +72 physical-reduction line makes it nonplain. Real plain bases replaced those references; ten recipe floors were retuned at the legal character levels before the passing run.
- The two focused runtime tests also passed under `-race`; `go vet ./internal/domain`, source-fixture `--check`, and `git diff --check` passed. These are explicit local verification results, not a deployment claim.

`v3_native_runtime_examples.tsv` contains 90 representative unique outcomes, including physical and caster weapons, armor/utility, an offensive magic ring and a high-level provisional case. `v3_formation_runtime_matrix.tsv` contains all 100 formation outcomes, including the ingredient-only score.

### Exact limits of those passes

The declared lane is an intended-use test, not simultaneous dominance over every possible yellow specialization. Attack and MAtk are panel metrics, not complete DPS. Physical/magic guards use an explicitly chosen incoming-hit scenario, not universal effective health. `healing_sustain` measures MP capacity; native healing amount is target-health-based, so this is not a claim of larger individual heals. Conditional traits, basic-attack frame breakpoints, encounters, full equipment sets and player behavior require separate gameplay validation.

The 1,321 out-of-pool bases retain provisional labels even when their tests pass. A source-anchored affix envelope applied to their real intrinsic values is a design check; it does not create a native random-drop pool, establish real same-level yellow gear, or prove live progression beyond the runtime level cap. The comparisons use no refinement, no ordinary socket cards, no other worn gear and no active conditional traits. Equal-investment comparisons with sockets and full builds remain distinct tests.
