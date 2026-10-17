# Changelog

All notable changes to WebMux are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [1.3.22] - 2026-10-17

### Removed
- Remove the unused Host sessions workspace and external session-manager integration (#162).

### Changed
- Update xterm fit and web-links addons to 0.11.0 and 0.12.0 (#138, #137).
- Update golang.org/x/crypto to 0.58.0, x/sys to 0.49.0, and x/text to 0.43.0 (#134).

## [1.3.21] - 2026-10-16

### Added
- Add a Pack control to compact the terminal grid (#147).
- Add a host session-manager workspace with provider-backed session controls (#149).
- Guide macOS local and SSH folder permissions and install a signed app bundle with a stable application identity (#151).
- Integrate desktop clipboard support, deliver pasted terminal images to their hosts, and support clipboard forwarding between separate WebMux windows (#154).

### Fixed
- Keep one macOS local-access check alive while desktop prompts are answered, remove each granted or denied folder from the pending list, and automatically close and remember completion without remounting the dialog (#157).
- Isolate browser-suite request budgets and keep first-run setup from interrupting terminal workflow fixtures (#154).

### Limitations
- Source and native macOS builds use a development signature unless Developer ID signing and notarization are configured; permission persistence across upgrades remains subject to macOS (#152).

## [1.3.20] - 2026-10-12

### Fixed
- Supply a UTF-8 locale for Mosh when a service manager launches WebMux without locale settings; preserve explicitly configured locales (#142).
- Honor SSH fallback for Mosh startup errors and initial UDP connection timeouts. Preserve diagnostics, retain SSH for reconnects, and avoid fallback loops, duplicate initial commands, or replacing established Mosh sessions (#144).

## [1.3.19] - 2026-10-12

### Fixed
- Silently fall back to a normal SSH terminal when optional browser integration is unavailable (#125).
- Recover browser-companion polling after transient navigation or request failures (#110).
- Scope RDP keyboard handling to the viewer and release held keys on blur; provide explicit desktop clipboard access with a manual fallback (#90).
- Add terminal Copy/Paste controls, preserve selection and bracketed paste, and report disconnected or oversized input (#88, #92).
- Release stale agent attachments, serialize polling, report failed mutations, and compensate/reload partially saved layouts (#92).
- Repair dependency lock identities, remove the stale frontend lock, and update vulnerable build dependencies (#89, #93).
- Give SSH USB tests bounded startup time, guaranteed child cleanup and captured diagnostics; exercise the built CLI and worker with a dummy exporter (#127, #128).

### Changed
- Require Go 1.27.2, pin GitHub Actions by commit, add dependency audit/update automation and include component provenance in native bundles (#91).
- Use noVNC 1.7's ESM entry point without the obsolete CommonJS build patch.
- Test native USB/IP enumeration, storage traffic and interruption cleanup through WebMux in a pinned Linux VM (#128).

### Limitations
- Physical USB/iPhone/Xcode acceptance remains unqualified; the Linux virtual fixture does not qualify macOS/Windows USB drivers. Windows signing requires publisher enrollment; WinGet publication awaits upstream review. Remote identity-provider and advanced browser-companion work remain tracked separately.

## [1.3.18] - 2026-10-10

### Added
- Add a USB setup button in every workspace with native software guidance, validated local forwarding commands, and cross-platform peer instructions (#120).
- Add an experimental, bounded SSH transport for separately configured native USB exporters, with verified host keys, loopback-only receiving listeners and automatic cleanup on expiry or disconnect (#120).

### Fixed
- Treat a closed USB transport pipe as normal shutdown when it races context cancellation.

### Documentation
- Record the frontend clipboard, keyboard lifecycle and dependency audit with tracked follow-ups (#87).

### Limitations
- USB redirection requires compatible native exporter/importer software and destination device drivers. Physical USB compatibility, iPhone/Xcode discovery/install/debug and live browser device status remain unverified; the USB button currently guides setup.

## [1.3.17] - 2026-10-07

### Added
- Session-linked CLI sign-in panels use the viewing browser for device codes and passkeys, with a one-shot, state-bound localhost callback relay and explicit terminal-host browser fallback (#114).

### Fixed
- Keep an older viewer from cancelling a newer sign-in attempt for the same terminal.
- Preserve the pinned Node 24 runtime after Homebrew setup changes the CI PATH, keeping packaging verification reliable (#116).

## [1.3.16] - 2026-10-03

### Other
- Separate browser input rate budget from workspace APIs (#110)
- Accept combined browser keyboard modifiers and explain passkey limits (#110)
- Send companion launches to the active terminal viewer (#110)
- Route CLI browser launches through linked terminal companions (#110)
- Route terminal link clicks into the companion browser (#109)
- Make the terminal browser launcher visible and explicit (#109)
- Add terminal browser companion for remote authentication (#109)

## [1.3.15] - 2026-09-29

### Added
- Add a Localhost · tmux quick-connect option for persistent local shells alongside remote SSH sessions in the same terminal workspace.
- Reattach local terminals to their existing tmux shells across reconnects and WebMux process restarts.

### Fixed
- Unlink the hosted macOS runner's legacy OpenSSL before Homebrew dependency upgrades so packaging checks can complete.

## [1.3.14] - 2026-09-22

### Fixed
- Drain Windows terminal output through ConPTY shutdown and cancel input before terminating the process tree, preventing reconnect and close hangs.
- On macOS, launch a local login shell for same-user loopback sessions using the default SSH port and no explicit credentials, retaining the WebMux process's GUI/keychain context.
- Continue using SSH for custom ports, explicit credentials, other users, remote hosts, and non-macOS platforms.

## [1.3.13] - 2026-09-21

### Changed
- Ship standalone Go executables with the web interface and default configuration embedded.
- Remove the legacy Node.js server; use the Go server for runtime sessions and APIs.
- Use native release bundles for Homebrew installations.

### Fixed
- Terminate Windows terminal process trees on reconnect and close to prevent raw-input children from hanging ConPTY teardown.
- Keep mouse and focus reports in their originating terminal when Type to All is enabled.
- Reset terminal modes on reconnect so stale mouse tracking does not leak into a new process.
- Suppress unbuttoned mouse movement reports that appear as stray characters; clicks, dragging, and scrolling remain available. Application hover behavior is intentionally disabled.
- Allow Option-drag text selection on macOS in mouse-enabled terminal applications and restore terminal focus after toggling transcript logging.

### Documentation
- Clarify terminal copy/paste, selection modifiers, and broadcast behavior in the in-app help.

## [1.3.12] - 2026-09-21

### Other
- Migrate server APIs and terminal sessions to Go with checked TypeScript tooling (#78)
- docs: connect the project narrative to the chronological AI chronicle (#76)

## [1.3.11] - 2026-09-12

### Other
- Unify dialogs and preserve terminal application shortcuts (#70)
- Add Windows ARM64 packaging job (best-effort) (#67)
- Fix vulnerable dependencies and switch release helper to PR-based flow (#66)
- Update Homebrew formula for v1.3.10 (#58) (#62)

## [1.3.10] - 2026-09-10

### Added
- publish a tested Windows x64 runtime ZIP and per-user MSI with checksum files and foreground/service launchers

### Fixed
- document and continuously test the Homebrew 6 formula-trust bootstrap required before tapping the custom repository URL

## [1.3.9] - 2026-09-10

### Added
- install WebMux with one Homebrew formula on macOS and Linux, including per-user launchd and systemd services
- publish macOS ARM64 and Linux x86-64 runtime bundles with SHA-256 checksums
- test Homebrew installations and extracted bundles on both platforms in CI

### Documentation
- document Homebrew installation, migration from source checkouts, upgrades, and configuration preservation

## [1.3.8] - 2026-09-09

### Changed
- require Node.js 24 LTS and upgrade GitHub Actions to their Node 24-based v7 releases
- replace unchecked VNC and RDP client types so frontend lint runs without warnings

## [1.3.7] - 2026-09-09

### Added
- add Ctrl+Shift+L to pause or resume transcript logging for the focused terminal
- show a REC indicator while transcript logging is active

## [1.3.6] - 2026-09-08

### Fixed
- queue WebSocket messages sent during connection setup so the initial terminal resize reaches the remote PTY ([#27](https://github.com/jordanhubbard/webmux/pull/27), thanks [@de-ataka](https://github.com/de-ataka))

## [1.3.5] - 2026-09-08

### Added
- add an operator settings dialog for runtime-safe application configuration
- expose session logging, terminal, workspace, transport, hosted-font, and host-switcher settings from the top-bar gear

### Security
- restrict application configuration writes to administrators in secure mode while retaining operator access in trusted mode

## [1.3.4] - 2026-09-08

### Added
- add opt-in, per-launch terminal transcript logging under `WEBMUX_HOME/logs/sessions/`

### Documentation
- refresh feature, architecture, development, and contributor documentation
- split macOS, Linux, and Windows setup into focused platform guides
- point the WebMux origin story at the successor `mac` project

## [1.3.3] - 2026-08-27

### Added
- show a prominent refresh action in disconnected terminal, VNC, and RDP windows
- add GitHub issue forms and a pull request template

### Changed
- replace beads task management with GitHub Issues and pull requests

## [1.3.2] - 2026-08-25

### Added
- add prominent reconnect overlays

### Other
- bd: update sync.remote

## [1.3.1] - 2026-08-23

### Fixed
- roll back xterm.js 6 mouse regression

## [1.3.0] - 2026-08-23

### Added
- upgrade terminal stack to xterm.js 6

## [1.2.7] - 2026-08-23

### Other
- Fix SSH sessions receiving xterm query escape replies.
- bd init: initialize beads issue tracking

### Fixed
- suppress xterm.js OSC/CPR query replies in SSH sessions so interactive
  programs like `gh auth login` no longer fail with unexpected `\x1b]` escapes

## [1.2.6] - 2026-08-23

### Fixed
- highlight the dock icon for the focused terminal

## [1.2.5] - 2026-08-21

### Fixed
- handle expired login sessions

## [1.2.4] - 2026-08-11

### Added
- admin-managed multi-user accounts with richer login UI

### Fixed
- refresh auth status on logout so login screen shows correct mode
- make start/stop/restart service-manager aware

### Other
- chore: make e2e test harness provision its own browser

## [1.2.3] - 2026-08-07

### Other
- test: wait for terminal cycle state

## [1.2.2] - 2026-08-07

### Fixed
- await persistence shutdown on Windows

### Other
- test: retry Windows temporary directory cleanup

## [1.2.1] - 2026-08-07

### Added
- add native Windows hosting support
- add terminal prompt favicon
- scroll focused terminal into view
- add keyboard terminal cycling
- add terminal grid limits
- add workspace minimap for quick navigation
- add client-side color themes from .itermcolors files
- add shift-to-scroll hint in terminal workspace
- add RDP support via Apache Guacamole (guacd)
- check port availability before binding to avoid collisions

### Fixed
- validate active GitHub account for releases
- await session event writes
- await session event writes
- recognize shifted terminal cycling keys
- load terminal grid config after auth
- add comment to empty catch block in useAuth test
- remove eslint-disable for missing react-hooks plugin
- repair test suite after RDP/rename feature additions
- prevent server crash on PTY resize and stop 1Password triggering
- improve diagnostics for startup port display and VNC connect errors
- correct MCP server config schema in .mcp.json
- add comment to empty catch block in useAuth test
- remove eslint-disable for missing react-hooks plugin

### Other
- Confine hosted fonts to config directory
- Improve touch terminal sizing and input handling
- Add config-directed terminal font hosting
- Add configurable terminal font family
- Fix agent session access enforcement
- Document agent views setup
- Add optional agent status hook
- Add configurable agent workspace UI
- Add configurable agent session backend
- Fix upstream PR integration issues
- test: clean up frontend renders
- test: clean up frontend renders
- security: code-review pass — auth, SSRF guard, input validation
- Persist minimize state on server for cross-device sync
- Restore minimized tiles to nearest empty cell instead of original position
- Force terminal re-render on restore from minimized
- Fix CI: update tests for new props, fix upload size limit handling
- Fix drag-and-drop, sticky tab bar, visual bell, and content preservation
- Persist minimize state across reloads and add direct geometry input
- Sort tab bar alphabetically by session title
- Add minimize/restore for terminal tiles with tab bar
- Add window lock/delete protection with global and per-window toggles
- Add configurable auto-scroll with global and per-window toggles
- Remove AI sidebar, suppress 1Password doorhanger
- Fix CI: update tests for new props, fix upload size limit handling
- Add window lock/delete protection with global and per-window toggles
- Add configurable auto-scroll with global and per-window toggles

## [1.2.0] - 2026-04-12

### Other
- Add VNC desktop session support with dual-pane workspace
- Fix release script clobbering dependency versions in package-lock.json

## [1.1.2] - 2026-03-11

### Other
- Fix release script to include package-lock.json in version bumps

## [1.1.1] - 2026-03-11

### Other
- Add release automation (make release / release-minor / release-major)

## [1.1.0] - 2026-03-11

### Added
- Window drag-to-move: drag any tile by its title bar to swap it with another tile, keeping the grid packed with no holes
- Scroll-to-bottom button (↓) in every window's title bar

### Fixed
- Test flakiness: wrap async `useEffect` calls in `act()` to suppress React warnings
- Wait for component data-loading in ConnectionDialog tests before firing events
- Use `--localstorage-file` flag to isolate localStorage state across vitest runs
- Add `--no-deprecation` to Playwright `NODE_OPTIONS` to silence Node warnings
- Split inline `border` shorthand that triggered a CSS parser warning

## [1.0.0] - 2026-03-05

### Added
- Configurable terminal tile size (cols/rows) with top bar controls
- Persistent service with auto-reconnect on startup
- Grid-based session placement
- Move runtime config to `~/.config/webmux`
- Multi-user auth, session scoping, saved hosts, input broadcast
- Web-native terminal multiplexer with SSH and Mosh support
- xterm.js frontend with React, Vite, and TypeScript
- Express + ws + node-pty backend
- launchd/systemd service management
- Full test suite: unit, integration, and E2E (Playwright)
