# Native credential hub qualification

This is the historical initial checkpoint. See the
[qualification after adversarial-review fixes](../credential-hub-review-fixes-2026-11-28/README.md)
for the revised source and 103-control native runs.

The production HTTP API ran on three distinct native machines: Linux AMD64,
Linux ARM64 and macOS ARM64. Each machine served as the designated hub and both
other machines consumed its granted keys. Every receipt passed all controls.

The fixture binaries were built from clean source commit
`a4731c8d43c14253034d09f1ad98faf373030d5d`. Runtime OS/architecture observations
were checked against the actual hosts, and every executable reported that source
revision with an unmodified tree. Endpoint and host identifiers are hashed.

| Hub | Passed checks | Receipt |
| --- | ---: | --- |
| linux-amd64 | 99 | [hub-linux-amd64.json](hub-linux-amd64.json) |
| linux-arm64 | 99 | [hub-linux-arm64.json](hub-linux-arm64.json) |
| darwin-arm64 | 99 | [hub-darwin-arm64.json](hub-darwin-arm64.json) |

Controls cover ordinary links without credential permission, explicit named
grants, both supported private AI consumers, foreign account/project/consumer,
changed receiving node and hub pin, concurrent single-use redemption, replay,
real handle expiry, rotation, disabled secrets, independent grant revocation,
enrollment replacement, local binding replacement, restart, secret deletion,
metadata/audit redaction, encrypted owner-only custody, missing/unsafe key files,
and hub unavailability without ambient-provider fallback. Receivers retained no
durable copy of the granted API-key values. Each exact owned fixture process was
stopped; production services and native provider profiles were untouched.

The OpenAI and NVIDIA bearer-key consumers were exercised against a dedicated
synthetic HTTPS provider using the supported private HTTP interface. Native CLI
login state, OAuth/device-bound key portability and external paid-provider
acceptance remain outside this evidence. File-based custody and the shared
service-account trust boundary are described in the
[contract and operator setup](../../credential-hub.md).

Original `make test`, `make lint` and `make build` passed on the production
source committed as `5d18e03bcbd95c619f9c69f15d5c268c3f4e1e26`.
The subsequent commit changes only the native qualifier and its regression
test; production source is identical. The two qualifier input tests also pass.
The isolated full-gate runner explicitly declared its supported Bash shell;
the original recovery assertions, deadlines and skips were preserved.

To repeat the native run, prepare a private inventory and cross-built fixture
executables using the schema in `scripts/qualify-credential-hub.py`, a built
frontend and source defaults. Run from a clean commit, choosing each inventory
role as hub in turn. The qualifier requires exactly three independent native
hosts and one local controller. It creates private fixture homes and finite
servers. Keep inventory, keys and raw operational files private.

Independent exact-head review, CI, exact-main validation and production
deployment are separate delivery gates. The broader MAC authority bridge and
native provider enrollment remain tracked in their roadmap issues.

## Receipt SHA-256

- `hub-linux-amd64.json`: `c0c4719dc0b0abf23b18750419bfe44f2582521235ab9f0d82ed758240a14b0b`
- `hub-linux-arm64.json`: `f5cef819521fb2d0a61ac2fdbe2462b1911b30251cf739cf4ba4ff26ebb4b4a3`
- `hub-darwin-arm64.json`: `2dd675b655b2497b4ef93767bd6c40a161b8957b3bce0e4c498525786cf92bf6`
