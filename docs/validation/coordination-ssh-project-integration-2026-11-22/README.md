# Execution-host project integration and global cache withdrawal

This [source-bound receipt](receipt.json) integrates
[#646](https://github.com/jordanhubbard/webmux/pull/646) with the corrected stack
through [#644](https://github.com/jordanhubbard/webmux/pull/644) for
[#630](https://github.com/jordanhubbard/webmux/issues/630).
The conflict resolution retains both current account policy and global-disable
checks before repository inspection, plus the main-branch Rig fixture receipt.

Review reproduced a global policy gap: retained or delayed peer snapshots could
appear current after disable/re-enable without a fresh observation. Controlled
real-TLS regressions failed through the policy API, maintenance observing persisted
disable, and direct refresh with stale inputs. Global withdrawal now clears peer
values and authorization scopes, invalidates preparation across the transition,
and prevents queued jobs under withdrawn scopes from starting another fetch.
Direct refresh checks current global policy. Delayed replies cannot commit into
the next authorization period. All three controls require pending context after
re-enable and current context only after a fresh fetch. Already-started requests
may finish; their old observations are discarded.

Final combined-source checks cover the full Node 24 Go suite, scoped HTTP API
race tests including all three controls and inherited revocation regressions,
vet and the production CLI/MCP HTTP/WebSocket/restart contract. Core/client/MCP,
session and recovery race tests plus strict TypeScript also passed on their
unchanged component sources before the cache correction.

A freshly built Linux ARM64 production executable was transferred over authenticated
SSH and verified by SHA-256. The current-source native qualifier independently
checks remote Git identity, refuses five invalid authentication/path controls,
and observes a real branch change without changing project identity. Recording
stays disabled. The initializer and repository live in a new private qualification
cache; no provider, installed service or existing user agent is started or changed.
The earlier successful RPC run before the cache correction is retained separately;
it is not presented as final-binary evidence.

This is native read-only SSH RPC acceptance, not remote agent onboarding, a full
broker/provider SSH/mosh session or a coding-job workflow. Windows execution,
full discovered/reconnect workflow, hosted checks and parent delivery remain
separate gates. The broader roadmap remains open.
