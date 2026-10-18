# Level-up snapshot CPU reduction

## Proven code path

Ordinary XP level-up calls `grantExpWithBonus`, which updates growth/stats, then
publishes attribute/XP/level/skill events and refreshes inventory, quests, life
skills, endgame items, and change-set tooltips. Character persistence is queued
through the dirty saver; inferred newbie-tip persistence is asynchronous. This
inspection did not establish a level-up database deadlock.

Inventory tooltips and blocked flags previously started a new equipment
requirement evaluation per item, repeatedly traversing the same worn-equipment
and passive-skill dependency graph. A full equipment bag amplifies that CPU and
allocation cost inside the scene actor. The fix shares the existing subset memo
within one synchronous inventory or change-set snapshot only. It is not retained
on the player or scene, and does not cross level/status/equipment mutations.

## Reproduction and results

Go1.23.12 linux/amd64, cloud AMD EPYC9V74; ten iterations per case using the
existing nine-slot/passive fixture and real admitted scene player. Fixture setup
and admission excluded; timed operation is one exact level-up and scene flush to
a fake sink. The 54-item fixture repeats head equipment, so this is controlled
server-side evidence rather than a capture of the user's character/client.

| Case | Before | After | Allocated bytes | Allocations |
| --- | ---: | ---: | ---: | ---: |
| Nine worn items | 5.48ms | 3.32ms | 2,888,650 → 2,310,375 | 9,156 → 5,370 |
| Nine worn +54 bag equipment | 55.36ms | 5.96ms | 32,950,352 → 5,220,934 | 105,290 → 9,112 |

Reproduce from `server`: `go test ./internal/game/scene -run '^$' -bench BenchmarkLevelUpDelivery -benchtime=10x -count=1`.
Add `-cpuprofile` to inspect the requirement/tooltip dependency traversal.
Timings are informational, not machine-dependent CI limits.

Regression compares encoded inventory/change-set packets against independent
per-item evaluations and checks unchanged attribute snapshots after level-up,
status addition/removal, broken/removed worn gear, learned-skill and base changes.
Existing requirement, combat-cache, inventory and change-set tests also run.
This establishes a bounded performance fix; Windows/client incident confirmation
still requires runtime observation. No gameplay formula or save policy changes.
