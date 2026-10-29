# Native UI migration-tail integration contract

From the repository root, with Node.js, Python 3 and `@electric-sql/pglite` available:

```sh
PGLITE_MODULE=/path/to/@electric-sql/pglite \
  node server/tests/native_ui_tail_126_129_postgres.mjs
```

`PGLITE_MODULE` uses the same module-path override as the neighboring PostgreSQL
tests. It may be omitted if Node can resolve the package normally. All repository
inputs resolve relative to the script, not the current working directory. The
test opens only a new in-memory PGlite database; it takes no DSN or player dump.

## Fixed scope

- 126: codex/scroll icon mapping.
- 127: 66 authored fixed-binding policies and active-instance binding repair.
- 128: native book-merchant letter paper.
- 129: rack retirement, pricing, costume categories and NPC ring-stock removal.

The fixture reuses `rack_catalog_source.py` and the committed rack audit, adds the
actual binding/container table dependencies, and adds missing binding templates
from 127's authored policy list. Overlapping template names/tables must agree;
the fixture never overwrites a conflicting identity to make a migration pass.
Owned-instance and account rows are synthetic, not production data.

Expected result: **16 groups pass**. From a pre-tail fixture without binding
columns, the test executes all four init bodies in one transaction, captures the
result, then rolls it back and verifies restoration. It next applies each real
upgrade with its own transaction and compares every fixture table with that
captured result. It also verifies a committed single-transaction replay, temporary
table cleanup, preservation of all 66 policies and owned data across the UI tails,
conflicting-policy rollback, exact rack deltas and the real shop/rack loader SQL.

This is a focused manual integration gate, **not CI coverage**, a full-schema
fresh installation, live PostgreSQL validation or deployment authorization. Keep
running the four neighboring migration scripts and `go test ./cmd/migrate`
(from `server/`) for their separate identity/rollback and initializer checks.
