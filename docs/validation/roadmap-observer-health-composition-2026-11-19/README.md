# Healthy observer and Windows fixture composition

This preparation retains canonical `47c7261`, Windows fixture correction
`cc8aaa8` (#535) and published owner `16cdbcf`. Its implementation merge is
`0d5dac6fd5f56f4c89f948de9944525f9825436b`; this receipt adds documentation.
Canonical draft #500 remains separate until fresh qualification succeeds.

## Corrected evidence boundary

Observer run [44429823595](https://github.com/jordanhubbard/webmux/actions/runs/44429823595)
on `47c7261` was green and its network/process-denial and policy assertions
passed. A deeper raw-artifact audit found all three guarded policy processes on
each architecture panicked in the native signal thread and required SIGKILL.
Those six processes cannot establish healthy signal handling. Original raw
artifacts, workflow outcomes and earlier scoped summaries remain retained;
[receipt.json](receipt.json) records the new rejection explicitly.

The earlier ARM64 stdout EOF had no stderr. This finding does not establish its
historical cause. The separate original Windows ARM64 activity-test failure is
retained under #535; the fixture now uses the production root-relative reader,
with a deterministic native DELETE-handle control and unchanged assertions.

## Published correction and independent checks

The #533 change allows only addressless, read/write-equivalent send/receive on
already connected descriptors, with restricted flags and canonical NULL
address/length words. Socket creation/connections, addressed I/O, message or
file-descriptor transfer, child creation, later exec and filter replacement
remain denied. It repairs the native signal pair's data path. The diagnostic
checker now rejects panic, forced cleanup, incomplete stderr and unexpected
exits; panic detection survives truncation of the retained tail.

The immutable component source `646c4ce` passed native
[run 44424131750](https://github.com/jordanhubbard/webmux/actions/runs/44424131750).
Independent artifact checks confirm six startup cases and twelve healthy
subprocesses per architecture on AMD64 and ARM64, no panics or forced kills,
complete diagnostics and preserved policy/network/process controls. Each native
Codex executable has the same hash as the corresponding failed `47c7261` case.
These are disposable unauthenticated homes with no model turn, not authenticated
Linux coding or recovery evidence.

The actual composed Python validator rejects both original panic-bearing
artifacts and accepts both corrected component artifacts. Seven subprocess
controls, Linux AMD64/ARM64 cross-compilation and full server vet, JSON/whitespace
checks and both retained-source provenance gates pass locally. Cross-compilation
is not native Linux execution. Fresh composed-head native/hosted qualification
is pending.

The composition changes only the Linux observer runtime, its kernel tests and
three diagnostic scripts relative to `cc8aaa8`, plus scoped documentation.
Frontend, native coordinator runtime and all retained VT/UV source are identical.
The imported never-approval recovery and private Windows VT deadline receipts
keep their original sources and limits; they are not fresh composed-head claims.
No complete roadmap or release acceptance is asserted by this preparation.
