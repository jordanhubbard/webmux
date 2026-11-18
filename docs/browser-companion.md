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
WebMux also ends its browser. A worker expires after 30 minutes without viewer
activity. Profiles never import the host's everyday browser credentials.

Temporary profiles are the default. Open **Browser settings** to configure saved
profiles. For an explicit saved profile, enter a name,
choose **Create saved profile**, then **Open selected profile**. Opening a selected
profile ends the current worker first. Saved cookies and site storage survive
worker and server restarts. The registry is scoped to the signed-in WebMux owner
and execution host/account (local server, or SSH username, hostname and port).
Changing an SSH key does not change that principal. Host aliases remain separate.
Only one worker may use a saved profile at a time, including other controllers
running under that execution account. An OS lock is released when its worker exits.
The execution account can read its own saved data; this is not OS-account isolation.

Saved data lives in private `~/webmux-browser-profiles/` directories named by opaque
keys. The API never accepts filesystem paths or exposes those keys. **Delete saved
profile** removes its data on the execution host, then its registry entry. End its
browsers first. A failed or unavailable-host deletion keeps the registry entry for
retry. Keep access to the original execution host/account until deletion succeeds.

Transferred files always remain temporary. A temporary worker uses a fresh private
`~/webmux-browser-*` directory; a saved worker uses a separate `~/webmux-transfer-*`
directory, removed when the worker exits. These home locations work with packaged
Chromium browsers that have a private temporary-file namespace. Abrupt host/process
termination can leave temporary files for the execution account to remove.

Viewers signed in as the terminal owner share its page, but one viewer controls
navigation, input, resizing, transfers, dialogs and ending. Other viewers see
**View only**. **Take control** explicitly replaces an active viewer after a
confirmation; **Release control** permits another viewer to claim it. The server
checks a private viewer token again when queued work reaches the worker. Leases
expire after 30 seconds without controller activity; visible polling renews them.
Hiding the drawer releases control. Hidden tabs stop polling. Handoff releases held
pointer buttons before further input. Update both controller and terminal-host
WebMux: these guarantees require browser worker protocol 3.

## Everyday browser controls

Use **Expand browser** for a full workspace, or resize the drawer. The Chromium
viewport follows the available area instead of scaling a fixed desktop layout.
On narrow screens the companion fills the screen, with an address row below the
navigation buttons and touch-sized controls. **Browser settings** holds profile
setup; controller ownership and **Return to terminal** remain directly available.
Scroll the companion when the screen or keyboard leaves less vertical room.
The [small-screen qualification](validation/browser-mobile-2026-11-17/README.md)
covers browser-emulated touch and reduced heights, with physical-device keyboard
and assistive-technology acceptance still open under #110.
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
file paths. Ending the browser removes transfer files; saved profile cookies remain until explicitly deleted.
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
  silently; clicked links still open the sign-in panel.
- New Mosh terminals with the matching terminal-host helper carry automatic
  browser requests through the existing authenticated SSH control channel.
  This works with logging disabled; Mosh's screen protocol does not have to carry
  a browser escape sequence. After updating the terminal-host helper, create a
  new Mosh terminal to get the shell integration. Existing providers are not
  restarted automatically. Older/missing helpers retain manual link clicks.
- Local and SSH launch requests travel through the existing PTY, including through local tmux,
  without a public listener or bearer token in the shell. The focused viewer
  receives the request; an unacknowledged request is retained for up to five
  minutes so reconnecting can open it. Owner-checked acknowledgements prevent
  replay after it has been delivered.
- Mosh keeps its latest pending request sealed in the terminal host's private
  control cache for a five-minute delivery window. Only the opaque acknowledged
  request ID is saved by the controller. Acknowledgements clear that exact host
  request and preserve a newer replacement. Durably acknowledged requests stay
  dismissed across reconnects and controller restarts. The normal SSH control synchronization
  removes acknowledged or expired pending records. No new listener or polling
  loop is added, and the request is separate from terminal activity capture.
- Terminal viewers consume ANSI/DEC mode queries (DECRQM) in the parser, alongside
  the existing identity/status/color query policy. Multiple viewers must not send
  duplicate replies into a shell or broadcast them into another terminal. Programs
  use their fallback when these capability queries receive no reply; ordinary
  mode changes, keyboard input and bracketed paste remain available.

## Acceptance

Use a local HTTP fixture that simulates authorization and redirects to a separate
loopback callback. Open it through the actual companion, type into its form,
submit, and prove the callback receiver observed the result. Verify hide/reopen,
explicit destruction, ownership denial, keyboard/paste, and launch failures.
Repeat on an SSH host before claiming remote-host qualification.

For [#253](https://github.com/jordanhubbard/webmux/issues/253), a real macOS 27.2
ARM64 run with Homebrew tmux 3.8 and Chrome 157 reproduced the same DECRQM reply
from both an earlier viewer and a later viewer, followed by shell-input corruption.
With parser suppression, both viewers still receive the query but send no reply;
the full `gh-browser` loopback callback workflow passes. The deterministic
`terminal-mode-query.spec.ts` control injects split query output at the WebSocket
boundary, checks both viewers and a broadcast peer, and verifies keyboard/paste
bytes through real PTY child receipts. It fails against the parent frontend.
Existing real mouse broadcast/restart/selection controls also pass. These are
terminal and synthetic callback controls, separate from real identity-provider
authentication acceptance under #110.

## Interrupted initial frames

A live worker can lose a screenshot while its page navigates. The companion now
continues frame polling after a matching worker startup response even when the first
image is interrupted. Recovery requests a new frame; the original sign-in
navigation executes once. A worker that fails before becoming available keeps
its startup error and is not polled or relaunched automatically.

The browser control uses a real worker, terminal link and loopback callback,
replacing only the first start/navigation response images with the interrupted
rendering response observed on macOS. It fails on the prior frontend and passes
with frame recovery, one navigation and the independently observed callback.
The unit controls also cover failed worker startup. This addresses one blank-start
path under [#282](https://github.com/jordanhubbard/webmux/issues/282); it does not
establish the cause of the historical macOS URL-poll timeout.

A temporary layout change also cancels its pending resize when the drawer
returns to the last requested size. Previously an A → B → A sequence could
still send B after the drawer had returned to A, leaving the worker viewport
out of step until another layout change. The deterministic observer control
fails the old behavior and verifies cancellation plus a later genuine resize.
Qualification retains bounded drawer/image/worker size samples on failure;
input still requires all three sizes to agree within the existing deadline.

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
automatically. Control leases apply to viewers authenticated as the terminal owner;
other owners cannot access the terminal or its browser.

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


### Packaged Chromium file visibility (2026-11-16)

Fresh Linux ARM64 qualification with Snap Chromium 156.0.8078.47 reproduced a
real upload failure: the chooser accepted a 1,126,400-byte selection but the
page's HTTP receiver got none of its bytes. Moving only the temporary root to
a shared home path made the unchanged workspace fixture pass, including exact
upload and binary download checks. The worker now creates its fresh 0700 profile
in the account home automatically, keeping transfers and cleanup in the same
filesystem namespace. Browser sandbox protections and test deadlines are
unchanged. A real Snap browser also passed root-tab lifecycle and profile removal
after worker stdin closed. See [#387](https://github.com/jordanhubbard/webmux/issues/387).

A separate ordinary Chrome 157.0.8103.79/macOS 27.2 ARM64 worker on puck passed
protocol-v2 frames and exact >1 MiB upload/download bytes over authenticated,
strict-host-key-checked SSH. This used a private worker, synthetic loopback site
and fresh browser profile; it did not copy or use everyday browser credentials.
It qualifies that worker transport, not the whole SSH-terminal UI or any real
identity provider. A private Chrome-for-Testing build on ubumeh refused startup
because that location had no usable browser sandbox; no protection was disabled
to obtain a pass there. Durable profiles, leases, mobile/accessibility and
provider/hardware authentication acceptance remain open in #110.


## Read remote page text

Choose **Read page text** to open a keyboard-focusable text view of the selected
page’s Chromium accessibility names and roles. Headings, links, buttons and form
labels appear in accessibility-tree order within each document and can be read
with the local screen reader. Embedded documents follow the selected page’s
document.
**Close page text** returns focus to remote-page input. No new keyboard shortcut
is reserved.

Supported controls offer **Focus** and **Activate** buttons. Focus selects the
remote field and returns your keyboard to remote-page input; ordinary typing,
editing keys and Shift+Esc keep their existing behavior. Activate invokes the
selected button, link or toggle's page click behavior. Use the viewport for
controls that need a different gesture. Text fields offer focus without an
activation action; their existing contents are not included in the text view.

This is an on-demand snapshot of the selected page, including supported
same-origin, cross-origin and nested frames.
Read it again after dynamic page changes. Navigation or a different selected tab
hides the old snapshot. An action consumes its references, including failed or
interrupted actions: read again before another action rather than retrying an
old reference. Other page input also clears the snapshot. Frame polling and
viewport resizing preserve semantic references, including when opening this
panel changes the viewport size.

If an action opens a JavaScript dialog, answer it before further page input.
Viewport changes wait until the dialog closes; the frame keeps reporting the
actual size during that wait. A control takeover also defers held-pointer
release until the dialog can accept it. Queued page input is refused rather
than sent into the page after dismissing the dialog.

Only the current controller can read or act, with the lease checked again at
dispatch. Each opaque reference is bound to the selected tab and its actual
frame/document ancestry. The worker verifies that ancestry, the document that
currently owns the native node, and its accessible name, role and availability.
It refuses moved, replaced, renamed, disabled or stale controls. Native DOM identifiers stay in
the worker; HTTP requests cannot supply selectors or JavaScript. Existing
owner, SSH and worker-channel boundaries apply.

The worker emits at most 512 entries and 32 KiB of names/roles, with individual
names limited to 2048 UTF-8 bytes and tree depth to 32. The view reports when
content was cut off. Ignored nodes, duplicate inline glyph runs, editable values
and editable descendants are omitted; markup is displayed as text. Action
references are bounded by that same entry limit and replaced on each reading.
Frame discovery is also bounded to 32 documents and 32 temporary iframe
attachments per request. Unavailable, unproven or excess frames are omitted;
read again or use the viewport when content is truncated. Temporary attachments
detach without closing or navigating the frame. Chromium site isolation and
frame sandbox restrictions stay enabled. A matching URL, name or browser
context in a different tab does not grant access to its controls.

This text view does not forward a local screen reader to the remote OS or
establish real identity-provider acceptance. Those broader accessibility and
authentication requirements remain under #110.

Actual macOS/Chromium qualification covers the visible Focus/Activate flow,
native typing (including a closed shadow root), single-use activation, link
navigation, disabled controls, page-initiated navigation and renamed controls.
See [root-control qualification](validation/browser-actions-2026-11-17/README.md)
and [frame-control qualification](validation/browser-frames-2026-11-18/README.md).

## Opt-in navigation diagnostics

For worker-side diagnosis, set `WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS=1` in the
browser worker's environment. Local workers inherit the controller environment;
set it on the terminal host for an SSH/Mosh worker. The default is off.

Frames then include bounded `navigationDiagnostics`: the first 128 events and
an explicit dropped count. The trace records navigation requests and CDP
acknowledgments, document requests/responses/failures and frame commits. Opaque
frame/loader/request IDs correlate events; errors use fixed classes. URL
correlation is a boolean computed from an in-memory hash. The trace does not
retain URLs, cookies, headers, profile keys or page bodies.

This observes the existing asynchronous navigation behavior. It does not wait
for a document to load, repeat a request, change ownership or increase a
navigation/assertion deadline. Windows browser CI enables it and retains it
inside the existing browser-profile action trace.
