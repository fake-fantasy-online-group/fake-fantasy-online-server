# Persisted endgame base items

Migration `server/migrations/000000_init-59.sql` moves the existing 32 base definitions into the same tables and loader used by ordinary items:

- 28 星篆: IDs `1900028001–1900028028`
- 洞天符钥: `1900028101`
- 星髓 / 净煞尘 / 八仙余韵: `1900028201–1900028203`

`gamedata.ov_item` owns names, level requirements, weight, stacking and mail/trade flags. `game_items` is the normal canonical name/level/price projection. `gamedata.ov_desc` owns descriptions and reviewed sprite IDs. The 28 runes have ordinary physical-card identity rows in `gamedata.ov_card`, and effects in `gamedata.ov_card_entry`. Their default effects now come through the existing `loadSocketAffixes` path. These are authored server additions, so `game_items.raw` is empty rather than a fabricated original-client record. They are ordinary items, not equipment; `ov_arm` and `game_equipment` are unchanged.

`data.LoadItems` requires all 32 persisted definitions. Missing configuration is an explicit startup error naming migration 59, never a code-catalog fallback. Gameserver registration publishes the loaded sprite mappings without creating or overwriting definitions. The dispatch search CTE already reads these same original tables; it needs no in-memory catalog merge. Showcase grant also uses the normal loaded definitions and no longer appends conflicting rune definitions.

## Compatibility

IDs, names, reviewed icons, requirements, weights and original permissions are unchanged. The key description intentionally adds the everyday search term “钥匙”: “开启演劫洞天的钥匙。试炼功成所得，每次消耗一枚，可邀同队道友同行。” This preserves its meaning while making ordinary description search find it; no code-only alias was added. All other descriptions remain unchanged. Runes remain nonstackable physical socket cards, weight 1, mail/trade enabled, levels 12–60. The key and three materials remain stackable, weight 0, no use-level requirement, mail/trade disabled. Prices remain zero. No new purchase, drop or use permission is granted.

The migration never changes player, inventory, UID, socket, card-snapshot, equipment-extension or receipt rows. New rune instances copy current persisted defaults; existing card snapshots remain untouched. A test explicitly preserves a prior rune card whose saved effect differs from the current template.

## Applying and rollback

No live database was changed while implementing this work. The migration runner remains empty-database-only. A fresh initialization includes fragment 59 in numeric order. For an existing installation, review and back up configuration and player data, then apply fragment 59 through the deployment's explicit incremental SQL procedure before launching the corresponding server build. Later endgame catalog/cube migrations must follow it.

The single DO statement is atomic and locks only relevant configuration tables for its duration. Exact prior rows are retained, so an unchanged second application is a no-op. Any item-ID or reserved row-number conflict, equipment collision, incompatible card identity or different effect aborts the entire statement. It will not overwrite a conflicting administrator-created item. Changing a migrated row intentionally means an old migration replay can fail; make later configuration changes with a new reviewed migration.

There is deliberately no automatic destructive down migration. Do not delete these definitions while running this build or while other configuration references them. The earlier code-only server also rejects persisted IDs as collisions, so rolling back its binary alone is unsafe. Use a reviewed paired application/configuration rollback from the backup or a forward fix, preserving all existing player and instance data.

## Reproduce verification

1. Source the trusted PostgreSQL toolchain, then run `python3 server/tests/sql/validate_endgame_item_definitions.py --work-dir /tmp/endgame-item-validation`. This creates a new isolated cluster; it cannot target a preexisting cluster. It initializes through 58, tests seven collision/atomic-rollback cases, applies 59 twice, compares all prior configuration and representative player rows, and executes the dispatch catalog's actual SQL with item-ID, name, “钥匙” keyword and level filters.
2. Set `FANTASY_ENDGAME_ITEMS_PG_DATA` to that new cluster's `data` directory and run `go test ./internal/data -run '^TestEndgameDatabaseDefinitions' -count=1 -v` from `server`. This executes production `LoadItems` queries in PostgreSQL read-only transactions, compares all 32 base/default definitions, creates actual rune stacks, and verifies an absent key row cannot be reconstructed from code.
3. Run the gameserver registration/icon tests, normal domain/store/scene/session/showcase tests, and rune-icon provenance tests. Test-only item fixtures read the JSON seed from migration 59; they do not maintain another production definition source.
