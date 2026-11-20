# Windows Codex process-owned journal discovery

Issue #627. Native Windows AMD64 and ARM64 controls pass for the new Codex-only
observer. A built ARM64 WebMux metadata helper also identifies the exact journal
held open by the existing, authenticated native Codex 0.169.0 process, both with
its normal executable name and as `codex.exe`. These checks neither submit a
model request nor restart the native conversation or controller.

## Source and binary binding

[receipt.json](receipt.json) records the modified development checkout's base
commit, tested source digests, Go version and executable hashes. These are
development builds, not released-package or clean-commit qualification. Each
native test executable's remote hash matches its local build. The live helper's
remote hash matches the local app build and its alias. Private raw receipts
retain the process lifetime, selected path and expected native journal binding;
public receipts omit account, host and conversation identifiers.

## Native results

Command on each Windows target (PowerShell):

```powershell
& .\recovery627.test.exe @(
  '-test.v', '-test.run=TestWindowsCodex|TestCodex|TestOriginalCLIWindows',
  '-test.count=1', '-test.timeout=2m'
)
```

The run expression is one literal argument in the launcher. Both targets have
14 passing top-level tests, no failures and one skipped Unix-only descriptor
test; the seven new Windows observer tests all execute and pass. Public logs
omit the one line of private fixture paths:
[AMD64](windows-amd64-tests.log), [ARM64](windows-arm64-tests.log).

Controls cover the owned journal with a newer closed competitor, another
project, a foreign Codex profile, multiple owned conversations, a mismatched
creation time, an exited process, a descendant-owned journal, an oversized
handle table, invalid file handles, helper timeout, response overflow, partial
JSON and mismatched response identity. A duplicated production metadata handle
cannot read or write a canary's data. Native test/helper processes are absent at
collection. The original qualification app, Codex and policy listener remain
separate from that cleanup check.

The parent retains the original CLI's OS lifetime before starting its waiter.
Descendant parent IDs and creation order are verified against pinned live
ancestors; an unobservable or unsupported scan returns no current confidence.
There is no newest-file fallback or privilege escalation. `GetFileType` handles
valid unknown device types using its documented last-error contract, without a
device-name allowlist.

Local recovery/main race suites, full server vet and Windows AMD64/ARM64
recovery/main vet pass. Hosted CI was not dispatched under the owner's quota
restriction. Earlier rejected development probes remain in the private working
area: the positive fixture initially failed closed on valid unknown device
handles, and a later live-process check caught additional device classes before
the raw documented API distinction replaced the temporary allowlist. The final
receipt binds the corrected implementation.

## Open acceptance boundary

Authenticated Windows coding and recovery remain open under #318/#438 and
[#628](https://github.com/jordanhubbard/webmux/issues/628). After the owner
approved default sandbox setup, an actual authenticated ARM64 Codex turn with
read-only/on-request policy failed with:

```
Failed to create unified exec process: timed out after 15000ms connecting runner pipe-in
```

The requested command also contained an unintended trailing `.` argument. It
remains a failed request. The pinned runner source connects its pipes before
sending the command; the observed failure precedes that handoff. The nonce read,
write-denial probes and loopback-denial probe did not execute, and their outcomes
are unknown. No identifier supplied in a prompt counts as tool nonce recall.
No controller-loss, reboot, packaged-install, or authenticated recovery pass is
claimed here. The runner's startup cause requires separate evidence.
