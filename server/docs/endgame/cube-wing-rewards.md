# Cube wing rewards and guide

- First milestones: 10/30/50/70/90/110/130/150, fixed 20/60/160/400/800/1500/2500/4000 white feather units.
- Units reuse white feather IDs 25027/25028/25029 at 1/5/25. No virtual currency is persisted.
- Repeat clear: `1 + floor(tier/10)` units, deterministic denomination split. No black feather choice in routine packages. Separately, each eligible character receives one independent 0.5% roll per completed Rift for one 暗之羽 (11682), unaffected by MF or tier. The purpose-separated run/member roll is frozen into the normal recoverable grant.
- A character's first-clear ledger and exact frozen reward stacks commit in the same existing revision-CAS transaction. Skipped milestones are backfilled; farming does not reduce first-clear packages.
- Full bags cannot destroy rewards. Individual ground stacks and offline batches preserve exact remaining stacks, including disconnect/reconnect and failed writes.
- Existing paid runs retain their frozen older policy. Rift entry does not require the wing guide.
- After 问鼎周天, 巧匠左慈 offers 幻翼初展 (1900030002). Nearby 幻翼域使者 only turns in the task and grants one bound native 光之翼 (20609). Quest completion and gift commit atomically.
- Upgrade, conversion and extraction belong solely to the portable cube. There is no `wings.*` menu or new client packet.

## Validation

Focused Go tests cover first/repeat grants, rule snapshots, per-character backfill,
full bags, partial offline claims, repeated clicks, failed writes, lost ACKs,
disconnect/reconnect, guide atomicity and forged NPC contexts.
`tests/cube_wing_migration_postgres.mjs` executes verbatim migration SQL on PGlite
with reduced prerequisites, checking active configuration, old-rule preservation,
SQL rollback/replay, guide contents and three-slot power shape. It does not claim
a full SQL initializer, live database, Unity UI or deployment test.
