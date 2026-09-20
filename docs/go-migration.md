# Go server migration

Tracked in [#77](https://github.com/jordanhubbard/webmux/issues/77).

The target architecture is a native Go server and the existing React/TypeScript
browser application. Keep xterm.js, noVNC, Guacamole, DOM structure, styles, and
fonts. WASM is not required. First-party browser and Node tooling sources must be
checked TypeScript; generated JavaScript and upstream dependencies are not
first-party source.

The Node server remains the default until the complete replacement passes the
compatibility gates. The Go implementation lives in `webmux/server`; partial
implementation is not a production replacement. Do not run both implementations
against the same writable `WEBMUX_HOME`.

Native packaging now applies the release-tag/version check before building and
verifies its complete upload set before retaining artifacts. Unix requires one
version/platform/architecture-specific archive plus its SHA-256 file; Windows
also requires the matching MSI and checksum. Missing, stale, nonregular, empty or
corrupted native artifacts fail the gate. Legacy files may coexist in the build
directory. The verifier passes the existing local native archive and regression
fixtures for incomplete/corrupted sets; TypeScript, all eight helper tests and
workflow parsing pass locally. Hosted execution is pending. Native release
publication and the default switch remain gated on the full migration checks.
Windows x64 at 1569987 reached this gate after passing installed MSI and service
checks, then correctly rejected an extra `.wixpdb` debug database. Packaging now
passes `-pdbtype none` to WiX, and a regression case rejects debug output from the
native upload set. Linux and Windows x64 native packaging passed the upload gate at
1747b59, including Windows installed-MSI/service checks before upload.

## Completion requirements

- Preserve HTTP methods, paths, response shapes, status codes, WebSocket framing,
  one-time tickets, ownership and viewer/focus semantics.
- Read existing YAML files, Argon2id password hashes and JWT signing secrets;
  preserve saved hosts, keys, layout, sessions and runtime settings.
- Preserve terminal SSH/mosh/exec transports, PTYs, agent/tmux integration,
  reconnects, resize, scrollback, broadcasting, transcripts and process cleanup.
- Preserve VNC and RDP proxying, network restrictions, credential lifetime and
  desktop session persistence.
- Preserve settings/font hosting, uploads, templates, AI integrations, audit
  logging and configuration reloads.
- Preserve macOS, Linux and Windows service, installation and packaging support.
- Convert maintained JavaScript tooling/configuration to checked TypeScript.
- Run differential HTTP/protocol tests against both servers; test concurrency,
  authorization, malformed input, restart and shutdown behavior on Go.
- Run frontend tests and browser workflows against Go. Establish visual baselines
  with fixed browser, viewport, fonts and scaling; compare unchanged UI states.
- Measure startup, memory and interactive throughput, and verify CI/build/release
  paths before removing the Node server and changing the default commands.

Tests establish parity for the covered cases, not universal equivalence. Any
intentional security improvement must be recorded separately from compatibility
claims. Work remaining stays on #77 until these requirements are verified.

Hosted macOS verification now uses the ARM64 `macos-15` image. GitHub's
[macOS 14 deprecation notice](https://github.com/actions/runner-images/issues/13518)
warns of longer queues beginning July 6, 2026 and retirement on November 2.
The old jobs remained queued without assigned runners; deprecation may contribute,
but the queue cause is not proven. Both workflows retain
`MACOSX_DEPLOYMENT_TARGET=14.0` for native macOS build tools. New-runner execution
and compatibility on an actual macOS 14 machine still need verification.

The Unix lifecycle fixture also supports `--make` on macOS. It invokes the real
Make install/start/stop/uninstall targets with a private launchd identity,
temporary application root and copied configuration. The already-built native
binary and web assets are reused (`-o build`); the service serializer and manager
commands execute normally. Local GUI-domain verification passes install, stop,
start and restart, HTTP/UI checks, configuration/signing-secret preservation, active PTY
child exit, acknowledged-output transcript draining and uninstall cleanup.
Repository defaults remain unchanged. This is now a macOS packaging CI step;
hosted execution and Linux user-manager Make installation remain unverified.
The fixture also exercises the separate Make `restart` command while a terminal
is active, requiring a different backend PID, restored persistent session,
preserved application configuration/signing secret, old PTY termination and a
complete shutdown footer on the transcript containing the acknowledged output.
It distinguishes that transcript from the new file opened during restoration.
Uninstall waits for launchd's asynchronous registration removal before deleting
the fixture directory.

The Make fixture now supports Linux's user manager as well. Packaging CI starts
the runner's user manager and supplies its runtime/bus environment before running
the fixture. The unit uses a unique filename in the invoking user's normal unit
directory; Make derives its control name from that path instead of hard-coding
`webmux.service`. The production default is unchanged. This checks the actual
install/enable/start/stop/restart/disable/uninstall path, with the same PTY,
transcript and persistence assertions. Linux execution of this addition is
pending; local TypeScript and helper checks pass.

Make start/restart now propagate service-manager errors, and restart aborts if
its rebuild fails instead of replacing the service and printing success. A
fixture that substitutes all manager commands reproduced the former false-success
exit and verifies manager failures, rebuild-before-restart ordering and successful
controls without touching any installed service.

## Implemented foundation

The Go server currently implements health, authentication/account, saved-host,
SSH-key catalog, runtime settings, layout and authenticated font routes,
Argon2id PHC password storage, HS256 JWTs, one-use WebSocket ticket storage,
per-client rate limiting, HTTP/TLS listeners and graceful HTTP shutdown. It uses
the existing `WEBMUX_HOME/config` and `data/events` layout. Authentication config
updates are serialized and atomically replaced, preserve unknown fields and
follow configuration symlinks. Catalog updates preserve legacy missing/null
fields and custom metadata, serialize concurrent changes, and redact key paths
and private metadata from API responses. Settings enforce administrator/trusted
access, preserve nested defaults and disabled agent definitions, and keep
environment overrides out of YAML. Font serving follows the real app.yaml
directory and confines file opens to that root, with range/conditional requests
and the existing cache headers. Terminal session HTTP routes now support
owner-scoped CRUD, patch precedence, reconnect and saved-session recovery. The
broker serializes grid allocation and persistence, rebuilds layout tiles while
preserving layout metadata, and rejects stale output/exit events from replaced
processes. Terminal WebSockets now support ticket/token authentication, atomic
scrollback-to-live delivery, input, resize, viewer presence/focus and deletion.
Transcript logging now follows the configured launch default, supports manual
pause/resume into the same file, and rotates files across launches. It preserves
headers/footers, audit event fields and owner-only POSIX permissions. Template
list/detail routes expose all five built-in templates with their existing text,
icons, setup steps and launch commands. The Go broker uses the same catalog as
the API. The Go server serves the existing production frontend from
`WEBMUX_ROOT/web`, with client-side navigation fallback, directory redirects,
GET/HEAD, range requests, stat ETags, Last-Modified and revalidation. Differential
fixtures compare asset/index bytes and cache/content headers against Node.
AI status/chat routes now preserve RCC-first routing, legacy LOOM environment
aliases and NVIDIA-before-OpenAI fallback. They retain the system prompt, last
ten history entries with system messages removed, the final 3,000 UTF-16 units of
terminal context, provider defaults, request parameters and response metadata.
RCC preserves reply whitespace; direct replies use JavaScript-compatible trimming.
Local differential fixtures compare authentication, status, missing messages,
RCC requests/replies, environment precedence, UTF-16 boundaries and unavailable
fallbacks. Go transport fixtures verify both direct provider URLs, credentials,
model selection, empty choices, errors, deadlines and cancellation. These tests
do not call real providers or use operator credentials.

Authenticated octet-stream uploads now preserve the existing path/name/size
response, random filename prefixes, 10 MiB per-file limit and 500 MiB shared
quota. Files stream to private disk files and rejected/interrupted writes are
removed. Cleanup runs at startup, daily and near the quota, deleting only files
older than 30 days that the key catalog does not reference. Differential fixtures
cover binary content, safe/unsafe/missing names, empty and maximum-size files,
authentication, rejection and cleanup across Node/Go restarts. The original API
uses shared authenticated storage, not per-user file ownership; that contract
remains unchanged.

Slave startup now honors `WEBMUX_SLAVE_HOST` and `WEBMUX_SLAVE_PORT`. After
ordinary recovery, it removes all terminal sessions (including other owners and
agent panes) and creates the system-owned exec console at row/column zero.
Desktop records remain intact. Differential fixtures verify both migration
directions, explicit/default ports, ownership, persistence and interactive input
through the console; broker tests verify old process and viewer cleanup.

The existing Node route order accidentally intercepted the template list as a
session ID. Both implementations now expose the intended authenticated list;
Node registers templates before the generic session route and explicitly applies
the existing auth middleware. Differential tests compare the entire catalog and
each detail response, missing IDs and unauthenticated access. This is an
intentional routing correction, not preservation of the previous list's 404.

The internal agent service now normalizes agent configuration independently,
checks the multi-user access policy, discovers tmux sessions, assigns duplicate
display names, resolves pane directories and builds attach argument vectors.
It reads existing base64url-named JSON status files and atomically updates them
while preserving unknown hook metadata. Agent HTTP routes now support config,
discovery, attach and scratch. The broker reuses owner-scoped panes, relaunches
changed attach commands and keeps live scratch shells when their selected cwd
changes. Agent sessions remain outside the terminal grid and startup reconnect.
Access checks cover HTTP, WebSocket joins and reconnect; a one-second policy
poll removes revoked agents and closes viewers with code 1008. Malformed app
configuration blocks new access without deleting recoverable saved records.
Activity writes debounce for 200 ms, suppress output during the first 1.5 seconds
of a launch and flush on shutdown. Status inference preserves waiting slop and
recent/stale boundaries. Tests use isolated homes and fixture processes; they
do not contact operator tmux sockets. Timestamp inference now also accepts
JavaScript UTC/local date strings, ISO dates without a time, zone-less local
datetimes, compact numeric offsets, and common US numeric/month-name dates.
Public API contracts check that both servers retain the hook's original spelling
and infer activity consistently; Go unit tests check the actual UTC instants,
including positive/negative offsets and local-time interpretation. This is not
complete JavaScript Date.parse compatibility. A dedicated ISO parser now handles
reduced dates, calendar overflow, 24:00, signed six-digit years, millisecond
truncation, lowercase separators and JavaScript's timestamp range limits. Node
oracle cases and API contracts also reject invalid numeric offsets, comma
fractions, negative zero years and instants outside that range. Local datetime
resolution now follows [ECMAScript's transition rule](https://tc39.es/ecma262/multipage/numbers-and-dates.html#sec-utc-t):
repeated times choose the earlier instant, and skipped times use the offset
before the transition. Node 24 oracle cases cover New York transition boundaries,
Berlin, Lord Howe's half-hour shifts, Apia's skipped day, and supported local
legacy spellings. Six of those cases failed before this correction. Host timezone
database differences and other legacy spellings remain outside verified parity. Display
name ordering still uses English collation; host-locale parity remains open.
Epoch-seconds conversion now also uses JavaScript-compatible numeric coercion
and the full Date range, including fractional milliseconds near year 9999 and
signed six-digit ISO years. Node 24 oracle tests exposed six rejected valid inputs
before the fix. Out-of-range epochs remain ignored instead of propagating Node's
RangeError; this defensive difference is intentional.

VNC and RDP session HTTP APIs now support owner-scoped create/list/get/move/delete
and reconnect state changes. Separate desktop brokers preserve saved-host port
defaults, per-owner/per-protocol grids, compaction and the legacy desktop YAML
files. Passwords remain in memory, are inaccessible to other owners and disappear
on deletion, shutdown or restart. Recovery marks desktops disconnected without
opening network connections. Transport lifetime notifications are available for
the proxies. VNC now supports authenticated binary WebSocket/TCP forwarding,
session ownership, one-use tickets, destination validation and DNS address
pinning. Private network destinations remain supported; the existing explicit
local-target environment override is preserved. Connections stop on deletion,
shutdown or account revocation. Multiple viewers retain independent connection
state so a closing viewer cannot disconnect another live viewer's session state.
The RDP proxy now shares the authenticated desktop transport lifecycle and
connects through the operator-configured guacd endpoint. Its handshake preserves
the existing parameter defaults and sends only the validated destination IP.
Local fake-guacd tests cover credentials, Unicode framing, bidirectional text,
protocol errors and deletion during the handshake. Node/Go differential fixtures
also compare the RDP handshake parameters, ticket authentication/reuse, text in
both directions, connected state and upstream closure. A real guacd/RDP server
and full browser-based VNC/RDP rendering parity remain required before switching
the default backend.

The internal terminal process layer now supports Unix PTYs and Windows ConPTY,
with independent process lifetime, serialized input, resize, exit status,
output draining and idempotent cancellation. SSH/mosh/exec command planning
preserves the existing keepalive, host-key, key-path, shell-template and password
environment behavior. Session routes now launch through this process layer. Its
tests launch a local fixture in a real terminal and check dimensions, Unicode,
large final output, exit codes and blocked-I/O cleanup. Windows runtime results
must be verified in CI; cross-compilation alone does not establish ConPTY parity.

Run from `webmux/server` with Go 1.26 or later:

```sh
go test -race ./...
go vet ./...
go run ./cmd/webmux --root .. --home /tmp/webmux-go-dev --listen 127.0.0.1:18080
```

From `webmux`, `npm run test:contract` builds both servers, runs the same
authentication, catalog and settings expectations against each, then restarts the
opposite server against persisted credentials, catalogs, settings and font files.
This verifies these formats across both migration directions. The settings-only
layout restart fixture has no sessions and therefore no tiles. A separate session
contract checks ownership, defaults, patch precedence, grid limits, reconnect
failure, layout reconciliation and cross-backend session recovery. These HTTP
fixtures deliberately use failed local exec launches; a real Go PTY test covers
initial-command injection, input, resize, reconnect, Unicode and process exit.
The terminal WebSocket contract also runs a checked TypeScript fixture inside a
real PTY through both servers and compares authentication, tickets, input,
dimensions, Unicode, late-viewer replay, focus, leave and deletion. It also
compares transcript defaults, pause/resume status, excluded paused output,
reconnect rotation, deletion footers and POSIX permissions. The broker tests
cover the replay/live boundary, bounded queues, disk failures, drain races and
shutdown flushing. Transcript filenames retain the legacy pattern; launch
generation numbers are per-runtime identifiers, not persisted sequence numbers.
An isolated Go tmux protocol fixture also runs through both servers to compare
agent discovery, errors, sizing, attach/replacement, interactive output, scratch
cwd/reuse, status metadata and multi-user revocation. It never opens a real tmux
socket. Desktop contracts compare both protocols' defaults, host resolution,
ownership, grid moves/compaction, credential exclusion and cross-backend restart
recovery. A local TCP fixture also verifies VNC authentication, ticket reuse,
binary data in both directions and upstream-close semantics against both servers.
The terminal size contract retains an exact dimension assertion and a
ten-second deadline; its TypeScript fixture opens a fresh terminal handle for
each size query because Node's cached stdout dimensions can remain stale on
Windows with cooked input.
Fixtures use temporary homes and never
connect to real terminal or desktop hosts. `npm run typecheck` checks the
TypeScript contract harness as well as the application.

### Browser and visual verification

Build the existing frontend before browser tests (`npm run build`). Run
`npm run test:e2e` against Node or `npm run test:e2e:go` against Go. The runner
initializes an isolated test home; the Go variant builds and launches the native
server. All 12 existing browser workflows pass locally against both backends,
including a real PTY, control-key delivery, search, transcript toggles, settings,
dialog focus and narrow desktop forms. The PTY fixture now uses checked
TypeScript instead of an embedded JavaScript string. Browser specs and Playwright
configuration are also included in `npm run typecheck`.

`npm run test:visual-parity` creates Node screenshots, checks a second Node run
against them, and then compares Go with zero pixel difference and zero color
threshold. An additional
decoded RGBA comparison checks every pixel because Playwright's default image
comparison can ignore anti-aliasing differences even with a zero threshold. All runs use
the same build and Chromium executable, locale, timezone, fonts and scale. Visual
runs disable GPU rendering and force sRGB; ordinary browser workflows retain
their default renderer. The
eight covered states are empty terminal workspace, terminal connection dialog,
settings, empty desktop workspace, and VNC/RDP dialogs at 1280×800 and 375×667.
All eight comparisons pass locally on macOS. Baselines are generated under
`tests/e2e/.visual-baseline` and ignored by Git. Linux and Windows CI now run Go
browser workflows and the sequential visual comparison, uploading image evidence.
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` can select an installed browser; otherwise
Playwright uses its installed Chromium version.

At 98eb3ae, Linux passed browser workflows and the original screenshot gate.
Windows passed the browser workflows but failed its workspace screenshot:
Playwright reported two changed pixels; decoding the images found 13 changed
pixels at toolbar button borders. This remains unresolved; the stricter raw-pixel
gate has not relaxed that requirement. Its eight local macOS comparisons pass.
At 678dffc, Linux and Windows both passed the stricter decoded-pixel comparison
and browser workflows. No UI change explained the earlier Windows difference;
retain it as evidence of intermittent rendering and continue enforcing the gate.
At 30d5808, Linux again differed at toolbar borders (16 raw pixels, RGB changes
of one). The software-rendering configuration and Node-to-Node control now pass
locally across all eight states; their CI results remain unverified. No pixel
tolerance has been introduced.
At fc9faab, the Windows Go race/vet/differential job passed, including the revised
timestamp assertion and slave startup contracts. Browser workflows passed on
Linux and Windows, but both visual jobs failed. Linux failed during the second
Node run, establishing that the baseline is not yet repeatable even with
software rendering; Windows passed the Node control and failed on Go. Renderer
pinning alone has therefore not resolved the visual gate.
Visual failures now attach baseline and actual browser/font information, button
bounds and computed styles alongside the images. These diagnostics do not mask
pixels or relax the gate. A deliberately changed local baseline is rejected and
produces both diagnostic attachments; the unmodified local comparisons pass.
At 8282422, Linux passed its full Go/browser/visual job. Windows passed the Go
race/vet/contracts job but failed the visual comparison again. The uploaded
baseline/actual diagnostics have identical browser version, viewport, font set,
all 15 button bounds and computed styles. This narrows the investigation but
does not establish the cause or satisfy the exact rendering gate.

Using CI's Chromium 145 headless shell locally reproduced the rounded-border
differences between two Node runs, with identical layout/style diagnostics.
The original capture differed in eight pixels by one channel value. Disabling
Skia runtime CPU optimizations did not make repeated captures reliable.
Disabling partial rasterization did: the Node baseline/control/Go comparison
and six further alternating Node/Go runs passed all eight states against the
same baseline. Removing that flag reproduced a four-pixel mismatch in the
terminal dialog; restoring it is the proposed renderer control. The visual
configuration now uses `--disable-partial-raster`, and diagnostics record the
actual rendering flags from Chromium's DevTools protocol. See Google's
[tooling flag documentation](https://github.com/GoogleChrome/chrome-launcher/blob/main/docs/chrome-flags-for-tools.md).
This changes only the screenshot renderer, with no CSS changes, masking, pixel
tolerance or baseline updates during comparisons. Hosted Linux/Windows
verification of this control remains required; it is not yet a completed gate.
At 055bde9, Linux subsequently passed the full Go job, including the exact
screenshot comparison. Windows also passed both its Go race/vet/contracts and
browser/visual jobs. This verifies the renderer control for the eight states
present at that checkpoint; macOS hosted work remains queued.

The visual runner now repeats the Node baseline/control/Go sequence with an
isolated local-authentication fixture as well as trusted mode. Eight additional
captures cover first-time setup, mismatched setup passwords, sign-in and rejected
credentials at desktop and narrow viewports. The same browser flow creates the
owner account, signs out, signs back in and reloads the authenticated workspace.
Credentials are generated per run; screenshot password lengths remain fixed.
All 16 states and both authentication modes pass locally with Chromium 145,
including exact decoded-RGBA comparisons. TypeScript checks pass. CI verification
of these added states is pending.

Six further captures now cover an active terminal, search highlights and the
disconnected overlay at both viewport sizes. A checked TypeScript child runs in
a real PTY and emits fixed ANSI colors, bold text and Unicode. The browser sends
input, verifies its rendered reply, searches two matches, exits the child, then
reconnects and verifies that the fresh screen replaces the prior output. The
fixture hides its cursor through ordinary terminal escape sequences so blinking
does not make the reference image time-dependent. No terminal output or socket
is mocked, and the session is deleted during cleanup. TypeScript and the full
22-state visual sequence pass locally and on Linux and Windows CI at 24cda10.

Four additional captures exercise the real noVNC client against an isolated
RFB 3.8 server through each backend's WebSocket proxy. The fixture supplies a
fixed raw-pixel framebuffer and records keyboard and pointer events. Thumbnail
and fullscreen views at both viewport sizes compare exactly; typing and clicking
also reach the RFB server. The complete 26-state Node baseline/control/Go sequence
passes locally and on Linux and Windows CI at beb7d95. This fixture
covers raw encoding and unauthenticated RFB only.

Four further captures cover the real Guacamole browser client receiving a fixed
desktop from a loopback protocol fixture through each backend. The fixture
validates the RDP handshake and records keyboard and mouse instructions; it is
not a real guacd daemon or RDP host. Thumbnail/fullscreen views are exercised at
both viewport sizes. This exposed an existing fullscreen bug: Guacamole's
negative-z-index canvas rendered behind the viewer's black background, while
the thumbnail's transform kept its pixels visible. The fullscreen viewer now
creates an isolated stacking context. A screenshot-pixel assertion verifies the
actual composited screen independently of the canvas buffer and visual baseline,
so two equally blank backends cannot pass this check. This is an intentional
shared UI correction, not a backend-specific styling adjustment.

The extended run also caught a terminal minimap race: the tile received live
WebSocket state while the overview retained the initial catalog state, sometimes
remaining yellow after connection. Tile state changes now update the workspace's
session state, and the visual test requires both connected and disconnected
overview colors before capturing. This corrects stale UI state without masking
the minimap, delaying for an arbitrary duration, or relaxing pixel comparisons.
The complete 30-state Node baseline/control/Go sequence, strict TypeScript checks,
183 frontend tests and frontend production build pass locally with these fixes.
Hosted validation of the RDP additions and UI corrections is pending.

This is scoped rendering evidence, not proof of every state or platform. Active
agent panes, remaining failure states, real guacd/RDP
and VNC hosts, and performance gates still need coverage before replacement.

`npm run typecheck` now checks source coverage before running all seven strict
TypeScript projects. It rejects tracked or untracked nonignored JavaScript and
TypeScript files outside those projects, resolving existing test-directory
symlinks before comparing paths. Backend/frontend tests and Vite configuration
are included, alongside runtime, browser, helper and tooling sources. Generated
ignored output and installed dependencies remain outside the source inventory.
The expanded check exposed and corrected incomplete session fixtures, readonly
property deletion in viewport cleanup, global fetch mock assignment and missing
Vite test-configuration typing. Both deliberately introduced JavaScript and
unchecked TypeScript probes were rejected locally.

Packaging scripts, browser test runners, the E2E fixture and agent status helper
now have checked TypeScript sources. ESLint and Jest configurations are
declarative JSON. `npm run build:helpers` produces the installable agent helper
and Windows launcher in `webmux/scripts/dist`; generated JavaScript is ignored
by Git. `npm run build` includes this step, and `npm run test:helpers` exercises
the actual compiled status hook. Packaging CI's release-version, tap metadata,
formula preparation and service checks now live in checked
`scripts/packaging-checks.mts`, replacing all inline JavaScript in that workflow.
Release metadata reads/updates/consistency checks, the managed-browser launch
probe, and Windows packaging's Node runtime inspection also use checked
TypeScript. Release metadata tests execute the CLI against temporary fixtures,
preserve root/dependency versions and reject incomplete records before writes;
they run with `npm run test:helpers`. The legacy Node bundle launcher still
requires Node 24, but now checks `node --version` using shell pattern matching
instead of an embedded JavaScript expression. Homebrew's legacy Argon2/PTY
checks use the same checked TypeScript verifier as archive smoke tests.
Targeted tracked-file searches find no `.js`, `.mjs`, `.cjs` or `.jsx` source
files and no inline Node expressions in scripts, formulae or packaging workflows;
generated browser/runtime JavaScript and upstream dependencies remain expected.

### Native bundle preview

`node scripts/package-native.mts [output-directory]` builds a native Go executable
and packages it with the existing production `web` build, `config.defaults`,
license, manifest and SHA-256 checksum. Build the frontend first with
`npm --prefix webmux run build --workspace=frontend`. The packager supports
macOS/Linux/Windows x64 and arm64 hosts, builds for the host with CGO disabled,
and rejects symlinks/non-regular entries. Its explicit allowlist excludes local
configuration, data, Node dependencies and development tools. Node is needed
to run the checked TypeScript build tooling, not to run the extracted server.

The binary discovers the installation root relative to its resolved executable
when adjacent defaults exist; `WEBMUX_ROOT` and `--root` retain explicit override
precedence. Source-tree runs without adjacent defaults retain `.` as their
default. An extracted installation can therefore start from a different working
directory without a JavaScript launcher.

Run `node scripts/smoke-native-package.mts <extracted-directory>` to verify the
actual artifact. The isolated fixture checks HTTP/UI serving, unauthenticated
rejection, bootstrap/login, a real PTY over WebSocket, deletion/closure, login
after restart, and byte-preserved operator configuration. Local macOS arm64
passes; packaging CI now runs this on Linux, macOS and Windows x64/arm64 runners.
Windows termination through Node's child-process API does not establish graceful
Windows service-stop behavior. Native service wrappers, Homebrew/MSI integration
and release publication remain unfinished. Preview artifacts are uploaded
separately and are not selected by the existing release publication job.
At 87844f5, Linux native archive creation and the complete extracted-artifact
smoke test passed. Windows x64 created its archive but Git Bash's GNU tar could
not extract the ZIP, so no Windows runtime assertion ran. Windows extraction now
uses PowerShell's `Expand-Archive`.
At 8282422, Windows extraction succeeded and the server started, but a fixture
cleanup EPERM masked the original assertion failure. The smoke test now retains
all assertion/cleanup errors, terminates its Windows process tree and checks
shell-generated output independently of ConPTY newline encoding. These changes
pass locally on macOS. At 4836ab3, native archive creation and the full extracted
runtime smoke passed on Linux and Windows x64/arm64. macOS CI is still queued.

Windows service registration now resolves the installed runtime: native bundles
launch `bin/webmux.exe` directly, while legacy Node bundles retain their existing
entry point. `-Backend go|node` can select explicitly; auto selects the native
binary when present. Native selection does not look for Node on PATH. Both
packagers include the shared runtime resolver; native Windows archives include
`bin/webmux-service.cmd` and the existing WinSW installer. Account selection,
wrapper checksum verification, external writable state and service controls
remain in that installer.

Windows packaging CI now checks runtime selection with missing files, spaced
paths, explicit choices and an empty PATH for native selection. It also installs
the native WinSW service as LocalSystem on an isolated runner, verifies HTTP/UI,
stop/start, preserved configuration and uninstall. The fixture refuses an
existing WebMux service/wrapper and retains state if service removal fails.
Basic lifecycle checks have passed in Windows CI; they cannot run on the local
macOS host. The fixture now enables transcripts in its private configuration and
calls `scripts/smoke-windows-service-shutdown.mts` before restart. This checked
TypeScript helper launches a real PTY child, waits for 128 output markers, stops
WinSW, and verifies socket closure, child termination, every acknowledged marker
in the transcript and its shutdown footer. TypeScript and the real child fixture
pass locally; Windows x64 passed the new WinSW drain assertion and complete native
MSI/packaging job at c89d6c1. Windows arm64 verification remains pending. Custom-account
behavior and switching default release/service commands remain unfinished.

`scripts/package-windows.ps1 -Backend go` now builds the frontend, native archive
and a `-native.msi` installer with a checksum. The default remains `-Backend node`
during migration. Both variants use the existing per-user WiX product and
installation directory, so they are replacement variants rather than side-by-side
products. WiX now enables same-version major upgrades so swapping backend variants
does not register overlapping products. Removal is scheduled after transaction
initialization so MSI can roll it back if the replacement fails; see the
[WiX upgrade semantics](https://docs.firegiant.com/wix/schema/wxs/majorupgrade/).
Versions must continue to use three meaningful numeric components because MSI
ignores a fourth component when comparing versions.
Windows native CI now builds both variants and exercises both fresh installation
and a same-version Node-to-Go upgrade. It first runs the installed Node bundle,
then checks a single new product registration, absence of legacy backend,
launcher and node_modules files, native runtime/service behavior, and removal
of product registration on uninstall. The first Windows x64 run at a64e6e0 failed
at the fixture's registry-based registration lookup before upgrade. The fixture
now uses MSI's RelatedProducts API keyed by UpgradeCode instead of assuming an
Add/Remove Programs registry location. Windows x64 passed the corrected fresh
installation and same-version replacement at ce2f34c, including one product
registration and legacy-payload removal; arm64 verification remains pending.
`scripts/smoke-upgrade-state.mts` seeds a private home using the installed Node
runtime before replacement, then uses that same home with the installed Go runtime.
It verifies existing account login, the pre-upgrade JWT, saved-host responses and
byte-preserved app/authentication configuration. TypeScript and the Node-to-Go
shared-home sequence pass locally; Windows x64 also passed the installed-MSI
sequence at ce2f34c, including WinSW transcript draining after both installations.
This covers stopped application replacement; active
service migration and forced-failure rollback tests remain outstanding.
The Windows service installer now provides `reconfigure -Backend go|node|auto`
to retarget an existing registration after replacement. It updates only the
runtime command, working directory and installation-root environment entry;
the SCM account, writable home, other environment and recovery/log settings
remain intact. It preserves stopped/running state, atomically replaces XML and
attempts to restore the prior definition if replacement or synchronous restart
fails. The isolated WinSW fixture now exercises running reconfiguration and
repair of a stopped definition that references the removed Node backend, checking
account/environment preservation and successful native startup. Windows execution
at b9ebe58 caught PowerShell coercing the replacement API's null backup path to
an empty string. Both replacement calls now use .NET `NullString.Value`; the
corrected Windows x64 execution passes at beb7d95, including running and stopped
reconfiguration after both fresh installation and MSI replacement. This does not
yet verify custom accounts or delayed startup failure
rollback. The documented migration sequence stops the service before MSI replacement.
Native MSI CI installs on an isolated runner, exercises the installed HTTP/UI,
authentication and PTY runtime, runs the WinSW service lifecycle from that
installation, and uninstalls. It refuses an existing installation/service and
retains the installation if service cleanup fails. These MSI checks await Windows
CI; no MSI has been executed locally on macOS.

At b8dca7a, Windows service-selection fixtures failed before service installation:
two Node installations on PATH produced an array of executable paths. The
resolver now selects the first application, and the fixture requires one string
path. This also corrects the legacy Node service selection behavior; verification
of the actual service lifecycle is still pending.
At cd5181f, Windows x64 passed native archive creation/extraction, runtime smoke,
MSI installation, the installed runtime checks, the actual WinSW service
install/HTTP/UI/stop/start/uninstall lifecycle and MSI uninstall. This resolves
the runtime-selection fixture failure. Windows arm64 and macOS CI remain pending;
upgrade, graceful transcript draining and custom-account tests remain required.

Homebrew now offers `--with-native-server` for a source revision containing the
migration. It installs only frontend build dependencies, uses the shared native
archive packager and installs its payload under `libexec`. Go and Node are
build-only dependencies for this variant. The stable release still defaults to
Node; requesting a native build from a release lacking Go source fails with an
explicit message. The normal `webmux` launcher, Homebrew service command, logs
and external configuration directory remain unchanged. Formula tests exercise
HTTP/UI and password bootstrap/login for both variants, and retain the legacy
native-module/PTY verifier when that helper exists in the source release.

An isolated macOS checkout passed the frontend-only install/build, native archive
creation and full artifact smoke pipeline. Ruby syntax, TypeScript and workflow
checks pass. Linux/macOS native packaging CI now installs the formula from this
revision, runs `brew test`, exercises the installed native payload and verifies
the service definition. At 559c4e0, Linux passed this complete Homebrew path,
including installed-runtime smoke and the service-definition check. macOS
hosted verification remains queued.

### Source builds and process control

The Makefile accepts `WEBMUX_BACKEND=go` for native builds, packaging, manual
start/stop, tests and source service installation. For example,
`make build WEBMUX_BACKEND=go` writes `webmux/bin/webmux` and builds the existing
frontend and checked helpers. `make install WEBMUX_BACKEND=go` uses native
launchd/systemd templates; their executable is the source tree's Go binary.
Changing the backend of an already installed service requires reinstalling its
definition. Normal service-aware start/stop continues to operate the installed
definition. Node remains the default until the full migration gates pass.

Go unit/race tests and vet join the native Make test/lint targets, and browser
tests select the Go runner. The root Makefile's remaining inline Node expression
now delegates to checked TypeScript; broader searches include the Makefile.
`scripts/smoke-source.mts` exercises manual controls in isolated state without
touching installed services. Both Node and Go pass local start/stop/restart,
HTTP, pidfile cleanup and byte-preserved configuration checks. The fixture uses
fresh candidate ports because the existing port selector avoids TIME_WAIT ports.
Native source build and process checks now run in Linux/macOS packaging CI.
`scripts/smoke-unix-service.mts` loads the shipped native template under a random
temporary service label, private configuration and loopback port. It checks real
launchd startup, UI serving, unchanged app configuration, signing-secret
preservation and two graceful stop cycles with zero exit status, followed by a
new process on restart. Each cycle leaves an exec PTY active at shutdown and
checks that its child exits, its WebSocket closes, and all 128 acknowledged
output lines reach the transcript before its shutdown footer. It removes the
registered fixture afterward. The local
macOS GUI-domain run passes; macOS packaging CI now runs this check too (using
the user domain if no GUI domain exists). Operator service labels are untouched.
The same fixture now runs in Linux packaging CI against systemd. It links a
temporary runtime-only unit under a random name, supplies the invoking user's
UID/GID, and uses noninteractive sudo only for manager commands. It checks the
same HTTP/UI, configuration, PID, PTY and transcript assertions, plus clean
systemd exit status and removal of the runtime unit. This lifecycle and the full
Linux native packaging job passed CI at 77ebf68. The native unit uses
[`KillMode=mixed`](https://github.com/systemd/systemd/blob/main/man/systemd.kill.xml)
so the server receives SIGTERM first and manages PTY/transcript shutdown, while
systemd still kills remaining processes after server exit or the stop timeout.
The shared macOS branch and TypeScript
checks pass locally. A system-manager unit with an explicit user does not verify
availability of a login user's systemd bus. This verifies template lifecycle,
not the full Make installer. End-to-end installer coverage, upgrade checks and Windows service transcript-drain
checks remain pending. The macOS check covers output already acknowledged over
the WebSocket; it does not promise to preserve output produced after shutdown begins.

The Make installer now delegates service serialization to checked TypeScript in
`scripts/render-service.mts`. It escapes launchd XML values, quotes systemd
environment/command words, preserves literal percent specifiers and disables
command-line environment expansion so dollar signs remain path data. Path-valued
systemd directives retain their distinct whole-value syntax. Linux commands use
`/usr/bin/env --` to exec the absolute target without a shell: systemd rejects
some characters in its executable token even when quoting is correct. Rendering validates
control characters before writes and atomically replaces the definition with a
private file. Make passes the destination through the environment instead of
embedding it into the renderer command, and service removal quotes that path.
This does not certify arbitrary path characters throughout every Make target.

Tests round-trip special paths through macOS's plist parser for both backends,
verify CLI replacement/failure behavior and run `systemd-analyze verify` on Linux.
Local TypeScript/helper checks and the real launchd lifecycle pass with a state
directory containing spaces and an ampersand. CI at 3708235 exposed the Linux
executable-token restriction, unrelated runner-unit warnings, and CRLF-sensitive
test expectations on Windows. Rendering now normalizes checkout line endings;
parser diagnostics are scoped to the fixture while retaining the exit-status
check. Local tests also execute special-character paths through env. All seven
helper tests and TypeScript checks pass locally; the corrected Linux parser and
helper job passed CI at 652d7ee; Windows's complete test/build/browser/visual job
also passed at that revision. At 3708235 Linux Go/browser/visual checks and Linux and
Windows native packaging passed; macOS hosted jobs remain queued. These
serialization checks complement the successful Linux systemd lifecycle at 77ebf68.

### Backend performance measurements

Run `npm run benchmark:server` from `webmux` using Node 24 and Go. The checked
TypeScript harness builds the native binary, alternates five isolated launches
of each backend, and records startup, backend-only resident memory, 30 terminal
round trips after five warmups, and a 1 MiB terminal transfer. It checks the exact
payload for one PTY and then repeats the workload with four concurrent PTYs,
recording each client's timings and the backend's concurrent RSS. The local
five-round run transferred all 50 MiB without loss or duplication. Concurrent
Node/Go p95 input latency was 0.73/2.34 ms, backend RSS 98.4/23.9 MiB, and median
per-client throughput 10.54/12.21 MiB/s. Go input latency remains higher on this
Mac despite lower memory usage and higher throughput; these measurements do
not establish browser-rendering, remote-network or sustained-load parity.
The Linux CI report at 24cda10 also passes all payload checks. Its concurrent
Node/Go p95 latency is 2.03/2.25 ms, RSS 90.7/17.7 MiB, and median per-client
throughput 8.85/6.44 MiB/s. Go throughput is lower on that runner, so the local
throughput improvement cannot be generalized across platforms.
The harness checks the exact
payload count and fails on lost output or a disconnected viewer. Configuration,
PTY children and listeners belong to the fixture; it does not use operator state.
Unix CI uploads the JSON report, including raw samples and host/toolchain details.
These are measurements with correctness assertions, not relative speed gates.

The initial macOS run found that Go's scrollback processing rescanned the retained
64 KiB on every output chunk. Tracking UTF-16 length incrementally removes that
scan; randomized comparisons retain the previous implementation as an oracle.
Removing the scan exposed a second problem: a local reader could overflow the
128-event viewer queue during a 1 MiB burst. The queue now coalesces adjacent
output for the same session, with each coalesced entry capped at 64 KiB. Control
events retain their order, events shared by viewers remain immutable, and a full
128-entry queue still disconnects the slow viewer without blocking its PTY.
JSON output uses complete WebSocket frames, avoiding write-buffer fragmentation.

A local Apple M4 Pro/macOS arm64 run using Node 24.20.0 and Go 1.27.1 measured:

| Metric | Node | Go |
| --- | ---: | ---: |
| Median HTTP startup | 208 ms | 25 ms |
| Median idle backend RSS | 73.4 MiB | 14.3 MiB |
| Median active backend RSS | 83.2 MiB | 22.0 MiB |
| p95 terminal round trip | 0.32 ms | 3.38 ms |
| Median terminal transfer | 27.4 MiB/s | 50.9 MiB/s |

This single-host workload uses trusted authentication, one local PTY and loopback
networking. RSS excludes the client and child process; startup is polled at 10 ms
intervals. It does not cover browser rendering, remote transports, concurrent
sessions or sustained load. Go input latency is worse in this run; performance
parity remains unproven. The measurement was made with these changes applied on
top of a56722e and an explicitly recorded dirty working tree.

At 2e847f6, the Linux CI benchmark also completed with exact payload delivery
(AMD EPYC 7763, Node 24.20.0, Go 1.26.0). Go/Node results were 15.9/244.7 ms
startup, 11.0/80.6 MiB idle RSS, 16.4/88.3 MiB active RSS, 0.73/1.01 ms p95
input latency and 19.8/20.8 MiB/s transfer. The differing latency/throughput
results reinforce the need for platform and workload coverage before asserting
performance parity. The report is in CI run 35539594099's
`backend-performance-Linux` artifact; that job subsequently failed its separate
exact screenshot comparison.

### Deliberate security differences

- Go rejects tokens for accounts that have been deleted, including refresh and
  ticket requests. Previously issued tickets must also be checked against current
  account/session authorization when WebSocket handlers are implemented.
- JWTs must use HS256 and have an expiry, matching tokens WebMux issues.
- Concurrent bootstrap attempts cannot overwrite the first account. Account
  changes recheck administrator rights in the storage transaction.
- Malformed authentication config fails closed; startup fails if it cannot load
  authentication or persist a new signing secret. There is no ephemeral-secret
  fallback or automatic test signing secret.
- Password verification supports existing Argon2id v19 defaults. Imported hashes
  outside the supported bounds (8–64 byte salt, 16–64 byte digest, 1–16 lanes,
  1–10 passes, memory between 8 KiB per lane and 256 MiB) fail closed. At most two
  password operations run concurrently.
- New state files and replacement configuration files use owner-only permissions
  on POSIX. Windows access control still follows the containing directory's ACL.
- Secure mode requires a usable TLS certificate/key; partial or unreadable TLS
  configuration fails startup instead of silently omitting HTTPS.
- Invalid JSON/types produce bounded JSON errors; oversized bodies receive 413.
- Settings updates validate the effective response before committing, so invalid
  environment limits cannot produce a failed response after saving a change.
- Font file opens use an OS-confined root in addition to canonical-path checks,
  preventing a replaced symlink from escaping the configured directory.
- Production assets also use OS-confined file opens. Dot paths, backslashes,
  Windows stream syntax, non-regular files and symlinks escaping the build root
  return 404 instead of serving arbitrary files or directory listings.
- Upload writers and cleanup serialize quota accounting, preventing concurrent
  requests from oversubscribing the quota. Files use exclusive creation and
  owner-only permissions; aborted and oversized writes are removed. Unsafe or
  overlong filename extensions fall back to `.bin`. Cleanup ignores symlinks,
  recognizes canonical key references and holds the key-catalog lock through
  deletion. An unreadable/malformed catalog disables cleanup rather than treating
  all files as unreferenced. No upload files are exposed through static UI serving.
- AI requests inherit HTTP cancellation and retain the 15-second RCC/20-second
  direct-provider deadlines. Provider responses are bounded to 1 MiB; malformed
  data produces sanitized errors without upstream bodies or credentials. Redirects
  within the configured origin remain supported (up to ten hops); cross-origin
  redirects are rejected rather than forwarding terminal context elsewhere.
  Invalid context/history types and roles produce 400 responses. Provider URLs
  remain operator-configured; the chat body cannot select a destination.
- Terminal launches reject invalid ports, unsupported transports and dimensions
  outside the WebSocket resize limits. Unreadable key/mosh configuration fails
  the launch instead of silently falling back to a different configuration.
- Slave mode rejects malformed, fractional or out-of-range ports before opening
  writable state; empty/zero ports retain the legacy fallback to 22. Reset or
  persistence errors stop startup rather than logging an error and continuing
  with a partially initialized console.
- Mosh key paths use complete shell quoting, including spaces and apostrophes.
  Unix terminal descriptors remain pollable so cancellation can release blocked
  I/O; Windows launch failures release pipes, attribute lists and native handles.
- Corrupt or duplicate saved terminal-session records fail startup instead of
  silently becoming an empty session list. Failed session writes roll back
  in-memory CRUD changes and close any process created by the failed request.
- Grid allocation searches occupied cells instead of iterating through the
  configured grid dimensions. Reconnect launch failures are saved as errors so
  disk state agrees with the failed request.
- Terminal WebSockets enforce session ownership and recheck the ticket/token
  subject against current accounts. The old terminal handler only checked that
  credentials were valid. Existing sockets recheck authorization on messages
  and every five seconds, closing when the account or authentication mode changes.
- Browser WebSocket origins must match the request Host; clients without Origin
  remain supported. Reverse proxies must preserve the browser-facing Host. The
  Vite development proxy now does so, without changing UI rendering.
- WebSocket upgrades share the HTTP request rate limit. Messages are limited to
  1 MiB. Each viewer has a 128-event output queue, and each PTY has an input queue
  bounded by 256 messages and 2 MiB of queued data. Slow viewers or excess
  input receive close code 1013 instead of blocking unrelated sessions or growing
  queues without bound. Viewer disconnection does not terminate its PTY.
- Transcript writes run outside the session lock with a queue bounded by 256
  chunks and 4 MiB. A write failure or overflow disables recording, emits an audit
  error and notifies viewers while leaving the terminal running. A socket may
  have at most 16 pending transcript toggles. Pause/resume requests are serialized
  per session, and draining an old log cannot disable a replacement launch's log.
- Transcript opens are confined to the log directory. Resume rejects a substituted
  symlink or non-regular file instead of appending to its target. Disconnected
  processes cannot start a new transcript through a stale process handle.
- Agent discovery commands have a five-second timeout and 1 MiB limit per output
  stream. Status reads are limited to 1 MiB and confined to the agent directory;
  writes use atomic replacement and owner-only POSIX file permissions. The
  configured home and agent directories remain operator-controlled.
- Concurrent attach/scratch requests serialize pane selection. If persisting a
  policy-driven deletion fails, Go still closes viewers and the revoked process,
  retaining the record for a later cleanup retry. Agent status writes occur
  outside the session lock and shutdown waits for their completion.
- Desktop records reject invalid ports and non-finite/negative positions. Failed
  create/move/delete writes roll back in-memory state. Corrupt, duplicate or
  wrong-protocol saved desktop records fail startup rather than being silently
  discarded. Passwords have no fields in the persisted session types.
- VNC upgrades enforce browser Origin/Host agreement and recheck accounts during
  input and every five seconds. Inbound messages are limited to 1 MiB with four
  queued messages; writes have ten-second deadlines. Destination lookup is
  bounded to five seconds and TCP connection establishment to ten seconds.
  Canonical address checks cover the full IPv6 link-local range and mapped IPv4
  forms, extending the old textual address checks. Dialing uses only the checked
  IP address. Tests use local fixtures or stub DNS answers, not remote hosts.
- RDP uses the same ownership, origin, account and destination checks. Its
  handshake has a ten-second deadline. Guacamole instructions are bounded to
  1 MiB and 1,024 elements; lengths count Unicode code points and separators
  inside values remain data. Complete instructions are forwarded as text frames,
  preserving UTF-8 characters split across TCP reads. This corrects the legacy
  handshake parser's semicolon splitting and UTF-16 length handling. Malformed
  guacd configuration fails closed instead of silently using the default daemon.

These are intentional changes, not claims of byte-for-byte error compatibility.

### Platform verification

The Linux Go CI job passed at commit 2176490. The Windows run exposed rooted
font-path validation and redirected-standard-handle inheritance failures.
Font validation now rejects leading separators on every platform. ConPTY
startup explicitly supplies null standard handles, following the
[Microsoft terminal maintainers' guidance](https://github.com/microsoft/terminal/discussions/15814),
so child terminal I/O does not inherit redirected server streams. Linux and
Windows Go race tests and vet then passed at 150073e, including native ConPTY
tests. Linux's complete Go and differential job passed at 69d2319. Windows still
failed the Node fixture's cached-size assertion despite ConPTY reporting the new
dimensions. The fresh-handle query above addresses that fixture limitation;
a successful Windows differential run remains required.
The subsequent Windows job at 8b98c85 stopped earlier on a shutdown assertion
that treated a buffered status frame as a live socket. That test now drains
buffered frames and requires an actual WebSocket close within its deadline.
Linux's full Go and differential job passed at 9b80338. Windows then exposed an
agent test that assumed record deletion and process shutdown were simultaneous;
the process deliberately closes outside the broker lock to allow output to drain.
That assertion now waits up to five seconds for process shutdown as well.
The complete Linux and Windows Go jobs passed at 5a6df28, including the race
suite, vet and differential contracts. macOS CI remains queued; local macOS
checks pass. Real remote desktop rendering remains outside these fixture checks.
At 30d5808, Windows failed the AI timestamp contract's strict cross-process clock
ordering assertion. The contract now requires an integer Unix-millisecond value
within one second of the request window and prints both clocks on failure;
production timestamps are unchanged. A Windows rerun is required to verify this
change.
