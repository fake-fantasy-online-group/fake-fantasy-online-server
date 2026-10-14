# Rift guardian equipment pools and color-first drops

Migration 82 publishes immutable rift rule version 7 and advances the active selector only from 6 to 7. A replay cannot downgrade later versions. The equipment pools are stored in `game_rift_rule_versions.equipment_pools`, loaded by the PostgreSQL rule loader, deep-cloned and included in each run's frozen rules. The JSON audit snapshot in `server/docs/endgame/rift-equipment-pools-v7.json` is evidence only; runtime does not load it.

## Native source and eligibility

- Sources: actual spawned monsters whose native kind is 精英, plus native `elite_id` aura replacements of monsters with actual map spawns. Source levels must be positive; the60-level ceiling applies to equipment requirements, not source monsters.
- Candidates: each source's actual `game_monster_drops` rows, both explicit `item_src='arm'` items and native type/resource-level ranges. Random rows are expanded with the same query-type mapping, inclusive native resource-level range, and position114/116 fallback used by `loadDropChoiceLevels`.
- Only positive valid drop probabilities are source evidence. Generated `dungeon_template:%` supplements are excluded.
- Random-kind candidates require native dyeable/type-drop eligibility (`no_type_drop=0`, recognized query type or native earring/belt position, nonnegative resource level). Explicit named native equipment remains eligible even when `no_type_drop=1`; that flag excludes random-type generation, not actual named drops.
- Both equipment requirement and original arm level requirement must be at most60. Ascension-only equipment is independently excluded: original newbie and five base-profession flags all zero, while any `sr_*` profession flag is nonzero. Shared ordinary equipment allowed to both base and advanced professions is not ascension-only.
- PostgreSQL extraction of the original data yields60 populated native monster-level bands, 4–63 inclusive, and318 distinct equipment IDs:259 ordinary dyeable bases and59 fixed named items. Band sizes range25–116. Representative sizes:4→25,15→71,25→59,35→79,36–37→116,38→109,45→70,60→81.

## Selection and rewards

The tier anchor is the maximum frozen admission level of the roster, matching the rift encounter's base-level source; it is not difficulty tier1–150 and does not change after level-ups. Select the nearest source monster level with at least one item legal for that recipient's admitted level, profession and gender. Equal-distance tiers favor the lower source level. Every item in that selected explicit band has equal selection probability. There is no whole-catalog fallback for new rules. Guaranteed unique selection is restricted to eligible unique bases in that same band.

Reward quantities remain the existing ten personal-MF normal-boss equipment rolls plus existing one/two special equipment slots, runes and dust. New dyeable equipment rolls color first, then sockets within that color budget and random attributes in the remainder. Fixed named equipment preserves its native properties: no added random affixes, sockets or unique promotion. Newly created uniques have zero sockets.

Already generated durable grants are never rerolled or rewritten. Historical rule snapshots remain valid and retain their old catalog-selection policy, but all newly generated gear, including pending old runs, uses color-first for ordinary dyeable bases and excludes ascension-only and over60 equipment. A new run is required to use the explicit version7 pools.

## Validation

Focused Go tests cover nearest/tied/legal bands; forbidden equipment; missing/empty v7 configuration failing closed; deep-cloned frozen pools; legacy snapshot resolution; all equipment slots and quantities; completed-grant replay; and personal MF/unique socket behavior. The original SQL data and migrations are additionally executed in PostgreSQL-WASM (PGlite), including production loader queries, historical preservation, immutable-guard enforcement and idempotent replay.

### Reproduce the PostgreSQL verification

Use Node.js with `@electric-sql/pglite` (verified version0.5.9) and Python3. From the repository root:

```
python3 server/tests/extract_rift_native_sources.py /path/to/complete/server/migrations /tmp/rift-native
RIFT_NATIVE_DATA=/tmp/rift-native PGLITE_MODULE=/absolute/path/to/node_modules/@electric-sql/pglite \
  node server/tests/rift_equipment_postgres.mjs
```

The extractor requires complete original initialization shards, including24–26 containing `gamedata.ov_arm`. It fails explicitly on an incomplete source checkout. It writes only extracted original statements and provenance to the chosen scratch directory; no game data or raw assets are added to this patch. The test uses original migration0 schemas, original native equipment/monster/drop/spawn statements, original migration32 monster stat updates, and historical rules migrations63/76–80. The fixture selects version6 to isolate migration82; activation81 has its own independent test.

The harness emits input SHA256s and `verification-report.json`, `loader-results.json`, `rules-snapshot.json`, and `verified-pools.json` in that scratch directory. It tests exact materialized pools, activation7, historical1–6 preservation, replay, published-data guards, rejection from active5, atomic empty-pool failure at active6, preservation of later selectors, and both verbatim production loader queries. The checked-in `server/docs/endgame/rift-equipment-verification-v7.json` records the completed run and source hashes. This is a reduced-dependency real PostgreSQL-engine test, not a production rollout or concurrency test.

The actual migration39 option matrix contains16,583 choices across135 keys. All259 eligible bases resolve an exact type/resource-level key containing9–15 distinct attribute IDs, so none needs a type0 fallback or lacks sufficient attributes for a five-slot color budget.


The60 bands draw on171 original aura/elite source monsters. The59 fixed named items are legitimate preascension equipment with wearer requirements20–60. They include利器/名器 weapons, named shields and accessories, five仙器 necklaces and ten暗抗 armor pieces. Their `no_type_drop=1` prevents random-kind generation; it does not prohibit explicit named drops. They remain in normal and non-guaranteed special selections and retain their original fixed properties. Guaranteed-unique selections use only the259 dyeable bases within the selected native band.

Coverage of the complete318-item pool checks40,050 combinations: wearer12–100, every party maximum from wearer level through100, five professions and both sexes. There are zero empty ordinary or guaranteed-unique intersections; minimum3 candidates for each. `server/docs/endgame/rift-equipment-coverage-v7.json` records that result.

The source-level61–63 bands are retained:凶暴的妃子之魂4156,魔性的妖魔管家4157,死者之王4158 and死者之后4159 all have valid equipment≤60 in their original drop lists. Each of those three bands contains66 ordinary bases already within the318-item universe.
