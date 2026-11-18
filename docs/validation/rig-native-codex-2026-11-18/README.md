# Authenticated native Codex creation contract

Published Rig 0.7.18 (3bf87c62), Node 24.21.0 and tmux 3.8 launched the actual
Codex CLI 0.169.0 in a private macOS ARM64 project through supported HTTP
`member.add`. The fixture deliberately selected `codex_permissions`
`{ sandbox: read-only, approval: never }` and referenced the existing local
Codex home. It did not read, copy, print or forward credentials.

The [receipt](receipt.json) binds the private probe/report and native session
bytes. The actual native turn contexts report read-only/never and the original
fixture directory. The native tool records corroborate both commands: reading a
fresh random challenge exits 0 with its exact value; writing the disposable
file exits 1 with `Operation not permitted`. The file does not exist afterward.
This goes beyond the earlier synthetic argv captures and provider assertions.

Before member-add dispatch, the fixture saved its operation UUID and exact
request text. Replaying that UUID and body returns the native replay receipt
without replacing the checked node/session occupant or its tmux pane PID.
The sibling retains its original PID. Checked native Stop returns success for the selected coding node/session
while the sibling survives; the selected coding PID was not separately sampled
after Stop. The private Rig daemon and tmux server are stopped during cleanup.

A preliminary attempt had a fixture PATH/tmux-target error and is excluded from
positive evidence. The corrected attempt required one explicit later Enter to
submit the staged prompt after native startup; no second provider was launched
for that input. A reusable harness must wait for readiness and separate pasted
text from Enter. The native generated-identity probe also reports an unreachable
daemon from inside its tool sandbox. This receipt does not qualify coordination
network access, every policy/profile/account combination or an owner-policy
resolver. It does not prove native CLI process-loss or host-reboot recovery.

#438 remains open: implement the WebMux native creation path with resolved owner
intent, durably retain its original UUID/request before dispatch, and inspect
that operation after uncertainty. Existing adopted seats stay adopted. Qualify
the actual browser/controller path and supported remote ownership semantics.
Normal review, CI, audit and release gates remain required.

## Explicit network-enabled read-only contract

A second actual authenticated fixture selects the supported granular envelope
`{ filesystem: { "/": "read" }, network_access: true, approval: "never" }`.
The [separate receipt](network-coordination.json) records the native policy and
tool evidence. Three native turn contexts retain read-only filesystem access,
enabled network and never-ask approval. Actual native commands read the fresh
challenge, receive a refused write, query the expected coordinator identity and
confirm its reachable status/rig ID. After stopping/restarting only the private
manager, exact operation replay retains the original node/session occupant and
pane PID. The same native conversation queries that coordinator again and
returns the remembered challenge without another file read in that turn.
This is survivor continuity; it does not exercise provider loss or a host reboot.

Independent native `codex sandbox` controls use a private loopback HTTP server.
With networking disabled, reads succeed, writes fail and loopback access fails
with EPERM. With networking enabled, reads succeed and the loopback request
returns HTTP 200 while writes still fail. These controls establish the network
boundary, not every cause of an unreachable coordinator. They do not authorize
silently enabling an owner's disabled network policy. The first control attempt
failed CLI admission because it omitted required `--permission-profile`; the
corrected command supplies it and the failed receipt remains private.

The second interactive fixture's terminal-screen assertion fails because the
TUI collapses the first answer, so the challenge is not visible twice. That
full fixture remains failed; no status is overwritten. The separate native
session verifier confirms the narrower policy/tool/conversation facts using
native JSONL, with no additional provider launch. Its first admission correctly
refused a parser expectation that `rig whoami` includes rigId; the published
reply instead supplies rigName/nodeId. The verifier now binds that identity to
the reachable `rig status` response's canonical rig ID. This adjustment does
not relax the policy or tool-evidence requirements.

`python3 scripts/verify-native-codex-launch.py --help` describes its scoped
artifact inputs. It reads one explicitly selected native session and prints
only summary facts/hashes; it neither runs recorded commands nor prints their
arbitrary contents. Ten controls reject model-only claims, widened/changed
policy or project, wrong/partial/unreachable coordinator identity, failed reads,
successful writes, duplicate sessions and uncorrelated/duplicate tool output.
They run in ordinary Frontend and Tooling CI. The verifier does not by itself
prove authentication, process identity or that a manager restart occurred.
The source-bound native fixture supplies those separately scoped observations.
