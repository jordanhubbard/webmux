# Default coordination architecture decision

Design and implementation work for [#630](https://github.com/jordanhubbard/webmux/issues/630)
and [#631](https://github.com/jordanhubbard/webmux/issues/631). This decision selects
a direction; full native agent/interface acceptance remains open.

## Observed responsibilities

The current server adapter boundary is
[`coordinationProvider`](../webmux/server/internal/httpapi/coordination_providers.go).
It exposes descriptors, status, launch and attach; optional interfaces extend
specific provider capabilities. Inventory and terminal-input messaging are not
a durable coordination protocol. The existing
[federation contract](server-federation.md) grants one account's terminal access,
not coordinator or configuration access. It deliberately refuses arbitrary
proxy paths and redirects. Neither contract establishes automatic project
participation, durable claims or acknowledged handoffs.

| Candidate | Demonstrated contribution | Missing requirement and deployment cost |
| --- | --- | --- |
| Agent Deck | Existing managed-session profiles, inventories, attach and ownership checks; remote manager forwarding and recovery have scoped native evidence in [session recovery](session-recovery.md). | Enrollment is provider-specific. Its inventory does not cover every ordinary WebMux shell or independent agent. Adding it as mandatory would add its CLI/tmux and profile management to otherwise standalone nodes. Remote ownership/resume support has version-specific limits. |
| Rig | Existing canonical seat/project inventory, structured launch/attach, version-qualified native creation, checked Stop and event subscriptions. [Native routing evidence](validation/rig-routing-current-2026-11-19/README.md) distinguishes modern from legacy workloads. | Native agent policy, platform and lifecycle acceptance remains scoped. Its owner queue is not a universal automatic WebMux session inbox. The server adapter materializes a Node runner plus native-launch helpers; tmux, a qualified Rig build and runtime-specific behavior remain operational dependencies. |
| Built-in WebMux plane | WebMux already owns account/session identities, durable storage, terminal creation, server startup/shutdown and an authenticated peer allowlist. The new Go DNS-SD browser has deterministic/race controls and a real Linux ARM64 multicast round trip using the already-pinned Pion dependency. | Authenticated HTTPS node/account identity and address migration now have a native two-host component result. A durable node-local project/claim/handoff/reservation ledger and scoped ordinary-session CLI now have controlled process and protocol tests in the [coordination checkpoint](session-coordination.md). Native agent-aware channels and cross-host coordination still need implementation and acceptance. Built-in discovery alone supplies none of these agent semantics. |

Code inspected: provider adapters above,
[`coordination_service.go`](../webmux/server/internal/httpapi/coordination_service.go),
[`federation.go`](../webmux/server/internal/httpapi/federation.go),
[`openrigcoord/coord.go`](../webmux/server/internal/openrigcoord/coord.go), and the
installed Pion v2.2.2 service/browse interfaces. Existing provider receipts retain
their own versions and source bindings; they are not new current-head native
qualification. Current full-suite failures and multicast boundaries are recorded
in [network discovery](network-discovery.md).

## Selection and ownership

Implement universal node/session coordination in WebMux's Go server. Reuse the
existing Pion DNS-SD library rather than adding another discovery daemon or a
mandatory agent manager. Retain Rig and Agent Deck as optional lifecycle adapters
for their existing managed sessions. Do not remove either integration or migrate
its provider processes as part of discovery rollout. The built-in plane owns
coordination records; it does not become a second owner of provider processes.

This choice makes the default plane available to ordinary sessions on Windows,
Linux and macOS without requiring a provider-specific launch form. That is the
implementation target, not a claim of present agent awareness. Rig hosting,
Agent Deck hosting, sandbox network access and each native agent channel retain
their own platform support/qualification limits. Node discovery currently has
Linux ARM64 multicast component acceptance, a two-host macOS/Linux identity and
address-migration component result, failed macOS multicast evidence, and no new
Windows or complete two-host discovery/agent acceptance.

Existing managed identities must map to their original WebMux account/session
and provider identity; registration must not launch, stop, adopt or duplicate a
provider. Legacy records remain readable. A future coordination protocol must
provide server/account/session-scoped capabilities, verified repository identity
or explicit mappings, durable expiring claims and exclusive worker reservations.
It must distinguish admission, delivery, acknowledgment and completion with
stable request IDs and refuse blind retries after uncertain delivery. Keep it
active in the server independently of the UI. Do not deliver automatic messages
by typing into terminals. Provider-native inbox/MCP or other supported agent
channels need separate interface tests, including sandbox reachability and
independently launched ordinary agents; they are not established by this decision.

## Gates still required

1. Qualify the implemented authenticated stable HTTPS node/account identity and
   address reconciliation across the full discovered two-host/platform workflow.
   DNS-SD claims never authorize access; the native protocol component result
   does not complete discovery, deployment or agent acceptance.
2. Extend the [durable node-local coordination checkpoint](session-coordination.md)
   through cross-node authority, execution-host provisioning and full native
   project isolation, stale identity and restart/reconnect acceptance.
3. Inspect and test real agent-facing interfaces across ordinary and managed
   launches while preserving original policies, credentials and lifecycle owners.
4. Qualify two independently started real agents on two real authorized WebMux
   hosts: automatic project discovery, overlap notification, acknowledged handoff
   and shared heavy-worker reservation, plus all negative controls in #630/#631.

Leave the roadmap architecture/coordination checkboxes and both issues open
until their complete evidence requirements pass.
