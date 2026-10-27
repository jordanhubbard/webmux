# Agent coordination

WebMux exposes coordinator-managed coding sessions behind the ordinary
**Terminals** grid. The **Coordination** button opens a project and session
browser backed by a versioned provider contract. Agent Deck is the recommended
session conductor; Rig remains an optional topology-centric provider. Creating
a session opens its interactive CLI in a normal terminal
tile. The raw Rig text dashboard remains available under **Advanced
diagnostics**.

New installations open Coordination in Agent Deck by default. Select
**Settings → Coordination** to make Agent Deck, Rig, or Disabled the persistent
experience. Upgraded installations that predate this setting ask once instead
of silently changing an existing Rig workflow. The Coordination header shows
the active provider; **Change** browses another provider for the current visit,
and **Make default** persists that choice. A configured provider that is not
installed remains selected with its installation guidance—WebMux never falls
back silently. Sessions remain owned by the provider that created them and are
available whenever that provider is selected.

The browser deliberately keeps three concepts separate: a coordinator project
(a Rig topology or Agent Deck group), a coordinator session/seat, and a
filesystem workspace on an execution host. Canonical provider discovery is the
source of truth. WebMux's private launch records enrich that inventory but do
not create a second, stale project list. Running, stopped, and historical Rig
topologies remain visible with their provider IDs.

## Projects and worker identity

The Coordination modal groups workers under expandable project headings. Each
worker shows its registered name (such as coder or reviewer), execution host,
runtime and observed state. Expand or collapse a project with its summary button
or Enter/Space; **Open session** attaches an existing viewer and **Restart
provider and open** is reserved for a stopped managed Rig seat.
A worker name is the registered name, not an inferred permission or capability.

Opening a managed terminal records its coordinator target, project, name, host
and runtime with that tile. The **Rig** badge shows project, worker and execution
host separately from the terminal's editable title and transport badge. For
remote workers the viewer still attaches locally to an SSH relay: its EXEC
transport is not a claim that the provider executes locally. Existing tiles gain
this metadata when opened again from Coordination. Renaming an ordinary terminal
does not make it Rig-managed. Tile identity is retained registration metadata,
not a live activity or verified-process indicator; consult Coordination for state.

## One coordinator, multiple execution hosts

The WebMux service supervises an optional, loopback-only OpenRig daemon. The
coordinator owns the common project/team state and messaging. Each managed agent
has a real tmux attachment on the coordinator host. For a remote agent that
attachment runs SSH to a persistent tmux session on the execution host. A
loopback-only SSH reverse forward gives its Rig client access to the central
API. Messages travel through Rig's ordinary tmux delivery path and the same SSH
terminal the owner can interact with. No remote Rig daemon is required.

Provider processes, credentials and project files remain on their execution
hosts. A shared project ID groups agents centrally; a project's directory is
mapped separately on each host. Git checkout synchronization is an independent
operation. Remote hosts use the service user's OpenSSH configuration and normal
host-key verification. WebMux's existing SSH connections and tmux sessions keep
their existing behavior.

## Agent Deck provider

Source `make install-server` and `make install-all` install WebMux's
qualified Agent Deck release by default. Homebrew installs Agent Deck as a
dependency. Use `make install-agent-deck` on a macOS/Linux execution host
that does not run the server. The installer is idempotent, pins the qualified
version, and uses Agent Deck's checksum-verifying upstream installer. Set
`INSTALL_AGENT_DECK=no` only for an intentional Rig-only or
coordination-disabled server.

When `agent-deck` is on the WebMux service PATH, Coordination also inventories
the current local Agent Deck profile and every remote registered with that
Agent Deck instance. Session IDs are qualified by location (`local:<id>` or
`<remote>:<id>`), so identical names on different hosts do not collide. The
browser shows the native Codex or Claude session ID when Agent Deck reports it,
and uses Agent Deck's own local or remote attach transport when opening a tile.

Creating a session in an Agent Deck group routes `launch` to the selected local
instance or registered remote and starts Codex or Claude in the selected
workspace. Remote workspace browsing resolves the registered remote to its SSH
host while retaining the Agent Deck remote name as the coordinator identity.
Authenticate Agent Deck and the chosen coding CLI on every host that owns
sessions. Installation cannot copy credentials or register remotes. WebMux does
not scan arbitrary shells: sessions must be known to Rig or Agent Deck before
they appear.

Agent Deck provides host/session inventory and lifecycle, including remote
sessions and native harness IDs. WebMux also uses Agent Deck's native conductor
relationships, guarded send queue, delivery status, completion inbox, and remote
drain commands. Instructions are supplied on standard input, never interpolated
into a command line. A local conductor can collect durable transition and
completion records from registered remotes without consuming the remote copy;
source-host attribution stays attached to every imported record. The toolbar
badge and project/session rows surface waiting, completed, and failed work that
needs attention.

Rig provides canonical topology and peer messaging. The providers are
alternative experiences rather than one being silently
treated as the other's project database. Additional coordinators can implement
the same internal status, launch, attach, messaging, delivery-status, and drain
adapters without changing the browser wire contract (`webmux-coordination/v1`).

## Remote identity verification follow-up

Cross-host message delivery and reconnect are qualified separately from native
process identity. OpenRig 0.6.14 sees the central SSH relay as the foreground
process of a remote Codex seat. Its attention reconciler can therefore report
`process_identity_ambiguous` even after a successful message exchange, and
`clear-attention` refuses to clear that pane-identity condition. Message ACKs
are not proof of process identity or conversation continuity.

[WebMux #176](https://github.com/jordanhubbard/webmux/issues/176) tracks the
integration with [OpenRig #1723](https://github.com/mvschwarz/openrig/issues/1723).
The [development fork](https://github.com/jordanhubbard/openrig) now implements
explicit SSH/tmux identity observation. WebMux detects the new attach options,
registers both ends, and uses the returned sender address independently of the
physical tmux session. A coordinator without those options retains the existing
transport, without gaining a remote identity claim.

The isolated [relay qualification](validation/openrig-relay-identity-2026-10-21/README.md)
used real macOS and Linux Codex processes, message replies, coordinator restart,
and a remote-shell refusal with no input written. The installed production
coordinator was not replaced. Remote Claude identity, provider telemetry hooks,
and automatic remote resume/fork remain outside this patch.

## Prerequisites

On the Linux/macOS WebMux server, put `rig`, `node` and tmux **3.4 or newer** on the service PATH.
Coordination rejects older or unrecognized tmux versions before calling Rig:
tmux 3.2a can crash its entire server when Rig probes a recently stopped seat.
Use a Node version supported by the installed OpenRig release (the qualified
version is OpenRig 0.6.14 with its own Node 22/24 runtime). WebMux's helper needs
Node 24 or later. A `rig` launcher must preserve `TMUX`, `TMUX_PANE`,
`OPENRIG_URL` and `OPENRIG_SESSION_NAME`: these bind the real transport and sender
identity. Do not install a wrapper that clears them.

Managed launches preserve the service's PATH order and append common tool
locations as fallbacks. This keeps project creation and agent enrollment on the
same Rig installation, including when the service uses a dedicated wrapper.

On each execution host, install the chosen provider CLI, authenticate it there,
and install a Rig **client** (`@openrig/cli@0.6.14`). For remote hosts, also install
tmux and Python 3 and enable noninteractive SSH from the WebMux service account.
The host must permit loopback reverse forwarding. Its login PATH or
`~/.local/bin`, `/opt/homebrew/bin`, `/home/linuxbrew/.linuxbrew/bin` must expose
these tools. Credentials are never copied by the integration.

An existing daemon is reused. WebMux remembers its port and checks it every 30
seconds so a restart uses the same endpoint. Closing WebMux viewers leaves the
daemon and tmux agents running. Native Windows coordination hosting is not
supported; Windows WebMux's existing terminals remain available.

## Owner workflow

1. Click **Coordination** and inspect the default provider's projects and
   sessions. Use **Change** for a temporary switch or **Make default** to keep
   the selected experience. The same default is available under
   **Settings → Coordination**.
2. Choose an existing project or **New coordinated project**. Select an
   execution host, browse to an absolute workspace on that host, name the
   session, and select Codex or Claude Code. Existing Rig projects also require
   choosing a canonical pod.
3. Click **Create session and open terminal**. The registered provider starts in a normal
   terminal tile with its Rig URL and sender identity already supplied. It
   discovers teammates using `rig whoami` and `rig ps --nodes -A`.
4. Repeat for another host using the same coordinator project and that host's
   workspace.
5. In either CLI, ask it to send a fresh phrase to its teammate with `rig send`
   and have the teammate reply. Watch both normal terminal tiles and the modal's
   recent coordination. Recorded messages are labeled with Rig's delivery state;
   a pending record alone is not proof of receipt.

Closing a tile detaches the WebMux viewer; **Open session** restores it without
launching another agent. After an SSH failure, **Restart provider and open** reconnects the
central attachment to the existing remote tmux session and reuses its tunnel
port. Reconnection retains Rig's exact enrolled session ID so historical records
with the same name do not make later reconnects ambiguous. Restarting WebMux reattaches persisted grid tiles without restarting the
provider processes. Explicitly exiting a provider stops that agent; reconnecting
a stopped agent launches a new provider session in the same registered seat.

Wheel over a managed terminal to scroll that seat's tmux history independently
of the 2D workspace. WebMux enables mouse mode on the coordinator's managed
session and, for remote seats, on the execution host's managed session. It leaves
the global tmux setting alone. Use the workspace scrollbar or minimap to
navigate between tiles.

On first use, complete the provider's folder-trust or sign-in prompt inside its
terminal. If the provider exits during startup, resolve the prompt and use
**Restart provider and open**. A live tmux attachment alone does not prove the provider is
ready or that a message was received.

Existing independently launched agents do not inherit environment changes.
They need an explicit Rig adoption/bootstrap step or a new managed launch to
participate. This release automatically enrolls agents created through the
modal; it does not inject instructions into arbitrary running CLI processes.

Coordination is an administrator feature in authenticated mode, matching host
Settings. Trusted mode exposes it to the trusted owner. It is not multi-tenant
isolation between users of the same OS account.

## Qualification

The browser tests use the actual published Rig daemon and text UI without
provider credentials. They also enroll two interactive provider stand-ins,
exchange a fresh challenge/ACK through Rig, inspect attributed messages, and
check grid refresh, tile close/reopen and two consecutive stopped-agent
reconnections. CI runs this against OpenRig 0.6.14;
these stand-ins qualify transport, not provider reasoning. Live cross-host
qualification additionally uses real
Codex processes on Puck and sparky, independent nonce/hash receipts, and the
central daemon's attributed outbox. See the [cross-host record](validation/openrig-central-2026-10-21/README.md).

Agent Deck's deterministic tests exercise local and remote discovery, guarded
queue/immediate sends, sanitized delivery status, conductor inbox records, and
remote drain provenance through a contract fixture. To run the read-only check
against the installed CLI, authenticated profile, and registered remotes, use
`WEBMUX_REAL_AGENT_DECK=1 go test ./internal/httpapi -run RealAgentDeck` from
`webmux/server`. It never launches a session or sends an instruction. Interactive
send/drain qualification remains explicit because it can affect live agents.

The [production Trickle workload](validation/openrig-trickle-2026-10-21/README.md)
records a real coding/review session created through the installed WebMux UI.

Earlier host-local tests remain as historical evidence:
[Claude/Codex on Linux](validation/openrig-live-2026-10-21/README.md) and
[initial Puck dashboard](validation/openrig-puck-2026-10-21/README.md). They are not
substitutes for cross-host qualification.

The optional diagnostics panel presents a **Connect dashboard** action and
**Refresh connection** when the coordinator is unavailable or has restarted.
Opening it does not launch agents. Once connected, closing the view preserves
the dashboard attachment; **Detach dashboard** disconnects only that viewer.
The main Coordination menu retains project and session management. The terminal dock's
**Pack** action fills visible grid holes from the top left while preserving
the current occupied column count.
