# Original weapon effects: unified source conversion

Cloud branch only; not pushed, merged, or released.

## Authoritative sources and preservation

- `domain/dungeon_equipment_manifest.json`: 60 original templates. Twelve low-level templates are stat-only and intentionally register **no skill effect**. The 48 active weapon identities are six original level-14 task weapons and 42 explicit true-dungeon IDs. Ordinary dungeon originals and fabricated task hard variants remain inactive.
- `domain/special_weapon_effect_registry.go`: 56 profession branches, migrated verbatim from the previous scene skill-membership switch. Shared caster weapons retain separate healer and warlock rows. Registry IDs are `special-weapon:<base>:<profession>`; no legendary book/drop entry is created.
- `SpecialWeaponSourceCandidate`: functional durability + original `EnhancedWeaponBaseID` rules, exact item/instance provenance.
- `applySpecialWeaponModifiers`: learned hard rank gate remains; common dispatcher calls the original numeric operation once. Pure definition preview delegates to the same canonical membership table and keeps original fixed values and copied-skill semantics.

## Runtime conversion manifest

| Existing boundary | Unified causal hook | Producer |
|---|---|---|
| copied status definition / inherited next-status power | status-definition | no |
| warcry lost-HP accounting / inherited basic bonus | warcry-accumulate / inherited-basic-bonus | no |
| learned skill definition | skill-definition | no |
| cast released / finished | cast-released / cast-finished | yes |
| conditional hit definition | before-hit | no |
| resolved hit secondary effects | after-hit | yes |
| dispel healing / residual steal | after-dispel | yes |
| shield-to-health conversion | convert-shield | yes |
| active icebind aiming | aim | no |
| exact active icebind consumption | consume-freeze | yes |
| immediate trap damage | instant-trap | yes |
| restored health event | heal | yes |
| overflow / mana-destruction shield | shield | yes |
| periodic smoke refresh | smoke-status | yes |
| warcry shield consumption | warcry-shield | yes |
| counter damage / native basic penalty | counter-damage / basic-attack | no |
| one-shot break followup | break-followup | no |

All causal IDs remain independent of UID: `special-weapon:<base>:<hook>`.
Two copies cannot recurse around the guard by changing instance identity. Distinct
weapon effects and ordinary modifiers still compose. Accepted cast/status
snapshots retain their original weapon identity across swap; no fresh worn-item
check silently cancels them. Generic status consumption in prepare/basic paths
is deliberately not gated on having a special weapon.

The `*Runtime` methods are implementation callbacks, invoked only by the shared
dispatch wrappers. The former direct `enterCombatEffect` in weapon healing is
removed rather than nested under a second guard. Its original `heal` causal key,
received-heal calculations, set copying, `Healed`/`HealResolved` event delivery,
and group-steal fresh-instance preservation remain intact.

## Verification

- `TestSpecialWeaponRegistryEveryOriginalSourceAndBranch`: all 56 branches and all
  120 original/offset identities (48 active, 72 inactive), broken gear, invalid
  professions/skills, copied catalog ownership, source provenance.
- `TestSpecialWeaponUnifiedModifierDelegatesExactlyOnce`: actual equipped/learned
  definition equals one baseline application; unlearned skill stays inactive.
- `TestSpecialWeaponUnifiedDispatchOneProducerAcrossTwoInstances`: same semantic
  producer blocked across distinct UIDs, pure modifier and distinct producer
  admitted, independent subsequent root admitted.
- Existing `TestSpecialWeapon*` tests exercise original numeric/status/combat
  behavior and accepted-cast interactions; companion behavior matrix records
  which actual branch settlement assertions exist. Catalog/identity tests alone
  are not claimed as 100% behavioral or statement coverage.
