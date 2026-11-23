# Browser pane wheel qualification, 2026-11-23

References: [#661](https://github.com/jordanhubbard/webmux/issues/661),
[#698](https://github.com/jordanhubbard/webmux/issues/698).

Native Chrome on macOS reproduced two independent defects. Shift wheel over a
browser pane sent a remote scroll action with modifier 8 and left the grid at
`[0,0]`: the pane is portalled outside the grid's DOM capture listener. Once
that routing was corrected, ordinary single-axis scrolling reached Chromium
but failed with protocol error `-32602`, requiring both `deltaX` and `deltaY`.
The pinned SDK omits zero-valued axes from its generated parameters.

Browser panes now capture wheel input with a non-passive native listener and
route Shift to their owning workspace's shared grid-pan handler. Native browser
wheel commands explicitly serialize both axes while retaining pointer, delta
and modifier validation. Other browser input still uses the pinned SDK.

## Verification

- All 499 frontend tests passed. The new portal test covers two owning grids,
  exclusive Shift routing, ordinary browser input and listener disposal.
- All maintained TypeScript projects, frontend lint, browser Go race tests and
  browser Go vet passed. Wire tests retain vertical, horizontal, stationary
  and fractional axes, including explicit zero values.
- Native Chrome E2E passed the new browser portal test, browser docking test and
  three existing terminal wheel/application mouse/broadcast tests together.
  The fixture uses the real WebMux Go server and a real Chromium page.
- Shift wheel `(120,180)` left the grid at `[120,180]` with no remote Shift
  action. Subsequent ordinary vertical `(0,220)` and horizontal `(180,0)`
  input scrolled the remote page to `(0,220)` then `(180,220)`, returned two
  successful browser frames, and left the grid unchanged.

The E2E test saves sanitized action, response and page-scroll observations in
`browser-portal-wheel.json` under its Playwright result directory. Original
failed controls and passing receipts are retained privately under
`_build/browser-wheel661/` in the primary checkout. Browser credentials and
control tokens are excluded from those observations.

This is automated native Chromium wheel evidence. It does not establish
physical trackpad behavior, other browser/OS combinations, or full VNC/RDP
acceptance. Issue #661 remains open. These changes operate in the browser
client and server protocol; use from another computer does not require a
preview on that computer's loopback interface.
