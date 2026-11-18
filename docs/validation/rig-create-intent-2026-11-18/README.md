# Durable native member-add intent

`rig-create-intent.mts` retains a UUID, verified-manager identity supplied by its
caller, loopback port, canonical rig/pod route and exact request text before any
native mutation. Intent and consumed-dispatch records use exclusive 0600 files
under a private 0700 owner directory. File and directory fsync happen before
HTTP dispatch; incomplete records refuse without allocating a replacement UUID.
Changed policy bytes, whitespace/key order, manager, port or route refuse.

Ordinary duplicates return consumed/uncertain without another mutation POST.
Explicit inspection checks the saved dispatch authority and replays the same
native operation with the original route, headers and exact request text. Rig's
`operation-correlation.v1`/`member.add` capability is required. Inspection is a
correlated POST, not a read-only GET: if the original request never arrived, this
can be its first execution. A reservation with no IDs or an unverified/lost
response stays uncertain. A recorded enrollment is not readiness or liveness.

The [source-bound receipt](receipt.json) records two actual published Rig0.7.18
native macOS ARM64 cases. Separate Node client processes retain the saved UUID
and exact bytes; a duplicate does not POST; explicit inspection preserves the
original checked native occupant and pane PID. Changed bytes refuse before
POST. The same enrollment and sibling survive a private manager restart.
Checked Stop removes the selected terminal and preserves the sibling, then the
private daemon/tmux cleanup passes. The second case deliberately drops the real
native response after creation: the module remains uncertain/consumed and the
new client's explicit inspection recovers that original enrollment.

Ten journal controls cover separate client processes, concurrency, response
loss, drift, unsupported correlation, malformed/empty receipts, partial files,
missing dispatch authority and unsafe directory access. All 139 helper controls
and five strict TypeScript projects pass locally. Tests join ordinary helper CI;
POSIX cases are explicitly skipped on Windows, where the journal refuses.

This is the creation protocol foundation for #438, not activation of a native
creation path. The caller still must resolve and preserve the owner's provider,
profile/account, project, permissions and roots; verify the actual local manager;
and bind a stable logical request key. It must inspect the native enrollment and
canonical physical attachment before reporting success to a browser. Existing
adopted seats remain adopted. UI/native coding/remote-host qualification,
review, normal CI, audit and release gates remain required.
