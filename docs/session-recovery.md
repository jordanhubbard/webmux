# Interrupted-work recovery (#318)

This is the implementation and qualification record for
[#318](https://github.com/jordanhubbard/webmux/issues/318), the first active
reliability priority. The feature is in progress. Reopening a window, displaying
history, or launching a new coding CLI does not prove recovered agent state.

## Recovery evidence

Each terminal owns a persisted recovery descriptor alongside its saved launch
identity. It records the last observed command and execution directory, the
coding CLI version and native conversation ID when known, and references to
retained WebMux transcripts. The existing atomic, synced session writer saves
observations while the process is running; shutdown is not the checkpoint.

Shell and CLI helpers send bounded metadata frames separately from display
output. A persisted per-terminal key signs these frames. They are removed before
logging, terminal replay and browser delivery. Logging pause also suppresses
command metadata capture, while readiness tracking remains active so an old
recovery command cannot be submitted into an unobserved running application.
Known credential arguments are omitted from recovery command metadata.

`GET /api/session-recovery/{id}` uses the session's owner boundary and returns
available activity and a bounded plain-text history preview. Missing or rotated
logs preserve the native-session descriptor. History readers reject traversal,
symlinks and substituted files, and remove terminal control sequences rather
than replaying clipboard/browser requests from disk.

`POST` on the same path submits a preview-bound native resume, approved command
replay or new-shell choice. It binds the owner, broker revision, execution-host
revision, waiting-worker instance and action UUID. The controller saves intent
before dispatch; the host consumes the UUID before launch. Repeating a consumed
request returns its receipt. A lost response is retained as an uncertain outcome.
`PATCH` persists the terminal's explicit automatic-native-resume preference.
Automatic resume requires active logging and a recorded running native
conversation; it never shares a transcript or replays a shell command.

The per-terminal logging choice survives restart. Pausing an integrated terminal
requires host acknowledgement before capture is reported paused. Minimal control
readiness stays available while paused, without recording new command/directory
content. An empty host cache can be seeded from the controller's saved evidence;
bootstrap never replaces newer host observations or an existing recording policy.

## Native adapters

- Codex: identify the rollout file opened by the owning process or its children,
  validate its `session_meta` project and ID, and select `codex resume <id>`.
  Concurrent candidates remain ambiguous; timestamps do not select a winner.
- Claude Code: preserve an explicitly selected ID, or allocate an exact native
  session ID using the installed CLI's supported `--session-id` option.
- Cursor Agent: preserve an explicit ID, or use `agent create-chat` and select
  that native ID when starting the interactive client.
  Recovery prefers the `cursor-agent` alias and verifies native help before
  using either alias: other products can also install an executable named
  `agent`. Unrelated clients pass through without Cursor session injection.
- OpenCode: allocate an exact native session through a temporary authenticated
  loopback server, stop that server, then select the ID in the normal TUI.
  No model turn is submitted during allocation.

Resume selectors use exact IDs, not `--last` or `--continue`. Supported explicit
restrictions (including read-only sandbox, Cursor ask/plan mode, Claude safe mode
and disabled tools) are preserved, along with Codex profiles. Unsupported custom
settings/tool rules require manual restoration rather than silently dropping
their policy. Explicit sandbox/approval bypass flags are not carried into a
recovery command. CLI stdout stays
attached to the original TTY, and the wrapper must preserve interrupt behavior.
Native state availability, supported versions, original execution host and
project still require validation at the recovery action.

The descriptor also snapshots supported CLI state/configuration path variables
and their absence. It does not serialize the complete environment or credential
values. Literal relative paths are retained and interpreted in the original
working directory; changing their spelling can also change native account or
keychain identity. A separate preflight process uses the recorded roots, and a
scoped CLI child receives them after consumption. Neither operation changes the
broker or waiting shell's environment. WebMux pins its own evidence-cache root
before launching the shell, so a CLI-specific `HOME` or XDG override cannot
redirect or lose its recovery observations. Unsupported inline OpenCode settings
require manual restoration and are excluded from automatic native recovery.
A new server discards an inherited parent-terminal cache pin before creating its
own providers. OpenCode metadata probes use `--pure` so they do not execute a
second copy of user plugins. Canceled preflight closes its private native server.
Claude and Cursor state-file references locate the recorded selector; observing
later interactive session switches remains separate work.

Native identity provenance is saved in signed host observations and durable
controller activity. A `launch-selector` identifies the conversation requested
at startup; it does not establish the current conversation after an interactive
switch. `process-state` identifies an ID observed in the running process's
native state. The recovery preview explains this distinction. Automatic native
resume requires process-state evidence; launch selectors and older records
without provenance remain available only for an explicit exact-session choice.
The other adapters still need active-conversation observers before their
automatic recovery can be qualified.

On 2026-11-09, the provenance qualification passed through real provider loss,
explicit native continuation and a new remembered-marker assistant response for
all four installed CLIs. Codex's embedded-profile case retained `process-state`;
Claude, Cursor and OpenCode retained `launch-selector`, with no promotion to a
verified active conversation. Broker regressions exclude launch selectors,
unavailable identity and legacy records from automatic execution while retaining
explicit continuation. Private manifests are under
`~/.local/share/webmux/recovery-qualification/`:
`configuration-06d50078-ee29-4d00-869c-d876e49323e0/manifest.json` (Codex/Claude)
and `configuration-a7b9d883-cfb0-4707-9013-2c33f86e0deb/manifest.json`
(Cursor/OpenCode). Both used executable SHA-256
`c7cceea75c096b58f9f5e0ff3c2009c05cc8de5a40cf32221e027e2374140515`.

## Acceptance record

| Requirement | Current evidence | Remaining work |
| --- | --- | --- |
| Durable owner-scoped activity and log references | Broker tests persist before shutdown; native VM SIGKILL/reboot retain activity and original transcript references | Remote/managed lifecycle qualification |
| Real shell command and directory capture | Bash/Zsh PTY probes preserve exit status; real WebMux local workflow records a long-running command, arguments and directory | Retain the separate Zsh probe in the qualification harness |
| Native conversation identity | All four installed CLIs recover a marker through WebMux provider loss and native recovery action; new native assistant records prove continuity; native preflight rejects another project; configuration-root and named-profile qualification below | Interactive session switches and remaining invocation-specific settings |
| Safe retained history | Bounded suffix, ANSI/OSC/C1 stripping, rotation and path controls; real local recovery retains the marker in API/UI history; redraw blank rows compacted only in the preview | Additional lifecycle/transport qualification |
| Surviving providers | Local, SSH and native mosh reconnect/restart/SIGKILL preserve provider and pane PIDs and job count; independent Linux user scopes avoid service-cgroup cleanup | Managed-provider qualification |
| Recovery preview and continuation | Owner-scoped API and browser actions; separate replay/handoff approval; broker tests for opt-in native auto resume and uncertain-outcome exclusion | Managed lifecycle integration and additional CLI lifecycle cases |
| Reboot/restart/update/crash | Native VM orderly restart, different-binary update, SIGKILL and kernel reboot pass; SSH and native mosh separately pass controller restart/SIGKILL and remote kernel reboot; approved replay, duplicate request and three reconnects preserve counts and original transcript references | Managed/CLI end-to-end coverage and additional remote failure modes |

Mosh synchronizes terminal screen state rather than an arbitrary byte stream.
Its recovery metadata path uses the separate authenticated SSH control channel
and must not
repurpose clipboard operations as a metadata channel. Pause behavior and
execution-host ownership apply to that path as well.

## Continuation control

Recovery actions must not be submitted as typed terminal input. A prompt-ready
observation can become stale while a user edits a partial command or starts an
application. A revision check alone cannot make that input safe. The unpublished
typed-input prototype was removed. The recovery-action HTTP endpoint now uses
the execution-host worker instead.

Provider startup, broker actions and browser continuation controls are connected
to the execution-host worker. The real local browser workflow exercises the full
path with a long-running job and an explicit approved replay. The browser also
previews recorded work for disconnected terminals.
Its fixed `--recovery-rpc` helper reads one bounded JSON request from stdin;
authentication keys and action payloads never travel as typed terminal input.
The RPC `ensure` operation provisions a stable tmux provider through a private
stdin script. Native tmux tests confirm that concurrent reconnects create one
provider, a surviving process keeps its PID, and a lost provider with recorded
activity receives the startup wait gate. The SSH control client uses a fixed
remote helper command, configured account/key/port and a bounded JSON pipe;
native SSH and mosh lifecycle qualification is recorded below.

On Linux, a tmux daemon created inside the WebMux service would inherit its
cgroup and be killed by the service's normal `KillMode=mixed` cleanup. Provider
creation therefore uses an independent transient scope under the same account's
systemd user manager when available. It retains the private stdin provisioning
path. The helper verifies actual provider cgroup ownership and reports a fallback
warning if jobs remain bound to the service. It does not change the service's
kill policy or enable lingering on the user's host.

The control path has these boundaries:

1. A stable, session-owned tmux provider is probed before creation. A surviving
   provider is reattached. A lost provider with recorded activity starts a
   dedicated recovery-wait worker, rather than silently replaying the command
   or injecting it into an existing shell/application.
2. The waiting worker owns a private local socket. Local operations and fixed
   SSH helper RPCs carry structured requests to that socket. The worker verifies
   its session key, native conversation/project/version, request revision and
   action ID before starting a child with clean terminal input. A live provider
   cannot be replaced by this action path.
3. Persist action intent and consumption before launch. Repeated requests and
   reconnects cannot launch the same action twice. Preserve an uncertain outcome
   after interruption instead of blindly retrying a command. Automatic recovery
   is opt-in and limited to native conversation resume, without transcript
   handoff or arbitrary command replay.
4. Execution-host helpers maintain bounded, signed activity/control snapshots.
   A private recording policy gates command persistence and honors logging
   pause. Mosh reads these snapshots over the authenticated SSH helper path;
   metadata must not depend on arbitrary OSC forwarding. No existing terminal
   key binding, clipboard state or user startup file is repurposed.
5. Rig and Agent Deck retain their lifecycle authority. Recovery must query and
   resume their existing provider identities; saved attachment argv alone does
   not authorize launching another agent.

Explicit managed reconnect now queries the enabled lifecycle manager's attach
operation for the exact saved provider-qualified target, after confirming that
the original process survives. Display status is not used as liveness evidence:
Agent Deck 1.16.34 omits status from `list --json`. Its read-only
`session ownership inspect` receipt must identify the exact instance and report
an owned live process with a surviving pane. Remote checks retain their original
remote namespace. Rig uses the coordinator's exact managed tmux probe or checks
the canonical tmux target directly. Neither probe launches, reconciles or kills
processes. Openable stopped entries are not treated as surviving work.
Reattachment checks the recorded
host, project directory and coding runtime before replacing the viewer, then
uses newly resolved attach arguments while retaining its WebMux session ID.
Unavailable/disabled managers, missing targets and changed destinations leave
the existing viewer and saved evidence intact. The broker rejects managed
reconnects that bypass this resolution. These requests never invoke a launch or
session-start operation. Tests cover native PTY viewer replacement through the
HTTP route, changed destinations, owner isolation and stale saved arguments.
Startup now performs the same resolution after ordinary terminal restoration.
The broker leaves saved managed viewers disconnected until their original
lifecycle owner validates them; it never starts their saved attach arguments
by itself. Current owner access is checked before provider discovery. A failed
manager leaves a bounded, plain-text recovery explanation beside the retained
history without preventing WebMux startup. A later successful reattachment
clears that explanation and retains the same viewer ID. The recovery preview
shows the recorded managed target, host and project directory.

Tests reconstruct the server around a real PTY viewer and a controlled Agent
Deck CLI fixture: a disabled manager preserves pre-restart history, later
validation uses fresh attach arguments, repeated restoration attaches once,
and revoked owner access prevents provider discovery. Broker tests cover
cancellation and unsafe/oversized manager diagnostics. These are lifecycle and
adapter-boundary regressions, not native Agent Deck or Rig conversation proof.
The separate native Agent Deck/Claude check below now covers controller
lifecycle and explicit provider-owned continuation. Rig, managed host reboot,
other managed CLIs/profiles remain open. The local Agent Deck/Claude managed-resume
action now has native proof below.

The native identity tests must also cover changing conversations inside one CLI
process. An initially selected conversation ID is not sufficient evidence after
the user switches sessions. Codex now rechecks its owning process's open native
state instead of permanently caching the first selector. Other adapters still
need their session-switch observations. Ambiguity requires an explicit fallback,
not a timestamp-based guess.

Remaining native integration includes observing active-session changes in each
interactive CLI, distinguishing a requested selector from verified active native
state, and extending supported invocation-specific settings without discarding
their policy. Configuration roots, a named Codex profile and the recorded
restrictions now have native continuation evidence below. Never carry
approval-bypass flags into a recovery command. The
four basic WebMux continuation paths now have native conversation proof below.
Those checks do not cover interactive session switches or every configuration
override.

Control tests exercise the real Unix socket, persisted consumption across a
worker's exit, stale and foreign requests, failed preflight, a revision change
during preflight, independent transcript/replay consent and one-time retrieval
of an approved replay script by its startup shell. A subprocess with its own
controlling PTY verifies that complete and partial input typed while waiting
cannot reach the reader started after recovery. These are worker/lifecycle
checks, not evidence of conversational CLI recovery or host-reboot acceptance.

Host lifecycle qualification uses a disposable native VM. Do not reboot
the user's workstation or existing shared workers. WebMux CLI-flow acceptance
needs a synthetic conversation marker recovered by each named CLI through its
continuation action, beyond
empty-session allocation/export or mocked adapter assertions.

## Native host lifecycle qualification

The disposable ARM64 QEMU guest passed orderly WebMux restart, abrupt SIGKILL,
actual guest host reboot, approved continuation and repeated reconnect tests on
2026-11-09. The guest uses the recommended systemd user-service model with
`KillMode=mixed`; its tmux provider runs in a separate user scope. Restart and
SIGKILL preserved provider PID 3227, pane PID 3228 and one job execution. Reboot
changed boot ID from `772b4a10-70fc-4867-af3e-84f8a3d1fdeb` to
`8c1097a8-c4c0-4245-adc7-e364a38257c9`, started a new waiting provider and kept
the execution count at one until explicit approval. Approved replay increased
the count to two. An identical action request and three reconnects kept it at
two and retained the original transcript reference.

A second complete run also performed atomic executable replacement. The update
artifact was built from commit `72285783b05c8529124e9be0f53a3eee22ab945d`,
SHA256 `b7877003c87d6d58cc136653ebb3f5bda05b7f55acf00ee50faa7372ba7db382`,
and differed from the retained baseline binary. Service PID changed from 2156 to
2574 while provider PID 1760, pane PID 1761 and one execution were preserved.
The installed guest hash was checked against the update artifact. The run then
passed SIGKILL and another actual reboot, from boot ID
`8c1097a8-c4c0-4245-adc7-e364a38257c9` to
`5a60bef7-c6ab-4307-b1a1-d953f1d70859`, followed by the same approval,
duplicate-request, retained-history and three-reconnect checks.

The fixture is based on the official
[Ubuntu Noble ARM64 image](https://cloud-images.ubuntu.com/noble/20261017/),
SHA256 `50e24e1fb79e89116e939d90aa7738f7af48c442fa1bc2510131e0b39c50348e`.
Its disk, private SSH key, QMP socket, source/binary receipt, service definitions
and JSON lifecycle receipt are retained under `_build/recovery-vm/`. This is a
real kernel reboot, not a process restart or mocked lifecycle signal.

The harness verifies the private QEMU process/socket and known fixture account
before running. It submits only a synthetic fixture job:

```bash
python3 scripts/qualify-recovery-vm.py --vm-root /absolute/path/to/disposable-fixture
# Add --update-binary /absolute/path/to/a/different-linux-arm64-webmux to
# exercise atomic executable replacement while preserving the provider.
```

The local VM and remote transport runs qualify provider lifecycle behavior.
Conversational CLI checks are recorded separately below; managed-provider
recovery remains open.

### Remote SSH and mosh

`scripts/qualify-recovery-remote.py` starts its own private controller and targets
only the verified disposable QEMU guest. Each transport passed controller
restart, controller SIGKILL, actual remote kernel reboot, explicit approved
replay and three reconnects. The native mosh run disables SSH fallback and
records a running descendant `mosh-client` at every stage. Separate authenticated
SSH snapshots capture its command metadata; screen rendering is not metadata.

SSH preserved provider/pane PIDs 2229/2230 across controller failures. Guest reboot
changed boot ID `2f11b5d2-254b-4573-8f1b-4ec6d9723fa2` to
`df1794d5-253c-429c-9964-db03c3f68540`. Mosh preserved PIDs 3513/3514; its reboot
changed boot ID `66fbf6c5-3bf1-411a-90e1-a52cf4a76804` to
`2f11b5d2-254b-4573-8f1b-4ec6d9723fa2`. Both retained execution count one until
approval, then count two after duplicate receipt lookup and repeated reconnects.
Original transcript references and pre-interruption markers remained available.

Receipts under the private VM evidence root are
`ssh-controller-8105a648-90d0-4c83-8df3-d5820c0e5fd6/receipt.json` and
`mosh-controller-da5d9c02-762d-4cc5-868a-46a54f207bfb/receipt.json`.
An earlier mosh attempt fell back to SSH and does not qualify mosh. Native
testing exposed mosh's option permutation: the launcher now terminates its own
options before the destination so remote `--session-attach` is passed to WebMux.

## Native conversation qualification

The Codex WebMux continuation path also passed a real interactive check. A
recorded running Codex provider was destroyed, then the structured native
recovery action resumed the same project and native ID. The new invocation kept
`--sandbox read-only`, `--no-daemon` and `--no-alt-screen`. A new assistant message
in the native rollout returned the earlier synthetic marker plus a fresh
post-recovery phrase; existing screen history was not the success oracle.
The private receipt is
`_build/recovery-qualification/webmux-native-15c396bf-4d2f-4253-bc1f-ceda0719dd62/codex-conversation-receipt.json`.
The following probes qualify the other named CLIs; session switching remains separate.

Claude Code also passed the WebMux provider-loss/native-action path. The installed
CLI updated from 2.1.318 to 2.1.321 during qualification; the supported resume
options were verified again. The resumed invocation retained `--safe-mode` and
`--tools ""`. A new assistant record in the original native JSONL returned the
earlier marker plus a fresh post-recovery phrase. Its receipt is beside the
Codex receipt as `claude-conversation-receipt.json`. The HTTP test client timed
out while waiting for preflight; the durable intent showed consumed and the
already-started native client was inspected without submitting another action.

Cursor Agent 2026.11.05-86047df passed the same WebMux continuation path with
`--mode ask --sandbox enabled` retained. The new assistant text was verified in
new JSON assistant blobs in the original project-scoped SQLite store, excluding
all blob IDs present before provider loss. Its private receipt is
`cursor-conversation-receipt.json` beside the Codex/Claude receipts. The executable
used was the verified `cursor-agent` alias, while `agent` resolved to Grok.

OpenCode 1.19.5 passed with `--pure --agent plan` retained. Its new assistant
message was verified by native export from the same session, excluding every
message ID present before provider loss. The receipt is
`opencode-conversation-receipt.json` in the same fixture directory. Thus all four
installed CLIs have actual WebMux provider-loss/native-action continuation
evidence, in addition to the separate headless checks. Interactive conversation
switches and managed lifecycle recovery remain open acceptance work. The tested
configuration-root/profile cases are recorded below.

### Original CLI configuration and profile qualification

`scripts/qualify-recovery-configuration.py` passed all four installed CLIs with
the private controller deliberately configured to use different state roots.
After provider loss, preflight and the resumed CLI used the original descriptor's
roots, including absence of an override. The before/after native argv matched,
and every CLI produced a new native assistant answer containing its previous
synthetic marker plus a fresh post-recovery phrase. Repeating the action returned
its consumed receipt. No transcript context was submitted.

Codex 0.167.0 retained a private `webmux-qualification` profile and read-only
sandbox. New native hook-trust prompts were answered by continuing without
trusting hooks. Claude Code 2.1.321 retained an unset `CLAUDE_CONFIG_DIR`, which
restored its original authenticated default despite the controller's override.
Cursor Agent 2026.11.05-86047df and OpenCode 1.19.5 retained their recorded roots
and ask/sandbox or plan restrictions. The private manifest is
`~/.local/share/webmux/recovery-qualification/configuration-99f75e1b-b49c-4173-84e1-15cfe5a3be67/manifest.json`,
with binary SHA256
`1ef5a0db50f3414da3b11003b90e5e26fcbdcff59370600fa78869bbba9f0fb1`.

Earlier fixture attempts did not qualify Claude: an invented configuration root
opened onboarding or lacked that root's native authentication. Those failures
are retained separately; they were not counted as recovery success. The harness
now preserves the already-authenticated native root or its original absence.
It references existing state paths rather than copying credentials and keeps
new fixture profile/state files in its private directory.

```bash
python3 scripts/qualify-recovery-configuration.py \
  --binary /absolute/path/to/standalone-webmux \
  --manifest /absolute/path/to/synthetic-native-manifest.json
```

These checks establish the tested root/profile cases, not interactive session
switches or every possible CLI flag/custom settings payload. Unsupported custom
settings continue to require explicit manual restoration.
The four-CLI root/profile comparison used `--no-daemon` for Codex. A separate
shared-daemon explicit-selection check now passes below; observing the active
conversation after in-process switches remains open acceptance work.

The configuration harness also accepts `--cli codex --codex-daemon` to exercise
explicit native continuation with the normal daemon, without the private
profile or `--no-daemon`. A failed
case writes a manifest with `passed: false`, the executable hash, last activity
and any completed receipts before fixture cleanup. A successful embedded case
must not stand in for this separate daemon case.

An earlier 2026-11-09 build displayed the synthetic conversation in the default
Codex 0.167.0 daemon case but cleared its explicitly selected native ID when the
TUI had no owned rollout file. That data-loss behavior is now corrected: a
verified native file for the exact selector/project is retained as a manual
choice. Later owned-process observations replace it; loss of observed ownership
removes automatic confidence while retaining the last known reference. This
does not establish which conversation is active after an unobserved switch.
Four observation experiments did not qualify a replacement:

- Routing through `--remote` rejected the original read-only sandbox override.
  That changes invocation semantics and is not used by the recovery adapter.
- Native TUI recording reported event kinds, including a conversation switch,
  without the selected conversation ID in the tested records.
- Native SQLite logs distinguish processes with a PID and process UUID, but the
  tested TUI records did not report its current conversation after resume and
  `/new`. Process-scoped log presence does not establish native identity.
- Injecting SessionStart hooks through `-c` produced Codex's warning that command
  configuration overrides require embedded mode. This does not establish
  shared-daemon observation. Only the fixture's reviewed metadata hook was
  trusted; existing hook trust and the shared daemon were not changed.

[Official Codex hook documentation](https://learn.chatgpt.com/docs/hooks) defines
session IDs and transcript paths in hook input. A viable integration still needs
to prove native lifecycle delivery, terminal ownership and conversation switches
while retaining the user's launch mode and hook policy. No unqualified observer
or permission workaround is installed by these experiments.

The corrected explicit-selection path passed real shared-daemon continuation
with Codex 0.167.0. WebMux retained the selected conversation and its native
state file after provider loss, accepted an explicit native action, and received
a fresh assistant record containing the earlier marker plus a new test phrase.
The native invocation retained `--sandbox read-only` and `--no-alt-screen`,
without adding `--no-daemon` or changing to remote mode. Both observations stayed
`launch-selector`; automatic recovery remained ineligible. The daemon's native
Unix peer PID `23567` and start time `2026-11-09 10:03:13` stayed unchanged.
Controller roots deliberately differed from the CLI's original roots.

Private receipt:
`~/.local/share/webmux/recovery-qualification/configuration-789c40d0-f922-480a-a61b-f5b87206a6ef/manifest.json`.
Executable SHA-256:
`215a449d59da42f09c4cbbb1390ec9a93438f69e7a6d311721019430fd25d1af`.
Qualification script SHA-256:
`42d36577e0cd9a57eefd5b07a842be72b18bc1b6ba73b4ccff7f8fc53672b846`.
The harness verifies its source stays unchanged and compares the daemon's native
peer PID/start identity before and after continuation. This qualifies explicit
recovery of the selected conversation, not automatic recovery of whichever
conversation is current after a switch.

On 2026-11-09, real two-process synthetic-marker checks passed for Codex
0.167.0, Claude Code 2.1.318, Cursor Agent 2026.11.05-86047df and OpenCode
1.19.5. Each first process exited before a second process selected its exact
native conversation ID and returned the remembered marker. The gated native
preflight test also passed for all four: it accepts their real native metadata
in the recorded directory and rejects the same ID from another project.
These checks do not establish interactive session-switch tracking, the WebMux
continuation path, transport recovery or reboot acceptance.

The retained harness uses installed authentication, submits only synthetic
content, and saves private evidence and projects outside existing Git checkouts:

```bash
python3 scripts/qualify-recovery-native.py
# Or select adapters with repeated --cli codex / --cli claude / ... options.
# The final JSON line gives the manifest's absolute filename.
```

Run metadata preflight on the same host and account using that explicit file:

```bash
cd webmux/server
WEBMUX_QUALIFY_NATIVE_MANIFEST=/absolute/path/to/manifest.json \
  go test -race -v ./internal/recovery -run TestQualifiedNativeConversationPreflight
```

The live checks uncovered two compatibility requirements now covered by
regressions: successful help can be written to stderr, and changing a subprocess
directory must also update its `PWD` environment. Identity commands continue to
read stdout alone; successful help includes both streams. Metadata output is
bounded during capture. No project-ownership validation was relaxed.

## Explicit managed native recovery

For a recorded local Agent Deck/Claude conversation, **Recover work** offers
**Resume through Agent Deck** with a separate approval checkbox. The summary
shows the saved host, project, profile, native ID and state-file reference. The
manager owns its saved launch command and current settings; the browser supplies
neither a command nor transcript contents to this action. Other managed runtime
and destination combinations keep their manual fallback.

The server rechecks current owner access, the enabled manager, the exact saved
destination and profile, and the native ownership receipt. A verified surviving
process is attached first. Before starting a replacement, the adapter requires
qualified Agent Deck 1.16.34, proof that no process remains, unchanged native
references and bounded native JSON records matching the ID and physical project
directory. The operation revalidates these observations immediately before
calling the manager's exact-session start command, then checks the resulting
native ID and process ownership before attaching the existing viewer.

A request is bound to the recovery revision and saved managed identity. Its
intent is persisted before the external operation. Duplicate consumed requests
return the saved result, including across controller restart. An unresolved or
uncertain operation blocks another start, even under a new request ID; inspect
the manager and reconnect manually. No force, reconciliation, abandonment,
message injection or arbitrary shell replay is used. Missing state, changed
projects/profiles and escaped or unverified processes preserve the logs and
require manual recovery. Unit and HTTP regressions cover these boundaries;
real native continuation evidence follows.

## Native Agent Deck lifecycle evidence

On 2026-11-09, real Agent Deck 1.16.34 and Claude Code 2.1.322 passed the
private managed-session harness. Orderly controller restart, SIGKILL and three
explicit reconnects kept native pane PID `64117` and exactly one managed pane on the private socket.
A new assistant record returned the earlier synthetic marker and a fresh test
phrase through the restored WebMux viewer.

After explicit loss of the native process, WebMux retained the prior transcript,
reported managed recovery unavailable and rejected reconnect with HTTP 409;
no native process was started implicitly. The explicit WebMux managed-resume
action then asked Agent Deck to resume the same native conversation, created pane PID `70930`, and WebMux
reattached the original viewer. Another new native assistant record returned
the earlier marker and a second fresh phrase. The native conversation ID stayed
`08bbfe0e-d31c-4cb6-9d18-546eea56a08c`; the WebMux viewer ID stayed
`e46b1a6f-72a0-4a7c-86ce-5302f033676e`. Repeating the same action, including after another
controller restart, returned its durable consumed receipt and retained exactly
one native pane with the same new PID. All fixture processes were stopped afterward.

The harness creates a private manager home/profile and tmux socket. A scoped
native launcher restores the original CLI configuration roots and their
absence, with plan mode and tools disabled; only the synthetic native transcript
is referenced into the private manager home. Authentication files are not
copied. The fixture declines the known native global-permission migration dialog
and verifies the original Claude settings file is unchanged. It waits for the
actual tmux client before sending input, and counts only new assistant records.

Private receipt:
`~/.local/share/webmux/recovery-qualification/agent-deck-native-7fe586aa-360f-4666-b6f2-8dcffc881d5c/lifecycle-manifest.json`.
Executable SHA-256:
`47c647500fbc851ae46f118d7fab4a40594036ace0266dc1c1d012d5e00f3d60`.
Qualification script SHA-256: `dfdbc6e75bee21108534d072c0139aa313c33190875d674c71fd968fd8bdd337`.
The harness verifies that its source stays unchanged during the run. The latest run also verifies that the recorded native
conversation ID, state-file reference, manager profile and observation provenance
survive both controller restart and SIGKILL. Logging-gated capture has separate
regressions: pause preserves the last reference; resume permits a new observation.
The preview labels these as manager inventory observations, because later
conversation switches are not yet verified. Local profile selection is explicit;
changed profiles and ambiguous IDs are rejected before attachment.
An earlier Claude 2.1.321 run established explicit provider-owned continuation
through the manager CLI; the WebMux action above was tested with 2.1.322.
Earlier failed fixtures remain negative evidence: one exposed the missing native
inventory status field; others stopped at the native permission dialog and did
not establish a conversational response.

```bash
python3 scripts/qualify-recovery-managed.py \
  --binary /absolute/path/to/standalone-webmux \
  --manifest /absolute/path/to/synthetic-native-manifest.json
```

This qualifies the tested local default-profile Agent Deck/Claude path and
the explicit WebMux managed-resume action through its lifecycle owner. It does
not qualify managed host reboot, Rig, remote managed hosts, other managed
CLIs/profiles or authentication-failure behavior. Those remain part of #318's
acceptance work.

## Sources

- [Official Codex command reference](https://learn.chatgpt.com/docs/developer-commands#codex-exec)
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference)
- [OpenCode CLI](https://opencode.ai/docs/cli/), checked against installed CLI help
- [Cursor CLI sessions](https://cursor.com/docs/cli/overview#sessions)
- [Mosh screen-state protocol](https://mosh.org/mosh-paper-draft.pdf)

- [Agent Deck ownership CLI source at the installed revision](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/cmd/agent-deck/session_ownership_cmd.go)
