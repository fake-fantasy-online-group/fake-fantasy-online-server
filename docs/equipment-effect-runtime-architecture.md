# Unified equipment-effect runtime

## Contract

`domain.EquipmentEffectDefinitions()` is the single sorted runtime definition
registry. Definitions have a stable ID, source kind, trigger channel and
nonempty description. Duplicate registration is an error, including an
identical duplicate: there must not be two owners executing the same mechanic.

`RuntimeEquipmentEffect` separates `DefinitionID` (mechanic lookup) from `ID`
(the existing causal producer key), retaining source UID, base item, trigger,
description and a typed parameter payload. Rolls, thresholds, scoped skill IDs,
proc chances, aura values and special-weapon profession branches are parameters,
not new mechanic IDs. Parameters are not parsed out of description text.

`domain.DispatchEquipmentEffect` admits registered, source-matched callbacks.
`Scene.dispatchEquipmentEffect` is its combat ingress and additionally bounds
producers with the existing stable-character causal path. Only the same
owner/producer recurring on the same branch is blocked. Distinct effects,
including sets, compose in either order; sibling targets/roots remain independent.
Pure modifiers never enter the producer barrier and apply to their own children.
Deferred callbacks/statuses continue carrying immutable causal-path snapshots.

Source adapters keep their existing functional-wear, durability, class, level,
formation-winner and accepted-source snapshot rules. Registration alone never
grants an effect. The registry does not participate in random legendary drops,
book unlocking, reroll ranges, or item availability.

## Source map and preserved catalogs

| Source | Definition owner | Actual runtime admission |
|---|---|---|
| Legendary powers | `legendary_powers.json`, 330 unchanged power IDs | `ActiveLegendaryPowers`; `registerLegendaryHandler` binds every typed hook to common ingress; event invocation owns producer scope |
| Original named/sharp/ancient special weapons | `special_weapon_effect_registry.go`, 56 profession branches across the existing approved bases | Original enhanced-base mapping and skill/profession checks; specialized payloads dispatched once, not old-plus-new |
| Unique traits | `uniqueTraitNames`, 29 kinds; kill-mend and kill-mana keep separate producer identities | `ActiveUniqueTraits`/astral traits and accepted snapshot selection; original strongest-per-kind reduction and caps preserved in evaluator |
| Typed equipment channels | `equipment_effects_v5.go` validation, 9 `Kind` values | `ActiveEquipmentEffects`, existing proc/aura/variant rules; typed payloads preserve parameters |
| Native equipment and sets | Built-in runtime effect definitions | Existing equipped native attributes and set eligibility; common producer barrier |

The nine actual typed `Kind` values are `skill_variant`, `magic_find_pct`,
`element_damage_bp`, `element_resist_bp`, `enemy_element_pierce_bp`,
`element_mitigation_bp`, `element_absorb_bp`, `proc`, and `aura`. Element,
proc payload, aura family and variant ID refine a channel; counting those as
additional families must not create additional executable effects or alter
availability. Original equipment with only stats is not assigned a fabricated
executable effect. The original item and special-weapon catalogs remain the
source of truth for which records have behavior.

## Verification contract

Registry tests assert sorted stable IDs, collision rejection, source matching,
parameter preservation and payload-copy isolation. Scene contract tests exercise
cross-source A→B→A cycle prevention, independent siblings and own-child pure
modifiers, plus all typed legendary hook bindings (unknown definitions cannot
execute). Mechanic-specific behavior and source-eligibility tests remain required:
registry coverage is not a substitute for actual HP/MP/status/damage settlement.
`docs/legendary-effect-behavior-gate.json` records the separate measured behavior
gate; this architecture document does not claim that gate is closed.
