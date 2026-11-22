# MAC, WebMux, Agent Deck and Rig: integration investigation

2026-11-22. Analysis for [#664](https://github.com/jordanhubbard/webmux/issues/664).
Recommendations below are proposed architecture, not evidence of a working bridge
or authorization to migrate running agents. The previous three-product MCP probe
was too narrow to settle this question: it omitted MAC and tested Agent Deck's
Recall interface, not the capabilities of the whole product.

## Conclusion

For the stated project/fleet goals, converge on **project-native intent and
delivery policy, MAC execution authority, and WebMux interactive access**.
Neither Agent Deck nor Rig needs to be a mandatory intermediary. They retain
useful optional runtime/team capabilities, and existing workers must keep their
current lifecycle owners until an explicitly qualified transition.

This conclusion is about the required product outcome, not the amount of code
already written. MAC and WebMux are not integrated merely because their agents
cooperate. Conversely, MAC is unnecessary overhead for someone who only wants a
local session manager or a persistent coding team without the fleet/project
execution requirements described here.

## Requirements used to weigh the products

The owner needs each project to answer four questions, for all participating
local and remote agents: what they are doing, whether their work duplicates or
conflicts, and whether it follows roadmap priorities and dependencies. The
project remains authoritative for requirements, fixes/ADRs, branch/review and
release methodology, guided by Literate AI. Humans and agents should operate
the same worker pool. Existing conversations and independent process owners
must survive convergence.

These requirements distinguish several meanings of ownership:

1. **Session ownership:** who may launch, attach, stop or restore a process.
2. **Work ownership:** which admitted task attempt may act, for how long.
3. **Write/resource ownership:** whether different tasks may touch a conflicting
   declared scope or consume an exclusive shared worker.
4. **Acceptance/publication authority:** which exact reviewed result may become
   canonical under the source project's gates.

A stable tmux session supplies none of the last three automatically. A task
lease does not automatically lock every file the task might edit. Separate Git
worktrees isolate local edits but still allow duplicated effort and conflicting
integration. A message being delivered is distinct from its recipient accepting
work or its result passing review.

## What each contributes, and where it overlaps

| Product | Valuable capability to retain | Substantial overlap | Limits relevant to these goals |
| --- | --- | --- | --- |
| MAC | Shared project/task/attempt ledger; canonical machines/workers; bounded leases; dependency-aware claim admission; resource/capability placement; evidence, review and canonical-publication records; operational history and fleet administration. | Rig's queue/workflow/role machinery; Agent Deck's conductor supervision and channels; WebMux's new claims/messages/reservations. | Only represented participants and tasks are coordinated. Source-roadmap mapping, strict priority policy, interactive session bindings and cross-task declared file-scope exclusion are not established by a task lease alone. Its shipped browser UI is deliberately read-only. |
| WebMux | Authenticated browser workspace spanning ordinary terminals, SSH/Mosh, VNC/RDP and managed viewers; multi-user accounts; stable node/account/session identity; shared terminal presence; recovery and execution-host context. Human access is useful well beyond coding agents. | Agent Deck/Rig launch/attach/inventory/status/recovery; MAC-style local work descriptions, claims, messages and reservations in the new built-in ledger. | Its existing provider interface is lifecycle-oriented. No MAC task/attempt bridge is implemented. Advisory file claims do not lock files. It does not currently enforce source-roadmap ordering. Recovery receipts retain native/platform limits. |
| Agent Deck | Multi-harness session ergonomics; profiles/accounts; status/hooks and attention; worktree/sandbox helpers; MCP/skill configuration; conversation search/context, usage and quota tooling; remote deck management. | WebMux terminal/view/recovery features; Rig session/team features; MAC/Hermes-style supervision and chat channels through conductors/watchers. | A conductor is an agent following policy, not by itself a project-wide lease/priority/publication authority. Managed inventory excludes unrelated sessions unless adopted. Native profile, hook and account behavior require preservation. |
| Rig | Persistent role-based teams/seats; context/specification/skills; native session continuity and adoption; durable owned work with handoff/blocking/wake outcomes; workflow graphs and typed receipts; cross-host routing/rosters and human interaction routes. | Much of MAC's work orchestration; WebMux's managed-session/recovery/context interface; Agent Deck's team supervision. | Its work records are host-owned. Cross-host queue routing does not create one globally serializable project authority. Installed workflow execution refuses remote host pins. Workflow receipt checks validate declared identities/fields, not substantive proof quality or this project's canonical publication by themselves. |

Agent Deck's conductor is materially more than a passive session list: it can
supervise persistent workers and use event watchers and chat channels. That
makes the conductor role overlap with the existing MAC/Hermes fleet. It does not
justify running two autonomous supervisors for the same worker.

Rig also deserves more credit than the earlier comparison gave it. Its installed
workflow projector enforces frontier identity, allowed outcomes, evidence
presence and typed acceptance values; dependency graphs and durable operation
receipts are real features. Its queue's ownership and closure/notification
semantics are useful even if MAC owns the higher-level project task. A low-level
Rig packet and a MAC task need not be the same object, but their authority and
relationship must be explicit.

## Evidence and current integration gaps

Native version reads returned MAC 1.5.0, Rig 0.6.14 (`9d0200f3`) and Agent Deck
1.16.34. Installed Rig implementation files were inspected, including
`queue-repository.js`, `workflow-runtime.js`, `workflow-validator.js`,
`workflow-projector.js` and `claim-service.js`. The latter's “claim” adopts a
session; it is not a file/task lease. Its optional adoption hint uses terminal
input, so that path cannot be treated as universally acceptable automatic
onboarding under WebMux's existing no-unsolicited-input requirement.

Rig's queue stores work on the target host and forwards cross-host requests.
That is useful durable federation, with uncertain/idempotent delivery handling;
it is a different authority model from MAC's shared task ledger. The installed
runtime explicitly returns `host_pin_remote_unsupported` for remote workflow
steps. The validator marks some declared workflow properties as unenforced.
These are version-specific boundaries, not a claim that Rig cannot send work to
another machine or could never replace a MAC capability.

MAC's `ControlPlane.claim_task` locks/rechecks the agent and task, capacity,
dependency state, project pause and machine policy in its transaction. This is
stronger than simply displaying a sorted queue. That inspected path does not
establish universal cross-task path exclusion or revision-bound source-roadmap
ordering. MAC's review/publication code checks evidence and reviewer eligibility;
project-native requirements and configured recovery/policy paths still need an
explicit compatibility audit. Do not infer the project's entire review policy
from a chapter heading or a signed evidence field.

WebMux's `coordinationProvider` exposes descriptor, status, launch and attach;
optional interfaces add lifecycle capabilities. Its standalone coordination CLI
adds durable messages, advisory claims and a fenced heavy-worker reservation.
Those are implemented features with scoped receipts, not a MAC adapter.
The current source search found no MAC task/lease integration in the server or
frontend. The prospective mapping in the adjacent design is not implementation.

A read-only snapshot of the selected MAC authority returned 44 agent records
and 18 project-summary rows. No project name or repository URL in that response
matched WebMux. Named fleet worker identities were present, while this coding
session has no `MAC_AGENT_ID`. This is evidence of a missing observed binding,
not proof of global non-registration or a fleet-health diagnosis: aliases,
visibility and other authorities would need reconciliation. A named persona,
physical host and individual Codex conversation must not be collapsed into one
worker identity. No agents were enrolled or heartbeat state changed.

The MAC hub's served UI is the observability console (`observe/`); `ide/` is an
unshipped local prototype under ADR 0025. Thus MAC having a browser UI does not
remove WebMux's interactive role. Equally, MAC CLI/API mutations already exist:
adding interactive access should reuse their authority rather than duplicate
an execution ledger in the browser or another server.

## Direct fit to the four questions

| Required answer | MAC | WebMux today | Agent Deck | Rig |
| --- | --- | --- | --- | --- |
| Local agent work | Registered workers/tasks/attempts; unregistered shells remain outside coverage. | Session/provider inventory and declared activity, with native observation limits. | Managed/adopted sessions and conductor observations. | Registered seats/occupants and owned packets/workflows. |
| Other-host project work | Shared ledger for connected workers/projects. | Authenticated peer context and retained component evidence; no MAC join. | Configured remote deck views and supervision. | Host-qualified queues/rosters/routing; target-owned records. |
| Duplicate/conflicting work | Same-task lease and claim admission; distinct-task semantic/path conflicts need explicit policy and scope enforcement. | Advisory overlap and a scoped heavy-worker fence; files remain unlocked. | Worktree isolation and supervisor policy; project-global exclusion is not established by the inspected contracts. | Packet ownership/workflow coordination; project-global cross-host write exclusion is not established by the inspected contracts. |
| Roadmap order | Dependency admission exists; source revision/priority policy still needs binding and enforcement. | No authoritative source-roadmap scheduling integration. | Conductor instructions can guide order; deterministic project-wide policy is not established here. | Workflow prerequisites/typed gates exist; source roadmap mapping and distributed execution/publication guarantees still need an adapter or replacement design. |

No column proves the complete goal today. In particular, strong local supervision
and remote messaging do not become cross-host project scheduling merely by
combining their inventories.

## Were the products necessary? Consider the counterfactuals

| Consolidation | What it can reasonably cover | What it loses or must build |
| --- | --- | --- |
| MAC + WebMux, with direct participant adapter | Best match for one project/fleet execution authority and interactive human/agent access. Reuses the cooperating fleet and project contracts. | Must implement session/task bindings, supported interactive channels, source-priority/scope admission gaps and shared visibility. Does not inherit every Agent Deck convenience or Rig team/continuity feature automatically. |
| MAC alone, with its console/CLI and ordinary SSH | Fleet task execution, registered-agent visibility and existing acceptance/publication machinery. | Does not provide WebMux's general browser terminal/desktop wall and shared session interaction. Sufficient if that interactive product requirement is dropped; that is not the stated goal. |
| Rig + WebMux, replacing MAC | Strong alternative for persistent interactive coding teams, owned packets, workflows and shared views. MAC is not inherently required for that narrower use case. | Must port/replace the actual fleet's placement, identity, operational controls, project bridge and exact publication/review contracts; reconcile distributed record ownership and remote workflow limitations. This is a control-plane replacement, not a thin adapter. |
| Agent Deck + WebMux, replacing MAC | Multi-harness sessions, remote attention and conductor-led supervision. Useful for operator-driven work. | Adds overlapping terminal surfaces while leaving the formal fleet/task/publication guarantees to a new layer. Prompted conductor policy is insufficient proof of the four project-wide goals. |
| All four, independently authoritative | Many features and preserved existing integrations. | Conflicting work ownership, duplicated project models, multiple supervisors/queues and difficult outage/recovery semantics. This is the current convergence risk, not a recommended default. |
| MAC + WebMux with optional Agent Deck or Rig executors | Preserves valuable existing session/team features without duplicating the authoritative task. | Requires a defined delegation boundary, receipt binding and one lifecycle owner per process; more compatibility/support work than the direct path. |

The integrations were reasonable ways to obtain managed coding sessions and
team ergonomics while the projects evolved independently. They were not
prerequisites for multi-host roadmap coordination. Conversely, MAC would not
have been necessary merely to build a local team/session manager. Under the
owner's present goals, replacing MAC would require reproducing the machinery
already organizing the working fleet; using it is the lower-disruption choice.
This is an architectural judgment, not a measured performance/cost benchmark.

## Recommended convergence and what to stop duplicating

Keep project requirements and delivery policy in their native repositories.
Use MAC as the authoritative project execution backend in connected mode.
Make WebMux a human/agent client of it, retaining its terminal/session authority.
Keep the existing built-in ledger only for explicitly standalone scope and
transport/session observations; do not grant parallel connected-mode task leases.

Preserve Rig/Agent Deck as optional execution/lifecycle adapters, chosen for a
specific benefit. Do not require both for ordinary work or place one worker
under two launch/recovery supervisors. If Rig runs a delegated team, define
whether MAC owns one enclosing attempt or explicit child attempts. Bind Rig
packets/outcomes to that attempt; a Rig packet closure cannot independently mark
its source project issue complete. Give any conductor the same boundary.
Use existing native inbox/channel support and preserve permission policy.

Reuse rather than rebuild useful components: MAC's project graph and authority
API; WebMux's accounts, sessions, execution-host context and private agent
channels; Rig's seat/continuity/context features where used; Agent Deck's
harness/profile/attention and productivity tools where wanted. Shared worker
reservations must have one authority for each actual resource, including when
some clients use the standalone product. Local and global grants for the same
physical worker cannot both claim exclusivity.

For the four questions, the first delivered integration should join MAC's actual
project/task/attempt/lease state with WebMux's local/remote session observations,
show source requirement/revision and dependency reasons, and label missing
bindings or stale readers. Humans and agents must read that same projection.
The next step is admission through MAC for independently launched interactive
sessions, followed by authority-side scope/priority enforcement and project-native
publication integration. Reading state must not silently enroll or dispatch work.

A shared NAS checkout is not the coordination protocol. Even healthy storage
would not resolve competing task/lease authorities or two sessions editing one
worktree. Keep per-owner workspaces and canonical publication controls. The
current failed NAS integrity gate independently prohibits unique-data cutover
and deleting local originals.

## Acceptance before consolidation is called complete

Use real participating agents on three authorized hosts and both a human and an
agent reader. Require identical project/task/attempt identities and current work;
refuse duplicate task acquisition; detect/refuse or explicitly negotiate
conflicting declared scopes across different tasks; honor source dependency and
priority changes; and refuse publication by a stale/lost attempt. Include
project-native tests/review/branch/release gates, outage/uncertain retry, foreign
accounts, reconnect/restart, unmanaged sessions and preservation of the same
native conversations. Demonstrate any delegated Rig/conductor path separately.

No system can prevent arbitrary unregistered software from editing local files.
Participation, scope admission and publication enforcement define the guarantee;
unregistered or unsupported writers must remain visible as a coverage gap.
Current inventory, component tests and provider tool catalogs do not prove this
complete result. Do not remove functioning integrations before their required
capabilities and live continuity have replacement evidence.

## Sources and limits

- MAC local revision `99925eaff54076310b58f15e0fb4ba9079746673`: `src/mac/services.py`
  (`claim_task`), `src/mac/review_service.py`, project/dependency/integration
  contracts, book chapters 1/3/4/5/6/8/11/16 and ADRs 0025/0034.
- WebMux base `86911d0828ca35f27bd4f108ad6ee6ca26265970`: current provider adapter,
  session coordination and recovery documentation, roadmap and retained native
  receipts. The earlier [MCP comparison](validation/coordination-architecture-selection-2026-11-22/README.md)
  qualifies only its stated interface probes.
- Agent Deck [installed-revision conductor](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/docs/conductor/README.md),
  [watchers](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/docs/WATCHER-SETUP.md)
  and [remote CLI contract](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/docs/remote-cli-contract.md),
  plus native version/help. Current upstream architecture was consulted separately;
  it is not installed-version acceptance.
- Rig [installed-revision coordination](https://github.com/mvschwarz/openrig/blob/9d0200f39c1e1e62ac98e6995744cf8efb301a30/docs/as-built/architecture/coordination-primitive.md)
  and [workflow runtime](https://github.com/mvschwarz/openrig/blob/9d0200f39c1e1e62ac98e6995744cf8efb301a30/docs/as-built/architecture/workflow-runtime.md),
  checked against the installed implementation. Those documents retain their own
  earlier source-verification pin; installed-code checks above cover the specific
  claims made here, not every sentence in those documents.
- Literate AI `.mac/project.yaml` at local revision
  `ebf88985be10f733dd64f469b7cd4aa685ce9083` is a build/test/evidence contract,
  not the interactive roadmap/task bridge.

The adjacent [source manifest](validation/mac-webmux-integration-2026-11-22/sources.json)
records hashes of inspected inputs and native version results. This was a
read-only product/source/interface investigation, with limited selected-authority
reads. No new fleet lifecycle, cross-host contention, policy migration or live
publication journey was run. Unsupported capability is distinguished from an
unqualified integration; absence was not inferred from a missing MCP tool.
