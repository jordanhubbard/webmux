# Authorized project context across WebMux nodes

This [source-bound receipt](receipt.json) covers the
[read-only peer-context contract](../../session-coordination.md#authorized-peer-discovery)
under [#630](https://github.com/jordanhubbard/webmux/issues/630). It does not complete
cross-node coordination or the full roadmap.

The native run uses macOS ARM64 as the controller and Linux ARM64 as the receiving
node, connected over a real routed network with direct TLS. Both use independent
real Git fixture repositories and linked worktrees with the same canonical remote
and root history. Each controller owns real ordinary PTY test children. Accounts,
server homes, databases, certificates and grants are isolated fixtures. No installed
service or active user agent is restarted; the Linux artifact transfer is verified
by SHA-256. The test trusts only its isolated fixture certificate; it does not
qualify deployment certificates or alter OS trust stores.

The final client makes no manual refresh calls and opens no browser. The normal
server maintenance loop reaches each required state:

1. The enrolled HTTPS peer has a local-account mapping, but the receiving grant
   is still terminal-only: coordination is refused, with no snapshot data.
2. The fixture operator enables project summaries on the receiving grant. The
   controller's background loop discovers two matching remote ordinary sessions.
   Node, receiving account and verified Git project all match the enrolled scope.
3. A different local account cannot borrow the mapped peer credential.
4. The operator revokes receiving permission. The background loop records refusal
   and clears the previously retained data.

The receiver witnesses each actual accepted `net.Conn`. Every credential-bearing
snapshot/control request follows a successful identity proof on that same TLS
connection; the proof request itself carries no bearer. Fixture control endpoints
exist only in the opt-in Go test binary. They are not product routes. The temporary
receiving grant and private fixture credential are removed after qualification.
These are ordinary process/protocol controls, not native LLM/coding-agent workflow
acceptance, mDNS discovery or installed-package qualification.

Deterministic/race controls additionally cover differently named local/receiving
accounts, private inbox/outbox omission, project filtering, unknown/unverified
mapping refusal, grant/global policy revocation, network loss and 30-second expiry,
late-response refusal barriers, legacy editor behavior, and real MCP schema/transport
consumption of peer context. Local MCP actions refuse remote reservations rather
than accidentally acquiring a local resource. The management UI defaults sharing
off and exposes it only after node identity verification.

Checks: full Node 24 server suite, Go vet, scoped race tests, 479 frontend tests,
frontend lint/production build, strict TypeScript and the production CLI/MCP
HTTP/WebSocket/restart contract. Windows ARM64 server/test cross-compilation is
compile evidence only. The parent #637 Windows job identified two fixture portability
errors; they were fixed separately in `0f909db` without changing runtime scope or
recording policy. Hosted validation for that exact fix remains a separate gate.

Limits: read-only point-in-time context, bounded refresh/cache/view payloads, no
transitive authorization, no private message export, no remote mutation or worker
lease grant. Cross-node durable delivery/receipts/authority, real coding-agent
handoffs/overlap/shared-worker acceptance, managed/SSH onboarding, explicit project
maps, large-context pagination/scale, native platform custody/distribution and full
LAN discovery/reconnect acceptance remain open. No roadmap checkbox is completed.
