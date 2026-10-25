# Rift block replication and paced snapshots

This is server-only. No opcode, client schema, AI, authoritative combat, loot,
monster population, map geometry, or trap visibility rule changes.

## Visibility

Only dynamic monsters in a Rift use local visibility. The separate visibility
index uses floor-divided 300-world-unit blocks; the existing combat AOI keeps its
original coordinate semantics. Player subscriptions change only when their block
window changes; monster block changes independently update standing observers.
A snapshot alone enumerates the scene once. Normal movement does not scan every
monster for every player.

The fixed supported viewport baseline is 2560×1440 plus 600 units of padding.
With authoritative native map dimensions, subscriptions use 15×11 blocks around
the native-clamped camera center. Current production configuration does not
supply proven native GSM dimensions. MASK collision dimensions are **not**
assumed equivalent: the conservative fallback covers the full viewport width /
height plus padding relative to the player, using 23×15 blocks. Actual movement
credit (including the existing 600 ms reports, catch-up allowance, and two delta
ticks) can enlarge this window. More than 4096 subscribed blocks falls back to
full indexed visibility, not an unsafe radius clamp. Arbitrary unknown ultrawide
viewports are not guaranteed; there is no claimed client viewport negotiation.

NPCs, players/party, pets, gates, doors, drops, and global Rift state retain their
existing global semantics. In particular, `rift.state` retains the full Aura list
and existing native JSON/pagination contract.

## Lifecycle and dependencies

Known means that all native Spawn packets were accepted by the connection FIFO,
not merely queued in the scene. Planned snapshots, known entities, and removals
are separate. Event-time entity pointers identify incarnations; map-loading and
session binding/epoch fences cancel obsolete work. Reentry follows Remove(0)
then a new Spawn. Unsent lazy rows are reconstructed from current pose, HP,
statuses and presentation metadata. Dead unseen rows do not create standing
corpses. Routine state may coalesce only before the next critical reference to
that entity, including source/target references rather than Subject alone.

Relevant combat explicitly introduces required offscreen source/target entities
before a native event refers to them. Unrelated remote combat does not introduce
hidden monsters. The old client retains actor Transforms for delayed skill art;
without a proven maximum lifetime, dependencies are conservatively pinned until
despawn/death cleanup or map reset. This can reduce AOI savings during prolonged
ranged combat. Pins are current-incarnation / current-scene state, not a global
history. Combat authority and reach are not reduced to the replication window.

## Bounded, nonblocking delivery

Each player has a lazy initial entity snapshot cursor and a world-presentation
FIFO. Non-world/control facts (inventory, money, global Rift state, native
entry/teleport, and other existing controls) keep the original immediate `Emit`
contract and FIFO order. They never enter the cancelable entity queue. Ground
world effects, actor owners/ride anchors, target-state and child-skill references
are paced world presentations too; economic/session facts are not.

Initial rows are ordered nearest first once, after a local fence following the
native entry/teleport prefix. Initial admission is limited to 32 events per
100 ms tick. Live events have a separate 2048-event budget. Per-player retained
encoding work is limited to 512 KiB per tick, with one larger existing event
allowed alone. The session encodes at most one pending event (1 MiB / 4096 frames)
and submits at most 32 frames / 64 KiB per call, except one existing large packet.
Spawn packet groups admit atomically. No scene goroutine waits for a socket,
timer, channel capacity, or another player's stream.

Entity-lane limits are explicit: 65,536 initial references, 16,384 staged event
records, 4 MiB retained event payload per player, and 30 seconds without transport
progress / for a critical presentation backlog. Progressing initial snapshots do
not have an arbitrary total-time cutoff. References to current entities are shared
with authoritative scene ownership; consumed/canceled queues clear their slots.
The connection remains at its original default 1024 frames. Paced entity admission
stops at a 4 MiB queued-byte watermark and reserves packet headroom for controls.
Normal `Send` retains the existing count-based admission and write-stall timeout;
controls can exceed the entity byte watermark, in which case entity admission
pauses until they drain. No new global byte limit, packet-capacity increase, or
unbounded additional queue is introduced.

This deliberately retains the legacy global-configuration boundary: pathological
control bursts exceeding the existing 1024 packet capacity, or snapshots beyond
the native per-frame contract, are not newly promised to work. A maximum supported
1 MiB raw Rift JSON snapshot uses 29 native pages / 1,403,014 inner bytes. Tests
also cover wire-legal large inventory, wardrobe and pet packets together above
4 MiB: those controls are not rejected by the entity watermark. Full Aura lists
and existing pagination are unchanged.

At a map transition, unaccepted scene work is canceled and generation-tagged
paced frames still waiting in the bounded connection queue are purged. Normal
control frames are never purged or reordered. At most one already-dequeued old
world frame can finish; the single writer completes it before later native reset
controls. Epoch purge is distinct from retrying a refused Spawn and uses only a
short bounded queue lock, never network I/O. This restores control capacity before
source NPC/drop removals (including roughly 1000 mapless rows) without increasing
the queue or delaying inventory/travel economics. Source removals remain immediate
mapless cleanup controls. No new deferred travel model or
consumptive-command guard is installed. Native same-map prebuilt worlds explicitly
remove admitted old rows before the replacement local snapshot, including rows
outside the new subscription. The session also validates entity/scene/epoch
binding through each paced transport admission.

## Validation

Focused tests cover 1160/4097 dense snapshots, five viewers including blocked
peers, 4097 moving entities and simultaneous status state, critical order,
incarnation reuse, reentry, fresh-drop presentation, nearest-first/fenced entry,
map rebuild/transfer, shutdown/cancellation, and retained memory bounds. Actual
TCP tests exercise Scene → real session event sink → Conn → encrypted native
frames with fast, throttled and non-reading peers. Additional TCP tests round-trip
all 1160 synthetic Aura rows and existing multi-page global responses unchanged.
The opt-in benchmark reads an external sanitized fixture through
`RIFT_REPLICATION_FIXTURE`; no private run fixture is committed.

Traffic reduction is not a blanket CPU improvement claim: maintaining exact
lifecycles and subscriptions adds bookkeeping. Dense same-block initial snapshots
still take multiple ticks (1160 roughly 3.6 extra seconds, 4097 roughly 12.8),
although healthy established players retain their separate live-event budget.
Under persistent transport pressure, coalesced movement may have a lower visual
update cadence; critical facts are retained or the genuinely overloaded/stalled
connection is closed, never silently dropped.
