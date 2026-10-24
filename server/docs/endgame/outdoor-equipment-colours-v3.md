# Outdoor equipment colours: loot rules v3

Migration 106 adds the nullable JSONB `outdoor_equipment` and `outdoor_normal_equipment` fields and publishes
loot3 as an exact copy of published loot2, except for its version and the new
outdoor configurations. The original rune pool (including sequence and weight),
socket levels, grade occurrence rates and socket weights are copied unchanged.
The six conditional ordinary-colour weights are `[20,25,20,15,10,5]`, total 95;
the independent legendary base chance is 50,000,000 parts per billion (5%).
At zero Magic Find and with all quality/promotion eligibility satisfied, these
combine to overall `20/25/20/15/10/5/5` probabilities. Existing Magic Find,
affix feasibility, equipment eligibility and legendary minimum levels still apply.

Outdoor normal monsters use separate conditional weights `[46,20,15,10,5,2]`,
total98, plus independent2% legendary promotion. Their exact MF0 result is
white46%, darkblue20%, lightblue15%, purple10%, green5%, yellow2%, legendary2%.
They reuse the same MF curves, affix limits, eligibility and ordinary fallback;
when promotion is unavailable the six ordinary weights normalize over98.
Boss/elite weights and the configuration consumed by new Rift extra10 remain
separate and unchanged.
When legendary promotion is ineligible, the ordinary roll is retained: with all
five ordinary affix lines available, its probabilities are `20/95,25/95,20/95,
15/95,10/95,5/95`. Fixed native affixes can impose a colour floor, and sparse
affix options can further change visible colours. Magic Find still uses the
existing ordinary-quality tail adjustment and bounded legendary curve, with the
new 5% legendary base rather than a flat 5% at every Magic Find value.

## Scope and activation

- Runtime consumes this override only for eligible natural outdoor normal/Boss/elite
  equipment. It does not replace the shared colour profile.
- Theft is another drop from the same target and shares its source-specific
  quality rules, including each outdoor target kind’s new probabilities. Its reserved
  ordinal `-1` retains independent random streams. Dungeon/trial/Rift, summoned, gathering and prop
  targets keep their existing source rules. When the new outdoor override is
  selected, a base without `LegendaryEnabled` never revives old Unique items.
- Only an active loot2 selector advances to loot3. Existing older selections and
  future selections are preserved, including on replay.
- Loot1/2 keep NULL in both new columns. Their other fields and child rows are
  untouched. Historical serialized rules keep their legacy behaviour.
- No Rift rule version, Rift selector, paid-run snapshot, reward grant, native
  drop table, equipment definition or player inventory is changed.
- The migration requires published loot2, complete dependencies and all four
  enabled production immutable-rule triggers. It refuses incompatible column
  definitions, an occupied/different loot3 or mismatching copied dependencies.
  It never disables guards or repairs published data in place.

Fresh initialization runs `000000_init-106.sql` in the initializer's transaction.
Existing databases use only
`upgrades/000106_20261024_outdoor_equipment_colours.sql`, whose explicit transaction
covers schema addition, version creation, dependency copy, publication and the
selector change. The new loader also accepts the pre-106 schema, interpreting the
absent field as NULL. The new probabilities require both the migration and the
compatible runtime; loading loot3 with an older runtime will not apply the override.
No deployed database or service was changed during local validation.

## Reproducible checks

Fast migration/initializer structural checks:

    cd server
    go test ./cmd/migrate -run 'TestOutdoorEquipmentMigrationContract|TestIntegratedInitSequenceAndUpgradeParity' -count=1

Actual SQL and fault-injection checks, using an installed official PGlite module:

    PGLITE_MODULE=/absolute/path/to/@electric-sql/pglite node server/tests/outdoor_equipment_postgres.mjs

The harness reconstructs historical loot and Rift rules using verbatim migrations
63/76/77/78/79/80 and item prerequisites from migration59. It checks fresh/upgrade
parity, activation, exact child copies, historical NULLs, unchanged Rift rules and
reduced player-state sentinels, rollback of both schema and data, replay, older
and future selectors, schema conflicts, incomplete source rules, occupied versions,
changed/missing children and disabled/replaced/partial immutable triggers.
It also executes the exact current `LoadLootRules` SQL projection before and after
migration: the absent column and old loot1/2 project NULL, and loot3 projects the
exact new object while every other projected field and dependency stays unchanged.

Local result: all 33 checks passed with PostgreSQL 18.6 via PGlite 0.5.11.
This is a reduced-dependency real-PostgreSQL test, not a full initializer import,
production database validation, concurrent-session test or gameplay simulation.
