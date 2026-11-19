# Agent coordination

WebMux is the always-available foundation: terminals, connected-server views,
and direct terminal messages. Agent Deck and Rig are independent optional
integrations. You can enable either or both; each agent retains one lifecycle
owner. Enabling both does not automatically merge their agent identities,
queues, or completion events.

Open **Coordination** to configure three independent choices:

- **Enable Agent Deck** and **Enable Rig** control discovery and integration
  services. Rig supervision follows its enable setting. Disabling an integration
  leaves existing agents and terminal viewers running.
- **Show sessions** filters **All sessions**, **WebMux**, **Agent Deck**, or **Rig**.
  **Make default view** saves that filter. All sessions includes this server's
  native terminals and enabled integration inventories, with owner labels and
  owner-specific actions. Connected WebMux server terminals remain available
  through the Terminals server selector.
- **Launch owner** selects who manages a newly created agent. **WebMux** opens
  the native terminal workflow; Agent Deck and Rig provide their own launch
  forms. **Make default launch owner** saves this preference. Existing agents
  keep their original owner when you change it.

**More → Settings → Coordination** exposes the same independent preferences.
New installs use WebMux as the launch default, All sessions as the view, and no
external integrations. A single unavailable integration reports its own error;
it does not replace or disable the other inventory.

```yaml
app:
  coordination:
    enabled_providers: [agent-deck, rig]
    default_launch_provider: webmux
    default_view: all
```

Legacy `default_provider: agent-deck` or `rig` migrates to enabling that
integration and preserving its launch/view preference. Legacy `webmux`, `None`
(`disabled`), or `choose`, and missing settings, migrate to WebMux with external
integrations off. Saving configuration writes the new independent fields.

WebMux direct messages report terminal-input queue admission, not agent
processing or task completion. Agent Deck adds guarded instruction queues and
completion records; Rig adds canonical team topology and peer messaging. Native
terminal state is observed transport state, not proof that a coding agent is
ready. Merely viewing inventories never launches an agent.

The following sections describe the optional external integrations.

The browser deliberately keeps three concepts separate: a coordinator project
(a Rig topology or Agent Deck group), a coordinator session/seat, and a
filesystem workspace on an execution host. Canonical provider discovery is the
source of truth. WebMux's private launch records enrich that inventory but do
not create a second, stale project list. Running, stopped, and historical Rig
topologies remain visible with their provider IDs.

## Start an agent

1. Open **Coordination**, enable **Agent Deck** or **Rig** and select it as **Launch owner**, and choose the folder containing your code. Use **Browse**
   to find it on the selected execution host.
2. Choose Codex or Claude Code and a purpose such as building, reviewing, or
   investigating. Purpose suggests an illustrative session name; it does not
   instruct or configure the coding agent.
3. Start a new project to group agents working on that code, or add an agent to
   an existing project. WebMux suggests a project name from the folder and a
   session name such as `codex-reviewer`, adding a numeric suffix to avoid known
   collisions. Both names are editable labels with no special command meaning.
4. Select **Start agent and open terminal**. Its terminal opens in the ordinary
   grid; give the agent its actual task there.

In an empty grid cell, choose **Open existing agent seat** to select a canonical
seat for that position. The Coordinator selector continues to offer creating a
new agent; Localhost and remote connections remain separate choices.

In the terminal grid, expand **Coordinated sessions** to see projects and seats
from enabled integrations. Selecting a seat reveals its existing viewer. For a
seat without an open viewer, choose an empty grid cell to place it, or choose
**Cancel seat placement**. Opening a viewer does not create a new coding agent.
The navigator preserves the existing tile's position and uses the coordinator's
current attachment identity. **Manage coordination** opens the full inspector.
The collapsed navigator does not request an inventory; hiding the grid suspends
its requests. An unavailable inventory removes its seat actions until a fresh
observation succeeds.

Existing sessions can be opened from **Your projects and sessions**. Activity
and provider diagnostics remain collapsed until requested. Merely visiting the
tab never creates a project or starts an agent.

## Projects and worker identity

Project identity comes from the coordinator's structured session record, keyed
by execution host, provider and session ID. Agent Deck supplies its group and
registered path; Rig supplies the topology/seat binding and registered workspace.
WebMux retains that association in its persisted session document and refreshes
it when the provider session is attached again. A registered workspace is not a
live CWD readout: agents may change directory without changing project membership.

Terminal titles and the session dock show **project / session**. The browser
window title follows the focused terminal and remembers its focus separately
per browser tab across reloads. Hover a title or open **Tools** for host, identity
source and the registered workspace, when available.

An unmanaged SSH/tmux terminal has no coordinator record. **Set project** saves
an explicit label on the server; it survives reconnects and restarts. WebMux
does not guess its project from terminal output, prompt text or a transient CWD,
and does not present an unobserved current directory as live information.

Source interfaces: [Agent Deck JSON session identity](https://github.com/asheshgoplani/agent-deck/blob/main/skills/agent-deck/references/cli-reference.md#session-current),
[Codex thread metadata](https://learn.chatgpt.com/docs/app-server), and
[Claude session/CWD hook events](https://code.claude.com/docs/en/hooks).
Coding-CLI metadata can supply additional location information when explicitly
integrated; WebMux's project label does not depend on parsing private transcript
formats or installing hooks into the user's coding tools.

The Coordination tab groups workers under expandable project headings. Each
worker shows its registered name (such as coder or reviewer), execution host,
runtime and observed state. Expand or collapse a project with its summary button
or Enter/Space; **Open session** attaches an existing viewer and **Restart
provider and open** appears for a stopped seat with a WebMux launch record and
registered workspace. A WebMux launch record is not native Rig lifecycle ownership.
A worker name is the registered name, not an inferred permission or capability.

Opening a managed terminal records its coordinator target, project, name, host
and runtime with that tile. The **Rig** badge shows project, worker and execution
host separately from the terminal's editable title and transport badge. For
remote workers the viewer still attaches locally to an SSH relay: its EXEC
transport is not a claim that the provider executes locally. Existing tiles gain
this metadata when opened again from Coordination. Renaming an ordinary terminal
does not turn it into a Rig viewer. Tile identity is retained registration metadata,
not a live activity or verified-process indicator. The separate tile status strip
labels current lifecycle, bootstrap and activity observations.

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

Agent Deck is optional and is not installed by default. Use
`make install-agent-deck` or `make install-server INSTALL_AGENT_DECK=yes`
on macOS/Linux. The installer is idempotent, pins the qualified version, and
uses Agent Deck's checksum-verifying upstream installer. Alternatively, install
the optional Homebrew formula:

```bash
brew trust --formula asheshgoplani/tap/agent-deck
brew tap asheshgoplani/tap
brew install asheshgoplani/tap/agent-deck
```

Enable Agent Deck in Coordination after installation.

When the integration is enabled and `agent-deck` is on the WebMux service PATH,
Coordination also inventories
the current local Agent Deck profile and every remote registered with that
Agent Deck instance. Session IDs are qualified by location (`local:<id>` or
`<remote>:<id>`), so identical names on different hosts do not collide. The
browser shows the native Codex or Claude session ID when Agent Deck reports it,
and uses Agent Deck's own local or remote attach transport when opening a tile.

To let Agent Deck panes follow WebMux's terminal colors, merge this setting into
Agent Deck's `config.toml` on each execution host (use `agent-deck config get
theme --json` to locate the effective file):

```toml
[tmux]
window_style_override = "default"
```

Add the key to an existing `[tmux]` table rather than creating a duplicate table.
This is Agent Deck's [supported pane-style override](https://github.com/asheshgoplani/agent-deck/blob/v1.16.34/internal/session/userconfig.go).
It lets the terminal emulator supply the background instead of Agent Deck
painting a fixed color based on the desktop appearance. The setting persists
across attaches and applies to Agent Deck sessions opened in other terminals too.
Existing attached panes may need a reconnect before Agent Deck reapplies it;
the coding agent itself does not need to restart.

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

Local Agent Deck messaging, delivery lookup, inbox reads and completion collection
select the discovered session profile, including an explicit `default` profile.
Ambiguous session IDs across profiles are refused for targeted actions; remote
session actions retain their registered-remote routing. See the
[profile-routing qualification](validation/agent-deck-profile-routing-2026-11-17/README.md).

Rig provides canonical topology and peer messaging. Both integrations can be enabled together. They retain separate ownership and
project databases; a shared-agent identity and event bridge is future work. Additional coordinators can implement
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

## Rig prerequisites

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

## Coordinator selection scope

WebMux's controller queries, lifecycle commands, diagnostic dashboard and new
local/SSH agent launchers follow their configured Rig endpoint. They bind the
private CLI host-selection context to `local`, preserving the attachment's
`OPENRIG_URL` and sender identity. An unrelated host selected in the operator's
ordinary Rig CLI no longer redirects these commands. User selection settings
are not rewritten. Existing running agents retain their launch environment;
apply an updated launcher through an explicit provider restart when needed.

On 2026-11-16, real Rig 0.6.14 (`df1c1517`) in an isolated home reproduced the
problem: discovery failed with an unrelated selection despite a healthy
configured daemon, and succeeded with local scope. The fix passed both cases.
The eight-case browser suite also passed with that unrelated selection,
including actual Rig enrollment, attributed messages, viewer reuse and dashboard
detachment that preserved the daemon. Those native cases use a provider stand-in
and do not qualify authenticated coding conversations. Helper controls cover a
stale tmux-server environment, local/SSH launchers, preserved endpoint/sender
identity and live-proof command scope.

This is a provenance repair under #194. Broader canonical inventory/adoption,
notifications and lifecycle qualification remain open; remote Rig reporting
through the outbound client (#200) and identity/event bridging (#246) remain
separate features.

## Execution-host evidence for external seats

The [#430](https://github.com/jordanhubbard/webmux/issues/430) candidate keeps an
externally enrolled Rig seat's execution host unknown until a trusted launch
record supplies it. A local tmux attachment may front SSH; it establishes where
the viewer connects, not where the coding process executes. The native Rig
`hostSelfId`, when available, is retained separately as `rigHostId` in inventory
and `rig_host_id` in terminal metadata. It is an opaque identity, never an SSH
destination. WebMux-created local and SSH seats retain their recorded hosts.

Opening an external viewer, reloading or reopening its tile preserves that
uncertainty and the original provider. Recovery previews show **Unknown execution
host** and do not offer native resume for an unverified destination. Explicitly
opening the canonical seat also corrects an older viewer's inferred localhost
label while reusing that viewer. Missing or unsupported recovery evidence still
leaves retained logs and manager-based recovery available.

The [qualification record](validation/rig-host-provenance-2026-11-17/README.md)
includes real Rig enrollment, ordinary terminal input, reload/reopen and one
unchanged process. It uses a synthetic terminal, not an authenticated coding
conversation. The broader macOS Rig browser suite has two failures reproduced
on unchanged main, tracked separately in [#431](https://github.com/jordanhubbard/webmux/issues/431).

## Rig owner workflow

1. Click **Coordination**, choose **Rig** as the Launch owner, and inspect
   its projects and sessions. Use **Make default launch owner** to keep that launch preference. The same default is available under
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
launching another agent. If Rig reports a stopped seat after an SSH failure,
**Restart provider and open** reconnects the central attachment to the existing remote tmux session and reuses its tunnel
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
**Restart provider and open** once Rig reports the seat stopped. A live tmux attachment alone does not prove the provider is
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
check grid refresh, tile close/reopen and two consecutive attachment-loss
reconnections. Each loss waits for a fresh native absence verdict tied to the
exact pane and kill time before applying WebMux's existing projection deadline.
Rig 0.6.14 samples identity every five seconds; its observation phase is bounded
to 15 seconds, and the overall case retains its 120-second limit. Every replacement
must exchange a new peer-generated ACK, because old readiness text can remain in
the reused viewer's scrollback. Native proofs and enrollment/challenge receipts are
retained in the test output. CI runs this against OpenRig 0.6.14;
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

Rig lifecycle comes from canonical `rig ps` rows. A missing WebMux-managed
tmux relay marks the attachment transport unavailable and disables Open session;
it does not mark the Rig seat stopped. Seats without a reported lifecycle show
unknown. Restart provider is offered only when Rig explicitly reports stopped.
A detached seat with a missing relay instead offers Reconnect attachment and
open, explaining that a surviving provider is reattached and a new conversation
starts if none remains.
For a running seat whose relay has disappeared, inspect Rig and its execution
host before launching another provider. Attachment transport is reported
separately in the coordination inventory; externally discovered seats without
a WebMux relay observation show unknown.


## Agent Deck minimum version

The candidate update in #414 requires Agent Deck 1.16.39 or newer for terminal
attachments and pins structured native recovery to exactly 1.16.39 until further
versions are qualified. Upgrade the optional coordinator on the controller and execution hosts before
using this WebMux update with Agent Deck. Attachments now reject an older local
coordinator with a clear update instruction while preserving the provider. Older 1.16.34 attachments can discard startup
keystrokes when terminal queries go unanswered; the candidate removes WebMux's
startup-query exception in favor of the upstream fix. Existing transcripts and
manual recovery remain available for unqualified native manager versions.

The ordinary installed coordinator is kept intact during private qualification.
Native lifecycle, reboot, policy and input-delivery evidence is required before
this candidate becomes the supported minimum. Rig and built-in coordination
retain their independent ownership and lifecycle.

## Canonical local seat identity

Local Rig inventory uses an opaque `local-rig/<key>` seat ID derived from the
canonical Rig ID and logical node ID. The key is scoped to this controller's
configured local coordinator. `projectId` and `logicalId` describe that canonical
identity. `managerTarget` is Rig's current session address; `attachmentTarget`
is the current tmux attachment address. Neither address is the logical seat ID.
The existing authenticated `host-rig/<client>/<key>` remote namespace is separate.
An opaque host self-ID remains diagnostic metadata, never an SSH destination.

Created viewers persist `rig.seat_id`, alongside the native manager `rig.target`
and `rig.attachment_target`. These are not coding-conversation IDs. Opening an
older exact-address viewer can add its freshly resolved canonical key without
creating a second viewer. Old API clients may still request an exact current
session address; missing or ambiguous canonical identities fail closed.

After an external owner explicitly rebinds a logical seat in Rig, **Open
session** follows fresh canonical inventory and reuses the viewer, retained
history and grid position. It detaches the previous viewer client, leaving the
old provider alone. A hidden viewer keeps its saved cell when free, falls back
to another free cell when occupied, and refuses a full grid. Placement failures
do not detach a running viewer.

Transport reconnect and controller startup reattachment require the original
canonical seat and native destination. They refuse a changed binding instead
of following it automatically. An attachment change discards cached native
process/conversation references; existing lifecycle-manager validation still
owns any later native recovery. This does not adopt providers, infer execution
hosts or qualify authenticated coding conversations.

The [native identity qualification](validation/rig-seat-identity-2026-11-17/README.md)
records an explicit provider-owned unclaim/rebind with stable seat/viewer/layout,
changed attachment, exact input routing, retained history and preserved old
provider. Broader lifecycle, adoption and change-notification work remains under
[#194](https://github.com/jordanhubbard/webmux/issues/194).


## Canonical activity in grid tiles

Rig tiles show Rig lifecycle, bootstrap status and activity as separate observations.
`Bootstrap ready` means Rig completed startup; it does not establish that a coding
provider can accept an instruction. Fresh, generation-resolved runtime hooks can
report agent running, idle or needing input. Terminal output is labelled as output;
other observational sources remain activity estimates. Silence, a connected viewer
and a surviving tmux session never become provider readiness claims.

The grid reads Rig facts every ten seconds only when an authorized workspace has
Rig viewers. It does not enable an integration, query Agent Deck or read message
bodies. Requests do not overlap, have a twenty-second deadline, and expire the
previous response after thirty seconds. A failed or partially failed discovery
clears positive observations. Activity samples older than thirty seconds, more
than five seconds ahead of the browser clock, stale, or of unresolved hook
occupant provenance are unknown. Clock disagreement can therefore conservatively
hide activity. These bounds are WebMux display limits, not Rig freshness promises.

Each logical seat is matched by its canonical ID. Activity and bootstrap facts
also require the viewer's saved manager and attachment targets to match current
inventory. A changed binding needs explicit Open before those facts can describe
that viewer. Logical lifecycle can still report `detached` after Rig removes a
stopped seat's attachment. Missing or duplicate identities remain unknown. These
checks do not add an atomic occupant-conditional lifecycle contract.

Raw activity evidence, prompt text, runtime event bodies, hook tokens and resume
tokens are omitted from the observation projection. The terminal-first empty
workspace, ordinary terminals and Agent Deck tiles keep their existing behavior.
The older standalone navigator remains unmounted, as established by #228.

The [native activity validation](validation/rig-seat-activity-2026-11-17/README.md)
covers actual builtin terminal seats, a native seat stop, sibling survival and
viewer placement. Hook attention and stale/rebind behavior have deterministic
coverage; authenticated coding-provider readiness remains unqualified.

### Native Stop contract still needed

A private WebMux launch record is not native Rig lifecycle ownership. Current
WebMux launchers use Rig's adopted/claimed enrollment path; native `rig seat stop`
refuses those sessions. For an SSH launch, the local tmux process is a relay whose
exit does not terminate the remote persistent provider. An isolated native test
confirmed that one natively launched terminal can be stopped without affecting its
sibling, while an adopted terminal is refused and stays alive.

Published Rig 0.6.14 accepts a stop reference plus reason/operator but no atomic
expected rig/node/session or occupant-generation precondition. A separate WebMux
preflight cannot protect a reviewed occupant from replacement before that native
command executes. Issue #438 tracks ownership, conditional identity and remote
effect requirements before a Stop control can ship. Hide, reconnect and native
recovery keep their existing separate contracts; this activity UI adds no stop
action and does not complete #194.


## Enrollment origin and launch records

The **Rig** badge identifies a viewer linked to a recorded Rig seat. It does not
claim that Rig launched the process or can stop it. Coordination labels its saved
WebMux launcher as **WebMux launch record**, independently of manager-reported
**Launched by Rig**, **Adopted by Rig**, or unknown enrollment. Tiles show the same
enrollment distinction only when their saved native binding matches inventory.
A changed binding or unavailable observation shows unknown for that viewer.

The optional coordination fields are `enrollmentOrigin` (`launched`, `claimed`,
`unknown`), `enrollmentSource` (`rig-session-registry`, `unavailable`),
`enrollmentId` and `enrollmentObservedAt`. The enrollment ID is a Rig registration
record, not a coding conversation or a resume token. Existing `managed` remains
compatible: it means a WebMux private launch record for Rig seats. A native Rig
launch can therefore have `managed: false`; a WebMux-launched process enrolled
with `attach --self` can have `managed: true` and `enrollmentOrigin: claimed`.

This read-only projection is qualified for published OpenRig 0.6.14 (df1c1517).
It joins the manager's node inventory and session registry within each immutable
rig ID, checks the logical seat, node, latest enrollment and native address, and
uses that build's greatest-session-ULID selection rule. History, conflicting
bindings, duplicate identities and malformed rows cannot establish current
lineage. Other builds, including the separate native recovery qualification,
remain unknown until their enrollment contract is qualified. Native recovery's
existing version and policy checks are unchanged.

Only display discovery requests these facts. Attach, survival and recovery reads
keep their original discovery path without enrollment HTTP requests. The reader
uses GET against the local daemon port reported by the configured CLI, refuses
redirects, caps each response at 512 KiB and each sweep at 8 MiB, uses two rig
workers (up to four simultaneous requests), and has an eight-second deadline.
At most 256 seats, 32 rigs and 4096 rows per response are accepted; excessive or
incomplete inventories degrade to unknown. One failed rig does not lend its
lineage to another. Host-client Rig observations retain their existing protocol
and report no enrollment origin through it.

Raw session tokens, prompt or activity evidence, and event bodies are not copied
to the public projection. Manager-reported enrollment describes registration
lineage; it neither proves provider liveness nor grants authority to stop a local
or remote provider. The conditional Stop, native launch/adopt and remote ownership
requirements in #438 remain open. See the [native enrollment validation](validation/rig-enrollment-origin-2026-11-17/README.md).


## Live local Rig changes

The #443 candidate refreshes visible Rig tiles when the qualified local daemon
reports a change. It re-reads canonical inventory before updating the tile; an
event itself never establishes lifecycle or provider readiness. Empty workspaces
and ordinary terminal workspaces do not subscribe. Closing or disabling the
workspace cancels its wait. Canonical polling and stale-observation expiry remain
the fallback when the optional stream is unavailable or its build is unqualified.

This path is qualified for local OpenRig 0.6.14 (df1c1517). Other builds and remote
host notifications remain outside this qualification. It performs no lifecycle
action, adoption or Agent Deck bridge. See [native notification evidence](validation/rig-change-notifications-2026-11-17/README.md)
for the paused-fallback-timer browser case, cancellation, bounds and remaining
integration gates.


## Combined grid and Agents qualification

The [Rig integration candidate](validation/rig-grid-integration-2026-11-17/README.md)
reconciles canonical seat identity/activity/enrollment and notifications with the
observed Agents pane. Its live case keeps a real Rig terminal seat and a separate
observed tmux terminal active through navigation, reload and viewer closure,
without transferring ownership. Hidden terminal panes now suspend Rig metadata
observation while their terminal viewers stay mounted. This remains a candidate
pending hosted/review/audit gates; the broader lifecycle, recovery and bridge
requirements remain separate.

## Checked local seat Stop

The checked Stop candidate under #438 requires the published OpenRig CLI and
daemon `0.7.18 (3bf87c62)`, a native local Unix manager, and `lsof`/`ps` process
and listener evidence. Linux also requires `readlink` and the manager's
`/proc/<pid>/exe` link; Node 24's `MainThread` thread name is not executable
identity. The manager's stamped build, OS owner, listener PID,
process start and daemon entry point are checked before a review is issued and
again before dispatch. Unverified, tunneled or changed destinations refuse.

For a running seat whose native enrollment says **Launched by Rig**, choose
**Review local seat Stop** in Coordination, supply a reason, inspect the
current destination and explicitly confirm. The reviewed node and enrollment
IDs are passed unchanged to Rig's `seat-stop.v1` checked HTTP operation. Rig
owns the final binding/occupant guard and terminal effects. Typing protection
and other native refusals remain effective.

The stated effect is the manager-owned local tmux seat and all its attached
viewers. Jobs outside that session, including remote providers, may continue.
The viewer's grid tile, placement and retained log stay available. Native
**Adopted by Rig** seats are ineligible; a WebMux launch record does not grant
lifecycle authority. WebMux-created adopted launchers, including their SSH
relays, remain ineligible. A [controlled SSH relay qualification](validation/rig-stop-relay-2026-11-18/README.md)
uses a native Rig terminal whose owner starts an actual SSH transport: confirmed
Stop ends that local transport while the remote Linux tmux process survives.
Native WebMux launch conversion, authenticated remote coding and host-failure
acceptance remain open under #438/#194/#318.

A review expires after three minutes. Before dispatch, WebMux durably claims
the immutable native enrollment, with private owner-scoped reviews/results and
an enrollment-wide claim that also covers another admin or a changed daemon
port. File contents and directory links are synced before lifecycle dispatch.
Duplicate requests return the retained result. A lost response or interrupted
controller leaves an uncertain consumed claim, including after the review
expires; no automatic retry is offered. A confirmed no-effect refusal permits
a new explicit review. A detach receipt is distinct from a kill attempt.

[Qualification and limits](validation/rig-checked-stop-2026-11-18/README.md)
cover real native replacement/adoption refusal, selected-seat termination,
sibling survival, retained layout/history and duplicate receipts. The CI
fixture installs 0.7.18 separately from the existing 0.6.14 compatibility lane.
These synthetic terminal controls do not qualify authenticated coding-session
recovery, a remote provider Stop, or an installed manager upgrade. Optional
manager dependency review remains part of #91; the isolated 0.7.18 audit
snapshot records the existing Hono static-middleware advisory separately from
WebMux's unchanged dependency-audit gate #429.

### Choose a pod in an existing Rig project

With the qualified Rig 0.7.18 integration, an empty pod can receive its first
agent. Click the desired empty grid cell, choose **Rig** as the coordinator, then
choose **Add an agent to an existing project**. Select the project and its current
**Rig pod**, choose the code folder and start the agent. Stopped projects remain
selectable when Rig confirms that their namespace still exists.

**Refresh pods** rereads that project's current definitions, including pods that
have no seats. Discovery starts no agent. If the project was archived or a chosen
pod was removed, WebMux refuses the launch; refresh the choices before trying
again. It does not create a replacement project or guess a default pod.

The legacy integration continues to expose its observed pod choices. The native
empty-pod contract and live qualification are recorded in
[canonical empty-pod selection](validation/rig-empty-pods-2026-11-18/README.md).
