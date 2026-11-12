# Interrupted-work recovery (#318)

This is the implementation and qualification record for
[#318](https://github.com/jordanhubbard/webmux/issues/318), the first active
reliability priority. The feature is in progress. Reopening a window, displaying
history, or launching a new coding CLI does not prove recovered agent state.

On 2026-11-10 the owner requested integration of the current checkpoint through
[#322](https://github.com/jordanhubbard/webmux/pull/322) and fix-forward work in a
Claude session. This supersedes the earlier decision to hold the PR as a draft
until full qualification. #318 remains open. See the current
[roadmap](roadmap.md) and [repository handoff](../MEMORY.md); historical receipts
below retain their original scope and must not be read as completing the remaining
native managed-reboot or remote acceptance work.

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
  session ID using the installed CLI's supported `--session-id` option. Qualified
  native SessionStart hooks additionally observe in-process clear and resume
  changes, as described below.
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
and disabled tools) are preserved, along with Codex profiles. Claude additionally
retains supported model, agent, effort, settings-source, system-prompt-snapshot,
variadic tool rules and extra-directory options. Unsupported or incomplete
settings require manual restoration rather than silently dropping
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
Cursor state-file references locate the recorded selector; its later interactive
switches remain unverified. Claude has a separately qualified, scoped observer
for same-project conversation changes. Cross-project changes require manual
validation of invocation roots and policy.

Native identity provenance is saved in signed host observations and durable
controller activity. A `launch-selector` identifies the conversation requested
at startup; it does not establish the current conversation after an interactive
switch. `process-state` identifies an ID observed in the running process's
native state. The recovery preview explains this distinction. Automatic native
resume requires process-state evidence; launch selectors, native hook
observations and older records without provenance remain available only for an
explicit exact-session choice. Native hooks can stop reporting when settings
change, so their observations do not acquire automatic execution confidence.
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
| Reboot/restart/update/crash | Native VM orderly restart, different-binary update, SIGKILL and kernel reboot pass; SSH and native mosh separately pass controller restart/SIGKILL and remote kernel reboot; approved replay, duplicate request and three reconnects preserve counts and original transcript references; remote managed hosts pass hung, rebooted and powered-off fallback with no duplicate execution ([details](#remote-managed-host-failure-2026-11-10)); Rig seats on an SSH host pass the same failures plus loss of the local seat ([details](#rig-seats-on-an-ssh-host-2026-11-10)) | Native coverage listed below |

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

### Codex shared-daemon observation (2026-11-10)

A further review of Codex 0.167.0 and upstream `main` through
`rust-v0.169.0-alpha.7` found no observation point that identifies a TUI's
current conversation while preserving the default daemon launch mode and
explicit sandbox policy:

- `thread/loaded/list` returns only the daemon's loaded thread IDs, with no
  per-connection mapping; `server/diagnostics` reports process gauges.
- The TUI session log (`CODEX_TUI_RECORD_SESSION`) records `/new` as a variant
  only. Outbound `UserTurn` ops include a `client_user_message_id`, but the
  daemon does not persist it in rollout records or thread items
  (`clientUserMessageId` appears only in turn/steer/queue inputs), so it cannot
  join a terminal to a thread. Matching prompt content or timestamps is not used.
- A transparent relay on the TUI's own connection reliably observes thread IDs in
  `thread/start`/`thread/resume` responses and `thread/started`, but requires
  `--remote`, which rejects the original `--sandbox read-only` override.
- The daemon socket is derived from `CODEX_HOME`; no separate socket override
  exists, and changing `CODEX_HOME` moves all native state.

**Decision (owner, 2026-11-10): Codex in default daemon mode is manual-only.**
Explicit selections remain qualified for manual continuation and observations
stay `launch-selector`; automatic recovery is not offered for these launches,
and WebMux does not change the user's launch mode to obtain evidence. A local,
opt-in way for the TUI to report its current thread was requested upstream as
[openai/codex#60625](https://github.com/openai/codex/issues/60625). Revisit when
that or an equivalent ships. A per-terminal `--no-daemon` opt-in was considered
and deferred.

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

## Scoped Claude conversation observations

The macOS/Linux implementation for Claude Code 2.1.322 adds only a
SessionStart hook through per-invocation settings. It observes native startup,
clear and resume events without writing user/project settings or altering
permission rules. Hook merging and policy restrictions remain Claude's own
[settings and hook behavior](https://code.claude.com/docs/en/hooks). Existing
`--settings` or `--setting-sources` invocations, bare/safe mode, safe-mode
environment overrides, explicit `--agent` invocations and unqualified versions keep the original
launch path and manual evidence. Native managed-only or disabled-hook policy is
never overridden.

The callback sends bounded native JSON to a private per-launch Unix channel.
The receiver authenticates its token and OS peer process ancestry, rejects
subagent callbacks and unknown event sources, and validates the declared native
state file against the recorded project. Hook stdout stays empty; terminal
history and instructions are not sent through this channel. Nested coding CLI
commands cannot replace the foreground CLI's recovery identity. Logging pause
suppresses persistence while retaining prior evidence. Normal completion drains
already authenticated callbacks before saving the final activity.

The preview labels this provenance `native-hook` and explains why it remains
manual recovery evidence: hook settings can stop subsequent observations.
Cross-project changes remove current-identity confidence and preserve the last
known conversation for an explicit choice. This does not qualify automatic
Claude recovery, native Linux/Windows execution, remote hooks or every hook
configuration. Linux and Windows builds are checked separately from native
conversation proof.

The native macOS check passed `/clear` into a new conversation, `/resume` back to the
original, and `/resume` into the new conversation again. After actual tmux
provider loss, the structured native recovery action selected the new ID rather
than the original launch selector. A fresh assistant record returned the new
conversation's synthetic marker plus a fresh phrase. The recovered observation
matched the new waiting worker's instance, restrictions and original roots.
Duplicate recovery returned the consumed receipt. Native permission-settings
checksums stayed unchanged; the fixture provider was removed afterward.

Private receipt:
`~/.local/share/webmux/recovery-qualification/configuration-cbeb3e1c-c037-492a-be3f-a9f93c85ab30/manifest.json`.
Recovered native ID: `c91820c5-636f-4ccc-9731-b9a80087da40`.
Executable SHA-256:
`1a391c73434bc9f2c8f19b9326e03f97d406a03aa7c499a4044558d20bf31259`.
Qualification script SHA-256:
`9598ecc81084e1f7daef6f5fdf07c6b8de44bb98a318d37a6dd19119569f8aff`.

```bash
python3 scripts/qualify-recovery-configuration.py \
  --binary /absolute/path/to/standalone-webmux \
  --manifest /absolute/path/to/synthetic-native-manifest.json \
  --cli claude --claude-hooks
```

Earlier negative fixtures remain evidence: one exposed alias dispatch into the
coding CLI instead of the private helper; another raced the wrapper's initial
selector before the authenticated callback. The alias regression and the
new-worker/provenance wait conditions now cover these failures.

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

## Unsupported invocation options

Codex, Cursor and OpenCode also reject unknown or incomplete recovery options
rather than silently dropping them. Exact native selectors are replaced;
supported restrictions retain their original values. Unknown sandbox values
require manual recovery. Explicit bypass modes are omitted. Settings without a
qualified preservation path, including explicit model overrides in these three
adapters, receive a manual explanation in the preview. This leaves the original
CLI launch unchanged; it limits the proposed recovery action.

After this change, all four named CLIs passed real provider loss, explicit
native recovery and fresh remembered-marker responses with original supported
configuration roots and restrictions. Private receipt:
`~/.local/share/webmux/recovery-qualification/configuration-c84a70e7-c30e-413b-9eb0-edb26f434992/manifest.json`.
Executable SHA-256:
`5a020592f8d943230a51f87ca3f350046dfd6556f903ccb54e90ae83f967b9d5`.
Qualification script SHA-256:
`e2ed9340d13f735e576ede95f9edf725b4f3706f1ef812be3e1b4509dbbab63d`.
The full Go race suite and vet passed. An earlier attempt used a development
binary without embedded assets and stopped before controller startup; its
negative manifest is retained and does not count as native evidence.

## Claude invocation settings

Claude recovery parses supported invocation settings explicitly. Variadic
`--tools`, `--allowedTools`, `--disallowedTools` and `--add-dir` values retain
their order, including an explicitly empty tool list. Named model/agent, effort,
permission mode, settings sources and system-prompt snapshot selection are kept.
Original conversation selectors are replaced with the exact recorded ID, and
initial prompts and permission bypass flags are not replayed. Unknown flags,
opaque settings, custom prompt payloads and incomplete options produce a
bounded manual-recovery explanation when first observed, so the preview does
not offer an action that discards those settings. Inline agent JSON is redacted
from saved command arguments.

A real Claude Code 2.1.322 conversation on macOS passed provider loss followed
by the explicit WebMux native-resume action. The resumed invocation retained
its model, named main-session agent, plan mode, disabled tools, allow/deny rules,
project-only settings sources, effort, extra directory and snapshot setting.
A new assistant record returned the earlier marker, a fresh test phrase and the
agent-profile tag, which was never supplied in the user prompt. The native ID
was unchanged; the recovered activity belonged to the new worker instance.
The duplicate request returned its consumed result. The generated profile and
native permission-settings file checksums remained unchanged.

Private receipt:
`~/.local/share/webmux/recovery-qualification/configuration-126af277-39c9-49f2-9660-5f57ef997e1e/manifest.json`.
Executable SHA-256:
`2c878c3d81fe92fc15b7cc32f0686f493c3773331bf412a5fc63c18f9a3ab202`.
Qualification script SHA-256:
`e2ed9340d13f735e576ede95f9edf725b4f3706f1ef812be3e1b4509dbbab63d`.

The harness's `--claude-options --trust-owned-project` mode creates a private
project and approves the native trust prompt only after checking the exact
project path, sole generated profile file, absence of symlinks and profile
checksum. This uses Claude's native project-trust flow and records that choice;
it can add a native project-trust record. It does not change WebMux recovery's
trust behavior. Without the explicit harness flag, the prompt remains pending.
Fixture providers and generated active profile files are cleaned up; receipts,
profile definitions, logs and failed attempts remain available. Earlier failed
runs exposed an unavailable profile and a pending trust prompt; neither proved
recovery. This evidence covers the tested local invocation, not every Claude
setting, remote host or native Windows execution.

## Native Rig lifecycle evidence

On 2026-11-09, a private Rig 0.6.14 (`9d0200f3`) daemon adopted one real
Claude Code 2.1.322 conversation through its native tmux registration. Orderly
WebMux restart, WebMux SIGKILL and three explicit reconnects retained pane PID
`72800`, one native pane and the existing WebMux viewer. A new assistant record
returned the earlier synthetic marker and a fresh test phrase through that
viewer. The Rig enrollment session ID was kept separate from the Claude
conversation ID; it was not offered as native conversation evidence. The
coordination inventory omits the misleading native-session field for Rig
enrollment IDs and exposes their value separately as `managerSessionId` with
a Manager session label. This preserves enrollment/re-enrollment tracking
without presenting it as a provider-native conversation. A real native regression failed against the preceding binary
and passed with this correction; the negative receipt is retained under
`rig-native-9936fb26-e4bc-46db-b55f-efcc8d4fa8f7/manifest.json`. The full Go
race suite and vet passed after the correction.

The fixture also killed its private Rig daemon after verifying the PID against
its own loopback health endpoint, private database path and instance record.
WebMux's existing coordinator supervisor started a replacement daemon
(`72563` to `74899`) and reattached the same native pane. A second case killed
the daemon while a fixture guard refused `rig daemon start`. WebMux started
with retained history and a managed-unavailable explanation, and reconnect
returned HTTP 409 without replacing the surviving native process. Removing
the guard, starting the manager and restarting WebMux reattached that same pane
and cleared the unavailable explanation. The refusal guard is test fault
injection; the daemon, manager inventory and native conversation are real.

After explicit loss of the native pane, WebMux retained its prior history,
reported managed recovery unavailable and rejected three reconnect attempts
with HTTP 409. No replacement was launched. Rig native-resume through a snapshot
is not implemented by this evidence; process loss requires recovery through Rig.

The harness uses an empty topology, no kernel or team launch, a private daemon
home/database and tmux socket, and a separately supplied installed CLI/runtime.
The native conversation uses its original configuration roots with restricted
mode, strict MCP configuration, tools disabled and plan mode. Provider
credentials are not copied. Native permission settings retain their checksum.
Cleanup verified zero fixture panes and a successful private-daemon stop.
The shared Rig daemon and primary WebMux service are untouched.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-c926121b-f6c5-45a4-86e0-f0396d6bf253/manifest.json`.
Executable SHA-256:
`040244bd4a5dd6a68acb2a684d751870c17ebbfc585ffa99c4486ab0260244f4`.
Qualification script SHA-256:
`634ce0672cbf59960e2f0f92a85b6a887c1d5f6e22100c78bde242c6b4c32e3c`.
Earlier negative fixtures exposed an incorrect target-prefix assertion and a
test assumption that the existing supervisor would leave the manager stopped.
Their manifests and cleanup receipts remain retained; neither is a completed
qualification. The earlier survival-only positive is also retained.

```bash
python3 scripts/qualify-recovery-rig.py \
  --binary /absolute/path/to/standalone-webmux \
  --manifest /absolute/path/to/synthetic-native-manifest.json \
  --rig-cli /absolute/path/to/openrig/dist/bin-wrapper.js \
  --rig-node /absolute/path/to/compatible/node
```

This qualifies local adopted Rig/Claude survival and manual fallback. It does
not qualify Rig snapshot/native-resume execution, managed host reboot, remote
Rig destinations or other Rig-managed runtime configurations. Those remain
part of #318's acceptance work.

## Rig snapshot-plan boundary

The harness's optional `--snapshot-plan` mode captures a private native Rig
snapshot and requests `rig launch <rig-id> agents.continuation --snapshot-id
<id> --plan --json` before and after native process loss. Both responses must
report `ok: true`, `planOnly: true`, the exact snapshot and registered node,
no unmatched seats and unchanged non-target seats. The live native pane must
retain its PID; the lost-process case must retain zero native panes. The
harness executes no snapshot launch.

This passed for Rig 0.6.14 (`9d0200f3`). The resulting snapshot's adopted
session had null `resumeType`, `resumeToken` and resume provenance even though
the exact-seat plan succeeded. A successful plan therefore cannot by itself
prove native conversation continuity or justify exposing a resume action.
The native conversation, host/project, original roots and invocation policy
must be verified separately through the lifecycle owner. The existing manual
fallback remains appropriate for this unverified adopted session.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-6ea648a9-515b-4ff0-bd00-711073ad0833/manifest.json`.
The fixture retains both plan responses and the private snapshot. Executable
SHA-256: `040244bd4a5dd6a68acb2a684d751870c17ebbfc585ffa99c4486ab0260244f4`.
Qualification script SHA-256:
`0fb03534c3656a4b3c5b2b16317d975b312a687df8bd1b08fcd22209ac2cb2a9`.
The complete survival/fallback matrix also passed in this run, and cleanup
confirmed no native pane and a stopped private daemon.

## Native Rig conversation observation

The optional `--snapshot-plan --native-hooks` fixture mode uses the installed
Rig relay's SessionStart handler with per-invocation Claude settings and the
routing environment returned by native Rig enrollment. It copies only the
published relay source into the private fixture; provider credentials and
user/project settings are not copied or rewritten. This test invocation allows
hooks instead of passing `--restricted`, while retaining strict MCP
configuration, disabled tools and plan mode. The production recovery adapter
does not change an original invocation's hook opt-outs on this evidence.

Rig 0.6.14 (`9d0200f3`) with Claude Code 2.1.322 recorded the correct
`claude_id` and `hook` provenance. A subsequent native snapshot, after WebMux
restart/SIGKILL and private Rig daemon loss/recovery, retained that exact
conversation ID and reported its native resume metadata as resumable. The
fixture checked the ID against the previously qualified native conversation;
it did not set a token through operator attestation or modify manager storage.
A fresh assistant record proved conversation continuity through the WebMux
viewer. Both exact-seat snapshot plans remained plan-only, with no native
replacement after process loss. The complete lifecycle/fallback matrix passed,
and cleanup confirmed zero fixture panes and a stopped private daemon.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-e9a4b78b-0893-43ff-b581-40ef3b181367/manifest.json`.
Executable SHA-256:
`040244bd4a5dd6a68acb2a684d751870c17ebbfc585ffa99c4486ab0260244f4`.
Qualification script SHA-256:
`24ab1a3f5760a660836c3366529570251b11e946f79c6ab4625a6bdc9af72d40`.
Published Rig relay source SHA-256:
`c7fbf01d1762203272ae5835549bfd5316f98bb11e0734270e35cb04fe0f9c4d`.

This establishes native Rig conversation observation for the tested local
invocation. It does not yet implement a WebMux Rig native-launch action or
qualify native resume after host reboot. Those still require exact snapshot,
destination, native-state and execution-policy validation through the lifecycle
owner, plus once-only launch and post-launch continuity evidence.

## Manager identity and browser validation

The Rig inventory preserves the enrollment identifier as `managerSessionId`;
`nativeSessionId` remains reserved for provider-native conversation evidence.
The UI labels these separately. The real published-CLI browser test uses the
manager identifier to verify re-enrollment and explicitly checks that no native
conversation is invented. Its diagnostic test requests the topology section
before asserting its content, because the native TUI may initially show its
startup screen.

The earlier hosted Linux failure at `9f8422b` exposed the old test's reliance on
the mislabeled field and its initial-page assumption. With the correction, all
eight published Rig browser tests passed locally. The full Go race suite/vet,
strict TypeScript, frontend lint/build and 337 frontend tests also passed.
Real Claude/Rig survival, manager-loss, native-hook identity, plan-only snapshot
and manual fallback checks passed again against the corrected inventory.
Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-29eb4580-d313-4eae-a43b-a6906c2e5e52/manifest.json`.
Executable SHA-256:
`6447fdcec05c9fdc03393605c853875025db4124d821b26899682d622498ea12`.
Qualification script SHA-256:
`4c861eed273a5917349fbdba6d4b59fc22e1e15ba95698489bd48a33c1cad140`.

The explicit `--native-resume` probe now qualifies native Rig-owned snapshot
continuation as described below. Earlier negative fixtures established that
adoption and materialization alone do not provide managed startup context.
Cleanup closes the generated private tmux socket, including attention shells
under derived canonical names; it does not touch other sockets or the shared
daemon. The WebMux Rig native-launch action remains unimplemented.

## Rig-owned native snapshot continuation

Rig 0.6.14 (`9d0200f3`) and Claude Code 2.1.322 passed the private managed
native-resume probe. A normal spec launch creates the manager's real startup
context. The fixture enters native plan mode and selects plan for future
managed continuation before seeding its synthetic marker. Its private
manager-owned executable restores the original provider roots and merges
SessionStart reporting plus a deny-all-tools rule into the manager's invocation
settings. It preserves the settings supplied by Rig for each invocation, including hooks and permission
rules. The documented deny glob removes tools from the model's context; the
native transcript also contains no model tool-use records.

After the controller/manager lifecycle matrix and loss of the native process,
Rig received one exact-seat launch with the saved snapshot ID. Its initial
joined process proof reported attention required while preserving the resumed
process. The fixture then used native `reconcile-session --no-launch` for the
same rig and node. The pane PID remained unchanged during reconciliation.
Reconciliation reports continuity and cwd as unverified; the fixture proves
them independently rather than promoting that receipt's confidence.

The resumed native CLI is a descendant of the new verified private pane. Its
kernel-reported cwd matches the original project, its configuration roots
(including absence) match the original roots, and its actual argv selects the
original conversation in plan mode. The fixture verifies that each invocation's manager-supplied
hook and permission settings survive the additional observer/deny rule.
The first-launch comparison below additionally checks preservation across resume.
Exactly one native resume execution is recorded. Through the restored WebMux
viewer, a new assistant record returns the earlier marker and a fresh test
phrase. Cleanup confirms no remaining private panes and a stopped daemon.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-45e07284-76c8-4205-b1bb-62c6c4443835/manifest.json`.
Executable SHA-256:
`6447fdcec05c9fdc03393605c853875025db4124d821b26899682d622498ea12`.
Qualification script SHA-256:
`2493f0e64f68b84cbeda00041621e09e7858c09170c91d2171873d4fa09a3a78`.

Earlier attempts remain negative evidence. They exposed missing startup
context on adopted seats, a native first-launch permission selection different
from a pre-materialized seat's selection, and the manager's strict process
parser rejecting extra CLI flags. The qualified no-tool rule is configured
through native settings from the initial managed launch; recovery does not
remove an original restriction to satisfy the parser. This evidence covers
provider-owned local native continuation. A WebMux recovery action still needs
durable snapshot/identity/policy binding, current owner/access checks,
survivor-first validation, once-only execution and equivalent post-launch proof.
Managed reboot and remote Rig destinations remain unqualified.

## Rig checkpoint recording in WebMux

For local default-profile Claude seats on Rig `0.6.14 (9d0200f3)`, attaching
an owned viewer with WebMux logging active now records the verified native
conversation, native state path and an exact manager snapshot. The reference
binds the rig, node, logical seat, physical project, native file identity and
current plan-mode policy. WebMux asks for generation metadata without a byte
offset, so that request does not retrieve conversation text. Settings are
hashed; their contents and unrelated snapshot data do not enter the recovery
record. Unchanged observations reuse a private snapshot reference.

Capture is serialized with logging changes and scoped to the current owner.
Paused logging does not capture new metadata and retains the previous record.
Missing native state, manager authorization failures, unsupported versions or
profiles, changed policy and snapshots without verified native identity keep
manual recovery. The preview shows the recorded manager checkpoint and native
conversation without offering an unimplemented Rig launch action.

The real private Rig/Claude qualification passed with WebMux capture enabled:
checkpoint capture and reuse, orderly restart, SIGKILL, three reconnects,
manager loss/recovery, lost-process fallback and provider-owned exact-snapshot
continuation with a fresh remembered-marker response. This does not qualify a
WebMux Rig launch action, managed host reboot or remote Rig destination.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-55b11beb-5713-458d-844e-de308b4d0db2/manifest.json`.
Executable SHA-256:
`a0ff01314c3fb2fdfff094d05576c53bab04eb4fd63412b6ae5070236fba46ce`.
Qualification script SHA-256:
`a67d8a3b5910cc3360511753e90fe2e12d79a50e7972c3a9f982f9f8e8fb38b9`.
Local validation passed: full Go race suite and vet, strict TypeScript, lint,
production build, 338 frontend tests and 40 helper tests. Broker tests cover
owner isolation, logging pause/resume, durable reconstruction and destination
changes; helper tests cover snapshot reuse, policy changes and rejected native
identity or manager authorization.

## Rig saved-checkpoint preflight

The embedded Rig helper now has a read-only preflight for the recorded checkpoint.
It resolves the current canonical seat and checks the saved rig/node/profile,
physical project, current plan policy and settings hashes. Capture and preflight
verify the native file's device, inode and creation time as well as bounded
native session/project records. Snapshot validation also checks the selected
node's runtime, profile and cwd and its active native conversation.

Preflight rejects a changed snapshot digest or a native plan that selects another
snapshot/node or reports non-target effects. It neither creates a checkpoint nor
launches a process. A successful plan does not establish process ownership or
absence and does not enable WebMux's Rig launch action. Capture now has a bounded
15-second request deadline.

The private native fixture exercised this embedded helper against WebMux's own
recorded checkpoint with both a surviving and a lost process. Neither plan
changed the process count or identity. Rig subsequently resumed from that same
recorded snapshot and returned a new remembered-marker response with the original
roots, settings, plan restriction and kernel cwd. Controller/manager lifecycle
and repeated manual-fallback checks also passed.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-a2330a20-d8ad-4e8c-9118-f79369f4db70/manifest.json`.
Executable SHA-256:
`7792af655c05554a34873b81a28fe6ea70eb8c8fe73b37c8d84e9aad6fae46fd`.
Qualification script SHA-256:
`475fa4b7453a6ebf5d5c1f4460b2d23ef2c98af13230657957da20462ab2f4f4`.
Full Go race tests/vet, strict TypeScript, the standalone build and all 42 helper
tests passed. Additional helper cases reject changed settings, native files,
snapshot contents, target nodes and non-target effects. The Windows CI failure
at `0e60a5c` came from Unix-only absolute paths in the broker test fixture;
`372a4fe` uses platform-native temporary paths without weakening production
validation. Exact-head Windows CI remains the required proof of that fix.

## Independent Rig native-process verification

The embedded helper can now independently verify the resumed Claude process
beneath an already-owned Rig pane. It reads a bounded PID/parent/UID census,
then reads arguments only for that pane's descendants under the WebMux execution
account. A second Claude conversation or an unrecognized Claude invocation
makes the observation ambiguous. The qualified grammar is the published Rig
launch form with plan mode, the exact native resume ID, canonical seat name and
inline settings last; unsupported forms remain manual.

The verifier joins the native executable path to the OS process, checks Claude
`2.1.322`, hashes the executable and settings without retaining their contents,
and compares the kernel cwd to the original physical project. It samples the
native arguments and start time again and confirms the same pane PID before
returning a bounded metadata receipt. The Linux implementation uses `/proc`;
the real native evidence below qualifies the macOS `ps`/`lsof` path. No Linux
native-process qualification or WebMux Rig launch is established here.

The private Rig fixture independently matched the verifier's native PID to the
fixture-owned launch PID after exact-snapshot continuation, verified its original
conversation/plan mode and received another remembered-marker answer. The full
controller and manager lifecycle, checkpoint preflight and one-resume checks
passed again. Earlier failed receipts exposed tmux's need for an exact session
prefix ending in a colon and macOS's signed `nobody` UID; both were reproduced
and corrected without treating an unknown process as absent.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-3872e0c7-5f3b-4e14-9ef5-f5549d39be28/manifest.json`.
Executable SHA-256:
`fac5ec119a028a75727db452743bbc88b04b78540e8578cb6cfb8ea1735a2662`.
Qualification script SHA-256:
`666de45839b6f2afdc611097436ab3cc6fc8c213a1e41ccbcfdb4bbe38d0b1f9`.

Strict TypeScript, the standalone build, HTTP/coordinator Go tests and the
focused process/metadata tests passed. Rejection cases include foreign
conversations, competing Claude processes, changed cwd/executable/version,
cross-account descendants, changed arguments and a changed pane. The Windows
Go fixture fix passed hosted CI at `ad38816`; its remaining tooling failure was
a POSIX mode-bit assertion. That assertion now runs on macOS/Linux, with all
other metadata tests retained on Windows. Native Rig remains unsupported on
Windows.

This is the independent post-launch verification seam. The WebMux managed
action still needs durable original process/boot identity, survivor-first checks
and once-only execution around the qualified manager launch/reconciliation flow.

## Durable original Rig process evidence

With logging active, checkpoint capture now also records the original qualified
Claude process: pane/native PIDs, execution UID, UTC process start time, native
conversation/provenance and executable/settings hashes. A fingerprint of the
kernel boot-session UUID distinguishes host reboot from controller or manager
restart. macOS uses `kern.bootsessionuuid`; Linux uses the kernel boot ID. No
process argv, settings contents, executable contents or credentials enter the
stored process record.

The broker validates the bounded record, persists it with the existing
owner-scoped checkpoint, retains it through reconstruction and logging pause,
and advances the recovery revision when only process identity changes. Unknown
or malformed records are rejected. A failed process observation clears its
current proof while keeping valid checkpoint/native-file evidence available for
manual recovery; an old process record is not silently reused as current proof.

The qualified first-launch grammar differs from resume: Rig places the inline
settings before `--session-id` and `--name` when starting a fresh Claude seat.
Resume places the exact `--resume` ID and name before the final settings. Both
forms join the kernel-observed native ID to the manager snapshot and native file;
the initial process is not identified from a terminal title or enrollment ID.
Unsupported argument forms remain manual.

Real native qualification matched the captured original PID to the private
fixture's actual native launch PID, retained the same record across orderly
restart and SIGKILL, and passed the complete manager-loss/reconnect/fallback
matrix again. Provider-owned continuation from WebMux's recorded snapshot
returned a fresh remembered-marker answer, with one native resume and independent
post-launch executable/cwd/ID verification. All eight published-Rig browser tests
also passed. Host reboot and the WebMux Rig launch action remain open.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-37fb4186-832d-40d7-8586-0271673864c8/manifest.json`.
Executable SHA-256:
`e614bf3f0a951115bd828daabf6185bc0c4f8822f1e93ae0c45e5f703799ce54`.
Qualification script SHA-256:
`b9d0de0eb9daad39af86e71b1a44ea08ef79f748b4755c956b43d181d2176355`.

Full Go race tests/vet, strict TypeScript and the standalone build passed.
Metadata tests cover stable/change-sensitive boot fingerprints, explicit process
provenance, stale-proof removal and checkpoint retention. Broker tests cover
owner/logging boundaries, reconstruction, process-only revision invalidation and
malformed, foreign or unknown process records. The preceding published
application checkpoint `9bf603f` passed all hosted CI and Packaging checks,
including Windows tooling and Go; the new head requires its own hosted results.

## Rig survivor-first inspection and original-policy comparison

The embedded read-only inspector now joins the stored process/boot reference to
current manager and OS evidence. It recognizes the surviving original or an
already verified continuation without starting anything. A process still alive
outside its pane, competing process identity, an unknown/partial process census,
permission failure, changed native conversation or changed original settings
requires manual inspection. Confirmed pane/process absence is sampled again;
a new kernel boot is handled separately from PID reuse. Process arguments are
read only for the selected pane's execution-account-owned descendants.

Inline settings fingerprints now sort object keys while preserving array order.
The inspector compares the original settings with the resumed process, rather
than merely checking the settings supplied to an individual resume invocation.

That comparison exposed a fixture policy transition: the older native probe
started with Rig's inherited team default, then selected explicit plan mode.
Rig's source chooses different generated settings for those policies. The
initial team default includes its Bash `PreToolUse` command-convenience hook and
permission allow entries; explicit plan selection omits those generated fields.
Earlier evidence established per-invocation fixture retention and conversation
continuity, and did not establish first-launch inline-settings equality.

The updated probe separates these cases. Native resume keeps the team default
unless the drift test is requested; `--keep-team-default` makes that explicit. Rig retains
its original policy from the first launch and WebMux verifies the original
survivor, confirmed process loss and the resumed continuation with equal original
settings/executable fingerprints. Rig reports a successful restore and a fresh
native assistant response returns the remembered marker. With
`--expect-policy-drift`, the probe deliberately selects plan after first launch
and requires WebMux to reject the changed settings. That mode qualifies the
rejection boundary, not policy-preserving recovery. Tools remain denied in the
private fixture from its initial native launch.

The fixture serializes its added settings compactly, matching native Rig's
format; changing JSON whitespace had made the manager's strict process proof
ambiguous in earlier probes. No manager receipt is promoted to a stronger claim.
The verifier supports the published first-launch and resume argument orders,
including inherited `acceptEdits`, settings before resume and optional inline
settings. The selected owned pane and exact native ID bind legacy forms.

Positive unchanged-policy receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-6ff6841e-e88b-4d51-ac1f-0f550ffa18a6/manifest.json`.
Executable SHA-256:
`069c98f29908217e8395ff92691e81aaa9dd88c80af1794c1014b2b8332fa9cd`.
Qualification script SHA-256:
`cad7dd06e381970ba2aae45f8bdfd4e06a55f52120845a9727df6b42464c8f20`.
The changed-policy rejection receipt is
`~/.local/share/webmux/recovery-qualification/rig-native-82edc583-4fee-4ff5-b0f3-729f7f090f81/manifest.json`;
it explicitly records `original_settings_preserved: false` and an expected
rejection. Both keep private evidence and stop only their own fixtures.

Strict TypeScript, standalone build, HTTP/coordinator Go tests and the focused
metadata/process tests passed. The published `9de076a` application checkpoint
passed every hosted CI and Packaging check. WebMux's Rig launch action remains
disabled pending once-only execution integration and its lifecycle acceptance.

## WebMux Rig recovery action

The recovery preview now offers **Resume through Rig** for recorded local
Claude conversations with qualified process/checkpoint evidence. It requires
explicit approval and sends no terminal output or arbitrary command to Rig.
Current owner access and integration enablement are checked before preparation
and execution. Older checkpoint-only or unqualified records remain manual.

Preparation verifies the original destination, process state, native file,
checkpoint and policy. A surviving verified process returns fresh attachment
arguments. Process loss requires an unchanged qualified permission choice and
the supported native CLI version. The broker persists its requested intent
before calling Rig; the coordinator repeats preparation under its mutation lock
before issuing one exact-seat/snapshot launch. Changed or uncertain results are
not retried. Native attention-required results permit only no-launch
reconciliation after independent process proof.

Completion requires the original native ID, executable/settings fingerprints,
kernel cwd and permission mode. The viewer is resolved anew through Rig. The
broker consumes the request before attaching, so duplicate requests and restored
receipts cannot repeat the launch. The frontend displays the manager-specific
approval and keeps transcript consent separate.

Real native HTTP qualification passed against Rig `0.6.14 (9d0200f3)` and Claude
`2.1.322`, using the unchanged team-default policy. The action reattached the
original survivor, recovered after process loss, retained the original settings
and roots, and returned a fresh remembered-marker response. The same request
was repeated before and after controller SIGKILL/reconstruction without another
native process or resume execution. All earlier controller/manager lifecycle
and fallback checks passed in that run.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-afa3d9aa-3456-42f3-a7da-c943fab44fc5/manifest.json`.
Executable SHA-256:
`806223f92b82bfd354e91782195b56e453c71894f3ee806c4caf547d6eb3762d`.
Qualification script SHA-256:
`7f6a21f1c67c8b204a8fd07a681975c2b28649010384dd80845893a3a4cdf95e`.
Reproduce with `--native-resume --native-hooks --snapshot-plan --webmux-action`
and the existing synthetic conversation manifest. This qualifies the local
managed action; managed host reboot, remote destinations and additional native
policy/profile variations remain open.

Validation passed: full Go race suite and vet, strict TypeScript, lint,
production/standalone builds, 339 frontend tests, 49 helper tests and eight real
published-Rig browser tests. Broker coverage checks that Rig execution follows
durable intent and consumed receipts survive reconstruction; helper cases check
survivor reattachment, one launch, no-launch reconciliation, policy rejection and
post-launch uncertainty. Published checkpoint `d92c891` passed all hosted CI and
Packaging; the new head needs its own hosted results.

## Recovery interrupted before its native result

The native Rig harness now supports `--interrupt-action` with `--webmux-action`.
It gates only its private native launch, confirms that the exact request ID and
`outcome: requested` are on disk, and verifies that no native process has started.
It then SIGKILLs only the private WebMux controller before releasing the launch.
The request loses its response while the native manager completes one launch.

After controller reconstruction, the original request and a different UUID are
both rejected as uncertain. A second controller reconstruction still rejects a
retry. Explicit viewer reconnection reattaches the same native PID without
starting again. The resumed native conversation returns the original marker
with a fresh test phrase; original settings, executable, cwd and one-resume
checks pass. This tests an in-flight request, beyond a consumed receipt whose
response was already returned.

Private receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-c4b46e25-e799-4c26-bed0-0b27ea4751dc/manifest.json`.
Executable SHA-256:
`adaab277e3c614d77fa181b76d5eaf4a805626bdd1373a3e1d9a0738b133abd0`.
Qualification script SHA-256:
`6d8ff8f07c1d3925078ea9945d4bfe0f7d5c30fb34474b649674e234bc8c0d28`.
This checkpoint changes qualification only. It does not establish managed host
reboot or remote native recovery. The private launch gate has a deadline and is
released during fixture cleanup; installed services and other managers are not
controlled by this test.

## Managed shell ownership across an actual host reboot

`scripts/qualify-recovery-managed-vm.py` qualifies the real Agent Deck 1.16.34
manager with a synthetic Python shell job inside the disposable ARM64 QEMU
guest. It does not use a mocked manager or a coding CLI. Native conversational
continuation remains a separate requirement; this run proves manager ownership,
survival, retained history and safe fallback when the native process is gone.

The final 2026-11-10 run kept pane PID 2384 and execution count one through
orderly controller restart and SIGKILL. An actual guest reboot changed boot ID
from `2b583521-8778-47a2-874c-b1cd04f67910` to
`1f0eff88-92eb-4e51-969e-e3de1e6ce2a9`. WebMux then retained the original
manager target, project, lifecycle owner and all pre-reboot transcript references.
It reported `managed-unavailable`, with pending recovery and shell replay,
managed resume and ordinary CLI resume all disabled. Three reconnects left the
native pane absent and execution count at one. The history still contained the
original synthetic marker.

The harness verifies the private QEMU process, QMP state, pinned SSH fixture
account and guest marker before controlling services or rebooting. Each phase
verifies the private controller's actual executable through `/proc`. It disables
the guest's previous test service while using its port, then restores that
service and verifies it is active. Cleanup checks native pane absence before
accepting an already-stopped manager session. The final receipt has no cleanup
errors. Neither the workstation nor an installed shared service is controlled.

Receipt:
`_build/recovery-vm/qualification-441375b00aa1/managed-lifecycle-a9edfed7e14848f5891d94404bd05d5c.json`.
WebMux executable SHA-256:
`a0671b14624134fefa52f944e95460423f68b07ebc0f44e06441f35f7a4c076e`.
Qualification script SHA-256:
`f06e8d3653fb869efb9c96a586dd939b2a30cfe39716ab57534f2a7b88370909`.
The Linux ARM64 Agent Deck executable SHA-256 is
`bd65d34251f6f4ddf2816a58292c7c2711da29ef083974c7176148368adc60be`;
its release archive matched the published v1.16.34 checksum before installation
in the fixture. Python compilation and the complete native harness passed.

Reproduce with the running, retained disposable guest and its private
`qualification/agent-deck` executable:

```bash
python3 scripts/qualify-recovery-managed-vm.py \
  --vm-root /absolute/path/to/disposable-fixture \
  --binary /absolute/path/to/linux-arm64-webmux
```

The first attempt retained a failed receipt because both guest controllers
started after reboot and competed for the port. A later attempt completed every
recovery phase but flagged cleanup's expected manager-not-running response.
Those receipts remain negative evidence; the final harness corrects both fixture
errors. This does not establish coding CLI conversation continuity across a
managed host reboot, Rig host reboot, remote managed recovery or Windows native
execution. Those acceptance gaps remain open.

## Original Rig executable checked before native launch

Rig process capture now records the canonical OS executable path alongside its
SHA-256. Missing-process preparation verifies that this exact path still resolves
to itself and has the original bytes before running its version command or
requesting a native manager launch. It verifies the bytes again after version
inspection. Survivor and post-launch proof also retain the original path binding.
The manager still owns command selection; independent proof after launch remains
required, and an uncertain result is never retried.

Older process records without the executable path remain readable and retain
their transcripts, but no longer offer native start. Relative paths and control
characters are rejected by the persisted descriptor validator. The helper tests
cover unchanged success, missing/changed images, modification during version
inspection, and legacy records. Images rejected by the initial hash check are
not invoked. A fresh live
capture can supply the newly required path evidence without changing native
settings.

The real Rig 0.6.14 / Claude 2.1.322 WebMux-action harness passed with this change:
survivor reattachment, controller restart/SIGKILL, manager loss and recovery,
exact-snapshot native continuation, consumed-request deduplication after restart,
original settings/executable/project and a fresh remembered-marker answer.
Receipt:
`~/.local/share/webmux/recovery-qualification/rig-native-b0100c24-953b-4391-917b-752571801821/manifest.json`.
Executable SHA-256:
`2fd0d66a60e3546776f09c61f3abea473535ee5e63c6fe8ba18e1c5d03f21a13`.
Recovery helper source SHA-256:
`728f0e5ba6626af8620040a10e433693a43b218950ed1c8f82d58ffabab05440`.
Affected Go race suites and vet, strict TypeScript, the standalone build and all
50 helper tests passed. This run does not add managed native host-reboot or
remote-conversation evidence.

## Guest-native managed conversation across actual reboot

The managed VM harness accepts `--native-claude`. This mode uses Claude 2.1.322
installed and authenticated directly in the disposable guest. It creates a new
synthetic native conversation there, resumes it through the real Agent Deck
manager, and requires fresh remembered-marker responses before and after an
actual guest reboot. The continuation uses WebMux's managed action; duplicate
requests are checked before and after controller reconstruction. Native
executable, project, state-file reference, roots and explicit launch policy are
checked as well.

On 2026-11-10 this mode passed against the Linux arm64 build of `296931b`
(SHA-256 `9b224f52de62cda918e9b954835462c630b90e56193bf7fe8335b884d5cf4b9c`).
Agent Deck 1.16.34 (`bd65d342…`) owned the session; guest Claude 2.1.322
(`f220ca50…`) used its own claude.ai login, completed through the CLI's
paste-code flow. No host credentials were copied. Phases: running, orderly
restart, SIGKILL, actual kernel reboot (new boot ID), three repeated reconnects
with no launch, approved managed continuation, and the duplicate receipt across
controller reconstruction. Exactly two native launches occurred, both
`--resume <original id> --safe-mode --tools "" --permission-mode plan` in the
original project with identical roots. The post-reboot answer was a fresh
assistant record containing the original marker and a new nonce, written to the
original native state file. Receipt:
`_build/recovery-vm/qualification-441375b00aa1/managed-lifecycle-4b04f27c726542ed9955dc9ab946ee27.json`,
qualifier SHA-256 `7767eacd56aa4d7127c21ef158e6fcc24e3c485ec30173a4d2dac36cc47e7e40`.

Two earlier runs on the same build are retained as negative harness evidence,
not recovery failures:

- The first stopped at Claude's first-run "Security notes" screen, which the
  harness did not recognize. Its folder-trust handler also pressed Enter
  blindly although the native default is `No, exit`. The harness now dismisses
  the notes and selects trust by the rendered cursor position.
- The second passed every phase through the approved post-reboot continuation,
  but the final question was never recorded by the native client. Restored
  history repainted the marker before the composer accepted input, satisfying
  the old readiness check. The harness now requires an empty, stable composer
  and verifies submission against the native state file, resending only when
  the nonce is neither recorded nor visible (at most three attempts). The
  answer oracle is unchanged.

In the passing run the first post-reconstruction send was still not recorded
and a second send succeeded. That loss was [#325](https://github.com/jordanhubbard/webmux/issues/325),
explained in the next section.

This receipt qualifies one Linux guest, Agent Deck 1.16.34 and Claude 2.1.322
with this launch policy. It does not qualify remote managed hosts or other
native CLIs across a managed reboot. Rig across reboot is recorded below.

## Agent Deck attach input loss (#325)

Every `agent-deck session attach` (1.16.34, lipgloss v1.1.0 / termenv v0.16.0)
starts with a terminal background probe: an OSC 11 query followed by a
cursor-position request (`CSI 6 n`). termenv then waits up to five seconds for
a reply, reading stdin raw and discarding everything else, before the tmux
client starts. WebMux's browser terminal deliberately suppresses all terminal
query replies, so every Agent Deck attach stalled about five seconds and
silently dropped keystrokes typed in that window. This was not specific to
reboot or controller restart. The broker also reported `connected` at the
probe's first byte.

Isolation, using a private controller, Agent Deck home, tmux socket and loopback
port:

- With a raw byte logger in place of the attach command, WebMux delivered every
  input, including one sent 78 ms after a post-restart reattach.
- Under a bare PTY with no WebMux, Agent Deck dropped input typed before about
  +5 s. Plain `tmux attach` delivered immediately. Answering only the
  cursor-position request let tmux start in about 0.1 s with input delivered.

**Mitigation.** For a run whose argv is an Agent Deck attach
(`agent-deck [-p profile] session attach …` or `agent-deck remote attach …`), the
broker answers that startup probe once, within three seconds of launch, and only
when OSC 11 precedes the cursor request. The reply is `ESC[1;1R` with no colour
report, so termenv treats OSC 11 as unsupported and uses its default rather
than a background the user's terminal never reported. No other query is
answered and the frontend's suppression is unchanged.

With the mitigation, the real Agent Deck reproduction delivered every input,
including input sent immediately after a controller restart. Before, 7 of 9
were lost. Setting `TERM=tmux-256color` also avoids the probe, but it changes
tmux's outer-terminal features (OSC 52 clipboard, cursor style, focus, title)
and is not used. The upstream report is
[asheshgoplani/agent-deck#3105](https://github.com/asheshgoplani/agent-deck/issues/3105);
remove the mitigation once a fixed release is the minimum supported version.

Separately, the terminal WebSocket no longer discards input silently when a
session has no running process, is waiting for recovery or no longer exists.
The viewer receives an `input_error` notice, at most once per second, that is
shown in the terminal's input banner.

## Rig-owned native conversation across actual reboot

`scripts/qualify-recovery-rig-vm.py` runs the native Rig harness inside the
disposable ARM64 guest in two stages around an actual reboot:

- `qualify-recovery-rig.py --reboot-prepare` runs the existing local
  `--webmux-action` flow up to native process loss: Rig-managed first launch,
  survivor reattach through the WebMux action, controller restart/SIGKILL,
  reconnects with a fresh answer, private daemon loss and recovery, and the
  snapshot. It then leaves the native process, private daemon and a detached
  controller running and records `reboot-state.json`.
- The orchestrator reboots only the validated guest (new kernel boot ID).
- `--resume-after-reboot` requires that new boot ID and continues from the
  process-loss phase. It expects `managed-unavailable`, reconnect refusals, the
  read-only inspector's `{state: missing, reason: host-reboot}`, then the
  approved **Resume through Rig** action. It also checks the once-only receipt
  across controller SIGKILL, independent process/executable/ID/kernel-cwd proof,
  unchanged original settings and roots, and a fresh remembered-marker answer.

On 2026-11-10 this passed with Rig `0.6.14 (9d0200f3)` on Node 22.23.3 and the
guest's own Claude 2.1.322 login. No credentials were copied. Boot ID changed
from `db29c102-69d2-4798-8175-761ed9746204` to
`9c5a1f60-3fe5-492d-92f4-4b3c319d0e90`. The resumed process kept native ID
`9c65ec2c-8a7b-4772-bd24-1f20789e3f4a` with the team-default `acceptEdits`
policy, deny-all tools and original settings. Cleanup left no panes and a
cleanly stopped private daemon. Receipt:
`_build/recovery-vm/qualification-441375b00aa1/rig-reboot-3608924c40254614affdb81536786caa.json`
(guest manifest embedded). WebMux SHA-256
`0dbe95052a3c8cca11e657c31ffffbdaee5874d15a61a34b57476f7f15939495`;
in-guest harness SHA-256
`f79f5486abcdf70f485e5e30c1d0710b4c2b118447fa88a0c627e01abba5e323`.

**Qualified Rig build.** npm's `@openrig/cli@0.6.14` reports `df1c1517`, not
the qualified `9d0200f3`. The qualified build was a locally packed tarball from
the October relay rollout that no longer exists. The orchestrator installs the
installed release directory, which is portable: its only native module,
better-sqlite3, ships a `linux-arm64` prebuild. It verifies the exact version
string before use. A copy is kept at
`_build/recovery-qualification/openrig-cli-0.6.14-9d0200f3-release.tgz`.

**Fixture onboarding.** A first Rig launch in a never-trusted project stops at
Claude's workspace-trust prompt, and Rig reports `attention_required`. Only
when trust is the sole attention reason for the fixture's own seat does the
harness select "Yes, I trust this folder" by the rendered cursor, confirm the
prompt cleared, and run Rig's documented `rig seat continue`.

**Product fix found by this run.** `RestoreSessions` started coordinator
supervision in a goroutine and immediately restored managed viewers. When the
Rig daemon was down at startup (always, after a host reboot), restoration could
query Rig first, mark the viewer `managed-unavailable` and never retry, even
though supervision restarted the daemon moments later. The first `ensure` now
completes before restoration, within its existing 25-second bound; periodic
supervision continues afterwards. `rig daemon start` returns only once the
daemon is healthy (about 2.4 s in the guest). The regression test fails against
the previous ordering.

The earlier failed attempts are retained as negative evidence: an unhandled
trust prompt, then the restoration race. This qualifies Rig on one Linux guest
with this launch policy, not remote Rig destinations or other runtimes.

## Remote managed-host failure (2026-11-10)

**Scope (owner decision).** Remote managed sessions get qualified *fallback*,
not native resume. Native recovery stays local to the execution host's own
lifecycle manager. WebMux reattaches a remote session that is still running.
Otherwise it shows a clear manual-recovery state with the retained transcript,
and never starts the job itself. The remote topologies are:

| Topology | Lifecycle owner | Execution host |
| --- | --- | --- |
| Agent Deck remote (`agent-deck remote add`) | the remote host's Agent Deck | remote |
| Agent Deck `--ssh` session | local Agent Deck (pane runs `ssh`) | remote |
| WebMux-coordinated Rig seat on an SSH host | local Rig seat (pane runs `ssh -tt`) | remote |

**Product fixes found by this qualification.**

1. *Surviving remote sessions could not be reattached.* The survival check
   ran `agent-deck remote <name> session ownership inspect`, which Agent Deck
   1.16.34 rejects ("unsupported remote command"). After an ordinary WebMux
   restart, a healthy remote session was stuck in `managed-unavailable`, and
   reconnect returned 409 with that raw error. Remote targets now use the
   forwarded `remote <name> show` state (`running`, `waiting` or `idle` with a
   tmux session). This permits only reattachment: `remote attach` fails for a
   session that is not running and does not start it (verified against a
   stopped job). Native resume on a remote host is not offered.
2. *A down remote delayed or failed unrelated sessions.* Every Agent Deck
   survival check and attachment called the full inventory, which polls every
   registered remote for up to 12 s. Agent Deck forces `ConnectTimeout=10` and
   multiplexes connections, so a hung remote cost about 10–12 s per call. A
   local viewer's check plus attachment needed two such calls against a 20 s
   budget, so healthy local sessions could fail, and startup waited once per
   viewer. Lookups for one session now poll only its own remote (none for a
   local session). Startup waits for an unreachable remote manager once,
   however many viewers it owns.
3. *A refused remote looked empty.* Agent Deck 1.16.34's
   `remote sessions <name> --json` prints `[]` and exits 0 when the connection
   is refused, so the remote seemed to have no sessions. WebMux now uses
   `--json --with-errors`, whose envelope carries the SSH failure.
4. *Unclear messages.* An unreachable remote is reported as `Agent Deck
   remote "<name>" did not respond (its host may be down or unreachable)` with
   the reason, instead of "session is unavailable". A killed command reports
   the time limit instead of an empty message.

**Qualification.** `scripts/qualify-recovery-remote-managed.py` runs a private
controller and an isolated Agent Deck on the Mac (private HOME, XDG roots,
tmux socket and an `ssh` wrapper that reaches only the guest alias). It drives
three shell jobs that append to execution files: one owned by the guest's own
Agent Deck as a registered remote, one local `--ssh` job executing on the
guest, and one local control job.

On 2026-11-10 it passed with Agent Deck 1.16.34 on both hosts:

| Phase | Result |
| --- | --- |
| Healthy guest: controller restart and SIGKILL | All three reattached (startup 1.1–1.2 s) |
| Guest hung (QMP pause) | Startup 12.8 s total. Remote `managed-unavailable` naming remote `qa`; reconnect refused in 12.1 s; `--ssh` and local reattached. After resume the remote viewer reattached. |
| Actual guest reboot (boot ID `ae3341a4-be92-484f-98d4-65c24fb7c22f` → `0bc470ff-74a6-44c3-9f92-e4fedabdf590`) | Remote and `--ssh` `managed-unavailable` ("not confirmed running"); three reconnects each refused; remote manager reports `error`; no job process on the guest; local reattached |
| Guest powered off (connection refused) | Startup 0.7 s. Remote reports `Connection refused` by remote name; `--ssh` unavailable; local reattached |

Every phase kept one execution per job and the pre-interruption marker in each
transcript. Receipt:
`~/.local/share/webmux/recovery-qualification/remote-managed-52b9d683048d/receipt.json`.
WebMux SHA-256
`c7104726cbf4229b663a100845f3878457c669f527e57aeb4328f060b4887b3b`; harness SHA-256
`785de32b88df1ea0a1aa26eba3c3263b53b6fee1d881d9effcbd1f859892388d`. Earlier failed
runs are retained: a marker that scrolled off before attachment, a process
probe that matched its own shell, and the refused-connection finding (fix 3).

WebMux-coordinated Rig seats on an SSH host are qualified separately below.
Remote Agent Deck sessions running a coding CLI recover through Agent Deck on
that host.

## Rig seats on an SSH host (2026-11-10)

A Rig seat whose coding agent runs on another host has two parts: a local tmux
seat, enrolled in the local Rig daemon, whose pane runs `ssh -tt`, and a tmux
session on the execution host running the coding CLI. Losing the local seat
(SSH keepalive failure, reboot of the WebMux host) leaves the remote CLI
running. Rebooting the execution host ends both. WebMux never restarts the CLI
itself. To get back, you start the agent again from Coordination with the same
name, host, code folder and coding agent. That recreates the local seat. If the
CLI's remote tmux session still exists, the seat reattaches to it; if not, a new
conversation starts. Native resume of a remote Rig seat is not offered: native
Rig recovery covers localhost Claude seats only.

**Product fixes found by this qualification.**

1. *A lost seat still read as preserved.* The recovery message comes from the
   last restore. A viewer reattached at startup whose seat later ended (here,
   SSH gave up about 58 s into a guest hang) still said "Reattached to the
   surviving session. Its running work was preserved." The message now says the
   terminal has since ended whenever the viewer has no running process. This
   applies to every managed viewer.
2. *The refusal gave no way back.* Reconnecting a lost SSH-host seat answered
   "not confirmed running. Resume it through its lifecycle manager", although
   the CLI might still be running and Rig offers no resume for it. For Rig
   seats on a non-local host, the refusal and the startup fallback now name the
   host and give the explicit way back (start the agent again with the same
   settings), including that a new conversation starts if the CLI is gone.
3. *Unreachable hosts reported a command line.* Starting an agent on an
   unreachable host failed with the probe's whole `ssh` command. It now reports
   `Cannot reach <host> over SSH: <reason>. Nothing was started.`, or that the
   host lacks the workspace or the required CLIs.

**Qualification.** `scripts/qualify-recovery-rig-ssh.py` runs a private
controller, Rig daemon, tmux server (`TMUX_TMPDIR`) and HOME on the Mac, and an
`ssh` wrapper that reaches only the guest alias. It launches one Claude Code
seat on the guest through the WebMux API, using the guest's own Claude login.
It exposes the qualified Rig build as `~/.local/bin/rig` on the guest, which
WebMux requires on the execution host. It counts Claude processes whose working
directory is the fixture workspace.

On 2026-11-10 it passed with Rig 0.6.14 (9d0200f3) on both hosts and Claude
Code 2.1.322 on the guest:

| Phase | Result |
| --- | --- |
| Healthy guest: controller restart and SIGKILL | Reattached (startup 1.0–1.1 s); same CLI process |
| Guest hung (QMP pause) past the seat's SSH keepalive | Startup 1.0 s, reattached to the waiting seat. SSH gave up after 59 s; the message then said the terminal had ended. Reconnect refused in 0.4 s with the way back. After resume the CLI was still running; starting the agent again reattached the same viewer and the same CLI process |
| Local seat lost, guest untouched (as at a reboot of the WebMux host) | `managed-unavailable` naming the host; three reconnects refused; CLI untouched; starting the agent again reattached the same CLI process |
| Actual guest reboot (boot ID `038618ec-d48e-46f7-9949-86518399de5e` → `1e7a6ade-2e4b-4577-a7ee-055acb567712`) | Seat ended at once; no CLI or remote seat survived; `managed-unavailable` naming the host; three reconnects refused and started nothing; starting the agent again started exactly one new CLI in the same viewer |
| Guest powered off (connection refused) | Startup 0.6 s; `managed-unavailable`; reconnect refused; starting the agent again failed in 0.2 s with `Cannot reach … over SSH: … Connection refused. Nothing was started.` and created no seat |

Every phase kept the pre-interruption transcript. Receipt:
`~/.local/share/webmux/recovery-qualification/rig-ssh-1088fb7bbdc8/receipt.json`.
WebMux SHA-256
`5dfe152a13aca70a840810f0eb47f86cedae347d3e1d16c7cae783809a1ddb91`; harness SHA-256
`b97418a803fbbdd381cf415f456d018b3401605b8464837b28aac2ffdef8ed9c`. Earlier runs
are retained: one before fix 3, and one whose own check rejected the correct
"Retained logs are preserved" wording.

**Isolation correction (2026-11-11).** The runs above found `rig` on the
workstation's PATH, which was WebMux's installed Rig wrapper. That wrapper
pins `HOME` and `OPENRIG_HOME` to the installed OpenRig instance. The Rig
database stayed private (`--db`), but the daemon wrote its log, shutdown and
activity-endpoint files and the `wmqa*` project and transcript folders into
the installed instance. No other Rig daemon was running, and the installed
database was unchanged. The stray folders were moved to
`~/.local/share/webmux/recovery-qualification/rig-ssh-installed-instance-leak/`.
The harness now requires `--rig-cli` and `--rig-node`, runs that release
through its own wrapper, and checks that the daemon log lands in the private
`OPENRIG_HOME`. The behavior results stand; the receipts above predate the
fix.

### Codex seats on an SSH host (2026-11-11)

Codex seats differ in how they enrol. When the installed Rig supports it,
WebMux attaches them with Rig's relay identity (`--relay-ssh-host`,
`--relay-tmux-session`). The launcher first creates the remote tmux session
over a separate SSH call, then runs `ssh -tt` that only attaches to it. A
relaunch runs the same create-if-missing step, so it reattaches a surviving
Codex or starts one new conversation. Native resume of Codex is manual-only
(shared-daemon mode), so the fallback is the same as for Claude.

`scripts/qualify-recovery-rig-ssh.py --runtime codex` passed on 2026-11-11
with Rig 0.6.14 (9d0200f3) on both hosts and the Codex 0.167.0 release binary
(`codex-aarch64-unknown-linux-musl`, archive SHA-256
`843489acb4d4b1079745c36b03c5963b887b83155af208fd53584e33ee8eb281`) on the
guest, logged in to the guest's own ChatGPT account. The seat's launcher
used the relay-identity attach. It counts `codex` processes whose working
directory is the fixture workspace.

| Phase | Result |
| --- | --- |
| Healthy guest: controller restart and SIGKILL | Reattached (startup 1.6–1.7 s); same Codex process |
| Guest hung past the seat's SSH keepalive | Startup 1.5 s, reattached. SSH gave up after 59 s; the message then said the terminal had ended; reconnect refused with the way back. After resume, starting the agent again reattached the same viewer and the same Codex process |
| Local seat lost, guest untouched | `managed-unavailable` naming the host; three reconnects refused (0.8 s each); starting the agent again reattached the same Codex process |
| Actual guest reboot (boot ID `733efd68-de1d-47c8-b60d-707f26f29847` → `b78163b7-85e6-4047-af35-f6ae34b41049`) | No Codex or remote seat survived; three reconnects refused and started nothing; starting the agent again started exactly one new Codex in the same viewer |
| Guest powered off | Startup 0.9 s; `managed-unavailable`; reconnect refused; starting the agent again failed in 0.4 s with `Cannot reach … over SSH: … Connection refused. Nothing was started.` and created no seat |

Every phase kept the pre-interruption transcript, and the run used the
private Rig home (the installed OpenRig instance was unchanged). Receipt:
`~/.local/share/webmux/recovery-qualification/rig-ssh-bc479669baeb/receipt.json`.
WebMux SHA-256
`3787709fb680e33578e4dfe438e53737016ca68c0e4185e423173672c9747297` (main
`4a53ac7`); harness SHA-256
`90be82b41276197d23fc1ec1ecc5116f807078bfe4f64970b4799f87b398f2de`.

The same harness and binary then reran the Claude Code 2.1.322 seat with the
isolation fix: every phase passed again (startup 0.5–1.1 s, SSH gave up 57 s
into the hang, boot ID `54881e47-2f01-47ba-9331-7332acefacfe` →
`d8bf273e-fc96-4201-82f3-82c72559a24c`), with the installed OpenRig instance
unchanged. Receipt:
`~/.local/share/webmux/recovery-qualification/rig-ssh-50cc7ffc940b/receipt.json`.

## Sources

- [Official Codex command reference](https://learn.chatgpt.com/docs/developer-commands#codex-exec)
- [Claude Code command reference](https://code.claude.com/docs/en/commands)
- [Claude Code permissions and tool-name deny globs](https://code.claude.com/docs/en/permissions#tool-name-wildcards)
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference)
- [OpenCode CLI](https://opencode.ai/docs/cli/), checked against installed CLI help
- [Cursor CLI sessions](https://cursor.com/docs/cli/overview#sessions)
- [Mosh screen-state protocol](https://mosh.org/mosh-paper-draft.pdf)

- [Agent Deck ownership CLI source at the installed revision](https://github.com/asheshgoplani/agent-deck/blob/f36e77d6170b8d8b54f50c960c3f16e8a2c5df19/cmd/agent-deck/session_ownership_cmd.go)
