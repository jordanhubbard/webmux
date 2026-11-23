# Selected-window wheel routing

Implementation checkpoint for [#661](https://github.com/jordanhubbard/webmux/issues/661).
The [receipt](receipt.json) binds the inspected source, built frontend bundles,
actual private browser-test server and retained check logs. No physical trackpad
acceptance is claimed; #661 stays open.

Unmodified two-finger/wheel input scrolls the selected terminal, including while
the pointer is over another terminal or unused grid space. Without a selected
terminal, native handling at the pointer remains available. Shift routes both
axes to the terminal grid before applications see the gesture. Ordinary window
scrolling does not chain into the grid when scrollback reaches either boundary.
The routing hook is installed in local and connected-server terminal workspaces.
Native form controls and embedded browser-pane content retain their own ordinary
wheel handlers; Shift still belongs to the grid. Other modifier combinations
retain their existing handling.

Forwarding preserves modern and native legacy delta properties. Chromium's
constructed events otherwise produce legacy values with different semantics,
which reversed xterm's normal-buffer direction in the first current-asset probe.
No vendor code or terminal private API is changed. xterm's supported custom-wheel
callback marks generated application input local: wheel-derived arrow sequences
must not become broadcast keyboard input. Keyboard broadcast remains available.

## Automated checks

- All 488 frontend tests pass, including seven wheel ownership/normalization/
  containment/disposal controls; strict TypeScript and frontend lint pass.
- Production frontend build passes. Browser assertions verify that the newly
  built terminal surface is loaded rather than accepting stale assets.
- Eight native macOS/system-Chrome browser tests pass through the real Go server
  and PTY children: selected history while pointing at another tile, history
  limits with repeated real wheel events, preselection and selected Shift grid
  panning, mouse-report suppression, wheel-arrow isolation while keyboard
  broadcast is positively verified, managed tmux history, existing mouse input
  across broadcast/restart/selection, and embedded-browser interactions.

The tests preserve existing terminal/application assertions. The continuous
history-boundary test sends repeated wheel input rather than assuming that one
large synthetic delta bypasses the browser's per-event normalization. Input
isolation excludes empty packets and focus reports, while rejecting all other
peer input; failing diagnostics encode bytes as hex so control sequences cannot
look like empty strings in the terminal log.

## Retained failures and remaining acceptance

Earlier logs remain local: an initial probe used previously built frontend
assets; the rebuilt probe exposed legacy-delta reversal; a broadcast probe exposed
browser microtask checkpoints between capture and xterm processing. The local
flag now starts inside xterm's wheel callback. A loading-state dependency ensures
the grid handler installs after the workspace first renders. The final browser
run includes grid panning before explicitly selecting a terminal.

A gesture-capable physical trackpad, other browser/platform combinations and
native connected-server gestures still need acceptance. Automated WheelEvents
and browser input do not establish device-level gesture behavior. The original terminal checkpoint did not complete desktop-grid behavior, MAC
convergence or other roadmap gates. Its parent was blocked under #669 at the
time; the subsequent #672 startup fix entered main as `2d118a2` after all
applicable CI passed. The original receipt retains its historical source binding.
No installed service or shared-storage data was changed.


## Desktop extension and empty-cell control

The [desktop receipt](desktop-receipt.json) binds the later implementation and
checks separately from the original terminal result. The same capture/bubble
ownership now covers the graphics grid, with explicit VNC/RDP selection and a
visible selected outline. Ordinary pointer clicks remain disabled in desktop
thumbnails. Routed wheel input uses the existing noVNC handler or Guacamole
mouse implementation; the scaled RDP thumbnail maps the wheel position into its
remote display coordinates. Fullscreen keyboard handling remains in its existing
fullscreen-only path.

The real-client browser test exposed an add-session-cell exception: these cells
are buttons, and treating every button as a native scroll control let ordinary
wheel input pan the grid. Buttons now follow selected-window routing. Text fields
and selection controls retain their native input handling. A direct unit control
and a browser test over an empty add-session cell cover the correction.

On macOS ARM64 / Chrome 158.0.8127.36, 490 frontend tests, strict maintained
TypeScript checks, frontend lint and the production frontend build pass. Seven
Go-server/PTY browser regressions pass, including scrollback boundaries, tmux,
application mouse reports, restart and keyboard-broadcast isolation. A further
browser test runs the actual noVNC and Guacamole clients through the Go server
against controlled RFB/Guacamole peers. It observes wheel protocol input at the
selected desktop only; selection changes reverse the owner; Shift moves both
grid axes and sends neither desktop wheel input. No coding CLI or named fleet
worker is used.

Retained attempts include a missing bundled Playwright browser (before browser
execution), a new-case username that did not match the strict RDP fixture, and
the reproduced empty-cell routing failure. The final case uses the installed
Chrome executable in a private profile and the fixture's required username.
It keeps the original deadlines and positive/negative input assertions.

These are real browser/client/protocol-fixture checks, not a physical trackpad
or native remote-desktop-server acceptance result. Guacamole's existing mouse
adapter supplies vertical wheel input; this extension does not add a horizontal
RDP protocol capability. Physical gestures, other browser/platform combinations
and native connected-server gestures remain open under #661. No service restart,
fleet policy or NAS operation was performed.
