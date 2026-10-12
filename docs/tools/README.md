# Cloud Go test-toolchain recovery

Restore a private pinned Go SDK and reproducible test cache locations on Linux x86_64:

```sh
bash docs/tools/bootstrap-go-test-toolchain.sh /workspace/shared/server-toolchains
source /workspace/shared/server-toolchains/env.sh
go version
(cd server && go test ./...)
```

The script downloads the official `go1.27.2.linux-amd64.tar.gz` archive from `https://go.dev/dl/`, verifies SHA-256 `ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5`, and extracts it under the requested directory. The checksum was verified against the official Go download JSON on 2026-10-12. It needs Bash, curl, sha256sum, and tar; it does not need sudo or a system package install.

The generated `env.sh` sets:

- `GOROOT`: `<toolchain-root>/go`
- `GOPATH`: `<toolchain-root>/gopath`
- `GOMODCACHE`: `<toolchain-root>/gopath/pkg/mod`
- `GOCACHE`: `<toolchain-root>/gocache`
- `GOTOOLCHAIN=local`, so tests cannot silently download a different Go SDK
- The official Go module proxy and checksum database
- `PATH`, so `go` resolves to this SDK

Always verify `go version`; some cloud images contain an unrelated program named `/usr/bin/go`. A mismatched existing SDK or cached archive stops the script rather than overwriting an unknown installation. This script does not start a server or database, build a desktop controller, change business code, configure credentials, or modify system settings.

Recovery verification on 2026-10-12: `go version go1.27.2 linux/amd64`; `server/` → `go test ./...` passed. Database-backed validation and runtime integration remain separate checks.
