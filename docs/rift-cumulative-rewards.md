# Rift cumulative Legendary rewards

New Rift rule version 12 pins loot version 3 and replaces the old special equipment slot, its timely bonus slot, the 60% chance cap and tier-10 first-item guarantee with a single cumulative entitlement.

- Each tier contributes 10 percentage points.
- Every complete 100% guarantees one Legendary; the remainder rolls once for one more.
- Tiers 1/9/10/11/15/99/100/149/150 yield 0+10%, 0+90%, 1, 1+10%, 1+50%, 9+90%, 10, 14+90%, and exactly 15 special Legendaries.
- Timely completion and personal MF do not increase this special count. Timely rune/dust bonuses and other rewards remain unchanged.
- Ten additional ordinary Boss items remain separate. They use the frozen Boss/elite outdoor profile, not the ordinary-monster profile, and retain personal MF. They can independently become Legendary, above the special entitlement.
- Special items must have a real six-affix/executable-power foundation and wearer-legal native base. No white item or legacy Unique consumes a guaranteed slot. Admission rejects missing foundations before taking the key; generation stages all grants before marking them ready.
- Existing ready grants are never rerolled. Published Rift11 and older configurations are immutable and retain their old count policy. Migration107 only advances an active11 selector, never a deliberately older or newer selection.

Validation includes deterministic boundaries and every remainder draw, actual tier150 awards, MF/timing independence, saved replay, mixed parties, native levels12–65 across classes/genders/employment, failure before key debit, and PostgreSQL/PGlite migration/loader/collision/rollback/immutability checks. PGlite uses documented reduced native prerequisites, not a production database or the full initializer.

## Monster-level power eligibility

The actual guardian entity level is captured with its death event, retained across deferred economic processing, and saved as RewardSourceLevel. Both cumulative items and the ordinary ten carry that immutable source level; power eligibility uses it without altering the base's original wearing requirements.

For an older pending run with no captured level, recovery first uses a surviving registered guardian, then the exact encounter Boss baseline selected by the original floor seed/spawn algorithm. A named guardian's natural-world level and the party-level lookup key are not substitutes. Ambiguous floorless sources remain unknown; new cumulative rewards fail closed rather than invent a level. Ready grants return unchanged before recovery or rolling.

The old-version branch uses variant-free source-aware Legendary generation. A pre-variant normalized-item golden oracle verifies unchanged RNG consumption when source level and the previous base gate are equal. Source-based eligibility may legitimately expand the powers of an as-yet ungenerated old-version reward, while its count policy remains frozen.
