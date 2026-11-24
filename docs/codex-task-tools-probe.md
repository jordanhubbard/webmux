# Codex task-tool readiness probe

`scripts/probe-codex-task-tools.mjs` checks one existing thread on a local macOS
Codex daemon. It is a prerequisite for the four-session survival qualification
in [#710](https://github.com/jordanhubbard/webmux/issues/710), not that complete
qualification. It never restarts a controller, viewer or daemon, resumes a
thread, starts a turn, changes configuration, or retries a failed tool call.

Use Node 24 with the locked `webmux` dependencies installed. Supply independently
identified native viewer and execution-daemon PIDs, the exact native conversation
UUID and original execution directory. `WM_PROOF_*` below are operator-supplied
values, not discovery by project label or timestamp.

```sh
node scripts/probe-codex-task-tools.mjs \
  --socket "$WM_PROOF_SOCKET" --thread "$WM_PROOF_THREAD" \
  --cwd "$WM_PROOF_PROJECT" --viewer-pid "$WM_PROOF_VIEWER_PID" \
  --daemon-pid "$WM_PROOF_DAEMON_PID" --output "$WM_PROOF_NEW_RECEIPT"
```

The probe requires an owner-only Unix socket and parent directory in the current
OS account. It verifies that the socket belongs to the specified native daemon,
checks the exact thread and canonical project, and identifies the registered
`codex_tui` loopback listener's process owner. A listener belonging to a different
viewer cannot qualify, even if that viewer can read the same project.

Cached MCP inventory saying `connected` is insufficient. The probe makes an
authenticated native `mcpServer/tool/call` of `codex_tui/read_thread`, using the
same thread as both the caller and read target, and checks the returned thread.
It then rechecks both process epochs. Interactive server requests are refused;
the probe never supplies an approval or extracts an MCP bearer credential.

An optional `--turn "$WM_PROOF_TURN"` additionally requires the same exact turn
to be `inProgress` before and after the probe. Omitting it explicitly leaves
`active_work_qualified` false. Completion or replacement of that turn fails this
particular in-progress-work check; it is not relabeled uninterrupted survival.

Receipts are new files created with mode 0600 and exclusive creation. Existing
failed receipts are not overwritten. They retain process fingerprints, native
IDs, a hash of the expected project and probe source, and fixed failure stages
and codes. Prompts, tool output, source paths, credentials, environment values
and raw transport error messages are not written to receipts or stdout. Tool
payloads are consumed in memory only; responses are bounded to 4 MiB.

`passed` means this one readiness check passed. Every receipt sets
`four_session_survival_qualified` to false. The complete gate still needs four
identified sessions, provider and execution epochs, ongoing-work witnesses, exact
conversations, and real management-tool calls across controlled controller
restart, crash and reconnect. It must refuse disruption when its baseline fails.
Legacy embedded clients, other execution hosts and non-macOS platforms require
separate adapters. Fixture tests of this probe are not native coding evidence.
