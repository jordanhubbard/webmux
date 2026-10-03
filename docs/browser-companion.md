# Terminal browser companion

## Experience

Each terminal has a **Open browser** action. It opens a resizable companion drawer,
leaving the terminal running and available beside it. The drawer identifies the
terminal and the host where the browser actually runs. Click an authentication link in the terminal, or paste its
URL, interact with the remote page, and return to the terminal when the CLI
reports success. A redirect alone is not evidence that authentication succeeded.

The page executes on the shell host: `localhost` callbacks reach the CLI there.
Only the browser viewport is transmitted; no desktop, taskbar, or unrelated
windows are shared. Navigation, keyboard input, paste, scrolling, and switching
between pages opened by the site stay inside the companion.

**Return to terminal** hides the drawer and retains browser state. Reopening it
reattaches to that terminal's browser. **End browser** closes Chromium and removes
its temporary profile, including cookies. Deleting the terminal or stopping
WebMux also ends its browser. Profiles are separate for every terminal and user;
they do not import the host's everyday browser credentials. Browser state is
ephemeral across server restarts. An idle browser expires after 30 minutes without
viewer activity, so hiding it is suitable for short interruptions, not long-term
credential storage.

## First implementation

- Native Go browser worker using Chromium, with a fixed 1100 × 760 viewport and
  compressed viewport frames. No public debugging port or new network service.
- Local terminals start the worker on the WebMux server. SSH/Mosh terminals start
  it through SSH on the terminal's host, using the saved key and existing SSH
  configuration/trust. Password-only SSH asks the user to configure key/agent
  access. Arbitrary exec transports cannot prove host identity and are rejected.
- The remote machine needs this version of the native WebMux binary on PATH and
  Chrome/Chromium installed. Failures explain these prerequisites; there is no
  fallback to a browser on the wrong machine.
- Authenticated, owner-checked session APIs carry frames and input. Browser
  cookies stay in a private temporary directory on the execution host. URLs,
  credentials, and frames are not written into terminal transcripts or audit logs.
- Terminal link clicks (plain URLs and OSC 8 hyperlinks) open the linked
  companion and navigate there, including when its drawer is already open.
  Manual URL entry is also available. A CLI invoking the OS browser opener
  directly still needs its printed link clicked; shell-launch capture is pending.

## Acceptance

Use a local HTTP fixture that simulates authorization and redirects to a separate
loopback callback. Open it through the actual companion, type into its form,
submit, and prove the callback receiver observed the result. Verify hide/reopen,
explicit destruction, ownership denial, keyboard/paste, and launch failures.
Repeat on an SSH host before claiming remote-host qualification.

## Next iterations

Tracked in [#110](https://github.com/jordanhubbard/webmux/issues/110).

1. A shell-scoped `BROWSER` opener routes launch requests to the linked companion;
   an unobtrusive “Browser requested” badge opens it without stealing focus.
2. Dock/undock and remembered pane sizes; adapt to narrow screens with Terminal /
   Browser tabs and a visible pending-auth indicator.
3. Opt-in reusable profiles per host and user, plus clear expiry and sign-out
   controls. Add a single-controller lease before cross-user collaboration.
4. Qualify identity providers individually. Headless/automated browsers may be
   rejected by some providers; hardware keys, client certificates, native SSO,
   downloads/uploads, and screen-reader access to the remote DOM need dedicated
   support. Do not claim these work merely because a callback fixture passes.

## Trying it

Build with `make build`, start an isolated instance, and add a Local terminal.
Click **Open browser**, paste the URL printed by the CLI, and complete authentication.
On an SSH target, install the matching native binary as `webmux` on PATH and
Chrome/Chromium; confirm `ssh HOST webmux --help` works non-interactively first.
`WEBMUX_BROWSER_EXECUTABLE` can select a Chromium executable on the worker host.
The browser uses Chromium's normal sandbox; running it as root is not supported.

The first version transmits compressed snapshots, aimed at forms and navigation,
not video. Clipboard paste is explicit; clipboard contents are never fetched
automatically. All viewers authenticated as the terminal owner share its browser;
the first version does not provide a multi-person control arbitration protocol.

Qualification so far: macOS Chrome 154 and Chromium 145 headless-shell pass the
real-browser checks. The full Chrome-for-Testing 145 build on macOS timed out
when capturing a viewport; use current Chrome or headless-shell for the preview.
