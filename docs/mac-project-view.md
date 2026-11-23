# Scoped MAC project observation v1

Work in progress for [#674](https://github.com/jordanhubbard/webmux/issues/674),
using the [versioned binding boundary](mac-project-bindings.md). The Go
`ObserveProject` collector supplies `webmux.mac.project-observation.v1`. The server
loads explicit private observer configuration and exposes the same session-scoped
read to the browser Project activity action and `webmux_mac_project_view` MCP
tool. #673/#674 and the overall #664 outcome retain their remaining live fleet
integration, roadmap reconciliation and acceptance gates.

The receiving account and exact server-verified repository select an explicit
MAC authority/project key before any request. Only fixed read operations are
available. The initial project graph must have the supported schema, matching
project key, valid time/counts and an available graph section. A degraded or
missing graph returns an unavailable result, rather than a successful empty list.
Graph `found` never proves a canonical project exists.

Each page selects at most eight task candidates from the bounded upstream graph;
four pipelines operate under a ten-second total budget. Every returned task must
independently match the selected task ID and project. Its title, reported priority,
state, owner, attempt counter and lease observations come from that scoped task
response. Graph metadata supplies only explicit hold/join-policy observations.
Missing metadata stays unavailable. Related worker/machine joins retain their
own identity and ownership checks; failures are counted without partial tuples.

Dependencies are returned only when both endpoints are independently authorized
and present in this page. Foreign, unobserved, outside-page, malformed and duplicate
edges contribute withheld counts without exposing their identifiers, titles,
projects, states or verdicts. Raw graphs, command/transcript data, credentials,
origins and unrelated worker context are never copied into the projection.

The result retains upstream omissions, adapter omissions, failed task/worker
counts and withheld dependencies. These counts have separate meanings and must
not be added into a fabricated total. Coverage stays incomplete whenever these
bounds or failures matter. A valid empty graph is only an empty observation for
the configured key; project existence remains unverified. Conflict and roadmap
ordering verdicts remain unknown, source-roadmap links remain unlinked, and
freshness stays unqualified until a consumer applies its explicit policy.
Reported numeric priority and dependency verdicts do not authorize admission.

Numeric `offset`/`next_offset` select fresh pages, not a frozen snapshot. Upstream
changes can cause repeated or missed rows across pages. The collector does not
claim complete inventory, ownership exclusivity or publication authority.

Authentication controls run on each fixed-route read. Any observed credential
refusal discards all collected rows; a final read qualification precedes release.
Its metadata is included as `read_authentication`, retaining unverified principal
identity and unqualified non-read operations. No earlier successful observation
bypasses a fresh check. The operator must qualify the transport before supplying
a credential; a loopback URL alone does not establish publisher ownership.

`Bindings` is an immutable selection snapshot. The runtime checks private policy
and credential file revisions before releasing in-flight data. It keeps no
response cache. Backend policy is accepted only at startup: editing the policy
invalidates reads and does not silently transfer ownership or enable a new backend.
Admission transactions use the immutable accepted policy without file/network I/O.

## Explicit observer configuration

The server reads `WEBMUX_HOME/config/mac-observation.json`. Its schema is
`webmux.mac.observer.v1`, with three exact top-level keys: `schema`, `bindings`
(the full [project binding document](mac-project-bindings.md)), and `credentials`.
Each credential entry contains `authority_id`, a dedicated `webmux-` prefixed
`client_id`, and an absolute `enrollment_file` referring to that client's original
native `mac.client_enrollment.v1` manifest. No inline token, administrator profile
discovery, environment credential fallback or browser credential setting exists.
The manifest must identify the selected origin/client, contain exactly the `read`
scope and a future expiry. The actual client must also be refused generic write
scope before collection. Configuration is bounded to 128 KiB and enrollment to
64 KiB; regular-file ownership/privacy checks apply. Unix files must be owned by
the server UID with no group/other permissions. Windows checks owner and DACL;
only the server user, SYSTEM and Administrators may have allowed entries.

HTTPS uses normal system certificate verification. On Unix, explicit loopback
HTTP additionally requires a `publisher` object with `pid` and `fingerprint`:
the SHA-256 of `ps -p PID -o pid=,uid=,lstart=,comm=` stdout. The operator must
qualify that publisher first. Both before and after collection, `lsof` must find
only that PID, owned by the server UID, and the process fingerprint must match.
A loopback forwarding process alone does not attest the remote destination.
Windows supports HTTPS for the observer; HTTP publisher attestation is unavailable.
Private configuration paths, origins and tokens never enter the public projection.

An absent optional file preserves existing standalone behavior. Invalid accepted
configuration fences admission without stopping terminals. MAC-bound projects
cannot admit standalone work, claims, reservations, renewals or new messages.
The guard covers exact local receipt replay, queued peer attempts, inbound peer
admission and cached peer lease status. Existing release/receipt actions can drain
history. Unknown repository context or drift of a configured root/remote cannot
select standalone mode. A separate verified, unconfigured project retains its
own standalone mode. No read creates a MAC worker, dispatches a task or grants a
lease, and a refused MAC read never enables standalone fallback.

Before adopting a different execution policy, the owner must reconcile existing
standalone claims, leases, uncertain peer operations and active writers, then
coordinate reopening the server. No claim is imported, deleted or silently seized
by this implementation. This change does not configure or restart installed services.

## Shared human and agent surface

The authenticated browser route is
`GET /api/session-coordination/projects/{session-id}?offset=0`; the existing
session capability interface exposes `GET /project?offset=0`. The browser receives
`webmux.mac.session-project-view.v1`, exactly as the MCP tool does. Only the
bounded offset is request-selectable. The server derives account, incarnation,
execution host and verified repository from the broker. Remote paths are resolved
on their execution host. Account, incarnation, repository, policy and remote
verification are rechecked before release. Global coordination disablement applies
to both routes. Browser tokens and session capabilities are not interchangeable.
The old agent standalone view refuses MAC-bound projects instead of advertising
competing resources; legacy history remains retained for reconciliation.

The terminal's Project button loads on demand, including while disconnected.
The dialog shows reported task priorities, scoped worker/machine identities,
lease observations, omissions and withheld dependencies. Retained session
inventory includes revoked incarnations without reviving them. Broker presence
does not establish MAC participation or task execution. Late session responses,
failed refreshes and revoked access clear the displayed facts. Each next page
replaces the preceding fresh observation instead of fabricating an atomic list.
Conflict and ordering assessments remain explicitly unknown; source-roadmap and
session/worker links remain unverified. These remaining requirements keep #674 open.

The [qualification receipt](validation/mac-project-view-2026-11-22/receipt.json)
separates controlled cases, deliberate mutation controls and real bounded MAC
reads. Native reads use a private explicit fleet-root test context, with no new
project, task, worker, provider, credential or service mutation. They do not
establish a live WebMux product binding or finish the human/agent acceptance.
