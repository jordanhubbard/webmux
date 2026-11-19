# Browser pane docking and layout

Issue [#513](https://github.com/jordanhubbard/webmux/issues/513), implementing the
remaining workspace arrangement portion of #110. The desktop browser begins
at the right edge. Undock exposes a movable, resizable pane inside WebMux;
Dock restores the separate dock width, and Expand/Restore temporarily fills
the workspace. Focused handles accept arrows and Shift for larger steps.
Pointer gestures use capture and stop on cancellation or layout changes.

The same mounted BrowserCompanion owns all modes. Layout gestures do not start
or replace the worker, navigate the page, claim control, or release its lease.
The existing resize observer follows viewport changes. A versioned local value
contains only mode, dock width, and floating rectangle. Invalid/unavailable
storage is safe, bounds are fitted to the current viewport, and small screens
fill the display while retaining desktop preferences. Preferences are device
layout values shared by newly opened companions, not session records.

## Local verification

- Full frontend suite: 43 files, 425 tests pass, including seven new lifecycle,
  pointer identity/cancellation, persistence/privacy, denied storage, view-only,
  mobile and multi-companion focus controls.
- Strict TypeScript checks pass for all five maintained projects; frontend lint
  and production build pass.
- Actual Go server and Chrome on macOS: browser-pane-layout.spec.ts passes,
  including real pointer move/resize and keyboard width/move; dock/float and
  expand/restore retain geometry. The same target, URL and profile survive
  hide/reopen and reload, with exactly one fixture document and three clicks.
  The layout-only sequence retains one control lease with no release, claim,
  additional start or navigation. A full page reload may need explicit Take
  control under the existing viewer-lease lifecycle; the test does so rather
  than assuming control can persist across a new viewer identity.
- Existing native mobile and interactive workspace regression tests both pass,
  covering three small screen geometries, touch input, dialogs, clipboard,
  file transfer, navigation, tabs and accessible page controls.
- The first exploratory native run timed out after reload while view-only;
  retain its failure separately. The accepted run adds explicit reload control
  acquisition and bounded fixture teardown; no production lease rules changed.

Native trace and screenshot are retained under the private per-run Playwright
output directory. Native Linux/Windows qualification on the published head is
still required. This is not actual assistive-technology, real IdP, security-key,
client-certificate, physical-mobile or complete #110 acceptance. Audit #429 and
other release gates remain unchanged.

The branch includes the independently tracked #514 sysfs resolution observer
fix, based on #512 browser shutdown. Neither repair is attributed to layout.
