# Fixed binding repair: deployment contract (not executed)

## Authored scope
Migration 127 contains the sole JSON seed for 66 reviewed non-equipment definitions:
28 runes, 10 ores, 4 feathers, 9 dynamic spirits (fixed unbound); 7 modern tools/materials/keys and 8 explicitly bound native items (fixed bound). Five ambiguous point consignment cards remain unspecified. This is not a global inference from CanTrade/CanMail, names or storage instance_kind.

Both existing `game_items` and `game_equipment` gain nullable `fixed_binding` and `binding_rule_source`. No table, owner, role or ACL is created/changed. Nine spirit policies apply only after dynamic spirit finalization (`Equip=nil`); genuine equipment is rejected by the data loader and retains per-instance behavior. LoadItems now requires migration 127: an unmigrated database produces an explicit startup catalog error, not a deferred character-login failure. Build and preflight before stopping service; run real loader smoke after migration, before claiming readiness.

## Production read-only dry-run, 2026-10-29 12:25–12:26 +08
Runtime remains `6f01fdcf0051ff6b204acc60f73ea96de2a25df4`, PID 32040. Only application-role READ ONLY queries and a standalone diagnostic were run; no migration, inventory action or restart.

- 66 definitions present: 57 `game_items`, 9 `game_equipment`. `11682` is in game_items.
- 101 active location stacks, all in bags; 85 have UIDs. Other checked locations: warehouse, stall, mail, family stash, equip/change slots.
- Projected mutations: `character_items.bound` 63 rows; `item_instances.bound` 58; `card_instances.snapshot_bound` 61; `equipment_instances.bound` 0; other locations 0.
- Rune-only snapshot: 82 active stacks / 120 units / 3 owners. Position vs root mismatch 0. 43 NULL card snapshots use legacy root fallback; they are not missing cards. 16 unowned historical rune roots (including 5 explicit root/card mismatches) are deliberately left untouched.
- Additional wrong-bound material stacks: ore4001=889, ore4004=445, ore4007=234, feather25027=52, feather25028=6 (1626 units total). All five should be unbound.
- The player continued playing during separate read-only transactions; these are diagnostic observations, NOT hardcoded migration invariants. Recompute at the stopped final-save boundary.

## Offline ordering and safety
1. Build exact reviewed public SHA. Verify current PID/identity/version and application-role permissions; keep client 1.0.6 unchanged.
2. Back up database and binary/config; use existing graceful CtrlBreak plus shared log reader. Require process exit, scene drain, final-save/clean-close evidence; never force-kill writing process.
3. Confirm unique old instance is stopped. Take final consistent database dump and validate its listing. Preserve it; do not silently roll back player data.
4. Apply idempotent upgrade127 in one transaction with ON_ERROR_STOP. It validates exact ID/name/table coverage, conflicting policy, ownership/UID/card integrity, and forbids targets occupying actual equip/change slots. It locks existing tables, normalizes only authored binding bits in active root/card/equipment caches and location rows; it never merges or deletes stacks or unowned roots.
5. SQL asserts complete row counts and hashes of every nonbinding field before/after, and exact unowned root/card/equipment preservation. UID, quantity, position, effects, locks, timestamps and currencies are not modified. SQL failure rolls back the entire migration.
6. Start new binary only after migration succeeds. Real fantasy-role LoadItems + char7/3 all-loaders smoke; verify owned roots/card/cache equal canonical definitions, exact total quantities and identity/effect/lock preservation. Normal sorting may subsequently merge equivalent stacks; do not perform a destructive player test.

Direct online SQL is not safe: the old process holds snapshots and can overwrite repaired bits on its next save. New load/save canonicalization consumes the same injected ItemDefs as scenes; no per-source or hardcoded store policy. Historical receipt/digest JSON is not rewritten; pending grant delivery must normalize its runtime output before adding/saving.

## Validation performed in cloud
- PGlite PostgreSQL migration execution: 66 identities, first-run repair, repeated idempotence, all location types, nested socket/spirit refs, true/false/NULL snapshots, UIDless material, equipment parent preservation, orphan preservation PASS.
- Fail-closed transaction/DDL rollback: wrong name/table, missing template, target equipped, UID identity mismatch, duplicate ownership, missing card PASS.
- Go store focused unit/regression: canonical root+card+location save, UIDless hydration, immutable caller snapshot, memory/nested equipment preservation, legacy sort persistence, spirit/mail/warehouse/stall/family/snapshot tests PASS. Generic DB-backed tests are not run against production.

## Runtime regression evidence
- The exact 13 duplicate rune item/count pairs observed by the production read-only loader are retained in `internal/game/scene/testdata/sort-fixed-binding-20261029.json` without character identity or instance UIDs. Effects come from the authored rune SQL; fixed binding comes from migration127's JSON, not a second rule list.
- Both bag and warehouse sorting collapse each pair under its canonical unbound definition, conserve quantity, retain the surviving UID/effects, reclaim empty cells and are idempotent. Distinct effects/locks stay separate. Warehouse stack limits and partial rune stack identities are preserved.
- NewStack/Add/AddStack, explicit warehouse placement, rift rewards, cube outputs/refunds/aggregates, pending reward delivery, embedded rune insertion/removal and spirit separation apply the same definition rule. Instance-kind1 power scrolls remain permanently bound, while actual equipment bind-on-equip remains untouched.
- Focused data/domain/scene/store and gameserver/showcase build-test checks pass. Related Rune/Astral/Cube/Spirit/Sort/Warehouse regressions pass in all four packages. Fixed-binding/sort race checks pass for domain, scene and store. These are cloud source tests, not claims that the live migration or deployment has occurred.
