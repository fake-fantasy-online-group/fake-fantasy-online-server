# Native portal ownership, using the existing protocol

## Scope and source of truth

This change builds on server `27291d0` and the restored client flow in `8066014`.
No opcode, packet field, capability, or client map-transition control flow changes.
It fixes the remaining ordinary-portal crossing: the server used to act on Move
before the client's existing ready arrived, sending `0x803a` into an already
built target. The earlier receipt fix only covered a ready already received.

The ordinary client gate calls local `SwitchMap` without a preceding target
request or forced movement flush. Periodic `SavePos(entered=0)` still names its
current source map. Its first target message is `0x100a entered=1` after the
world has been built. A last movement point inside the polygon is not guaranteed.

Migration 65 records ownership from audited repository provenance, not a proc-ID
range and not a claim about unseen local files. Its exact original seed
fingerprints mark 457 configured ordinary gates as client-owned (26 same-map).
The five explicit server-only exits added by migration 42 remain server-owned.
Scripted gates, modified/unrecognized rows, and future insertions default to the
server. The loader preserves this into `domain.Teleport.ClientOwned`; nonempty
scripts and dungeon-script entrances cannot accidentally become client-owned.
No complete client JSON export is needed for this bounded provenance migration.

## Existing packet sequences

- Client-owned ordinary gate: Move may update source position, but does not move
  scene ownership. After successful local build, existing entered=1 requests the
  normal handoff. The first destination snapshot follows, without `0x803a`.
  Missing/failed local build sends no ready, leaving source ownership usable.
- Server-owned gate, NPC/item/GM/script/rift teleport: retain the existing
  authoritative `0x803a`, then wait for the destination's entered=1 and send the
  first snapshot. No fixed delay substitutes for readiness. The target actor additionally
  checks the existing ready landing against its authoritative position using the
  real I32 wire → Single storage → I32 ReportPos conversion. An old local ready
  for a different landing cannot release a crossed GM/NPC transfer snapshot.
- Same-map native gate: the settled source SceneID/connection entity/epoch and
  exact configured client-owned landing identify the existing ready. It reuses
  combat interruption, old-event flush, force movement, pet/passenger movement,
  and landing suppression, then sends the snapshot without `0x803a`.
  The final epoch check is atomic with advancing the local epoch. Already-landed
  rebuilds do not perform the movement side effects again. Private-instance
  same-map resynchronization remains its previous behavior.
- Normal same-map rebuild: an unknown/different reported coordinate does not
  move the player. It still requests the existing full snapshot. A later genuine
  rebuild cannot be globally deduplicated just from the unchanged wire packet.
- Village selection and other existing client-first cross-map routes remain
  supported without requiring a geometric gate. This change does not add a new
  distance/anti-cheat rule or claim that the old compatibility path validated one.

## Source rows in flight

Independent production Session/Router reproduction showed an old source ground
item surviving the first destination snapshot: another source player dropped an
item after the client had built locally but before its ready reached the server.
The actual sequence was source `0x8013`, target `0x8013`, with no source removal.

Player/monster/pet spawns carry their native map guard. Ground and NPC rows do
not. Cross-scene transfer now emits only the source's live network ground/NPC
IDs as `0x8014` / `0x8047`, using the existing NPC-publication skip rule, and
flushes them before handoff. Thus their delayed spawns are followed by removals
before the first target snapshot. Local player, static scenery, and the other
entity registries are untouched. Same-scene movement does not clear these rows.
Destination IDs come from the shared segmented entity allocator.

## Verification entry points

- `server/internal/session/native_portal_ready_test.go`: real Session/Router,
  independently controlled source/destination actors, same-TCP FIFO with actor
  interleavings, native/server-owned and same/cross-map cases, no final Move,
  duplicate pending ready, empty destination, failed target, disconnect, A→B→C,
  exact same-map coordinates, final epoch rejection, and complete peer/NPC/
  monster/ground first snapshots. The late-source test checks identity and
  kind-specific removal ordering, not merely packet counts.
- `server/internal/session/native_portal_trace_test.go`: exports actual packet
  batches for the client production-iterator replay. It distinguishes true
  second authoritative transfers from the removed duplicate native load.
- Existing `client_map_ready_test.go` and `handoff_order_test.go` retain coverage
  of source receipt, target Enter-before-ready, panic recovery, bounded queue
  progress, queue rejection, instance/reconnect ownership, and submitted-ready
  state. No wire identities or timing heuristic were introduced.
- `server/internal/data/teleport_ownership_test.go` and
  `server/tests/sql/validate_portal_ownership.py`: persisted ownership, all five
  server exits, unchanged route data, default/modified rows and idempotent replay.

Aggregate and independent race results are recorded by the final review for the
exact final source. Intermediate focused passes do not substitute for those runs.

## Unchanged-wire limit

A genuinely independent forced teleport can coincide with an in-flight local
ready for the identical map and identical integer landing. Those original
packets contain no operation identity. The coordinate check does not distinguish
that case. The complete real packet/coroutine trace nevertheless recovers: the
late genuine GM teleport causes its second requested build, that build emits
its normal ready, and the existing same-map resynchronization sends a complete
snapshot again. The first snapshot can be dropped while switching; this is a
temporary visibility interval across two independent operations, not permanent
entity loss or an extra load caused by an ordinary gate response. No removed
client compensation ACK is needed. Do not suppress the legitimate forced
teleport or add a speculative delay to hide this distinction.

## Six requested travel categories

| Category | Actual entry and cloud evidence | Boundary |
|---|---|---|
| Dungeon entry/exit and floors 1/2/3 | `TestNativeDungeonReadyThreeFloorsAndNondefaultReturnLandings` uses production-loader configuration for Sky Spring 20801/20802/20803, NPC menu's client-first ready, same private instance and exact reverse-link landings, then public exit. `TestNativeTrialMenuCommandsReachReadyTerminalState` additionally exercises actual 0x1032 NPC context and 0x1006 trialtask/trialgo/trial/trialleave through all three authoritative-ready boundaries, with real PostgreSQL entry/exit data, fee/quest/party failures, duplicates and complete peer/NPC/monster/drop snapshot. Original dungeon/rift suites remain required. | This does not implement missing hard-mode commands/rules. Legacy trial commands are covered as server-driven entries; the dedicated rift flow retains its own tests rather than being relabeled an ordinary native gate. |
| Wilderness transport point | `TestNativePortalWildernessTransportActualOpenChoosePackets`: actual map4/NPC3/list6, 0x1071→0x1072, rejects unopened/invalid selection, consumes duplicate selection once, waits for correct landing ready, sends all four visible entity kinds. | Samples the shared transport handler; not a manual play-through of every list. |
| Physical gates | `TestNativePortalFIFOReadyDriven`, actual DB-loader portal contract, cross/same-map, no final Move, reversed actor timing, duplicate pending ready, build failure/no ready, preserved server-only gates. | Initial gate suppression remains the client's authority; a rounded server movement hit is not a transfer. |
| New-character village selection | `TestNativePortalVillageMenuKeepsClientFirstWithoutGeometry`: actual four configured village maps/landings 61/37/62/38, duplicate pending ready, one snapshot, no forced teleport. | Existing menu compatibility is preserved without invented polygon requirements. |
| Return to town | `TestNativePortalReturnItemsActualUsePacket`: actual 0x1015 for 3381 and10025, actual919→58 return; checks consumable quantity, duplicate queued use, missing consumed item, infinite book's real same-map reuse. `TestNativePortalDeathReturnActualRevivePacket` exercises0x1018 from alive/dead states and one ready/snapshot. | Dead state is fixture setup; combat death itself remains covered by existing revive/combat tests. |
| GM teleport | `TestNativePortalGMForceRejectsCrossedDifferentLandingReady`: real0x1006 goto, same/cross-map and crossed old ready. `TestNativePortalForcedReadyUsesNativeIntegerFloatRoundTrip` decodes real803a to test fractional/large coordinates. `TestNativePortalGMIdenticalLandingRestoresAfterSecondRealBuildReady` tests complete same-landing terminal recovery. | Two genuinely independent same-landing operations may produce two real builds. The unchanged wire cannot assign them an operation ID. |

`native_server_travel_test.go` uses the actual bag page/slot mapping for network
item packets, not the global slot zero used in some internal scene fixtures.
All new source changes were frozen before the final aggregate; added tests are
validated separately against that same production hash manifest.

### Trial fixture provenance

`testdata/native_trial_routes.json` was read through production `LoadMonsters`,
`LoadDungeons`, `LoadTrial` and `LoadNPCs` on the isolated migration-65 database.
It retains the real quest233, level12–100 range, claim/guard NPCs, 61→41 landing
(3341,4135), price5000, dungeon21100 landing (9581,4985), and exit7(7424,5922).
The file samples one real normal and one real boss spawn and the corresponding
level12/15 candidates; the lifecycle test explicitly reduces counts from25/2 to
1/1. It is not a density or reward-balance verification. Normal trial spawning
rules themselves are unchanged. The test opens actual NPC context packets and
uses actual menu command packets, including task acceptance, rather than setting
QuestNPC or bypassing the entry with a synthetic Teleport.
