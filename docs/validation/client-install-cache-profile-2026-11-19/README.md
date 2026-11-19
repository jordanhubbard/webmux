# Reuse the client build profile for the private launchd fixture

Issue #542 retains the unchanged 30-second client-install timeout from hosted
qualification. A later local compiler-stage comparison on `694ee83` passes all
266 helpers in both cache conditions, but observes shared standard-library
compilation again in the cold client build. That local pass does not repair or
reproduce the hosted failure.

A separate four-case local experiment starts with a fresh private Go cache,
compiles the existing launchd fake, then uses `go list -deps -json` to inspect
cache staleness for the unchanged default client build. It neither builds the
client nor requests exported package data during inspection. Changing fixture
trimpath and CGO independently shows:

| Fixture preparation | Client runtime/reflect/encoding-json cache |
| --- | --- |
| trimpath, CGO 0 | stale |
| default path profile, CGO 0 | reusable |
| trimpath, CGO 1 | stale |
| default path profile, CGO 1 | reusable |

Client-specific packages remain stale, as expected. This isolates an avoidable
cache-profile mismatch; it does not identify the historical worker's cache,
CPU allocation or complete timeout cause.

Remove only the private fake's explicit `-trimpath`. Keep its CGO 0 setting,
Go-only preparation, 60-second compile bound, native query execution and all
readiness/image-hash checks. The fake is temporary and is not distributed; its
receipt checks the actual executable hash. The real client build arguments,
full helper scheduling, 30-second install assertion, one-second service queries,
production service behavior and distributed build profiles remain unchanged.
No extra client prebuild or cache preparation is added to the helper suite.

Local changed-source qualification passes all 266 helpers in both fresh-cache
conditions, with no skips, and all five strict TypeScript projects. The cold
client case takes 2,323.546 ms; the local baseline takes 4,430.854 ms. Warm takes
269.738 ms (baseline 225.211 ms). These are individual local observations, not
statistical timing guarantees. The cache-reuse control is the deterministic
mechanism evidence. The new observer records the existing fixture CGO 0 setting
and matching path profile; real client arguments remain unchanged.

[The source-bound receipt](receipt.json) retains the code revision, exact source
hashes and raw-log hashes. No new hosted jobs are permitted by the owner's
local-resource instruction. Earlier hosted failures and all local controls
remain retained; #542 and the complete roadmap/release acceptance stay open
until their required scope is demonstrated.
