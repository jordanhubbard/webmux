# Agent Deck 1.16.39 qualification

Actual macOS ARM64 and Linux ARM64 qualification passed under #414. WebMux now
requires optional coordinator Agent Deck 1.16.39 or newer and removes the
broker’s startup-query reply exception. Structured native recovery remains
restricted to the qualified exact version 1.16.39.

A bounded preflight in the attachment child rejects older coordinators with an
update instruction while preserving the existing provider. It reads no terminal
input and keeps the actual attach environment. Version output is limited to
256 bytes and the version process group to three seconds. Only the diagnostic
version child uses TERM=dumb. Other broker jobs remain independent.

The release contains upstream agent-deck#3113
(`66190b4485025e51bb61b66cd6154b765a764a34`), which removes the CLI initialization
probe outside its TUI. Official archive checksums and staged binary digests are
in [sources.json](sources.json). The installed workstation coordinator was not
changed. Sanitized measurements, controller/coordinator/qualifier digests and
private-receipt hashes are in [native-results.json](native-results.json).

| Actual control | Result |
| --- | --- |
| Private Linux PTYs, early input at 0.3, 1 and 3 seconds | 1.16.34 lost all three early inputs, with OSC/DSR queries and a 5.027-second startup. 1.16.39 started in 0.0085 seconds without queries and delivered every early/late input exactly once through the production WebMux attach worker. Each trial retained the same payload; cleanup verified its exit. |
| Old-version refusal | The production worker rejected 1.16.34 in under one second without queries or writing provider input. Its existing payload remained available for the direct baseline trials. |
| Old record, new coordinator, native Claude on macOS | A record created by 1.16.34 continued under 1.16.39. Initial attachment, orderly restart, SIGKILL, repeated reconnects, native-process loss fallback and explicit once-only managed native recovery passed. |
| Native Claude after actual Linux guest reboot | The guarded controller retained the original conversation/project/roots/policy, obtained fresh remembered-marker answers before and after reboot, and retained the duplicate receipt through controller reconstruction. Original native configuration was preserved; cleanup reported no errors. |
| Local, SSH and remote managed shell jobs | Restart, SIGKILL, paused/resumed guest, actual reboot and powered-off guest passed. Hung-host startup took 12.8 seconds against the unchanged 40-second limit. Each job ran once; local work remained available and transcripts survived. |

The native CLI was Claude Code 2.1.322. These controls do not qualify every
coordinator release, every CLI, Windows managed providers or remote native
automatic recovery. Remote native resume remains with the execution host’s own
manager by owner decision. Broader #318 acceptance remains open.

The reboot fixture used a private copy-on-write overlay on the NAS with original
backing layers read-only, using the guest’s existing native sign-in directly.
Only this owned guest was paused, rebooted and powered off. No workstation reboot,
installed service replacement or authentication export was involved. Private
raw receipts and earlier failed attempts remain retained outside Git.

Reproduction uses `scripts/qualify-agent-deck-attach.py --webmux`,
`qualify-recovery-managed.py --bootstrap-agent-deck`,
`qualify-recovery-managed-vm.py --native-claude` and
`qualify-recovery-remote-managed.py`, with the explicit selected coordinator
paths/versions. The PTY control waits for the actual native tmux client before
sending late input: the old library restarts its five-second wait after each
byte, so an earlier late token would prolong its probe.
