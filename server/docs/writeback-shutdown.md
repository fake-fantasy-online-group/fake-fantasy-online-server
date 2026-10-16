# Writeback isolation and shutdown

Writeback schedules one active operation per character. Two-character settlements
reserve both character lanes and start only after their older writes finish.
Unrelated characters continue while a slow save or receipt reconciliation waits.
The queue coalesces ordinary snapshots and preserves full snapshots when a newer
position-only update arrives. Pending counts report unique unresolved characters.

Each normal write/transaction attempt receives a 10-second context. Context expiry
does not prove a transaction failed and does not forcibly release its ownership
lane. Mail and endgame mutations reconcile their durable receipts before returning.
Trade and stall settlement use the narrower CommitSnapshots API: only absolute
snapshot replacements are replayed after an ambiguous COMMIT. Both owners remain
reserved until a later successful transaction proves the exact candidate durable.
A later failed retry cannot disprove an earlier unknown COMMIT. Generic mutation
callbacks are never automatically replayed.

Concurrency is bounded by ownership lanes, not a fixed global worker pool:
at most one running callback per participating character (a pair uses one callback).
PostgreSQL also retains its bounded connection pool. A database-wide outage can
still suspend all affected owners; this is intentional rather than granting assets
or admitting a conflicting login with an unproven outcome.

The -drain budget covers gateway handler cleanup, scene stop/final snapshots,
writeback drain, and connection-pool close. A timeout returns a nonzero process
status and reports incomplete work. It must never be interpreted as a successful
save or forced refund. On failed draining the process does not wait indefinitely
again in pool Close. Ordinary in-memory progress is not a durable journal: if the
shutdown budget expires while the database is unavailable, uncommitted snapshots
can still be lost. Operators must treat the nonzero exit as an incident.

Tests use in-memory/fault-injection stores, including blocked writes, commit-then-
connection-error, later failed retries, same-owner ordering, independent-owner
progress, pair ordering, snapshot merge, and blocked pool close. No production
database availability or throughput claim is made by these tests.
