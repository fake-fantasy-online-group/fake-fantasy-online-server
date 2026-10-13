# Seeded, multi-floor 演劫洞天

## Player-facing design

Difficulty is **第 1–150 阶**. Traversal is **本次洞天第 N 层**. Starting a run selects one unlocked difficulty; descending changes only the map layer, never difficulty, progress target, reward tier or entry price. A successful clear advances `max(1, floor(frozenRemainingSeconds / 180))` difficulty tiers; the personal maximum never decreases and caps at 150. Even a successful overtime clear before hard failure advances at least one tier. Floor count has no connection to the unlock ladder. See [current completion, experience, ground-reward and exit contracts](RIFTS.md#rewards-and-exact-identities).

A single opener pays one bound key for the frozen party. Every run has one 15-minute timed target, 20-minute hard limit, shared 10,000-point progress counter and five-second death penalties. New floors target 96–160 monsters, scaled to verified connected walkable area with seeded variation; geometry-constrained maps use only available safe positions. Encounters form nearby groups of 4–8 rather than scattering isolated enemies. Blue elites remain 6/9/12 and yellow elites 2–4 per ordinary floor (very small fallback plans retain the existing adaptation). Elites supply progress globes. The shared 10,000-point target and per-monster progress remain unchanged; a dense floor can fill the bar without a forced minimum floor count. Reach the marked exit or any usable native map gate to skip an unpleasant layout or surviving pack; the opener advances the whole present party. No minimum floor kill quota. Uncollected globes and surviving enemies are abandoned. Offline admitted members reconnect to the current layer, not the abandoned one. No previous-layer entrance exists.

At full progress, remove trash and globes and summon one guardian on the current floor. Floor advance is disabled during guardian combat. One run has one final reward settlement, not one reward per floor. Exact bound grants remain authoritative: after durability, guardian experience settles automatically, saved item stacks appear as personal native ground rewards, and 归元道使 provides a durable exit. Atomic pickup preserves bag-full retries; bulk `rift.claim` recovers only outstanding items/experience after the live instance is gone.

## What is generated

This version generates a **seeded sequence of existing maps plus a seeded population and encounter plan**. It does not generate new terrain meshes, tile art, collision geometry or scene artwork. This distinction is intentional: native map loading, collision, movement and resource IDs remain intact.

At server startup, the immutable floor catalog filters existing non-city maps with real hostile monster assets. It loads the native collision MASK and uses four-neighbor flood fill to retain one connected component of valid native spawn anchors. Missing collision/resource definitions and special island map aliases are excluded. Entry, exit and guardian use real anchors on that component. Population positions use a globally aligned 64-pixel lattice, within 160 pixels of native hostile anchors, with each candidate checked against the same flood-filled component. No random coordinate jitter can place monsters in walls or on disconnected islands. A 192-pixel neighborhood around each seeded pack center supplies nearby positions; the fixed lattice prevents overlapping spawns. Minimum anchor count, separated entry/exit and a protected entry radius are enforced. Startup catalog failure disables new admission before any key is spent.

A static audit of shipped SQL and gzip MASK resources identified 189 candidate maps, including 桃源, 仙踪森林, 千叶森林 and 沙角. This is not a production runtime or rendered-map count: runtime checks loaded definitions, hostility and attackability too.

Generator version 2 uses SHA-256 over the versioned run identity and floor index, then an explicitly implemented SplitMix64 stream. Map and monster pools and anchors are sorted before selection. Adjacent floors avoid repeating a map when alternatives exist. Combat randomness does not consume the layout stream. The decimal UInt64 seed in the UI is the current floor seed. The complete admitted plan is persisted; recovery never regenerates a paid floor against a changed catalog. Saved generator-version-1 plans remain valid with their original population (at most 64), seed, positions and encounter state; only new plans use the denser version-2 rules.

The version-2 population target is connected walkable area / 448² pixels, clamped to 96–160 before and after seeded 90–110% variation, then limited by safe candidates. Entry keeps a 160-pixel enemy-free radius and exit an 80-pixel radius. Difficulty 1–150, monster stats, ordinary XP/loot suppression, progress values, final rewards and equipment rules are unchanged. The immutable catalog is built once at startup; no resource upload or SQL migration is required.

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

1. Validate current run ID and observed floor index; require the living opener within 160 native pixels of the current exit or a server-loaded gate polygon, active phase, ready map, and no conflicting run/character economic write.
2. Build the next immutable plan and reserve an **unpaid** destination actor.
3. Clone run state; retain tier, timer, progress, roster and grants, but retire all old monster/globe IDs. Save the next floor with revision CAS. No inventory/key mutation occurs.
4. Freeze source combat while the write outcome is unresolved. On a failed acknowledgement, read the durable run. If unchanged, discard only the unpaid target and resume the same source floor. If committed, publish that exact plan. Unknown outcomes retain ownership and retry the read.
5. Atomically retire source admission and publish destination admission after durability. Transfer all present admitted players using native map-load and ownership handoff. Existing-only rift handoff prevents a stopped target from lazily recreating an ordinary scene. A prepared actor lost before publication is replaced with the same saved plan.
6. Stop the retired source once players leave. Neither old deaths nor globes can contribute again. Replay with the old floor index cannot skip a second floor.

The shared wall-clock deadline includes loading and persistence waits. Dead members retain their revive timestamps through transfer. Subsequent-floor loading does not expel disconnected roster members. Native portal destinations and scripts are disabled in rift instances. Their server-loaded geometry instead admits the existing `rift.advance` operation; no ordinary map is entered first. Voluntary exit remains the existing durable `rift.leave` operation.

## Recovery and compatibility

- Durable run payload versions 1 and 2 remain supported; new runs use version 2. Optional floor/recovery fields and the current completion fields remain in existing JSON storage. Legacy nil-Floor records still decode and honor saved grants or existing interruption handling; only newly opened runs require the validated floor catalog. Already saved experience is not recomputed, and missing legacy completion timestamps use the conservative progression fallback described in [RIFTS.md](RIFTS.md#saved-record-and-protocol-compatibility).
- No SQL migration, new items or new art assets are needed. Existing run JSON storage carries the additions.
- The envelope remains `endgame.v1`; the DTO gains seed, generatorVersion, floorIndex, mapName, exit coordinates, canAdvance, transitioning, maxTier, unlockedTier, aura affixes and telegraphs. `rift.advance` requires runId, floorIndex, a unique request ID and the ordinary character snapshot expectedRevision. That revision is not the continuously changing run combat revision.
- `endgame.rift-portals.v1` is negotiated separately in `hello`. Supporting clients receive a lightweight `rift.context` result with `inInstance` as a boolean string, derived from the actual scene actor, not its reused map ID or the remembered run. Private context precedes native map entry; public context follows the authoritative teleport so an old-world gate cannot race the exit. Initial login repeats the context after negotiation. It remains private during durable leave until the destination actor takes ownership. The capability survives runtime handoff and resets on reconnect; the existing strict rift DTO does not gain a new key.
- The client consumes native gate edges while its initial context is pending, sends one ordinary `rift.advance` request for a ready active run, and waits for the server teleport. Loading, pending mutation, guardian combat and terminal phases never fall back to the original map or script. Leaving/re-entering a rejected gate rearms it; run/floor guards and the existing single transition prevent duplicates. Old servers without the capability keep normal map behavior; an unanswered initial handshake has bounded retries and cannot disable ordinary gates forever. Once a private context is confirmed, a delayed run view or failed refresh cannot clear it.
- The client uses a strict DTO parser. **Deploy this client and server together.** Older clients cannot safely consume the expanded rift projection. Native positional/map/combat packet formats are unchanged.
- Reconnect finds the current live admitted floor even if the last character save points at an earlier one. It never lazily recreates a lost rift. For an interrupted ordinary-map instance with no live run, login falls back to its public map at the saved valid location; the original sponsor can use existing one-time compensation.
- Restart does not resume live monster simulation. Saved completed grants remain claimable; a missing nonterminal run follows the established one-time sponsor replacement-key policy. Saving the plan is evidence and deterministic recovery input, not a claim that combat resumes after restart.

## Verification and boundaries

Pure generator tests cover stable seeds, ordering independence, changed floors, no adjacent repeat with alternatives, valid map/entry/exit/population, area-scaled density, dense packs, unchanged elite counts, version-1 compatibility and persisted round trips. Collision tests cover disconnected regions, invalid/missing masks and blocked anchors. Scene tests cover source/target ownership, save-before-publish, lost ACK, retry identity, target failure, stale/duplicate requests, party/reconnect/death cooldown, shared progress/deadlines and no repeated globes/deaths. Ability tests cover each shape, dodge, guardian rotation, pause/rollback, warning recipients, high-tier overflow and reflection-killed casters. Protocol tests cover closed request schema, missing identity, floor guards and tier150/151 boundaries. Client tests cover strict DTOs, commands, UI and plugin API projections.

Source and automated test evidence do not replace a Unity graphics acceptance run. Remaining real-game checks: map rendering/aliases, entrance and exit navigation on representative assets, camera-scaled warning geometry, latency/readability, party transitions and actual high-tier combat balance. No Unity Player build or live deployment was performed in the cloud.

### Density audit (generator version 2)

The checked-in SQL/MASK static catalog has 189 candidate maps. Connected walkable area ranges from 1,935,600 to 146,665,300 pixels² (median 27,213,600); the new verified population lattice has 231–5,007 positions per map (median 1,841). Across 100 seeds per map (18,900 complete plans), the actual Go generator produces 96–160 monsters, averaging 130.07, versus the prior generator’s 15–64 and average 49.03 on the same catalog and run/floor identities: 2.65× population. Every sampled plan passed validated-position membership, unique coordinates and entry/exit safety checks. These are static fixture and server-generation results, not a rendered-game playtest or measured frame-time result.

Reproduce from `server/`:

```sh
python3 tests/rift_layout_audit.py --output docs/endgame/rift-layout-audit.json --templates-output /tmp/rift-density-static-templates.json
FFO_RIFT_LAYOUT_AUDIT_TEMPLATES=/tmp/rift-density-static-templates.json go test ./internal/domain -run TestRiftFloorStaticCatalogDensity -count=1 -v
go test ./internal/domain ./internal/data ./internal/game/scene -run 'RiftFloor|SeededRift|RiftLoginReconnect' -count=1
```

The optional template export is temporary audit input, not a checked-in replacement for the runtime collision catalog. Existing dungeon traversal remains available until progress reaches the unchanged guardian threshold. Live acceptance should compare near-entrance encounters, pack readability and frame rate on both narrow corridors and large outdoor maps.

### Native-portal regression commands

- Server: `go test ./internal/game/scene -run 'TestRiftNativePortal|TestRiftSceneContext|TestRiftPublicExitContext|TestRiftLegacyClient|TestRiftLeave|TestRiftFloor' -count=1` (from `server/`). Covers direct private-to-private handoff, frozen party/tier/progress/timer/seed, no additional key, stale replay, polygon distance, opener/guardian restrictions, source-only geometry, negotiation compatibility, and private/public packet order including the same map in a different instance.
- Client: `python3 Tests/Endgame/run-rift-portal-contract.py`. Executes the real context state, native gate trigger and rift interceptor against deterministic Unity substitutes. Covers ordinary/script gates, initial/reconnect context, duplicate/pending/rejected edges, guardian and terminal states, stale view after exit, unsupported servers and bounded hello timeouts. This is source-contract evidence, not a rendered Unity playtest.
