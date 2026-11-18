# Combined browser and USB qualification

Continue canonical [#500](https://github.com/jordanhubbard/webmux/pull/500)
under [#452](https://github.com/jordanhubbard/webmux/issues/452). The pinned
[receipt](receipt.json) retains the previous canonical commit and all inputs:
#506 closed process-pipe relay handling, #509 owned iframe actions, #512
verified Windows browser process exit, #515 sysfs resolution race handling,
and #516 in-workspace docking/layout. The inherited #503 diagnostic-label
change remains present without claiming an original #450 runtime repair.
All merges are conflict-free; every pinned input remains an ancestor.

Source comparisons confirm the frontend tree is identical to #516, the iframe
and CI paths to #509, shutdown/worker/manager to #512, and relay code/tests to
#506. No helper policy, dependency lock, gate or release version changed.

Local checks on the composed source:

- All 425 frontend tests in 43 files pass, with five strict TypeScript projects,
  frontend lint, helper compilation and the full application build.
- All 31 server test packages pass under race; host vet passes. All 129 helper
  tests pass with zero skips.
- Actual macOS Chrome with the production allocator: 30 native browser test
  pass events, including all 25 frame subcases, saved-cookie persistence, worker
  cleanup, authorized End failure reporting and pre-target trace-owner lifetime.
  No skips or failures. Go/frontend source trees remain identical across the
  later test-only viewport assertion correction.
- Four actual Go/Chromium Playwright workflows pass together: accessible root/
  iframe actions and refusal controls, mobile/touch, docking lifecycle, and
  interactive workspace with navigation, dialogs, clipboard and files.
- Thirteen USB discovery/readiness and nine qualifier admission/cache tests
  pass. The corrupt fixture emits its expected traceback while its test passes.
- Windows AMD64 vet and browser test cross-compiles for Windows AMD64/ARM64
  pass. These are cross checks, not native Windows runtime acceptance.
- Affected workflow YAML syntax and whitespace checks pass.

The first composed Playwright run passed iframe, mobile and workspace cases
but read geometry before React processed a viewport resize in the layout case.
The retained failure led to #516 commit `b4a8358`, which waits for settled bounds
at both viewport transitions. Expected values, lifecycle/lease assertions and
production code are unchanged. The corrected feature case and all four composed
cases pass; the initial failure remains retained separately. The original
#516 Linux job `134870159060` independently reproduces the same line-114
intermediate geometry on `4fac69f`; its log is retained. Fresh hosted results
for the correction remain required.

Component hosted receipts retain their original source/platform scope. Exact
combined-head CI, native Windows/ARM/browser and packaging remain required.
Standalone Go vulnerability scans on #516 are supplemental source-context
evidence, not a substitute for the failed npm audit or a green security job.
Audit #429, authenticated/platform acceptance #318, conditional native lifecycle
#438, bridge #246, actual assistive-technology/IdP/hardware/mobile acceptance
#110, and other roadmap/signing/release prerequisites remain open. Keep #500
draft. No main merge, issue closure, tag or release publication is claimed.
