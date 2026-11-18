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
