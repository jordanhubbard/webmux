# WebMux active roadmap

Reviewed 2026-10-31 against the owner's recovered session and the open GitHub
issues. GitHub issues track implementation and remaining acceptance work.

## Current priority: connected WebMux servers

[#238](https://github.com/jordanhubbard/webmux/issues/238) resumes the owner-approved
multi-host architecture. Earlier execution stopped before any repository changes
because the command runner could not negotiate with its host. That execution
blocker is now resolved.

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
- Release preparation, exact-commit CI and native packaging, publication and
  artifact verification. The feature is not released until these gates pass.

The first implementation PR is [#239](https://github.com/jordanhubbard/webmux/pull/239).
Its initial hosted dependency gate exposed a newly published advisory in the
existing lint dependency tree. [#240](https://github.com/jordanhubbard/webmux/issues/240)
tracks the compatible minimatch/brace-expansion update required before release;
keep the audit gate enabled.

## Next reliability work

[#236](https://github.com/jordanhubbard/webmux/issues/236): bound Windows MSI smoke
operations and preserve progress/installer diagnostics after a stalled packaging
run. The stall is not established as a product regression. Deferred while #238
is the owner's highest priority.

## Integration and feature follow-ups

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
