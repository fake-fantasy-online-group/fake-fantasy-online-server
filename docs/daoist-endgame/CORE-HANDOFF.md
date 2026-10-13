# Protocol and persistence handoff

Rebuilt source validation, 2026-10-11 UTC. Earlier workspace test evidence is not counted.

## Implemented boundaries

- Dedicated negotiated protocol v1, bounded strict JSON, lossless string UIDs/revisions, native/auth compatibility.
- Shared DTOs and asynchronous domain backend; no Store dependency in scene.
- Unique overlay metadata on existing UIDs; ordered rune/socket instance facts reused.
- Atomic snapshot/wallet/state/run/receipt transaction with character and run locks.
- Durable request deduplication, exact receipt replay, digest conflict detection, CAS.
- Lost-ACK reconciliation stays inside WriteBack serialization. Unknown outcome stays pending; baseline acknowledgements are preserved after proven rollback.
- Full authoritative snapshot recovery on newer durable revision; no stale overwrite or candidate callback.
- Error/panic rollback for the complete in-memory store aggregate, including account wallets.
- Additive migration50 and separate existing-database upgrade SQL; frozen trial reward/completion fields persisted.

## Regression evidence

Passed rebuilt-source command:

```
go test -race ./internal/store ./internal/protocol ./internal/session ./internal/game/scene ./internal/arch
```

Tests include all four transaction fault boundaries, replay/CAS/conflict, lost ACK with outage and recovery, run checkpoint replay, snapshot isolation, panic rollback, invalid unique metadata, coalesced SaveAck preservation, strict codec bounds/duplicate/case-alias rejection, capability/map barrier, reconnect receipt before ownership/revision, disconnect cleanup, frozen final save with deferred reward, exclusive authoritative recovery, shared Character pointer preservation, asynchronous revision recheck, and shutdown commit drain.

`protocol/testdata/endgame` holds six small synthetic JSON plus hexadecimal-wire fixture pairs. `TestEndgameInteropFixtures` compares them against production encoding. The client also consumes production-generated text fixtures; no generated binary blobs are required.

## Integration cautions

- Keep the original Character pointer when applying recovery: session shares it.
- Matching replay plus RequiresReload is a successful stored receipt, not a failed operation.
- Never run candidate success callbacks after authoritative recovery; release transient reservations separately.
- One run-mutating checkpoint/claim transition should be in flight per actor.
- Uncommitted snapshot candidates must never enter ordinary autosaves. Disconnect/final-save leases remain frozen until the durable result.

## Verification limit

No live or disposable PostgreSQL migration was executed in this rebuild. SQL integration tests requiring a PostgreSQL endpoint remain unverified; passing in-memory/unit/race suites is not a claim of PostgreSQL round-trip validation. No deployments, commits, or pushes were performed by this lane.
