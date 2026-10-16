# Player-owned combat attribute entity

`Player.CombatAttributes` owns the player's derived runtime attribute entity:
functional equipment eligibility, raw skill ranks, final primary/secondary stats,
and skill-growth resource deltas. `Scene.onEnter` explicitly initializes it after
restoring character/status inputs and before exposing the player. Pet restoration
and existing mutation handlers update it before gameplay. The first attack does
not construct it. The scene retains only a dependency-tracking record.

The object remains owned by the same player across equipment, progression and
status changes; its revision advances when rebuilt. It is never persisted or
included in the cross-scene runtime packet. Destination admission constructs a
new owned object, and source departure removes its dependency record. Rejected
admission also removes any partially constructed record.

## Dependency boundary

Reads compare equipment pointer/effect revision; character base, level,
profession/employment, gender and base move speed; hard skill ranks by value;
effective status affixes; and live pet stat affixes. Status affixes are compared
as a canonical multiset so map iteration does not cause spurious rebuilding.
Existing mutation handlers refresh live combat stats; dependency comparisons also
protect direct authoritative input mutations.

Positive durability wear does not change eligibility or stats. Returned equipment
views still copy current stack values, so durability/tooltips do not become
stale. Breaking, repair, sockets, refinements and other instance changes use the
existing equipment effect revision.

Initial construction uses one short-lived prerequisite-resolution context for
functional equipment, raw ranks and growth/stats. Recursive subsets memoize raw
ranks. Subsets without equipment rank bonuses or grants share the identical pure
permanent-loadout resolution. The context is discarded after construction and
never retained on the player. The legacy raw-rank memo uses equipment
identity/revision and exact canonical hard-point encoding instead of serializing
and hashing every stack/card/socket. Identical with/without-growth passive input
lists need only one pure stat computation.

Item/skill/catalog tables are initialized before scene construction and immutable
for the scene's lifetime. There is no runtime catalog reload API. Tests that
change templates or inject synthetic resolved ranks explicitly invalidate the
inputs first. A future catalog reload feature must invalidate all dependent
runtime state before using new definitions.

## Intentionally live

HP/MP, XP, free points, stamina, riding projection and outgoing attribute arrays
are rebuilt on every snapshot. Target selection, aura range/alive checks, proc
cooldowns, random rolls, actual damage and reward/drop ordering are unchanged.
Post-computation status resource floors and attack limits retain their existing
live application path. Existing equipment effect/proc caches are unchanged.

## Validation and measurements

Tests cover direct input mutation, status suppression, pet effects/death,
positive wear, same-ID replacement, preview isolation, XP/resource snapshots,
level growth, raw-subset oracle equivalence and the existing mana/cooldown
boundaries. Real admission tests load nine equipment slots before `Enter`, inspect
readiness without a warming accessor, and verify the first attack does not change
the owned object or revision. Real transfer, input mutation and rejected-admission
cleanup are covered separately.

The batch test resolves ten lethal targets with nine equipment slots and eighty
ground drops, and verifies each XP message follows its authoritative attribute
snapshot. Benchmarks use real admission with equipment loaded beforehand:

- Full admission, including derived attributes and outgoing snapshots: 4.48 ms,
  3.77 MB / 8441 allocations.
- Admitted player, first ten-kill/eighty-drop batch, no extra warming: 0.400 ms,
  0.375 MB / 886 allocations. The attribute revision stays unchanged.
- Explicitly invalidated attributes, rebuild plus the same batch: 0.758 ms,
  0.718 MB / 1715 allocations.
- Isolated derived-attribute rebuild: approximately 0.47 ms / 0.344 MB;
  an unchanged attribute snapshot is approximately 0.3 microseconds / 480 bytes.

These are 100-iteration synthetic development-host CPU measurements, not deployed
end-to-end latency promises. Admission and rebuilding are intentionally reported
separately from ready-player combat. Earlier cold reproducer measurements (85 ms
before this work, then 3.7 ms after its first phase) replaced the whole character
and equipment *after* login, so they included artificial post-login construction.
They must not be described as the normal first-attack cost.

Run from `server/`:

    go test ./internal/game/scene -run '^TestCombatAttributeCache' -count=1
    go test ./internal/game/scene -run '^$' -bench '^BenchmarkCombatAttribute(Admission|BatchDeaths)$' -benchmem -benchtime=100x
    go test ./internal/game/scene -run '^$' -bench '^BenchmarkCombatAttributeCacheNineSlots$' -benchmem
