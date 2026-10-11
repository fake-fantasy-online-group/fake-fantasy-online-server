# 星篆 / 宿阵 v1

This source catalogue is complete: 28 independent rune templates, 40 executable ordered formations, ten each of lengths 2/3/4/5. Machine-readable rows are in `astral_runes.json`; runtime rows are `internal/domain/endgame_runes_catalog.go`. The same catalogue drives validation, bonuses and discovery. No formation advertises an unimplemented proc.

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
- Recipe requirements are character level20/30/40/50 for2/3/4/5 holes, plus profession masks and item slot list. Restricted arrays require an employed profession.
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

- Insertion:100% success, no currency cost or random destruction. Requires character level10+.
- Single extraction:1星髓 +1净煞尘 +2500铜. Materials are removed from the candidate first, so a freed material slot can hold the returned card. Invalid/locked target, insufficient materials/money or capacity failure discards the whole candidate.
- Bound rune binds its equipment; inserting into bound equipment binds the inserted rune. Saved binding remains with the card on extraction. Locked cards/equipment cannot be operated on.
- Duplicate/cross-owner UID, occupied/nonexistent hole and invalid card snapshots are rejected. Unrelated refine/wash/fusion/unique state is preserved.
- The legacy random-success native inlay command rejects astral cards and directs players to the new interface, avoiding a second unsafe economic path.
- Pet pickup and ordinary pickup both transfer the complete saved `Drop.Stack`; partial stackable transfers update both ground count fields. Neither path regenerates unique equipment or cards. TradeBusy blocks pet transfers during settlement.

## Integration contracts

Domain exports: `AstralRuneItems(icon)`, `AstralRunes`, `AstralRecipes`, `AstralRecipeByID`, `MatchAstralRecipe`, `ActiveAstralRecipes`, `AstralBonuses`, `ValidateAstralCatalog`, `RollDroppedSocketCount`, `EquipmentRollTable.RollWithReservedSockets`, `PlanAstralInsert`, `PlanAstralRemove`.

Scene owns `onEndgameSocket`, `astralDropGrade`, `rollAstralRuneLoot`; root startup registers actual rune definitions with an existing icon, validates the catalogue, dispatches extension operations and projects catalogues/sidecar views. Unique combat lane adds `AstralBonuses` once in `passiveStatAffixes`.

Tests cover all40 recipes, order/gap/extra-hole/duplicate UID failures, profession/level/broken equipment, deterministic channel selection, legacy hydration, clone isolation, bindings, five-property limits, durable commit/replay/failure, real combat-stat changes, native-path denial, and complete-Stack pet pickup/full-bag/partial retry. Test pass claims belong in verified logs, not implied by this list. No live database migration, deployment or Unity run is part of this reconstruction.
