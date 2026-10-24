# Legendary eligibility follows the source monster

New monster equipment records its generation level in `Endgame.SourceLevel`.
Legendary candidate admission compares the existing catalog `MinLevel` with that
monster level, not the item's wear requirement, affix resource level, or the
recipient's level. Slot, type, profession, and executable-handler restrictions
remain unchanged. No new learned-skill gate is added.

For example, a level43 monster can produce a threshold42 universal defensive
power on a level18 face item. The face item still requires level18 to wear and
retains its own native affix pool. A level12 monster cannot produce that power
even on a level60 base. True and secret multipliers and all six affixes remain
valid after reload because runtime checks use the persisted generation level.

Natural kills, original Boss fixed rows, theft, new Boss guarantees, and both
Rift special and ordinary10 rewards use the actual source monster. Rift completion
freezes the guardian's actual level in the death event and persisted reward state;
replay never substitutes a current player level. Historical ungenerated runs may
recover the exact seeded encounter Boss definition, including Boss-offset rules.
An unknown source is left pending instead of silently producing inferior rewards.
Already-generated grants are never changed.

Ordinary monster gear keeps provenance even when the legendary roll misses.
Later cube awakening uses that provenance; reforge preserves the power, tier,
source and bounds. Legacy/non-monster items with unknown source0 retain their
historical base-level awakening and validation behavior. No old item is rerolled
or inferred from the current player or template.

Migration108 adds a nonnegative integer source_level column with default0 to
existing equipment metadata. Production SQL save/load includes it. Apply migrations
106,107,108 in order before running the combined server binary. Old binaries
do not honor source-level eligibility and can deactivate higher-level powers on
lower-wear bases, so code rollback is not a safe semantic rollback after new drops.

Validation includes generation/activation/JSON/crafting/store tests and focused
actual PostgreSQL-engine migration/save/load/conflict tests through PGlite.
Those focused checks are not a claim of running the complete production initializer
or live PostgreSQL server integration suite.
