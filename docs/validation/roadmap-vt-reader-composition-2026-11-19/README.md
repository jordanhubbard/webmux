# VT reader cleanup composition

Merge `e6d9645b24b77ea7414487460c1737a332eaefb7` composes published owner
`0c75112d82df33d318266e3d6f232d1414867af6` onto canonical `6156a5a` in a separate
preparation branch. The incoming VT changes affect two test files and their
retained-source provenance. Production sources are unchanged. The complete VT
tree equals the published owner's tree; provenance verifies VT 241/19 and UV
104/7. The later merge `d7a6908f5f1910f6d2447faf7af22f98f7289fc7` also includes policy guard #540 (`4b0d4b0`).

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

[PR #540](https://github.com/jordanhubbard/webmux/pull/540) supplies the native
interactive-permission guard. Its four original hosted workflows pass with 19
raw artifact ZIP digests and NAS readbacks independently verified. The combined
source passes all five strict TypeScript projects, 252 helper tests, four affected
Go race suites and vet, plus both provenance checks. An initial client-install
helper test exceeded its existing 30-second limit; unchanged isolated and full
suite runs pass, and the original failure remains retained with cause unproven.
The production coordinator, API, recovery and frontend trees equal `4b0d4b0`.

The [packaged private service upgrade](../packaged-service-upgrade-2026-11-19/README.md)
qualifies actual packaged6156-to4b macOS service replacement and service SIGKILL
with unchanged workspace-write/never and real native tools. It retains the failed
first harness attempt. It does not qualify a notarized version upgrade or reboot.
Fresh combined Windows checks, review/integration, broader roadmap acceptance,
security/distribution requirements and release remain open. No runtime overlay,
version, tag or publication change is introduced.
