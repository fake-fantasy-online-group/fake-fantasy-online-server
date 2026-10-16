# Gateway resource budgets

These limits protect the game process. They do not replace upstream connection or DDoS protection. No wire opcode or password hashing cost changes.

## Defaults

`internal/net.Options` accepts explicit overrides; zero or negative values use defaults:

- Maximum accepted connections: 4096 globally, 128 per actual transport IP. No forwarded/client-supplied IP is trusted. IPv4-mapped addresses normalize to IPv4.
- Maximum simultaneous RSA handshake operations: 16. Excess attempts close instead of creating a waiting work queue.
- Absolute handshake read deadline: 10 seconds.
- Absolute post-handshake authentication lifetime: 2 minutes, unaffected by pings. A successful account login calls `MarkAuthenticated`.
- Each complete inbound frame, including idle wait/header/body: 2-minute absolute deadline. Trickling bytes does not reset this deadline.
- Per connection ingress: 100 frames/second with burst 200; 2 MiB/second with burst 4 MiB. Length accounting includes the prefix and occurs before allocating the frame body. Existing 1 MiB maximum frame remains.
- Output queue: 1024 packets; full queue closes that connection. Write deadline: 10 seconds, or configured `WriteWait`.
- Shutdown wait: 10 seconds unless `ShutdownContext` supplies a shared overall budget. Timeout returns an error: handlers may still own work, so callers must not infer successful cleanup.

`internal/session.Deps` allows `MovementRate`/`MovementBurst` overrides. Ordinary position reports default to 40/second, burst 80, before entering the shared scene mailbox. The actual companion client sends periodic movement every 300 ms; pre-combat flushes may run every 60 ms, and a subsequent stationary frame can send a stop report (`GameBootstrap.PositionReporting.cs`). The budget therefore admits two reports every 60 ms plus ordinary position saves, rather than relying on the older server comment about 600 ms. Scene-ready/transfer lifecycle receipts retain their existing path. Movement events are not deduplicated or coalesced; stop/facing/portal/correction ordering stays intact. Exceeding an ingress budget closes only the offending connection.

Production injects one shared `AuthWorkLimiter`. `AuthWorkOptions` defaults:

- 4 concurrent credential operations (login, registration, reset/change password, change security code, item unlock).
- Peer budget: 2 attempts/second, burst 30, shared across that peer's connections/accounts.
- Peer+account budget: 0.2 attempts/second, burst 5. Reconnecting does not reset it; another IP cannot spend an account's budget.
- At most 8192 retained hashed budget keys, expired after 10 minutes of inactivity. Cleanup occurs at most once/minute during admission; a full key map rejects new keys rather than growing. Sustained high-cardinality key churn can deny new peer/account combinations until keys expire; this fail-closed bound is not comprehensive distributed-abuse protection.

Credential admission failure occurs before account lookup/bcrypt. Pre-login attempts close the requesting connection; authenticated password/security changes and item unlocks receive their normal failure response so a legitimate burst does not log the player out. Existing password costs are preserved; relevant credential DB calls have 5-second contexts. Limiter saturation is not queued. Slow/faulty DB drivers must honor context cancellation for the DB timeout guarantee.

The 2-minute idle policy is a deliberate disconnect/reconnect boundary, including suspended clients that cannot keep their connection alive. It is not a promise that arbitrary mobile background sessions survive. The companion client sends a ping every 5 seconds while its Update pump runs (`Runtime/Protocol/NetClient.cs`, `PingIntervalSeconds` and `Update`), and already has a 30-second socket receive timeout. Its title login connects on button submission and immediately sends the captured credentials (`TitleFlow.AccountConnections.cs`, `BeginLogin`). Thus foreground idle/character-selection traffic fits the 2-minute budget. The client also caps total transmits at 45/second, below the 100/second server budget. Tune defaults using observed deployment traffic before rollout. Per-IP limits also need adjustment for unusually large shared NATs.

## Regression checks

`go test ./internal/net ./internal/session` covers silent/partial handshakes, stalled headers/bodies, frame admission before body reads, independent peer quotas, output deadlines, global/IP connection caps, incomplete shutdown reporting, auth concurrency/reconnect/retention, and normal movement cadence. Run the same packages with `-race` as well.
