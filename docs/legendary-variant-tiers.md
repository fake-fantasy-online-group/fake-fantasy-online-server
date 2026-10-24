# Legendary instance tiers

New legendary generation performs one mutually exclusive draw after selecting its existing power and six independent affixes:

- 1 out of 100: `秘*`, secret tier.
- 10 out of 100: `真*`, true tier.
- 89 out of 100: ordinary legendary, unchanged name.

The prefixes contain one literal ASCII `*`. Existing stored equipment is never promoted, rerolled, renamed by tier inference, or assigned a tier while loading. The optional `LegendaryInstance.Tier` string is persisted in the existing legendary JSONB state and whole-stack snapshots. Missing means ordinary. Each frozen pre-v12 Rift reward path must call the explicit legacy generator to retain its prior no-tier RNG structure; not-yet-generated powers use the captured monster level, while already-ready grants remain unchanged.

## Attribute scope

True tier independently rerolls each of its existing six selected affixes between both original legendary endpoints multiplied by 1.5. Secret tier locks each selected affix to the strongest original legendary endpoint multiplied by 1.5. For signed attack-interval reductions, the stronger endpoint is the negative minimum, not the numerically largest value. Both secret bounds equal the locked value. Integer conversion uses signed truncation toward zero; int64 intermediates prevent overflow, and invalid draws/overflow fail atomically.

True and secret multiply only the native equipment template's innate white attributes by 2 and 3. Already-resolved fixed refinement increments remain separate; a template attack of 100 plus fixed refinement 30 becomes 230/330, not 260/390. Random affixes, skill/power coefficients, suit bonuses, socket/fusion attributes, requirements, durability and weight costs are not multiplied. Combat aggregation and native tooltips share the same pure intrinsic calculation.

Reforge retains the tier and its immutable bounds. Secret rolls have no variable range, so reforge/temper previews and execution reject without consuming ingredients. Salvage and transfers retain their existing rules.

## Presentation and compatibility

All tiers retain the current dark-gold quality 5 and existing tooltip palette. The originally requested deep-red change was withdrawn to avoid requiring a client update. There are no new palette codes, client changes, resources, or binaries. The server sends its prefixed item name, authoritative numeric attributes, and optional `legendary.tier`; the existing client deliberately ignores unknown optional legendary fields.

Dungeon true weapons, class-set pieces and suit headings now use `真*` consistently. Their stable template IDs and numerical bonuses are unchanged. This catalog naming correction is independent from legendary tier generation and cannot multiply a set's attributes. Registration still rejects a second pass before mutation.

## Verification

Targeted tests cover the exact 1/10/89 partition, integer/signed endpoints, six-affix identity and unchanged power, actual stat aggregation, locked cube actions, JSON and memory reload/receipt replay, true-set names, native inventory/equipped/sidecar/stall/ground presentation, and real trade/equip/unequip/drop instance preservation. Historical Rift generator wiring is integrated with the cumulative-reward change and tested together before release. A live PostgreSQL test still requires an explicit test database and is not implied by memory/JSON coverage.
