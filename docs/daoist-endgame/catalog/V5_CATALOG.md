# V5 unique and formation content

Implemented 2026-10-12. This page records code and focused runtime checks, not deployment or full combat balance approval.

## Scope and identity

New drops and awakening use exactly the approved259 BaseIDs. All original unique names, base IDs, sex/profession requirements, intrinsic equipment stats, and lore remain. Tiers are183 transition,54 highvalue,18 builddefining,4 godlike. Ordinary equipment definitions, ordinary drop sources and the previous1619-base salvage scope remain unchanged by the catalog narrowing. No new61+ unique bases or ascended identities are synthesized. `native_equipment_drop_bases.json` is a separate885-ID classification reconstructed from original native type-drop SQL;626 of those ordinary bases are outside the unique whitelist. Rift equipment selection uses that ordinary classification, then separately attempts unique promotion. The existing tier10 first guaranteed-unique slot instead selects the highest wearer-legal base from259 (class, sex, actual level≤60); remaining slots keep the full ordinary level range. No eligible unique means an ordinary fallback in the same slot, without fabricating or adding an item.

`v5/assignment.json` preserves the approved design, source SQL provenance, actual same-base yellow pool evidence, identity decisions and heuristic U ledger. Its original design-only markers correctly describe that input. `v5/authored_stats.json` is the separately frozen executable numeric source; `compile_v5.py` compiles it without deriving names, choosing IDs or cycling mechanics.

Current content has22 MF items,28 skill-bonus items,30 elemental-property items,3 proc items,3 aura items,3 absorb items and51 local-enhancement items. Counts overlap. There are124 fixed items,120 with one variable and15 with two variables. Advanced payloads and skill bonuses are fixed; the existing stored roll controls only explicitly variable lines. Existing curses remain explicit drawbacks and do not count toward five positive static lines.

## Numeric authoring and actual yellow comparisons

Core attribute choices follow each item's explicit design candidates. Initial values reduce the old anchor by its allocated core share before new effects are added. The primary lane value is then calibrated through the actual Go `Compute` path, retaining each chosen attribute rather than replacing the package with an ID-derived template. Local enhancement ranges and advanced payloads stay at their approved per-item values. The explicit final values are checked in; no runtime balancing or dynamic stat generation occurs.

`TestV5UniqueSameBaseStaticBudgetRuntime` covers259 items across570 intended item/class cases × three rolls =1710 scenarios. All have an actual native yellow pool at the same resource tier and equipment type. The reference enumerates compatible max-roll attributes with the real remaining natural-line capacity, including fixed-natural shields. This is a strong lane-optimized yellow, not a claimed whole-item P90. Fresh uniques and conversions retaining the exact same yellow are both computed. Fixtures have zero sockets/refinement, legal sex/class and requirements, natural growth, and remaining free points in STR or INT.

All required passive floors passed; minimum measured ratio was1.100000. Static ratio ranges across classes and rolls:

- Transition:1.100–1.500
- Highvalue:1.170–1.503
- Builddefining:1.130–1.381
- Godlike:1.281–1.601

The upper ranges do not certify the design's full-outcome target bands. Low-level integer granularity and shared flat stats across classes produce outliers; static-only metrics omit crit timing, attack-speed breakpoints, skill curves, triggers, auras and MF. No claim is made that every passive panel lies within a narrow total-DPS band.

Each item retains five or more independent useful static attributes at minimum roll; the test evaluates them through actual stat computation for each intended class and rejects opposite-route offense padding. Primary-lane calibration also retains at least a quarter of a comparable native yellow single-line magnitude, where available, instead of forcing a decorative tiny value to satisfy the count.

Notable explicit changes include1310 flat MAtk200→52 while local MAtk becomes160–190% and all skills+1;1510 flat dual attack255→106 while local attack becomes180–220%. Item2509 requires+523 HP, versus its old98 anchor, to meet the all-class static vitality floor while reserving a separate20%/15s low-health self-heal mechanic. Other stats were reduced before that targeted correction. This exposes the limitation of the design's U ledger: it is a heuristic opportunity-allocation ledger, not an exact universal price conversion. Its arithmetic and advanced share sum are checked, but a formal cross-stat U-price proof has not been invented.

See `../balance/v5_unique_runtime_static.tsv` for every actual result and the adjacent JSON for source/result hashes and limits. The opt-in authoring solver is `TestV5AuthorStaticValuesUsingActualCompute`; ordinary tests never rewrite authored data.

## Formations

The28 rune IDs, order, requirements and saved card effects are unchanged. All40 formation identities and static packages remain. Ten explicit V5 replacements remove an existing trait before adding a new mechanic:

-4:healing-cost trait→healing school+1
-8/14/24:old fire mastery→12%/10%/8% typed fire damage
-11:scoped12% damage→chain spear+1
-32:scoped18% damage→blood-drinking sword+2
-35:low-health rescue trait→healing school+1
-37:generic magic guard→ice resistance12 points and dark6 points
-38:kill healing→MF15%
-40:skill damage→all learned skills+1

No new formation proc or aura is added. These are replacements with explicit tradeoffs, not an extra free effect layer. The existing passive formation budget tests remain applicable because static affixes and rune ingredients are unchanged; full ranked/healing/elemental outcomes need the separate combat tests.

## Existing-instance compatibility

Migration53 adds `unique_content_version` and `formation_content_version` with default0. Version0 is permanently resolved against frozen V4 definitions, including all1360 bases excluded from new generation. Version5 selects new definitions. Loading never guesses a version or rewrites a roll. Unknown versions fail closed without deleting an item. A server-binary rollback after writingV5 items requires an explicit compatible reader/restore plan; an old binary that ignores content-version columns is not a supported rollback.

New awakening writes unique version5. New equipment writes formation version5 when its one-time formation roll is created. Existing completeV4 formations remain version0 on read and on operations that keep them complete. Explicitly completing a formerly unmatched old blank/partial item through rune insertion, array creation or reordering stamps version5 without rerolling its saved quality (legacy zero remains midpoint). Removing a rune from a V4 formation and then completing it again intentionally counts as a new formation and opts intoV5; mere loading, wear or movement does not. Cube previews disclose that version change. Existing processing, sockets, card snapshots, UID, ownership and original rolled affixes are preserved.

Appearance mapping is deliberately broader than generation: all1619 previously published base icon aliases remain via `UniqueAppearanceBaseIDs`, while drops/awakening consult only259. This prevents old shoes or extended-slot equipment reverting to incorrect fallback artwork. The icon audit remains an original-SQL mapping audit, not a new resource upload or new visual preview.

## Validation and remaining boundaries

Passed: both byte-exact generators; Python authoring/negative-contract tests; all domain tests; all store tests; real catalog startup checks; exact scope/tier/effect counts; legacy unique/formation preservation; deep clone isolation; actual same-base passive comparison. Historical V4 tests explicitly use frozen version0 data rather than being deleted when new content stops using old trait families.

Aura aliases are白虎卫身、余烬庇佑、圣羽护佑 and are independently resolved by the client implementation. Actual packed-resource availability and rendered results require separate verification. Owner/remote/pet visibility, AOI recovery, invisibility and visual previews remain separate release checks.

No database was migrated, no live save was rewritten, and no commit/push or asset upload is performed by this implementation. Full loadout, PvP, trigger cadence, healing effectiveness, aura team benefit and MF-per-hour balance are not established by the static report.
