# Scoped MAC project observation v1

Work in progress for [#674](https://github.com/jordanhubbard/webmux/issues/674),
using the [versioned binding boundary](mac-project-bindings.md). The Go
`ObserveProject` collector supplies `webmux.mac.project-observation.v1` for the
planned shared human and agent read path. No product configuration, HTTP endpoint,
browser setting or MCP tool invokes it yet. #673/#674 and the overall #664 outcome
retain their remaining integration and acceptance gates.

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

`Bindings` is an immutable selection snapshot. A future runtime consumer must
resolve current configuration and recheck its revision before releasing cached
or in-flight data after a policy change. This collector keeps no response cache
and does not implement configuration reload or execution-backend selection.
Human/agent consumers must share this projection and preserve unavailable,
unlinked, stale and withheld states without falling back to a competing ledger.

The [qualification receipt](validation/mac-project-view-2026-11-22/receipt.json)
separates controlled cases, deliberate mutation controls and real bounded MAC
reads. Native reads use a private explicit fleet-root test context, with no new
project, task, worker, provider, credential or service mutation. They do not
establish a live WebMux product binding or finish the human/agent acceptance.
