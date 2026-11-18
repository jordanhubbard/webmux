# Roadmap USB dashboard composition

Compose pinned #494 with #488, including #483's corrected general USB/simulator
scope. The owner has explicitly accepted simulated USB devices for #120; physical
equipment, iPhone/Xcode, Apple signing and particular OS/device pairs are not
requirements. This does not qualify every native backend or peripheral.

The USB setup dialog creates owner-scoped leases, pairs the operator-launched
helper, shows transport/freshness state and requests Stop. The helper reports
actual worker completion; Stop remains pending until confirmed release. Existing
SSH host-key, loopback-only, bounded transport and cleanup controls remain.

The qualification wrapper merge conflict is resolved by retaining both the
checksum-verified source-cache build function and the dashboard admission
function. The latter requires both data-path and dashboard markers with exit 0
and a clean kernel. All 17 wrapper/source-cache controls pass, as do sysfs
readability controls. Existing readable exact-payload enumeration is retained
in the merged kernel fixture. No workflow, job, gate or deadline changes.

On this combined application source, full server/retained VT/ultraviolet race
tests and vet, 416 frontend tests, 129 helper checks, five strict TypeScript
projects, frontend lint and application build pass. Actual macOS Chromium passes
three cases: narrow USB setup, helper-report/Stop/release UI and real Mosh/private
SSH/sign-in/acknowledgment. The USB browser reporter is synthetic; it is not a
USB-device acceptance claim. AMD64 and ARM64 Windows PE cross-compilation passes,
which is not native execution.

Input #488 CI `44285261257` passes every non-audit job, including native virtual
USB; #429 remains failed. That input result does not qualify this combined head.
Exact combined hosted CI/native kernel, packaging and diagnostics remain pending.
Keep draft with #429, #450, #473 and broader release gates open. No main, tag,
release, physical device, signing or operator-machine mutation is performed.
