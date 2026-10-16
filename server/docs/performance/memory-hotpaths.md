# Memory hot-path optimizations (2026-10-16)

Baseline: server-runtime-reliability commit 095a7b9. Changes preserve game rules and wire format. Measurements are cloud amd64 microbenchmarks on an AMD EPYC 9V74 shared host, not production capacity estimates. No hardware cache-miss counters were collected.

## Packet framing

Each connection reader and writer owns separate AES/HMAC setup state. SetKeys clones input key slices; pointer replacement invalidates cached setup. The outbound sequence, plaintext and padding are assembled in one owned final frame. Each inbound frame retains its unique allocation and is authenticated before in-place decryption. No global pools or retained plaintext buffer reuse are introduced. Fresh random IVs, constant-time MAC verification, packet sequence behavior and key changes remain intact.

GOMAXPROCS=4, three 300ms samples, median nanoseconds:

| Payload | Send before/after ns | Send B/op | Send allocations | Receive before/after ns | Receive B/op | Receive allocations |
| --- | --- | --- | --- | --- | --- | --- |
| 32 B | 1147 / 574 | 1848 / 624 | 14 / 2 | 1123 / 473 | 1680 / 608 | 10 / 2 |
| 256 B | 1990 / 961 | 3064 / 864 | 14 / 2 | 1264 / 741 | 2144 / 832 | 10 / 2 |
| 4096 B | 13323 / 10358 | 21528 / 5376 | 14 / 2 | 8103 / 6686 | 11264 / 5376 | 10 / 2 |

Receive includes copying into owned frame storage in both cases; it differs from an Open-only benchmark. Excludes protocol encoding, sockets and queueing. Retained payload, bidirectional key replacement, malformed packet and compatibility tests accompany the changes.

## Monster pathfinding

Typed heap replaces interface-boxed heap nodes; ordering, tie breaks, expansion bounds and collision/path simplification behavior are preserved. No shared reusable search scratch is introduced.

Five 250ms samples:
- Detour: 154 to 38 allocations/search; 30368 to 24824 B/search.
- Unreachable: 586 to 46 allocations/search; 121312 to 95416 B/search.
- Median times approximately 81.6 to 74.7 microseconds and 356.9 to 323.3 microseconds respectively. Timing noise means allocation reduction is the stronger result.

Tests compare randomized heap output and complete paths against the prior implementation.

## Consumed scene events

After delivery, clear consumed pending entries before truncating the outbox. This releases high-water array references without clearing event payload buffers retained by sinks. The test inspects backing slots and verifies retained payloads remain intact. This addresses retained heap; no CPU speedup is claimed for the clear operation itself.

## Pending snapshot coalescing

The first full-plus-position merge deep-clones as before. Subsequent movement updates can modify only that private, undispatched clone under the existing mutex. Ownership is revoked before writer dispatch, aggregate replacement and retry merging from an exposed snapshot. Public MergeSnapshots remains unchanged. Per-character ordering, pair mutations and acknowledgement semantics are preserved.

Delayed-writer parallel producer microbenchmark, GOMAXPROCS=4, three 300ms samples:
- Prior repeated merge: 14.4–19.4 microseconds/op, 132664 B/op, 6 allocations.
- Private pending clone reuse: 55.7–59.7 ns/op, amortized zero allocations.
- First merge still costs 132664 bytes/6 allocations. The large reduction applies only to repeated coalescing while a writer is occupied, not all saves or overall server throughput.

Tests cover alias isolation, chronological equivalence, both failed-write retry branches, callbacks retaining snapshots, pair mutations and acknowledgements.

## Reproduction and limits

Run tests from server with the repository Go toolchain. Comparative benchmarks live beside their relevant tests: codec_test.go, path_memory_test.go and writeback_coalesce_owned_test.go. Serialize benchmark runs and keep the same CPU concurrency across comparisons. Race tests and whole-package tests establish correctness separately from performance measurements. Database-backed integration completed on the baseline; this optimization batch adds no migrations and does not constitute phone/client gameplay acceptance.

## Second batch: status, AI and session hot paths

Baseline is the first batch above (remote 5e403d4). No protocol, gameplay rule, scan cadence or persistent cache is changed.

- Status suppression bookkeeping returns early only after the existing scan proves there is no suppression source. It avoids redundant scans and list compaction without changing callback boundaries, permanent/exclusive expiry behavior or final periodic ticks. Audited no-suppression bookkeeping improved about 41–45%; allocations were unchanged. Clean 300-target subsystem tests measured one poison at 0.420 ms, five mixed statuses at 0.763 ms; an artificial 32-poison saturation case fell from 102–104 ms to 71.657 ms. That artificial case is not a certified legal build or whole-frame benchmark.
- Monster acquisition finds the nearest candidate linearly, then sorts remaining candidates only when that target is unreachable. Distance/ID ties, retry state and unreachable fallback are preserved. For 64 candidates the audited prototype measured about 7.81 to 2.40 microseconds and 8 to 2 allocations. At eight candidates the small local buffer eliminated allocations.
- Single-target healing uses the same HP-fraction, distance and ID ordering to select its minimum directly. Area healing drops a health sort whose result was immediately discarded by an ID sort. At 64 candidates the audited single-selection microbenchmark measured 4.97 to 1.09 microseconds, 8 to 1 allocations; area selection 6.69 to 2.02 microseconds.
- Outgoing packet debug arguments are built only when Debug logging is enabled. Enabled log output and packet delivery are unchanged. A 100-observer movement-delivery microbenchmark measured roughly 20 to 8–9 microseconds and 600 to 200 allocations. This excludes full network transport.
- Inventory/equipment/warehouse diff maps store slice indices instead of copying entire views. Comparable fields use typed equality; inventory pet payloads retain pointer identity/deep equality handling. Duplicate slots, removals, output ordering, fallback snapshots and cloning remain unchanged. At 128 bag items plus 12 equipped items, a currency-only update fell from 98464 to 10464 B/op and 292 to 12 allocations; audited timing about 85–94 to 29–30 microseconds.

Second-batch audit timings used sequential isolated microbenchmarks (GOMAXPROCS=2, two 150ms samples) except the separately documented status scene cases. They establish local opportunities, not additive overall-server speedups. Production regression tests retain old algorithm references and test unreachable target fallback, chase side effects, duplicate slots, pet equality, payload ownership, suppression endpoints and final ticks.
