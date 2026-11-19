# Native Codex interactive permission recovery

Actual packaged `6156a5a` reproduced [#539](https://github.com/jordanhubbard/webmux/issues/539):
changing a running Rig/Codex session from workspace-write/never to Read Only
through its native `/permissions` UI did not change either configuration file.
A real tool turn recorded read-only/on-request and denied six writes and network.
Public original-conversation recovery subsequently restored workspace-write/never:
the next native turn could write the project and original extra root again.
Other four writes and network stayed denied. The conversation identity and
lifecycle controls passed, which did not establish policy preservation.

Code `5a0256d29cf6d9e6fba863915edcffb0a6704532` checks the original local native
journal's applied-settings events before preparing recovery and again before
dispatch. These events include interactive changes without a subsequent model
turn. Changed or unverified settings refuse structured Resume and explain manual
continuation with retained logs. Settings from history never become new launch
arguments. Surviving original processes still reattach without Resume, retaining
their actual current permissions. Ordinary duplicates neither recheck policy nor
reopen a handoff. Frozen operation identity and uncertain-request handling remain.

## Actual native application results

One clean, normally stamped Go executable ran all three fresh cases through real
Chrome, Rig 0.7.18 and Codex 0.169.0 on macOS. Each retains 35 source bindings and
the same executable hash. The existing native account was used in place; no auth
file was copied and no new login occurred. Project trust was reviewed through
the native CLI. A private loopback browser used authentication mode `none`.

| Case | Independently verified result |
| --- | --- |
| Interactive Read Only followed by a native tool turn | Six actual denied writes and denied network; controller SIGKILL retains the original provider; after owned CLI loss, public recovery and its repeat refuse before any native Resume POST. |
| Interactive Read Only with no following model turn | Earlier workspace writes succeed; the native applied-settings record changes after the last turn context; after controller crash and later CLI loss, public recovery and its repeat refuse before any native Resume POST. |
| Unchanged workspace-write/never | Graceful controller restart preserves the provider; later original-only recovery survives controller SIGKILL during dispatch, recalls the original marker without tools, and passes fresh before/after tools with two allowed writes, four denied writes and denied network each. |

All 40 browser/lifecycle controls pass. Native journals independently bind tool
calls, outputs, settings and original conversation identity. Refusal keeps the
viewer, sibling and original history. The unchanged case retains terminal-log
paths/prefixes and duplicate controls. Private tmux servers and controllers stop.
The producer reports a clean source revision; this is a build self-report, not
external attestation.

Current guard admission also passes the retained original native journals and
manifests for default/full access, workspace-write/on-request, read-only/on-request,
workspace-write/never and read-only/never. These five checks use historical native
artifacts; they are not five fresh coding or lifecycle runs.

## Failed attempts and limits

Two earlier full-harness attempts remain failed and retained. The first stopped
at the ready-creation duplicate assertion before the permission selection. The
second completed actual read-only tools but exceeded the private harness's
ten-second controller-stop limit before testing refusal. No cause is established
for either failure. The focused negative cases use a controller crash; the
unchanged positive retains the original graceful-stop assertion and deadline.
No product deadline or existing test assertion was relaxed.

The [receipt](receipt.json) binds original failure, source/binary/journal hashes,
fresh cases, historical admission and failed attempts. Six raw fixture archives,
drivers, original receipts, native journals where captured, and the executable
are retained privately with NAS checksum readback. Inherited broad scope strings
in the original exploratory harness are not acceptance assertions; the case
checks and independent journal audit define the scope above.

Local checks pass all five strict TypeScript projects, 252 helper tests, and
affected coordinator/HTTP/session/recovery Go race suites and vet. Frontend and
retained VT/UV sources are unchanged from `6156a5a`. Hosted review-head checks
remain pending at this publication snapshot; the parent's ten successful
workflows do not qualify the new guard.

Journal reads stay local and bounded to 32 MiB total and 1 MiB per record.
Missing, partial, unstable, unrecognized or larger evidence refuses structured
Resume with retained manual recovery. This does not add selected-profile or
arbitrary permission-graph launch support, restore previously lost interactive
choices, or qualify reboot, service migration or other platforms. #318, #438,
review/integration and full roadmap/release acceptance remain open.
