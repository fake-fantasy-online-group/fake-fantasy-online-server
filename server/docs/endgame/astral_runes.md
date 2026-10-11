# 星篆 / 宿阵 v2

This source catalogue contains 28 stable rune identities and 40 individually authored ordered formations, ten each of lengths 2/3/4/5. Each formation records its identity, intended benefit, opportunity cost, explicit stats, and optional executable traits. Machine-readable rows are in `astral_runes.json`; runtime rows are `internal/domain/endgame_runes_catalog.go`. The same catalogue drives validation, bonuses and discovery. No formation advertises an unimplemented proc.

## Setting and naming

The original game's 水魔珠 / 老君演算之阵 / 应劫者 story supplies the setting. The new gameplay extension is original: 星篆重定宿序，稳固演算之阵. Specific formation names and stat values are authored game content, not historical Daoist ritual prescriptions.

The mansion sequence and four groups follow the Hong Kong Space Museum:
https://hk.space.museum/sc/web/spm/resources/teachers-corner/constellations-and-myths/glossary-of-chinese-star-regions-asterisms-and-star-names.html

Full animal/seven-luminary names are supported by National Central University's art-history paper, printed pages35–36 (壁水貐; 张月鹿 follows the standard luminary order rather than the scanned typo):
https://art.ncu.edu.tw/artConf/main/public/10/%E5%85%AD_%E6%A5%8A%E9%9B%85%E7%90%B2_.pdf

Official game setting:
- https://fo.qq.com/web200705/information/1_1.htm
- https://fo.qq.com/web200705/information/1_1_6.htm
- https://fo.qq.com/web200705/information/1_1_7.htm

## Concrete item identity

IDs1900028001–1900028028 follow east 青龙, north 玄武, west 白虎, south 朱雀, seven each. There is no rune rank in v1. Each is a nonstackable existing `ItemInstanceSocketCard`, with a permanent UID and `CardInstanceState` effect snapshot. Insertion transfers that exact UID into existing native socket arrays. Extraction returns it without regenerating its effects.

Existing `Stack.SocketCount`, `Sockets[5]`, `SocketUIDs[5]`, `SocketCards[5]` remain the sole socket facts. No extra activation flag or duplicate rune inventory is persisted. Legacy hydrated equipment with positive UID and kind=None is accepted only with a verified equipment template; modified candidates canonicalize kind=Equipment. Rune card kind remains strict.

## Exact matching and active effects

- Required socket count equals recipe length; an extra empty hole prevents a shorter prefix from activating.
- Read hole0 through hole4. Empty holes, wrong order, old non-astral cards, duplicate card UIDs and missing snapshots do not form an array.
- Each rune has an explicit level requirement from 12 to 60, progressing in the existing mansion order. Recipe requirements are the maximum of their authored level20/30/40/50 floor for2/3/4/5 holes and every ingredient rune requirement, plus the existing profession masks and item slot list. Restricted arrays require an employed profession.
- 战阵 uses weapons4/13; 守阵 uses head2/shield5/body8; 辅阵 uses face1/neck3/gloves6/ring7/shoes9.
- Only functional currently worn equipment grants formation bonuses. Broken gear does not. A complete formation may still be described while in the bag.
- At most one formation per channel contributes: more holes wins, then lower recipeID, then lower worn slot. Other complete formations keep their single-rune effects but not their formation bonus.
- Single-rune effects already flow through socket card aggregation. `AstralBonuses` is appended once to `Compute.Passive`. Percent bonuses use the existing additive bucket, not another multiplier.
- Real engine semantics apply: defense affects avoidance; physical/magic resistance are flat damage subtraction; physical/magic crit use basis points; negative attack interval percentages accelerate attacks.

## New-drop sockets

The existing five-property budget is preserved: fixed natural affixes + rolled natural affixes + holes <=5. Empty holes consume budget. Reserve holes before rolling the remaining affix budget, only while creating a new drop. Never reroll an existing UID on pickup, redrop, trade, mail or warehouse movement.

Random sockets apply to combat slots1–9 and13. Equipment wearer requirement below10 gets none; requirements10–19 cap2,20–39 cap3,40–49 cap4,50+ cap5. Fixed natural affixes can lower that cap further.

Weights for0/1/2/3/4/5 holes:
- Ordinary:72/21/6/1/0/0
- Elite:45/25/18/9/3/0
- Explicit boss:25/25/23/17/8/2
- Realm reward helper:15/20/25/22/13/5

An over-cap roll truncates to the actual capacity. Fixed-boss table membership counts as boss even when the legacy monster kind is normal/elite. Realm rewards may use the provided helper; ordinary rift trash must follow the rift lane's no-native-loot rule.

A level20+ combat kill rolls at most one supplemental rune: ordinary0.6%, elite8%, explicit boss20%, with existing level-gap penalty. All28 choices have equal weight. This pool is separate from the native drop-table limit. It does not run in worlds lacking registered rune definitions.

## Safe operations and economy

`PlanAstralInsert` and `PlanAstralRemove` return cloned candidates without changing live inventory. The scene calls the shared durable revision/receipt transaction gate and only publishes after committed success.

- Insertion:100% success, no currency cost or random destruction. Requires the character to meet the selected rune requirement and, when completed, the formation requirement. Cube inscription and reordering enforce the same current requirements during preview and commit.
- Single extraction:1星髓 +1净煞尘 +2500铜. Materials are removed from the candidate first, so a freed material slot can hold the returned card. Invalid/locked target, insufficient materials/money or capacity failure discards the whole candidate.
- Bound rune binds its equipment; inserting into bound equipment binds the inserted rune. Saved binding remains with the card on extraction. Locked cards/equipment cannot be operated on.
- Duplicate/cross-owner UID, occupied/nonexistent hole and invalid card snapshots are rejected. Unrelated refine/wash/fusion/unique state is preserved.
- The legacy random-success native inlay command rejects astral cards and directs players to the new interface, avoiding a second unsafe economic path.
- Pet pickup and ordinary pickup both transfer the complete saved `Drop.Stack`; partial stackable transfers update both ground count fields. Neither path regenerates unique equipment or cards. TradeBusy blocks pet transfers during settlement.

## Integration contracts

Domain exports: `AstralRuneItems(icon)`, `AstralRunes`, `AstralRecipes`, `AstralRecipeByID`, `MatchAstralRecipe`, `ActiveAstralRecipes`, `AstralBonuses`, `ValidateAstralCatalog`, `RollDroppedSocketCount`, `EquipmentRollTable.RollWithReservedSockets`, `PlanAstralInsert`, `PlanAstralRemove`.

Scene owns `onEndgameSocket`, `astralDropGrade`, `rollAstralRuneLoot`; root startup registers actual rune definitions with an existing icon, validates the catalogue, dispatches extension operations and projects catalogues/sidecar views. Unique combat lane adds `AstralBonuses` once in `passiveStatAffixes`.

Tests cover all40 recipes, order/gap/extra-hole/duplicate UID failures, profession/level/broken equipment, deterministic channel selection, legacy hydration, clone isolation, bindings, five-property limits, durable commit/replay/failure, real combat-stat changes, native-path denial, and complete-Stack pet pickup/full-bag/partial retry. Test pass claims belong in verified logs, not implied by this list. No live database migration, deployment or Unity run is part of this reconstruction.


## Level requirements and compatibility

The 28 rune levels, in item-ID order, are: 12, 14, 16, 17, 19, 21, 23, 24, 26, 28, 30, 32, 33, 35, 37, 39, 40, 42, 44, 46, 48, 49, 51, 53, 55, 56, 58, 60. They are evenly rounded across the authored 28-mansion order. Rune snapshot fields, crafting costs, drop rates, slot lists and profession masks remain unchanged. The v2 formation names, lore, derived affixes and conditional traits are intentionally redesigned. Universal formations remain universal.

`EquipmentRequiredLevel` derives the maximum of base-equipment level, installed rune levels, and any exactly matched formation. Equip, alternate-set switching, native blocked flags and tooltip descriptions, sidecar item levels, functional equipment selection, combat stats and granted equipment skills use this current requirement. The rune catalog wire DTO adds `minLevel`; ship the paired client update with the server because its JSON schema is strict.

No database migration or instance rewrite is required. Existing rune UIDs, saved card effect rolls, socket order and equipment identities remain intact. Previously worn gear that no longer meets the level requirement remains worn but grants no functional bonuses until eligible; it is not deleted or silently moved. Unequip and extraction remain available, including below the new requirement, under their existing inventory/cost rules. Level-up refresh and relog reevaluate eligibility. New drops and crafted runes may still be owned or traded before their use level; possession is not restricted.

The snapshot uses the same functional-equipment selection as combat. `同类宿阵优先级未满足` is returned only for an independently eligible formation that lost canonical channel selection; level, durability, profession and other equipment requirements are reported separately.

## Version 2 authored formation and skill effects

Compile with `python3 docs/daoist-endgame/catalog/compile_astral.py`; `--check` verifies exact generated Go. The compiler formats explicit rows only, never manufactures prose or stats. All40 names/lore are individually written and each recipe includes a distinct use/cost; statistical string uniqueness is not an editorial quality claim.

Formation strength is evaluated as base + ingredient snapshots + selected formation, compared with the same base's coherent five-line yellow opportunity. Strong physical five-rune specialists use roughly40–44% global attack with additional focused lines; they do not borrow D2's on-weapon enhanced-damage scale. Off-build and utility arrays can remain substantially weaker for pure panel damage. No formula implies every five-rune array is best for every build.

Skill scope is an optional `SkillID` filter on an existing trait. Canonical skill names and class provenance are frozen in `catalog/skill_scopes.json`. The actual strike/cast/heal passes the current ID, filters before strongest-kind selection, and applies existing caps. Examples: 连环枪法 damage, 流虹之枪 mana economy, 群疗术 ally healing, 暴雪术 cast time or area. This grants no skill and modifies no global SkillTable. Future+skill levels remain reserved inactive data, separately from these working per-skill effects.

The server projects executed formation traits and intended use/cost in the existing description field, so no invisible wire-only field is required. At most one formation per channel still wins. All saved rune/equipment UIDs, card snapshots, ingredient ordering and potency are preserved. Existing arrays receive the new derived formation definition; there is no inventory rewrite or silent reroll.

轸星篆's saved flat recovery amount now has a narrow rune-only two-second pulse, using the actual stored card effect. Ordinary equipment regeneration fields are not globally activated. It requires living, eligible, valid worn equipment and unique card identity; unequip/death/relogin cannot yield offline catch-up or resurrection.
