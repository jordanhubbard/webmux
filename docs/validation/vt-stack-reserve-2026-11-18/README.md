# Windows stack-reserve comparison for #450

This is a diagnostic experiment for the retained Windows VT runtime crashes.
It does not adopt a patched Go runtime for WebMux releases or change any existing
CI requirement. A later passing repetition cannot establish the cause or repair
of an earlier crash.

## Pinned upstream input

`go-reserve.patch` is the exact patch for [Go CL 828724](https://go-review.googlesource.com/c/go/+/828724),
revision `5ec5e90237a6f364d60ad44d2f2d9c08597b8a8e`, retrieved November 18,
2026 while the change was **NEW**. The proposal sizes Windows exception reserve
from `InitializeContext` and accounts for that reserve when growing and shrinking
goroutine stacks. See [Go issue 81238](https://github.com/golang/go/issues/81238)
and [the Go 1.27 backport request](https://github.com/golang/go/issues/82978).
Upstream status can change after this snapshot.

`overlay-lock.json` binds the patch, Go BSD license, all 14 stock Go 1.27.4 source
files and all 14 resulting files by SHA-256. Preparation refuses changed inputs,
other SDK versions, existing output and output inside the SDK. It writes Go
source overlays outside the SDK and verifies that the installed source bytes
remain unchanged. The regular compiler and ordinary workflows remain stock.

## Native experiment

`windows-vt-reserve.yml` runs one cohort of four native Windows AMD64 workers.
It runs for diagnostic-file pull request changes or an explicit dispatch; use
one trigger per cohort. Pull requests check out and report the actual head SHA.
Each records its actual processor, enabled XSTATE, `InitializeContext` sizing,
compiler identity, source revision and resource headroom. Worker allocation does
not guarantee an AMX processor.

Each worker builds four actual AMD64 PE executables:

1. The unchanged full VT test suite with stock Go 1.27.4.
2. The identical suite with the pinned reserve overlay.
3. The proposed runtime's three Windows exception-reserve regression tests.
4. Those runtime tests with only the new shrink-headroom condition removed, as
   a deliberately broken control in a separate overlay.

The runtime tests must all pass without skips. The broken executable must exit
1 specifically because less than the forced 30,000-byte reserve remains below
SP, after an actual observed stack shrink. A crash, missing output, unchanged
stack, different test failure or vacuous success does not satisfy this control.

Both full VT executables run the original ten repetitions, seeds, operations,
assertions and GC behavior. Each successful phase must retain 21,860 individual
test passes and the same three existing skips, ten times each. Their passing
test multisets must match. Native exit words, JSONL, source overlays, worker
context and binaries are retained even on failure.

The independent verifier fails the workflow when either VT phase fails or any
required evidence is incomplete. A stock failure with a reserve pass is recorded
descriptively and still fails the workflow. Two passing phases are classified
`both-passed-no-attribution`. Every report keeps
`originalFailureRepairProven: false`.

## Local admission

The five overlay admission controls exercise the real installed Go 1.27.4
sources, changed-source refusal, changed-patch refusal, version refusal, SDK
protection and preservation of prior output. Existing native process-supervisor
controls and retained VT provenance checks also apply.

Local cross-compilation uses a private SDK copy outside `GOMODCACHE`, because Go
correctly refuses source overlays within its module cache. The installed SDK is
preserved. PE architecture and reserve markers are inspected locally; compiling
on macOS is not native Windows execution evidence. Native receipts belong to
the exact dispatched source revision and must be assessed separately.

The initial local admission completed all five overlay controls and all four
Windows AMD64 cross-compilations. The stock VT image has no reserve marker; the
other three images contain it. Three portable supervisor controls passed; its
Win32 high-bit status control requires native Windows and was skipped on macOS.
Retained VT provenance passed for 241 upstream and 17 explicitly patched files.

The initial native run [44428491622](https://github.com/jordanhubbard/webmux/actions/runs/44428491622)
failed admission on all four workers before compiling or executing the comparison:
constructing the negative file through Python's Windows locale changed UTF-8
source bytes, and the pinned hash correctly refused them. The correction uses
byte reads and replacement throughout; none of the expected source hashes or
runtime assertions changed. The failed run remains evidence of a diagnostic
setup failure, with no stock/reserve outcome.
