# Character login release gate

A successful migration COMMIT and world-data startup are not a character-login
check. On 2026-10-28 an administrator applied migration 120; its two new tables
were owned by that administrator without grants to the application role. Startup
passed, but inventory hydration failed with PostgreSQL SQLSTATE 42501 even for
characters that did not own a spirit.

## Before opening a release

- Prefer applying upgrades as the configured application/schema-owner role. If
  administration requires a separate migration role, explicitly review ownership
  and runtime privileges for every new object. Do not grant to PUBLIC or grant
  unrelated tables as an incident workaround.
- Run the smoke below using the **actual application DSN**, not an administrator.
  Supply environment variables through the existing secure configuration path;
  never paste a password into logs, source control, or the command line.
- Test at least one existing character with instantiated equipment in its bag.
  The smoke reads persisted state through the real pgx loader, checks every CRUD
  privilege on the spirit tables individually, rejects superuser connections,
  uses one database-enforced READ ONLY transaction for character state, and
  reads catalogs with server-enforced read-only pool connections (nested catalog
  queries require separate connections). The character transaction is rolled back.

From `server/`, with `FANTASY_CHARACTER_SMOKE_DSN` and
`FANTASY_CHARACTER_SMOKE_NAME` set:

```sh
go test ./internal/store -run '^TestCharacterLoginReadOnlySmoke$' -count=1 -v
```

**Never remove the exact `-run` filter against a production database.** Other
store tests are destructive fixtures. Do not set `fantasy_TEST_DSN` to production.
A skipped smoke (missing environment) is not a passed release gate.

Coverage: character row, bag with unified item/spirit hydration, stale stall,
equipment, quick-change set, skills, quests including spirit daily state, pets,
seen tips, and warehouse/wardrobe when their database-loaded rules are valid. This deliberately does not touch login timestamps, recover a
stall, save snapshots, or simulate a network login. Scene entry and the block list are not covered by this test; verify actual
client entry separately.

## Bounded recovery for this incident

With authorized database administration, grant only SELECT, INSERT, UPDATE,
DELETE on `public.spirit_instances` and `public.equipment_spirit_fusions` to the
verified runtime role in a transaction. Do not delete inventory, reset character
state, change table ownership unnecessarily, or roll back the binary against a
newer database. Afterwards run the application-role smoke and verify real client
entry. Table-existence checks alone cannot verify this recovery.
