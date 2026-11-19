# Linux native observation guard

Follow-up [#533](https://github.com/jordanhubbard/webmux/issues/533) found a gap
in the earlier startup evidence: the helper allowed unnamed socket pairs but
blocked the addressless send/receive forms used by native signal delivery.
The new bounded stderr fixture exposed a signal-thread panic and forced cleanup
in receipts whose policy assertions had passed. Those earlier receipts retain
their original results; they do not prove healthy signal handling.

The #533 repair admits only addressless, read/write-equivalent send/receive
forms with restricted flags and canonical zero address/length words. External
socket creation, connect/bind/listen/accept, addressed I/O, message/descriptor
transfer, child creation and later exec/filter replacement remain denied. New
kernel controls exercise real signal-pair data I/O plus both positive and
refused address/flag/message forms. The native fixture now refuses observed
panics, forced cleanup, incomplete stderr and unexpected exits; panic detection
survives truncation of the retained 32 KiB tail. The separate [native signal-path qualification](../codex-observer-signal-2026-11-18/README.md)
passes on AMD64 and ARM64 with no panic or forced cleanup. The original early
stdout EOF had no stderr; its exact historical cause remains unproven.
The dependency's [UnixStream signal pair](https://raw.githubusercontent.com/vorner/signal-hook/v0.3.18/src/iterator/mod.rs)
provides source context for the recorded native failure.


The internal Codex owner resolver now has an enforced Linux AMD64/ARM64 guard,
provided by the actual WebMux executable. An explicit native-create action
snapshots the running controller into a private directory under the owner
namespace and writes a mode0600 reference with its SHA-256. Snapshot directories
are mode0700; the immutable executable is mode0500. An installed-path upgrade
cannot replace the selected helper during account lookup. Existing changed
snapshots refuse replacement. Neither the helper reference nor its path enters
the coding launch environment or Rig member request.

The resolver validates the pinned helper before account lookup and again before
policy inspection. The no-thread account stage retains native network access
for workspace routing. The separate policy stage enters the helper's private
CLI mode, verifies its own loaded image and the selected native ELF digest,
and executes that verified file descriptor. The original helper PID becomes
the native CLI PID. The actual coding process is launched independently with
the original owner's permission policy and environment.

The Linux filter permits runtime threads, ordinary file operations and unnamed
Unix socket pairs needed by Tokio signal handling. It denies child creation,
external socket creation/connections, io_uring, cross-process inspection and
filter replacement. clone3 returns ENOSYS so native threading can use clone;
clone requires the thread/VM/signal-handler flags. ABI checks reject other
architectures and the x32 syscall convention.

A one-shot kernel notification permits only the helper's initial file-descriptor
exec, from its locked exec thread. The listener is close-on-exec and all helper
threads disappear on exec, so subsequent exec requests fail without a resident
broker. The kernel documents the [notification and ABI rules](https://docs.kernel.org/userspace-api/seccomp_filter.html)
and tests [closed-listener ENOSYS behavior](https://kernel.googlesource.com/pub/scm/linux/kernel/git/torvalds/linux/+/e521f516716de7895acd1b5b7fac788214a390b9/tools/testing/selftests/seccomp/seccomp_bpf.c).
The helper permits no later seccomp/prctl filter replacement. This is targeted
startup enforcement for the qualified native CLI, not a general filesystem
sandbox or a permission policy for coding tools.

The original [receipt](receipt.json) records the earlier Linux ARM64 kernel
and policy-snapshot controls. Use the #533 qualification above for healthy
native signal delivery; the original artifacts retain their original scope. Native ordinary and race tests verify usable
runtime threads/private signal pairs, positive and denied network/child/exec
operations, same-executable denial, filter-replacement denial, unchanged PID,
closed listener, executable-digest refusal and private immutable references.
The actual coordinator bridge verifies that Node receives the controller's
owner-private reference before executing the native action. Native CLI controls
cover full-access, read-only and workspace-write policies, including roots,
network=false and temporary-directory exclusions. Late stdio and HTTP MCP
servers start in the positive cases and do not start under the guard.

The shared native fixture launcher now sets its physical cwd, and each case
independently asserts the returned native project and workspace roots. Pairwise
policy equality alone could otherwise accept the same wrong directory twice.
The full resolver also refuses a missing native account, and the native project
trust control refuses disabled project configuration without granting trust.
No default case copies authentication or submits a coding turn.

A native Linux AMD64/ARM64 workflow builds the actual controller helper, checks
OS/Node/Go/ELF architecture, records worker headroom and pinned npm native-package
provenance, runs the kernel race controls, then runs the maintained native CLI
qualifier. It preserves every outcome. These jobs qualify native startup and
permission snapshots; they do not claim authenticated coding or recovery.
The [runner labels](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
select native Linux architectures. Run the qualifier manually with:

```bash
python3 scripts/qualify-codex-observation-isolation.py \
  --codex /absolute/path/to/native-codex --node /absolute/path/to/node24 \
  --helper /absolute/path/to/current-webmux --output /private/receipt.json
```

On macOS, omit `--helper`; policy inspection continues to use its native OS
guard. `--authenticated-cwd /absolute/project` optionally uses an existing native
account in place. Authenticated inspection and fresh coding after creation-helper
loss have been recomposed on macOS, retaining original native enrollment,
environment, duplicate controls, sibling survival and checked Stop. The rebuilt
browser/controller namespace check also retains one original project through a
lost native reply and controller restart. These are separate evidence scopes.

Linux authenticated creation/environment-handoff/conversation recovery remains
open under [#523](https://github.com/jordanhubbard/webmux/issues/523): the current
Linux qualification account is not signed in. Public native-owned member
creation/readiness/attachment/uncertainty and recovery remain incomplete under
#438/#318. Canonical empty-pod selection, selected profiles, named graphs,
broader startup settings and other native platform acceptance remain open.
The prior pushed candidate's legacy Rig wheel and dashboard CI failures are
tracked in #524 and #525; they are not relabeled as guard fixes. Review,
dependency audit #429 and full roadmap/release gates remain required.
