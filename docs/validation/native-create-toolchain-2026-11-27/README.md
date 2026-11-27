# Native creation toolchain qualification — 2026-11-27

Issue [#780](https://github.com/jordanhubbard/webmux/issues/780) reported a
missing owner request directory on Linux ARM64 after a direct Go invocation
with Node 26. The production native-creation helper deliberately admits Node
24.x. The source Make bootstrap already enforces that major and supplies owned
Go caches; a direct `go test` does not inherit that environment.

Unchanged source `15d035a0ddfe7ee5edf57d2fe3cb8d21fa5463af` gives these native
results with Go 1.27.4 and the original fixture assertions/deadline:

| Native host architecture | First unsupported runtime | Qualified runtime | Repeated unsupported runtime |
| --- | --- | --- | --- |
| Linux x86_64 | Node 22.22.1: owner-directory assertion fails | Node 24.22.3: passes | Node 22.22.1: same failure |
| Linux aarch64 | Node 26.8.1: owner-directory assertion fails | Node 24.22.3: passes | Node 26.8.1: same failure |

Both architectures also pass `go test -race -p 2` and `go vet` for
`internal/openrigcoord` and `internal/nativeobserve`. Race execution explicitly
uses `CGO_ENABLED=1` with the existing native compiler and `GOMAXPROCS=2`.
The [receipt](receipt.json) binds versions, source-file hashes and retained log
hashes. No production guard, test assertion or timeout changed.

Two initial preparation failures remain separate: the AMD64 shared Go module
cache was root-owned, and the ARM64 ambient environment disabled CGO. Neither
attempt reached its intended test scope; neither is counted as passing evidence.
Owned caches and the normal native race-build setting resolve those conditions.

Use the [targeted Go instructions](../../../README.md#targeted-go-tests), or the
full source Make targets, to select the qualified runtime and writable caches.
A newer major is not a qualified substitute for Node 24 in these helpers.
The platform guides now use the same Node series and Agent Deck 1.16.39
attachment minimum as the maintained source installer.

This is native-host unit/fixture qualification. It does not establish
authenticated Codex creation, conversation recovery, managed-provider activation,
physical input acceptance or completion of #438/#523 or the whole roadmap.
