# Migration 81 release validation

Migration 80 stages immutable rift v6. Migration 81 is the paired-client/server release gate: it activates the rift selector from 5 to 6 after validating the staged configuration and native dependencies. Run it only with the v6-capable client and server in the same release. Existing runs remain bound to their historical rule version; the loot selector is unchanged.

## Reproduce the isolated SQL audit

Install `@electric-sql/pglite@0.5.9` into a temporary directory (no dependency change to the server), then run from the repository root:

```sh
PGLITE_MODULE=/absolute/path/to/node_modules/@electric-sql/pglite \
  node server/tests/elite_activation_postgres.mjs
```

The default output is `server/docs/endgame/elite-activation-postgres-report.json`; override with `ELITE_ACTIVATION_REPORT`. The report records the actual engine version and SHA-256 hashes of the SQL inputs.

The harness executes unmodified migrations 63 and 76–81 on PostgreSQL 18.6 / PGlite 0.5.9. The reduced dependency fixture derives item/rune identities from migration 59 and executes native monster, AI, skill description, skill binding, and skill effect SQL from migrations 0, 2, 31, and 32. It does not synthesize native boss expectations from migration 81.

Verified outcomes:

- Fresh activation changes only the active rift selector from 5 to 6; all other fixture tables remain byte-equivalent as normalized JSON, including loot selectors and historical rules.
- Replay at version 6 is idempotent.
- The original migration 63 published-rule triggers continue rejecting immutable parent/child edits.
- Nineteen damaged-input cases are rejected without activation, including missing/old selector, disabled/replaced guards, changed config, unpublished rules, missing historical publication, missing class, altered legacy ability, changed weighted loot, missing rune source, incorrect boss identity/AI, missing/disabled/zero-chance native skills, absent descriptions, and unresolved effects.
- Replay at version 7 changes nothing even when obsolete v6 dependencies have been removed or their guard disabled.

Corruption tests deliberately disable and re-enable guards only within rolled-back test transactions to manufacture invalid persisted inputs. The successful activation uses the original enabled guards.

Boundary: this is an actual SQL-engine integration test against a reduced dependency database, not a full historical database import, live deployment, or concurrent-session locking test. No production runtime code or migrations were changed by this audit. The native source fixture selection is intentionally bounded to the dependencies read by the release gate.
