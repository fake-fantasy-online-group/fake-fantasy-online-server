# Bounded real-skill combat stress, 2026-10-16

## Scope and method

All 28 cases passed, 20 fresh scenes each (560 samples). New opt-in test: `server/internal/game/scene/combat_attribute_stress_scenarios_test.go`. Each scenario admits players through `Enter`, installs the real 745-row skill source with V5 progression, equips nine requirement-resolution fixture slots, and executes actual `UseSkill` for firebolt 10401 (single target) or fireball 10402 (area). The timer includes command validation, target selection, source snapshots, cast resolution, death/reward/drop processing, and event-sink delivery. It excludes fixture construction, admission, allocation-counter reads and correctness checks. No production code was changed by this worker.

A deterministic clock advances through windup and projectile arrival; elapsed results measure CPU-side processing, not player-perceived casting duration. Admitted cases do not manually warm the cache. `cold=true` deliberately invalidates already admitted players, so it is a forced dependency rebuild, not process-cold startup or admission cost. Death cases set monster HP to 1; nonlethal HP is 100 million. Eight guaranteed drops per kill intentionally stress reward processing. Equipment bonuses and monster population are synthetic. The equipped objects are identical shared fixture gear; this scenario does not exercise gear mutation or durability depletion.

Assertions verify exactly players×targets delivered damage events across player sinks (damage is personalized), each target damaged, target alive/dead states, completed cast queue, and exact XP and drop counts (one XP event/eight drops per kill). Multiple players are in one serialized scene, not concurrent goroutines. Fifty players share ten targets, producing 500 actual hits; no zero-work timing cases are included.

Hardware reported by /proc/cpuinfo: AMD EPYC 9V74 80-Core Processor; Go runtime sees 9 CPUs; GOMAXPROCS explicitly 2; Go 1.27.2, Linux cloud workspace, about 9.7 GiB visible memory. Actual host CPU quota was unavailable through /sys/fs/cgroup. Other benchmark workers were paused during the timing slot. Cloud scheduling/GC remain sources of noise. Twenty samples gives a coarse p95 (19th sorted sample), not a production tail-latency guarantee.

Excluded: sockets, binary protocol encoding, real database/save I/O, AI/movement/main-loop scheduling, client rendering, Mac/phone execution, and network latency. Fake sinks use mutex-protected event slices. Do not extrapolate this to supported real-player capacity or mobile/client performance.

## Results

Columns: scenario, average, p95, maximum, bytes allocated, allocations, real hits, XP events, ground drops, delivered sink events. All counts are per measured batch.

| Scenario | avg | p95 | max | B/op | allocs/op | hits | XP | drops | delivered |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| players1_targets1_lethalfalse_coldfalse | 73.68µs | 92.848µs | 136.202µs | 25568 | 87 | 1 | 0 | 0 | 6 |
| players1_targets1_lethalfalse_coldtrue | 580.94µs | 728.922µs | 1.111982ms | 368208 | 957 | 1 | 0 | 0 | 6 |
| players1_targets1_lethaltrue_coldfalse | 144.083µs | 205.765µs | 238.804µs | 59480 | 179 | 1 | 1 | 8 | 18 |
| players1_targets1_lethaltrue_coldtrue | 545.384µs | 790.463µs | 1.038912ms | 402120 | 1049 | 1 | 1 | 8 | 18 |
| players1_targets10_lethalfalse_coldfalse | 91.431µs | 108.27µs | 119.527µs | 44160 | 150 | 10 | 0 | 0 | 24 |
| players1_targets10_lethalfalse_coldtrue | 600.441µs | 791.125µs | 1.712284ms | 386801 | 1020 | 10 | 0 | 0 | 24 |
| players1_targets10_lethaltrue_coldfalse | 514.974µs | 823.368µs | 1.003215ms | 385931 | 931 | 10 | 10 | 80 | 144 |
| players1_targets10_lethaltrue_coldtrue | 987.438µs | 1.33968ms | 1.344197ms | 728571 | 1801 | 10 | 10 | 80 | 144 |
| players1_targets50_lethalfalse_coldfalse | 220.327µs | 307.747µs | 729.018µs | 117888 | 365 | 50 | 0 | 0 | 104 |
| players1_targets50_lethalfalse_coldtrue | 789.466µs | 1.26995ms | 1.844559ms | 460529 | 1235 | 50 | 0 | 0 | 104 |
| players1_targets50_lethaltrue_coldfalse | 2.046344ms | 2.755673ms | 3.035237ms | 1869594 | 4119 | 50 | 50 | 400 | 704 |
| players1_targets50_lethaltrue_coldtrue | 2.579873ms | 3.408718ms | 3.695884ms | 2212222 | 4989 | 50 | 50 | 400 | 704 |
| players1_targets100_lethalfalse_coldfalse | 309.189µs | 444.82µs | 575.089µs | 213200 | 622 | 100 | 0 | 0 | 204 |
| players1_targets100_lethalfalse_coldtrue | 815.626µs | 1.384296ms | 2.296231ms | 555841 | 1492 | 100 | 0 | 0 | 204 |
| players1_targets100_lethaltrue_coldfalse | 5.135578ms | 7.644745ms | 8.699787ms | 3682034 | 8088 | 100 | 100 | 800 | 1404 |
| players1_targets100_lethaltrue_coldtrue | 5.606798ms | 6.542812ms | 7.006857ms | 4024677 | 8958 | 100 | 100 | 800 | 1404 |
| players1_targets300_lethalfalse_coldfalse | 1.225676ms | 1.932045ms | 2.310642ms | 506646 | 1639 | 300 | 0 | 0 | 604 |
| players1_targets300_lethalfalse_coldtrue | 1.567334ms | 2.455887ms | 2.853609ms | 849441 | 2510 | 300 | 0 | 0 | 604 |
| players1_targets300_lethaltrue_coldfalse | 14.572435ms | 18.241914ms | 18.665488ms | 10908112 | 23956 | 300 | 300 | 2400 | 4204 |
| players1_targets300_lethaltrue_coldtrue | 15.17257ms | 17.550507ms | 18.289363ms | 11250791 | 24826 | 300 | 300 | 2400 | 4204 |
| players10_targets1_lethalfalse_coldfalse | 584.173µs | 967.016µs | 1.385352ms | 283460 | 864 | 10 | 0 | 0 | 240 |
| players10_targets1_lethalfalse_coldtrue | 4.998007ms | 5.890628ms | 7.912875ms | 3712023 | 9570 | 10 | 0 | 0 | 240 |
| players10_targets10_lethalfalse_coldfalse | 693.731µs | 849.893µs | 905.675µs | 467752 | 1457 | 100 | 0 | 0 | 420 |
| players10_targets10_lethalfalse_coldtrue | 4.813433ms | 5.654957ms | 6.256911ms | 3896319 | 10163 | 100 | 0 | 0 | 420 |
| players50_targets1_lethalfalse_coldfalse | 2.996769ms | 3.863889ms | 4.463288ms | 1566720 | 4374 | 50 | 0 | 0 | 5200 |
| players50_targets1_lethalfalse_coldtrue | 24.5166ms | 27.167665ms | 27.87792ms | 18712984 | 47888 | 50 | 0 | 0 | 5200 |
| players50_targets10_lethalfalse_coldfalse | 4.512732ms | 6.254368ms | 6.348378ms | 2428138 | 7277 | 500 | 0 | 0 | 6100 |
| players50_targets10_lethalfalse_coldtrue | 24.639413ms | 30.756567ms | 31.037735ms | 19574401 | 50791 | 500 | 0 | 0 | 6100 |

## Interpretation and remaining cost

- An admitted 300-target nonlethal AoE averages 1.23 ms; lethal with 300 XP events and 2,400 generated drops averages 14.57 ms, with 10.91 MB allocated. This experimentally localizes substantial remaining work to the death/reward/drop path, rather than target damage alone. It is not a CPU-profile attribution to one function.
- Fifty admitted players each casting at ten targets average 4.51 ms, p95 6.25 ms (500 hits, 6,100 sink deliveries). Invalidating all fifty raises average to 24.64 ms and allocation to 19.57 MB. Cache rebuilding remains materially more expensive, so broad invalidation during busy gameplay deserves attention.
- Single-player forced invalidation consistently adds approximately 342,640 bytes and 870 allocations for the same work. Shared immutable catalog/helper evaluation during rebuild is still a meaningful cost; retained admitted attributes avoid it.
- Raw timed run is `/tmp/stress-combat-matrix.log`. This is a synthetic server-side stress validation, not a client or production load test. No comparison against an unoptimized checkout was performed in this matrix.

## Reproduce

From repository `server/`: source `/workspace/shared/toolchains/server/env.sh`, then run `FFO_COMBAT_STRESS=1 GOMAXPROCS=2 go test ./internal/game/scene -run '^TestCombatAttributeStressScenarios$' -count=1 -v`. Without the environment variable this resource-bounded test skips. Maximum tested batch is 300 deaths/2,400 drops or 50 players/500 nonlethal hits; the full run took 25.6 seconds including fixture setup and admission.
