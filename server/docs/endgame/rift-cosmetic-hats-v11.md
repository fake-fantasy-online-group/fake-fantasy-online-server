# Rift v11: cosmetic hats and missing face/backpack slots

Migration 105 copies published v10, filters its native equipment pool item lists,
and freezes a separate normal face/backpack fallback-ID list. The unpublished
105/v11 change is extended instead of introducing a redundant version 12.
The excluded native IDs are 2037–2044 and 2060–2063; the migration contains the
reviewed names and exact original identity checks. It is not a level-one,
resource-level-five, head-slot, or global native-drop exclusion.

## Version and activation boundaries

- Existing active v10 advances to v11; new admissions freeze v11.
- Published v10, existing paid-run rule snapshots, ready reward grants, existing
  equipment instances, native monster drops, dyes and legendary power minimum
  levels remain unchanged.
- All class, ability, reward quantity, chance, level and generator settings are
  copied unchanged. Native pool/source/item order is retained except the twelve IDs.
  Fallback membership is separate; original item lists are not cumulatively widened.
- Replays accept an identical v11 and reject a conflicting v11. A future active
  selector is never rolled back. Missing source identities, disabled immutable
  guards, or an emptied pool fail the transaction.
- Fresh initialization intentionally remains active v9, as migration 99 requires
  paging-client rollout before generator 6 activation. Such an environment still
  uses the old pool until its separately authorized client-gated activation.
  After satisfying that existing gate, activate v11 rather than v10. This patch
  does not change that deployment gate or deploy anything itself.

## Regression evidence

The repository-native fixture covers 60 source tiers and 318 distinct templates.
Fourteen tiers contain reviewed hats; none becomes empty. Level 42 changes from
70 to 58 raw templates; male/female assassin candidates change from 21 to 13,
while all 11 legendary-eligible candidates remain in that native pool. The new
missing-slot step then adds seven bases: 20 usable, 18 legendary-eligible total. Seven ordinary level-one
items, including the native iron headguard, remain in their source pools.

The fixture exports the fields consumed by eligibility and quality generation,
including native fixed affixes, plus 15,458 applicable native affix options. It
is not a complete production item/combat database. Rebuild it with:

    python3 server/tests/sql/export_rift_equipment_fixture.py

Focused Go regressions exercise candidate levels 1–65 for both genders,
version binding, immutable clones, and 3,584 real reward settlements across
versions 10/11, levels 1/12/24/35/42/60/65, both genders, tiers 1/10 and 64 seeds.
They require the old frozen v10 to still produce all twelve hats, v11 to produce
none, quantities and ready grants to stay stable, and all eight legitimate
level-42 legendary slot categories (including the two repaired slots) to be observable.

    cd server
    go test ./cmd/migrate ./internal/domain ./internal/data ./internal/game/scene -run 'TestRift(Cosmetic|SlotFallback)' -count=1

The opt-in PostgreSQL harness creates a new isolated cluster from trusted PG_BIN
executables and executes the actual migration against the native pools and
production immutable-rule trigger. It checks activation/replay, preserved old
rules and dependencies, v9/future selectors, conflicts, identity drift, disabled
guards, missing hats, empty tiers, rollback and published-rule immutability:

    PG_BIN=/usr/lib/postgresql/17/bin python3 server/tests/sql/validate_rift_cosmetic_hats.py --work-dir /tmp/rift-hat-pg

This focused harness is not a full initializer or production validation.


## Missing-slot scope and frozen data

The new nullable `equipment_slot_fallbacks` column is populated only on v11.
It is projected into `RiftRules.EquipmentSlotFallbacks`, deep-copied by the rule
table and serialized with paid-run snapshots. Versions 1–10 keep NULL/omitted
fallbacks and never execute this path. No live global-catalog scan is used.

The migration resolves the 17 ordinary native random-drop bases 2601–2610 and
2701–2707. Source predicates check both slot/type/position, `no_type_drop=0`,
resource tier, both original wear-level columns, normal vs ascension-only job
flags, and absence of fixed-extra rows. Costumes, fixed gear and ascension bases
are not fallback. Validation repeats the runtime-representable constraints.

Selection first keeps the existing nearest native tier, including its existing
per-recipient eligibility and lower-tier tie rule. Only afterwards, each absent
face/backpack slot receives all legal variants at the highest wearable resource
tier. The recipient's frozen admission level is used, not the highest party
level or their later experience/reward-context level. Class, beginner and gender
gates apply before choosing the tier. An already occupied slot is never replaced
or widened, even if its existing candidate is below the fallback's highest tier.
Other slots are never filled. Existing candidate order is preserved; the reward
generator retains its final ID sort and uniform per-base selection. Adding new
bases therefore dilutes individual old-base probabilities; it does not introduce
new slot weighting, extra quantities or different quality/promotion rolls.

For an actually missing slot, native boundaries are:

- Face: below level 6 no base; 6–11 ID 2701; 12–17 ID 2702; level 18+ IDs 2703–2707.
- Backpack: below level 6 no base; 6–11 ID 2605; 12–17 ID 2609; 18–23 IDs 2606/2608;
  24–29 IDs 2604/2610; 30–35 IDs 2601/2607; level 36+ IDs 2602/2603.
- These thresholds describe character wear requirements, not resource tiers.
  At 42/43, face bases remain wear 18/resource 3 and wings wear 36/resource 6.
  They respectively have 9 and 23 executable legendary-power candidates each.
  Actual generation produces valid six-affix legendary instances on both slots.
  The assassin's distinct reachable power names remain 94: this repairs missing
  placement choices rather than unlocking character-level 43 powers on low bases.

Additional regressions cover 2,600 native candidate scenarios over levels 1–65,
all five professions, both genders, beginner/employed and mixed-level parties;
1,024 actual 42/43 settlements across frozen v10/new v11; 128 mixed-party recipient
grants; synthetic class/gender/beginner restrictions and non-monotonic resource
vs wear tiers; invalid fallback sources; frozen membership and clone isolation;
and real loader decoding/binding through a query double. The query-double test
is not PostgreSQL execution. The fixture exporter evaluates the actual migration
candidate relation in its disposable SQLite read-model, without touching a game DB.

The opt-in PG harness additionally covers the 17 real bases and 16 negative
fixtures, frozen fallback membership, original NULL versions, DDL rollback,
each absent source slot, profession-gate boundaries, replay conflict and immutable
fallback/dependency updates. PostgreSQL execution remains a separate validation
step; compilation/static contract tests do not prove the SQL migration executed.
