# Owner-scoped USB forwarding status and Stop

The foreground helper can pair with a browser-created lease, report its
transport state and honor an owner Stop request. The data path remains the
existing loopback exporter, SSH stdio protocol and receiving loopback worker.
Device claiming and application behavior remain with the native importer.

The implementation is tracked by #485 under the generalized #120 scope. A
simulated USB device is sufficient; no physical iPhone or Xcode target is required.

## Authority and lifecycle

| Operation | Authority |
| --- | --- |
| Create/list forwarding leases | Normal WebMux authentication; owner-filtered list |
| Stop a lease | Its WebMux owner |
| Pair a helper | One-use, five-minute code bound to host, ports and duration |
| Report/poll Stop | Temporary credential for that lease only; owner must still exist |

Only digests are persisted. The helper receives no account token. Pending
commands, running transports, stale reports, requested Stop and confirmed or
uncertain cleanup are separate states. A pending command that expires or is
canceled cannot subsequently pair. Reports cannot resurrect a terminal lease or
change an established receiving port. Corrupt stored authority fails closed.

The controller retains stop requests and reporter identity across reconstruction.
The native helper independently retains the existing lease deadline and SSH
cleanup envelope. Status requests have bounded bodies, responses and deadlines;
redirects are refused. Owner and global active-lease quotas and bounded history
limit retained state. Existing wire framing, stream/byte limits and SSH trust
requirements are unchanged.

## Local evidence and scope

The [local check receipt](local-checks.json) binds source files and retained logs
to these observations. Its HTTP-suite skip is the unrelated optional native
Agent Deck read-only qualification; it is not counted as executed coverage.

- Linux ARM64, Go 1.27.4, Chromium 156.0.8078.47.
- Both affected Go packages pass the race detector; root Go vet passes.
- HTTP controls use actual authenticated Alice/Bob accounts to exercise owner
  isolation, one-use claim/replay, limited credential scope, account deletion,
  bounded bodies and controller reconstruction.
- Lease controls cover concurrent claims, target binding, expiry, owner/global
  quotas, stale status, immutable terminal outcomes, digest-only persistence and
  refusal to overwrite corrupt authority.
- A built CLI and separate real SSH worker transfer exact binary bytes. An
  owner Stop survives manager reload. A held remote EOF keeps the native worker
  and receiving listener alive: the dashboard remains stopping without release
  confirmation until the worker actually exits. It then records ended/released,
  and both the listener and active stream are closed.
- Redirect and revoked-reporter controls cover the native HTTP reporter.
- All 374 frontend tests pass, including 15 USB dialog cases. Strict compiler
  coverage, frontend build/lint and local documentation links pass.
- Two actual Chromium cases pass: keyboard/workspace/narrow-screen setup and the
  actual server's pairing/report/Stop/acknowledgment API through the UI. The latter
  uses a synthetic reporter; it does not claim an attached USB device.
- Windows AMD64/ARM64 test binaries are compilation controls, not native execution.

An initial browser run exposed focus loss when command creation temporarily
disabled its focused button. Explicit focus moves to the generated command or
status region after the corresponding action; the original containment and
narrow-screen assertions pass unchanged.

## Native USB acceptance

The dedicated virtual-kernel fixture now starts the actual WebMux controller,
pairs the actual CLI, forwards the USB/IP mass-storage simulator over real SSH,
kills and reconstructs the controller, verifies continued exact USB reads, and
requests owner Stop. It requires acknowledged release, USB disappearance and
worker/listener cleanup. A separate ordinary CLI run preserves the original
native SIGINT interruption check. Kernel negative/held-reply/direct-cycle
controls and existing deadlines are retained.

Both the data-path and dashboard completion receipts are now required by native
admission. Python admission controls reject a legacy data-only receipt, missing
dashboard completion, nonzero exit and kernel faults.

The exact-source [hosted kernel job](https://github.com/jordanhubbard/webmux/actions/runs/44284270303/job/134732754160) passed on `80f7c65b3159f5ace1317077c18015948939ce7b`. The
[native receipt](native-checks.json) records all four admitted outcomes, the
artifact digest and individual log digests. The unpatched negative control
failed in its required way; corrected held-reply, twenty direct cycles and
WebMux over SSH passed. Both required data-path and dashboard completion
markers are present. This receipt update changes documentation only.

Hosted Windows and packaging checks, dependency audit #429, review and final
integration remain gates. No release or broad peripheral/OS compatibility claim
is made here.
