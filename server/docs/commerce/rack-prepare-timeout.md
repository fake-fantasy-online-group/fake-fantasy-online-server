# Rack purchase preparation timeout

The session waits up to five seconds for the scene's prepared purchase/refund.
Previously it abandoned the reply on timeout, even if the scene subsequently
prepared or had already buffered a candidate. That candidate had changed currency
and inventory and set RackBusy, but no session remained to persist/finalize it.

`RackRequest` now makes cancellation and result publication a single atomic
handoff. Before publication, timeout wins and the actor skips preparation or rolls
back the unowned candidate. After publication, the session must drain the fresh
buffered reply and continue its existing transaction/finalization path. Actor
publication never waits for session I/O. Cancellation never cancels an already
handed-off database transaction. Purchases and refunds use the same handoff.
A warning records preparation timeout with character and scene identifiers.

Regression coverage: pre-prepare cancellation, cancellation at the prepared/result
boundary for purchases and refunds, restored currency/inventory/RackBusy, retry,
published-result ownership, missing consumer rollback, pet removal, concurrent
handoff ownership, and session timeout/result draining. Run:

    go test -race ./internal/game/scene ./internal/session -run TestRack -count=1

An isolated pre-fix baseline (7940846) reproduced an orphan purchase with no result
consumer: initial Caiyu100 became90, one item appeared, and RackBusy remainedtrue.
The patch's tests pass with both preparation outcomes covered.

This proves a prepare-timeout defect, not the cause of a specific reported client
hang. Purchase settlement still waits on the existing writeback transaction in the
session input handler; live database waits, errors and client logs are required
to diagnose a separate settlement or rendering stall. This patch does not alter
transaction reconciliation, finalization routing, or writeback semantics.
