# WebMux active roadmap

Reviewed 2026-11-19 against the implementation, qualification receipts and GitHub
issue states. GitHub issues track implementation and remaining acceptance work.

## Automatic session coordination and network discovery

Owner requirement added 2026-11-21. These are planned capabilities; the current
inventory, terminal messaging and optional integrations do not establish them.
Every session running under WebMux should participate in coordination without
per-session opt-in, including agents independently started on different WebMux
nodes. WebMux should discover other WebMux nodes on the local network through
mDNS/DNS-SD rather than requiring their addresses to be entered individually.

Agent Deck and Rig were selected without verified evidence that they satisfy
this goal. Their selection is open for reconsideration: retain, replace or
remove either integration according to demonstrated requirements and native
acceptance results. Neither tool, nor their proposed bridge under
[#246](https://github.com/jordanhubbard/webmux/issues/246), is a prerequisite by
definition. Existing provider documentation records current behavior, not a
commitment to use those providers for this work.

Ordered implementation and acceptance:

- [ ] **Evaluate the coordination architecture** under
  [#630](https://github.com/jordanhubbard/webmux/issues/630). Explain what Agent
  Deck and Rig actually contribute, inspect and test their relevant interfaces,
  and compare them with a built-in approach. Record capabilities, gaps,
  supported runtimes/platforms, maintenance and deployment costs, and the
  evidence-backed selection. Preserve existing agents and their lifecycle
  ownership during any migration.
- [ ] **Discover network nodes** under
  [#631](https://github.com/jordanhubbard/webmux/issues/631). Advertise and browse
  WebMux services through mDNS/DNS-SD, reconcile discovered and configured peers
  by verified node identity, and handle address changes, multiple interfaces,
  duplicate names and departing nodes. Provide server-wide policy controls and
  a manual-peer fallback when multicast is unavailable. Discovering a node does
  not authorize it: retain authenticated enrollment and expose no credentials
  in advertisements. This is separate from known `.local` hostname resolution
  completed under #208.
- [ ] **Coordinate sessions automatically** under
  [#630](https://github.com/jordanhubbard/webmux/issues/630). Provision agent
  awareness and an agent-facing coordination interface for ordinary and managed
  sessions across authorized nodes, without special launch forms or manual
  session setup. Support project/peer discovery, task/branch/file ownership,
  overlap notifications, acknowledged handoffs and shared heavy-worker
  reservations. Preserve server/account/session identity and use verified
  repository identity or explicit mappings across worktrees and hosts. Keep
  coordination active without an open browser, retain state through reconnects
  and restart, and distinguish message admission, delivery, acknowledgment and
  completion. Do not inject unsolicited text into arbitrary terminal prompts.
- [ ] **Qualify the complete workflow** under #630/#631 with independently
  started real agents on two real WebMux hosts: discover nodes, authenticate
  enrollment, discover project peers without per-session opt-in, detect an
  overlapping claim, acknowledge a handoff and honor a shared worker reservation.
  Cover ordinary and managed sessions, restart/reconnect, stale identities,
  unrelated projects, foreign accounts, untrusted advertisements, unavailable
  multicast/peers and duplicate or uncertain delivery. Publish supported
  boundaries and evidence before marking either requirement complete.

The [architecture decision](coordination-architecture.md) selects a built-in
coordination plane with optional existing provider lifecycle adapters; real
agent-facing channel evaluation remains open. The
[node discovery implementation checkpoint](network-discovery.md) adds a server-owned DNS-SD candidate browser and server-management visibility. It keeps
announcements untrusted and retains manual enrollment. Authenticated HTTPS node/
account identity and address migration now have a native macOS/Linux component
result; complete discovered two-host acceptance and automatic coordination remain
open; this checkpoint does not complete any of the requirements above. The
[built-in coordination checkpoint](session-coordination.md) adds a durable
node-local ledger and scoped ordinary-session CLI. Real-agent onboarding,
managed/SSH interfaces and cross-node coordination still require implementation
and acceptance. The native MCP extension now attaches startup guidance through
qualified ordinary Codex/Claude aliases; [single-host native startup evidence](validation/agent-coordination-2026-11-21/README.md)
does not complete the cross-host or managed-agent workflow.

## Current reliability work

**First priority: [#318](https://github.com/jordanhubbard/webmux/issues/318).**
Session-state recovery has an implementation checkpoint in
[#322](https://github.com/jordanhubbard/webmux/pull/322). The owner requested that
this checkpoint enter `main` for fix-forward work in a Claude session. This is
an integration handoff, not completed acceptance: #318 remains open and first
priority ahead of the other reliability and integration follow-ups.

### Integrated implementation and remaining qualification

[#452](https://github.com/jordanhubbard/webmux/issues/452) tracks the independent
underline/snapshot profile, Rig/Agents integration, owning-profile routing and
Windows Chromium AppContainer installation repair. The implementation stack,
including ordinary-local Windows Stop/Hide and the required hosted application
gate, entered `main` through [#568](https://github.com/jordanhubbard/webmux/pull/568).
Its qualified input `0056000` passed all ten original workflows; that evidence
retains its own source and platform limits. The later native ARM64 application
gate [#571](https://github.com/jordanhubbard/webmux/pull/571), Windows diagnostic
input retention [#575](https://github.com/jordanhubbard/webmux/pull/575), and browser
refresh race repair [#574](https://github.com/jordanhubbard/webmux/pull/574) also
merged after their original checks passed.

Each later main revision still needs its own applicable checks and package
reconciliation. [#429](https://github.com/jordanhubbard/webmux/issues/429) retains
the bounded runtime/build-input review and final-source reconciliation; its
generic matcher limitations remain. The [source bindings and scope](validation/roadmap-integration-2026-11-17/README.md)
preserve the separate #318, #438 and #246 acceptance gaps. Integration does not
complete those items or the overall roadmap and does not cut a release.

The integrated fixture follow-ups #535, #524 and #525 and USB follow-ups #505
and #459 have completed their scoped acceptance audits and are closed. Their
[acceptance reconciliation](validation/scoped-acceptance-2026-11-19/README.md)
binds the original failures, controls and native results to the relevant current
sources. This does not qualify the whole current main revision or the release;
the changed-source private Windows VT timeout under #536 remains unresolved. A
separate [scheduler-trace diagnostic](validation/vt-scheduler-clock-2026-11-19/README.md)
retains the failure and exposes a decoded-trace clock discrepancy against its
embedded monotonic/wall snapshots. Clock interpretation and the deadline cause
remain open; no runtime or test-budget workaround is applied.

The [native-platform follow-up audit](validation/native-platform-acceptance-2026-11-19/README.md)
also completes #543's Windows connected-shell fixture, #466's regular-file
inventory admission and #517's frozen Ubuntu qualification-image libc minimum.
Their broader recovery, vulnerability/license and deployment boundaries remain.

The [relay provisioning audit](validation/relay-provisioning-acceptance-2026-11-19/README.md)
reconciles #545/#546 timeout evidence and #549 signed HTTPS fixture provisioning
with the retained full native relay and Linux APT controls. #550's changed cache
helper now has complete guest qualification, including all 20 direct cycles and
WebMux-over-SSH. The [held-counter correction](validation/usb-held-counter-2026-11-19/README.md)
under #615 independently reproduces and repairs a counter gap for native replies
queued before the test hold. The complete six-control candidate succeeds, with
the legacy counter failing its specific causal control and the corrected count
matching the actual queue. All source pins, original I/O/admissions/deadlines
remain intact. The earlier uninstrumented timeout retains its failed outcome;
its exact interleaving was not recorded. Local guest execution uses no hosted
identity. Broader physical-device and release acceptance remain separate.

The [Mosh control-channel audit](validation/mosh-acceptance-2026-11-19/README.md)
completes #468 automatic launch/acknowledgement and #561 provisioning-evidence
acceptance. Twenty source-bound native samples and current protocol/ownership
controls pass. Historical provisioning and cleanup failures retain their original
outcomes; broader external-provider and physical-device #110 acceptance stays open.

The [current small-screen native audit](validation/browser-mobile-current-2026-11-19/README.md)
completes #454 with all four original browser regressions plus page-text typing
and pane-layout controls. Portrait, landscape and reduced-height views pass on
native macOS/Chrome; physical mobile keyboard, Safari, assistive and external
provider/device #110 qualification remain open.

Windows private browser cleanup #473 also has completed
[native acceptance](validation/browser-process-exit-2026-11-18/README.md#integrated-acceptance--2026-11-19):
verified owned-child exit precedes one-shot private-file removal. Original
cleanup and trace-helper failures remain retained; broader navigation #282 and
recovery/release acceptance remain open.

Federated renderer capability forwarding #484 has completed its
[native acceptance audit](validation/federation-profile-acceptance-2026-11-19/README.md),
including credential/query isolation and a profile-requiring original ConPTY
owner. The separate #478 qualification follow-up is now closed after the
[native visibility-oracle audit](validation/federation-lifecycle-2026-11-18/README.md#integrated-acceptance--2026-11-19).
That audit preserves uncertainty about the geometry-free historical occurrence;
broader recovery and release acceptance remain open.

The [current Rig routing qualification](validation/rig-routing-current-2026-11-19/README.md)
keeps #464/#469 open: the original legacy 13-case workload passes, while modern
Rig passes 12 cases and refuses synthetic Codex in its tracked native-creation
path. #603 tracks the remaining modern workload. The response observer now
reveals the actual refusal without changing the original success assertion;
separate authenticated native receipts retain their own scope.

The original [native viewer/loss record](validation/rig-native-viewer-loss-2026-11-19/README.md)
verifies real coding-conversation continuity through reload and native viewer
navigation; its initial attachment-loss gate remains recorded as failed.
The [qualified native recovery follow-up](validation/native-recovery-pending-2026-11-19/README.md)
now passes both original-conversation loss cycles, no-tool canary recall,
unchanged sibling identity and the same retained transcript path/prefixes.
PR #612 closes #608 and the discovered log-continuity defect #614. The original
10-second observer, six-Check limit, 45-second budget and earlier failed reports
remain unchanged. Peer-message acceptance, full #603/#464/#469, broader
#318/#438 lifecycle recovery and release acceptance remain open.

The separate [native controller-reconstruction qualification](validation/native-recovery-controller-crash-2026-11-19/README.md)
completes #617: actual SIGKILL after the original request is banked but before
native dispatch, after environment handoff but before confirmation, and after
consumption. Both loss cycles recover the same original conversation/request and
retained log; pending duplicates stay pending and consumed replay stays read-only.
This scoped macOS acceptance does not complete #318/#438 or release qualification.

The [observer integration audit](validation/observer-acceptance-2026-11-19/README.md)
verifies integrated bounded diagnostics and reproduced signal-I/O/notification
repairs. Both retained native architectures and fresh local ARM64 controls pass.
#533 remains open because the exact original stderr-less exit cause is unproven;
authenticated Linux coding/recovery remains open under #523.

### Recover active work after interruption

[#318](https://github.com/jordanhubbard/webmux/issues/318) adds session-state
recovery to the active reliability scope. After a host reboot, server update,
restart or crash, WebMux should help the user continue the work they were doing,
beyond reopening windows at their last SSH/mosh or managed-agent connection.

With session logging enabled, retain the pre-interruption transcript and record
reliable last-command, execution-host, working-directory and agent-session
metadata. Reattach surviving processes first. For lost coding CLI processes
(Codex, Claude Code, OpenCode, Cursor `agent` and other popular tools), use
version-qualified native resume mechanisms and their original session IDs/state.
Where supported, offer the previous WebMux log as additional recovery context;
a terminal transcript is not a replacement for a CLI's native session format.

Show the proposed recovery and its evidence, preserve ownership and lifecycle
boundaries, avoid duplicate jobs, and require deliberate approval for arbitrary
command replay or external transcript handoff. Handle disabled, missing, rotated
or partial logs and unavailable hosts with clear fallback choices. Acceptance
requires real restart/update/crash and host-reboot recovery, plus native coding
CLI resume evidence; reopening a window or replaying its scrollback is not
proof of restored agent state. The implementation is available for fix-forward;
full qualification remains open.

Current evidence and limits:

| Area | Progress |
| --- | --- |
| Recovery metadata and controls | Implemented: owner-scoped durable activity/native-session evidence, retained history, recovery previews, explicit replay/context consent, survivor checks and durable duplicate/uncertain-request handling. |
| Named native CLIs | Real exact-conversation continuation passed for qualified Codex, Claude Code, Cursor agent and OpenCode versions, with fresh remembered-marker responses and original project/configuration checks. In-process conversation switches are followed for Codex (direct process), Claude (scoped hooks) and Cursor (open chat store); OpenCode 1.19.5 exposes no selection to observe, so its recovery stays an explicit choice ([anomalyco/opencode#54329](https://github.com/anomalyco/opencode/issues/54329)). This does not qualify every invocation or platform. |
| Ordinary local/SSH/mosh lifecycle | Real controller restart/update/SIGKILL and disposable guest reboot checks passed, including retained history and no duplicate execution. |
| Local managed recovery | Real Agent Deck/Claude and Rig/Claude process-loss continuation passed. Rig additionally passed original executable/settings checks and an in-flight controller-loss test that refused unsafe retries. |
| Native Rig/Codex public recovery | Passed on macOS with Rig 0.7.18/Codex 0.169.0 and the default owner policy: actual coding, graceful controller restart with unchanged provider, owned CLI loss plus explicit idle-seat checked Stop, public original-only Resume, controller SIGKILL during dispatch, exact-conversation no-tool recall, retained native/terminal log prefixes and duplicate/sibling controls ([qualification](validation/codex-public-recovery-2026-11-18/README.md)). Public restart/crash recovery also passes with native read-only/on-request and workspace-write/on-request policies, fresh allowed/denied filesystem and denied-network probes, and retained owner environment ([restricted-policy qualification](validation/codex-restricted-recovery-2026-11-18/README.md)). The same public lifecycle now also passes workspace-write/never and read-only/never with fresh before/after policy probes and 32 lifecycle controls ([never-approval qualification](validation/codex-never-recovery-2026-11-18/README.md)). A separate real launchd/Make functional executable upgrade passes survivor reattachment and original recovery after service SIGKILL/automatic restart with the default policy ([service qualification](validation/codex-service-upgrade-2026-11-18/README.md)). An interactive permission change now refuses structured native Resume before dispatch, including a switch without another model turn; surviving original processes still reattach with their current permissions ([native qualification](validation/codex-interactive-policy-2026-11-19/README.md)). The implementation is integrated through #568; native Rig/Codex host reboot, other policy/platform combinations and release-package migration remain open. |
| Native Windows Codex executable upgrade | Private user-run AMD64/Codex 0.153.0 active tool survives main8f-to-5fad controller replacement with unchanged process identities; public original-conversation Resume after provider loss, fresh filesystem restrictions, retained prefixes and durable duplicate/sibling controls pass ([qualification](validation/windows-functional-upgrade-2026-11-18/README.md)). Local Stop/hide now passes scoped [native application qualification](validation/windows-local-stop-2026-11-18/README.md) and is integrated through #568; #541 retains its remaining acceptance audit. Legacy default-shell/network limitations (#538), installed-package upgrades and authenticated ARM64 remain open. |
| Linux native Codex startup inspection | AMD64/ARM64 kernel controls and twelve disposable native startup cases pass with the #533 signal-pair repair: no panic/forced cleanup, unchanged policy snapshots and retained network/process denials ([qualification](validation/codex-observer-signal-2026-11-18/README.md)). This does not qualify authenticated Linux coding/conversation recovery under #523. The startup inspection implementation is integrated through #568. |
| Native Codex project trust | Implemented and qualified through actual Go/Chrome/native Codex: typed pre-launch refusal, durable owner-scoped selection, manual native trust review with unchanged intended policy, deliberate frozen retry, and uncertainty after a discarded public retry response. Ordinary duplicates/status refresh do not re-observe the account or launch automatically; changed selections fail before topology allocation ([qualification](validation/codex-project-trust-2026-11-18/README.md)). #529 implementation is integrated through #568. Initial creation-response loss has separate [native qualification](validation/codex-initial-response-loss-2026-11-18/README.md): never-received request, lost actual native/public response, frozen original continuation, no-observer/no-POST duplicates, controller reconstruction and subsequent exact-conversation recovery. #530 implementation is integrated through #568. |
| Rig conversation after reboot | Passed on a Linux guest with Rig 0.6.14 (9d0200f3)/Claude 2.1.322: host-reboot detection, approved Resume through Rig, once-only receipt, original settings/roots and a fresh remembered-marker answer. Startup now starts the Rig manager before restoring managed viewers. |
| Managed shell job after reboot | Real Agent Deck shell-job guest reboot passed: manager/project/log identity retained, explicit fallback, no automatic restart and no duplicate execution after reconnects. This is not native coding-conversation recovery. |
| Remote managed-host failure | Passed for Agent Deck 1.16.39 remotes and `--ssh` sessions against a hung, rebooted and powered-off guest: clear fallback naming the unreachable remote, bounded startup, unaffected local sessions, retained transcripts, no duplicate execution. Surviving remote sessions now reattach after a WebMux restart. Remote native resume stays with the execution host's own manager (owner decision). |
| Rig seats on an SSH host | Passed for Rig 0.6.14 with Claude Code 2.1.322 and (relay-identity attach) Codex 0.167.0 on a hung, rebooted and powered-off guest, and with the local seat lost: truthful fallback naming the host, refused automatic reconnection, retained transcripts, no duplicate CLI. Starting the agent again reattaches a surviving CLI, starts one new conversation after a reboot, and starts nothing when the host is unreachable. |
| Native managed conversation after reboot | Passed for guest-native Agent Deck 1.16.39/Claude 2.1.322 on a Linux guest: same native conversation, project, roots and launch policy after an actual reboot, a fresh remembered-marker answer, and once-only recovery across reconnects and controller reconstruction. The upstream startup probe is fixed in the new minimum 1.16.39; real early-input and old-record migration controls passed ([qualification](validation/agent-deck-2026-11-17/README.md)). |

The implementation/preparatory-harness checkpoint `3afb3bf` passed every hosted
CI and Packaging check. Local qualification also includes affected Go race
suites/vet, strict TypeScript, 50 helper tests and real native Rig recovery.
Consult #322 for the checks on its final handoff commit. Full receipts, hashes,
supported versions and historical limits are in [session recovery](session-recovery.md).
The next session's working context is in [MEMORY.md](../MEMORY.md).

Remaining #318 work, in order:

1. Finish additional native active-session, policy/profile and platform
   coverage. Remote managed-host failure fallback is qualified
   ([details](session-recovery.md#remote-managed-host-failure-2026-11-10)), as
   are Claude Code Rig seats on SSH hosts
   ([details](session-recovery.md#rig-seats-on-an-ssh-host-2026-11-10)) and
   Codex Rig seats on SSH hosts (relay-identity attach,
   [details](session-recovery.md#codex-seats-on-an-ssh-host-2026-11-11)).
   Cursor in-process chat switches are now observed
   ([details](session-recovery.md#cursor-chat-switches-2026-11-12)). OpenCode
   switches await upstream ([anomalyco/opencode#54329](https://github.com/anomalyco/opencode/issues/54329);
   owner decision: explicit choice until then). Explicit models and other
   ordinary settings are now resumed for Codex, Cursor and OpenCode
   ([details](session-recovery.md#unsupported-invocation-options)); native
   Windows qualification is scoped to the cases below. An authenticated Windows
   AMD64 Codex 0.153.0 headless exact-ID continuation passed; native lookup tests
   also reproduced and fixed the Windows executable-resolution gap
   ([scope and remaining work](session-recovery.md#windows-native-baseline-2026-11-12)).
   The Windows recovery worker now passes native ConPTY input-handoff,
   cancellation and once-only controls
   ([worker scope](session-recovery.md#windows-recovery-worker-2026-11-12)).
   An independent Windows ConPTY owner now passes native controller/job-loss,
   duplicate-creation, resize and explicit-replacement controls. Windows
   local terminals now attach directly to that owner, and a real browser with
   authenticated Codex passed application crash/restart survivor reattachment
   with unchanged owner/root/CLI identity and a fresh remembered-marker answer
   ([integration scope and remaining limits](session-recovery.md#windows-local-provider-integration-2026-11-13)).
   Manual exact-ID Codex continuation after provider loss is also demonstrated.
   The browser's structured native Resume action also passed with the supported
   Windows unelevated-sandbox setting, preserved restrictions, a fresh native
   remembered-marker response and duplicate-action controls; another controller
   crash preserved the resumed coding process
   ([action qualification](session-recovery.md#windows-structured-native-resume-2026-11-13)).
   Versioned snapshots now preserve the original Windows profile; authenticated
   native recovery also passed with a changed replacement-controller profile
   ([root qualification](session-recovery.md#windows-profile-root-recovery-2026-11-13)).
   An approved actual Windows host reboot also passed standalone Codex recovery:
   exact conversation/project/policy, a fresh remembered-marker response,
   byte-identical pre-reboot transcript/native-file prefixes and durable
   duplicate receipts across another controller crash
   ([reboot qualification](session-recovery.md#windows-native-conversation-after-host-reboot-2026-11-13)).
   Installed-service qualification on the Windows 11 AMD64 worker reached
   authenticated browser login but was blocked by Defender's classification of
   the verified preview image ([#384](https://github.com/jordanhubbard/webmux/issues/384)).
   The service was stopped with protection unchanged; native diagnostic and
   no-duplicate-owner controls passed separately
   ([evidence](session-recovery.md#windows-installed-service-security-block-2026-11-16)).
   A newer main artifact now passes authenticated ZIP/WinSW LocalSystem service
   recovery with Codex 0.153.0: controller crash survival, actual guest reboot,
   explicit exact-conversation Resume and durable duplicate-action controls
   ([qualification](validation/windows-installed-recovery-2026-11-17/README.md)).
   The earlier image's Defender classification remains unresolved. Other service
   accounts and broader CLI/configuration coverage remain open. A private
   user-run AMD64/Codex 0.153.0 executable upgrade now preserves an actual active
   native tool across replacement, then passes public exact-conversation Resume,
   fresh filesystem denials, retained prefixes and durable duplicate/sibling
   controls ([qualification](validation/windows-functional-upgrade-2026-11-18/README.md)).
   This explicitly selected cmd.exe case retains the legacy default-shell and
   network-denial failures under #538. The observed public-delete orphaning gap
   now has an owner-bound Stop/hide API and UI implementation under #541. Actual
   native application checks pass lost replies, unresolved-stop reconstruction,
   sibling/viewer retention, original job teardown and retained logs; the exposed
   minimized-layout bug is fixed and requalified
   ([qualification](validation/windows-local-stop-2026-11-18/README.md)). This implementation
   is integrated through #568; applicable current-source checks remain required.
   Installed-package different-executable upgrades and
   authenticated ARM64 conversation recovery remain open. A separate
   [native ARM64 component qualification](validation/windows-arm64-native-2026-11-17/README.md)
   under #456 adds architecture-bound server/ConPTY qualification; its native
   results remain separate from authenticated conversation recovery. Remote native resume is
   manual by owner decision. Unqualified cases must
   continue to offer explicit fallback with retained logs.
   Hosted Windows fixtures originally reported unconfirmed native-provider
   startup. [#543](https://github.com/jordanhubbard/webmux/issues/543) is now
   closed after diagnostics identified a restrictive immediate-job boundary
   and the mandatory actual connected-shell/confirmed-Stop gate passed through
   the bounded disposable SYSTEM fixture. The
   [acceptance audit](validation/native-platform-acceptance-2026-11-19/README.md)
   preserves native user-account evidence and the synthetic hosted scope. The
   [browser-fixture lifecycle repair](validation/windows-local-stop-2026-11-18/ci-fixtures.md)
   retains unresolved access and closes owned browser workers; application
   gate evidence remains separate from authenticated coding recovery.
2. Codex in default shared-daemon mode is manual-only by owner decision: no
   observation point preserves launch mode and sandbox policy after interactive
   switches. Revisit when [openai/codex#60625](https://github.com/openai/codex/issues/60625)
   or an equivalent ships ([details](session-recovery.md#codex-shared-daemon-observation-2026-11-10)).
3. Agent Deck attach-probe retirement is qualified under [#414](https://github.com/jordanhubbard/webmux/issues/414):
   minimum 1.16.39 contains the upstream fix; old-record migration, native reboot,
   local/SSH/remote lifecycle and production attach-worker early-input controls
   passed ([scope and evidence](validation/agent-deck-2026-11-17/README.md)).
4. Audit #318 against its complete acceptance criteria before closing it. The
   owner-approved merge of #322 does not close this requirement or cut a release.
   The [seven-criterion audit](validation/recovery-acceptance-audit-2026-11-19/README.md)
   maps the implementation and retained native evidence to every requirement;
   it records outstanding coverage and integration, not completed acceptance.

[#320](https://github.com/jordanhubbard/webmux/issues/320) is closed through
[#381](https://github.com/jordanhubbard/webmux/pull/381). Real Linux ARM64 and
hosted AMD64 controls reproduce the legacy guacd initial-size registration race;
the verified guacd 1.6.0 build passes the same ordering control, ordinary and
stalled-previous-viewer reconnects, real pixels and keyboard delivery. Bounded
connection/frame/process receipts retain raw pixels and codec evidence. The
Windows resize observer also passed twenty repeated real measurement controls,
including a negative case that cannot invent the requested size. All seventeen
expected PR-head outcomes passed; exact-main CI and Packaging passed at
`232dbdce8b18698e0363a3fd288896e1fd33b8a1` before subsequent work.
The USB-forwarding CLI's real SSH delayed/hung/failed cleanup controls and
[#338](https://github.com/jordanhubbard/webmux/pull/338)'s peer-policy separation
remain separate evidence. These reproduced fixes do not establish the cause of
every historical disconnect. See [RDP scope and setup](rdp-proxy.md).

[#324](https://github.com/jordanhubbard/webmux/issues/324) separates fake-manager
preparation from the production service-query deadline. A shared executable
keeps private state for each case and must complete its readiness invocation
before the checker runs. Controls retain the stable-PID/exited-job assertions
and prove that slow or failed manager queries still fail. Production launchd
commands and deadlines are unchanged; real private launchd replacement remains
part of qualification.

The shared Node fake's per-query interpreter startup reached the unchanged
one-second query deadline in hosted CI, reopening #324. The prepared native fake
passed all seventeen affected controls and all 253 helper tests at default
concurrency on the Mac qualifier. Hosted helper checks subsequently passed, and
the fix entered main through #568. #324 is closed; blocked-Node, cold-start,
failed-query and hung-query controls retain the production boundaries
([qualification](validation/launchd-native-fixture-2026-11-18/README.md)).
The Linux hosted suite explicitly skips its separate native macOS launchd service
case; a fixture pass does not establish native service or #318 acceptance.

### Windows packaging reliability

[#236](https://github.com/jordanhubbard/webmux/issues/236) and
[#274](https://github.com/jordanhubbard/webmux/issues/274) are addressed by
[#336](https://github.com/jordanhubbard/webmux/pull/336): bounded MSI/download
processes, complete-smoke deadlines, stalled-process-tree controls and retained
phase/installer/service diagnostics. See [packaging](packaging.md#windows-msi-smoke-deadlines-and-diagnostics).
The observed stalls are not established product regressions. Native Windows
coding-session recovery in #318 remains separate and unqualified.

### Windows screen restoration

[#355](https://github.com/jordanhubbard/webmux/issues/355) and
[#427](https://github.com/jordanhubbard/webmux/issues/427) meet the scoped screen
mirror and independent underline/extended-state requirements. The bounded
mirror preserves visible and future terminal state independently of the output
tail. Explicit v1/v2 capability negotiation and one-use snapshot metadata refuse
unsupported states or older incompatible viewers while the same job continues.

The [current acceptance record](validation/windows-screen-acceptance-2026-11-19/README.md)
verifies original source005 AMD64 and ARM64 native uploads: 28 actual ConPTY
captures and 1,381 initial/continued/recaptured comparisons per architecture.
Each upload has 259 bare-xterm comparisons and 1,122 production-hyperlink-reference
comparisons; neither reference uses the snapshot adapter. ARM64 capture is
native, but its browser is AMD64 under emulation. Ordinary server tests' native
lifecycle skips remain separate from dedicated capture passes.

The screen, filter, owner, retained VT/ultraviolet, renderer and dependency sources
are unchanged at audited main9c6a549. Current component race and provenance gates
pass. The original source005's ten hosted workflows passed; this audit did not
dispatch hosted jobs or qualify the entire current application on Windows.

[#458](https://github.com/jordanhubbard/webmux/issues/458) also meets its scoped
aggregate-budget requirement: all ten full non-race VT repetitions and original
assertions remain intact, with per-test JSON and always-retained artifacts.
The final native suite took 291.4 seconds within the ten-minute allowance.
Both original four-minute failures remain retained as failures. The earlier
runtime corruption (#450) and private Ice Lake timeout (#536) remain unresolved.

Historical milestones remain in the [no-wrap](validation/xterm-nowrap-2026-11-17/README.md),
[hyperlink](validation/screen-links-2026-11-17/README.md), and
[underline](validation/xterm-underline-2026-11-17/README.md) records. The specific
dependency advisory under #429 is closed. Broader native coding-session recovery
and release acceptance remain under #318 and their existing follow-up issues.
Unsupported display features remain explicit attachment refusals.

## Runtime compatibility implementation

[#277](https://github.com/jordanhubbard/webmux/issues/277) is closed: bounded
[server compatibility and self-reported build metadata](server-info.md),
preserving existing authentication, federation and provider boundaries, has
been implemented.

## Connected WebMux servers: v1.4.0

[#238](https://github.com/jordanhubbard/webmux/issues/238) tracks the owner-approved
multi-host architecture through release and distribution. The implementation
merged in [#239](https://github.com/jordanhubbard/webmux/pull/239), and
[v1.4.0](https://github.com/jordanhubbard/webmux/releases/tag/v1.4.0) is published
from `98b92e84c29d8ba85674026ef5b50f7c5c5927a4`. Exact-source
[CI](https://github.com/jordanhubbard/webmux/actions/runs/41245370881) and
[packaging](https://github.com/jordanhubbard/webmux/actions/runs/41245370869)
passed before publication. The issue retains the final publication, artifact
and Homebrew verification record.

Each host runs WebMux and owns its local terminals. A configured, authenticated
server directory supplies a local view, a selected-server view and an all-server
view, plus session-targeted agent messages. Rig and Agent Deck remain optional
orchestration layers. See [the server contract and setup](server-federation.md).

Implementation and acceptance:

- Peer origins and per-owner incoming grants, with server-held credentials and
  revocation checks.
- Compact server selection, labeled remote terminals, viewer-only hide/reconnect,
  explicit provider restart and independent unavailable-server warnings.
- Cross-host terminal messages with queue-admission semantics and source audit
  records; no claim of agent processing or task completion.
- Native two-server and real browser coverage, ownership/security checks,
  regression gates and a reviewed implementation PR.
- Consistent v1.4.0 release metadata, exact-commit CI and all four native
  packaging targets, followed by publication of 28 artifact/checksum files.
  GitHub-stored digests matched the qualified artifacts at publication.

[#240](https://github.com/jordanhubbard/webmux/issues/240) resolved the newly
published lint-dependency advisory through compatible minimatch/brace-expansion
updates. The dependency-audit gate remains enabled and passed for this release.

## Integration and feature follow-ups

- [#598](https://github.com/jordanhubbard/webmux/issues/598): durable,
  owner-scoped Agent Deck send intent now consumes an exclusive dispatch right
  before the provider call. Browser reload, lost replies and controller crash
  preserve the original request rather than enqueueing it again. The
  [source-bound local/native qualification](validation/coordination-send-intent-2026-11-19/README.md)
  covers real queue acceptance and delayed-request dismissal. The shared-identity
  and completion-event bridge under #246 remains separate and incomplete.
  [Native completion evidence retention](validation/coordination-completion-identity-2026-11-19/README.md)
  preserves the owner-provided turn fingerprint/source profile and selected inbox
  profile through Peek/Drain; no global identity or bridge completion is implied.


- [#264](https://github.com/jordanhubbard/webmux/issues/264): the [strict source
  coverage gate](typescript.md) uses compiler CLIs and is qualified separately
  against native TypeScript 7; the full compiler upgrade still requires a
  compatible ESLint toolchain.

- [#275](https://github.com/jordanhubbard/webmux/issues/275): source-backed
  [native bridge integration research](native-bridge-integration.md) for Codex iOS
  is complete and the research issue is closed. Connector implementation, scoped
  authorization choices and physical cross-project acceptance remain separate,
  unqualified work.

- [#194](https://github.com/jordanhubbard/webmux/issues/194): all six grid
  integration requirements and the two-seat scenario have a
  [source-bound acceptance audit](validation/rig-grid-acceptance-2026-11-19/README.md).
  It combines actual two-Codex conversations, selected-cell/viewer identity,
  canonical status and reviewed Stop with native navigation/coexistence and
  explicit broadcast-recipient evidence. Execution host and attachment availability now appear
  separately in the tile status strip. Full recovery #318, remaining native lifecycle #438,
  the opt-in bridge #246 and release gates remain open. Controller queries, lifecycle
  calls, diagnostic views and new launchers now pin their configured coordinator
  despite an unrelated CLI host selection
  ([scope and qualification](openrig.md#coordinator-selection-scope)).
  [#430](https://github.com/jordanhubbard/webmux/issues/430) keeps an
  external seat's unverified execution host separate from its local viewer
  transport and native Rig host identity. Its native input/reopen control and
  persisted recovery metadata are qualified separately. The original macOS
  mouse-isolation and diagnostic-endpoint fixture failures were resolved under
  closed [#431](https://github.com/jordanhubbard/webmux/issues/431).
  [#443](https://github.com/jordanhubbard/webmux/issues/443) adds bounded
  local change notifications that invalidate canonical inventory, with real Rig
  browser coverage while fallback timers are paused. These implementations
  entered main through #568; retain their source-specific
  [qualification and remaining gates](validation/rig-change-notifications-2026-11-17/README.md).
- [#200](https://github.com/jordanhubbard/webmux/issues/200): remote Rig state over
  the outbound managed-host client. An opt-in bounded canonical-seat report now
  carries host-scoped identity and observation provenance, with stale/offline
  inventory and read-only SSH attachment. The published-Rig observer and
  authenticated channel controls are covered separately. A real Linux ARM64
  guest now also passes authenticated host check-in, browser/SSH input, multiple
  windows, client loss/reconnect, controller restart with stable seat/layout,
  revocation and ordinary SSH with Rig disabled. See [managed-host observation](managed-hosts.md#observe-existing-rig-seats).
  Server federation is separate from this contract.
- [#110](https://github.com/jordanhubbard/webmux/issues/110): browser companion
  mobile/accessibility work and real-provider/SSH-terminal
  authentication qualification remain open. Opt-in saved profiles and exclusive
  viewer control are implemented in [#401](https://github.com/jordanhubbard/webmux/pull/401);
  see the [profile and lease scope](browser-companion.md). File controls are implemented.
  An on-demand [remote-page text view](browser-companion.md#read-remote-page-text)
  exposes Chromium accessibility names and roles. Explicit Focus/Activate
  controls now support bounded DOM interaction with native stale-reference,
  identity and once-only dispatch controls under [#419](https://github.com/jordanhubbard/webmux/issues/419).
  Broader DOM/iframe, mobile and assistive-technology acceptance remain open.
  Automatic launch for new Mosh shells is implemented under
  [#468](https://github.com/jordanhubbard/webmux/issues/468), using the existing
  authenticated host control channel with sealed, expiring requests and durable
  acknowledgement. Real macOS loopback SSH/Mosh/PTY/browser callback controls
  pass with logging off; Linux hosted qualification remains required. See the
  [qualified source and scope](validation/mosh-browser-2026-11-18/README.md).
  Packaged Chromium file visibility and a real SSH worker transfer have separate
  [qualification evidence](browser-companion.md#packaged-chromium-file-visibility-2026-11-16).
- [#120](https://github.com/jordanhubbard/webmux/issues/120): opt-in USB forwarding
  for general USB devices, including robots and USB security keys. A simple
  simulated device is sufficient for device acceptance; the existing Linux
  USB/IP mass-storage gadget exercises enumeration, exact reads/writes, detach,
  interruption and cleanup. Physical hardware, iPhone and Xcode are not required.
  Owner-scoped browser lease status and Stop controls are implemented under
  [#485](https://github.com/jordanhubbard/webmux/issues/485), with one-use helper
  pairing, durable stop requests and separate confirmed/uncertain cleanup states.
  Exact-source native simulation and normal merge gates remain required.

## External distribution and qualification

- [#91](https://github.com/jordanhubbard/webmux/issues/91): WiX migration review,
  deployment-specific component inventories. A [read-only collector](native-components.md)
  records native tool identities and a selected Linux guacd process's loaded
  plugins/libraries, with exact Debian package versions and copyright evidence.
  It is qualified separately on macOS tools and a private Linux ARM64 proxy;
  deployment-specific security/license review remains open. The browser library now uses a
  reproducible local package from verified Apache Guacamole 1.6.0 sources, with
  license/notices and source/package digests retained in native bundles
  ([provenance](../webmux/vendor/guacamole-common-js/README.md)).
- [#63](https://github.com/jordanhubbard/webmux/issues/63),
  [#65](https://github.com/jordanhubbard/webmux/issues/65), and
  [#152](https://github.com/jordanhubbard/webmux/issues/152): Windows signing,
  WinGet publication and macOS Developer ID notarization. The [WinGet
  submission workflow](winget.md) binds inspected MSI identities to release
  digests; upstream acceptance remains separate.
- [#186](https://github.com/jordanhubbard/webmux/issues/186) is closed: its five
  named hosted runs were verified completed successfully on 2026-11-12, covering
  exact-v1.3.23 CI/packaging/publication and formula-update CI/packaging. The
  [issue evidence](https://github.com/jordanhubbard/webmux/issues/186#issuecomment-6841784248)
  records the commits and runs; it does not qualify current source or #318.

External credentials, hardware and native-target evidence remain distinct from
local implementation tests. Existing dependency/contributor PRs need their own
review and checks; they are not folded into #238 automatically.

## Built-in coordination default

[Issue #242](https://github.com/jordanhubbard/webmux/issues/242) makes WebMux the
default built-in experience, with Agent Deck and Rig as explicit optional
integrations. Existing saved choices are retained. Selection and setup are
explained in [agent coordination](openrig.md).
[Issue #243](https://github.com/jordanhubbard/webmux/issues/243) updates stable
Homebrew distribution for v1.4.1: Agent Deck becomes optional, while tmux remains
a direct dependency for built-in local terminals.

## Independent coordination integrations: v1.4.1

[Issue #245](https://github.com/jordanhubbard/webmux/issues/245) separates enabled
integrations, inventory filters, and default launch ownership for v1.4.1. Agent
Deck and Rig can be enabled together; native terminals remain available. Each
session retains its lifecycle owner. Shared-agent identity/event bridging is
outside this release and tracked by [#246](https://github.com/jordanhubbard/webmux/issues/246).

[v1.4.1](https://github.com/jordanhubbard/webmux/releases/tag/v1.4.1) is published
from `0ebdfa0379825873c0be0bf6932879e1d96fc77b`. Exact-source
[CI](https://github.com/jordanhubbard/webmux/actions/runs/41326686398) and
[packaging](https://github.com/jordanhubbard/webmux/actions/runs/41326686382)
passed before publication; all 28 native asset/checksum files were verified.
The stable formula update uses the published source archive and does not move
the release tag to incorporate its necessarily post-publication checksum.

[#251](https://github.com/jordanhubbard/webmux/issues/251), implemented in
[#252](https://github.com/jordanhubbard/webmux/pull/252), permits explicitly
selected temporary fleet runners for trusted release checks. The actual
macOS version is recorded; public pull-request jobs keep hosted runners.
See [temporary runner operation](ci-runners.md).

[#253](https://github.com/jordanhubbard/webmux/issues/253) reproduced duplicate
DECRQM replies from two viewers corrupting tmux 3.8 shell input. Viewers now
consume these queries in the parser, extending the existing query policy without
filtering ordinary input bytes. The actual tmux 3.8 two-viewer browser workflow
passes, as do deterministic split-query/broadcast controls with real PTY
keyboard/paste receipts and existing mouse lifecycle checks. See the
[evidence and scope](browser-companion.md#acceptance); real identity-provider
qualification remains under #110.


The [Rig grid integration](validation/rig-grid-integration-2026-11-17/README.md)
reconciles #432/#433/#435/#437/#440/#441/#444/#446 with local native coverage and
a hidden-grid observation fix. This implementation is in `main` through #568.
The original macOS fixture follow-up #431 is closed. Source-specific evidence,
the [completed six-part #194 audit](validation/rig-grid-acceptance-2026-11-19/README.md)
and #429 disposition stay distinct from full roadmap and release acceptance.

Ordinary-local Windows Stop/Hide is composed under #541, including a
[preparation-save error-state repair](validation/windows-local-preparation-save-2026-11-19/README.md).
Native application startup work under
[#543](https://github.com/jordanhubbard/webmux/issues/543) includes bounded
phase/error diagnostics and a mandatory fresh-shell/confirmed-Stop browser gate.
The [original preparation and limits](validation/windows-provider-startup-2026-11-19/README.md)
remain historical; later actual application gates pass on disposable hosted
AMD64 and ARM64 VMs, including both mandatory local-shell/Stop cases under #571.
These use the guarded SYSTEM fixture and do not establish authenticated user
coding recovery, installed-service upgrades or native ARM64 browser execution.
Browser-companion anchors and ConPTY captures remain separate evidence. Full
recovery under #318, applicable current-source checks and release acceptance stay
open.
