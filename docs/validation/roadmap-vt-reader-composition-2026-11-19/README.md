# VT reader cleanup composition

Merge `e6d9645b24b77ea7414487460c1737a332eaefb7` composes published owner
`0c75112d82df33d318266e3d6f232d1414867af6` onto canonical `6156a5a` in a separate
preparation branch. The incoming VT changes affect two test files and their
retained-source provenance. Production sources are unchanged. The complete VT
tree equals the published owner's tree; provenance verifies VT 241/19 and UV
104/7. The canonical PR #500 remains separate.

Go parser AST ranges independently establish that all thirteen pre-existing
test bodies in those two files are byte-identical after normalizing only the
helper argument and extra-return assignment changes. New tests require readers
to exit at owner cleanup, including reply-channel backpressure. The original
response waits, expectations, seeds and operation counts remain intact.

Both published workflows were independently bound to implementation `5fadbbc`:
[full CI](https://github.com/jordanhubbard/webmux/actions/runs/44434794421) has eight
successful jobs, and [stock/guard diagnostics](https://github.com/jordanhubbard/webmux/actions/runs/44434798810)
has verified actual AMD64 PE hashes, guard absence/presence and native exits zero.
All nine original ZIPs pass GitHub SHA-256 and NAS readback checks.

The required Windows CI and both diagnostic variants each retain every original
2,186 passing test identity ten times, plus seven lifetime identities ten times:
21,930 pass events and the same three existing skips repeated ten times. All
three pass/skip multisets agree. The required worker reports AMD EPYC and XState
0x8e7; the diagnostic worker reports Intel Xeon 8573C and XState 0xe7. Neither is
the original #450 AMX context 0x608e7.

The actual composed macOS VT/snapshot/vtgen race suites pass in
116.759/1.232/1.269 seconds, followed by successful vet. Source, log and inventory
bindings are in the [receipt](receipt.json).

The imported native Windows reports preserve two failures. #536's private VM
still exceeds the unchanged full-run deadline despite verified reader cleanup;
there is no timeout-cause, private-suite-pass or #450 corruption-repair claim.
#538's legacy unelevated coding preflight denies canary writes but allows its
private loopback connection, failing the planned strict network prerequisite;
no application upgrade or recovery case was started by that preflight.

Separate [PR #540](https://github.com/jordanhubbard/webmux/pull/540) contains the
native interactive-permission guard. It is not part of this preparation merge.
Fresh combined Windows checks, review/integration, broader roadmap acceptance,
security/distribution requirements and release remain open. No runtime overlay,
version, tag or publication change is introduced.
