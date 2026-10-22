# Patched OpenRig relay qualification — 2026-10-21

The actual WebMux coordination runner used an isolated patched OpenRig daemon
and a private local tmux server. Production was not restarted or reconfigured.

| Check | Actual result |
|---|---|
| macOS SSH loopback (alias rocky resolves to Puck), Codex 0.163.0 | Verified remote process; `RELAY_MAC_OK` reply |
| Separate Linux host sparky, Codex 0.162.0 | Verified remote process; `RELAY_LINUX_OK` reply |
| Agent-to-agent transport | Both real agents exchanged attributed READY messages |
| Private coordinator restart | Binding retained; actual macOS `RELAY_RESTART_OK` reply |
| Remote Linux Codex replaced with shell, local SSH still live | Identity became mismatch; send exited 2 and marker was absent from the pane |

Existing provider permissions and trust settings were used unchanged. Hook and
origin-provenance warnings remained visible; those subsystems are not qualified
by this receipt. Only test-owned sessions and the private daemon were stopped.

Initial failures were retained: a too-long macOS Unix socket path, followed by
the launcher's mistaken assumption that physical session and sender address were
equal. The latter caused a real launcher exit before SSH and is fixed by using
the attach response. The private directory was shortened for the socket limit.

The runner controls additionally cover local Codex, remote Claude, the new relay
interface, and an older coordinator lacking the relay options. These controls
are synthetic orchestration tests; the table above records actual provider use.

Local artifacts: `/private/tmp/rig-relay-f8i41u_z` (private launch, message,
negative-input and restart receipts). No provider secrets or activity tokens are
included in this public record. The OpenRig fork retains matching source hashes
in the local receipt. Hosted CI is separately tracked on the pull requests.
