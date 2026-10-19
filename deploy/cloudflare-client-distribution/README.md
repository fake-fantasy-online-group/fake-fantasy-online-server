# Cloudflare client distribution (source and local validation only)

This directory is a **not-yet-deployed** distribution source. There is no real
account ID, domain, bucket, client binary, original resource set, production
catalog, token or key in it. `example.invalid` is deliberately nonfunctional.
Do not treat passing local tests as a deployed service or an installable release.

## Architecture and boundary

- Workers Static Assets serves the download page and a copy of the existing
  幻想数据库 frontend. The public copy retains item/equipment filtering, details,
  pagination, saved filters, copy history and the copy-only GM command handbook.
  It removes account management and live relation requests. Original local GM
  files under `server/cmd/dispatch/web` are unchanged.
- The Worker reads **only** `current.json` from an optional private `RELEASES`
  binding. `/dispatch.json` returns its `dispatch` object, keeping the actual
  `servers` array and `update.schemaVersion: 1` contract. `/release.json` returns
  the same descriptor for download links and the catalog. Both are `no-store`,
  including HEAD and error responses. Missing or invalid configuration is 503,
  never fake download links or an empty invented server list.
- `update.resourceManifests[platform][appVersion]` contains exactly
  `{manifestUrl, sha256, sizeBytes}`. An explicitly present empty map remains
  authoritative; it never falls back to an unrelated platform/version. Legacy
  `resourceManifestUrl` is optional when the map is present. No incompatible
  top-level fields are added to `/dispatch.json`.
- Every package, TSV manifest, passive resource and catalog snapshot is served
  from a **separate R2 public custom domain**, directly. They are never streamed
  through the Worker. Immutable keys are under `releases/<releaseId>/...`.
- There is no `/api/gm` route, origin reverse proxy, generic bucket reader, public
  write endpoint, database binding or database connection. Catalog filters run
  in the browser against the explicitly exported public snapshot.
- Game TCP traffic still connects to the server address in dispatch. HTTP
  distribution and Cloudflare Tunnel are not a generic TCP game proxy.

The release descriptor is a private publication envelope, not a new client wire
schema. Its fields are `schemaVersion:1`, `releaseId`, `defaultDispatchUrl`,
`artifactBaseUrl`, `dispatch`, and optional pinned `catalog`. Website and client
metadata switch together through one conditional private R2 object write.

## Local validation

Node 22+ is sufficient; tests and release tools have no npm dependencies:

```sh
cd deploy/cloudflare-client-distribution
node --test test/*.test.mjs
node tools/release.mjs --help
```

The tests create tiny synthetic files in the OS temporary directory and never
contact Cloudflare. Override `TMPDIR` if necessary. Optional real browser tests
need an installed Chromium and permission to open local sockets:

```sh
CHROMIUM=chromium node test/browser-check.mjs
```

This browser test uses a loopback server and CDP-fulfilled synthetic artifact
responses. It covers retries, unavailable release, search/details, copy-only GM
navigation, Back/Forward, hash failure and recovery. It is not a real package
install or a production data validation.

For a local Cloudflare-runtime preview, use an already approved installation of
Wrangler from the official registry (do not automatically install or log in):

```sh
wrangler dev --local --config wrangler.jsonc --port 8787
```

Without a local `RELEASES` binding, the static shell loads and metadata returns
503 as intended. For a populated local preview, copy the config to ignored
`wrangler.local.jsonc`, add a local `RELEASES` R2 binding, and seed its local
`current.json` with `wrangler r2 object put ... --local`. Never use `--remote`
for fixture testing. Browser screenshots and generated datasets stay outside Git.

## Prepare a release offline

1. Export the public item/equipment catalog using
   [the read-only exporter](../../docs/public-catalog-export.md). No account,
   character, inventory or credential tables are included. Inspect the export
   before authorizing public upload. A catalog is optional; an omitted catalog
   is shown as unpublished.
2. Build each desired platform client through its existing approved build
   process. The release tools do not build, sign, install or run packages.
   Android accepts `.apk`; Windows `.exe`, `.msi`, `.zip`; macOS `.dmg`, `.pkg`,
   `.zip`; Linux the client's supported package extensions. Filename and 8 GiB
   package limits match `ClientArtifactDownload.cs`.
3. Generate an online `FFO-MOBILE-1` TSV for each exact platform/application
   version using the client repository's `scripts/generate-resource-manifest.py`.
   Use `--client-platform` and `--client-version` explicitly. Place manifests in
   the immutable resource-set directory alongside their relative source files.
   Multiple platform/version manifests may reference identical payload files.
4. Save a private dispatch template containing the **actual approved servers**
   and the intended `update` object. At least one server and `update` are required
   for publication. Each advertised downloadable client must have a matching
   exact resource map entry (or a matching single-platform legacy manifest).
   URLs must be canonical HTTPS without credentials, query strings or fragments.
   Existing pins, if supplied, must match. Missing hash/size pins are computed
   from the local files; release tooling never invents missing files.
5. Create a private release configuration outside the repository. Example
   structure (all filenames and addresses below are placeholders):

```json
{
  "releaseId": "release-1.2.3",
  "defaultDispatchUrl": "https://download.example.invalid/dispatch.json",
  "artifactBaseUrl": "https://assets.example.invalid/",
  "dispatchFile": "/private/dispatch-template.json",
  "catalogManifestUrl": "https://assets.example.invalid/releases/release-1.2.3/catalog/manifest.json",
  "files": [
    {"path": "/private/build/client.apk", "key": "releases/release-1.2.3/clients/client.apk"},
    {"path": "/private/build/windows.zip", "key": "releases/release-1.2.3/clients/windows.zip"},
    {"path": "/private/build/macos.zip", "key": "releases/release-1.2.3/clients/macos.zip"}
  ],
  "directories": [
    {"path": "/private/resources", "keyPrefix": "releases/release-1.2.3/resources"},
    {"path": "/private/catalog-export", "keyPrefix": "releases/release-1.2.3/catalog"}
  ]
}
```

`files` and `directories` can be combined. Every included file must be reachable
from dispatch/resource manifests/catalog; extras are rejected rather than
uploaded. Do not point directories at your home, repository or general build
folder. Materialize symlinks first. Config, dispatch input, originals and staging
must be outside the Git checkout. A new staging directory is required:

```sh
node tools/release.mjs prepare \
  --config /private/release.json \
  --out /private/stage-release-1.2.3 \
  --default-dispatch-url https://download.example.invalid/dispatch.json
node tools/release.mjs verify --stage /private/stage-release-1.2.3
```

The explicit CLI dispatch address must exactly match the private config. Build
all three clients with this same approved default dispatch URL using the client
build script's explicit parameter. Preparation only copies/hashes local files
and writes a private `plan.json` plus `current.json`. It does not upload anything.

The resource graph validator checks UTF-8, online TSV header, exact `@client`,
source and destination passive-file allowlists, safe paths, case collisions,
1–100,000 entries, 8 MiB manifest and 64 GiB resource-set limits. It checks every
referenced file's full SHA-256 and size. Original resource bytes are unchanged.
Hot-update/code publication is deliberately rejected by this passive-resource
workflow. Full-package availability does not imply automated installation:
Android uses the OS installer; supported desktop auto-apply requires the client's
separate trusted-helper/marker/ZIP contract. Other accepted package formats are
manual downloads. This website never promises unattended install.

## Cloudflare changes require separate authorization

Nothing in this implementation authorizes login, new credentials, paid services,
account settings, DNS changes, bucket creation/public access, uploads, Worker
deployment or release announcement. Use command-line tools only after the user
has approved the specific target, scope and costs. Do not use a browser/CUA
workflow for account setup. Existing credentials must be provisioned by the user
through the approved secure process; never put secrets in arguments, Git or logs.

The planned targets are:

- One **already approved public artifact bucket**, with its R2 custom domain
- One **already approved private metadata bucket**, containing `current.json`
- One Worker/Static Assets site bound only to the private metadata bucket
- A reviewed read/write R2 credential scoped to those two buckets

Use Wrangler for Worker configuration/deployment and optional `cloudflared` only
for separately approved development HTTP tunneling. The template has no account,
routes or live bucket. `workers_dev` and preview URLs are disabled. Keep real
values in ignored `wrangler.local.jsonc`. Do not expose the local dispatch/GM port.
A custom domain and bindings must be reviewed before running:

```sh
wrangler deploy --config wrangler.local.jsonc
```

Before publication, configure the artifact origin to return identity bytes,
correct Content-Length, HTTP Range support and `Cache-Control: public,
max-age=31536000, immutable`. Disable transformations/compression for these files;
the client rejects a nonidentity Content-Encoding. Configure R2 CORS for GET/HEAD
from the approved website origin (or a separately approved `*`), and allow
necessary Range headers. Catalog JSON downloads must pass browser CORS.
Never cache `/dispatch.json` or `/release.json` with an overriding cache rule.
Do not attach a public domain to the metadata bucket.

## Explicit publication and rollback

`publish` uses the R2 S3 API from this CLI, with a dependency-free SigV4 transport.
It exposes no management operations and never creates a bucket, credential or
DNS record. Wrangler object PUT does not provide the publication compare-and-swap
workflow, so do not substitute an unconditional `wrangler r2 object put current.json`.

The operator first reads/reviews the exact current metadata ETag using an approved
R2 S3 HEAD command. Initial publication uses the explicit literal `absent`.
`R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY` must already be set in the secure
execution environment. After approval, the command is:

```sh
node tools/release.mjs publish \
  --stage /private/stage-release-1.2.3 \
  --account-id APPROVED_ACCOUNT_ID \
  --artifact-bucket APPROVED_PUBLIC_BUCKET \
  --metadata-bucket APPROVED_PRIVATE_BUCKET \
  --expected-etag '"EXACT_CURRENT_ETAG"' \
  --confirm-publish
```

The tool rehashes local staging, compares the prior ETag before uploading,
creates new artifact keys with `If-None-Match: *`, verifies existing keys instead
of overwriting them, and checks full bytes through both S3 and the public HTTPS
origin. Public verification rejects redirects, missing CORS/immutable headers,
wrong size/digest and compression. Only after **all** files pass does it PUT
`current.json` with `If-Match` (or `If-None-Match: *` for first publication). It
then reads back and verifies the committed descriptor and returned ETag.

A transport error/timeout on a write is an uncertain outcome: inspect exact
objects/current state; do not blindly retry or replace the expected ETag with the
latest value. A 412 means another publication won. A failed artifact upload or
verification leaves the old pointer unchanged. Uploads that succeeded before an
error remain immutable and can be safely verified/reused on a later authorized
retry. No cleanup deletion is automatic.

This transport uses single PUT for files up to 5 GiB. A larger already approved
artifact needs a separately approved immutable multipart upload; the publisher
can verify/reuse it but will not create it with an oversized single PUT. Client
artifacts still cannot exceed 8 GiB. Retain old release directories and their
private staging plans. Rollback is a new, explicitly confirmed publication of
that previously verified plan with the current ETag, not an unconditional copy
or deletion. All old bytes are reverified before the pointer changes.

After publication, verify `/dispatch.json` and `/release.json` from the real site,
then run client smoke tests for actual Android/macOS/Windows builds. Inspect the
chosen platform/version TSV, first-run resource download, interruption/resume,
package hash, and supported OS handoff. No synthetic test proves code signing,
real installation, public DNS, Cloudflare billing or production connectivity.

## Upstream references

- [Workers Static Assets configuration](https://developers.cloudflare.com/workers/static-assets/binding/)
- [Wrangler R2 commands](https://developers.cloudflare.com/workers/wrangler/commands/r2/)
- [R2 public custom domains](https://developers.cloudflare.com/r2/buckets/public-buckets/)
- [R2 CORS](https://developers.cloudflare.com/r2/buckets/cors/)
- [R2 conditional S3 PUT](https://developers.cloudflare.com/r2/api/s3/extensions/)
- [R2 S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/)

The custom transport and mocked integration tests are not a live R2 acceptance
test. Run the separately approved staging-cloud checks before production release.
