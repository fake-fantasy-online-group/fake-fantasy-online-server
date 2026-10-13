# Map transition ownership

The existing native `entered=1` position report remains the client's completed-map signal. No wire fields or opcodes are added. Ordinary imported map gates and server-created teleports must have distinct initiators.

## Database contract

`map_teleports.transition_owner` is explicitly `client` or `server`, with `server` as the default for new rows. `domain.Teleport.ClientOwned` carries this policy to scene handling. A zero-valued portal remains server-driven. A nonempty script, including `Trap20502`, remains server-owned in the loader even if its stored owner is mistakenly set to `client`.

Migration 65 sets client ownership only for 457 exact, resolved, polygon-backed, empty-script routes from the original migration 0 `.link`/`.prc` import. This includes 26 same-map gates. The source map identity, key, complete polygon, destination, landing/direction and return fields must still equal the audited original route. A modified route under an old key is not silently classified as native. These are design ownership assignments based on imported provenance; they do not assert that every legacy route exists in the currently mounted client JSON.

The audited baseline contains 994 original keys, all matching the imported fixture, plus five server-created exits from migration 42. Those five exits stay server-owned: maps 21100,21200,21300,21400 and21500, each proc 90001, to map 7 at (7424,5922). Classification is based on the explicit route ledger and complete field equality, never a numeric proc range.

In the fixture through migration 65:

- 457 rows are client-owned, including 26 same-map routes
- 542 rows retain the server default: 387 unresolved sources, 31 unresolved destinations, 118 without valid polygons, five added exits and one script gate
- The actual loader admits 457 client-owned routes and six server-owned routes (the five exits plus `Trap20502`)
- The joined loader count is 1039 because several map IDs share an imported map filename; this is distinct from 999 stored teleport rows

The ownership backfill runs only when adding the column. Replaying the migration preserves subsequent administrator choices and never rewrites route data. An administrator editing an existing route after migration must also review/set its intended initiator; route UPDATEs do not automatically reset ownership. Future inserts that omit an owner conservatively stay server-driven. An administrator can deliberately set an owner through the existing database configuration process.

## Required behavior

A client-owned gate must not transfer ownership merely because a periodic movement report hits its server polygon. Keep the source scene until the original client completes its local transition and sends `entered=1`. Cross-map completion preserves the existing generic client-ready path without adding a portal-route restriction, so original local NPC and village travel remain compatible. Same-map gate completion is identified by an exact configured client-owned destination/landing match and performs one ready-driven move/snapshot. NPC transport commands, GM, script, rift and explicitly server-owned physical exits continue using their server-driven path.

This distinction matters even with equal polygons: the client tests float coordinates, whereas position reports truncate to integers. On the actual 919→920 polygon, (4660.25,3590.75) is outside and (4660,3590) is inside. The original client also suppresses gates when initially spawned inside a polygon. These cases make a server movement hit insufficient evidence that the client has started switching.

## Validation

`server/tests/sql/validate_portal_ownership.py --work-dir <new-directory>` builds a fresh disposable PostgreSQL fixture through migration 65 without binding a port. An optional `--seed-dir <idle-disposable-through64-cluster>` first copies the fixture and never opens the source cluster. Tests cover failed-transaction rollback, changed route fields, unknown rows, exact classification counts, unchanged original data, administrator edits, replay and schema constraints.

Run the real loader with the resulting idle cluster:

`FANTASY_PORTAL_OWNER_PG_DATA=<copied-data> PG_BIN=<trusted-pg-bin> go test ./internal/data -run '^(TestLoadTeleportsExplicitOwnership|TestTeleportOwnershipActualReadOnlyPostgres)$' -count=1 -v`

The focused in-memory loader test separately verifies explicit client/server flags, conservative unknown values, stable ordering and script precedence. These tests do not replace scene/session timing tests or a Unity Player run.

## Acceptance entry samples

These are verified current configuration/source samples for the requested test categories, not a claim that every UI flow has already been played:

- Three-floor dungeon: Sky Spring maps 20801 → 20802 → 20803, with native floor gates and return gates. The backward landings differ from each floor's default entry and must remain unchanged. Entry can come from map 32 NPC 二郎神杨戬 (transport list 107) or the original client's local dungeon-entry option. Both existing entry paths must survive. The five separate legacy trial exit gates remain server-owned; the rift worker separately covers new-rift floor progression.
- Wilderness transport point: the actual 野外传送点 NPC on 华山脚 uses list 6 and native open/choose transport commands, for example to 下古岳平原. This is a server-driven NPC flow, distinct from walking into a geometric portal.
- Physical gate: ordinary 7 → 14 and verified 919 → 920 are client-owned. All 26 current same-map routes have exact integer destination coordinates, as required by the existing integer position wire; their source maps have no intersection with the 90 configured dungeon maps.
- New-character village choice: the 传送仙子 menu directly calls local DoTeleport for 桃源东村/西村 and 长乐东村/西村 (61/37/62/38), then sends the original ready. It does not require a geometric portal or a transport-list request. Generic cross-map client-ready handling must remain intact.
- Return-to-town: item 3381 is the consumable scroll and 10025 the unlimited book; item return and death return both remain server-driven. Map 919 returns to 58 at (7715, 5323), deliberately different from its ordinary gate exit.
- GM travel: authorized `/goto` reaches the server's GMTeleport path. Both same-map and cross-map destinations still require the ordinary server teleport notification.
