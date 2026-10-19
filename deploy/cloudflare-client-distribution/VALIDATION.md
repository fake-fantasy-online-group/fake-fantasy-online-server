# Validation record (2026-10-19)

Source-only implementation; no Cloudflare account/configuration mutation, object
upload, DNS change, real package installation or release publication was run.

Passed:

- 27 dependency-free Node unit/integration tests (`node --test test/*.test.mjs`).
  Includes all platforms, schema/filename/size/path rejection, immutable
  create-only uploads, exact ETag switching, concurrent-publisher conflict,
  explicit confirmation, network/hash/CORS/encoding failures, retry semantics,
  public route boundaries, public-field projection and browser-side filtering.
- All `.js`/`.mjs` files passed `node --check`.
- Go exporter → private staged release → remote-verification mock → website data
  loader integration, using the exporter-generated two-row synthetic fixture.
  The snapshot SHA-256 was
  `fddb682e4df684be20813b81f525a60c9f643a39b3671fd30a7616bd900f70bf`.
  Its HTML-sensitive JSON text was verified as original bytes, not reserialized.
- Latest `origin/main` fetched and confirmed an ancestor of the feature branch.
- Original `server/cmd/dispatch/web` files remain unchanged.

Not run / blocked:

- Real Chromium UI acceptance: the smoke harness exists as
  `test/browser-check.mjs`, but Chromium could not launch because the execution
  sandbox blocked its local socket. The supported escalation route also failed
  before running the command. No sandbox restriction was bypassed.
- Wrangler/Cloudflare-runtime preview and live R2 SigV4/conditional-write checks:
  no authorized account/environment deployment was available for this task.
- Android/macOS/Windows actual package execution, signing and install acceptance:
  synthetic fixture bytes intentionally are not installable packages.

Before production, run the real-browser harness in a supported local environment,
then the separately approved Cloudflare staging smoke and actual platform/client
checks described in README.md. Passing mocked tests is not proof of deployment.

## Attack-range regression after desktop browser feedback

The later desktop check found `物攻 undefined–10` on a synthetic equipment card.
The exporter legitimately omits zero-valued `minAttack`/`maxAttack` fields via
Go's `omitempty`; `stats.attack` is only the maximum used for numeric filtering.
The public renderer now defaults omitted endpoints to numeric zero while retaining
the actual endpoint fields. Original local GM files remain unchanged.

- Three new renderer regression tests pass: omitted/explicit zero endpoints,
  separate range versus normalized filter fields, and fallback stat selection.
- `node test/attack-range-integration.mjs` passed with the real Go exporter using
  a temporary test overlay and four synthetic rows. It checks exported raw JSON,
  maximum-only filter values, exporter display attributes and the deployed
  JavaScript formatter. The overlay harness disables vet only because the Go
  toolchain cannot vet a newly added overlay-only filename; this does not replace
  the normal exporter test/vet suite.
- The browser smoke harness now requires visible `物攻 0–10` and no `undefined`
  in the item grid. Desktop revalidation of this source fix is still pending.
