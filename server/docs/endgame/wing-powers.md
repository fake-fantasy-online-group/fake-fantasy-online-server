# Wing power extraction contract

- The cube selects one exact wing UID and one exact legendary UID. `ExtractWingPower`
  is a pure planner helper; the cube owns atomic donor consumption and wing write.
- `Stack.WingPowers` is a value array in attack / defense / utility order. Extraction
  replaces only the donor's category. Upgrade/change copies the entire instance.
- Donors must be valid, unlocked, singular darkgold equipment with an executable
  power eligible for that original template, original source level, and class/slot.
  Wings are not promoted to darkgold, renamed, or given darkgold random affixes.
- Current legendary power mechanics have no per-instance rolled potency or stage.
  `LegendaryInstance.Tier` scales intrinsic white stats and the six random affixes,
  **not** the power handler. Extraction retains `SourceTier` and `SourceLevel` as
  provenance, but never copies the six affixes, their ranges, or base multipliers.
  Source true/secret tiers therefore do not multiply wing attributes or power effects.
- Runtime enters the existing `ActiveLegendaryPowers` pipeline. All three powers can
  operate concurrently. Wrong-class and novice wearers cannot activate class powers;
  duplicate PowerIDs still choose the lowest source UID and never stack. Broken or
  otherwise non-functional wing equipment contributes no powers. Existing worn
  requirement checks remain unchanged.
- Existing orange multiline tooltip effect blocks show all three power names and
  effects; no client packet or item quality change is needed.
- Persistence uses `equipment_wing_power_state`, separately from legendary state.
  Save/load and aggregate validation are attached to ordinary item persistence, so
  inventory, storage, equipment and transactional snapshots retain the value array.
  Migration must create this table even when wing economy configuration is inactive.

Required DDL:

```sql
CREATE TABLE equipment_wing_power_state (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 state JSONB NOT NULL CHECK(jsonb_typeof(state)='array' AND jsonb_array_length(state)=3)
);
```

Focused tests: `go test ./internal/domain ./internal/store ./internal/game/scene
-run TestWingPower -count=1`. The PostgreSQL roundtrip test requires the repository's
normal test database configuration; it skips rather than claiming SQL coverage when
that database is absent. Memory fault tests cover state/snapshot/receipt rollback,
lost COMMIT acknowledgement reconciliation and receipt replay without re-consumption.

Display uses exactly the existing legendary orange `PowerName：effect` lines. There are no wing/category headers, slot labels, empty placeholders, new panels or extra actions.
