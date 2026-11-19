# Native Windows stop confirmation

`nativeprovider.Stop` now confirms exit of the original Windows provider owner
before returning success. The authenticated `stopping` response only
acknowledges the request; previously it could return success with a live owner.
This is a teardown prerequisite for [#541](https://github.com/jordanhubbard/webmux/issues/541),
which remains open. The public Stop/detach/delete workflow is not implemented by
this change, and deleting a local window still has the limitation recorded in
[the actual application qualification](../windows-functional-upgrade-2026-11-18/README.md).

The client opens a synchronize-only process handle before dispatch. It retains
that original process object through confirmation, so later PID reuse cannot
change the process it observes. It sends the existing authenticated stop frame;
it does not issue a process/tree kill. After acknowledgement it waits for owner
exit, honoring caller cancellation. An unconfirmed or unobservable exit returns
an error rather than success. A failure to open the observation handle refuses
before dispatch. Controller/viewer Close continues to detach.

## Native validation

Windows 11 Enterprise AMD64 10.0.26100 executed the Go 1.27.4 CGO0 test binary.
Compiler concurrency settings were confined to the Mac cross-build; the native
runner verifies no GOMAXPROCS/GODEBUG/GOMEMLIMIT/GOGC override. No authentication,
CLI installation, service, account, firewall, protection or reboot changes were
made for these disposable component checks.

- The acknowledgement-only parent implementation, built with the same new
  tests, fails the live-owner regression with `live owner was reported stopped:
  <nil>` (native exit 1, expected negative control).
- The changed implementation passes both new tests ten times: twenty passes,
  no failures/skips. The acknowledgement test waits for the client to receive
  the frame and close its connection while the observed test owner stays alive;
  cancellation must report an unconfirmed stop. The actual ConPTY test starts a
  root and child, calls authenticated Stop, and checks owner/root/child process
  handles are all signaled immediately after success.
- The full native-provider package passes all twenty source-defined top-level
  tests, yielding 76 unique test/subtest passes with no failures/skips. Existing
  test bodies are byte-identical after removing only the two added tests and
  import. No old operations, seeds, counts or deadlines changed.
- Windows AMD64 and ARM64 cross-builds and Windows-target vet pass. ARM64
  cross-compilation does not establish native ARM64 execution.
- All four recorded driver/test processes are gone, all fifty recorded temporary
  provider roots have been removed by fixture cleanup, and no test reports
  forced owner cleanup.

The [receipt](receipt.json) binds source-file and binary hashes, raw logs,
negative overlay, results and limits. These binaries were built from modified
source over `b3bed01`; the exact source hashes are the binding, with no clean
release-artifact claim. The private raw archive and parent-source overlay are
retained alongside the binaries.

The remaining application work must preserve a discoverable authenticated
session on detach, bind Stop to the original owned provider, persist intent
before dispatch, retain uncertainty after lost replies, and qualify failure,
restart, sibling/viewer and log-retention behavior. This component receipt does
not close #318/#438/#541, repair the private Windows VT timeout, or establish
full Windows policy or release acceptance.
