# Rift scenery exclusions

The player reported that Huang Lei Island still cannot change floors on the
live client and explicitly requested removal instead of further portal repair.
The previous snapshot identity repair was a real independent defect, but its
unit contracts did not establish that these map portals worked online.

Remove all `pw23800` aliases (23800, 23900, 24000, 24100, 24200, 24300), plus
family quarry maps 933–937, the complete native spawn set of family stone 30069.
Also remove 20100 and 28502–28505: raw native link records mix ordinary gates
with destination-zero Trap script gates (Trap20100_01/02 and Trap329_2/3/4/5).
These are the same requested risk class, not a claim that every gate in those
maps is broken.
Conservatively also exclude 24600–24800 (raw native link/proc contain no ordinary
gates) and 36700 (native gate records unavailable). These four are unverified,
NOT proven player-runtime failures. They are removed under the requested
conservative usable-pool policy. In the current 189-map audit, the 20 new
exclusions leave 169 candidates; this is not an online playability guarantee.
Keep the pre-existing special island/protocol alias exclusions 60 and 21000.

`domain.RiftMapExcluded` is applied both by the startup data loader and by all
floor generator versions. New runs and future floors of existing live runs
cannot select these maps, even from a previously built template catalog.
Ordinary map definitions, native portal permissions, character data, loot and
rewards are not changed. There is no SQL migration.

## Existing runs and restart

Do not rewrite saved floor plans or make them invalid: finished rewards and
interruption compensation must remain readable. The existing server does not
recreate active rift actors after restart. Login resolves a missing instance to
its ordinary-world counterpart (or configured dungeon exit), not to a restored
private floor. The original key payer can claim one bound key for a nonterminal
run with no live actor. Successful compensation sets failed/compensated, so it
cannot be claimed again. Other members receive no extra key. Completed rewards
remain on the normal existing claim path. This is NOT continuation preserving
the current run's playable progress; the durable record is not cleared.

Focused tests cover every removed ID against generator versions 1–5, a mixed
safe/blocked catalog, an all-blocked catalog, unchanged adjacent IDs, readable
old floor/progress records and existing exactly-once interruption compensation.
They do not claim Unity/player-runtime validation.
