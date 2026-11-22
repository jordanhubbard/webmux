# Project coordination through MAC

Proposed architecture amendment for [#664](https://github.com/jordanhubbard/webmux/issues/664),
2026-11-22. This proposes the implementation contract. Read the [four-product investigation](coordination-integration-investigation.md) for alternatives and evidence. The bridge is not yet
implemented or qualified. It supersedes any implication in the earlier provider
comparison that WebMux should become a competing execution authority for
MAC-connected projects.

## Questions the product must answer

For each project, the same human and agent interface must show:

| Question | Required authoritative evidence |
| --- | --- |
| What are agents on this machine doing? | Canonical project, machine, worker and interactive session; current task/attempt, declared write scope, lease and observed activity, with freshness. |
| What are agents on other machines doing? | The same identities and state from the shared execution authority, independent of which host serves the UI. Unknown/offline participants remain visible. |
| Are we duplicating or conflicting? | Task acquisition and scope-admission outcomes from the authority, including owners and acknowledged resolution. A process inventory or advisory overlap notice alone cannot prove exclusivity. |
| Is this the right roadmap order? | Source requirement and revision, project priority policy, dependency verdicts and admission reason. A queue sort or heartbeat alone cannot prove compliance. |

A quiet terminal is not proof of inactivity. An unavailable authority is not an
empty project. An unlinked session is not safely coordinated. Views must name
missing, stale, truncated or unregistered observations and distinguish declared
intent from an action or completion proved by evidence.

## Authority and reconciliation

| Domain | Authority | WebMux behavior |
| --- | --- | --- |
| Requirements, priorities and delivery policy | Each project's roadmap/issues, ADR/fix process, branch/review/release rules and installed Literate AI guidance. | Link the exact source and revision. Project-native edits update the source first. A projection never becomes a second editable backlog. |
| Shared execution for a MAC-connected project | MAC's canonical project/task/attempt ledger, admission, leases, dependencies and evidence/review/publication machinery. | Read through an authenticated adapter; send authorized mutations to this authority with stable request identity. Refresh receipts after writes. |
| Interactive access and execution context | WebMux node/account/session capabilities and verified execution-host repository bindings. | Map a session to its real worker/task attempt; present the same state to humans and supported agent channels. |
| Provider processes and conversations | Existing lifecycle owner: ordinary session, MAC worker, Agent Deck or Rig. | Registration and attachment never launch a replacement, restart a conversation, impersonate a worker or rewrite native approval policy. |
| Derived state | WebMux caches, transport receipts and source-to-task mappings. | Record source version, observed time and authority provenance. Surface drift and failed reconciliation; never invent a successful write or silently seize ownership. |

Choose one execution backend explicitly per verified project binding. A
MAC-connected project must not also admit work through the standalone WebMux
ledger. Retain historical local records as such; migration must reconcile live
owners and preserve history before any backend change. During an outage, show
stale state and refuse new authority-dependent admissions. Preserve uncertain
requests for status reconciliation instead of blindly replaying mutations.
Standalone mode retains its bounded local coordination behavior and says that
independent MAC workers are outside its authority.

## Identity and source binding

The versioned adapter contract must bind these concepts without conflating them:

- Source repository identity, project/branch policy, requirement or issue URI,
  roadmap/policy revision and dependency identifiers.
- Canonical MAC project/task identifiers; execution attempt and lease identifiers,
  validity and authority-issued ownership/fence information where supported.
- Canonical machine and worker identifiers, independently of display names.
  A host persona is not the identity of every coding session on that host.
- WebMux node, receiving account, session incarnation and execution host;
  original provider identity and conversation where a supported channel supplies it.
- Worktree, branch and exact head/evidence/publication revision; declared write
  scopes and any project-approved shared resource reservation.

These are required meanings, not a claim that MAC already exposes one combined
wire schema. Audit and version actual fields/capabilities before shipping the
adapter. Never fabricate a fence token or infer authority from a task title,
local path, worker nickname, CWD or advertisement. Shared repository identity
needs verified normalization or an explicit mapping across hosts/worktrees.
Source import must deduplicate by source identity and preserve the existing
external-task translator's ownership; repeated observations cannot spawn another
task. Unmapped source items and ambiguous bindings require a visible resolution.

Authorization is per project/account and operation. Read-only access must not
imply dispatch, claim, credential or terminal access. An interactive participant
uses its own revocable identity and supported claim/start/heartbeat/evidence
workflow; it never borrows a named fleet worker's credentials or administrator
identity. Lease loss preserves work but removes authority to publish under that
attempt. MAC and project-native integration gates must verify the accepted exact
attempt/head at publication; the browser cannot be the enforcement boundary.

## Existing capability and gaps

### Read authorization audit

At the inspected MAC revision, a tenant binding constrains writes; reads
intentionally expose fleet state. Ordinary project/dashboard GET routes require
`read` scope. The project-graph handler discards the authenticated principal
before building its result, and task listing accepts optional caller-supplied
tenant/project filters. These are selection filters, not principal-derived row
authorization. See [the pinned principal contract](https://github.com/jordanhubbard/mac/blob/99925eaff54076310b58f15e0fb4ba9079746673/src/mac/api.py#L91),
[GET scope selection](https://github.com/jordanhubbard/mac/blob/99925eaff54076310b58f15e0fb4ba9079746673/src/mac/api.py#L2037),
[project graph](https://github.com/jordanhubbard/mac/blob/99925eaff54076310b58f15e0fb4ba9079746673/src/mac/api.py#L4549)
and [task selection](https://github.com/jordanhubbard/mac/blob/99925eaff54076310b58f15e0fb4ba9079746673/src/mac/api.py#L4907).

For this contract, a WebMux server is a trusted projection boundary. An
operator-owned read credential stays in local server configuration; browsers,
ordinary sessions and their MCP/CLI surfaces receive only the authorized
projection. A receiving account/session must map to a verified canonical MAC
project and repository binding. A project display name, tenant field or query
argument cannot establish that permission. Use fixed upstream operations and
an explicit output-field allowlist; do not expose an arbitrary fleet API proxy.
The adapter must qualify the actual installed authority's authentication and
capabilities before enabling this mode, including refusal after credential
revocation. An HTTP 200 alone does not prove that a read principal was enforced.

For example, a WebMux account bound only to project Alpha must not receive
project Beta's tasks, worker activity, transcripts or dependency details merely
because the server's MAC read credential can see them. This applies to related
objects and error responses as well as the selected task list. Cross-project
edges, shared workers, ambiguous project names, pagination/truncation, stale
cache entries, foreign accounts and changed bindings need explicit negative
controls. Hidden or unavailable facts remain unknown; filtering cannot turn
missing evidence into proof that no conflicting work exists.

Read permission grants neither dispatch nor a fleet worker's identity.
Interactive admission uses a separately authorized participant and supported
MAC claim/lease workflow. The read adapter never borrows Rocky, Natasha or
Bullwinkle's worker credentials. This audit adds an implementation prerequisite;
it does not establish a shipped adapter or a live authorization qualification.

### Execution authority contracts

Read-only source inspection used MAC revision
`99925eaff54076310b58f15e0fb4ba9079746673` and Literate AI revision
`ebf88985be10f733dd64f469b7cd4aa685ce9083`. It did not dispatch tasks, change fleet
policy or qualify a new live adapter. The inspected MAC contracts are:

- `docs/integration-authority-contract.md`: canonical/derived state, read/write/
  reconciliation policies, timestamped observations and idempotent findings.
- `docs/book/03-projects-and-tasks.md` and `docs/task-dependency-semantics.md`:
  project/task APIs, held work and dependency outcomes. Dispatch classes and
  aging are scheduling hints, explicitly not correctness gates.
- `docs/book/04-machines-and-agents.md`: separate canonical machine and worker
  identities, revocable credentials, heartbeat availability and bounded task leases.
- `docs/book/05-evidence-review-completion.md` and
  `docs/book/11-publication-and-refs.md`: exact-attempt independent review,
  remotely verified canonical integration and guarded reconciliation of divergence.
- `docs/adr/0034-project-mission-control.md`: an existing bounded GET-only project
  graph, schema `mac.dashboard.observe.project_graph.v1`, with explicit omitted
  rows/edges and join verdicts. Reuse its authority semantics where applicable;
  a graph snapshot is not a complete agent/session/write-scope view.

Literate AI's `.mac/project.yaml` is `mac.repository_contract.v1`: toolchain,
bootstrap, project-owned test command and evidence requirements. It does not
supply the roadmap/task/session bridge. Keep those project-native gates intact.

Task leases do not by themselves prove cross-task file/scope exclusivity.
MAC's documented dispatch hints do not by themselves enforce the source
roadmap's ordering. Audit existing admission, resource and publication paths,
then extend the execution authority where needed for declared conflicting
scopes, strict project ordering and stale-attempt rejection. Do not implement
those rules as a second lease registry inside WebMux. Neither backend can
prevent an arbitrary unregistered process from editing local files; show that
boundary and enforce participation/publication where project policy requires it.

## Delivery order and acceptance

1. **Authority/capability mapping.** Agree and version the source/project/session/
   attempt mapping and operation capabilities. Reconcile #630's closed tracker
   state with its unchecked historical requirements. Keep #664 open for the
   clarified outcome. No dispatch or new live participant is implied by this ADR.
2. **Read-only project/fleet view.** Use actual authenticated MAC reads, source
   revisions and WebMux session observations. Show local/remote current work,
   dependency/priority reasons, owner/freshness and unlinked participants to both
   human and agent readers. Honor bounds, pagination, account scope and outages.
3. **Interactive participants.** Add ordinary and managed session adapters using
   the same admission/lease authority as fleet workers. Register actual session
   identities, preserve existing conversations and supported native channels,
   and distinguish task admission, delivery, acknowledgment and completion.
4. **Authority-side admission.** Reuse or extend MAC's atomic task acquisition,
   declared write-scope/resource checks, dependency policy and source-priority
   revision handling. Report concrete refusal/negotiation reasons. A priority
   change never silently cancels another owner's running work; follow the
   project's explicit policy and revalidate at the relevant admission boundary.
5. **Review/publication.** Bind evidence to the exact attempt/head and enforce
   project-native tests, independent review, branch and release gates. Refuse
   stale-lease publication and reconcile divergent refs with canonical receipts.
6. **Native three-host acceptance.** Run real cooperating agents and a human
   WebMux view across three authorized hosts. Both surfaces must see the same
   project/task/attempt identities and authority outcomes. Prove duplicate-task
   acquisition refusal, conflicting-scope refusal or acknowledged negotiation,
   dependency and strict-priority policy, source-priority changes, lost-lease
   publication refusal, and preservation of project-native delivery gates.

Create implementation dependencies in that order before releasing work. Include
reconnect/restart, offline/stale/unlinked sessions, foreign accounts/projects,
revocation, bounded/truncated views and uncertain/idempotent delivery controls.
Retain failed controls alongside successful receipts. A mocked projection,
heartbeat, tool catalog or standalone-only pair is insufficient acceptance.
DNS-SD and authenticated enrollment keep their own #631 transport/identity gates;
working MAC connectivity cannot substitute for them.
