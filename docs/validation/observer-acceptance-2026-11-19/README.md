# Linux observer integration audit

The [receipt](receipt.json) audits #533 against the integrated implementation at
`93face27003d096e6c7dc4dea67240266ecebb6e`. All fourteen observer implementation,
fixture, owner-policy, dependency and workflow files are byte-identical to the
qualified input `0056000940ec0dbc96a010c3b88d349759c51c48`.

| Acceptance requirement | Verified evidence |
| --- | --- |
| Bounded diagnostics, including launch/initialization failures | Existing seven subprocess controls pass again locally. Bounded stderr, complete drain, original exit versus cleanup status, case/stage and source/native/helper bindings are retained; panic detection survives tail truncation. |
| No deadlock, lingering process or relaxed bound | Both retained native architectures contain twelve processes that stay alive until intentional cleanup, then exit zero without forced escalation. Existing RPC, cleanup and fixture bounds are unchanged. |
| Every-outcome upload and damaged-process rejection | The integrated workflow retains artifacts on `always()`. The current validator rejects both original panic-bearing cohorts and the original canonical31ac SIGKILL receipt. Independent damaged-receipt controls reject incomplete stderr, forced cleanup, pre-cleanup kill and panic for each architecture. |
| Diagnose the actual native ARM64 startup exit | **Partial.**  The actual signal-thread panic exposed denied addressless I/O on an already connected Unix pair. Its restricted repair preserves external networking/process/exec denials. A separate real-kernel ordering control proves live notification withdrawal, reproduces the old broker decision's pre-exec SIGKILL, and verifies replacement execution with all denials intact. Returned-exec errors retain their own error; stdout JSON and stderr diagnostics remain separate. |
| Native AMD64/ARM64 and reviewed integration | Original [run 44541568596](https://github.com/jordanhubbard/webmux/actions/runs/44541568596) is still successful on its exact source. Both original artifact ZIP digests and every raw-file hash were rechecked; each architecture passes six real Codex policy/startup cases, twelve healthy processes and all eight top-level kernel groups, including a positively admitted withdrawn notification. Exact-input checks remain successful, with only the inapplicable release-attachment step skipped. #568 integrated the qualified stack. |

A fresh local Ubuntu ARM64 guest additionally passes the same six real native
Codex 0.169.0 cases with the actual WebMux helper built at `ee13bce`, all eight
kernel groups, a native Go 1.27.4 race run and all seven Python diagnostic tests.
These executions use the same observer sources. The account is signed out;
none submits a model turn. The guest's Rig ABI and browser smoke tests are
preparation for the separate authenticated #523 qualification.

The [signal-path record](../codex-observer-signal-2026-11-18/README.md) and
[notification record](../observer-exec-notification-2026-11-19/README.md) retain
the causal controls and original failures. The first stderr-less EOF and the
exact branch or external cause of the original uninstrumented canonical31ac
SIGKILL remain unproven. No later pass rewrites either failure or proves its
historical scheduling. The controlled broker negative uses a private ordering
seam; it is not execution of an unchanged old binary. Production enables no
such hook.

The integrated bounded-diagnostics and reproduced observer-defect controls pass.
**#533 remains open:** the exact original stderr-less exit/SIGKILL cause is still
unproven. These controls complete neither that historical-causality requirement
nor authenticated Linux creation/recovery #523. Broader
#318/#438 lifecycle and platform requirements, Windows runtime failures and
whole-current-main/release qualification remain open. No hosted job, native
account copy, installed SDK/service/protection change or release mutation was
needed for this audit.
