# MAC observation binding contract v1

Implementation foundation for [#673](https://github.com/jordanhubbard/webmux/issues/673)
and the [project authority contract](mac-project-coordination.md). The Go package
[`internal/maccoord`](../webmux/server/internal/maccoord/bindings.go) parses and
resolves this versioned operator document. It has no file loader, environment
discovery, native credential-store access or dispatch path. A separate opt-in
read-route qualifier is described below. No product setting currently enables
this mapping or invokes that qualifier. The read adapter and its human/agent consumers
remain [#674](https://github.com/jordanhubbard/webmux/issues/674).

```json
{
  "schema": "webmux.mac.project-bindings.v1",
  "authorities": [
    {"id": "configured-fleet", "origin": "https://mac.example.test"}
  ],
  "bindings": [
    {
      "owner": "webmux-account",
      "repository_id": "exact-server-resolved-repository-identity",
      "repository": "example.test/team/project",
      "authority_id": "configured-fleet",
      "project_key": "exact-mac-project-key",
      "capabilities": ["project.observe.v1"]
    }
  ]
}
```

The values above are illustrative, not enrollment instructions or live fleet
configuration. Do not identify a project from its display name, local directory
or successful graph request. The repository ID includes the existing resolver's
canonical remote and root-commit witnesses; an explicit administrator mapping
is also supported. The resolver's account, ID, canonical repository and
`verified-git`/`explicit-map` provenance must all match the configured scope.
Session/browser input cannot supply its own verified observation.

MAC currently selects graph tasks by an authority-scoped `task.project` string.
`project_key` names that exact key; it is not a fabricated project UUID. An
operator-selected authority ID identifies a configuration entry, not a verified
remote principal. HTTPS origins are accepted; cleartext is limited to explicitly
configured loopback origins. Credentials in URLs, path/query/fragment routing,
invalid ports and unsupported schemes are refused. Later transport code still
needs normal TLS checks, fixed operations, redirect refusal, bounded responses
and actual upstream authentication/capability qualification.

There is exactly one binding per account/repository ID. Conflicting or duplicate
entries are refused, including otherwise identical duplicates. Unknown schema,
unknown fields, case aliases, duplicate JSON keys (including escaped equivalents), invalid
text, excessive nesting, oversized documents and unsupported capabilities are
refused. Credentials have no field in this contract. Errors do not echo policy
contents, other accounts, origins or supplied secrets.

Only `project.observe.v1` is requested by this schema. It does not grant that
operation against a live authority. Dispatch, participation, claims, leases,
resource fences, review and publication cannot be enabled by adding capability
names. No machine nickname is an authenticated worker identity. No session,
task attempt, lease, source revision or admission outcome is inferred from a
project selection. The later observation and participant contracts must retain
those separate, potentially unlinked identities.

The parsed policy has a SHA-256 revision over normalized, sorted entries. Origin
root-slash differences and entry order do not change the revision; changed
bindings do. Resolution returns independent copies. Later observation caches
must include this revision plus account and repository scope, and must recheck
current bindings before releasing data. A configured mapping alone is never
proof that a cached observation or a live credential remains authorized.

## Read-route authentication observations

[`CheckReadAccess`](../webmux/server/internal/maccoord/read_access.go) resolves
the receiving account and server-verified repository before making any request.
It accepts a credential supplied by trusted server plumbing and an optional
operator-owned CA pool; it never discovers native profiles or reads token files.
Three GETs to the same fixed project-graph route check absent credentials, a
fresh invalid credential, and then the intended credential. Both negative
requests must return 401/403. The positive response must have the exact supported
schema and selected project key. Graph `found` and unrelated graph content are
not copied into the result and never prove project existence.

The method uses a five-second total context budget, no retries, no ambient proxy,
no redirects, normal TLS verification and bounded response headers/body. It
returns no evidence on cancellation, refusal, unsupported schema, malformed or
ambiguous JSON, wrong project, excessive body size or untrusted TLS. Errors omit
credentials, endpoints and response bodies while preserving cancellation through
`errors.Is`. Every call repeats the authentication observations, so an earlier
success cannot bypass a subsequent credential refusal.

The result is `webmux.mac.read-authentication.v1`. It records the binding revision,
authority/project selection, local observation time and the three read-route
outcomes. Principal identity stays `unverified` and other operations stay
`unqualified`. It is not a worker identity, enrollment receipt, least-privilege
scope audit, lease, admission or publication grant. The selected origin still
needs trusted deployment provenance: allowing a configured loopback URL does not
verify an SSH tunnel's host key, publisher or continued ownership of that port.
These checks are prerequisites before supplying a real credential; status-code
controls alone cannot authenticate an arbitrary local listener.

Controlled Go HTTP/TLS tests cover normal refusal/success, anonymous and
arbitrary-bearer configurations, changed credential acceptance, foreign accounts,
redirects, exact wire fields, size bounds, TLS trust, cancellation and data
redaction. Deliberately removing either negative control or exact policy-field
checking causes the relevant controls to fail. This is client-contract evidence,
not live MAC qualification by itself.

A subsequent [same-host native qualification](validation/mac-bindings-2026-11-22/native-read.json)
used two dedicated MAC clients with exactly the `read` scope. The native hub-owner
CLI enrolled them without activating a different profile. The test pinned the
already-running local TCP publisher to its OS account, PID and process-start
fingerprint before supplying either credential. Actual `CheckReadAccess` calls
passed for the observer and temporary probe. After native revocation, the probe
was refused while its expiry was still in the future; the observer continued to
pass against the same publisher. Both clients refused a generic write request to
an intentionally nonexistent route. Existing client metadata and active profile
were preserved. The observer profile remains private and local; the temporary
probe is revoked. No credentials or enrollment manifests are published here.

This qualifies those same-host read/scope/revocation observations. It does not
qualify an SSH tunnel, remote publisher, actual project existence or a complete
least-privilege audit. The synthetic project key only exercises authentication.
No product loader or human/agent consumer invokes this helper yet.

## Execution-host source observation

`ObserveSource` adds the versioned `webmux.mac.source-observation.v1` boundary.
It resolves the receiving account and exact configured repository before touching
its server-supplied worktree. Actual Git remote/root witnesses must match that
binding. The result includes the binding revision, configured authority/project
selection, `verified-git` provenance, canonical worktree, exact commit, full branch
ref (or explicit detached state), dirty flag and observation time.

The helper uses a five-second total budget and bounded Git output. It strips
inherited Git routing/config overrides, disables filesystem-monitor hooks and
optional index writes, and never fetches or contacts MAC. Repository witnesses
are rechecked, and the commit/ref must match on both sides of the status and
second repository check. Refusal returns no source metadata; errors do not expose
paths, remote configuration or command output, and cancellation remains detectable.

This is a local metadata observation, not an atomic filesystem snapshot or lock.
The dirty flag covers tracked and ordinary untracked changes, including staged
changes; ignored outputs are outside its scope. The commit identifies a Git
object and does not identify the content of a dirty worktree. Concurrent edits,
including a change and restoration between samples, cannot be excluded. No task,
lease, roadmap item/revision, approved branch policy or accepted publication head
is inferred from this observation. Those require separate authority/source joins
and validation at admission/publication. No product consumer invokes this helper
yet. See the [source qualification](validation/mac-bindings-2026-11-22/source-observation.json).

## Task, attempt-counter and lease observation

`ObserveTask` reads only `/dashboard/observe/tasks/{task_id}`, schema
`mac.dashboard.observe.task.v1`, after resolving the receiving account and exact
repository mapping. It applies the same missing/invalid/intended-credential
controls to that fixed route. TLS, redirect/proxy refusal, five-second context,
response limits, duplicate-key checks and redacted errors are shared with the
graph qualifier. There is no arbitrary route, query or mutation proxy.

The output `webmux.mac.task-identity.v1` contains only the selected task's ID,
project key, state, owner-worker ID, reported attempt counter, lease ID/expiry,
update time and authority/local observation times. The returned task ID and exact
project key must match; foreign-project and missing tasks both return
`ErrUnmapped` with no partial metadata. Descriptions, commands, transcripts,
history, evidence, unrelated task fields and credential data are not returned.
Exact wire keys are required, including explicit null for absent owner/lease
fields. Missing or malformed fields cannot become an unleased observation.

MAC's `attempt_count` is retained as a counter, not converted into an invented
attempt UUID or fence. Lease observation distinguishes `unleased`, `incomplete`,
`observed_expired` and `observed_unexpired`, comparing expiry with the reported
MAC server time. This does not validate a claim or authorize execution/publication.
The owner field can survive completion and does not establish current activity.
Machine identity remains unavailable; WebMux session and source links remain
unlinked until independently joined. A machine nickname or matching task title
never supplies those joins.

The chosen observability route uses read-only queries at the inspected MAC
revision. Ordinary `GET /tasks/{task_id}` (including its compact view) can rebuild
missing task-flow records, so it is not used for this observation boundary.
The [task receipt](validation/mac-bindings-2026-11-22/task-identity.json) records
controlled checks and real read-only qualification against the user-supplied
fleet-root task. That task's actual unleased state was preserved, and a different
project selection plus a missing task were refused. This test-only mapping does
not enroll a WebMux project or session. The explicit lowercase `webmux` graph
selection returned zero nodes; no project-existence conclusion follows. No
product browser, CLI or MCP consumer invokes this reader yet.

## Separate session and worker/machine observations

`ObserveSession` converts trusted WebMux broker inventory into
`webmux.mac.session-identity.v1`. It requires the receiving owner, verified
repository binding and complete node/account/session/incarnation reference.
Execution-host and transport labels remain separate from canonical MAC IDs.
Present, absent, unknown and revoked observations retain their original timestamps;
freshness remains unqualified until the view applies its explicit policy. Broker
activity does not prove MAC task execution. MAC machine/worker/task/attempt/lease
fields remain null and `mac_link_state` remains `unlinked`. Free-form declared work
is not copied into a task ID. Projection performs no I/O or registration.

`ObserveTaskWorker` starts from a newly authorized project task and follows only
its owner through the audited read-only agent and machine GET routes. It returns
`webmux.mac.worker-identity.v1` with canonical worker/machine IDs, separate display
labels, reported status/health, deletion and observation timestamps. It rechecks
the worker's machine/deletion association and task owner/state/attempt/lease
around the join. Changed observations yield no partial tuple. Task, worker and
machine reads each retain their negative authentication controls and share a
five-second total budget. Unowned tasks cannot trigger arbitrary worker lookup.
The MAC agent/machine records are unversioned; the adapter requires the exact
inspected fields and supplies its own versioned projection.

A shared worker's other current task, resources, capabilities and persona context
are withheld. A deleted historical owner remains identified as deleted. Neither
reported busy/healthy/trusted fields nor an unexpired lease grant execution or
publication. The joins are metadata sampling, not an atomic snapshot; changes
and restoration between samples remain possible. Freshness remains unqualified,
and the WebMux session and source links remain unlinked.

The [worker/session receipt](validation/mac-bindings-2026-11-22/worker-session.json)
separates controlled positive joins from native evidence. The real fleet-root task
was unowned; the native reader correctly refused to invent its worker. No positive
native worker/machine or session join is claimed. No product consumer invokes
these helpers. In particular, the existing standalone owner view filters revoked
members: the future project view still needs a supported retained-inventory path
before it can expose offline/revoked sessions. These helper tests do not complete
that lifecycle requirement or #673/#674.

## Ordered implementation

The following are sub-issues of #664 with actual GitHub blocker relationships;
creating this plan does not dispatch parallel fleet work:

1. [#673](https://github.com/jordanhubbard/webmux/issues/673): versioned identity
   and operation-capability boundary. This primitive is a first implementation
   component; remaining wire mappings and integration qualification stay open.
2. [#674](https://github.com/jordanhubbard/webmux/issues/674): trusted read
   projection shared by human and session-agent consumers.
3. [#675](https://github.com/jordanhubbard/webmux/issues/675): ordinary/managed
   interactive participation through MAC's execution authority.
4. [#676](https://github.com/jordanhubbard/webmux/issues/676): authority-side
   scope/resource and source-roadmap admission.
5. [#677](https://github.com/jordanhubbard/webmux/issues/677): project-native
   review/publication and stale-attempt refusal.
6. [#678](https://github.com/jordanhubbard/webmux/issues/678): real three-host
   human/agent acceptance and lifecycle/failure controls.

The current binding primitive does not complete #673, #674 or the overall
convergence outcome. In particular, it does not yet answer the four product
questions or reconcile existing participants and execution backends.
