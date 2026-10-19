# Public read-only catalog export

`server/cmd/catalog-export` produces local, immutable item/equipment JSON for the Cloudflare catalog. The public site needs no connection to dispatch, PostgreSQL, or the game server. This command never initializes/migrates the database, opens a network listener, uploads data, or deploys a site.

## Publication scope

Schema version 1 publishes named item and equipment definitions. It preserves the existing `/api/catalog/items` display contract and item detail attributes:

- Identity/name/category, level, purchase and sale prices, weight, stacking and trade/buy/drop flags
- Public combat stats, durability, icon ID, item description, requirements and displayed equipment affixes
- Explicit numeric filter values in `stats`, a numeric `sex` restriction and an array of supported `professions`

Only these source tables are read: `gamedata.ov_item`, `gamedata.ov_arm`, `gamedata.ov_desc`, `gamedata.ov_card`, `public.game_items`, `public.game_equipment` and `public.game_equip_extra`. Columns are explicitly projected. The public scope intentionally does not import dispatch's generic JSON endpoints or its relations query.

Accounts, characters, GM levels, credentials, NPC scripts, quests, recipes, drop sources/probabilities and other relations are not exported. The existing `/api/catalog/relations` contains private NPC scripts and `game_drops`; it must never be dumped into this bundle. `droppable` is a public item capability flag, not a monster/drop-source relation. Copyable item command text in the UI confers no server permission.

All named item/equipment definition rows are in scope, matching the existing library; there is no per-item manual approval chore. An administrator must select an approved source snapshot whose item names/descriptions/affixes are suitable for publication. This exporter does not infer unpublished content from item IDs or scrub secrets someone has put into otherwise public description fields. It does not include the unrelated datasets above, even if present in the same database.

## Run against an approved source

Use an isolated approved database copy and a dedicated least-privilege reader with `SELECT` only on the seven source tables. Do not use a production owner/administrator credential. The exporter requires its own explicit connection variable; it never uses `DATABASE_URL`, game server defaults or a migration/init command. The URL must name its host and database. Credentials belong in the process environment or an approved secret mechanism, never in Git or a command-line argument.

```sh
cd server
go build -o /tmp/catalog-export ./cmd/catalog-export
# Set CATALOG_EXPORT_DATABASE_URL using your approved secret mechanism.
# Example shape, without a password:
# postgresql://catalog_reader@127.0.0.1:5432/approved_catalog_snapshot?sslmode=require
/tmp/catalog-export --out /tmp/public-catalog-release --source-revision release-source-label
```

The optional `sourceRevision` is a public operator label, not a claimed verified game release. Do not put a DSN, private filesystem path or secret in it. `contentRevision` is always computed from the actual exported public content. No source/release label is required to create a verifiable snapshot.

The command applies `default_transaction_read_only=on`, fixes the search path to `pg_catalog`, and reads with `REPEATABLE READ READ ONLY`. It verifies the transaction settings, gets the time from PostgreSQL's transaction timestamp, and rolls back before returning data. Metadata/counts are derived from those same rows. SQL row bounds use a bound parameter. Cancellation, query errors, empty data, invalid/duplicate records and exceeding `--max-rows` fail without publishing a partial or truncated manifest. Default row limit is 250,000 and default overall timeout is two minutes.

## Artifact contract

The output directory contains:

- `catalog-<sha256>.json`: immutable snapshot; existing files with the same name are verified, never overwritten
- `manifest.json`: atomic pointer to the latest fully written snapshot

The output must be on a local filesystem supporting hard links, atomic rename and directory synchronization (the intended cloud/Linux workflow). Snapshot bytes and the directory entry are synchronized before publishing the manifest, then its renamed directory entry is synchronized. A failed publication may leave a complete unreferenced snapshot; it never replaces an existing snapshot with different bytes, and rerunning is safe.

`manifest.json` fields:

```json
{
  "schemaVersion": 1,
  "contentRevision": "sha256:<hash of normalized public content>",
  "sourceRevision": "optional public operator label",
  "generatedAt": "RFC3339 UTC timestamp from the database transaction",
  "snapshot": "catalog-<snapshot byte hash>.json",
  "sha256": "<snapshot byte hash>",
  "bytes": 12345,
  "counts": {"item": 1, "equipment": 1, "total": 2}
}
```

The example is a schema illustration, not a real export. The snapshot has `schemaVersion`, `contentRevision`, optional `sourceRevision`, `generatedAt`, `meta` and `items`.

`meta` retains `itemCount`, `equipmentCount`, `total`, `maxLevel` and `categories` (`value`, `kind`, `code`, `label`, `count`). `npcCount`, `taskCount` and `skillCount` are zero and `maps` is empty because those collections are outside this public scope, not because the game has none. The public frontend must label unavailable collections accordingly.

Each entry keeps the existing public item properties: `kind`, `id`, `name`, `typeCode`, `typeLabel`, `level`, `price`, `sellPrice`, `weight`, `stackable`, `buyable`, `tradable`, `droppable`, optional `minAttack`, `maxAttack`, `magicAttack`, `defense`, `magicDefense`, `durability`, `iconId`, `description`, and `attributes` (`label`, `value`, optional `kind`). It additionally always has:

- `sex`: 0 for unrestricted, 1/2 for the existing restriction values
- `professions`: array of the same Chinese profession names used by the original filter
- `stats`: numeric `attack` (maximum physical attack), `magicAttack`, `defense`, `magicDefense`, `str`, `vit`, `agi`, `int`, `spi`, `dex`, `luk`

Consumers can filter/sort/page this local array and return the existing `{items,total,page,pageSize}` shape. They must use the normalized fields for filters, rather than parse formatted attribute strings. Item/equipment detail is already in each row; private relation endpoints are neither necessary nor provided.

The content revision is SHA-256 of compact Go JSON for `{meta,items}`, with entries ordered by numeric ID then kind and categories by kind then numeric code. It excludes export time and the operator source label. The snapshot byte hash covers the entire emitted compact JSON **including its final newline**. Fetch the relative snapshot filename from the manifest's own directory, validate size and SHA-256 on the raw response bytes, then check matching schema/revision/time/count metadata. Do not reconstruct those bytes using JavaScript `JSON.stringify`; Go escapes HTML-sensitive characters. Immutable snapshots may be cached long-term; the mutable manifest should be revalidated. Hashes detect mismatches; they are not an independent signature or authorization to publish data.

Keep generated JSON, real game resources, database copies and credentials outside Git. Only code/tests and explicitly synthetic fixtures belong in this repository. Staging this output in a site bundle is a separate operation from publishing it.

## Validation

```sh
cd server
go test ./internal/publiccatalog ./cmd/catalog-export
go test -race ./internal/publiccatalog ./cmd/catalog-export
python3 tests/public_catalog_postgres.py --pg-bin /path/to/trusted/postgresql/bin
```

The Python check creates a new disposable PostgreSQL cluster, loads only `internal/publiccatalog/testdata/synthetic.sql`, executes the exact exported SQL using bound parameters in a read-only repeatable-read transaction, and tests projection/parity, denied private-table reads and denied writes. It uses the single-user backend and starts no TCP or Unix listener. It removes its temporary database when done.

For environments with supported local PostgreSQL sockets, load that same fixture into a new disposable database, connect as `public_catalog_fixture_reader`, and set `FANTASY_PUBLIC_CATALOG_TEST_DSN` to run `TestPostgresSyntheticFixture`. This opt-in pgx integration test is read-only and skipped by default. Do not target an existing user database. Unit tests cover pgx transaction modes, error/limit cleanup, normalized fields, projection allowlists, real hashes/counts/revisions, immutable files, and atomic manifest replacement.

Validation of synthetic fixtures is not evidence that a real game catalog has been exported, approved for publication, uploaded or deployed.
