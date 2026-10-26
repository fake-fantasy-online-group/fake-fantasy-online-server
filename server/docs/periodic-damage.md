# Periodic damage: source identity and magic mitigation

## Status instances

Periodic HP-loss statuses retain one instance per `(StatusID, responsible caster)`
on both players and monsters. Reapplication refreshes the lifetime rather than
adding another tick stream. Different casters and different status IDs coexist.
This rule supersedes the overlay matrix's same-ID diagonal for DoTs; cross-ID
blocks, replacement and counteraction still apply. Healing, MP-only changes and
nonperiodic statuses retain their existing overlay behavior.

A same-source refresh preserves the pending tick, fractional damage balance and
stronger existing overlay priority. A stronger replacement updates its payload
without restarting the clock. Class-set fractional balances are converted when
the authored tick-count denominator changes. Copied poison retains the actual
remaining lifetime and clock, not a fresh full-duration refill.

Existing summon/pet/trap ownership normalization remains authoritative. Saved
external source IDs are deliberately discarded by the existing save format;
those restored unknown sources remain independent rather than being guessed to
be one caster. No database migration or new client protocol is required.

## Damage classification

All sustained HP damage is magical. Native status ticks reuse the ordinary magic
native-defense formula (`RollMagicFixedDefense`) once, followed by the normal
magic incoming buckets: applicable element/unique mitigation, magic damage-taken
and reduction, passive half damage, flat reduction, party/set reductions,
dungeon scaling, applicable legendary reductions, mana absorption, shields and
pet protection. Live defenses apply to the snapshotted payload. Physical-only
reductions do not apply. Unknown elements remain unspecified, not inferred as fire.

Native periodic ticks retain their existing specialized trigger contract: no
new normal-attack accuracy/critical reroll or attack-reflection generation.
Explicit periodic-critical powers still work. Healing and MP-only ticks do not
enter damage mitigation.

Rift poison pools select one strongest overlapping effect per caster and target;
different casters retain independent cadence. Repeated lightning pulses use the
same source-aware admission. Both are magical, preserving the original attack
budget. One-shot delayed explosions are unchanged. Cadence is runtime-only and
is rearmed on restore/reconnect, as before. Warrior quake's sustained field is
magical; its initial direct physical strike remains unchanged.

## Regression coverage

- `domain/TestDOTSource*`: per-source instances, overlays, strength, refresh
  cadence, fractional balances, expiry, restored unknown sources and copies.
- `scene/TestDOTSource*`: player/monster targets, overlapping applications,
  different sources/types, cross-ID rules, and summon ownership.
- `scene/TestPeriodic*`, `TestRiftPeriodic*`: real magic defenses and reductions,
  shields, live changes, missing sources, zero damage, independent ground cadence
  and restore/rearm behavior.
