# Native creation environment and helper-loss handoff

The native creation factory now preserves the owner's execution environment
without putting arbitrary environment values in its durable state or Rig's
member request. The private manifest stores standard path/locale values and
hashes of other owner values. Before dispatch, the explicit creation process
opens a mode0600 Unix socket in a canonical owner-private mode0700 runtime
directory. The wrapper obtains the other values over that socket, verifies
their hashes and acknowledges receipt. The sender durably records only the
environment fingerprint before allowing execution. The socket is once-only,
bounded to 45 seconds and removed during cleanup; it is not a background
service. A handoff receipt is not proof that Codex is ready or has started.

The wrapper reconstructs the owner environment, removes manager-only values
and parent conversation/shell bookkeeping, and retains fresh Rig and terminal
bindings. Its PATH retains the owner's exact PATH after Rig's verified paired
CLI directory. That directory must contain only the expected `rig` symlink,
with a native-format path hash and matching owner. This preserves coordination
through the selected manager rather than selecting an unrelated global Rig.
The Codex executable, project, model and permission checks still apply.

Native testing found that the seat environment omitted many owner values.
Requiring those values to survive the manager was therefore insufficient.
The socket supplies the original values from the creation process's memory;
it stores no credential values in the launch manifest, journal, handoff receipt
or member metadata. The existing native account home remains a reference. No
authentication file is copied. If the environment changes before inspection,
inspection refuses instead of silently rebuilding the original intent.

The [receipt](receipt.json) binds actual authenticated Codex 0.169.0, Rig 0.7.18
and Node 24 execution on macOS ARM64 to the current sources. After observing
the original enrollment through the supported native nodes API, the fixture
SIGKILLs only its creation helper, before the handoff completes. An ordinary
duplicate remains consumed and posts nothing. Explicit inspection reclaims
the stale owned socket and reuses the original native operation and node ID.
The original pending wrapper can wait up to 30 seconds for that same handoff;
it does not restart the provider, replay another launch or accept a changed
environment. One authenticated provider then executes a fresh coding challenge.

Correlated native tools verify owner HOME and TMPDIR, the full owner PATH
suffix, paired `rig whoami`, an owner-only executable, delivery of an arbitrary
fixture value omitted from the incoming seat environment, and absence of a manager-only value.
The owner and manager have different path, home and temporary-directory values.
Native Codex adds its own tool prefix to PATH after the wrapper's execve; the
native tool check preserves the full owner suffix rather than assuming Rig is
the only prefix. The owner has default full access with approval `never`, and
four actual permitted writes confirm that policy is not narrowed. Duplicates
retain the same native occupant. Checked Stop ends only that coding seat;
the original sibling survives. The socket disappears, the fixture value does
not occur in durable service state, and private manager/tmux cleanup passes.

Local validation covers 191 helpers, including cross-process execve and
handoff tests, once-only delivery, expiry, collision/refusal, stale-socket
reclamation after an owning-process crash and bounded receiver waiting. Five
strict TypeScript projects, 272 installed package identities, coordinator Go
race/vet and whitespace pass. An unchanged private-tmux cleanup test failed in
one full run; its isolated repeat and subsequent complete runs passed without
altering that gate. The earlier failed native candidates and diagnostic-only
wire alteration are excluded from positive evidence.

This is an internal factory/wrapper checkpoint. Public HTTP/UI creation,
durable new-topology allocation, uncertain-request controls and native recovery
composition remain incomplete. A recovery caller must establish a fresh
handoff before asking the manager to launch a lost provider; an old consumption
receipt does not authorize another provider launch. The helper SIGKILL case
does not qualify a WebMux controller crash, host reboot or full browser flow.
Read-only/workspace enforcement, selected profiles, named permission graphs,
other platforms and broader settings remain separate qualification work.
Native observation still has the documented concurrent-MCP-startup limitation.
Review, exact-head CI, audit #429 and the full roadmap/release gates remain open.
