# Required Windows application fixture on hosted runners

Canonical5b CI exposed a second-hop owner-start access denial. The same unchanged
application test passes against its original Packaging executable on the private
Windows user account. Separately, published d4 diagnostics observe a hosted
bootstrap immediate job that disallows breakaway; the original failure remains
retained. This is an environment boundary requiring a suitable positive test
fixture, not evidence that fixture-only browser checks establish shell operation.

This composition adapts published025d245/1a1aa14. Both full Windows CI and manual
Windows terminal qualification now require the native application gate in a
short-lived SYSTEM task on a disposable hosted VM, matching the existing native
ConPTY collector boundary. The launcher refuses non-Actions/self-hosted hosts,
requires the actual clean checkout and a fresh RUNNER_TEMP directory, builds the
normal application, and binds image/test bytes by SHA-256. The worker verifies its
actual SYSTEM identity, uses isolated cache/configuration and a loopback listener,
and removes its temporary task afterward. The prebuilt image requires its exact
digest; ordinary browser tests retain the normal build path.

The canonical application test is byte-identical to5b, renamed only for explicit
native discovery. It still requires public connected state, fresh executed output
absent from literal input, authenticated original-selection Stop, owner/root exit,
then DELETE204/GET404. Its four-assertion attachment is required evidence. Native
configuration rejects non-Windows hosts. Normal fixture discovery excludes the
native filename, paired with the mandatory positive full-CI step. No assertion,
startup or test deadline, authentication, launch flag, lifetime mutex or uncertain
recovery rule changes. The original native failure is not rerun to claim repair.

Only public evidence (image/source/test/account binding, reporter output/traces
and fixture executable) is uploaded. Private cache and session/recovery-key state
are excluded. On uncertainty the test fails and retains its recovery access;
there is no broad provider kill. No native sign-in state or coding turn is used,
and no installed service/account/protection/default-sandbox setting changes.
Synthetic SYSTEM coverage remains separate from authenticated user-account
conversation, policy, service/MSI, reboot and ARM64 qualification.

The published owner025d hosted run44468055939 is historical evidence for its own
gate. Our adapted fixture keeps the different canonical5b gate and needs a fresh
exact-source hosted result. Local checks and that run will be bound in a separate
composition receipt. No prior positive is relabeled as this composition passing.
