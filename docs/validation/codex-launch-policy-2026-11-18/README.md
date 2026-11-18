# Native Codex launch policy preservation

Rig0.7.18 adds writable git metadata, seat and queue directories to an explicit
legacy workspace-write launch. Those additions change the owner's restriction.
The supported `launch_wrapper` path can remove them while keeping the native
Codex executable, process ID and Rig lifecycle identity.

The [policy transformer](../../../webmux/server/internal/openrigcoord/codex-launch-policy.mts)
checks a frozen native owner plan and qualified fresh/explicit-UUID resume shapes.
It requires matching sandbox, approval, project, writable roots, network and
temporary-directory choices; drift, duplicates and unknown options refuse.
Owner roots remain in the explicit native envelope. The fresh member contract
adds all `--add-dir` arguments itself, so the transformer removes them. It also
removes Rig's generated login-shell override and team developer instructions,
letting the owner's existing native settings apply. Model/effort/tier are bound
to the frozen observation even when Rig omits those flags. The operational
`--no-daemon` and disabled update check remain explicit so Rig manages the
qualified native process and executable.

The [wrapper](../../../webmux/server/internal/openrigcoord/codex-launch-wrapper.mts)
requires a canonical manifest file owned by the current user, mode0600, in an
owned mode0700 directory. It checks plan fingerprint, executable bytes, project,
native account-home reference and Rig environment identity before `execve`.
It preserves the inherited environment and replaces its own process with Codex;
it emits only a generic refusal on failure. A manifest is policy input, not
creation/recovery authorization. The caller still must authorize its owner,
verify the manager, durably publish the manifest and original operation intent,
revalidate the observation and inspect uncertain outcomes.

The [historical fresh-launch receipt](receipt.json), source `4995a07`, covers
actual Rig0.7.18, Codex0.169 and
Node24 on macOS ARM64. Two private fixtures project a documented native workspace
policy, reference the existing authenticated native account without copying
credentials, and launch the actual wrapper through native `member.add`.
Native conversation/tool records verify the exact policy and a fresh challenge,
two permitted writes (project and extra root) and five actual `EPERM` write
refusals covering the outside directory, `.git` and Rig-generated grants. All target parent
directories exist; a missing path does not count as a sandbox refusal.

Native network=false produces the CLI's truthful offline/partial coordinator
identity. Network=true resolves the full native seat identity and sibling roster;
the online response does not contain a `daemonReachable` boolean. Both cases
match the host manager's immutable selected node, keep the same occupant and pane
PID on original-operation replay, and pass checked Stop with the selected Codex
process gone and the sibling alive. Each private daemon/tmux server stops.
Native evidence uses correlated tool results and policy records, not model-only
claims or terminal screenshots. Failed and incomplete harness records remain
retained privately outside these positive cases.

Eight launch controls and all 166 local helper tests pass, along with five strict
TypeScript projects, 272 installed lock identities and whitespace checks. These
controls include policy drift, fingerprint/manifest/executable changes, model
choices and a real subprocess proving PID/environment preservation.

Current source also accepts an explicit native conversation UUID on resume,
including Rig's `-m` alias and omitted cwd flag. The wrapper pins the original
cwd and model choices before that subcommand; picker/name/last/fork/prompt and
drifting arguments refuse. The caller/manager must authorize the original UUID.
See [current native recovery evidence](../codex-native-resume-2026-11-18/README.md)
for provider-loss continuation, original log append, repeated policy execution,
duplicate-request controls and a missing-conversation refusal.

This remains unactivated. Native UI/creation conversion, manifest persistence,
authorization and uncertainty handling must compose with #504/#508. Native
selected-profile selection, Linux/Windows and host reboot remain unqualified.
Acceptance covers explicit workspace policies, not a selected/default owner
workspace configuration. Read-only/full-access and non-default service-tier
execution have unit coverage only. Existing adopted seats gain no native Stop
authority. The counts above describe the historical fresh-launch baseline;
current local validation is 168 helpers and ten launch controls.
