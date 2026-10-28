# Player-facing equipment effect display audit

## Scope and boundary

Audit covers all 330 legendary definitions at valid legacy-zero, minimum (7500), intermediate (8123), and maximum (10000) rolls, plus every registered effect source, current unique catalog, typed effects, named weapons, class sets, and five skill variants. It changes prose/rendering only. No damage, trigger, stacking, immunity, cooldown, item UID, stored roll, six-affix range, binding, or use rules are changed.

Registry `Description` is not universally player prose. The 440-row companion JSON maps each registered identity to its actual display consumer; internal typed/set dispatch labels are not sent as tooltip text. Unique `Description()` returns `Lore`; `Benefit`, `Tradeoff`, `Identity`, and `LoreRoot` are not tooltip fields.

## Display path matrix

| Path | Authoritative producer and consumer | Finding/result |
|---|---|---|
| Inventory gear | `inventorySnapshot` → `ownedItemTooltip` → native `desc` | Exactly one `U+0006` name/effect row, actual roll. Ordinary six-affix bounds untouched. |
| Equipped gear | `ownedItemTooltip` + `endgameItem` legendary sidecar → client `OverlayTooltip` | Same named text, no raw-effect lore duplication. |
| Other-player inspection | `onInspectPlayer` → `itemTooltipForViewer` | Uses same actual instance renderer; tested with independent viewer negotiation. |
| Wings | Native tooltip `WingPowers` → `LegendaryPowerForRoll` | Each category uses donor's stored roll; no category headings or ranges. |
| Book owned | `onEndgamePowerBook` → `LegendaryPowerForRoll` → `PowerDescription` | Actual recorded description. Current and range columns use paired first authored parameter. |
| Book unowned | Same list renderer without ownership | Reference effect, no invented current value. Client current column remains dash. |
| Book range | `LegendaryPowerPrimaryRangeText` → `PowerRangeText` | Two endpoints only; no effect prose in column. No other hover consumes this field. |
| Scroll native fallback | `inventorySnapshot` → non-equipment `itemTooltip` | Fixed missing effect: now includes actual stored-roll orange description without requiring sidecar. Usage remains intact. |
| Scroll with snapshot | `endgameItem.PowerScroll` → `PowerScrollTooltip` | Identical named current effect; exact native-line match deduplicates/recolors rather than appending twice. |
| Original/sharp/ancient named weapons | `DungeonWeaponEffectLines` → native `specialEffects` | Existing exact identity gate retained; ordinary originals and inert quest weapons unchanged. |
| Unique effects | `UniqueTrait.Describe` → native effect / snapshot Trait | 29 semantic kinds have intrinsic condition/value. Removed unconditional shared-stacking appendix from native tooltip. |
| Typed effects | `equipmentV5Lines` | Removed variant client-version/stacking suffix. Intrinsic proc trigger, chance, skill, and cooldown retained. |
| Class sets | `classSetSkillText` | Removed five repeated aura stacking-policy suffixes, retained class skill effects and aura values/radii. |
| Skill variants | `EquipmentSkillVariantRule.Variant` | All five descriptions now parameter-derived, so existing DB rows cannot reintroduce old notes. Removed no-chain/no-secondary-status/no-knockback footnotes and swap/storage/client policy. Kept actual missing-health calculation, target, shape, damage, delay, cooldown. |

## Verification

- `TestPowerEffectDisplayAll330AtEveryRollBoundary`: actual native gear/inspection, bag/equipped sidecars, wing and scroll native/snapshot producers; immutable stack comparison includes UID/affixes/rolls.
- `TestEffectDisplayRuntimeAppendersDoNotExposeImplementationPolicy`: every active unique catalog row, all ten class-set skill tiers, all unique typed effect output.
- `TestPowerScrollNativeFallbackAndSnapshotShareActualEffect`: production inventory item text and snapshot agree for TY-A02 roll 8123 (`48.73%`), orange marker exactly once, no roll range. Optional export is deterministic test evidence, not a live capture.
- `TestEquipmentRegistryDisplayAuditCoversEveryRegisteredSource`: 440 stable registry identities, every source assigned to its real display pipeline.
- `TestEquipmentVariantDisplayIgnoresLegacyPolicyDescriptions`: injects old policy-bearing DB text and verifies it cannot escape, without changing parameters or identity.
- Existing named-equipment, legendary-tooltip, unique-lore, typed-tooltip, and variant regression tests rerun with the above.

The first-parameter anchors remain in every rewritten legendary effect. SS-A19's `初始总伤害65%` is the intrinsic delayed explosion multiplier, not a generic lifesteal/mana settlement rule. Shared numeric column fixtures are owned by the metadata task; this audit does not alter numeric mechanics.

## Environment limitation

`git fetch origin main` cannot succeed here: origin is the provided local `server-87bdf14.bundle`, which has no `main` ref (`fatal: couldn't find remote ref main`). No push, deployment, external game service, or Mac run was performed. Tests are deterministic local production-renderer evidence; client UI screenshot QA is handled separately.

### Final combined verification

After catalog commits `03f4757`, `1d29e3a`, and fixture refresh `8c61d04`, the focused combined domain/scene audit passes (domain 0.223 s; scene 2.585 s). The 315 numeric and 15 fixed definitions yield 1,275 valid roll cases; fixed effects reject nonzero rolls rather than inventing variation. Registry counts are 330 legendary, 56 named-weapon branches, 29 unique traits, 9 typed channels, 14 set sources, and 2 native sources.

The client owner reports 5,137 UI contract assertions passing, including production-exported native scroll text plus snapshot: the overlay returns the exact same tooltip bytes, with one orange current-value effect. Both native tooltip implementations map `U+0006` to RGB `(255,136,48)`. Shared native scroll fixture SHA-256 is `1334b1929357d609834ef858f4f269693d681f09500e7c2d8c61de596386e7bb`.
