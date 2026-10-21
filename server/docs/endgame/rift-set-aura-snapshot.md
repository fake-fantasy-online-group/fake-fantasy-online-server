# Rift gates blocked by set-aura snapshot identity

## Failure and scope

Four-piece class set auras use internal synthetic source keys `-930001` through
`-930005`. `equipmentAuraViews` formerly exposed those keys as `sourceUID` in
production snapshots and combat-stat updates. The released client's item identity
parser accepts nonnegative canonical decimal strings only. It rejects snapshot
chunk zero with `Nondecimal identity: sourceUID`; subsequent chunks cannot finish
the interrupted snapshot. Physical rift gate intents wait for a current completed
snapshot, so a visible gate never sends `rift.advance`.

This depends on wearing a set or receiving a teammate's set aura, not on a map's
family, quest, level, native portal destination, or leadership requirement.
Huang Lei Island uses IDs 23800, 23900, 24000, 24100, 24200, 24300. Native family
stone monster 30069 occurs on quarry maps 933–937. All eleven are among the 189
native map plans in the existing layout audit.

## Minimal compatible repair

Only the wire projection for validated class-set auras uses `sourceUID: "0"`,
meaning no single source inventory item. Distinct aura ID/family and source entity
remain unchanged. Internal negative source keys, eligibility, leases, combat
values and removal behavior are unchanged. Actual item-backed aura UIDs retain
their exact positive values, including values above JavaScript's safe integer
range. No map, admission, guardian, economy or ordinary portal rules change. No
migration or client rebuild is needed.

The released client's aura consumer accepts zero and does not use this field for
inventory lookup, mutation or deduplication; it is exposed only in read-only
plugin observations. Do not relax the general identity parser or apply this
projection to skill variants, which require actual nonzero item identities.

## Verification

`TestClassSetAuraSnapshotWireIdentities` exercises production snapshot splitting
and encoding for all five wearers and a teammate without any set, each with 40
bag items and unique positive inventory IDs above 2^53. It verifies five separate
aura views and preserved internal source binding. Set
`CLASS_SET_SNAPSHOT_FIXTURE_DIR` to export the exact snapshot chunk JSON arrays.
The unchanged code fails all 30 aura identity assertions; the repair passes.
The companion client contract consumed these actual before/after arrays through
its production parser, state and physical gate implementation: broken snapshots
never became ready and sent zero advance requests over 100 frames; corrected
snapshots completed and sent exactly one request from the waiting gate intent.

`TestRiftAllAuditedMapsSnapshotAndMemberHandoff`, with
`RIFT_PORTAL_AUDITED_PLANS` pointing at the existing native-plan audit JSON,
exercises protocol decode, production request dispatch, receipt lookup, a
set-wearing non-opener, simultaneous member requests, one durable floor CAS and
private scene handoff for all 189 source maps, including all eleven above.
The destination actor uses the existing deterministic test layout. This is not
an online database inspection or a Unity/device rendering test, and does not
claim that unrelated static scenery is a functional native gate.
