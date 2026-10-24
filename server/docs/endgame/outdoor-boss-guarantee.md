# Outdoor Boss kill: two additional legendary items

With the new outdoor loot configuration active, each natural public-world Boss
kill adds two legendary equipment instances beyond its original drops. Elite and
normal monsters, props, gatherables, summons, private instances, dungeons, trials,
and Rift paths do not receive this pair. Theft still rolls one item using its
target's source quality rules; it neither grants nor consumes the kill pair.

Candidates come only from that Boss's existing natural equipment drop rows and
its map-specific fixed equipment pool. Duplicate references are deduplicated;
two distinct bases are preferred when available, while a one-base pool yields
two separate item identities. No global catalog fallback, new higher-level base,
receiver-class filter, or current-wearability restriction is introduced.
Original natural drops do not use those player filters either.

Power eligibility uses the killed monster's actual level. Each selected item
passes the same native-base, compatible six-affix, executable-power and socketless
legendary gates. The source level is recorded on the instance. The ordinary,
true or secret variant is generated exactly once by the common AtLevel API.
The existing stable loot Owner is preserved, including pet/trap attribution;
party members cannot substitute their identity or MF. Bonus generation has its
own deterministic selection/variable streams, leaving ordinary items and runes'
RNG unchanged.

A per-monster-life marker prevents repeated settlement from creating another
pair or rerolling the already generated instances. Both instances are built
before either is published. Invalid/empty content fails closed with a log that
identifies monster, level and source; it never fabricates powers or old Unique
items. A fresh spawn has a fresh marker.

## Native coverage and verification

The public-map source manifest is derived from the original map spawn rows,
migration32 monster/drop literals and the native ov_arm kind/resource-level
classification used by LoadDrops. All 11 public Boss templates are level28–60
and each has at least66–74 candidate bases proven to generate valid legendaries.
The level8 赤枭幻身 template is not a public natural Boss spawn. Four original
bases above equipment level60 are absent from the reused native item fixture;
the nonempty-pool proof does not depend on them, and production does not exclude
them simply because they are missing from a test fixture.

Focused tests cover two independent identities, one-base and duplicate-source
pools, distinct-base preference, original drops plus the pair, repeat settlement,
new lifetime, stable owner, source exclusions, source-level eligibility, single
item theft followed by the kill pair, and unchanged normal RNG consumption.
