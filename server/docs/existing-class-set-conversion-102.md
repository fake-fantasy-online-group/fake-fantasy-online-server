# Existing class sets: one-time true-template conversion (102)

## Scope

For the approved pre-release enhanced sets already owned at maintenance time, convert exactly the 47 set-member template IDs in the equipment manifest to their `1800000000 + original ID` true templates. This includes the seven set weapons, but none of the 42 special weapons. No player identifiers or instance configuration are embedded in this script.

The same transaction updates both `item_instances.item_id` and the `item_id` cached in these ownership locations:

- `character_items`: inventory
- `character_equips`: equipped
- `character_change_set_items`: alternate equipment set
- `character_warehouse_items`: warehouse
- `character_mail_attachments`: existing mail attachments
- `game_family_stash_items`: family stash
- `character_stall_items`: sell-stall escrow only (`character_stalls.stall_type = 0`)

UID, quantity, position, owner, binding, timestamps, all refinement/strength, durability, socket IDs and socket-instance UIDs, rolled/fused properties and every other field are unchanged. `equipment_instances` is compared in full before/after; item facts and location rows are compared excluding only `item_id`. Socket/card/endgame property tables remain attached to the same UID and are not rewritten.

Purchase-stall requests, orphan/unowned item facts, future loot definitions, unclaimed/random Rift grants, completed grant responses, sales history, operation receipts and other historical records are not converted. Stop/drain the server first so in-flight trades and loaded characters cannot overwrite the database from memory.

## Existing database runbook

1. Deploy the matching server build, but keep all game processes stopped and prevent automatic restart. Drain/close all database clients other than this maintenance connection. Use the game's database/schema with a maintenance role able to lock/update these tables.
2. Execute **only** `migrations/upgrades/000102_20261022_existing_class_sets_true.sql` against the existing database:
   `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f server/migrations/upgrades/000102_20261022_existing_class_sets_true.sql`
3. Require successful COMMIT and the conversion NOTICE. Check `SELECT * FROM class_set_true_conversion;` for the single `class-set-true-20261022` receipt and its converted instance count. A zero count is valid only if there were no matching owned items.
4. Restart the matching server, then verify the existing character's equipment names, enhancement and sockets in game. Do not run an older pre-true-template server against the converted data.

The script rejects other sessions/prepared transactions, invalid/duplicate UIDs, missing or inconsistent item/equipment facts, and old/true sell-stall template collisions. It does not try to repair these conditions. Any exception rolls back the entire conversion and receipt; do not restart until the error has been understood. It takes exclusive locks with NOWAIT and checks that no second database session appeared before finishing. This is defense in depth, not a substitute for stopping services.

The receipt makes subsequent executions no-ops, including when ordinary set items have been acquired later. Never delete the receipt to retry a successfully committed conversion. A post-COMMIT rollback is not provided: blindly reversing all true IDs would destroy the distinction between converted and newly acquired true gear. Resolve any post-commit issue using the deployment's normal recovery procedure.

## New database path

`000000_init-102.sql` creates only the same receipt with count zero, because new empty databases have no pre-release enhanced player items. The normal initializer refuses nonempty databases. **Do not manually run init102 on an existing database before the upgrade**, as its receipt intentionally prevents later conversion. Init0..102 stays continuous; this migration follows the integrated main sequence (99 density, 100 task weapons, 101 pet supplies).

## Validation and limits

`python3 server/tests/test_existing_set_conversion_102.py -v` passes five static-contract/independent transactional-model tests: exact 47-template mapping, all seven locations with non-ID data preservation, future-drop rerun safety, special-weapon/purchase/history exclusions, and fail-closed bad ownership/facts. Independent code review checked the real schema and authoritative item loader/cache requirements.

These are not PostgreSQL execution tests. This cloud environment has no PostgreSQL DSN or server/client, so actual SQL execution and live character verification remain the maintenance operator's acceptance steps. No production SQL was executed while preparing this change.
