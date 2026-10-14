# Color-first equipment drops

New natural equipment drops and newly generated rift equipment rewards use one
color budget followed by a socket split. Existing persisted items are not rerolled.

1. Sample the existing color profile with the existing MF tail transformation.
   At MF=0 the original integer profile weights are used unchanged.
2. The sampled count N is the **total** natural-attribute-plus-hole budget, not
   an additional budget above fixed native attributes. Fixed native lines F
   remain intact: random capacity is max(0, min(N, 5) − F). For example F=2,N=4
   permits two more lines/holes; F=2,N=0 retains the unavoidable native floor of 2.
3. Resolve compatible distinct random attributes before splitting. If a legacy
   or sparse options table cannot supply the requested count, its actually
   realizable count determines the feasible color first. No unrelated attribute
   is fabricated, and the socket split can never lower this feasible color.
4. Set M=feasible random count. The old level-based drop socket cap does not
   apply to new color-first drops: even low-level four-line equipment can receive
   four holes. For eligible
   equipment, choose holes from 0..M using decreasing arithmetic probabilities.
   Ineligible slots and M=0 always produce zero holes.
5. Replace that many random lines with holes. Native lines + remaining random
   lines + holes is exactly the feasible total selected before the split.

For M≥1 the integer weight of outcome i is
M(M+1) + 2(9−M)(M−i), for i=0..M. The weights sum to 10M(M+1), so the maximal
eligible hole count has exactly 10% probability with no rounding. Rebuild this
distribution on the feasible M rather than clamping outcomes drawn above it. Thus M=4 gives 30%,25%,20%,15%,10%; M=5 gives
23⅓%,20⅔%,18%,15⅓%,12⅔%,10%; M=1 gives 90%,10%.

MF still affects the color roll and unique probability. As holes are now part
of that color budget, MF can indirectly affect the hole distribution; item,
rune and other loot streams remain independent. Socket splitting has a separate
purpose RNG and cannot perturb another item's roll.

Successful UNIQUE/dark-gold promotion removes empty holes. Promotion refuses
any stored socket card, UID or card payload (including stale out-of-range
entries), before consuming RNG or changing the item. Players must explicitly
extract cards before awakening. New cube and native drilling reject UNIQUE
items. No loading or migration process deletes old unique sockets: their cards
and extraction remain supported. Existing native affixes and other progression
are preserved. Formation potency initialized on new ordinary equipment is not
proof of an active formation and remains untouched.

All newly generated rift equipment uses color-first allocation, including rewards
not yet materialized for older runs. Durable granted items and ground pickups
retain their entire saved Stack without rerolling.
