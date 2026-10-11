# Validation and delivery status

Date: 2026-10-11 UTC. These are source/test results, not a deployed or play-tested release.

## Reproducible checks

From `server/`, using the repository Go toolchain requirements:

```sh
go test ./... -count=1
go test -race ./internal/domain ./internal/game/scene ./internal/session ./internal/store ./internal/protocol
```

From the repository root, `make build` produces the existing gameserver, dispatch, and migrate programs. Building does not deploy or apply a database migration.

Real PostgreSQL fresh initialization, twice-applied populated upgrade, transaction rollback, sequential CAS, and restart persistence passed in a disposable database. See [POSTGRES_VALIDATION.md](POSTGRES_VALIDATION.md) for reproducible tests and exact limits.

## Latest observations

- Final full server suite and the five affected race suites passed after the trial-recovery guard, elite-stat fixes and requirement-view mapping.
- The full server run recorded 899 passing test/subtest events and 43 skips. These are test events, not feature coverage.
- Skips require unavailable PostgreSQL network access or a missing legacy capture. Offline PostgreSQL execution does not replace pgx roundtrips or concurrent database clients.
- Gameserver, dispatch, and migrate compiled successfully.
- Client: seven original managed assemblies compiled without substitutes, including all 221 Protocol files and the gateway.
- Client: 116 model/codec/wire-fixture assertions, 26 actual panel actions, 38 actual host/gateway/crypto checks, 80 source/meta checks and 112 Go/client DTO field-type checks passed. Host tests use documented Unity/presentation substitutes and real gateway/session/crypto code over loopback.
- Existing client regression suites passed separately; see the client Documentation/Endgame/README.md.

## Delivery boundary

Client source and tests are on `feature/daoist-endgame`, checkpoint `297d5be2a307ecb044b905355ff73528262adb44`. Server core/protocol/storage/catalog checkpoint `c2b1a03e3a03652bdec69cc35ff620df14f7b974` is published; later scene/runtime/crafting integration and final tests remain unpublished pending repository-write approval. A separate source recovery archive preserves unpublished work. Do not treat the server branch at that checkpoint as the complete integrated feature.

## Not verified

No Unity Editor, PlayMode, or Player build was run. No rendered UI/gameplay session, multiplayer playthrough, production deployment, live database upgrade, or balance playtest was performed. Managed-only compilation and UI substitutes cannot establish rendered behavior or complete Unity assembly compatibility.

Socket restrictions prevented actual pgx database roundtrips and multi-client concurrency. In-process retry identity survives reconnects but is not a durable client journal across process termination. Restart recovery refunds interrupted paid runs exactly once rather than restoring live monsters.
