# Guardian arrival and quiet rewards (migration 79)

Apply migration 79 after migration 78 and restart with the paired client build.
Migration79 publishes rift v5 from immutable v4, changing only the optional
`guardian_arrival_ms` to 5000 and the exit spirit's display name to `归元道使`.
The blue native spirit model 362, existing lonely greeting, tier 30 stat anchor,
completion-level experience, ten additional personal MF-scaled boss equipment
slots, special equipment, runes and dust all remain unchanged. Historical rule
versions 1–4 retain zero arrival delay and their original frozen snapshots.
Replay checks the complete published configuration and preserves later active
versions. No binary packet or opcode changes are required.

## Authoritative sequence

Full progress changes the new run to `boss_arriving`, clears remaining trash,
cancels their pending combat and clears collectible globes. The server chooses
the living, ready payer/leader as anchor; if unavailable, the lowest admitted,
living, ready character ID wins deterministically. Until such a character is
ready, no countdown is invented. A seeded search selects a nearby connected
walkable landing footprint, separated from characters and NPCs. An impossible
map uses the established `spawn_failed` compensation path.

The complete arrival ID, seed, anchor, position, start and deadline are frozen
in the run payload before materialization. The guardian cannot appear before
both the fixed five-second deadline and durable plan acknowledgement. Repeated
ready messages, reconnects, actor reconstruction, duplicate snapshots and lost
ACKs preserve this identity and deadline. If someone occupies the landing
point at the deadline, materialization waits for it to clear without moving the
point or resetting the timer. Terminal states and the hard timeout cancel the
sequence; no delayed callback can resurrect a guardian.

Snapshot writes retain their exact submitted payload. Uncertain acknowledgements
reconcile the storage revision against that payload while retaining newer live
kills, full progress and frozen grants. A verified rollback retries the current
actor state; a genuinely competing record quarantines the actor. Normal ticking
and graceful shutdown use the same reconciliation. Economic aggregate reloads
remain separate. Shutdown also drains queued deaths and persists arrival or
completion state before releasing ownership.

## Additive JSON observations

`rift.state` may contain `guardianArrival` only when its final projected phase is
`boss_arriving`. Its fields are `id`, `seed`, `anchorCharId`, `x`, `y`, `startedAtMS`,
`deadlineMS`, `durationMS` and `remainingMS`. IDs/seed are strings, epoch times are
int64, coordinates/durations are int32. Duration is 5000; remaining is clamped
0–5000. The client uses the server's remaining duration rather than comparing
unrelated wall clocks, deterministically renders native elite-spawn lightning,
and never replays elapsed strikes on reconnect. An absent object during the
arrival phase means waiting for a valid anchor. Interrupted/failed recovery
views omit it so compensation remains available.

Special-monster aura entries include `originalName` for native monster naming.
The v5 guardian's native spawn also retains this real name. Client presentation
adds the requested elite/chief/guardian suffixes. `pendingLoot` counts the
viewer's uncollected personal stacks only while the member still owns an
unclaimed eligible entitlement.

Extra ability speech, per-item acquisition text, experience settlement text
and successful phase notices are silent. A rift ground pickup that cannot fit
returns structured `rift.reward`/`bag_full` with current pending loot, keeps the
exact item entitlement, and emits no native bag-full notice. Storage/error
responses and backend diagnostics remain observable. Global inventory errors
are unchanged. Completion uses the paired client's single retained sentence:
`拾妥所得，可与归元道使交谈离境。`

## Verification

`TestRiftGuardianArrival*` drives kill and globe progress, exact 5000ms gating,
leader/fallback ordering, blocked geometry/occupied landings, ready/reconnect/
reconstruction, malformed saved plans, historical immediate guardians, timeouts,
terminal cancellation, silent bag-full/pickup, lost ACKs before full progress
and after guardian death, queued deaths during reconciliation and graceful
shutdown. `FANTASY_RIFT_WIRE_FIXTURE` optionally exports actual scene-generated
arrival/bag-full/interrupted responses for the client parser's interop test.

Run `tests/sql/validate_rift_arrival.py` only against an explicitly selected idle,
disposable migration 78 PostgreSQL fixture. It covers fresh/replay/rollback,
unchanged historical/loot rules, occupied version rejection, exact-delay
constraints and future-active preservation. Then run the production
`TestBalanceActualPostgresLoader` against that same fixture to verify active
loot 2/rift 5. These deterministic/server checks do not claim Unity visual QA.
