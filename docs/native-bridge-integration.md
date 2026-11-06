# Native bridge integration research

- **Status:** research complete; connector implementation and qualification pending.
- **Owner:** [#275](https://github.com/jordanhubbard/webmux/issues/275).
- **Reviewed source:** [v1.4.2 main, `94e3861`](https://github.com/jordanhubbard/webmux/tree/94e3861e126937bc7807a506fba141cb5323f666).
- **Requested consumer:** a first-class WebMux adapter in the Codex iOS host bridge.

WebMux supplies useful terminal and coordinator APIs today. It does not currently
supply a Codex iOS connector or a runtime loader for arbitrary integration plugins.
The bridge would be a client of WebMux, with its own reviewed adapter and consent
boundary. Adding a new provider *inside* WebMux is a separate design choice.

## Available contracts

| Surface | Current behavior | Boundary for a bridge |
| --- | --- | --- |
| `GET /api/sessions` | Lists the authenticated owner's persisted sessions. | A terminal is not a Codex conversation, goal, job or readiness receipt. |
| `GET /api/coordination` | Returns `webmux-coordination/v1`, provider descriptors, projects, seats, messages, errors and attention counts. Includes native terminals and independently enabled Agent Deck/Rig inventories. | Authenticated mode requires an administrator. Check enabled/available capabilities and per-provider failures; do not replace failures with an empty or healthy inventory. |
| `POST /api/coordination/attach` | Opens an owned native terminal or a provider-qualified viewer. | Reopening a viewer and restarting a stopped provider are distinct operations. Do not launch merely to observe state. |
| `POST /api/coordination/agents` and `/workspaces` | Explicit provider launch and execution-host workspace browsing. | These are privileged actions, not discovery prerequisites. A workspace is a registered association, not observed current working directory. |
| `POST /api/coordination/messages`, `/send-status`, `/drain` | Provider-specific structured messaging where its adapter implements the corresponding capability. Agent Deck advertises queue/status/completion-inbox/drain capabilities. | Preserve the actual provider result. Native WebMux and Rig do not thereby acquire Agent Deck's completion semantics. |
| `GET /api/federation/sessions` | A configured incoming bearer grant selects one existing local account and lists that owner's sessions. | A federation credential does not authenticate coordinator, account, configuration, key or desktop APIs. |
| Federation session creation, reconnect, messages and terminal WebSocket | Fixed allowlisted terminal operations under that owner; no arbitrary-path proxying or redirect following. | The grant allows terminal interaction and restart/creation within its owner scope. It is not a read-only or per-capability grant. |
| Connected-server directory and proxy | Administrator-controlled origin allowlist, separate peer credentials and server-scoped session identity. | No mesh discovery, copied credentials, shared home, synchronized checkout or automatic remote-Rig state is implied. |

The route and adapter authority is in
[`server.go`](../webmux/server/internal/httpapi/server.go),
[`openrig.go`](../webmux/server/internal/httpapi/openrig.go),
[`coordination_providers.go`](../webmux/server/internal/httpapi/coordination_providers.go)
and [`federation.go`](../webmux/server/internal/httpapi/federation.go).
[Coordination setup](openrig.md) and [federation setup](server-federation.md)
remain the operator-facing configuration contracts.

## What flexible integration means here

The internal Go `coordinationProvider` interface defines descriptor, status,
launch and attach methods, with separate optional messaging/drain interfaces.
The server currently constructs Rig and Agent Deck adapters directly. Configuration
accepts only `agent-deck` and `rig` as external enabled-provider IDs; native WebMux
is always available. Rig and Agent Deck retain separate lifecycle owners even
when both integrations are enabled.

Only Agent Deck currently implements the HTTP messaging/status/drain interfaces.
Rig's `peer-messaging` descriptor refers to its coordinator capability, not a
uniform delivery or completion receipt through those WebMux routes.

A third-party URL, module, executable or manifest cannot register an arbitrary
provider through the current API. A future Codex iOS bridge plugin can consume
existing WebMux APIs without adding a dynamic code loader or making the bridge
another lifecycle owner for an existing provider.

## Identity and action semantics

Keep the owning server, account, provider and returned session/seat ID together.
Treat provider IDs as opaque. An editable title, project label, terminal output,
registered workspace or current directory is not authoritative conversation
identity. Native project `runningCount` counts connected terminal transports; it
does not prove that coding agents are executing, ready, idle or completed.
Provider activity and attention fields remain observations, not goal authority.

Native/federated terminal messaging writes bracketed-paste input followed by Enter.
It can become a shell command or an answer to a different prompt if the target is
wrong. The native response is `202`, `queued: true`, `processed: false`: admission
to terminal input, not task execution or completion. Messages are not idempotent;
there is no durable native inbox or processing receipt. After ambiguous delivery,
retain uncertainty and require a fresh explicit decision rather than automatic
retry. Structured provider send IDs/status must retain that provider's meaning.

The bridge must not infer structured Codex transcript, approvals, goal controls,
turn identity or completion events from a terminal or from a provider's presence.
Those remain capabilities of the actual runtime/provider, not WebMux terminal
transport. Shared-agent identity/event bridging is separately tracked by
[#246](https://github.com/jordanhubbard/webmux/issues/246).

## Authorization choices still to design

The ordinary coordination APIs use an existing account JWT and, in authenticated
mode, administrator authorization. Trusted `auth.mode: none` permits anonymous
host control and is not a proposed authentication strategy for a phone bridge.
Federation uses a private, configured incoming grant and existing account owner.
It is narrower in API reach but still broad terminal-control authority for that
owner. Neither surface currently supplies adapter-specific consent, read-only
coordination access, capability-scoped grants or per-action approval authority.

Before storing or using a WebMux credential, choose and review a bounded model:

1. Select an explicit WebMux origin, expected server/owner and contract. Use
   verified HTTPS, or a separately reviewed private loopback/tunnel boundary;
   preserve WebMux TLS verification and the phone bridge's existing pairing and
   App Attest protections. Never copy phone credentials into WebMux or expose
   WebMux service credentials to the phone/browser/transcript.
2. Decide whether the first adapter is read-only. Document exactly which existing
   credential would authorize it and whether its broader authority is acceptable.
   If least-privilege consent needs new WebMux grant scopes, implement and test
   those scopes upstream before declaring the adapter available.
3. Make observation, terminal attachment, terminal input, provider launch/restart
   and structured runtime operations independently visible capabilities. Default
   to observation; require fresh target identity and explicit action consent.
4. Define server/contract compatibility and capability negotiation. Coordination
   reports a contract string; the health endpoint alone is not a version or
   capability handshake. Do not assume every installed release has this surface.
5. Define credential revocation, adapter disablement, stale snapshot rejection,
   cancellation, bounded responses and unavailable/partial observation. Disabling
   the adapter must not stop an unrelated terminal or coding agent.

The research found no need to change current runtime behavior merely to document
this boundary. Any scoped-grant or handshake change is a separate implementation
with its own public issue, source review and qualification. Do not bypass the
current guards to make a prototype appear connected.

## Qualification before connector availability

Current tests already cover important reusable contracts:

- `TestEnabledIntegrationsDiscoveredTogetherWithIndependentFailures` and
  `TestDisabledCoordinationDoesNotDiscoverMachineWideProviders` preserve
  independent integration failure and explicit enablement.
- `TestNativeCoordinationInventoryReopensOwnedViewerWithoutLaunching` and
  `TestAgentDeckProviderDiscoversHostQualifiedNativeSessions` cover ownership,
  provider-qualified identities and viewer reuse.
- `TestOpenRigRequiresAdministratorAndReportsMissingInstallation` covers the coordination authorization
  boundary; the federation suite covers owner isolation, TLS/redirect refusal,
  missing/bad credentials, revocation and native terminal transport.
- Provider tests use controlled CLI stand-ins except separately gated real-provider
  qualification. Their success does not qualify an installed phone bridge.

The reviewed source's targeted Go race suite passed **22 tests**, with zero
failures and one explicit opt-in real-Agent-Deck qualification skip, on macOS
ARM64. The command was `GOMAXPROCS=2 go test -p 2 -race -json
./internal/httpapi ./internal/config -run 'Test.*(Coordination|Federation|AgentDeck|OpenRig)'
-count=1` from `webmux/server`. Relative source links and named test functions
were checked. This verifies existing controlled contracts, not an installed
connector, physical iPhone or newly admitted live-provider session.

For the future connector, retain exact WebMux and bridge source/runtime identities
and prove:

1. Allowed observation versus denied write/foreign-owner/provider actions using
   the chosen consent model, including real credential revocation.
2. Fresh immutable target selection, reconnect without duplicate providers, stale
   or changed identity rejection, bounded response parsing and partial failures.
3. Non-idempotent/uncertain terminal sends with no automatic replay, plus distinct
   queued, processed and completed states where the actual provider supplies them.
4. iPhone/bridge/WebMux integration with the existing pairing, device-owner lock,
   foreground/background, transport loss and reconnect controls intact. Use owned
   disposable sessions; never test mutations in a person's active conversation.
5. Any upstream scope/handshake changes against negative and mutation controls,
   independently reviewed before activation.

Do not reuse the old Host sessions client activation as current support. That
client was removed in WebMux PR #163; the newer coordination/federation surface
above is a different contract. Connector implementation and physical-device
acceptance remain pending in the Codex iOS roadmap.
