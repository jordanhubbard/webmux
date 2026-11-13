# WebMux active roadmap

Reviewed 2026-11-12 against the implementation, qualification receipts and GitHub
issue states. GitHub issues track implementation and remaining acceptance work.

## Current reliability work

**First priority: [#318](https://github.com/jordanhubbard/webmux/issues/318).**
Session-state recovery has an implementation checkpoint in
[#322](https://github.com/jordanhubbard/webmux/pull/322). The owner requested that
this checkpoint enter `main` for fix-forward work in a Claude session. This is
an integration handoff, not completed acceptance: #318 remains open and first
priority ahead of the other reliability and integration follow-ups.

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
| Rig conversation after reboot | Passed on a Linux guest with Rig 0.6.14 (9d0200f3)/Claude 2.1.322: host-reboot detection, approved Resume through Rig, once-only receipt, original settings/roots and a fresh remembered-marker answer. Startup now starts the Rig manager before restoring managed viewers. |
| Managed shell job after reboot | Real Agent Deck shell-job guest reboot passed: manager/project/log identity retained, explicit fallback, no automatic restart and no duplicate execution after reconnects. This is not native coding-conversation recovery. |
| Remote managed-host failure | Passed for Agent Deck 1.16.34 remotes and `--ssh` sessions against a hung, rebooted and powered-off guest: clear fallback naming the unreachable remote, bounded startup, unaffected local sessions, retained transcripts, no duplicate execution. Surviving remote sessions now reattach after a WebMux restart. Remote native resume stays with the execution host's own manager (owner decision). |
| Rig seats on an SSH host | Passed for Rig 0.6.14 with Claude Code 2.1.322 and (relay-identity attach) Codex 0.167.0 on a hung, rebooted and powered-off guest, and with the local seat lost: truthful fallback naming the host, refused automatic reconnection, retained transcripts, no duplicate CLI. Starting the agent again reattaches a surviving CLI, starts one new conversation after a reboot, and starts nothing when the host is unreachable. |
| Native managed conversation after reboot | Passed for guest-native Agent Deck 1.16.34/Claude 2.1.322 on a Linux guest: same native conversation, project, roots and launch policy after an actual reboot, a fresh remembered-marker answer, and once-only recovery across reconnects and controller reconstruction. Early input loss on Agent Deck attach was an upstream startup probe, now mitigated ([#325](https://github.com/jordanhubbard/webmux/issues/325)). |

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
   Windows lifecycle recovery is still open. An authenticated Windows AMD64
   Codex 0.153.0 headless exact-ID continuation passed; native lookup tests
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
   Native process-loss resume, configuration-override support and Windows host
   reboot qualification remain open. Remote native resume is
   manual by owner decision. Unqualified cases must
   continue to offer explicit fallback with retained logs.
2. Codex in default shared-daemon mode is manual-only by owner decision: no
   observation point preserves launch mode and sandbox policy after interactive
   switches. Revisit when [openai/codex#60625](https://github.com/openai/codex/issues/60625)
   or an equivalent ships ([details](session-recovery.md#codex-shared-daemon-observation-2026-11-10)).
3. Remove the Agent Deck attach-probe mitigation ([#325](https://github.com/jordanhubbard/webmux/issues/325))
   once a release fixing [asheshgoplani/agent-deck#3105](https://github.com/asheshgoplani/agent-deck/issues/3105)
   is the minimum supported version.
4. Audit #318 against its complete acceptance criteria before closing it. The
   owner-approved merge of #322 does not close this requirement or cut a release.

[#320](https://github.com/jordanhubbard/webmux/issues/320) tracks intermittent
hosted SSH/lease/reconnect failures. The USB-forwarding CLI now waits for remote
command completion before reporting released connections; real SSH controls
cover delayed, hung and failed remote cleanup ([details](usb-forwarding.md#verification-and-remaining-work)).
Peer policy rejection now stays separate from controller authentication, with
real two-server browser coverage in [#338](https://github.com/jordanhubbard/webmux/pull/338).
RDP reconnect qualification remains open in #320; the reproduced SSH and peer
policy fixes do not establish the cause of every historical disconnect.

[#324](https://github.com/jordanhubbard/webmux/issues/324) separates fake-manager
preparation from the production service-query deadline. A shared executable
keeps private state for each case and must complete its readiness invocation
before the checker runs. Controls retain the stable-PID/exited-job assertions
and prove that slow or failed manager queries still fail. Production launchd
commands and deadlines are unchanged; real private launchd replacement remains
part of qualification.

### Windows packaging reliability

[#236](https://github.com/jordanhubbard/webmux/issues/236) and
[#274](https://github.com/jordanhubbard/webmux/issues/274) are addressed by
[#336](https://github.com/jordanhubbard/webmux/pull/336): bounded MSI/download
processes, complete-smoke deadlines, stalled-process-tree controls and retained
phase/installer/service diagnostics. See [packaging](packaging.md#windows-msi-smoke-deadlines-and-diagnostics).
The observed stalls are not established product regressions. Native Windows
coding-session recovery in #318 remains separate and unqualified.

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

- [#275](https://github.com/jordanhubbard/webmux/issues/275): source-backed
  [native bridge integration research](native-bridge-integration.md) for Codex iOS
  is complete and the research issue is closed. Connector implementation, scoped
  authorization choices and physical cross-project acceptance remain separate,
  unqualified work.

- [#194](https://github.com/jordanhubbard/webmux/issues/194): remaining canonical
  Rig discovery, lifecycle/provenance contract and optional Agents-pane
  reconciliation. Its grid-centered UI work landed in #196; the issue remains
  open for its other acceptance requirements.
- [#200](https://github.com/jordanhubbard/webmux/issues/200): remote Rig state over
  the outbound managed-host client. Review its relationship to #238 after the
  multi-server foundation ships; do not treat server federation as proof that
  this separate Rig contract is implemented.
- [#110](https://github.com/jordanhubbard/webmux/issues/110): browser companion
  download/upload, durable profiles/controller lease, mobile/accessibility work,
  and real-provider/SSH authentication qualification.
- [#120](https://github.com/jordanhubbard/webmux/issues/120): opt-in USB forwarding
  and real iPhone/Xcode qualification.

## External distribution and qualification

- [#91](https://github.com/jordanhubbard/webmux/issues/91): WiX migration review,
  Apache-source browser package migration and deployment-specific component
  inventories.
- [#63](https://github.com/jordanhubbard/webmux/issues/63),
  [#65](https://github.com/jordanhubbard/webmux/issues/65), and
  [#152](https://github.com/jordanhubbard/webmux/issues/152): Windows signing,
  WinGet publication and macOS Developer ID notarization.
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

[#253](https://github.com/jordanhubbard/webmux/issues/253) tracks a reproduced
tmux 3.8 multiple-viewer capability-reply/input issue. It was found by expanded
Mac browser testing and remains a known limitation; neither the coordination
changes nor the runner/fixture corrections claim to resolve it.
