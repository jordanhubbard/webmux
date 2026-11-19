# Native two-seat grid placement

Refs #194, #318. Production input is main `25b28d35251e4094eff05f071aaffdee3749ed08`; only the appended acceptance case and documentation differ. The existing rebind case remains byte-for-byte unchanged.

The new native macOS ARM64 case starts two published Rig builtin shells, then uses the real empty-cell and navigator controls to open them at (0,1) and (1,0). An ordinary Node PTY occupies (0,0). After a page reload and explicit transport reconnect of each Rig viewer, both original viewer IDs, cells and complete Rig metadata match. Canonical seat sets and both original provider PIDs remain unchanged. Fresh split `printf` markers reach the actual terminal and retained history; the entire marker does not occur in the typed command. The ordinary terminal records exactly `xy`, with input before and after the Rig workflow and no acquired Rig identity.

Both maintained-file cases pass with no skips, retries or flaky outcomes. All five strict TypeScript projects pass. Actual execution uses Rig0.6.14, tmux, Node24 and Chrome158; this is builtin-shell/grid qualification, not authenticated coding-conversation, remote-host, host-reboot or complete #194/#318/#438 acceptance.

Two earlier test-authoring failures remain retained: an incorrect HTTP200 expectation for a documented201 response, and an overbroad API snapshot that compared dimensions/timestamps changed by legitimate terminal resize. The final assertion preserves the required stable IDs, cell and full Rig metadata; provider-PID checks remain separate. Product code, the original case and its deadlines were unchanged throughout.

Raw logs, receipts, bounded process observations, adapters, source-at-run snapshots and all three inactive private fixture homes remain retained. All observed fixture-tree PIDs exited, and the native daemon reports clean, complete shutdown without failures. The public receipt binds the actual final test bytes, native raw receipt and un-attested private executable. No hosted workflow was dispatched.
