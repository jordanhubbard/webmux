# Frontend, clipboard, and component audit — 2026-09-21

Tracking: #87. Source revision: `438c14e` (native Go server with embedded UI).
This is an audit and local service recovery, not a dependency upgrade or clipboard fix.
Registry versions and advisories are a point-in-time snapshot.

## Results and priorities

1. **Local service recovered.** The existing macOS LaunchAgent still invoked the removed Node backend and was stopped. Rebuilt the frontend/native binary, backed up the old plist, rendered the repository's native template, and reloaded the existing LaunchAgent. Verified the native process listening on 8080, HTTP 200 from `/` and `/api/health`, and a fresh native startup log. Existing configuration/state directories were retained. The build includes pre-existing local terminal-launcher changes; this audit does not commit or review those changes. Health does not establish that every saved remote session successfully reconnected.
2. **Repair dependency metadata first (#93).** 42 installed package entries disagree with their lockfile versions; each incorrect lock version is `1.1.2`. For example scheduler is actually 0.27.0 and esbuild is 0.25.12. Tarball URLs still identify the real versions. `git blame` traces the scheduler overwrite to `d04bd21` (Release v1.1.2); a later `c7249ec` fixed the release script. The current release metadata helper scopes updates to workspace records, but residual corruption remains. A successful `npm ci --dry-run` does not validate tarball package identity.
3. **Update vulnerable development dependencies (#89).** Lock-based `npm audit` reports 19 affected entries: 3 critical, 11 high, 4 moderate, 1 low. A separate registry bulk-advisory query using actual installed package names/versions confirms advisories for **14 package names**. These counts differ because npm propagates findings to dependents and the corrupted lock creates false hits for minimatch and word-wrap. The critical installed packages are **Vitest 1.6.1 and tar 6.2.1**; coverage inherits the Vitest finding. Production npm audit reports zero, and none of the installed browser dependency names appear in the bulk-advisory response. This is absence of known registry findings, not a proof of security.
4. **Repair terminal clipboard UX (#88).** Normal macOS copy/paste works when xterm has focus. Mouse-reporting applications cannot be overridden for browser selection on macOS with the current options; toolbar focus also explains dropped pastes. There is no explicit terminal Copy/Paste control, clipboard feedback, or clipboard-specific automated coverage.
5. **Repair desktop keyboard/clipboard lifecycle (#90).** Inactive Guacamole document keyboard handlers continue to suppress typing after viewer teardown. Both desktop clipboard controls hide failures, and neither implements remote-to-local copy integration.
6. **Refresh toolchain/provenance (#91), then lifecycle and error handling (#92).** CI's Go 1.26.0 setting lags patched releases; the Guacamole npm wrapper is behind Apache upstream despite being npm-latest. Major upgrades require their own compatibility checks.

## Terminal clipboard findings

### Confirmed: macOS cannot force browser selection in mouse-reporting applications

`webmux/frontend/src/components/Terminal.tsx` sets `macOptionIsMeta`, but omits `macOptionClickForcesSelection`. These are independent options. In installed xterm 5.5, the latter defaults to false. `SelectionService.shouldForceSelection()` uses Option plus that option on macOS; Shift is the override only on non-Mac platforms.

A local raw PTY fixture enabled standard terminal mouse reporting and displayed harmless text. In Chrome 153 on macOS, plain, Shift, and Option double-click all produced no xterm copy selection. The same UI with mouse reporting disabled copied `READY` using Cmd+C. This explains failures inside mouse-enabled tmux/TUI sessions, but the user's exact browser/application combination was not supplied.

Remediation: enable and document macOS Option-selection, retain Shift-selection on other systems, preserve application mouse input without the override, and test both mouse-reporting and normal modes. [xterm option documentation](https://xtermjs.org/docs/api/terminal/interfaces/iterminaloptions/).

### Confirmed: toolbar focus makes the next paste disappear

`TerminalActions.tsx` invokes Log without returning focus to xterm. A real browser probe copied `READY`, then pasted Unicode and two lines through Cmd+V. The raw PTY received the exact UTF-8 text with LF normalized to CR. After clicking Log, the active DOM element was the Log button and Cmd+V produced no terminal input frame.

Remediation: preserve/restore terminal focus for terminal actions while retaining keyboard accessibility. Search intentionally owns focus while open and already returns it on Escape. Add selection-aware Copy and Paste controls that call `getSelection()` and `term.paste()`, with clear feedback and a manual fallback where browser permissions do not allow clipboard access. Do not replace paste with raw WebSocket sends: that loses xterm's newline/bracketed-paste processing. [xterm terminal API](https://xtermjs.org/docs/api/terminal/classes/terminal/).

### Other terminal behavior and limits

- xterm already installs native `copy`, `paste`, and context-menu handlers. WebMux does not globally cancel clipboard events or override terminal Ctrl/Cmd keys. A blanket claim that basic xterm copy/paste is broken is not supported.
- Native keyboard paste uses the browser paste event, not `navigator.clipboard.readText()`. Non-local HTTP restrictions on the async Clipboard API therefore do **not** by themselves explain the terminal's keyboard paste failure.
- Terminal output is not ordinary selected page text. An application-owned selection is different from xterm's selection. OSC 52 remote clipboard requests are not implemented by this frontend. If added, use an explicit policy and user-mediated copy flow rather than implicit remote clipboard reads/writes.
- `InputBroadcastContext` routes all xterm `onData`, including paste, to every registered non-excluded session when Type to All is on. Hidden/mounted panes can remain registered. Make paste broadcasting clear and test exclusions and destination paste modes.
- `shouldSuppressTerminalInput()` also examines pasted data. A paste consisting exactly of a filtered terminal response can be discarded. Ordinary text and Unicode are unaffected by the observed test.
- `useWebSocket` has an unbounded CONNECTING queue and silently drops sends while no OPEN/CONNECTING socket exists. The server caps incoming WebSocket messages at 1 MiB. Large paste needs bounded queueing/chunking, explicit failure handling, and Unicode/bracketed-paste tests; do not blindly replay command input after reconnect.
- The click handler updates local focus but does not send the server's `focus` message. The help text about last-clicked multi-viewer focus needs reconciliation. This does not explain the successful single-viewer paste path.
- Ctrl+C must remain available as terminal interrupt. Do not globally capture Ctrl+Shift/editor chords to repair clipboard behavior.

## Desktop clipboard and keyboard findings

`VncFullscreen.tsx` and `RdpFullscreen.tsx` call `navigator.clipboard.readText()` without checking whether `navigator.clipboard` exists. On non-local HTTP it may be absent, causing a synchronous error before the Promise catch. When permission is denied, both swallow the rejection. RDP additionally suppresses stream errors. Show errors and provide manual paste input; use HTTPS for browser APIs that require a secure context.

`VncViewer.tsx` has no clipboard event listener; `RdpViewer.tsx` has no `onclipboard` handler. Remote copy therefore is not connected to a browser copy workflow. RDP's existing TextEncoder/ArrayBufferWriter path correctly avoids the old StringWriter surrogate-pair problem; preserve that behavior.

`RdpViewer.tsx` creates `new Guacamole.Keyboard(document)` and leaves callbacks installed after cleanup, merely setting `active = false`. Installed Guacamole's listeners check whether callbacks exist and can still prevent default input when callbacks do nothing. A browser probe using the installed library confirmed an ordinary input accepted text before creating the handler and accepted none after the component's inactive-cleanup pattern. This is a confirmed library/lifecycle interaction; a full live-RDP navigation test remains required.

Scope keyboard listeners to the viewer, release/reset keyboard state, and neutralize/remove listeners during cleanup. Test returning from RDP to terminals and forms. Desktop options currently use clickable divs without keyboard semantics; replace them with accessible controls.

## Frontend review coverage

Static inspection covered component entry points, event handling, state/lifecycle code, network paths, CSS selection/overlays, and dependency integration. Dynamic evidence is limited to the tests and probes listed below; this is not an exhaustive cross-browser or penetration test.

| Area | Components/utilities | Assessment / follow-up |
| --- | --- | --- |
| App shell/auth | App, main, useAuth, authSession, LoginPage | Expiry events and modal gating present; token remains in localStorage and WS URLs. Prefer short-lived WS tickets / protected-cookie design in a separate security design review. No frontend HTML-injection sink found in searched application source. |
| Terminal | Terminal, TerminalActions, terminalInput, terminalFont, terminalSizing | Selection/focus issues above; parser filters, fit/search/link addons and font lifecycle inspected. Preserve PTY keys and bracketed paste. |
| Input transport | useWebSocket, InputBroadcastContext | Queue/drop/backpressure and paste-broadcast behavior above. Cleanup clears queued messages on session teardown. |
| Terminal tiles/grid | Tile, Workspace, WorkspaceMinimap | Drag prevention is scoped to chrome; no broad terminal selection blocker found. File upload appends a raw returned path through sendInput; shell-specific quoting and visible upload errors need review. Two-request swaps can persist partially while UI rolls back. |
| Agent views | AgentWorkspace | Request IDs avoid displaying stale attachments, but late successful attachment responses are not deleted. Polling can overlap. Add cancellation/cleanup tests. |
| Desktop grid | GraphicsWorkspace, VncWorkspace, VncTile, RdpTile | Combined grid is active in App; legacy VncWorkspace remains separately tested. Error reporting and partial move rollback need work. |
| Desktop viewers | VncViewer, RdpViewer, VncFullscreen, RdpFullscreen | Confirmed keyboard lifecycle and missing/fragile clipboard integration above. Native connection teardown paths inspected; actual remote protocol compatibility not retested here. |
| Forms/settings | ConnectionDialog, VncConnectionDialog, RdpConnectionDialog, SettingsDialog, UsersDialog | Basic validation, busy/error states, password field types and shared modal use inspected. Some background fetch/mutation failures remain silent. |
| Modal/accessibility | Dialog, Dialog.css, AddSessionCell, ReconnectOverlay | Native showModal provides inert background and focus restoration; tested desktop and narrow layouts. Desktop option menus remain an exception. |
| Navigation/help | TopBar, HelpDialog, WorkspacePaneContext | Explicit terminal controls preserve PTY shortcuts. Help omits copy/paste and selection modifiers; focus and broadcast wording needs correction. |
| Supporting code | api, themes, viewport, types, frontend build config | Same-origin REST, protocol-aware WS URLs, event cleanup and guarded theme storage inspected. noVNC transform is version-sensitive; retain tests during upgrade. |

## Dependency findings

### Actual installed packages with advisories

Bulk query sent only public package names/versions to the npm advisory registry. Rows use the maximum severity returned for each package; conditions and reachability vary. Vite/Vitest server advisories apply when those development servers run, not to the static frontend served by Go. Archive tooling can still matter during installation/build, even though it is not a runtime dependency.

| Package | Installed version(s) | Max severity | First action |
| --- | --- | --- | --- |
| [@babel/core](https://github.com/advisories/GHSA-4x5r-pxfx-6jf8) | 7.29.0 | low | Refresh patched Babel dependency |
| [baseline-browser-mapping](https://github.com/advisories/GHSA-w5vr-8v7q-w6rv) | 2.10.0 | moderate | Refresh to >=2.11.0 |
| [browserslist](https://github.com/advisories/GHSA-c83g-rgw3-j3cx) | 4.28.1 | high | Refresh above affected <=4.28.6 |
| [esbuild](https://github.com/advisories/GHSA-67mh-4wv8-2f99) | 0.21.5, 0.25.12 | moderate | Nested 0.21.5 is affected; direct installed 0.25.12 is outside reported range |
| [flatted](https://github.com/advisories/GHSA-rf6f-7fwh-wjgh) | 3.4.1 | high | Refresh above affected <=3.4.1 |
| [form-data](https://github.com/advisories/GHSA-hmw2-7cc7-3qxx) | 4.0.5 | high | Refresh to >=4.0.6 |
| [js-yaml](https://github.com/advisories/GHSA-52cp-r559-cp3m) | 4.1.1 | high | Patch 4.x to 4.3.2 before considering 5.x |
| [picomatch](https://github.com/advisories/GHSA-c2c7-rcm5-vvqj) | 2.3.1, 4.0.3 | high | Refresh to >=2.3.2 / >=4.0.4 on relevant branches |
| [nanoid](https://github.com/advisories/GHSA-28wg-ghj8-5hjv) | 3.3.11 | high | Refresh 3.x to >=3.3.18 |
| [postcss](https://github.com/advisories/GHSA-6g55-p6wh-862q) | 8.5.8 | high | Refresh above affected <=8.5.22 |
| [tar](https://github.com/advisories/GHSA-23hp-3jrh-7fpw) | 6.2.1 | critical | Upgrade/remove legacy argon2 pre-gyp chain; use patched tar >=7.5.21 |
| [vite](https://github.com/advisories/GHSA-p9ff-h696-f583) | 5.4.21, 6.4.1 | high | Patch direct Vite to 6.4.3; remove vulnerable nested Vite via Vitest upgrade |
| [vitest](https://github.com/advisories/GHSA-5xrq-8626-4rwp) | 1.6.1 | critical | Upgrade with coverage; patched >=3.2.6, current 5.0.1 |
| [ws](https://github.com/advisories/GHSA-96hv-2xvq-fx4p) | 8.19.0 | high | Upgrade to 8.21.3 (high finding fixed >=8.21.0) |

Lock-only findings additionally name `@mapbox/node-pre-gyp`, `@vitest/coverage-v8`, and `vite-node` through their dependencies. Locked minimatch `1.1.2` and word-wrap `1.1.2` are false metadata hits: installed versions are 3.1.5 and 1.2.5 and receive no bulk advisories. Repair the lock before using scan counts as a gate.

### Every direct npm dependency

Current versions below are actual installed versions (also matching direct dependency lock entries), not requested semver ranges. Latest is the registry `latest` tag on the audit date, not an automatic upgrade recommendation. All root dependencies are development dependencies after removal of the Node backend. TypeScript appears in both workspaces deliberately.

| Workspace | Use | Package | Current | Latest |
| --- | --- | --- | --- | --- |
| root | development | @playwright/test | 1.58.2 | 1.63.0 |
| root | development | @types/node | 20.19.37 | 26.6.2 |
| root | development | @types/js-yaml | 4.0.9 | 4.0.9 |
| root | development | @types/jsonwebtoken | 9.0.10 | 9.0.10 |
| root | development | @types/ws | 8.18.1 | 8.18.1 |
| root | development | argon2 | 0.31.2 | 0.45.1 |
| root | development | jsonwebtoken | 9.0.3 | 9.0.3 |
| root | development | js-yaml | 4.1.1 | 5.4.2 |
| root | development | ws | 8.19.0 | 8.21.3 |
| root | development | typescript | 5.9.3 | 7.0.2 |
| frontend | runtime | @novnc/novnc | 1.6.0 | 1.7.0 |
| frontend | runtime | @xterm/addon-fit | 0.10.0 | 0.11.0 |
| frontend | runtime | @xterm/addon-search | 0.16.0 | 0.16.0 |
| frontend | runtime | @xterm/addon-web-links | 0.11.0 | 0.12.0 |
| frontend | runtime | @xterm/xterm | 5.5.0 | 6.0.0 |
| frontend | runtime | guacamole-common-js | 1.5.0 | 1.5.0 |
| frontend | runtime | react | 19.2.4 | 19.3.0 |
| frontend | runtime | react-dom | 19.2.4 | 19.3.0 |
| frontend | development | @testing-library/dom | 10.4.1 | 10.4.2 |
| frontend | development | @testing-library/jest-dom | 6.9.1 | 7.0.1 |
| frontend | development | @testing-library/react | 16.3.2 | 16.3.3 |
| frontend | development | @types/guacamole-common-js | 1.5.5 | 1.5.5 |
| frontend | development | @types/novnc__novnc | 1.6.0 | 1.6.0 |
| frontend | development | @types/react | 19.2.14 | 19.3.0 |
| frontend | development | @types/react-dom | 19.2.3 | 19.3.0 |
| frontend | development | @typescript-eslint/eslint-plugin | 7.18.0 | 8.70.1 |
| frontend | development | @typescript-eslint/parser | 7.18.0 | 8.70.1 |
| frontend | development | @vitejs/plugin-react | 4.7.0 | 6.1.1 |
| frontend | development | @vitest/coverage-v8 | 1.6.1 | 5.0.1 |
| frontend | development | eslint | 8.57.1 | 10.11.0 |
| frontend | development | jsdom | 24.1.3 | 30.1.0 |
| frontend | development | typescript | 5.9.3 | 7.0.2 |
| frontend | development | vite | 6.4.1 | 8.3.0 |
| frontend | development | vitest | 1.6.1 | 5.0.1 |

Upgrade xterm core/fit/web-links as a compatible set: 6.0.0 / 0.11.0 / 0.12.0; search is already 0.16.0. Retest selection, search decorations, Unicode widths, resize, terminal replies, IME and platform modifiers. Update React/ReactDOM together. noVNC 1.7.0 needs review of the build-time browser.js patch and live VNC behavior. Guacamole npm-latest 1.5.0 is a third-party wrapper, not Apache-latest; evaluate a maintained distribution of Apache 1.6.0 instead of assuming npm-latest means current upstream.

ESLint 10 and TypeScript 7 are separate migrations. Align typescript-eslint parser/plugin, plugin-react, Vite/Vitest/coverage, jsdom and testing-library versions using their peer requirements. Align @types/node with the supported Node 24 baseline rather than blindly choosing Node 26 types. Keep the existing Node 24 CI baseline until a deliberate runtime/tooling decision.

`webmux/package-lock.json` is the workspace install authority. `webmux/frontend/package-lock.json` is stale (application version 1.2.7 and xterm 6 metadata versus workspace xterm 5.5). Consolidate/remove it after repairing the workspace lock.

### Go modules and standard library

| Direct module | Current | Latest in its module path |
| --- | --- | --- |
| github.com/creack/pty | v1.1.24 | v1.1.24 |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | v5.3.1 |
| github.com/gorilla/websocket | v1.5.3 | v1.5.3 |
| go.yaml.in/yaml/v3 | v3.0.5 | v3.0.5 |
| golang.org/x/crypto | v0.57.0 | v0.57.0 |
| golang.org/x/sys | v0.48.0 | v0.48.0 |
| golang.org/x/text | v0.42.0 | v0.42.0 |

`go list -m -u all` also reports newer module-graph entries x/net v0.58.0 → v0.59.0 and x/tools v0.49.0 → v0.50.0. These are graph entries, not proof of code linked into the server.

Govulncheck v1.8.0 with Go 1.27.1 found zero vulnerable imported packages or reachable symbols for Darwin/arm64, Linux/amd64 and Windows/amd64 source configurations. It reports [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) at module level: x/crypto's unmaintained OpenPGP package, which WebMux does not import. There is no fix version; do not remove the server's cryptography dependency on that basis.

The local binary uses Go 1.27.1. CI uses `go-version-file` with `go 1.26.0`; latest supported patches are 1.26.8 and 1.27.1 according to [Go downloads](https://go.dev/dl/). Update the CI build toolchain independently of minimum language requirements and scan exact release binaries. The current scan does not certify binaries built with Go 1.26.0.

### External tools, packaging, and supply chain

| Component | Observed/configured | Review result |
| --- | --- | --- |
| Node/npm | Local Node 26.9.0; CI/Homebrew Node 24 | Local passing tests do not substitute for Node 24 CI. Node is build/test-only for the native runtime. |
| OpenSSH | Local Apple OpenSSH 10.3p1 / LibreSSL 3.3.6 | OS-managed local executable; formula declares openssh. Assess OS/distributor advisories, not just portable version strings. |
| mosh | Homebrew 1.4.0_42 | Formula revision 1.4.0_43 available in queried metadata. No CVE conclusion inferred from revision alone. |
| tmux | Resolved custom build reports next-3.7; Homebrew also has 3.7c | Actual PATH-resolved binary is not the Homebrew binary; record custom build provenance. |
| sshpass | Not found in local PATH | Optional password-SSH dependency; no installed-version audit possible. |
| guacd | Local custom binary 1.6.0 | Matches Apache current release. Browser npm wrapper is only 1.5.0. Record native plugins/libraries separately. |
| guacd native libraries | libguac plus cairo, jpeg-turbo, libpng, OpenSSL, ogg/vorbis and webp | Linkage inspected. Full plugin/transitive SBOM and advisory matching remain deployment work; npm/Go scans do not cover them. |
| WiX | Packaging pins 5.0.2; upstream latest v7.0.0 | Evaluate supported migration and licensing/build requirements with Windows MSI install/upgrade tests. No specific WiX vulnerability established here. |
| GitHub Actions | checkout/setup-go/setup-node/upload-artifact v7; download-artifact v8 | Current major lines. Queried latest patches: checkout/upload v7.0.1, setup-go/setup-node v7.0.0, download v8.0.1. Prefer reviewed SHA pins with update automation. Homebrew setup is already SHA-pinned. |
| Release formula | Source tag v1.3.12 plus SHA256 | Versioned source and hash are present. Inspect exact release binary metadata/SBOM rather than treating source scans as release certification. |
| Security automation | No dependency bot config or npm/govulncheck audit gate found | Add scheduled and PR scans for runtime and development dependencies, plus exact native artifact scans. Existing signing/WinGet work is tracked in #63/#65. |

[Apache security reports](https://guacamole.apache.org/security/) distinguish guacd terminal/VNC/RDP native flaws from browser code. For example, the terminal-emulator issue fixed in 1.6.0 is not proof that the browser JS wrapper has that flaw; WebMux uses xterm/Go for its terminal path. The local guacd version alone does not establish the patch status of every library or every remote deployment.

## Validation and boundaries

- Native/frontend production build passed. Native LaunchAgent state, executable, listener, UI, health endpoint and startup log verified.
- `npm run lint` passed (Go vet and frontend ESLint).
- Frontend unit suite: 19 files, 183 tests passed. Existing useAuth test emits an act warning.
- Go test suite passed, including the pre-existing local launcher changes.
- Playwright interaction suite: 4 passed in macOS Chrome 153.0.8010.52, including real PTY control keys and modal focus tests.
- Additional isolated browser probes confirmed normal Cmd+C/Cmd+V with Unicode/multiline PTY delivery, lost paste after Log focus, lack of mouse-mode selection override on Mac, and inactive Guacamole document-keyboard suppression. Probes used temporary fixtures and did not interact with saved user sessions.
- Current npm full/production audits, actual-installed bulk advisory query, direct registry version inventory, module update inventory, and three-platform source govulncheck completed.
- No dependency versions or frontend behavior were changed by this audit. Actual Safari/Firefox/Windows/mobile clipboard behavior, OS context menus, large-paste recovery, live VNC/RDP interoperability, and exact packaged release scans remain unverified.
- The user did not supply the browser/origin/terminal-app combination for the original failure. Confirmed failure modes explain specific cases; they are not proof of the user's exact trigger.

## Follow-up order and acceptance

1. #93: restore lock identity, then verify a clean install and trustworthy inventory.
2. #89: patch critical/high tooling with focused compatibility work; require clean runtime audit and reviewed development exceptions or zero findings.
3. #88: terminal selection/focus/clipboard controls; require real clipboard-to-PTY and PTY-selection-to-clipboard tests across platforms, plus bracketed paste, broadcast, permission failure and disconnected-state coverage.
4. #90: desktop clipboard and keyboard teardown; require terminal/form input to work after leaving RDP and explicit permission/fallback UI.
5. #91: update CI Go patch level and component provenance; validate actual native release artifacts and Windows MSI migration.
6. #92: lifecycle, backpressure, partial persistence and error feedback, with focused tests per finding.

Keep the restored local service on the native LaunchAgent. Preserve unrelated working-tree changes and existing stashes; this audit's documentation is isolated on its own branch.
