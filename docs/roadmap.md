# WebMux active roadmap

Reviewed 2026-10-31 against the owner's recovered session and the open GitHub
issues. GitHub issues track implementation and remaining acceptance work.

## Current reliability work

**First priority: [#318](https://github.com/jordanhubbard/webmux/issues/318).**
Session-state recovery is the next implementation and qualification work, ahead
of the remaining reliability and integration follow-ups. Its full acceptance
criteria remain required; this priority change does not establish recovery
support.

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
proof of restored agent state. Implementation and qualification remain open.

### Windows packaging reliability

[#236](https://github.com/jordanhubbard/webmux/issues/236): bound Windows MSI smoke
operations and preserve progress/installer diagnostics after a stalled packaging
run. The stall is not established as a product regression. This remains a
follow-up after #318; it does not displace session-state recovery.

## Runtime compatibility follow-up

[#277](https://github.com/jordanhubbard/webmux/issues/277): expose bounded
[server compatibility and self-reported build metadata](server-info.md),
preserving existing authentication, federation and provider boundaries.

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
  [native bridge integration research](native-bridge-integration.md) for Codex iOS.
  Current coordination/federation APIs are reusable, but connector implementation,
  scoped authorization choices and physical cross-project acceptance remain pending.

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
- [#186](https://github.com/jordanhubbard/webmux/issues/186): retained exact-release
  hosted qualification follow-up for v1.3.23. Verify its named runs before closing.

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
