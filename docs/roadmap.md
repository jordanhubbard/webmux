# WebMux active roadmap

Reviewed 2026-11-17 against the implementation, qualification receipts and GitHub
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
   Authenticated installed-service recovery, broader CLI/configuration coverage
   and ARM64 hardware qualification remain open. Remote native resume is
   manual by owner decision. Unqualified cases must
   continue to offer explicit fallback with retained logs.
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

### Windows packaging reliability

[#236](https://github.com/jordanhubbard/webmux/issues/236) and
[#274](https://github.com/jordanhubbard/webmux/issues/274) are addressed by
[#336](https://github.com/jordanhubbard/webmux/pull/336): bounded MSI/download
processes, complete-smoke deadlines, stalled-process-tree controls and retained
phase/installer/service diagnostics. See [packaging](packaging.md#windows-msi-smoke-deadlines-and-diagnostics).
The observed stalls are not established product regressions. Native Windows
coding-session recovery in #318 remains separate and unqualified.

### Windows screen restoration

[#355](https://github.com/jordanhubbard/webmux/issues/355) tracks screen-state
restoration separately from native conversation recovery. Saved-cursor restores
now preserve the frontend's live extended underline style/color, including
1048/1049 and batched modes. Unrepresentable hidden or contradictory underline
flags explicitly refuse new attachments while preserving the existing job;
a full terminal reset repairs the mirror. The portable browser suite covers
40 initial/continued/second-attachment comparisons, and Windows CI now runs the
same renderer against real ConPTY captures. See the
[scope and controls](session-recovery.md#saved-underline-restoration-2026-11-16).
Guest palette/default colors now have a bounded metadata path, preserving
indexed cells across later palette changes without replaying color queries.
The portable renderer suite has 51 cases; Windows CI adds nine real ConPTY
captures ([color scope](session-recovery.md#guest-palette-and-default-colors-2026-11-16)).
Bounded OSC 8 hyperlink targets, Unicode IDs and link groups now survive initial
attachment, continued output and a second attachment through [#405](https://github.com/jordanhubbard/webmux/pull/405).
The production renderer retains the current markerless hyperlink with bounded
lifecycle cleanup. Its gate passes 61 portable comparisons, 13 actual ConPTY
cases and an independent renderer lifecycle control
([hyperlink evidence](validation/screen-links-2026-11-17/README.md)).
No-wrap cursor, resize and retained REP parser-state restoration are implemented
through [#422](https://github.com/jordanhubbard/webmux/pull/422), addressing
[#421](https://github.com/jordanhubbard/webmux/issues/421) and
[#424](https://github.com/jordanhubbard/webmux/issues/424). The opt-in engine
preserves addressed versus advanced edge cursors, wide-cell overlap, saved and
alternate-screen state, edits, margins and resize truncation. A versioned
renderer profile restores parser state that ANSI cannot safely reseed. Required
capabilities and a one-use token precede snapshot output; older incompatible
viewers are refused while the same job continues.

The exact PR head passed all 17 required outcomes, including fresh Windows
server/protocol contracts and all 22 actual ConPTY controls. All 431 retained
initial/continued/second-attachment renderer comparisons matched, with no
replayed queries and an unmodified reference renderer. Portable/parser and
controlled-terminal tests have separate scopes from native ConPTY captures;
see the [qualified sources and limits](validation/xterm-nowrap-2026-11-17/README.md).
Hidden or contradictory underline flags, other unqualified OSC display features
and the broader native coding-session lifecycle remain under #355/#318.

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

- [#264](https://github.com/jordanhubbard/webmux/issues/264): the [strict source
  coverage gate](typescript.md) uses compiler CLIs and is qualified separately
  against native TypeScript 7; the full compiler upgrade still requires a
  compatible ESLint toolchain.

- [#275](https://github.com/jordanhubbard/webmux/issues/275): source-backed
  [native bridge integration research](native-bridge-integration.md) for Codex iOS
  is complete and the research issue is closed. Connector implementation, scoped
  authorization choices and physical cross-project acceptance remain separate,
  unqualified work.

- [#194](https://github.com/jordanhubbard/webmux/issues/194): remaining canonical
  Rig discovery, lifecycle/provenance contract and optional Agents-pane
  reconciliation. Its grid-centered UI work landed in #196; the issue remains
  open for its other acceptance requirements. Controller queries, lifecycle
  calls, diagnostic views and new launchers now pin their configured coordinator
  despite an unrelated CLI host selection
  ([scope and qualification](openrig.md#coordinator-selection-scope)).
  Candidate [#430](https://github.com/jordanhubbard/webmux/issues/430) keeps an
  external seat's unverified execution host separate from its local viewer
  transport and native Rig host identity. Its native input/reopen control and
  persisted recovery metadata are qualified separately; the broader macOS
  mouse-isolation and diagnostic-endpoint failures remain under
  [#431](https://github.com/jordanhubbard/webmux/issues/431).
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
  mobile/accessibility work, Mosh launch and real-provider/SSH-terminal
  authentication qualification remain open. Opt-in saved profiles and exclusive
  viewer control are implemented in [#401](https://github.com/jordanhubbard/webmux/pull/401);
  see the [profile and lease scope](browser-companion.md). File controls are implemented.
  An on-demand [remote-page text view](browser-companion.md#read-remote-page-text)
  exposes Chromium accessibility names and roles. Explicit Focus/Activate
  controls now support bounded DOM interaction with native stale-reference,
  identity and once-only dispatch controls under [#419](https://github.com/jordanhubbard/webmux/issues/419).
  Broader DOM/iframe, mobile and assistive-technology acceptance remain open.
  Packaged Chromium file visibility and a real SSH worker transfer have separate
  [qualification evidence](browser-companion.md#packaged-chromium-file-visibility-2026-11-16).
- [#120](https://github.com/jordanhubbard/webmux/issues/120): opt-in USB forwarding
  and real iPhone/Xcode qualification.

## External distribution and qualification

- [#91](https://github.com/jordanhubbard/webmux/issues/91): WiX migration review,
  deployment-specific component inventories. The browser library now uses a
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
