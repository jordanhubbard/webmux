# Interrupted Windows MSI diagnostics

Qualification for [#685](https://github.com/jordanhubbard/webmux/issues/685).
The original [Windows packaging job](https://github.com/jordanhubbard/webmux/actions/runs/45137659456/job/137567698686) passed MSI/service qualification, then exceeded its 30-minute maximum during diagnostic collection. Its log is unavailable; the exact stalled operation is unverified.

Collection now runs only after failed/cancelled MSI qualification, in an owned child with a 20-second deadline. CIM also has a 10-second operation timeout. Progress remains outside the child so later artifact upload can retain timeout evidence. The collector preserves its field allowlist and 200-line log tails and refuses directory links. The MSI/service checks and package gates are unchanged.

[The receipt](receipt.json) distinguishes macOS PowerShell/filesystem/subprocess checks from pending native Windows qualification. A controlled removal of the directory-link guard fails the same filesystem test. The existing real hung-descendant test passes. No installed service, fleet policy or NAS access changed.

The [integration receipt](integration-2026-11-23.json) records subsequent native Windows qualification on original head `822218b`, including both packaging architectures and the full ARM64 workflow. All applicable original checks passed; the release-only attachment was skipped. The current-main composition retains identical script/workflow bytes and passes both local PowerShell suites again. Native Windows execution is attributed to its original input, without claiming a new whole-composition run. The original timeout and unavailable log remain unchanged. Publication and merge subjects use `[skip ci]` under the recovered local-resources constraint.
