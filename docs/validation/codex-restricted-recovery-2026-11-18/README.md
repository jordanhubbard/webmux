# Native Rig/Codex recovery with restricted owner policies

A later [project-trust qualification](../codex-project-trust-2026-11-18/README.md)
adds typed refusal and frozen original-request retry in the real public app,
including native trust review with an unchanged read-only project configuration
and loss of an approved public retry response. This earlier receipt retains its
own trust setup, failed probes and source binding.

The actual Go application and Chrome pass public original-conversation recovery
on macOS with two native Codex owner policies. The server is a clean build of the
source revision in [receipt.json](receipt.json), using Rig 0.7.18, Codex 0.169.0
and Node 24. The browser is private loopback auth-none; the native account remains
in its original home and no authentication files are copied.

| Native owner policy | Approval | Allowed tool writes | Denied tool writes | Tool network |
| --- | --- | --- | --- | --- |
| Workspace-write | on-request | Project and configured extra root | Outside roots, .git, .codex, excluded owner TMPDIR | Denied |
| Read-only | on-request | None | All six project/extra/outside/git/config/TMPDIR targets | Denied |

Both cases record the exact native policy in every coding turn context and in
the immutable owner launch plan. Each performs fresh correlated native tool
probes before and after recovery, retaining the original model, HOME/tmp/PATH,
paired Rig/tool discovery and inherited values while excluding manager-only
values. Denied operations return EPERM; allowed files contain the probe's
expected bytes and denied files remain absent after cleanup. A separate
unsandboxed connection proves the fixture's local network endpoint is available.
The restricted tool's network connection returns EPERM.

The network-denied tool does not call Rig's HTTP identity endpoint. Instead,
its native OPENRIG_NODE_ID joins the original node already validated by the
public launch/capture/recovery path. This is a different probe from the
network-capable default-owner qualification; no network exception is added.

Trust is reviewed through the native interactive CLI without submitting a model
turn. The read-only preparation first uses a workspace-write configuration in
the owned private project to expose the native trust choice, exits that CLI,
then restores the final read-only project configuration before managed creation.
That configuration's hash, actual read-only plan and every read-only coding turn
are verified. No trust record is manually written and no native guard is relaxed.
An untrusted-project attempt separately returns native_project_trust_required
before an owner launch record or native member POST. The native updater is
skipped when offered; Codex remains at the qualified version. Earlier readiness
probe failures remain retained and excluded. One completed browser run failed
an offline verifier that expected an explicit false network field in read-only
journal entries. Codex omits that default there. The corrected verifier requires
the exact recorded format, a network-disabled resolved plan and live network
denial; a fresh complete run confirms the result. The earlier failed raw receipt
is unchanged and its exact driver source is retained by hash.

For each policy, actual native creation handles a discarded member reply through
explicit inspection of the original operation; ordinary and ready duplicates
post nothing. A graceful controller restart retains the original provider/pane,
and the public survivor recovery path sends no native Resume. The qualifier then
kills only its owned Codex process and explicitly confirms checked Stop of the
idle original seat while its sibling survives.

Public original-only recovery banks the approved request, then the actual
controller is SIGKILLed just before the native POST. Reconstruction preserves
uncertainty and the original request. An ordinary duplicate posts nothing;
browser Check-and-complete retains the same native UUID/body/revision until
confirmed. The recovered native conversation recalls the removed challenge
without tools and then repeats the fresh policy/environment probes. Native
history and the same terminal-log paths retain their byte-identical prefixes
and append. Consumed duplicates, viewer/sibling retention, final checked Stop
and private daemon/tmux cleanup pass.

This extends the earlier
[default-owner public recovery](../codex-public-recovery-2026-11-18/README.md).
The separate [installed-service upgrade](../codex-service-upgrade-2026-11-18/README.md)
keeps its default full-access policy scope; this receipt does not qualify a
restricted-policy service upgrade or a host reboot. Other policy variants,
interactive escalation/permission changes, selected profiles/permission graphs,
broader startup options and platform combinations remain open. Native trust
refusal guidance is tracked in [#529](https://github.com/jordanhubbard/webmux/issues/529). Required review/current-head CI and
complete #318/#438/release acceptance remain separate. Private drivers, raw
records and source archives are retained by hash; account details and model
turns are excluded from these public files.
