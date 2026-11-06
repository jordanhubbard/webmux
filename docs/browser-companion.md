# Terminal browser companion

## Experience

Terminal links and shell browser requests now open a **session-linked sign-in
panel**. Prefer your current browser for device codes, SSO, passkeys and manual
code flows; state-bound localhost OAuth can use the callback relay. See
[CLI authentication](authentication-handoff.md) for the flow and prerequisites.

Each terminal also retains a **Tools → Open browser** action. It opens a resizable
companion drawer, leaving the terminal running beside it. Choose **Use browser on
terminal host** in the sign-in panel when you need this fallback. The drawer
identifies the terminal and host where the browser runs. A redirect alone is not
evidence that authentication succeeded.

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

## Everyday browser controls

Use **Expand browser** for a full workspace, or resize the drawer. The Chromium
viewport follows the available area instead of scaling a fixed desktop layout.
Back, forward, reload, stop, new tab, close tab, and tab selection are available.
**Find text** searches the page, with previous/next matches. Cmd/Ctrl+F, R, T and
W provide page search, reload, new tab and close/hide respectively.
Site-created popups appear in the tab selector.

Hover, drag, double-click, right-click, horizontal/vertical scrolling and combined
keyboard shortcuts reach the page. Native page dropdowns get an option chooser;
JavaScript alert, confirm and prompt dialogs get visible controls. Text input
supports Unicode, composition events and mobile keyboard input. **Escape** reaches
the remote page; **Shift+Escape** returns focus to WebMux controls. **Cmd/Ctrl+L**
focuses the address, and **Copy selection** / **Cmd/Ctrl+C** copy selected page text
to this device. Ordinary paste and the manual paste field insert text into the
focused page field.

Click a file input on the page to choose files from your device. Up to eight files
and 10 MiB can be sent in one selection, with 50 MiB retained per browser. Downloads
appear in **Downloads**; choose **Save** to transfer completed files to your device.
Downloads are limited to 10 MiB each, 20 retained files and 50 MiB per browser.
Files travel through the authenticated browser worker connection, including when
the worker is on an SSH host; the remote host does not need access to your local
file paths. Ending the browser removes its private files together with its profile.
Directory pickers and OS-native browser UI are not forwarded.

For a site that rejects automated Chromium or needs your device's passkey, native
SSO, client certificate or security key, use **Open in your browser**. Public sites
open directly on this device. **Sign-in handoff** uses the existing session-linked
flow for CLI sign-in and callback relaying. Loopback URLs and OAuth URLs with local
callbacks use that handoff rather than silently opening localhost on the wrong
machine. Cookies are separate between the companion and your everyday browser.
This does not claim that remote hardware authentication is supported.

## Current implementation

- Native Go browser worker using Chromium, with a viewport that follows the drawer size and
  compressed viewport frames. Pointer/key input does not wait for a screenshot
  after each event. No public debugging port or new network service.
- Local terminals start the worker on the WebMux server. SSH/Mosh terminals start
  it through SSH on the terminal's host, using the saved key and existing SSH
  configuration/trust. Password-only SSH asks the user to configure key/agent
  access. Arbitrary exec transports cannot prove host identity and are rejected.
- The remote machine needs this version of the native WebMux binary on PATH and
  Chrome/Chromium installed. Failures explain these prerequisites; there is no
  fallback to a browser on the wrong machine.
- Authenticated, owner-checked session APIs carry frames and input. Browser
  cookies stay in a private temporary directory on the execution host. Browser
  control packets are removed before terminal logging and scrollback. Browser
  frames and inputs are not logged; URLs printed normally by a CLI remain ordinary
  terminal output and can appear in enabled session logs.
- Terminal link clicks (plain URLs and OSC 8 hyperlinks) open the sign-in
  panel; choosing its terminal-host fallback navigates the linked companion.
  Manual URL entry is also available.
- New Local shells set shell-scoped `BROWSER` and `GH_BROWSER` helpers. Bash/Zsh
  startup hooks preserve user profiles, then restore the helper environment.
  PATH-based `open`, `xdg-open`, and `sensible-browser` calls with one HTTP(S) URL
  also route into the sign-in panel. No user startup files or OS associations change.
- SSH probes for the matching helper with verified host keys and key/agent access
  before starting a browser-linked shell. Missing support leaves a normal shell
  silently; clicked links still open the sign-in panel. Mosh uses manual link
  clicks because its terminal protocol does not reliably carry the launch control
  sequence.
- Launch requests travel through the existing PTY, including through local tmux,
  without a public listener or bearer token in the shell. The focused viewer
  receives the request; an unacknowledged request is retained for up to five
  minutes so reconnecting can open it. Owner-checked acknowledgements prevent
  replay after it has been delivered.

## Acceptance

Use a local HTTP fixture that simulates authorization and redirects to a separate
loopback callback. Open it through the actual companion, type into its form,
submit, and prove the callback receiver observed the result. Verify hide/reopen,
explicit destruction, ownership denial, keyboard/paste, and launch failures.
Repeat on an SSH host before claiming remote-host qualification.

## Next iterations

Tracked in [#110](https://github.com/jordanhubbard/webmux/issues/110).

1. Dock/undock and remembered pane sizes; adapt to narrow screens with Terminal /
   Browser tabs and a visible pending-auth indicator.
2. Opt-in reusable profiles per host and user, plus clear expiry and sign-out
   controls. Add a single-controller lease before cross-user collaboration.
3. Qualify identity providers individually. Headless/automated browsers may be
   rejected by some providers; remote hardware keys, client certificates, native
   SSO and screen-reader access to the remote DOM need dedicated support. Do not claim these work merely because a callback fixture passes.

## Trying it

Build with `make build`, start an isolated instance, and add a Local terminal.
Run `gh auth login`, choose browser authentication, and press Enter when prompted.
The sign-in panel should open inside WebMux. Choose **Continue in this browser**
for device login, or explicitly choose the terminal-host browser fallback. Check
the CLI for success. You can also click any HTTP(S) link printed in the terminal.

Existing tmux shells keep their old environment across server updates. Create a
**new Local terminal** to enable automatic launch; simply refreshing or reconnecting
an old terminal cannot change the environment of an already running CLI. Explicit
absolute OS opener paths (such as `/usr/bin/open`), app-specific browser overrides,
and unsupported shell startup scripts may bypass the helper; click the printed
link in those cases.
On an SSH target, install the matching native binary as `webmux` on PATH and
Chrome/Chromium; confirm `ssh HOST webmux --help` works non-interactively first.
An older remote worker is reported as an upgrade requirement before the new
controls are used.
`WEBMUX_BROWSER_EXECUTABLE` can select a Chromium executable on the worker host.
The browser uses Chromium's normal sandbox; running it as root is not supported.

Passkey and security-key prompts are not forwarded to the viewer's device. On
GitHub's passkey two-factor page, choose **More options** and another method you
already configured, such as an authenticator app. If no supported alternative
exists, this companion cannot complete that sign-in. Do not disable account 2FA.

The companion transmits compressed snapshots, aimed at interactive pages and forms,
not video. Clipboard paste is explicit; clipboard contents are never fetched
automatically. All viewers authenticated as the terminal owner share its browser;
it does not provide a multi-person control arbitration protocol.

Earlier companion checks passed macOS Chrome 154 and Chromium 145 headless-shell.
The expanded workspace is locally qualified with macOS Chrome 156; CI records its
managed Chromium and native platform evidence separately. The full
Chrome-for-Testing 145 build on macOS previously timed out capturing a viewport;
that historical result does not qualify the expanded workspace.

The v1.4.2 workspace fixture exercises actual Chromium over the owner-checked API:
hover/drag/double/right click, confirm/prompt/dropdown interaction, selection copy,
Unicode insertion, a >1 MiB upload, exact binary download, back/forward, new/closed
tabs, site popups, hide/reopen and cleanup. These fixtures qualify the controls;
provider-specific sign-in, native device authentication and SSH-host acceptance
remain separate qualification work in #110.
