# Rig grid integration candidate

This candidate reconciles eight pending PRs under #194. Main and the release
remain unchanged. It is an integration checkpoint, not closure of the full
roadmap, #194 or native recovery #318.

| Parent PR | Included work | Qualified parent head |
| --- | --- | --- |
| #432 | Configured coordinator scope and external-host provenance | 279db1e |
| #433 | Explicit private tmux mouse configuration | 6fe00dc |
| #435 | Observed tmux provenance and Agents navigation | 6d6181d |
| #437 | Stable canonical Rig seat identity | e6aa2db |
| #440 | Canonical lifecycle/bootstrap/activity in tiles | 14827c7 |
| #441 | Rig enrollment origin separate from WebMux records | cd32c86 |
| #444 | Bounded local notifications and canonical invalidation | c7d349c |
| #446 | Verified persistent USB fixture source cache | 91db99f |

The original commits remain in history. One workflow conflict was resolved by
retaining all six explicit native invocations: legacy Rig, host, identity,
activity, notifications and independent Agents coexistence. The private mouse
fixture is now tracked in this candidate, so final positive native runs require
no fixture or runtime overlays. The source-cache step remains manifest-keyed and
checksum-verifying; no native admission rule or audit gate changes.

## Interaction fix and new acceptance

The earlier Agents case checked that a real Rig daemon was reachable, but had no
active Rig seat. Its extended case creates a real builtin-terminal seat through
the published manager and a separate observed Node terminal on its own tmux
socket. Both are present while the user navigates between Agents, Coordination
and the grid, reloads and opens/closes a scratch shell. The observed viewer never
acquires Rig identity. The canonical seat retains its own ID, viewer, (1,1)
position and provider PID. It remains Rig-launched with no WebMux launch record.
After the WebMux viewers close, both provider PIDs still survive until the
test's explicit teardown. These providers retain separate ownership; no manager bridge is created.

That interaction exposed a visibility bug: the terminal grid stays mounted behind
other panes, and Rig observation continued there. Observation now follows the
existing pane context. Hiding the grid cancels its wait and canonical polling;
returning requests fresh observations while preserving the mounted terminal.
Permission, provider enablement, expiry and non-overlap rules remain intact.

A frontend control verifies abort/resume without replacing the terminal DOM or
requesting session deletion. The same typed test against the old Workspace
component fails at the missing abort. This is an explicit component overlay for
the unit negative only, restored and SHA256-checked; no native-parent result is
claimed. See [the negative binding](hidden-grid-negative.json).

## Final local evidence

The [native integration receipt](native-integration.json) records thirteen passing
cases across six actual browser files. The [active-seat coexistence receipt](native-agents-coexistence.json)
records retained observed PID/viewer, active canonical Rig seat and independent
identity. Notification coverage pauses fallback timers and observes a real daemon
change driving canonical tile state; later subscriptions cancel/reuse cleanly.
The eight-case legacy workflow and separate host/identity/activity cases also
pass. Execution uses the published Rig, actual tmux, Node and native Chrome on
macOS ARM64, with unrelated CLI host selection inherited by the application.
Builtin terminals and the observed Node fixture are not authenticated coding
providers. This does not qualify remote notifications, adoption, Stop-seat
ownership, arbitrary manager builds or the #246 event bridge.

Final frontend validation passes 398 tests in 41 files, strict TypeScript, lint
and build. The assembled backend passes coordination/HTTP race suites and Darwin
and Windows AMD64 root vet. All 92 helper tests pass. USB source acquisition and
admission/boundary tests pass (8 each), with 3 apt-policy controls. The final
source/receipt bindings are in [the manifest](source-manifest.json). The native
runner's base commit predates the final UI/test edits; the manifest binds the
actual tested file bytes rather than presenting that base as the final source.

The [parent warm-cache proof](parent-cache-warm.json) belongs to exact 91db99f:
a fresh hosted job restores all eight files, rechecks them in three builds with
zero downloads, and retains four expected native USB outcomes. The integrated
cache helper/verifier/pins are byte-identical; this parent proof does not replace
required hosted qualification of the combined candidate.

The initial Python command used an incorrect working directory and did not run;
it is excluded and the corrected checks pass. Strict checking also caught an
incomplete test response fixture; it was filled with the full contract, not
suppressed. Historical qualification documents remain bound to their old sources.

## Remaining gates

Current-head hosted CI, review, audit #429 and exact-main qualification remain
required. Picomatch 4.0.7 still has no patched published successor at this check.
The separate #442 investigation reproduces a Windows first-navigation abort;
its experimental startup/transport diagnostics and test-only listener are not
included here, and this candidate does not claim that abort fixed. The separate
#428 terminal-restoration candidate is also outside this assembly. Broader
#194/#318/#438 and #246 acceptance remains open. No merge or release is made.
