# Local USB kernel qualification

> Historical #618 checkpoint. The [held-counter correction](../usb-held-counter-2026-11-19/README.md)
> supersedes its pending #550/#615 disposition with a reproduced counter gap,
> a native repair and a complete six-control result. The original failed/passing
> receipts and their source/platform limits remain unchanged below.

The USB qualifier can now run under the local-resource instruction without
claiming a GitHub Actions identity. `--local-container` admits a disposable,
unprivileged Linux Docker container and verifies that USB operations run inside
a separate booted QEMU/virtme guest. The existing hosted invocation remains
supported. This is test infrastructure, not a runtime USB or coding-policy change.

The source and results are recorded in [receipt.json](receipt.json). The actual
WebMux CLI is built from main `4989e16b59e91924d74dd8e83c5fbfa669f6b179`,
using Go1.27.4, Linux ARM64 and CGO0. The new fixture/helper source is separately
hashed. The inner ARM64 kernel runs under QEMU TCG, without KVM or physical USB
devices. These are real kernel/USB/IP/SSH controls, not ARM64 hardware-performance
or physical-peripheral qualification.

The complete current-input sequence passes all four controls in 240.700s.
The preceding complete sequence, using the earlier `03e1b21` CLI, passes in
254.608s and remains separately source-bound. Both include all 20 direct cycles
and the entire SSH lifecycle. The current cold build verifies eight bundled
files; both later builds reuse eight verified cache entries each. There are
zero source-download attempts. Actual readback verifies all 260 copied source/
configuration files, the executable and the explicit dependency patch.
All five unit suites pass on both macOS and Linux: 48 tests per platform.

## Execution boundary

Local admission requires an overlay-root Docker container with no effective
capabilities beyond Docker's ordinary defaults. Module-directory bind mounts,
added module-loading/mount/ptrace/reboot capabilities, unknown capability state
and any `GITHUB_ACTIONS` environment entry refuse. The actual containers have
no host mounts, host devices or published ports; each has a two-CPU quota,
4GiB memory limit and 512-process limit.

The guest must be root, have the exact `virtme-ng` hostname and virtme kernel
argument, identify QEMU virtual hardware, and have a canonical boot ID different
from its parent. These are operator-error guards, not authentication against a
hostile root operator. Actual negative invocations refuse an ordinary container,
the same boot, and a claimed different boot without virtme. Local execution
never sets `GITHUB_ACTIONS`; hosted execution retains its existing identity gate.

The original three module builds and four required controls are preserved:

1. Unpatched gadget: the specific held-native-reply loss must reproduce.
2. Corrected gadget with the same scheduling hook: discard the real held reply,
   acknowledge replacement import and verify exact payload.
3. Corrected ordinary gadget: all 20 direct import/read/write/detach cycles.
4. WebMux-over-SSH: dashboard pairing, controller-loss continuity, owner Stop,
   ordinary SIGINT and acknowledged worker/listener/USB cleanup.

Source acquisition pins, eight bundled files, cache verification, retries,
budgets, patches, source identities and all existing admissions remain intact.
The guest still has the 180-second timeout, 15-second kill grace and 210-second
outer observation bound. The original 30-second queue barrier is unchanged.
The local invocation also has a 780-second process bound, below the existing
15-minute CI job allowance. Each guest has the existing 1GiB/two-CPU settings.

## Preserved failures and #615

The first complete attempt failed after 142.283s: its unpatched negative control
was admitted, but corrected-held timed out waiting for a real native completion
to enter the held TX queue. Cleanup later reported two discarded replies. This
does not establish which operation produced them or the timeout's cause.

The changed-observer attempt passed unpatched, corrected-held and all 20 direct
cycles. Its SSH case failed because the local source snapshot omitted tracked
application defaults. Supplying the unchanged defaults/package inputs then
passed the entire standalone SSH control in 49.396s. That setup correction is
separate from the earlier queue timeout.

A bounded, read-only diagnostic now runs **after** that existing queue assertion
fails, then re-raises the same exception. It records the original/current counter,
hold state, reader exit/wait/stack, disk counters and at most eight allowlisted
kernel tasks. Reads are capped at 2KiB; the task scan stops after 256 numeric
entries. It reads no process arguments or environments. Its passing-path native
I/O and all original workload/cleanup assertions remain unchanged.

[#615](https://github.com/jordanhubbard/webmux/issues/615) remains open. A later
pass does not repair or explain the initial timeout; the failure-only observer
did not execute in passing cases. Both failed cohorts and all bootstrap failures
remain retained. #550 and release acceptance must not be declared complete from
the component/unit passes or a later green result alone.

## Local substrate and reproduction

Use a fresh private `ubuntu:24.04` container with the limits above; do not expose
host devices, mount host module directories or add capabilities. The run installs
the pinned `6.8.0-146-generic` image, modules-extra, headers and tools, QEMU ARM,
build tools, busybox-static, Python venv, OpenSSH, iproute2, kmod, cpio, zstd and
udev inside that container. It creates the fixture's locked `runner` account and
provides busybox's `poweroff` applet for guest shutdown. Host services and kernel
modules are untouched.

The [published virtme-ng1.41 source](https://pypi.org/project/virtme-ng/1.41/)
is digest-verified and installed only in the container's `/opt/virtme` venv.
Docker's host kernel command line supplies `virtiofs.poll_us=200`; its real
`modprobe --show-depends` output appends that option to the module filename.
The pinned dependency's greedy filename regex includes the option and prevents
initramfs creation. The explicit [local module-path patch](virtme-module-path.patch)
separates the filename from options. Original/modified hashes are recorded;
this run does not claim an unmodified virtme release. The patch touches no USB
source, admission, test assertion or deadline.

The sdist's optional prebuilt init is AMD64. Local mode selects virtme's supported
`--no-virtme-ng-init` shell-init path on the ARM64 guest. A deliberate exit7 boot
probe returns exactly7 and shuts down cleanly in 23.492s. Missing PATH/zstd/udev/
poweroff and wrong-init bootstrap attempts are preserved as failed preparation,
not USB evidence. The [upstream execution model](https://github.com/arighi/virtme-ng/blob/v1.41/README.md)
describes its disposable guest filesystem and emulation fallback.

Copy the complete tracked checkout, including `webmux/config.defaults/`, into
`/source`, and the current executable Linux ARM64 CLI into `/qualification/webmux`.
Apply the supplied dependency patch only inside the private venv, retaining both
source hashes. Then run inside the admitted container:

```bash
PATH=/opt/virtme/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  python3 /source/scripts/qualify-test-vudc.py --local-container \
  --kernel 6.8.0-146-generic --vm-runner /opt/virtme/bin/vng \
  --binary /qualification/webmux --build-root /qualification/native \
  --source-cache /qualification/source-cache
```

The build root must be fresh; existing private modules are refused. Preserve
failed roots and receipts. This run uses separate disposable containers for
complete cohorts, preserving the originals and checking the retained digest
before removing only the copied generated module in a derived fresh context.

Private raw inputs/results are under `_build/usb550-local-native/`: exact source
archives/manifests, package and dependency records, binary/build identities,
all boot/native outputs, process statuses and original failed cohorts. The public
receipt contains hashes and scope, rather than fixture keys, transient tickets or
raw application output. No hosted job, login, credential-store copy, physical
device, host-service change, release, version or tag is involved.
