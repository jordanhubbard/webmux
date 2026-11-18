# Nil grid diagnostic labels

The two grid property tests format generated `*uv.Cell` pointers in operation
labels, including nil pointers. Cell implements fmt.Stringer, and its String
method reads the receiver's Content without a nil check. Go 1.27.4 fmt calls
that method and recovers the nil panic before printing `<nil>`. The retained
native Windows AMD64 stock executable independently disassembles this method
to a receiver-relative load (`MOVQ 0(AX), CX`), with no receiver guard.

Only six diagnostic arguments now use an untyped nil for a nil cell; non-nil
cells retain their original formatting. Grid operations still receive the
original pointers, including nil. Removing the helper and unwrapping those six
arguments restores the parent's test file byte-for-byte: all seeds, generated
operations, assertions, GC, readers and registered test identities are retained.
Production terminal/UV code, required CI commands, SDK and dependencies do not
change. The test-file patch hash is explicitly recorded in VT provenance.

A disposable local build overlay counts nil String dispatches and hashes all
diagnostic labels while the original strict grid assertions run. Before: 2,486
nil dispatches; after: zero. All 36,040 labels have identical SHA256
`b6b7477a3f6d11b0a65d071ebbcb14d9f74b4cc2bc3ea5c8ff97c8d86fedb82c`.
The counter measures method dispatch, not Windows exceptions or heap writes.
Instrumentation is confined to private source copies and is absent from this
patch and the qualified native test binaries. Full VT/snapshot/vtgen races and
vet pass locally; VT/UV provenance controls pass. AMD64/ARM64 Windows PE tests
cross-compile, which is not native execution.

This removes an avoidable nil-fault trigger from test diagnostics. It does not
establish that this caused the earlier Windows corruption/termination, repair
the Go runtime, or exclude other hardware faults. Upstream
[Go issue81238](https://github.com/golang/go/issues/81238) concerns recovered
hardware faults and a host-dependent Windows exception stack. Keep #450 open.
The original failures and all unchanged native/CI/audit gates remain retained.
Exact-head native qualification is pending; do not substitute a later passing
run for attribution of the original failure or claim release readiness.
