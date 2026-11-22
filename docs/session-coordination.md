# Built-in session coordination checkpoint

The server now owns a durable, enabled-by-default coordination ledger under
[#630](https://github.com/jordanhubbard/webmux/issues/630). This checkpoint exposes
a scoped local CLI to ordinary local/exec sessions, attaches native MCP guidance
through qualified ordinary CLI aliases, and records all broker session identities.
Its advertised scope is `node-local`. Managed/SSH execution-host onboarding,
cross-node coordination and full real-agent acceptance remain open in the
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
operation receipts and monotonically increasing reservation fences persist.
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
execution-host provisioning; cross-node membership, delivery and shared authority;
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
