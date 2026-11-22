# Built-in session coordination checkpoint

The server now owns a durable, enabled-by-default coordination ledger under
[#630](https://github.com/jordanhubbard/webmux/issues/630). This checkpoint exposes
a scoped local CLI to ordinary local/exec sessions, attaches native MCP guidance
through qualified ordinary CLI aliases, and records all broker session identities.
It supports local actions plus separately authorized cross-node context,
messages, advisory conflicts and shared reservations. SSH/mosh project context
can now be verified on its execution host. Key-authenticated SSH sessions also
have a native-qualified scoped CLI channel on the tested macOS/Linux topology;
managed-provider onboarding and complete real-agent acceptance remain open in the
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

## SSH and mosh repository context

Ordinary SSH/mosh sessions now resolve an authenticated live directory on the
execution host through a fixed `coordination-project` recovery RPC. The request
uses the existing session authentication over SSH stdin; it carries no shell
command and does not expose an agent-facing path selector. The receiving helper
runs the same bounded, read-only Git verification as local sessions. A matching
directory on the controller cannot establish remote repository identity.

The broker checks the live run, owner/incarnation, transport, SSH destination,
account, port, key selection and directory observation before and after the RPC.
Network/Git work occurs outside its lock. The existing four-second rotating
maintenance budget includes each remote query's two-second limit. A failed
refresh clears that observation; an unrefreshed remote observation expires after
25 seconds. Cache entries are scoped to the session, owner, destination, live broker run and
authenticated activity. Superseded lookups cannot replace newer observations. Entries are discarded against the complete broker inventory, so one account's partial refresh
does not evict another account's context. Global disable prevents new refresh
rounds, clears cached verification and invalidates prepared/in-flight cache
updates; requests already in flight retain their original bounds. No project binding is restored as
current authority after controller restart. Observed path syntax follows the
execution host, including Windows and Unix paths, rather than the controller OS.

This read-only query works with transcript recording disabled and does not add
remote directory/command history to the execution-host recovery cache. It needs
a helper supporting the new operation. Older or unavailable helpers remain
`execution-host-verification-unavailable`; missing live directory observations
remain `working-directory-unobserved`. Provider-owned attachment viewers remain
excluded from this ordinary-session path.

The [repository RPC component result](validation/coordination-ssh-project-2026-11-22/README.md)
proves production RPC authentication, repository identity and a branch change
between macOS and Linux. It is separate from the scoped channel below.

## Private SSH coordination channel

The controller issues only the new session's coordination capability. Its SSH
bootstrap authenticates with the selected execution-host account, prepares a
private Unix socket directory, and installs an SSH stream-local reverse forward
to the controller's private loopback agent listener. The remote host opens no TCP
coordination listener. Administrator and federation credentials are not supplied.

Before publishing the remote locator, the helper reads a real scoped view through
the forward and checks its complete node/account/session/incarnation identity.
Only then does it atomically replace the access file and start or reattach the
existing provider. A refused probe preserves the old file. If an installation
reply is uncertain, the new private forward remains with its owning SSH run
because the file might already have been published. Closing that run closes its
private master. Each run has its own control path, so an older close cannot
terminate a newer run's connection.

Normal qualified native aliases use the same CLI/MCP client and private locator
as local sessions. MCP reloads access on every call. A shell-only
`ssh-stream-local-configured` observation means the channel was configured and
verified, not that a coding agent adopted its tools. Without that observation the
member remains `execution-host-onboarding-pending`; native alias policy/version
statuses remain separate. No terminal input, startup prompt, native tool approval
override or per-session coordination toggle is injected.

The bootstrap capability probe honors the original SSH command's selected
`accept-new` first-use policy; fixed background RPCs keep strict host-key checking.
A read-only, authenticated query of the provider's current pane supplies initial
directory context even if a shell event happened before attachment with capture
off. This live directory/status metadata is not persisted as recovery history.

The [native channel qualification](validation/coordination-ssh-channel-2026-11-22/README.md)
passes ordinary SSH startup, scoped view/action, a fresh private known-host file,
reconnect using the exact retained pane PID, access replacement and private-master
shutdown. Its forwarding-refusal control leaves an ordinary usable shell without
advertising a configured channel. No installed service or existing provider was
restarted. The [corrected-stack qualification](validation/coordination-ssh-channel-integration-2026-11-22/README.md)
repeats those cases and proves public account deletion refuses the retained SSH
CLI's reads and actions while preserving its original pane and terminal lifecycle.

The [native SSH agent qualification](validation/coordination-ssh-agent-2026-11-22/README.md)
also passes two real Claude Code 2.1.319 runs through the ordinary alias. An
independent audit checks actual scoped view/work results and the matching source
ledger. This uses an explicit two-tool native approval profile; other permission
policies remain unqualified. The
[persistent native SSH reconnect qualification](validation/coordination-ssh-native-reconnect-2026-11-22/README.md)
keeps the same Claude and stdio MCP process alive across a planned reconnect
between completed turns. It verifies the old SSH master stops before new native
view/work calls succeed through refreshed access. The
[corrected-stack reconnect qualification](validation/coordination-ssh-reconnect-integration-2026-11-22/README.md)
additionally observes the same processes after the completed second turn and
regresses stale project-cache writes. The
[production controller restart qualification](validation/coordination-controller-restart-2026-11-22/README.md)
also preserves that native process pair, task, original claim ID/expiry and
history epoch through two separate production executable processes; the native
agent then releases its retained claim. The
[corrected-stack restart qualification](validation/coordination-controller-restart-integration-2026-11-22/README.md)
observes the original processes and pane again after the completed post-restart turn.
SIGKILL, message/reservation continuity,
unexpected tunnel loss and interrupted delivery remain unqualified.

This requires compatible helpers at both ends, supported SSH multiplexing/
stream-local forwarding, and a private POSIX socket path of at most 100 bytes.
Older/refusing helpers leave normal SSH available. Password-authenticated startup,
native Mosh/Windows, unexpected tunnel-loss repair, existing agents that lack an
MCP channel, broader coding-agent adoption and complete controller-restart/claim/
message/reservation continuity remain acceptance work under #630. Existing cache
artifacts may remain after a run; session capabilities still require a live,
authorized broker subject. Native per-run socket-cache retention/cleanup policy
needs completion before broad lifecycle closure.

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
not a second durable owner of remote state. Remote reservation actions require separate authority permission. Remote messages and overlap notices require the
separate permissions and durable protocols below. Shared reservations now use the single-authority protocol below; full real-agent
acceptance remains required before the complete workflow is finished.

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
only after another live source/project check and a fresh sharing authorization
check immediately before admission. A known admitted message that
becomes absent reports `history-lost`; a fresh receiver database reports
`history-changed`. Neither causes automatic replay. The receiver atomically stores
accepted/refused outcomes and deduplicates exact envelope retries. Conflicting
payloads under one ID are refused. Original request results remain frozen even
if permissions later change. This guarantees unique inbox admission for retained
history, not exactly-once execution of external work.

The message worker requires no browser. It wakes after queuing or every two
seconds, with four network workers, an eight-second batch budget and five-second
per-record budget. It polls an active-ID index rather than scanning historical
completed message text. Federation message envelopes are limited to 64 KiB to
allow JSON escaping of valid text and bounded identity fields; receipts are
limited to 32 KiB. The ordinary agent action API retains its 16 KiB input bound,
and message text retains its 8 KiB semantic limit. Long-term retention, large-outbox
pagination/scale and backoff tuning remain open. No network occurs under the
broker or ledger lock. Shutdown cancels and joins the worker before ledger close.

The [native protocol receipt](validation/coordination-messages-2026-11-21/README.md)
covers macOS/Linux delivery with a deliberately lost committed reply, exact retry
and explicit recipient completion using ordinary PTY fixtures. Real coding-agent
handoffs, real-agent overlap negotiation and shared-worker acceptance remain open.
The [admission follow-up](validation/coordination-message-admission-2026-11-22/README.md)
covers revocation during status recovery and valid text expanded by JSON encoding.
Global policy withdrawal also clears retained peer observations and invalidates
preparing, queued and in-flight fetch authorization. Re-enabling requires a fresh
peer observation; a delayed pre-disable response cannot restore current context.

## Cross-node claim checks and overlap notices

A `claim` stays in its originating ledger. The response also includes conflicts
from current authorized same-project peers and `peer_claim_checks` with full node
and receiving-account identity. Remote claims use the same task/branch exact-name
and file/directory containment rules as local claims, including conservative case
folding when either filesystem requires it. Different local and receiving account
names retain their separate identities. Unexpired advisory claims still count
when their owner is inactive; they expire by lease, not by an assumed disconnect.
These checks never confer an exclusive file lock or a remote worker lease.

Each check reports its snapshot `state` and `notification_policy` (`allowed` or
`unavailable`). Current context is a point-in-time observation, not proof that
another node cannot claim concurrently. The source's network request start plus
the remaining lease at `authority_time` bounds remote claim lifetime; transit and
clock offset cannot extend it. The normal snapshot TTL also applies. Clockless
older summaries remain readable discovery context but cannot establish a complete
ownership check. A missing/stale/disabled/unavailable peer is not an empty claim
list. The local claim is recorded, while the CLI/MCP tool reports incomplete
remote context as non-success and requires rechecking before overlapping edits.
A new intent can renew the original claim and check current context; an exact
retry always returns its original frozen conflicts/context result.

After each background peer refresh, the server reconciles still-active local
claims against remote observations. This detects simultaneous claims made before
either node had refreshed. It records a durable local `overlap` event labeled
`transport: peer-observation`, even when only summary permission is available.
That label means this node observed a conflict; it does not assert that the peer
has received or acknowledged a message. No terminal text is injected.

Separate message permissions plus the peer's `overlap_notices` capability allow
an immutable notice to enter the remote claimant's coordination inbox. The
sender rechecks its live session/project and original unexpired claim before new
transmission; the receiver rechecks its active recipient and matching unexpired
claim before first admission. Release or a different claim cycle refuses stale
notifications. A request cannot forge the internal `overlap` kind through `/act`.
These envelopes use the durable message worker, history epoch, status-before-replay
and explicit receipt protocol above. A summary permission never grants sending.
`notification_policy: allowed` describes available permission, not delivery.

Both controllers derive the same recipient-specific event ID from the original
two claim IDs. A local observation and a later authenticated notice coalesce into
one inbox entry, changing its transport to `remote-inbox` while retaining explicit
receipts. If the recipient already reviewed and completed the observed conflict,
a later delivery reports that event's existing completion; this is acknowledgment
of the same overlap event, not proof of external task execution. Renewal, repeated
refreshes and reconnect/reconstruction do not create another event in that claim
cycle. A released/expired claim reacquired with new intent has a new claim ID.

The [macOS/Linux protocol receipt](validation/coordination-claims-2026-11-22/README.md)
qualifies ordinary PTY fixtures, including claims created before peer refresh.
Real coding-agent overlap negotiation and shared-worker acceptance remain open.

## Shared heavy-worker reservations

Every `node:<node_id>:heavy` resource has exactly one authority: that node's
retained coordination ledger. Local and authorized remote reserve/renew/release
requests use the same atomic reservation record and monotonically increasing
fence, across accounts and projects. A peer's availability summary is never a
lease, and no reservation starts a job or takes over a provider. Participating
workers must honor the authority node, history epoch, original record/fence and
expiry; stop work before release or lease loss.

The outgoing peer and incoming grant must separately enable
`coordination_reservations`, in addition to the mapped local account, enrolled
HTTPS identity and incoming summary permission. The incoming grant binds
`source_node_id`/`source_owner` to its receiving `owner`. Message permission does
not enable reservations, and reservation permission does not enable inbox writes.
A fresh reserve or renew requires a current snapshot advertising `reservations`
for that source namespace and a verified project available on the receiving
account. If several accounts on the same node are eligible, provide `target`
as a matching peer member ref to select the account; ambiguous selection is
refused. The worker resource itself remains node-wide. Other accounts see
occupancy/fence/expiry, not the foreign holder. Visibility uses the receiving
account, preserving a differently named source account's full identity.

Use the existing `reserve`, `renew-reservation`, `release-reservation` actions
with that remote resource ID. The source atomically freezes the exact request,
selected peer/node/account/key/project and receiving history epoch before network
I/O. Responses include `shared_reservation` with the durable `intent_id`,
authority identity and transport. `reservation-pending`, `uncertain` and
`authorization-unavailable` do not grant work. Retry the identical intent with
the same `request_id` to resolve uncertainty; do not create another reserve ID
as an automatic retry. Coordination does not require a browser.

The fixed HTTPS endpoint is
`POST /api/federation/coordination/reservations`, with `operation: status|admit`
and an immutable envelope. Before mutation bytes, the source commits uncertainty
and rechecks its current live session/project and current outgoing sharing scope.
After interruption or a lost reply,
it queries the original receipt first; authenticated absence in the same history
permits exact replay. The authority atomically records every outcome, including
busy/project/lease denials. Old busy IDs cannot acquire a later free worker, and
old renew IDs cannot extend the original grant. `history-changed` or disappearance
of a known outcome (`history-lost`) stops replay. Current configuration supplies
credentials; the saved intent never stores a bearer or silently changes scope.

A successful original outcome is historical evidence. Work requires both an
unexpired returned `expires` and `lease.current: true`. The source verifies the
exact authority record/fence is still held and uses request start plus the
remaining authority duration as a conservative local deadline. Clock offsets and
transit cannot extend it. That deadline is frozen on first receipt and cannot be
extended by status queries or retries; a new renew intent is required. Releasing
or replacing the lease makes an old successful receipt non-current. A known local
release also fences delayed replies from reviving that lease. Status/network
failure removes current verification. After either a status or admission reply,
the source checks outgoing sharing again; revocation preserves the historical
outcome while removing current-lease permission. An RPC started before revocation
can still commit at the authority. Workers must not infer a grant from the
old `reserved`/`renewed` status. A changed source project cannot keep using its
old grant. A live original holder may still release its retained record/fence
after changing projects, using its frozen receiving scope.

Local reservation replies also include the current-lease check. Lease-aware
clients automatically declare `X-Webmux-Coordination-Lease: 1` on the private
interface. Older clients receive HTTP 428 for reserve/renew/release, rather than
ignoring current-lease evidence. New clients refuse positive results lacking a
current lease and expiry, including replies from older controllers. Ordinary
view/claim/message compatibility remains separate. Reservation calls have a
12-second client budget to cover the bounded four-second source inspection and
six-second two-host exchange; ordinary client calls retain five seconds. The
private server retains its ten-second write bound, so a boundary timeout remains
uncertain and must be resolved under the original ID. Envelopes/replies are
limited to 32 KiB; redirects and proxies remain refused. Shutdown/restart preserves
intent, outcome, history and fences without dispatching jobs.

Schema 3 adds scoped remote reservation intent receipts and receiving-account
custody semantics, preserving version-1/2 signing keys, history epoch, messages,
request receipts and fences through an atomic upgrade. Older binaries refuse
schema 3. Do not downgrade by deleting the ledger or reset/roll back authority
while workers may still hold leases. Fresh-history recovery requires operational
reconciliation; fencing is meaningful within the authority's retained epoch.

The [native protocol receipt](validation/coordination-reservations-2026-11-22/README.md)
uses ordinary PTY/API fixtures on macOS and Linux. Two real coding agents honoring
the resource while running their own jobs, managed/SSH onboarding and complete
reconnect/discovery acceptance remain open. The
[admission review](validation/coordination-reservation-admission-2026-11-22/README.md)
covers outgoing revocation during delayed status and admission replies.

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

For native inbox waiting, `webmux_coordination_view` accepts `after` and optional
`wait_seconds` (0–30). Use the last `next_cursor` to wait for new entries without
shell sleeps. The adapter polls the existing scoped API at bounded intervals,
reloads private access each time, and returns on arrival, cancellation, refusal
or its deadline plus at most one bounded read request. Waiting never advances
message receipts. Callers that omit the field retain immediate-view semantics.
Native policy may still require approval for MCP tools: provisioning an interface
is not permission to bypass that policy. Supported exact versions include Claude
2.1.319 with its separately qualified explicit session-tool allowlist. See the
[two-host native qualification record](validation/coordination-native-pair-2026-11-22/README.md)
for tested policies, failed attempts and remaining gates.

Native macOS ARM64 acceptance covers Codex 0.171.0 and Claude 2.1.327 launched
independently through ordinary broker-owned PTYs, with transcript recording off
and no browser. Each called the scoped MCP view, resolved a verified Git project
and discovered its three same-account members. See the
[native receipt](validation/agent-coordination-2026-11-21/README.md). Those are two
separate single-agent startup checks on one host. The later
[two-host native workflow](validation/coordination-native-pair-2026-11-22/README.md)
qualifies Claude 2.1.327/macOS and 2.1.319/Linux under explicit scoped-tool
permissions: real counterpart discovery, conflict/busy checks, acknowledged
handoff and a guarded coding/verification job under the shared fence. Codex's
read-only/never mutation refusal remains a failed workflow attempt with unchanged
policy. Managed/SSH, other permissions and discovered/restart acceptance remain
open.

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

Administrators can inspect their registry using
`GET /api/session-coordination`. They can use **Manage servers →
Automatic session coordination** to read and change the server-wide policy.
The control reads back the authoritative setting after saving. A lost response
or failed readback shows an unconfirmed state; **Reload coordination policy**
checks the current value without repeating the write. It does not add a
per-session opt-in or change optional provider preferences.

`GET /api/session-coordination/policy` returns only the current policy, without
project queries, peer requests or membership refresh. This read and the existing
`PUT /api/session-coordination/policy` require administrator access (or the
existing no-auth administration mode). Set `{"enabled":false}` to disable and
`{"enabled":true}` to re-enable. The setting persists in the durable ledger.
Disabling does not stop terminals/providers or erase receipts.

## Storage and recovery

The lazy ledger is `$WEBMUX_HOME/data/coordination/state.db`, backed by pinned
[bbolt](https://pkg.go.dev/go.etcd.io/bbolt). New directories/files use 0700/0600
where Unix permissions apply. One service owns a database file at a time. Stable
capability signing material, policy, membership, claims, inboxes/outboxes,
operation receipts and monotonically increasing reservation fences persist. Schema
version 2 atomically upgrades an intact version-1 ledger with remote outbox/inbox
indexes and a random receiver history epoch, preserving signing keys, existing
receipts and fences. Missing version-2 buckets are corruption, never recreated.
Schema 3 additionally records remote reservation intents and receiving-account
custody. Older binaries cannot read the upgraded authority; do not downgrade by
deleting its data.
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
execution-host provisioning; real-agent cross-node delivery/overlap and shared-worker
execution acceptance;
explicit project mappings; platform custody/sandbox acceptance; full native LAN
discovery and reconnect qualification; and the complete two-real-host workflow.
No roadmap checkbox is completed by this checkpoint.

## Account removal

Coordination admission follows the current WebMux account policy. Removing a
local account refuses its old coordination capability without stopping its
terminal. Background maintenance marks that retained membership inactive for
coordination, and restoring/reconnecting its broker entry cannot provision access
under the removed account. In unauthenticated mode, only the `anonymous` owner
qualifies. The [account-removal regression](validation/coordination-account-revocation-2026-11-22/README.md)
records the original failure and corrected ordinary-session/race results.

## Explicit native configuration

The native Codex adapter preserves explicit MCP configuration in compact
`-cKEY=VALUE`, spaced `-c KEY=VALUE` and assigned `-c=KEY=VALUE` forms. An explicit
transport choice likewise prevents its default compatibility override. The
[compact-option qualification](validation/coordination-compact-policy-2026-11-22/README.md)
records the original regressions and isolated native parser evidence. These
checks preserve user choices; they do not qualify the explicitly selected gRPC
transport that failed the earlier native bootstrap experiment.

## Peer mapping revocation

Once refresh observes a mapping's removal, its in-flight responses cannot populate
a re-enabled mapping. Re-enabling the same configuration leaves context pending
until a fresh fetch succeeds. Refused, disabled and invalid observations also
retire requests from the previous authorization period. The
[controlled TLS regression](validation/coordination-peer-mapping-revocation-2026-11-22/README.md)
records the original failure, recovery check and limits of this qualification.
