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
| Native conversation identity | Adapter selector/ownership tests; installed OpenCode 1.19.5 allocation/export passes | Real conversational resume for Codex, Claude Code, OpenCode and Cursor |
| Safe retained history | Bounded suffix, ANSI/OSC stripping, rotation, traversal and symlink controls | Automatic prior scrollback presentation in the recovery UI |
| Surviving providers | Existing local tmux reattachment remains the baseline | Provider survival observations, remote SSH/mosh and managed-provider recovery |
| Recovery preview and continuation | Owner-scoped read endpoint | UI, revision-bound action, native validation, command replay consent and selected transcript handoff |
| Reboot/restart/update/crash | No new end-to-end qualification claimed | Disposable native host reboot, update and abrupt termination; duplicate-execution controls |

Mosh synchronizes terminal screen state rather than an arbitrary byte stream.
Its recovery metadata path needs separate native qualification and must not
repurpose clipboard operations as a metadata channel. Pause behavior and
execution-host ownership apply to that path as well.

## Sources

- [Official Codex command reference](https://learn.chatgpt.com/docs/developer-commands#codex-exec)
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference)
- [OpenCode CLI](https://opencode.ai/docs/cli/), checked against installed CLI help
- [Cursor CLI sessions](https://cursor.com/docs/cli/overview#sessions)
- [Mosh screen-state protocol](https://mosh.org/mosh-paper-draft.pdf)
