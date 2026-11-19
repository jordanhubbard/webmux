# Current viewport measurement during browser layout checks

Issue #587 retains a Windows failure at source `3d64d10`: the layout test saved
an image height of 593 pixels before polling, while the remote browser later
reported 648 pixels. The original five-second exact comparison failed. Its job
log and GitHub artifact are bound in [receipt.json](receipt.json). The original
transient trigger is not established by that log.

A deterministic macOS Go/Chrome diagnostic injects a temporary error into actual
browser-worker replies while retaining their live target, image and control
metadata. After the application renders the notice, the driver measures the
smaller viewport, clears the synthetic error, and waits for native resizing.
The unchanged old comparison fails after five seconds: 598 expected, 653 actual.
This reproduces the obsolete-measurement mechanism with the same 55-pixel change;
it does not prove what caused the original Windows transition.

The maintained test now remeasures the current visible viewport on each attempt
and compares its exact clamped dimensions with the native frame. The comparison
retains its five-second bound. The full case retains its 120-second limit,
image-width and click checks, floating/docked/expanded geometry, responsive
layout, reload, target/profile/control identity, one start, one navigation and
one document. The synthetic transient-notice case is retained as a regression.
No production implementation changes.

The corrected real Go/Chrome case passes in 4.6 seconds (13.43 seconds including
fixture startup/teardown). Its receipt independently records the captured,
current visible and current native dimensions. All five strict TypeScript
projects and the unchanged production frontend build pass. The first diagnostic
setup could not hold a frame-only notice long enough to reach the geometry
assertion; its source, failed result and complete inactive fixture home remain
retained separately. Both later runs use the corrected fault setup, and the
negative run retains the old measurement assertion.

Normal review and exact-head checks remain required. These results do not claim
native Windows requalification, an identified historical transient cause, full
#110 acceptance, physical mobile or assistive-technology qualification, or a
release.
