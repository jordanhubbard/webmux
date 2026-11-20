# USB held-completion counter correction

The disposable gadget's scheduling hook missed completed native URBs that were
already queued when the hold was armed. Its counter advanced only at enqueue
when the hold flag was already set. Holding an earlier reply can block the next
SCSI stage, leaving the counter at zero while a real completion remains held.
This is a concrete defect in the queue-barrier observation under #615.

The corrected hook marks each `TX_SUBMIT` item once under `lock_tx`, both at
enqueue and while TX is held. A dedicated boolean in the test queue item prevents
repeated 10ms hold-loop visits from counting it again. Empty queues and unlink
replies cannot advance this counter. The actual completion, USB/IP framing,
transmission hold, stale-reply correction and payload remain unchanged.

The final candidate's complete six-control qualification succeeds in 343.204s.
Its original four controls all meet their required outcomes; the legacy causal
control fails at the required barrier and the fixed causal control passes with
`held=1`, `queued=1` after repeated hold iterations. All 51 fixture unit tests
pass on both macOS and Linux. Current cold acquisition verifies eight bundled
files; the four subsequent builds each reuse eight verified cache entries, with
zero source downloads. The complete results and original exit codes are in the
receipt; expected negative executions retain exit1.

## Causal evidence

An additional, independent TX pause first queues a **real native completion**
with the original hold disabled. After verifying that completion and a blocked
reader, the fixture arms the original hold and removes the independent pause.
Both module variants run the same workload; no USB/IP request or response is
manufactured.

The private paired prototype demonstrates:

| Hook | Actual prequeued completions | Hold counter | Outcome |
| --- | ---: | ---: | --- |
| Legacy | 1 | 0 | Original 30-second completion-queue barrier fails; reader remains blocked. |
| Count each held completion | 1 | Positive | Original detach/reimport/discard/exact-payload admission passes. |

The common prototype workload hash and actual ARM64 ELF/module identities are
in [receipt.json](receipt.json). This comparison finishes in 100.917s. Its first
attempt incorrectly required two queued completions; the observed state was
one completion, with `dd` blocked in device-open `TEST_UNIT_READY`. That
unadmitted preparation failure and its diagnostic attempt are preserved. The
one-completion precondition is a separate diagnostic setup; the original native
barrier remains `held > before` with its original 30-second allowance.

The observed completion can belong to SCSI readiness during device open, before
the reader's payload-read syscall. It is still a real kernel USB completion.
The later reimport/exact-payload check remains independent and mandatory.

## Permanent regression and full qualification

The ordinary four controls remain mandatory and unchanged outside explicit
new-mode dispatch:

1. Unpatched gadget must reproduce the specific stale-reply failure.
2. Corrected held gadget must discard stale native replies and reimport/read.
3. Corrected ordinary gadget must complete all 20 direct read/write/detach cycles.
4. WebMux-over-SSH must complete dashboard/controller-loss/Stop/SIGINT/cleanup.

Two additional controls use separate builds with an explicit prequeue patch.
The legacy scheduling patch reconstructs the original hook's applied C source;
it is available only for this corrected-gadget causal comparison. Its specific
barrier failure is accepted only with an actual queued completion, hold enabled,
independent pause disabled, zero counter, blocked reader and no handshake/kernel
fault. Other failures refuse qualification.

The current hook must pass the entire original repair admission, plus compare
its count with the actual queued `TX_SUBMIT` count after repeated hold iterations.
Missing prequeue evidence or an inflated count cannot qualify. The independent
pause and queue sampler are absent from the ordinary module builds.

All original admission functions, original unit-test classes and ordinary native
I/O/cleanup statements are retained. Literal source reversal removes only the
explicit new argument/branches/checks to reconstruct the original workload.
The source manifest, eight upstream files, SHA/size checks, cache handling,
real stale-reply/dequeue patches, retries and existing deadlines are unchanged.
Only the test scheduling patch changes its counting rule.

Native execution uses the same explicit local-container/separate QEMU ARM64
Linux6.8.0-146 context as [the previous qualification](../usb-local-native-2026-11-19/README.md).
It retains the private virtme1.41 module-path parser correction and shell init,
two-CPU/4GiB container limit and 1GiB/two-CPU guest settings. No hosted identity,
host device/mount, added privilege, coding account, host service or release
mutation is used. The current WebMux executable is the previously independently
hashed Go1.27.4 Linux ARM64/CGO0 image; runtime Go remains byte-identical.

## Historical scope

The original uninstrumented corrected-held timeout remains a failed execution.
Its exact interleaving was not retained. Its symptoms are consistent with the
proved counter gap; the paired control directly establishes that gap and its
repair, rather than retroactively establishing the old thread schedule.
The earlier SSH failure was a separate omitted-defaults setup error and remains
separately recorded. Later unchanged passes alone were not used as a repair.

Private evidence under `_build/usb615-causal/` preserves prototype source,
both failed two-completion setup attempts, paired raw kernel/proc observations,
source manifests and the final candidate's complete native results. Original
records under `_build/usb550-local-native/` remain unchanged. Public receipts
project outcomes/hashes and scope, excluding transient fixture tickets/keys and
unrelated process data. Broader roadmap/release acceptance remains separate.
