# Broadcast receipt observation — 2026-11-18

Fresh Linux CI on combined #500 source `a9cf258` failed the broadcast test after
the ordinary B child had received hex `62`, while Playwright's outbound-frame
observer was still empty. The retained A/C child and frame receipts were correct.
The PTY-child file and browser protocol observer are independent delivery queues;
child receipt does not establish that the browser observer has caught up.

The test now awaits each exact typed-frame value through the existing assertion
polling deadline. It retains all raw frames, the two exact focus-report exclusions,
A/C recipient selection and exclusion, ordinary B delivery after mode-off, and all
exact child-file checks. Product routing, fixtures and deadlines are unchanged.

Real Go/Chrome qualification on macOS passes the normal fixed source. A controlled
observer holds only B's notification until the first typed-frame read, then queues
its release as a microtask. With original immediate assertions, the real B child
contains `62` but the frame assertion fails with an empty value. With the same
control and bounded observation, the test passes and the released frame is exactly
`62`. The original failing run remains retained. Final source has no injected
control and matches the passing normal-source hash. Strict E2E TypeScript,
frontend build and whitespace checks pass.

The [receipt](receipt.json) binds the parent, final test source, original hosted
artifact, all local outcomes and log hashes. One control-preparation command used
the wrong relative path; it ran the normal fixed source successfully and is
classified as that normal run. It is not counted as a negative control.

This is a test observation correction, not a product-routing repair. Fresh
hosted exact-head checks and #429 remain required. Refs #521, #461, #452.
