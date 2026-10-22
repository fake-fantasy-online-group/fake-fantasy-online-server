# Native rift portal geometry regression

The original import matched some polygon UPDATEs to a one-based record ordinal
rather than the actual PRC `proc_id`. On 29 of the 168 eligible native maps this
left 67 ordinary native gate polygons NULL. The client still displayed and
triggered the original gate, while the server could admit only its seeded exit.

Migration 103 restores the exact PRC polygon at the exact original LINK route.
It does not change destinations, return points, scripts, player state, rewards,
movement validation, or the 160px admission tolerance. Newly restored NULL
routes become client-owned like the other ordinary native routes. Already
populated matching routes retain their explicit ownership. Missing/modified
route identities and conflicting populated geometry abort the whole upgrade.

## Scope and independent proof

- 168 eligible maps, 354 positive-destination, empty-script LINK→PRC gates.
- 287 existing geometries already match native source; 67 require restoration.
- All 354 exact polygons intersect the selected walkable combat component.
- No new map exclusions are required by this audit.
- No claim is made about unrelated decorative GSM objects without a LINK, or
  about script-only regions. The fixture contains only native map resources and
  imported static rows, never account, character, live-run or host data.

The compressed fixture embeds original PRC/LINK bytes, compressed MASK bytes,
and selected anchors. Its hashes, actual proc IDs, full route identities,
polygons, connected components and strictly interior reachable points are
rechecked entirely offline:

    python3 server/tests/native_portal_geometry/verify_offline.py --output /tmp/native-portals-proof.json
    python3 server/tests/test_native_portal_geometry_103.py
    cd server && go test ./internal/game/scene -run TestRiftNativeGeometryEveryCandidateGateReachesAdvanceHandler -count=1

The scene regression enters the real request handler for every native gate with
rules 10 / layout 6, uses missing-geometry negative controls, and verifies the
successful durable floor handoff without another key charge. The SQL test is an
offline contract and atomic transformation model, **not PostgreSQL execution**.

## Existing database upgrade

Apply `migrations/upgrades/000103_20261022_native_portal_geometry.sql` using
`psql -v ON_ERROR_STOP=1` before restarting the matching server. The script is
one transaction, verifies all 67 identities/geometries and reports the number
of restored NULL rows. Replaying reports zero changes. Geometry is loaded at
server startup; merely applying SQL does not refresh running scene actors.
Fresh databases execute the identical body in `000000_init-103.sql`.
