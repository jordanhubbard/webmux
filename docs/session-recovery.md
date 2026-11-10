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

Resume selectors use exact IDs, not `--last` or `--continue`. Prior approval or
sandbox bypass flags are not carried into a recovery command. CLI stdout stays
attached to the original TTY, and the wrapper must preserve interrupt behavior.
Native state availability, supported versions, original execution host and
project still require validation at the recovery action.

## Acceptance record

| Requirement | Current evidence | Remaining work |
| --- | --- | --- |
| Durable owner-scoped activity and log references | Broker tests persist before shutdown; native VM SIGKILL/reboot retain activity and original transcript references | Remote/managed lifecycle qualification |
| Real shell command and directory capture | Bash/Zsh PTY probes preserve exit status; real WebMux local workflow records a long-running command, arguments and directory | Retain the separate Zsh probe in the qualification harness |
| Native conversation identity | All four installed CLIs recover a synthetic marker after process exit and exact-ID resume; real native metadata preflight accepts the original project and rejects another project | Interactive session switches and complete WebMux continuation flow |
| Safe retained history | Bounded suffix, ANSI/OSC/C1 stripping, rotation and path controls; real local recovery retains the marker in API/UI history; redraw blank rows compacted only in the preview | Additional lifecycle/transport qualification |
| Surviving providers | Local, SSH and native mosh reconnect/restart/SIGKILL preserve provider and pane PIDs and job count; independent Linux user scopes avoid service-cgroup cleanup | Managed-provider qualification |
| Recovery preview and continuation | Owner-scoped API and browser actions; separate replay/handoff approval; broker tests for opt-in native auto resume and uncertain-outcome exclusion | Native CLI continuation through WebMux, managed lifecycle integration |
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

The native identity tests must also cover changing conversations inside one CLI
process. An initially selected conversation ID is not sufficient evidence after
the user switches sessions. Codex now rechecks its owning process's open native
state instead of permanently caching the first selector. Other adapters still
need their session-switch observations. Ambiguity requires an explicit fallback,
not a timestamp-based guess.

Remaining native integration includes observing active-session changes in each
interactive CLI, distinguishing a requested selector from verified active native
state, and verifying how each supported version restores invocation-specific
configuration roots, profiles and restrictive policy. Preserve those boundaries
when needed; never carry approval-bypass flags into a recovery command. The
complete WebMux continuation path still needs native conversation proof, beyond
the separate provider/headless and preflight checks.

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

The local VM runs and separate remote transport runs do not establish managed
provider or conversational CLI recovery through WebMux; those remain separate
acceptance work.

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

## Sources

- [Official Codex command reference](https://learn.chatgpt.com/docs/developer-commands#codex-exec)
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference)
- [OpenCode CLI](https://opencode.ai/docs/cli/), checked against installed CLI help
- [Cursor CLI sessions](https://cursor.com/docs/cli/overview#sessions)
- [Mosh screen-state protocol](https://mosh.org/mosh-paper-draft.pdf)
