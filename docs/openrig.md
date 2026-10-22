# Agent coordination

WebMux 1.3.23 integrates OpenRig behind the ordinary **Terminals** grid. The
**Coordination** button opens a modal showing managed agents, execution hosts,
project directories, and recent attributed messages recorded by Rig. Creating
an agent opens its interactive CLI in a normal terminal tile. The raw Rig text
dashboard remains available under **Advanced diagnostics**.

## Projects and worker identity

The Coordination modal groups workers under expandable project headings. Each
worker shows its registered name (such as coder or reviewer), execution host,
runtime and observed state. Expand or collapse a project with its summary button
or Enter/Space; Open terminal and Reconnect agent retain their existing behavior.
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

On the Linux/macOS WebMux server, put `rig`, `node` and `tmux` on the service PATH.
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

1. Click **Coordination**. Inspect current agents and recent messages.
2. Under **Create agent**, enter a project ID, agent name, execution host and
   absolute project directory on that host. Select Codex or Claude Code.
3. Click **Create and open terminal**. The registered provider starts in a normal
   terminal tile with its Rig URL and sender identity already supplied. It
   discovers teammates using `rig whoami` and `rig ps --nodes -A`.
4. Repeat for another host using the same project ID and that host's directory.
5. In either CLI, ask it to send a fresh phrase to its teammate with `rig send`
   and have the teammate reply. Watch both normal terminal tiles and the modal's
   recent coordination. Recorded messages are labeled with Rig's delivery state;
   a pending record alone is not proof of receipt.

Closing a tile detaches the WebMux viewer; **Open terminal** restores it without
launching another agent. After an SSH failure, **Reconnect agent** reconnects the
central attachment to the existing remote tmux session and reuses its tunnel
port. Reconnection retains Rig's exact enrolled session ID so historical records
with the same name do not make later reconnects ambiguous. Restarting WebMux reattaches persisted grid tiles without restarting the
provider processes. Explicitly exiting a provider stops that agent; reconnecting
a stopped agent launches a new provider session in the same registered seat.

On first use, complete the provider's folder-trust or sign-in prompt inside its
terminal. If the provider exits during startup, resolve the prompt and use
**Reconnect agent**. A live tmux attachment alone does not prove the provider is
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

The [production Trickle workload](validation/openrig-trickle-2026-10-21/README.md)
records a real coding/review session created through the installed WebMux UI.

Earlier host-local tests remain as historical evidence:
[Claude/Codex on Linux](validation/openrig-live-2026-10-21/README.md) and
[initial Puck dashboard](validation/openrig-puck-2026-10-21/README.md). They are not
substitutes for cross-host qualification.
