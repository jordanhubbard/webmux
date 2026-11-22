# Automatic session coordination architecture

Decision completed 2026-11-22 for the architecture-evaluation item in
[#630](https://github.com/jordanhubbard/webmux/issues/630), with review tracked by
[#656](https://github.com/jordanhubbard/webmux/issues/656).

WebMux's Go server owns the node/account/session coordination plane. Retain
Agent Deck and Rig as optional lifecycle owners for their existing managed
sessions. Reuse the already-pinned Pion DNS-SD library for node discovery and the
existing authenticated HTTPS enrollment/directory for peer authorization. The
[source-bound comparison](validation/coordination-architecture-selection-2026-11-22/README.md)
completes architecture selection. Automatic managed/existing-agent onboarding,
complete discovery and full workflow acceptance remain implementation gates.

## What was inspected and tested

The current installed binaries report Agent Deck 1.16.34 and Rig 0.6.14
(`9d0200f3`). Their native MCP processes were tested in empty private HOME/XDG
profiles alongside a current-source WebMux executable built from `3cacedc`.
Each completed initialize, tool discovery and a controlled unavailable read.
Agent Deck's catalog used only a fixture-owned, non-executable MCP definition;
its disabled-Recall control exited with the expected refusal. All evidence is
bound to executable/source hashes and exact protocol responses in the receipt.

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
| Agent Deck | Existing profiles, managed-session inventory, attach/ownership and native recovery adapters; configurable MCP catalog. The installed Recall server advertises three search/show/context tools. | Its profile/configuration and conversation-index scopes require explicit mapping to WebMux accounts and verified projects. Recall's three tools do not implement WebMux claims, acknowledged inbox receipts or worker fences. Its configuration/restart path cannot establish automatic awareness while preserving existing agents. Requiring it on every node adds another CLI, tmux/profile management and native configuration compatibility. |
| Rig | Existing topology, managed launch/attach, owner queue and recovery adapters. The installed MCP server advertises 23 tools, including five queue operations and manager lifecycle/terminal operations. | Rig seat/daemon identity needs an adapter to WebMux node/account/session identity and project authority. Its queue is useful provider-owned work state; the inspected MCP surface does not expose WebMux's fenced worker-resource contract. Its daemon, Node/native dependencies, tmux and version-specific launch wrappers add deployment and upgrade work. Keep lifecycle operations with their owner. |
| Built-in WebMux plane | Existing WebMux identities, private session capabilities, durable ledger, separately authorized peer context/messages/claims/reservations, authenticated execution-host project observations and scoped CLI/MCP. The current MCP server exposes two coordination tools and refuses reads without private access. | Ships with the Go server/helper, avoiding a mandatory manager on otherwise ordinary nodes. WebMux owns schema, transport, revocation, retention and resource-bound maintenance. Runtime-specific native policies, sandbox reachability, managed-provider channels and platform custody still require dedicated adapters and acceptance. |

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
The later [native Bonjour adapter qualification](validation/discovery-bonjour-2026-11-22/README.md)
adds bidirectional macOS/Linux node and exact legacy-instance component evidence.
It retains the failed raw self-loop and host-only listener-selection controls;
it does not complete discovered/enrolled native-agent acceptance.

## Ownership and migration rules

The built-in plane owns coordination records and scoped authority. Existing
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

The remaining roadmap items require complete discovered/enrolled two-host
acceptance, automatic ordinary and managed participation, verified or explicitly
mapped projects, and full native stale-identity, revocation, restart/reconnect and
uncertain-delivery controls. Durable messages, advisory claims and shared worker
reservations must be observed through actual native calls and authoritative
receipts throughout that workflow. Native sandbox/permission boundaries remain
authoritative. Retention/scale and all stated transport/platform gaps remain
tracked under #630/#631. Completing architecture selection closes none of those
implementation or qualification requirements.
