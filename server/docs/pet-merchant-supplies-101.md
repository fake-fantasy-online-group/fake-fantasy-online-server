# Pet merchant supplies recovery

Isolated change against public deployable server main `a0ede0a`. No private
historical repository, old init fragments, prices/effects, premium shop racks,
player data or unrelated unmerged features are changed.

## Scope and source

Append native items 3020–3031 to 1003, preserving existing slots and capture tools
3382/3388/3383/3384/3389. City aliases 1303/1403/1503/1903/2102/2602/3003 continue
to use 1003; baseline inventory becomes 17 items.

Native feeding help in init-29 also names beginner-village item merchants. Only
verified shops 1101 (道具商人清悠) and 1201 (道具商人杜若), identified in init-0,
detach from 1001: copy its current stock/order, then append the missing foods.
Their baseline inventory becomes 29 items. Shared 1001 and other grocery merchants
remain unchanged. Replay never recopies over an already detached shop.

Native prices from init-3, copper per unit:

- 3020 百叶草, 3021 千层肉, 3022 万般香: 1000 each.
- 3023 活血丹, 3024 回魂散: 300 each.
- 3025 百花草, 3026 百果草, 3027 千块肉, 3028 千堆肉,
  3029 万物香, 3030 万灵香: 1000 each.
- 3031 无尽淳: 100000.

These are existing source prices, not new choices. The upgrade checks item
identity and positive valid prices but does not overwrite later price changes.
Pet feeding, hunger/trust and HP/MP restoration effects are unchanged.

## Applying

Run `migrations/upgrades/000101_20261022_pet_merchant_supplies.sql` only against
the intended backed-up database through a separately approved deployment. Then
restart/reload through the normal release process: shops are cached at startup.
This branch does not execute production SQL, publish or deploy.

Fresh empty-database initialization includes the identical SQL body in
`000000_init-101.sql`, after rift rules 99 and quest rewards 100. No extra manual
post-init step is needed. Existing databases use only the standalone upgrades;
never replay the full initialization set against player data.

One transaction locks shop configuration and appends only absent items. Missing
native items, invalid prices, unexpected source references, missing capture tools,
renamed beginner NPCs, noncontiguous slots or the 120-slot limit abort atomically.
Existing extra inventory, selling/repair settings and item definitions are kept.
Replay is a no-op absent later edits.

## Tests

From `server`:

```
go test ./internal/data ./internal/game/scene -run TestPetSupplies -count=1 -v
FANTASY_PET_SHOP_TEST_DSN='<isolated PostgreSQL DSN>' \
  go test ./internal/data -run TestPetSuppliesUpgradePostgres -count=1 -v
```

Source-price/scope checks, mocked loader and scene purchases cover all 12 items
in all ten shops, displayed prices, stock admission, bag count, exact deduction
and rejection of an unlisted item. The optional real PostgreSQL test uses only
connection-local temporary tables, checks baseline/partial stock and alias
resolution, applies twice, checks unrelated stock and six atomic rejection cases (including failure after earlier shop writes).

Cloud validation 2026-10-22: source/scope, mocked loader and scene purchase tests
passed. Actual PostgreSQL SQL execution/idempotence was **not run**: no server or
test DSN is available. The test explicitly skips without one; passing unit tests
must not be presented as real database replay evidence.

An expanded `-run 'Pet|Shop|Init'` first encountered two fixture failures because
this worktree's sparse checkout excluded tracked `docs/daoist-endgame` files.
The fixture blob was verified identical to the other worktree at HEAD, the sparse
checkout was expanded (no fixture or business edits), and both formerly failing
tests passed: `TestCombatAttributeCacheIndependentPetDependencies` and
`TestRedesignIndependentGrowthTransferAndLoginInitialization`.
