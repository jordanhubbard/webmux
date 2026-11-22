# Built-in session coordination checkpoint

The server now owns a durable, enabled-by-default coordination ledger under
[#630](https://github.com/jordanhubbard/webmux/issues/630). This checkpoint exposes
a scoped local CLI to ordinary local/exec sessions, attaches native MCP guidance
through qualified ordinary CLI aliases, and records all broker session identities.
Its action scope is `node-local`, with explicitly authorized read-only peer
summaries. Managed/SSH execution-host onboarding, cross-node mutations and full
real-agent acceptance remain open in the
[roadmap](roadmap.md).

## Ordinary session interface

New ordinary children inherit `WEBMUX_COORDINATION_CLI` (the WebMux executable)
and `WEBMUX_COORDINATION_FILE` (a private access file). Existing ordinary children
with their original WebMux recovery environment can locate a provisioned access
file without restarting their provider or extracting credentials. The server
maintains membership without a browser. It never types coordination messages or
startup instructions into terminal input.

From an eligible session:

```bash
"$WEBMUX_COORDINATION_CLI" coordinate guide
"$WEBMUX_COORDINATION_CLI" coordinate view
printf '%s\n' '{"request_id":"edit-login-1","action":"claim","kind":"file","resource":"src/login.go","lease_seconds":120}' |
  "$WEBMUX_COORDINATION_CLI" coordinate act
```

For an already running shell that predates the new environment variables, use
`webmux coordinate` from the installed executable. `guide` prints instructions
and the subject, never the bearer capability. Do not copy the private access file
into logs, prompts, repositories or another session. Reading a guide is not proof
that a native agent has adopted it. The registry distinguishes available aliases,
configured MCP, preserved explicit policy, unqualified versions and unavailable
interfaces. A configured launch is separate from a successful tool invocation.

`view` returns same-account, same-verified-project peers, active claims, inbox,
outbox and heavy-worker availability. Inbox/outbox windows contain at most 100
records each; use `view --after <next_cursor>` to read the next inbox page. Reading
never records delivery or acknowledgment. `act` reads one JSON object, up to
16 KiB, from stdin. It prints the structured result and exits nonzero for refused
admission, an expired original lease or an overlapping claim. An overlapping
claim is recorded and both parties receive notices; negotiate before editing.

Supported operations:

| Action | Additional fields | Behavior |
| --- | --- | --- |
| `work` | `text` | Update this session's task description. |
| `claim` | `kind`, `resource`, optional `lease_seconds` | Advisory task, branch, file or directory ownership; report overlap. |
| `release-claim` | `record_id` | Release this session's claim. |
| `message` | `kind` (`message` or `handoff`), `target` (peer `ref`), `text` | Admit one durable peer message. |
| `delivered`, `acknowledge`, `complete` | `record_id` | Recipient explicitly advances receipt state in order. |
| `reserve` | `resource`, optional `lease_seconds` | Acquire the advertised node heavy-worker resource exclusively. |
| `renew-reservation`, `release-reservation` | `resource`, `record_id`, `fence`, optional `lease_seconds` | Act only on the original holder's current fence. |

Every operation requires a stable `request_id`. Retrying the exact intent after
an uncertain response returns its original committed result; reusing that ID
with different content is refused. A retry does not refresh an expired lease or
retry a cached busy denial. Use a new ID for genuinely new intent. Lease defaults
to 120 seconds, with a 15–900 second range. Claims do not lock files. Reservations
do not run or stop jobs: participating workers must honor expiry and fencing.
There is one resource, `node:<node_id>:heavy`, shared across accounts on this
node; another account/project sees occupancy but not the holder's identity.

## Authorized peer discovery

[Connected-server coordination controls](server-federation.md#share-project-coordination-context)
map one local account to an enrolled HTTPS receiving node/account and separately
authorize incoming project summaries. Legacy terminal grants remain unchanged.
The session's verified repository ID selects remote context; matching folder
names or terminal titles never establish a match. The `members`/`claims` fields
remain local. The nested `peers` field preserves the remote node/account/session
identities and a `mode` of `read-only`. An account may be mapped to a differently
named receiving account; other local accounts see none of that mapping's data.

Summaries contain matching members/tasks/branches/worktrees, advisory claims and
node heavy-worker availability. Inbox/outbox content, request receipts, capability
files and credential values are never exported. Requests use one fixed endpoint,
`POST /api/federation/coordination/snapshot`, with a project selector. Its incoming
grant must explicitly permit coordination, and end-to-end HTTPS is required.
Each response is checked against the enrolled node, receiving account, requested
project and resource authority. The receiving node exports only its local ledger;
cycles in the connection graph do not recurse or confer transitive access.

The server refreshes context on its ten-second maintenance timer with four
network workers, a six-second aggregate budget and a five-second per-request
budget (covering the receiver's bounded Git inspection). A rotating cursor keeps
slow nodes/projects from starving the rest. No browser is needed. Agent requests
read cached summaries and never hold the broker lock during network traffic.
Only `current` denotes a recent point-in-time summary; it grants no lease or
liveness guarantee. `pending`, `stale`, `invalid`, `unauthorized`, `disabled` and
`unavailable` require reporting incomplete context. Network failures retain an
explicitly stale snapshot for at most 30 seconds from its last successful fetch.
Receiving refusals/disabled policy clear data; a late old response cannot revive
context after a refusal. Local mapping, credential, identity or endpoint edits
invalidate the corresponding cache key on subsequent reads.

Each remote response is limited to 1 MiB, retained remote payloads to 16 MiB and
remote additions to one view to 3 MiB. Capacity limits report unavailable context
with `cache-capacity`/`view-capacity` reasons; they do not silently truncate a peer
membership list. Large-context pagination and scale qualification remain open.
A controller restart begins with pending observations and refetches; the cache is
not a second durable owner of remote state. Local reservation tools refuse remote resource IDs. Remote messages require the
separate permissions and durable protocol below. Cross-node overlap notifications
and reservation authority remain required before the full workflow is complete.

The [macOS/Linux qualification](validation/coordination-peers-2026-11-21/README.md)
uses independent real Git repositories/worktrees and ordinary PTY fixtures on two
native hosts. Background refresh discovers authorized project peers, preserves
foreign-account isolation and observes revocation without a browser or manual
refresh. This is protocol and ordinary-session context acceptance; two real coding
agents exchanging work across hosts remain a separate gate.

## Durable messages across nodes

A summary alone grants no message authority. The outgoing peer must explicitly
set `coordination_messages: true` with a mapped local account and enrolled HTTPS
identity. Its receiving grant must separately set `coordination_messages: true`,
`coordination: true`, `source_node_id` and `source_owner`. That credential trusts
one sending controller to assert members only within this node/account namespace;
the receiving account remains the grant's `owner`. A current project snapshot
must advertise matching message permission and a nonempty `history_epoch` before
a fresh message is queued. The recipient must be an active member in that verified
project. Incoming admission rechecks its live broker membership and project.

The existing scoped `message` action accepts the remote member's full `ref`.
Its atomic local result is `status: admitted, transport: pending`: this proves
only durable local queuing. The outbox subsequently reports `uncertain`,
`remote-admitted`, `authorization-unavailable`, `sender-inactive`, `sender-revoked`,
`refused`, `history-changed` or `history-lost` as appropriate. An inactive sender
holds new delivery; removed/revoked/project-changed logical senders cannot send.
A known admitted message's receipt can still be queried after the sender stops.
Receipt `state` advances only through the recipient's explicit `delivered`,
`acknowledge`, `complete` actions. Viewing an inbox does not advance it. Pending
and uncertain transports are not delivery acknowledgments. Messages never type
terminal input, start a provider, execute commands or reserve workers.

The dedicated fixed endpoint is `POST /api/federation/coordination/messages` with
`operation: status|admit` and an immutable `envelope`. Each call uses the enrolled
same-connection TLS identity proof before the bearer credential. There are no
redirects or proxy routing. Current configuration supplies credentials; durable
outboxes retain only the selected peer, receiving node/account, pinned TLS key,
project and receiver history epoch. Changing account mapping, identity or outgoing
permission stops further network attempts. An attempt already in flight may have
committed; revocation cannot retract it. Each direction needs its own grant.

Before sending delivery bytes, the source commits `uncertain`. After interruption
or a lost response it queries the exact message receipt first. An authenticated
`absent` in the same history epoch permits replay of the identical envelope/ID
only after another live source/project check. A known admitted message that
becomes absent reports `history-lost`; a fresh receiver database reports
`history-changed`. Neither causes automatic replay. The receiver atomically stores
accepted/refused outcomes and deduplicates exact envelope retries. Conflicting
payloads under one ID are refused. Original request results remain frozen even
if permissions later change. This guarantees unique inbox admission for retained
history, not exactly-once execution of external work.

The message worker requires no browser. It wakes after queuing or every two
seconds, with four network workers, an eight-second batch budget and five-second
per-record budget. It polls an active-ID index rather than scanning historical
completed message text. HTTP envelopes are limited to 16 KiB and receipts to
32 KiB; message text retains its 8 KiB limit. Long-term retention, large-outbox
pagination/scale and backoff tuning remain open. No network occurs under the
broker or ledger lock. Shutdown cancels and joins the worker before ledger close.

The [native protocol receipt](validation/coordination-messages-2026-11-21/README.md)
covers macOS/Linux delivery with a deliberately lost committed reply, exact retry
and explicit recipient completion using ordinary PTY fixtures. Real coding-agent
handoffs, cross-node claims/overlap and shared-worker acceptance remain open.

## Native agent startup and MCP

The existing execution-host `codex` and `claude` aliases attach a session-scoped
stdio MCP server when the native version and invocation are qualified. Native
startup instructions ask the agent to inspect project peers before work, register
claims, negotiate overlap, acknowledge handoffs and reserve heavy workers. The
native tools are `webmux_coordination_view` and `webmux_coordination_act`;
`webmux coordinate mcp` runs the stdio adapter. It uses the pinned official
[MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0), with
bounded frames and the existing loopback capability client. It reloads private
access per call so a surviving MCP pipe can follow controller endpoint changes.
Tool errors preserve refused admission, lease expiry and claim-conflict semantics.
The adapter owns no provider and runs no job or terminal input.

Native macOS ARM64 acceptance covers Codex 0.171.0 and Claude 2.1.327 launched
independently through ordinary broker-owned PTYs, with transcript recording off
and no browser. Each called the scoped MCP view, resolved a verified Git project
and discovered its three same-account members. See the
[native receipt](validation/agent-coordination-2026-11-21/README.md). These are two
separate single-agent startup checks on one host; a simultaneous two-agent
claim/handoff/reservation workflow remains pending.

Codex 0.171.0's default gRPC code-mode host timed out in native qualification.
Keeping the host enabled and selecting its supported non-gRPC transport passed.
The version-specific adapter adds `--disable code_mode_host_grpc` automatically;
explicit CLI transport selectors are preserved. It does not change sandbox,
approval, model or credential settings. This adjustment is part of the qualified
boundary, not a general statement about other Codex versions.

MCP configuration is supplied only for that launch, using
[Codex configuration overrides](https://developers.openai.com/codex/mcp) or
[Claude's native MCP configuration](https://code.claude.com/docs/en/mcp).
No user/project configuration or credential home is rewritten. Only private
interface locations and an expected session ID enter the native server definition;
bearer/recovery keys do not enter launch arguments. Existing native restrictions
such as safe/bare/restricted mode, strict/explicit MCP configuration, remote mode
or explicit tool selection pass through with a visible policy-preserved status.
Other native versions remain visible as unqualified. Aliases are required;
absolute-path bypasses, older helpers and already running agents do not gain a
new MCP connection automatically. Existing running shells retain CLI locator
access. Live native-agent notification push and manager-specific channels remain
separate work; startup guidance asks agents to check at natural work boundaries.

## Membership, repository identity and authority

Membership retains node/account/session/logical-incarnation identity. The
incarnation derives from the broker session ID and creation time, not a native
process PID. Request admission resynchronizes with observed broker runs under
the broker lock: retained connected flags do not authorize stopped sessions.
Deletion tombstones an incarnation permanently. Existing provider lifecycle,
credentials and launch policies retain their original owners.

Project resolution uses bounded, local, read-only Git commands with inherited
Git environment and global configuration disabled. A canonical remote plus root
commit witnesses identifies a project; linked worktrees retain their individual
paths and branches. Credentials are stripped from remote URLs. Shallow, unborn,
missing or ambiguous repository identities fail closed. A shared remote is a
local identity witness, not remote-server authentication. Public explicit mapping
configuration and execution-host inspection for managed/SSH sessions are pending.
Ordinary shells without a trusted observed working directory remain unverified.
Minimal signed live directory/interface observations are retained independently
of transcript recording, without saving command text/argv as recovery history.
The resulting verified project binding remains durable coordination state. New
PTYs discard inherited outer tmux routing; durable tmux panes explicitly override
shared-server coordination locations and bind to their own session identity.

The agent API lives on a separate literal-loopback HTTP listener with only
`GET /view` and `POST /act`. Its HMAC capability is scoped to the node, account,
session and incarnation. It grants no configuration, terminal, provider, admin
or federation authority. Browser JWTs and federation tokens cannot substitute
for it. The client refuses proxies, redirects and non-loopback endpoints.
Private files rely on the service account's filesystem custody; separate WebMux
accounts running under one OS identity are not an OS-level hostile-code boundary.
Native Windows ACL custody and agent sandbox reachability remain qualification
gates.

Authenticated owners can inspect their registry using
`GET /api/session-coordination`. Administrators can globally disable admission
using `PUT /api/session-coordination/policy` with `{"enabled":false}` and re-enable
with `true`. Disabling does not stop terminals/providers or erase receipts.
The control currently has an API, with no dedicated settings UI.

## Storage and recovery

The lazy ledger is `$WEBMUX_HOME/data/coordination/state.db`, backed by pinned
[bbolt](https://pkg.go.dev/go.etcd.io/bbolt). New directories/files use 0700/0600
where Unix permissions apply. One service owns a database file at a time. Stable
capability signing material, policy, membership, claims, inboxes/outboxes,
operation receipts and monotonically increasing reservation fences persist. Schema
version 2 atomically upgrades an intact version-1 ledger with remote outbox/inbox
indexes and a random receiver history epoch, preserving signing keys, existing
receipts and fences. Missing version-2 buckets are corruption, never recreated.
Older binaries cannot read version 2; do not downgrade by deleting authority data.
Admission and its receipt commit in one transaction. Clock rollback, missing
buckets, invalid key material and changed node identity fail closed rather than
reinitializing authority. A ledger failure leaves ordinary terminal startup
available and reports coordination unavailable.

Stop WebMux before copying/restoring the ledger and private node identity.
Never replace a live database, delete its receipts to resolve a retry, or restore
an old snapshot while workers might still hold newer leases/fences. Backup
rollback/reconciliation and long-term retention/compaction policy need separate
operational design. Access files are regenerated; keep retained ledger and node
identity together.

## Evidence and remaining acceptance

The [qualification receipt](validation/session-coordination-2026-11-21/README.md)
covers a real local database crash/reopen, concurrent reservations, scoped HTTP
admission, ordinary PTY child launches and the production executable's CLI before
any browser opens. These test children are controlled fixtures, not independently
started coding agents.

Remaining #630/#631 work includes complete live native inbox/notification and
existing-agent onboarding; independently started ordinary/managed pairs; SSH/remote
execution-host provisioning; real-agent cross-node delivery, claims/overlap and
shared reservation authority;
explicit project mappings; platform custody/sandbox acceptance; full native LAN
discovery and reconnect qualification; and the complete two-real-host workflow.
No roadmap checkbox is completed by this checkpoint.
