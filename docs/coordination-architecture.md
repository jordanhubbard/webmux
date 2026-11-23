# Automatic session coordination architecture

Provider-interface comparison completed 2026-11-22 under
[#656](https://github.com/jordanhubbard/webmux/issues/656). The proposed project-authority
amendment below responds to the design direction clarified in
[#664](https://github.com/jordanhubbard/webmux/issues/664); its adapters and full
native acceptance remain unfinished.

**Each project owns its roadmap and delivery method. MAC is the shared execution
authority for MAC-connected projects. WebMux is the interactive human and agent
surface onto that worker pool.** The built-in WebMux ledger remains a bounded
standalone backend and a source of transport/session observations, rather than a
second task/lease authority for a MAC-connected project.

WebMux retains node/account/session identity, terminal presentation, authenticated
peer access and DNS-SD discovery. Agent Deck, Rig and existing fleet workers retain
their lifecycle owners. The
[source-bound provider comparison](validation/coordination-architecture-selection-2026-11-22/README.md)
remains valid within its tested scope; it did not compare MAC or establish shared
project priority, admission or publication authority. The
[project coordination contract](mac-project-coordination.md) defines the authority
mapping, delivery order, current gaps and three-host acceptance requirements.

The deeper [four-product investigation](coordination-integration-investigation.md)
revisits necessity, overlap and alternatives; it is the basis for this proposed
amendment. It gives Rig workflow semantics and Agent Deck conductors their full
scope, beyond the earlier MCP catalog comparison.

## What was inspected and tested

At the pinned comparison checkpoint, the installed binaries reported Agent Deck 1.16.34 and Rig 0.6.14
(`9d0200f3`). Their native MCP processes were tested in empty private HOME/XDG
profiles alongside a prototype WebMux executable built from `3cacedc`.
That prototype comparison does not qualify the runtime on this branch.
Each completed initialize, tool discovery and a controlled unavailable read.
Agent Deck's catalog used only a fixture-owned, non-executable MCP definition;
its disabled-Recall control exited with the expected refusal. All evidence is
bound to executable/source hashes and exact protocol responses in the receipt.
The final repeatable qualifier also preserves failed diagnostics, refuses output
reuse and refuses optimized Python before producing output. Its historical
receipt remains pinned to the actual inputs; publishing it does not turn those
inputs into a fresh execution of this branch or of every provider capability.

Agent Deck's Go build records revision
`f36e77d6170b8d8b54f50c960c3f16e8a2c5df19`, matching the inspected release source.
The [MCP command](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/cmd/agent-deck/mcp_cmd.go)
and [native configuration adapter](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/internal/session/instance_mcp.go)
write project/native configuration; Codex uses its global profile configuration.
Immediate loading through the command's restart option can restart the agent and
send continuation input through its terminal. That path is excluded from
WebMux's automatic onboarding design. The separate
[Recall MCP implementation](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/internal/recall/mcp/mcp.go)
provides conversation search, detail and context retrieval.

The installed Rig MCP entrypoint and tool implementation were inspected directly
and hashed. Its explicit daemon-port option allowed a probe against an owned
refusing endpoint without starting a daemon. Tool discovery made no backend
requests. The controlled status read produced the only HTTP request, a GET to
that fixture's `/healthz`, and returned an MCP error. No lifecycle, terminal-send,
queue mutation, live conversation or existing provider configuration was accessed.

## Comparison and costs

| Candidate | Demonstrated contribution | Fit and deployment/maintenance costs |
| --- | --- | --- |
| Agent Deck | Profiles, managed sessions, native recovery, remote deck management, conductors/watchers, worktree helpers and MCP/skill tooling. The separate Recall server exposes search/show/context. | Its profile/configuration and conversation-index scopes require explicit mapping to WebMux accounts and verified projects. Recall's three tools do not implement WebMux claims, acknowledged inbox receipts or worker fences. Its configuration/restart path cannot establish automatic awareness while preserving existing agents. Requiring it on every node adds another CLI, tmux/profile management and native configuration compatibility. |
| Rig | Topology, managed launch/attach and continuity, durable owned work/handoffs, workflow dependency graphs and typed receipts, cross-host routing/rosters and human routes. The earlier native MCP probe exposes only part of that product. | Rig seat/daemon identity needs an adapter to WebMux node/account/session identity and project authority. Its host-owned queue/workflow model overlaps MAC and needs an explicit delegation/authority boundary. Installed remote-pinned workflow execution is refused; no global project write-scope/resource guarantee is established by its MCP catalog. Its daemon, Node/native dependencies, tmux and version-specific launch wrappers add deployment and upgrade work. Keep lifecycle operations with their owner. |
| Built-in WebMux plane | Existing WebMux identities, private session capabilities, durable ledger, separately authorized peer context/messages/claims/reservations, authenticated execution-host project observations and scoped CLI/MCP. The pinned native comparison exposes two coordination tools and refuses reads without private access. | Ships with the Go server/helper, avoiding a mandatory manager on otherwise ordinary nodes. WebMux owns schema, transport, revocation, retention and resource-bound maintenance. Runtime-specific native policies, sandbox reachability, managed-provider channels and platform custody still require dedicated adapters and acceptance. |
| MAC execution authority | Project/task ledger, canonical machine/worker identities, task-attempt leases, dependencies, evidence/review/publication and integration observations/findings. | Reuse the existing fleet for connected projects. The interactive-session adapter, source-roadmap mapping and strict project priority/write-scope admission still require implementation and qualification. MAC dispatch classes are scheduling hints, not proof of roadmap ordering or arbitrary file exclusivity. |

The existing adapters inspected include
[`coordination_providers.go`](../webmux/server/internal/httpapi/coordination_providers.go),
[`coordination_service.go`](../webmux/server/internal/httpapi/coordination_service.go),
[`openrigcoord`](../webmux/server/internal/openrigcoord/coord.go),
[`federation.go`](../webmux/server/internal/httpapi/federation.go) and
[`session_coordination.go`](../webmux/server/internal/httpapi/session_coordination.go).
Their tests and retained native receipts establish specific provider behaviors;
MCP tool availability alone establishes no new native workflow acceptance.

## Runtime and platform boundaries

| Surface | Evidence and supported boundary |
| --- | --- |
| Installed provider interface probes | Native macOS ARM64 processes: Agent Deck 1.16.34 catalog/Recall and Rig 0.6.14 MCP. The qualifier explicitly requires POSIX HOME/XDG isolation. These probes do not qualify managed launches or Windows profile custody. |
| Agent Deck lifecycle | The separate [native lifecycle result](validation/agent-deck-2026-11-17/README.md) covers its stated Linux guest/Agent Deck 1.16.39/Claude 2.1.322 ownership and reboot cases. That build is distinct from the installed binary inspected here. |
| Rig lifecycle | [Native routing](validation/rig-routing-current-2026-11-19/README.md) and [session recovery](session-recovery.md) retain their exact manager/runtime versions, native launch policies and host boundaries. Their results do not qualify all 23 MCP tools or automatic WebMux coordination adoption. |
| Built-in ordinary native channels | [Ordinary native startup](validation/agent-coordination-2026-11-21/README.md), [two-host agent workflow](validation/coordination-native-pair-2026-11-22/README.md), [SSH adoption](validation/coordination-ssh-agent-integration-2026-11-22/README.md), [reconnect](validation/coordination-ssh-reconnect-integration-2026-11-22/README.md) and [production-controller restart](validation/coordination-controller-restart-integration-2026-11-22/README.md) provide source-bound macOS/Linux component results. Exact native CLI versions and explicit tool-approval boundaries remain part of each receipt. |
| Remaining transport/platform acceptance | Managed/existing agents, password SSH, native Mosh/Windows, native Windows access-file custody and the complete discovered/enrolled real-host lifecycle remain open. Portable Go implementation, compilation and platform CI each retain their own scope. |

Discovery has Linux ARM64 multicast component evidence, authenticated macOS/Linux
identity/address-migration evidence and unresolved native macOS multicast failures.
The [transport diagnostics](validation/discovery-transport-2026-11-22/README.md)
retain those failures. Manual authenticated peers remain available when multicast
is unavailable; a working SSH route does not complete LAN discovery acceptance.
The later [native Bonjour adapter qualification](https://github.com/jordanhubbard/webmux/blob/22dd1965b8bc88ece69ed8e6f15360aa0be9b5a9/docs/validation/discovery-bonjour-2026-11-22/README.md)
adds bidirectional macOS/Linux node and exact legacy-instance component evidence.
It retains the failed raw self-loop and host-only listener-selection controls;
it does not complete discovered/enrolled native-agent acceptance.

## Ownership and migration rules

For a MAC-connected project, task admission, leases and execution mutations go
through MAC; WebMux stores scoped bindings, observations and receipts. It must
not grant a competing local lease or silently fall back on hub loss. Standalone
projects explicitly retain the built-in backend and its documented limits. Existing
Agent Deck/Rig agents retain their lifecycle owner, native policy, credentials,
conversation identity and provider state. The architecture comparison performed
no migration, restart, adoption, message send or global native configuration
rewrite. Optional provider enablement and launch/view preferences remain separate
from the [server-wide coordination policy](session-coordination.md).

An adapter must bind a managed worker to its original account and stable provider
identity independently of attachment viewers. It must use a supported native
agent channel, preserve permission policy and avoid duplicate providers. A
provider inventory row, configured MCP server or available socket is insufficient
proof of agent awareness. Existing agents without a supported channel remain an
explicit onboarding gap; automatic messages must not be typed into arbitrary
terminal prompts. Future migrations require source-bound lifecycle and authority
controls before changing existing agents.

## Acceptance work after this decision

The project-authority amendment adds the ordered #664 gates in the
[convergence contract](mac-project-coordination.md#delivery-order-and-acceptance).
Read-only visibility precedes interactive admission. Source priority/dependency
changes and stale-lease publication refusal must be enforced by the execution
and project-native authorities, not inferred from a WebMux inventory. Three
real hosts must share the same project/task/attempt identities. Existing
#630/#631 component receipts do not prove these additional requirements.


The remaining roadmap items require complete discovered/enrolled two-host
acceptance, automatic ordinary and managed participation, verified or explicitly
mapped projects, and full native stale-identity, revocation, restart/reconnect and
uncertain-delivery controls. Durable messages, advisory claims and shared worker
reservations must be observed through actual native calls and authoritative
receipts throughout that workflow. Native sandbox/permission boundaries remain
authoritative. Retention/scale and all stated transport/platform gaps remain
tracked under #664/#631, retaining #630's historical requirements. Completing architecture selection closes none of those
implementation or qualification requirements.
