# Endgame extension contract v1

## Compatibility

C2S `0x10d0`, S2C `0x80d0`: little-endian U16 schema=1, then existing U16 byte-length UTF-8 JSON. Maximum JSON is 60 KiB. Native inventory rows, auth, and legacy `0x1091` remain unchanged. JSON request field names are case-sensitive, unique, and closed; unknown fields and trailing bytes are rejected. Exact DTO definitions: `server/internal/domain/endgame_contract.go`.

After entering the world and completing the map-load barrier, send `{"op":"hello","capabilities":["endgame.v1"]}`. Only negotiated sessions receive extension events. Capability resets on leaving the world. Gameplay requests still use session-owned entity IDs, never client-provided character IDs.

UIDs and revisions are canonical decimal strings (positive signed int64 UIDs; revisions permit zero). Run IDs are opaque bounded ASCII, not numeric. Mutation request IDs are 16–64 printable ASCII bytes. Supported operations: hello, snapshot, catalog, socket.insert, socket.remove, cube.quote, cube.execute, rift.start, rift.state, rift.claim, rift.leave. No product-only native craft overload.

Cursor absent/empty starts paging; empty nextCursor is terminal. Snapshot chunks/cursors are actor-owned. Client selection binds durable UID and expected character revision, not an old bag index. Item tab is native 0–3 with page-local slot. Result maps contain strings to avoid 64-bit number loss.

## Storage and mutation boundary

`Stack.Endgame` is server-owned metadata on the existing UID. UniqueID equals base ItemID in a separate namespace; potency is 8000–12000 basis points. Ordinary items have all-zero metadata. Rune identities and five ordered socket/card snapshots remain the existing durable fact source; rune operations do not need extra persisted fields.

`domain.EndgameBackend` is async and injected into Scene; game packages do not import Store. Constructor: `store.NewEndgameBackend(raw, writeBack)`. `Commit` synchronously clones Before and candidate. Caller retains inventory/lifecycle ownership until the result, including disconnect. No provisional wallet refund or bag publication is allowed.

One WriteBack-ordered transaction locks the character, checks the durable receipt before revision validation, CAS-updates state, saves the full candidate snapshot, optionally locks/CAS-updates the run, and inserts the immutable receipt. Receipt key is (character, requestId), bound to operation and SHA-256 digest. Same key/different digest is a conflict. Receipt lookup also precedes scene ownership/affordability validation for reconnect retries.

On ambiguous commit failure, the same WriteBack slot remains held while a new transaction acquires the same character lock and proves receipt presence/absence. DB unavailability leaves the result pending. Confirmed failure preserves Before only if its expected revision is still current. Coalesced baseline SaveAck waiters complete only after a durable outcome. Run checkpoints similarly lock a stable per-run advisory key and reconcile exact revision/state/JSON after a lost acknowledgement.

Newer durable state returns `RequiresReload` plus detached `Authoritative`. Apply the entire durable aggregate and returned ledger state while frozen; preserve the existing Character pointer shared with session. Entity runtime position/life/status/party remain actor-owned. Never autosave stale Before/candidate after this recovery. Matching replay returns the stored successful receipt; do not publish candidate or rerun success callbacks. Recovery without a matching receipt reports stale revision.

## Migration and verification

Fresh schema adds migration50. Existing DBs use the separately reviewed additive `docs/migrations/20261011_daoist_endgame_v1.sql` after a backup. New tables: equipment_endgame_state, character_endgame_state, endgame_receipts, endgame_runs. Existing characters gain zero-default frozen trial reward and completion ID. Existing item IDs/UIDs and native tables are not rewritten.

Memory.WithTx now restores all persistent maps/account wallets on error or panic, matching PostgreSQL rollback semantics. Focused tests cover codec limits/duplicates, capability gating, each transactional failure boundary, CAS/replay/digest conflicts, lost ACKs, clone isolation, invalid metadata, and coalesced writeback acknowledgements.

This rebuild has not executed a live or disposable PostgreSQL migration. PostgreSQL validation from the lost workspace is historical evidence only and is not claimed as verification of this rebuilt source.
