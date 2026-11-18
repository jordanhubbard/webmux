# Durable native member-add intent

`rig-create-intent.mts` retains a UUID, verified-manager identity supplied by its
caller, loopback port, canonical rig/pod route and exact request text before any
native mutation. Intent and consumed-dispatch records use exclusive 0600 files
under a private 0700 owner directory. File, journal-directory and journal-parent fsync happen before
HTTP dispatch; incomplete records refuse without allocating a replacement UUID.
Changed policy bytes, whitespace/key order, manager, port or route refuse.

Ordinary duplicates return consumed/uncertain without another mutation POST.
Explicit inspection checks the saved dispatch authority and replays the same
native operation with the original route, headers and exact request text. Rig's
`operation-correlation.v1`/`member.add` capability is required. Inspection is a
correlated POST, not a read-only GET: if the original request never arrived, this
can be its first execution. A reservation with no IDs or an unverified/lost
response stays uncertain. A recorded enrollment is not readiness or liveness.

The [source-bound receipt](receipt.json) records three actual published Rig0.7.18
native macOS ARM64 cases. Separate Node client processes retain the saved UUID
and exact bytes; a duplicate does not POST; explicit inspection preserves the
original checked native occupant and pane PID. Changed bytes refuse before
POST. The same enrollment and sibling survive a private manager restart.
Checked Stop removes the selected terminal and preserves the sibling, then the
private daemon/tmux cleanup passes. The second case deliberately drops the real
native response after creation: the module remains uncertain/consumed and the
new client's explicit inspection recovers that original enrollment.

Eleven journal controls cover separate client processes, concurrency, response
loss, drift, unsupported correlation, malformed/empty receipts, partial files,
missing dispatch authority and unsafe directory access. All 144 helper controls
and five strict TypeScript projects pass locally. Tests join ordinary helper CI;
POSIX cases are explicitly skipped on Windows, where the journal refuses.

This is the creation protocol foundation for #438, not activation of a native
creation path. The caller still must resolve and preserve the owner's provider,
profile/account, project, permissions and roots; verify the actual local manager;
and bind a stable logical request key. It must inspect the native enrollment and
canonical physical attachment before reporting success to a browser. Existing
adopted seats remain adopted. UI/native coding/remote-host qualification,
review, normal CI, audit and release gates remain required.

The first two native receipts bind the prior module. Review found that flushing
entries inside a new journal did not flush the journal name in its parent. The
current module flushes that parent entry before any native POST, including when
a previous process left an existing directory behind. An isolated child-process
fsync fault shows the parent dispatched once despite the injected parent error;
the correction refuses before any POST for both new and preexisting journals.
The third native receipt binds this corrected source and repeats actual response
loss, separate-client duplicate/inspection, manager restart and cleanup controls.
These are process/durability protocol checks, not a physical power-cut test.

## Verified native manager binding

The read-only `reviewRigCreateManager` check reuses the qualified version,
loopback listener, OS owner, executable/entry-point and process-start checks.
The published health response's selfHostId supplies the native host identity.
All three observations must agree on it. The durable hash binds that identity,
owner and qualified build; a separate process fingerprint detects a process
change during observation. No private database or credential file is read.
The Stop response projects its original fields and retains its original
process/occupant checks; creation identity does not authorize Stop.

The [manager-binding receipt](manager-binding.json) binds an actual published
Rig daemon before and after its private restart. Its PID/process fingerprint
change, while the verified durable identity stays equal. That identity is used
by the journal fixture: response loss stays consumed, a new client's duplicate
makes no POST, exact inspection keeps the same native occupant/pane PID,
manager restart preserves it, checked Stop removes the selected terminal and
keeps its sibling, and private cleanup passes.

Four new controls reject absent/malformed self identity, foreign/unverified
listeners or proxies, process changes and self identity changes during an
observation; they also verify stable binding across a simulated process restart.
All 24 manager/Stop controls and 144 total helpers pass. Actual acceptance here
is macOS ARM64; the new manager binding still needs native Linux qualification.
This is a caller building block, not WebMux native-create activation, caller
permission, an atomic mutation precondition or provider readiness evidence.
