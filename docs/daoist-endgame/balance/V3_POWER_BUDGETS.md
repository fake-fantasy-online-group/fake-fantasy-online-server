# v3: useful unique and formation upgrades

Catalog version: `endgame-v3-authored`. This supersedes the earlier policy permitting weak niche items and one-line unique packages. The change is authored catalog data and validation, not a new damage formula, persistence migration, drop-rate change, or skill-tree implementation.

## Design contract

- Every one of the 1,619 uniques and 40 formations has at least five beneficial, distinct, executable EXTRA attribute IDs. Innate base stats, cards, rune ingredients, traits, curses and reserved future skill ranks do not count. A different mode of the same attribute cannot pad the count. Percentage MinAtk/MaxAtk/Atk share one runtime percentage bucket and cannot be used as three independent lines.
- Existing individual signature attributes and traits guide the packages. Supporting lines follow the class, item slot and role: a healer’s sustain budget uses mana, spirit and defense; spell attack is never advertised as increasing native healing. Shared pieces may explicitly target a subset of their permitted classes, while universally defensive pieces need five relevant benefits for every intended class.
- All unique minimum rolls must clear their declared intended-use floor. Conditional traits are deliberately excluded from that floor. A low roll must be competent before its special circumstances occur; the preserved traits provide additional distinct play value.
- The 1,080 transition items are useful leveling upgrades, not weak filler. The 449 high-value items have stronger budgets; 84 build-defining pieces emphasize existing skill/condition mechanics; six godlike pieces receive exceptional budgets. These labels describe catalog composition, not a new acquisition probability. Existing 1% native unique checks and conversion economics are unchanged.
- Formations have 20 transition, 14 high-value and six build-defining entries. Low-level basic formations use flat offensive bonuses so a starter is not maintained forever by a huge global percentage. All recipe IDs, ingredient sequences, restrictions and 28 rune definitions remain unchanged.

Current uniques have 1,077 five-benefit, 451 six-benefit, 85 seven-benefit and six eight-benefit packages. Additional curse lines bring total displayed extra lines as high as ten. Useful overlap between packages is allowed; neither artificial stat uniqueness nor a minimum line count proves good item design.

## Source anchors, cards and equal investment

`research_card_budgets.py` reconstructs native equipment and physical cards from the original SQL. `native_equipment_budget_references.json` identifies the 298 bases with actual native random pools, including ten bases with one fixed natural line and only four remaining random opportunities. The other 1,321 do not acquire fictional native pools.

Card families guided magnitudes without pretending a named card grade is a character level. For example, a +6 primary card reaches 12 points while a tier-six natural primary roll reaches six; grade-ten HP/MP cards reach 78 and MAtk cards reach 39%. High source families reach thousands of HP, hundreds of attack or primary points, but are stress references, not the normal budget to import onto every leveling item. All physical card level requirement fields are zero; descriptor tiers do not establish player-level gating. See `CARD_BUDGET_RESEARCH.md` for exact SQL provenance, source applicability, unsupported effects and current ordinary-inlay restrictions.

The numeric floor uses equal refinement and socket investment: both sides have zero refinement and zero socket investment, with the same actual innate/fixed properties. A five-natural-line yellow cannot simultaneously hold five cards; the equipment budget is shared. This test does not claim that every possible socket, refinement, party buff or full-equipment combination is dominated. Future expanded fixtures must reduce random-line opportunities when adding equal sockets to both sides.

## Targets and runtime verification

`unique_power_targets.json` is an explicit per-ID design/verification manifest, separate from shipped item state. It records the intended classes, outcome lane, reference base, reference quality and minimum ratio. It preserves original signature/identity data for review. It is not a runtime item-stat generator; the compiler reads only the authored records.

`TestUniqueAllSourceBudgetLanesThroughWornRuntime` executes the real `Compute`, `UniqueAffixes` and equipped-instance paths at 80%, 100% and 120% potency. It exhaustively compares compatible mode choices at maximal source values for the attributes affecting the declared lane, respecting four/five random-line capacity. Flat versus percentage MAtk is selected by actual resulting output. Primary-stat synergies such as VIT+HP and SPI+MP/MDef are included. These are strongest-source-lane references, not a median item or whole-item P90 statistic.

- 298 native bases: 1,896 class/potency scenarios use the same base’s actual random pool.
- 1,321 out-of-pool bases: 6,006 class/potency scenarios use an explicitly marked `provisional_anchor`. The actual candidate’s intrinsic stats and wearer level remain present, but a reference pool is borrowed. This is a provisional numerical floor, not evidence that a same-level native yellow exists.
- The current runtime level cap is 60. Source requirements above 60 and their class growth calculations are extrapolated fixtures, not playable high-level combat certification.
- Minimum fresh/reference ratios are 1.08 transition, 1.16 high-value, 1.23 build-defining and 1.38 godlike. Physical weapons add a 0.10 panel margin; that margin is a design cushion, not an attack-speed or DPS theorem.
- The test also checks that at least five useful class-route lines survive minimum potency, better potency does not worsen the lane, and converted yellow retains both the original yellow and fresh-unique benefit.

The latest unique run passed 7,902 scenarios. Its minimum fresh/reference ratio was 1.080000. The maximum converted/reference ratio was 3.356368 on base 2863’s early-level physical-defense scenario. High ratios at tiny low-level baselines are explicitly exposed; this is not a hidden “all converted items are below 2×” guarantee. The authored gains are finite, existing trait-channel caps remain active, and this revision does not introduce multiplicative per-item proc loops. Converted/full-build stacking still needs broader combat review. Representative actual runtime results are in `v3_native_runtime_examples.tsv`.

### What the lanes measure

| Lane | Measured quantity | Important limit |
|---|---|---|
| physical | Mean of final MinAtk and MaxAtk | No hit, crit, animation or skill DPS claim |
| magic | Final MAtk | No cooldown, preparation or spell-DPS claim |
| vitality | Final MaxHP | No claim about every incoming attack pattern |
| sustain / healing_sustain | Final MaxMP | Treatment sustain only; native heal amount is not derived from MAtk |
| physical_guard | HP / max(1, D−PhysRes) × (H+Def)/H | A specified incoming physical model, not universal EHP |
| magic_guard | HP / max(1, D−0.63×MDef−MagicRes) | A specified incoming spell model, not elemental resistance |

Here D = 100 + 12 × fixture character level, and H = 20 + 3 × fixture character level. The incoming magnitude avoids trivially treating modest flat mitigation as immunity. Traits remain outside these floor scores.

Attack-speed effects are real negative Attr25 percentages. Player basic-attack scheduling floors the resulting interval to 100ms ticks; adding the first small bonus and adding the same bonus to an already accelerated build can land on different breakpoints. The authoring retains individual modest speed budgets and does not advertise “+X% speed = +X% DPS.” New Attr69/70/118 lines are flat because percentage resistance on a zero baseline can be inactive. STR/INT/AGI/DEX magnitudes are at least three where used, avoiding a minimum-roll one-point bonus disappearing in the /2 secondary-stat conversion.

## Formations and compatibility

`formation_power_targets.json` records native plain-base reference fixtures for all 40 recipes. A shield with fixed natural attributes cannot be a formation base, even if its pool is otherwise suitable. Each reference must satisfy the actual plain-base rule, use an allowed slot/class, include all original rune snapshots and use a legal character `FixtureLevel` at least as high as the recipe, base and ingredient requirements. Rune ingredients are counted in the outcome comparison but never in the five-extra-affix contract. The normal one-winner-per-channel rule is retained. `TestFormationAllSourceBudgetLanesThroughActivation` passed all 40 formations and 100 legal class/base fixtures through actual recipe activation, saved card snapshots, `AstralBonuses` and equipped `Compute`. Its lowest formed/reference ratio was 1.122172. It also verifies class and level rejection and five useful recipe-only benefits.

Unchanged persisted state includes base/unique IDs, equipment UID, saved potency, rolled attributes, sockets, card UIDs/effect snapshots, refinement, washing and fusion. Existing uniques derive the new catalog bonuses on load without rerolling those fields. Neither the catalog tier names nor the balance manifests are player-save migrations. Name/lore editing is a separate field-owned change; no automatic stat operation changes those fields.

## Reproduce

From the repository root:

```sh
python3 docs/daoist-endgame/catalog/generate.py --validate-authored
python3 -m unittest discover -s docs/daoist-endgame/catalog -p 'test_*.py'
python3 docs/daoist-endgame/balance/research_card_budgets.py --check
cd server
go test ./internal/domain -run 'TestUniqueAllSourceBudgetLanesThroughWornRuntime|TestFormationAllSourceBudgetLanesThroughActivation' -count=1 -v
```

The compiler’s minimum-five/mode tests and scene display/wire tests are additional checks. None of these results certify Unity execution, an actual player database migration, production deployment, all combat matchups or every possible endgame build.
