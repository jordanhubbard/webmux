# Native Windows VT scheduler trace and clock discrepancy

A distinct diagnostic for [#536](https://github.com/jordanhubbard/webmux/issues/536)
uses the unchanged stock Go 1.27.4 Windows AMD64 executable from clean `5fadbbc`
(CGO disabled), all 19 hash-checked fixtures and the correct working directory.
The VT tree, module locks and native exit supervisor match main `93face2`.
Only `-test.trace=trace.out` is added to the full
`-test.count=10 -test.timeout=10m` invocation. No launch-time GC, GOMAXPROCS or
GOEXPERIMENT override is set. The existing allocation tests retain Go's ordinary
`AllocsPerRun` behavior, which temporarily selects GOMAXPROCS 1 and restores it;
the trace observes values 1 and 16. No test or runtime policy is changed.

| Check | Observed result |
| --- | --- |
| Native process | Exit 2 / `0x00000002`, 600.250 seconds; original ten-minute timeout fires. |
| Active test | `TestVTGen_Metamorphic` (reported active for 1 second). |
| Completed pass events | 17,060 of the reference 21,930; 1,644 identities have nine passes and 283 have eight. |
| Unreached identities | 266 of 2,193 reference identities have no pass; fuzz seed execution is unreached. |
| Existing skips | Same three identities, each observed nine times. |
| Assertion failures before timeout | None; this incomplete inventory still fails acceptance. |
| Native exit supervisor | All four controls pass, including high-bit Windows status retention. |
| Trace | 93,228,080 bytes; native and retrieved SHA-256 match. |
| Process cleanup | Same native PID observed while running; absent after completion. |

## Embedded clocks disagree with decoded trace time

The trace parser succeeds, but its complete decoded event span is 651.052134418
trace-clock seconds. More decisively, the first and last of its **585 embedded
clock snapshots** describe the same interval using three clocks:

| Clock | Last minus first snapshot |
| --- | --- |
| Decoded trace | 650.351537935 seconds |
| Embedded monotonic | 599.430936200 seconds |
| Embedded wall | About 599.430636 seconds |

Decoded trace time advances 1.084948 times as much as embedded monotonic time.
The snapshots cover slightly less than the process lifetime; the independent
native supervisor measured 600.250 seconds. All snapshot values are retained in
[clock-snapshots.json](clock-snapshots.json), including their original wall-clock
offsets. No post-hoc test2json conversion timestamps are used for this comparison.
The separate inventory conversion retains native PASS elapsed values and labels
its generated Time fields as post-hoc clocks.

The inspected Go 1.27.4 runtime has a separate tracing clock/conversion path on
Windows, and the trace format carries monotonic and wall snapshots. The
[receipt](receipt.json) pins the reviewed standard-library sources. This source
inspection and the observed mismatch do **not** establish which clock/conversion,
VM behavior or startup measurement is responsible. No frequency, SDK or host
setting is changed, and no corrected-duration or timeout-waiver claim is made.

The derived [scheduler](sched-flat.txt), [synchronization](sync-flat.txt) and
[syscall](syscall-flat.txt) reports retain their original numbers, prefixed with
a clock warning; cumulative reports are alongside them. For example, the
scheduler report totals 63.49 trace-clock seconds, synchronization 2,204.08 and
syscalls 1.82. These are aggregate delays, **not wall-time contributions**:
concurrent waits overlap, parent/cumulative rows overlap, and synchronization
includes normal test-parent and trace-helper waits. The parser also records
17,294 completed GC sweep-termination and 17,293 completed GC mark-termination
ranges. Two ranges remain active at trace stop and are excluded from completed
range durations. None of these observations proves a deadlock or timeout cause.

## Resources, retention and remaining work

Bounded guest observations show 10–31% CPU, at least 57.9 GiB available RAM and
at most 84.6 MiB native working set. C: remains a disk warning at roughly 17.3 GiB
free (about 3.5%): a bounded inspection of two task-owned evidence folders finds
69 files / 32,474,465 logical bytes, all retained. No guest overload is observed;
live VT job ancestry and hypervisor reservations, caps and overcommit remain
unknown. No host changes or cleanup deletion are performed.

The first archive transfer hit its inherited 45-second limit; its incomplete
fragment and a subsequent SFTP path-resolution refusal remain private evidence.
The same completed archive was resumed using the absolute Windows SFTP path and
verified against its full native SHA-256. The native workload ran once. Original
raw streams, native status, executable binding, fixtures, runner, trace, profiles,
clock snapshots, scripts and all failed attempts remain under
`_build/vt536-trace-current/`. Connection/account details and native absolute
paths are excluded from this report.

[#536](https://github.com/jordanhubbard/webmux/issues/536) remains open. Resolve
the clock discrepancy before interpreting trace magnitudes, then establish and
qualify any actual deadline correction with the full unchanged inventory. The
[earlier timeout and reader/profile evidence](../vt-reader-lifetimes-2026-11-18/README.md)
retain their original outcomes. This does not repair or attribute
[#450](https://github.com/jordanhubbard/webmux/issues/450), qualify the private full
suite, substitute for CGO-enabled hosted evidence, or complete recovery/release
acceptance. No hosted run, coding login, account copy, service/protection change,
SDK adoption, reboot, version, tag or release is performed.
