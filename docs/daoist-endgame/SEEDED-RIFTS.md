# Seeded, multi-floor 演劫洞天

## Player-facing design

Difficulty is **第 1–150 阶**. Traversal is **本次洞天第 N 层**. Starting a run selects one unlocked difficulty; descending changes only the map layer, never difficulty, progress target, reward tier or entry price. A timely clear unlocks the next difficulty, capped at 150. Floor count has no connection to the unlock ladder.

A single opener pays one bound key for the frozen party. Every run has one 15-minute timed target, 20-minute hard limit, shared 10,000-point progress counter and five-second death penalties. New floors normally contain 40–64 monsters (smaller validated maps adapt density). Elites supply progress globes. Reach the marked exit to skip an unpleasant layout or surviving pack; the opener advances the whole present party. No minimum floor kill quota. Uncollected globes and surviving enemies are abandoned. Offline admitted members reconnect to the current layer, not the abandoned one. No previous-layer entrance exists.

At full progress, remove trash and globes and summon one guardian on the current floor. Floor advance is disabled during guardian combat. One run has one final reward settlement, not one reward per floor. Existing bound rewards, bag-full retry, exact saved grants and atomic claim receipts remain authoritative.

## What is generated

This version generates a **seeded sequence of existing maps plus a seeded population and encounter plan**. It does not generate new terrain meshes, tile art, collision geometry or scene artwork. This distinction is intentional: native map loading, collision, movement and resource IDs remain intact.

At server startup, the immutable floor catalog filters existing non-city maps with real hostile monster assets. It loads the native collision MASK and uses four-neighbor flood fill to retain one connected component of valid native spawn anchors. Missing collision/resource definitions and special island map aliases are excluded. Entry, exit, monsters and guardian use real anchors on that component; no random coordinate jitter can place monsters in walls. Minimum anchor count, separated entry/exit and a protected entry radius are enforced. Startup catalog failure disables new admission before any key is spent.

A static audit of shipped SQL and gzip MASK resources identified 189 candidate maps, including 桃源, 仙踪森林, 千叶森林 and 沙角. This is not a production runtime or rendered-map count: runtime checks loaded definitions, hostility and attackability too.

Generator version 1 uses SHA-256 over the versioned run identity and floor index, then an explicitly implemented SplitMix64 stream. Map and monster pools and anchors are sorted before selection. Adjacent floors avoid repeating a map when alternatives exist. Combat randomness does not consume the layout stream. The decimal UInt64 seed in the UI is the current floor seed. The complete admitted plan is persisted; recovery never regenerates a paid floor against a changed catalog.

## Dynamic monsters and readable abilities

Appearance comes from the selected map's native monster assets. Stat and native AI/skill baselines come from the frozen roster's trial level, so choosing a low-level wilderness appearance does not lower the selected rift difficulty. Each entity has its own scaled base stats; buff/debuff recalculation cannot reset them or modify global templates.

- HP: original × 1.08^(tier−1) × party factor × elite/guardian factor.
- Attack: original × 1.04^(tier−1) × elite/guardian factor.
- Native signed-int32 stat limits remain; saturation is deliberate at the protocol limit. Arithmetic for damage, crits, healing, DoT, reflection, mitigation and HP percentages is widened/clamped to prevent wraparound. High-tier numerical balance still needs live playtesting.
- Blue elites receive one seeded affix, yellow elites two. Guardian rotates all three attack shapes.
- Circle eruption, ring nova and line fissure use locked geometry with 1.4–1.8-second visible tells and recovery time. The client draws matching world-space warnings and countdowns with existing UI primitives. No new art/skill-resource pack is required.
- Damage is server-authoritative through the native status/mitigation/reflection/death pipeline. Players are checked at impact, so moving out avoids it. Unready, newly loaded or newly revived players do not receive an old warningless impact. There are at most four simultaneous tells.
- Transition, loading, terminal and economic-pending states suppress abilities and combat. Rollback cancels pending tells but preserves affix identities.

## Floor ownership transaction

1. Validate current run ID and observed floor index; require the living opener near the current exit, active phase, ready map, and no conflicting run/character economic write.
2. Build the next immutable plan and reserve an **unpaid** destination actor.
3. Clone run state; retain tier, timer, progress, roster and grants, but retire all old monster/globe IDs. Save the next floor with revision CAS. No inventory/key mutation occurs.
4. Freeze source combat while the write outcome is unresolved. On a failed acknowledgement, read the durable run. If unchanged, discard only the unpaid target and resume the same source floor. If committed, publish that exact plan. Unknown outcomes retain ownership and retry the read.
5. Atomically retire source admission and publish destination admission after durability. Transfer all present admitted players using native map-load and ownership handoff. Existing-only rift handoff prevents a stopped target from lazily recreating an ordinary scene. A prepared actor lost before publication is replaced with the same saved plan.
6. Stop the retired source once players leave. Neither old deaths nor globes can contribute again. Replay with the old floor index cannot skip a second floor.

The shared wall-clock deadline includes loading and persistence waits. Dead members retain their revive timestamps through transfer. Subsequent-floor loading does not expel disconnected roster members. Native portals are disabled in rift instances; voluntary exit remains the existing durable leave operation.

## Recovery and compatibility

- Durable payload version remains 1, with optional `Floor`, `StorageRevision` and `DeathTimes`. Legacy nil-Floor records still decode and can settle/receive existing interruption handling; only newly opened runs require the validated floor catalog.
- No SQL migration, new items or new art assets are needed. Existing run JSON storage carries the additions.
- The envelope remains `endgame.v1`; the DTO gains seed, generatorVersion, floorIndex, mapName, exit coordinates, canAdvance, transitioning, maxTier, unlockedTier, aura affixes and telegraphs. `rift.advance` requires runId, floorIndex, a unique request ID and the ordinary character snapshot expectedRevision. That revision is not the continuously changing run combat revision.
- The client uses a strict DTO parser. **Deploy this client and server together.** Older clients cannot safely consume the expanded rift projection. Native positional/map/combat packet formats are unchanged.
- Reconnect finds the current live admitted floor even if the last character save points at an earlier one. It never lazily recreates a lost rift. For an interrupted ordinary-map instance with no live run, login falls back to its public map at the saved valid location; the original sponsor can use existing one-time compensation.
- Restart does not resume live monster simulation. Saved completed grants remain claimable; a missing nonterminal run follows the established one-time sponsor replacement-key policy. Saving the plan is evidence and deterministic recovery input, not a claim that combat resumes after restart.

## Verification and boundaries

Pure generator tests cover stable seeds, ordering independence, changed floors, no adjacent repeat with alternatives, valid map/entry/exit/population and persisted round trips. Collision tests cover disconnected regions, invalid/missing masks and blocked anchors. Scene tests cover source/target ownership, save-before-publish, lost ACK, retry identity, target failure, stale/duplicate requests, party/reconnect/death cooldown, shared progress/deadlines and no repeated globes/deaths. Ability tests cover each shape, dodge, guardian rotation, pause/rollback, warning recipients, high-tier overflow and reflection-killed casters. Protocol tests cover closed request schema, missing identity, floor guards and tier150/151 boundaries. Client tests cover strict DTOs, commands, UI and plugin API projections.

Source and automated test evidence do not replace a Unity graphics acceptance run. Remaining real-game checks: map rendering/aliases, entrance and exit navigation on representative assets, camera-scaled warning geometry, latency/readability, party transitions and actual high-tier combat balance. No Unity Player build or live deployment was performed in the cloud.
