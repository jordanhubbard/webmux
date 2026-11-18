# Private tmux fixture receipt publication

Issue [#519](https://github.com/jordanhubbard/webmux/issues/519).
The test polled for a receipt pathname before reading its directory. A regular
write creates/truncates that pathname before writing its contents. An empty read
made the live-listener fixture bind `tmux-<uid>/default` relative to its cwd.
The actual wrapper correctly probed the distinct generated private directory,
received ENOENT, removed that absent-server directory and returned the child
status. The test then reported a missing expected cleanup rejection.

A controlled native open-before-write window reproduces that resolution with
an actual child exit0, failed shutdown-command exit1, probe ENOENT and a still-
bound relative listener with unchanged device/inode. An empty relative tmux
fixture directory in the original failing worktree is consistent with this
path. The original failure did not capture its probe event, so this is a
reproduced concrete fixture race, not a claim of complete historical tracing.
A separate 5,000-probe native listener control produced no false stopped result.

Publish both parent-polled string/JSON receipts by writing a sibling staging
file, closing it, then renaming it into place. The live-listener control now
holds the real creation/write boundary until the parent checks that the final
receipt remains invisible, then verifies the published absolute directory and
original live-socket inode. All existing shutdown, listener, unrelated-server,
child-exit, interruption, stale-socket and unexpected-error assertions remain.
The readiness and test deadlines, and the production helper bytes, are unchanged.
The barrier is fixture synchronization, not a production sleep or cleanup retry.

[Source-bound receipt](receipt.json): restoring only the original direct writer
fails the deterministic control, including in the full 129-test helper workload
(128 pass, one expected failure). The fixed source passes three complete helper
runs (129/129 each, no skips), 20 affected-suite runs (nine checks each), all five
strict TypeScript projects and whitespace checks. Every attempt is retained;
passing repeats do not erase the original or counterfactual failure.

This branch includes #518's independently qualified Ubuntu C-library baseline.
Hosted final-head checks remain required. Broader native creation/recovery #438,
security audit #429 and release acceptance remain separate; no main merge or
release is claimed.
