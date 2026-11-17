# Accessible remote-page actions (2026-11-17)

Actual Chromium controls passed on macOS ARM64 for #419 / #110. The backend
control exercised named activation, keyboard focus and typing (also inside a
closed shadow root), link navigation, disabled controls, renamed-element
rejection, a page-initiated navigation while only polling frames, refusal of a
reused reference and controller isolation. The visible browser-workspace flow
also passed Focus/Activate, keyboard handoff, editable-value omission and its
existing dialogs/files/clipboard/navigation/tabs checks.

The native tests are `tests/e2e/browser-accessible-actions.spec.ts` and
`tests/e2e/browser-workspace.spec.ts`. [native-results.json](native-results.json)
records the actual executable, tested source-content hashes, browser version
and retained log digest. The test server uses isolated synthetic pages and the
ordinary browser worker, not a fake DOM or injected action implementation.
The original operation/assertion deadlines remain in use.

References are opaque and bounded by the 512-entry reading limit. Each action
consumes them before dispatch and verifies current target/document/URL,
backend element identity, accessible name/role and availability. Regular input
and tab changes invalidate them; frames and resize preserve semantic targets.
No selectors, JavaScript or native DOM IDs cross from HTTP into the action.
Editable values and editable descendants remain absent from text responses.

These controls establish this synthetic local workflow. They do not establish
real identity-provider/security-key acceptance, iframe interaction, a live
screen-reader bridge or every accessibility/mobile case under #110. Hosted
Windows/Linux checks qualify only their own actual tested outcomes.
