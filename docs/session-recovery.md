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

## Native adapters

- Codex: identify the rollout file opened by the owning process or its children,
  validate its `session_meta` project and ID, and select `codex resume <id>`.
  Concurrent candidates remain ambiguous; timestamps do not select a winner.
- Claude Code: preserve an explicitly selected ID, or allocate an exact native
  session ID using the installed CLI's supported `--session-id` option.
- Cursor Agent: preserve an explicit ID, or use `agent create-chat` and select
  that native ID when starting the interactive client.
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
| Durable owner-scoped activity and log references | Broker tests read persisted evidence before graceful shutdown; race detector passes | Native abrupt-server-termination coverage |
| Real shell command and directory capture | Bash and Zsh PTY probes record commands and preserve exit status | Retain the native probe in the qualification harness and exercise through WebMux |
| Native conversation identity | All four installed CLIs recover a synthetic marker after process exit and exact-ID resume; real native metadata preflight accepts the original project and rejects another project | Interactive session switches and complete WebMux continuation flow |
| Safe retained history | Bounded suffix, ANSI/OSC stripping, rotation, traversal and symlink controls; plain-text Recover dialog | Lifecycle qualification of prior scrollback restoration |
| Surviving providers | Native tmux test preserves the provider PID and serializes concurrent provisioning | Connect startup and transport control; remote SSH/mosh and managed-provider recovery |
| Recovery preview and continuation | Owner-scoped read endpoint and explicit Recover dialog with host, project, observation, CLI identity and retained output | Revision-bound broker action, browser action controls, consent and lifecycle integration |
| Reboot/restart/update/crash | No new end-to-end qualification claimed | Disposable native host reboot, update and abrupt termination; duplicate-execution controls |

Mosh synchronizes terminal screen state rather than an arbitrary byte stream.
Its recovery metadata path needs separate native qualification and must not
repurpose clipboard operations as a metadata channel. Pause behavior and
execution-host ownership apply to that path as well.

## Continuation control

Recovery actions must not be submitted as typed terminal input. A prompt-ready
observation can become stale while a user edits a partial command or starts an
application. A revision check alone cannot make that input safe. The unpublished
typed-input prototype was removed; there is no recovery-action HTTP endpoint in
this checkpoint.

The execution-host control worker now exists. Provider startup, broker actions
and browser continuation controls still need to be connected to it before users
can recover work. The browser can already preview recorded work through the
explicit Recover control, including when a terminal is disconnected.
Its fixed `--recovery-rpc` helper reads one bounded JSON request from stdin;
authentication keys and action payloads never travel as typed terminal input.
The RPC `ensure` operation provisions a stable tmux provider through a private
stdin script. Native tmux tests confirm that concurrent reconnects create one
provider, a surviving process keeps its PID, and a lost provider with recorded
activity receives the startup wait gate. The SSH control client uses a fixed
remote helper command, configured account/key/port and a bounded JSON pipe;
its implementation is present, but remote lifecycle qualification remains open.

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

Control tests exercise the real Unix socket, persisted consumption across a
worker's exit, stale and foreign requests, failed preflight, a revision change
during preflight, independent transcript/replay consent and one-time retrieval
of an approved replay script by its startup shell. A subprocess with its own
controlling PTY verifies that complete and partial input typed while waiting
cannot reach the reader started after recovery. These are worker/lifecycle
checks, not evidence of conversational CLI recovery or host-reboot acceptance.

Qualification will use a disposable native VM for host reboot. Do not reboot
the user's workstation or existing shared workers. The actual resume acceptance
needs a synthetic conversation marker recovered by each named CLI, beyond
empty-session allocation/export or mocked adapter assertions.

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
