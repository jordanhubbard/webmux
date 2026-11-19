# Recovery acceptance audit

This audit maps all seven acceptance criteria in
[#318](https://github.com/jordanhubbard/webmux/issues/318) to the implementation
and retained qualification documentation in canonical source
`31acdf9e1df38317d92006e547bdf27413faa824`. It is a source and documentation
review, not a new execution or independent re-audit of every private native
artifact. Named tests identify implementation controls; their presence is not
a claim that they were rerun here. Later commits and exact-main release gates
need their own checks.

**#318 remains open.** Historical native results and component passes do not
establish complete recovery acceptance for the current candidate. This audit
does not waive requirements or broaden supported cases.

## 1. Owner-scoped durable activity and native-session evidence

`internal/recovery/state.go` retains command/arguments, project, CLI version,
native ID/state location, configuration and provenance alongside transcript
references and durable action identity. `internal/session/recovery.go` joins
that state to the terminal's execution host, transport and lifecycle owner.
Source controls include `TestMetadataSignatureIsBoundToThePersistedTerminal`,
`TestNativeIdentityProvenanceIsAuthenticatedAndBounded` and
`TestRecoveryMetadataDoesNotRetainCredentialArguments`.

The [public native recovery](../codex-public-recovery-2026-11-18/README.md) and
[Windows functional upgrade](../windows-functional-upgrade-2026-11-18/README.md)
receipts document original conversation/project/configuration and history
bindings in their qualified cases. Additional provider, profile and platform
combinations are not inferred from them.

## 2. Reattach survivors first and avoid duplicate execution

Ordinary recovery binds the owner, preview revision, host revision, provider
instance and request body before dispatch. It persists intent before the host
action, retains uncertain outcomes and recognizes consumed duplicates in
`internal/session/recovery_control.go`. Ordinary recovery refuses to take over
managed paths. Source controls include
`TestBrokerRecoveryBindsPreviewOwnerAndConsumesOnlyOnce` and
`TestAutomaticRecoveryIsOptInAndDoesNotRetryAnUncertainAction`.

Native evidence includes the [lost creation response](../../session-recovery.md#lost-initial-native-creation-response-530),
[packaged service upgrade](../packaged-service-upgrade-2026-11-19/README.md) and
Windows functional upgrade. They have distinct source/lifecycle scopes; none
proves an arbitrary newer candidate's entire survivor/duplicate matrix.

## 3. Preserve logging and handle absent or partial history honestly

`internal/recovery/history.go` reads a bounded plain-text suffix and removes
terminal controls without modifying the original log. The recovery view keeps
metadata when a transcript is missing and distinguishes available history from
a reliably recorded command. Source controls cover pause acknowledgement and
restart, paused capture, malformed/truncated control packets, bounded UTF-8
history and history preceding a restored Windows viewport.

The Windows functional-upgrade receipt records 28 preserved prefix checks;
the packaged-service upgrade records retained log/native-file prefixes. These
are particular native lifecycle cases. They do not replace missing/rotated/
disabled-log and abrupt-termination controls on other supported paths.

## 4. Version-aware native recovery for the named coding CLIs

`internal/recovery/resume.go` checks the installed CLI's version/help and native
selector, validates the original project/native state and refuses unsupported
invocation restrictions. Selected transcript context is explicit, bounded and
converted to plain text; it is not a provider-native session file. See
[native conversation qualification](../../session-recovery.md#native-conversation-qualification)
for recorded Codex, Claude Code, Cursor agent and OpenCode versions and their
exact-conversation evidence.

Default shared-daemon Codex remains manual-only by owner decision. OpenCode
in-process switches remain an explicit choice where no supported observation
exists. The [interactive-policy guard](../codex-interactive-policy-2026-11-19/README.md)
refuses changed or unverifiable Rig/Codex settings rather than restoring a
broader recorded launch policy. Authenticated Linux recovery under the current
native-observation boundary remains under
[#523](https://github.com/jordanhubbard/webmux/issues/523). Unauthenticated
observer startup/policy checks do not satisfy it.

## 5. Preview, deliberate consent and existing access boundaries

The owner-scoped recovery view exposes host, project/activity, lifecycle owner,
history availability, warnings and allowed actions. Replay requires `Confirm`;
selected transcript context requires confirmation and a native action.
Automatic native recovery additionally requires opt-in, current process-state
evidence and no requested/uncertain prior action. Source controls include
`TestBrokerRecoveryDoesNotReplayOrShareWithoutConsent`,
`TestAutomaticRecoveryRequiresCurrentNativeIdentityEvidence` and
`TestUnrecordedInlinePolicyDoesNotOfferOrAutomaticallyRunNativeRecovery`.

The native policy guard and functional-upgrade receipts add real refusal and
original-policy cases. These do not establish every account/access boundary
or make an unsupported recovery format/provider supported.

## 6. Correct lifecycle owner, execution host and explicit fallback

The recovery view distinguishes WebMux, Rig and Agent Deck ownership. Native
preflight refuses missing CLI support, unavailable original projects and
missing/mismatched native state. The [Agent Deck qualification](../agent-deck-2026-11-17/README.md)
and [remote managed-host failure](../../session-recovery.md#remote-managed-host-failure-2026-11-10)
document native manager/host cases. Remote native continuation remains with
the execution host's manager by owner decision.

This review does not establish an exhaustive current-candidate/provider/platform
matrix for expired authentication, missing binaries and stale/deleted sessions.
Unsupported cases must preserve logs and explain manual recovery without
guessing identity or taking over lifecycle ownership.

## 7. Real restart, update, crash and host-reboot recovery

The [Windows installed-service case](../windows-installed-recovery-2026-11-17/README.md)
qualifies main `8f5d322`, ZIP/WinSW LocalSystem, Windows AMD64 and Codex 0.153.0
for crash/survivor recovery, actual guest reboot, original native continuation,
history prefixes and consumed duplicates across service restart. Its five
native responses requested no tools. It does not qualify an MSI or
different-executable service upgrade, other accounts or authenticated ARM64.

The later Windows user-run `8f5d322` to `5fadbbc` executable replacement retains
an active Python tool, then recovers the original conversation after provider
loss. It is not an installed-service upgrade. The macOS ARM64 packaged-service
`6156a5a` to `4b0d4b0` case qualifies original conversation and real policy/tool
checks under workspace-write/never and read-only/on-request, plus service
SIGKILL/automatic restart and durable original-request handling. It does not
qualify a notarized released-version migration or coding-host reboot.

Older real managed-conversation reboot evidence remains version-specific; see
[Rig](../../session-recovery.md#rig-owned-native-conversation-across-actual-reboot)
and [Agent Deck](../../session-recovery.md#guest-native-managed-conversation-across-actual-reboot).
Native-owned Rig 0.7.18/Codex 0.169.0 host-reboot recovery, additional supported
configurations and authenticated ARM64 coding recovery remain separate acceptance work. Windows
tool limitations in [#538](https://github.com/jordanhubbard/webmux/issues/538)
and the native VT deadline in [#536](https://github.com/jordanhubbard/webmux/issues/536)
also remain open. Native ARM64 component/capture passes cannot close these gaps.

## Remaining disposition

Review and integrate qualified candidates without promoting older receipts to
newer source. Complete the missing native/platform/policy cases or record an
explicit owner disposition that preserves truthful fallback. Verify the full
criteria and exact-main gates before closing #318 or publishing a release.
This audit makes no main, version, tag or release mutation.
