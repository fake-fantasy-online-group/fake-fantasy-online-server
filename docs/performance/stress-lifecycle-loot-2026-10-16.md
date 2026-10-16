# Scene lifecycle and grounded-loot stress, 2026-10-16

New tests: `server/internal/game/scene/stress_lifecycle_loot_test.go`.

Command (from server, after sourcing the shared toolchain):

```
GOMAXPROCS=2 go test ./internal/game/scene -run '^TestStress(LifecycleAdmissionTransferChurn|GroundLootBurstPickup)$' -count=1 -v
```

Exclusive timing slot, 20 samples per scale; fixture construction excluded. Timings include event flush to in-memory recording sinks. Allocation deltas use process-wide runtime counters around each batch; no parallel tests. No network, authentication, serialization, real persistence or database load. Manual scene/router handoff draining preserves production admission/teleport/leave code paths but does not measure goroutine scheduling or a real player's observed delay. Informational measurements, not machine-dependent pass/fail thresholds.

| Work per batch | Scale | Mean ms | p95 ms | Max ms | Bytes/batch | Allocs/batch |
|---|---:|---:|---:|---:|---:|---:|
| Enter+flush | 10 players | 2.698 | 5.064 | 5.229 | 5521191 | 1357 |
| Enter+flush | 50 players | 11.375 | 15.695 | 17.532 | 28392070 | 8720 |
| Enter+flush | 100 players | 19.770 | 22.372 | 23.446 | 59285705 | 22489 |
| Five equip/status rounds | 10 players | 7.314 | 8.236 | 10.772 | 1820107 | 10426 |
| Five equip/status rounds | 50 players | 45.284 | 56.271 | 61.000 | 9963692 | 52610 |
| Five equip/status rounds | 100 players | 87.033 | 91.951 | 96.595 | 22359194 | 105712 |
| Five transfers/player | 10 players | 4.346 | 5.175 | 8.405 | 1367905 | 6867 |
| Five transfers/player | 50 players | 33.152 | 41.113 | 45.284 | 10670848 | 44946 |
| Five transfers/player | 100 players | 72.455 | 79.839 | 90.600 | 30974242 | 115364 |
| Leave+flush | 10 players | 0.103 | 0.149 | 0.158 | 19648 | 143 |
| Leave+flush | 50 players | 0.910 | 1.307 | 2.906 | 160256 | 914 |
| Leave+flush | 100 players | 1.918 | 2.526 | 2.645 | 448258 | 1942 |
| Spawn loot+flush | 100 drops | 0.170 | 0.311 | 0.368 | 243097 | 656 |
| Spawn loot+flush | 500 drops | 1.252 | 1.964 | 2.565 | 1260441 | 3086 |
| Spawn loot+flush | 1000 drops | 2.209 | 2.973 | 2.991 | 2510527 | 6109 |
| Pick up+delivery | 100 drops | 2.357 | 2.542 | 2.840 | 743486 | 4313 |
| Pick up+delivery | 500 drops | 12.863 | 14.743 | 16.037 | 3721884 | 21520 |
| Pick up+delivery | 1000 drops | 24.212 | 25.316 | 25.347 | 7481793 | 43027 |

## Correctness and retention

All tests passed. Admissions initialized ready player-owned attributes and positive HP, with exactly N-1 peer spawn events per observer. Five equipment/status mutation rounds conserved the weapon and compared cached results with clean recomputation. Five transfers per player removed source ownership and created a distinct, ready destination cache attached to the destination entity. Player/entity/attribute dependency/raw skill/map-load/hit state counts were all zero after final leave.

Grounded loot emitted exactly one spawn and despawn per drop; every item reached the bag, duplicate pickup was rejected, and entity/player/cache state was empty after leave. Loot expiry callbacks remain queued after immediate pickup (one per original drop) until 1,800 logical ticks, 180 seconds. Tests verify they all drain at expiry. This is bounded temporary timer retention, not retained player ownership. No retained-heap byte claim is made: empty maps and allocated backing arrays may remain reusable, and allocation totals are not live heap sizes.

Raw output: `stress-lifecycle-loot-2026-10-16.log`.
