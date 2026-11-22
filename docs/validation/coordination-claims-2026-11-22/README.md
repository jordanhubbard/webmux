# Cross-node advisory claims and overlap notices

This [source-bound receipt](receipt.json) covers the
[cross-node claim contract](../../session-coordination.md#cross-node-claim-checks-and-overlap-notices)
under [#630](https://github.com/jordanhubbard/webmux/issues/630). It leaves full
real-agent collaboration, shared worker authority and the overall roadmap open.

Claims remain owned by the originating controller. A new claim also checks current
authorized same-project peer summaries, returning remote conflicts with full
node/account/session identities. A separate background reconciliation detects
claims registered between refreshes, without a browser or another claim request.
Lease aging uses remaining duration at the exporting authority, anchored before
the source request, so clock offsets/transit cannot extend a remote claim.
Inactive owners' unexpired leases remain advisory conflicts. Incomplete or legacy
clockless context is explicit and fails the CLI/MCP success check.

Each recipient gets one stable event per ordered claim pair. Local detection is
labeled `peer-observation`; later authenticated peer delivery upgrades it to
`remote-inbox`, retaining the entry and explicit receipt state. Incoming notices
must identify a still-live matching recipient claim. Agents cannot create the
internal notice kind through their mutation API. Sending requires the separate
outgoing/incoming message permissions and advertised overlap support; reading a
summary alone can only create a local observation. Released/expired source claims
cannot initiate new notice delivery. Exact retries, renewal and reconstruction
do not enqueue another event under the same claim cycle.

Native macOS ARM64 → Linux ARM64 qualification uses independent real Git
repositories/worktrees and ordinary broker-owned PTY children over routed pinned
HTTPS. The source registers a file claim before the receiver registers an
overlapping directory claim. The ordinary background workers detect the overlap,
create its local observation and deliver one remote notice. The recipient's own
scoped API explicitly records delivered/acknowledged/completed; the source sees
completion, and a new claim check returns the original remote owner. Repeating
that request preserves its frozen result. The run also repeats the parent lost
handoff-reply/status recovery and revocation controls. It opens no browser and
calls no manual refresh/maintenance function. The receiver witnesses identity
proof before bearer credentials on each actual TLS connection. The fixture
certificate does not modify OS trust stores; credentials and processes are
removed after the run, with no installed service/user agent restarted.

Deterministic and HTTP controls cover reciprocal observations coalescing with
actual delivery, explicit completion before later event admission, concurrent
claims before refresh, task/branch/file/directory overlap semantics, full account
identity, foreign/unrelated project isolation, missing/stale/legacy peer context,
clock offsets, relative expiry, summary-only scope, immutable notice payloads,
released claims, stable renewal and exact-request receipt freezing. Parent
messaging/crash and local ownership controls remain in the full suite.

This establishes advisory conflict/event protocol behavior, not exclusive file
ownership, actual peer agent awareness or exactly-once external execution. Remote
reservations, live runtime notifications/existing-agent onboarding, managed/SSH
provisioning, explicit project mappings, scale/retention/backoff, platform custody
and the complete independently started real-agent workflow remain open. No roadmap
checkbox is completed here.
