# VT test response-reader lifetimes

The conformance reply and Kitty response helpers started readers without closing
their emulators or joining those readers when the owning tests ended. Real-emulator
probes on parent `16cdbcf` reproduce one retained goroutine for each helper after
owner return. Explicit parent Close releases each reader. The original Windows
timeout stacks also retain these readers from completed tests.

The helpers now register emulator Close and reader join with their owning test.
Reply forwarding also observes cancellation while its bounded channel is full.
Cleanup fails if the reader has not joined within one second. This is a new
teardown check; the existing 300 ms reply wait and full-run deadline stay intact.
Production code is unchanged.

| Control | Result |
| --- | --- |
| Original lifetime probes | Both fail: 0 readers before, 1 after owner return; explicit Close restores 0. |
| Corrected lifetime probes | Both pass: 0 readers remain after owner return. |
| Remove owner cleanup in a disposable source overlay | All three committed lifetime checks fail. |
| Remove only reply-send cancellation | Full reply-queue cleanup fails; the blocked-read case still passes. |
| Final-source lifetime tests, race detector, 100 repetitions | Pass. |
| Full VT/snapshot/vtgen race suites and vet | Pass on Go 1.27.4. |
| Retained VT and UV provenance | Pass: 241/19 and 104/7 upstream/patched files. |

The new tests check completion before owner return and the closed emulator for
both a blocked read and reply-queue backpressure. All pre-existing test bodies
are byte-identical after normalizing helper-call arguments. Their response
expectations, inputs, assertions, seeds and generated-operation counts remain
unchanged. The [receipt](receipt.json) records that inversion, source hashes,
negative controls and local logs. No test filter, count, deadline or GC setting
is relaxed. Changed-source native qualification must preserve every old test
identity and add the lifecycle controls.

## Original-binary Windows CPU profile

A separate diagnostic run uses the unchanged stock Go 1.27.4 Windows AMD64 test
executable from `f4c2651`, all 19 hash-checked runtime fixtures and the correct
working directory. It retains the full ten repetitions and ten-minute deadline;
only CPU-profile collection is added. The four native supervisor controls pass.
The test process reaches its deadline and exits 2 after 600.578 seconds, active
in `TestUnicode_ClusterSurvivesAWriteBoundary`. Its incomplete pass inventory is
excluded from acceptance. The previous unprofiled timeout and invalid-fixture
attempt remain retained separately under [#536](https://github.com/jordanhubbard/webmux/issues/536).

The [flat report](profile-top.txt) and [cumulative report](profile-cumulative.txt)
cover 600.01 seconds and report 783.62 sampled CPU seconds. Flat samples include
14.46% in runtime.semasleep and 12.05% in runtime.semawakeup; runtime.gcDrain has
30.78% cumulative samples. Flat and cumulative totals overlap. This records
substantial synchronization and GC work, but establishes no timeout cause or
causal contribution from the reader leak. Instrumentation can affect timing.
Only the Build ID header containing a private native path is removed from the
published reports; the original reports, executable, profile and raw archive
are retained by hash.

The changed-source native full inventory and hosted gates remain pending after
a clean commit/build. The reader-resource defect is demonstrated independently;
neither this patch nor the profile attributes or repairs the original Windows
corruption in [#450](https://github.com/jordanhubbard/webmux/issues/450). Broader
recovery, review/integration and release acceptance remain open.
