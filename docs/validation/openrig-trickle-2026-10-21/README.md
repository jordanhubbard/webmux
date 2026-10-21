# Production Trickle coding workload — 2026-10-21

Two real Codex agents were created through the authenticated production WebMux
Coordination dialog: `coder` on the local macOS server and `reviewer` on a remote
Linux execution host. Both appeared in ordinary terminal tiles. The owner
completed provider folder-trust prompts through those tiles and submitted the
coding tasks through their browser terminal inputs.

The workload uses the active
[Trickle GitHub repository](https://github.com/NVIDIA-Omniverse/trickle), not its
retired GitLab checkout. The agents investigated the upcoming release's
[CI-009 Windows Qt blocker](https://github.com/NVIDIA-Omniverse/trickle/issues/81).
The coder worked in an isolated worktree based on PR #82 and pushed
`43aed2f6c2b25cc722d19bde1d33465329050575`, opening
[draft PR #88](https://github.com/NVIDIA-Omniverse/trickle/pull/88). It preserves
partial QTest assertions when a suite times out, with missing, empty, partial
and passing-only timeout controls. The reviewer independently ran 35 non-GPU
Qt cases on Linux and exchanged code-level findings and review instructions.
The final pushed revision is `3de6a353aa338358b7ff09a9fa0aeeade452f51e`,
including the supported refresh of its changed CMake input fingerprints. The
reviewer independently checked the final delta and fingerprints, timeout driver,
19 CI contracts and 15 roadmap tests, and sent a verdict of no blocking source
findings within the diagnostic scope. That verdict explicitly excludes merge
and release readiness.

The [message record](messages.json) retains both directions of actual Rig
communication, bound to the central daemon's `transport:v1` provenance. Messages
were sent by the real agents with `rig send`, without sender overrides, direct
HTTP delivery, shared peer files or injected peer terminal input. The UI's
`pending` outbox state is retained verbatim; it is not treated as proof of
receipt. Reciprocal responses and the independently produced code/review work
establish that the agents consumed each other's messages.

The [receipt](receipt.json) records the production HTTP result, managed agent
identities and ordinary-grid session IDs. Browser refresh and **Open terminal**
reused the sessions. After installing the tested development candidate and
restarting the existing macOS LaunchAgent, the WebMux PID changed while both
central pane PIDs, the remote provider pane PID and grid session IDs remained
unchanged. The authenticated browser reattached and displayed the coordination
messages after restart.

The remote SSH relay was then deliberately stopped and recreated through
**Reconnect agent**. Its central pane PID changed while the existing remote
provider pane PID and reverse-forward port stayed unchanged. The remote agent
sent a fresh `RECONNECTED` nonce through Rig and received the local coder's
matching ACK. Both agents remained available in the production grid afterward.

## Repeatable browser gate

`openrig.spec.ts` uses the published OpenRig 0.6.14 CLI, actual daemon and tmux
transport in an isolated home/server. Two interactive provider stand-ins enroll,
exchange a fresh challenge and independently generated ACK, expose attributed
messages in the dialog, and survive refresh and tile close/reopen. The optional
dashboard also connects, accepts input, persists across refresh and detaches
without stopping the daemon. All three tests passed locally. Linux CI runs the
same gate with the published CLI pinned to 0.6.14.

This test exposed a launcher PATH bug: prepending common tool locations could
select another Rig executable after project creation. Preserving configured
PATH precedence fixes enrollment and keeps the isolated fixture on its own
coordinator. Launchers explicitly bind the service PATH even when the existing
tmux server retains an older environment. The test starts such a server before
installing its wrappers and checks that project shutdown preserves an unrelated
session. The fixture preserves enrolled sender/transport variables and clears
inherited routing once before starting its private daemon.

## Boundaries

The live session qualifies Codex-to-Codex coding and review. Claude Code and
native Windows coordination hosting are not qualified. Provider folder trust
and sign-in remain explicit provider interactions; a tmux session alone does not
prove readiness. The initial local provider exited at its trust prompt; the
owner reconnected it and completed trust before any workload success was claimed.

Trickle PR #88 is a scoped draft improvement, not a completed release repair.
The Windows startup failure, fresh Windows execution, full hosted CI and stale
Trickle lifecycle evidence remain separate gates. No release was published or
release disposition changed by this workload.
