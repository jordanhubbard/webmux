# Isolated native Windows VT runtime diagnosis

[#450](https://github.com/jordanhubbard/webmux/issues/450) retains a new failure
class on combined source `45151c356368b52eb20ccea92a34866472041e31`, CI
`44273583502`, Windows job `134695984223`. The unchanged native command
`go test -json . -count=10 -timeout=10m` terminates after 166.224 seconds with
`fatal error: traceback did not unwind completely` during GC stack scanning.
`TestEmptyPaneCostsNoGrid` is active; the scanned response-reader goroutine was
created in `kitty_payload_test.go`. No individual test failed before that runtime
fatal. The test and its forced GC are retained unchanged.

Artifact `14413378918` SHA-256
`d8f9afbfd82b32c77077673824d02a7a03fbf7eebca019884f75cd33a89bb93d`
contains 57,954 valid JSON events, including 13,497 test starts, 13,473 passes,
23 skips and one package failure. This is not assumed to share the earlier grid
mismatch or makeslice panic's cause.

[Go issue 81238](https://github.com/golang/go/issues/81238) describes Windows AMD64
exception-recovery writes below a goroutine stack on certain extended-state CPU
configurations. [Its 1.27 backport issue](https://github.com/golang/go/issues/82978)
is open. The failed WebMux job did not retain CPU/XSTATE or a fault-origin witness;
the upstream report is a hypothesis, not an attribution.

The separate diagnostic workflow retains CPU model/core counts, enabled XSTATE
bits, native OS/tool admission, worker headroom, exact source head and both test
outputs. It compiles the unchanged VT test inventory with official Go 1.27.4 and
runs its retained executable through the supported `go tool test2json` driver.
Both phases keep all cases, ten repetitions and the original ten-minute test
deadline. The new diagnostic job has a 30-minute aggregate limit.

The second executable uses Go's supported build overlay to compile only a private
copy of `src/runtime/signal_windows.go` with the pinned fail-fast guard. Neither
the installed SDK nor compiler is modified. Stock/guard executable hashes and
absence/presence of the diagnostic guard string establish which executable is
actually run. No project/retained-VT source, required assertion or deadline changes. The
existing Windows repetition step also retains CPU/XSTATE context for future
failures; unavailable queries remain explicit and do not skip the original test.

The patch is [CL 863173](https://go-review.googlesource.com/c/go/+/863173), revision
`071f5c2ee6820eef88de81c20700d9772d64e4b8`, a pending 1.27 backport of the reviewed
[merged master change](https://go-review.googlesource.com/c/go/+/824724).
Its eight added lines detect an already out-of-bounds exception-dispatch stack
and fail immediately. They do not prevent the underlying overrun. Retain the
upstream attribution and [Go license](GO-LICENSE.txt). Patch and pre/post source
SHA-256 checks fail closed; compiler and executable hashes distinguish the private diagnostic test
build even though its inherited version string remains Go 1.27.4.

Local verification applies the pinned patch to a private copy of the installed
1.27.4 source and obtains the expected exact Git blobs (`40547b8` to `3423a66`),
without modifying the installed SDK. A real Windows AMD64 runtime-test PE cross-compiles with the overlay and contains
the diagnostic guard string. YAML, original-gate invariants and whitespace checks
pass. These are source/compilation checks, not native Windows execution. Native diagnosis remains
pending. The diagnostic does not install a replacement toolchain, change project
requirements or waive an existing CI/audit/release gate. Its final status fails
if stock, build or guard execution fails or does not complete. A passing guarded
run is not a repair, and two passing runs mean no failure was reproduced on that
worker.

The first native probe, `44280465179` on `f150801`, did not execute the VT
inventory: PowerShell split its unquoted dotted test flags (`-test` was rejected).
The overlay admission then rejected checkout-converted patch bytes. Artifact
`14417165945` retains those harness failures and an AMD EPYC 9V45/XSTATE `0x8e7`
worker context, not evidence of a VT failure or of absence of the suspected bug.
The corrected workflow quotes each test flag and byte-preserves the pinned patch
through Git attributes. A simulated Windows CRLF checkout now preserves the
expected patch SHA-256; it previously changed all 52 line endings.
