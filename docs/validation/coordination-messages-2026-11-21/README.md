# Durable coordination inboxes across WebMux nodes

This [source-bound receipt](receipt.json) covers the
[durable messaging protocol](../../session-coordination.md#durable-messages-across-nodes)
for [#630](https://github.com/jordanhubbard/webmux/issues/630). It leaves the full
roadmap and real coding-agent collaboration acceptance open.

A native macOS ARM64 controller talks to a Linux ARM64 receiving controller over
routed end-to-end HTTPS. Both own ordinary PTY child fixtures, independent real
Git repositories and linked worktrees with matching root history and canonical
remote. Accounts, homes, capabilities, grants and databases are temporary. The
fixture trusts its isolated receiving certificate without changing OS trust.
No installed service or active user agent is restarted. Artifact transfer is
verified by SHA-256; the temporary credential is removed after the run.

The client uses background summary and message workers without a browser or
manual maintenance calls. It first verifies terminal-only summary refusal, then
explicit project sharing and foreign-account isolation. The operator separately
enables message permission on both nodes and binds the incoming credential to
the source node/account. The source queues a handoff to an observed same-project
remote member. The receiver commits its unique inbox record and deliberately
closes the HTTPS connection before returning the first admission reply. The
sender recovers by querying status, rather than submitting another admission.
The recipient's own scoped API explicitly records delivered, acknowledged and
completed. The source observes completion; retrying the original intent returns
its original local result. Recipient scrollback contains none of the message
text. Revocation then clears retained peer context.

The receiver witnesses each actual accepted `net.Conn`: every bearer follows a
successful identity proof on that same TLS connection, whose proof request has
no bearer. Fixture grant/receipt controls exist only in the opt-in test binary,
never in product routes. These are ordinary process/protocol controls; they do
not establish two native LLM agents negotiating work.

Deterministic controls additionally cover atomic queue/request receipts, exact
payload reuse refusal, source inactivity/project change, receiver namespace and
membership refusal, preserved refusal across permission changes, unique inbox
admission across reopen, abrupt receiver process loss after commit, no receipt
advance on reads, explicit receipt ordering, monotonic source receipt copying,
fresh-database history changes, accepted-history corruption refusal, schema
migration preserving prior capabilities/results, damaged outbox authority, legacy
summary-only permission, outgoing revocation, foreign-account receipt refusal,
and remote message admission through the actual scoped MCP SDK transport.

Validation includes the Node 24 full Go suite, Go vet, scoped race checks, 480
frontend tests, frontend lint/build, strict TypeScript, and the production
CLI/MCP HTTP/WebSocket/restart contract. Windows ARM64 test cross-compilation is
compile evidence only; hosted Windows acceptance remains an independent gate.
See the receipt for commands, artifacts, hashes and exact scope.

The protocol guarantees unique coordination inbox admission while history is
retained, not exactly-once external execution. Never roll back a live authority
or discard receipts to retry. Outbox retention, large-context pagination/scale,
backoff tuning, live native notifications/existing-agent onboarding, cross-node
claims/overlap and shared reservation authority, independently started real-agent
handoffs, managed/SSH provisioning, platform custody/distribution and full LAN
reconnect acceptance remain open. No roadmap checkbox is completed here.

## Revocation fixture ordering follow-up

The original PR-head Windows ARM64 server run failed
[the revocation control](https://github.com/jordanhubbard/webmux/actions/runs/45000609614/job/137141101284):
the new intent returned `peer-unavailable` before the test could establish its
pending outbox. The test canceled background maintenance immediately before
requiring a current snapshot. Cancellation can finish an in-flight fetch as
unavailable/stale, which is a valid refusal under the production freshness rules.
The fixture now cancels and joins its isolated source workers before establishing
the snapshots used by this control. This removes that setup race without changing
production behavior, source eligibility, deadlines or revocation assertions.
The original failed run remains failed; the correction's hosted native acceptance
is a separate gate. The source-bound receipt above retains the original runtime
and native protocol evidence rather than rewriting that history.
