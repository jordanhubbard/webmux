# Changelog

All notable changes to WebMux are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [1.4.0] - 2026-10-31

### Added
- Connect independently running WebMux servers through an optional authenticated
  directory. Keep peer credentials on the servers, map inbound grants to one
  session owner, verify TLS and revoke active viewers when access changes (#238).
- Choose This server, a selected server or All servers. Show server and durable
  project/session labels, interact with remote terminals, and hide or reconnect
  viewers without duplicating or stopping providers (#238).
- Send a message to an explicit server/session without requiring Rig or Agent
  Deck. Report terminal-input queue admission separately from agent processing
  and retain authenticated source/target audit metadata (#238).

### Fixed
- Distinguish provider state from viewer transport, retain stale-state warnings
  during server outages, and bound unresponsive peer connections with heartbeats.
- Keep directory polling separate from the mutation request budget and require
  confirmation before restarting a stopped provider.
- Avoid reporting dropped background resize controls as unsent user input;
  continue reporting actual input failures during reconnect gaps.
- Update the lint toolchain's minimatch dependency to 11.0.3 and retire the old
  brace-expansion override so its compatible fixed dependency is used (#240).

### Documentation
- Record the recovered multi-server priority and remaining work in the active
  roadmap; document configuration, ownership, message semantics and limitations.

## [1.3.29] - 2026-10-28

### Changed
- Restore the terminal-first home: start with a 2×2 grid, keep coordination in
  its own tab, and put display preferences, account/host tools, and per-session
  browser helpers behind deliberate controls (#228).
- Guide agent creation from a code folder, suggest editable project and
  purpose-based session names, and keep coordinator activity collapsed (#228).
- Show durable project/session identity in terminal titles, the dock and browser
  window titles. Use coordinator-owned identity for managed sessions and saved
  project labels for ordinary terminals, independently of changing CWD (#228).

### Fixed
- Hide dot-prefixed folders in the coordination workspace browser by default;
  use **Show all** to reveal them (#230).
- Keep **Code folder** synchronized with the folder shown in the browser,
  including parent-folder navigation, so launching an agent does not require
  selecting the same folder again (#232).

### Documentation
- Explain Agent Deck's supported pane-style setting for inheriting WebMux's
  terminal colors instead of using a desktop-dependent fixed background. This
  setting must be applied to an existing Agent Deck configuration explicitly
  (#233).

## [1.3.28] - 2026-10-28

### Fixed
- Surface the active coordinator in a toolbar menu with Agent Deck, Rig, and
  None; expose the same choices directly in the Coordination dialog (#227).
- Use Agent Deck by default when upgrading a configuration without a saved
  coordination provider; preserve explicit provider selections (#227).
- Label disconnected Rig diagnostic sessions accurately, offer explicit
  reconnection after server restarts, and show Rig diagnostics only for the Rig
  coordination provider (#227).
- Retry macOS service registration while launchd finishes unloading the previous
  job, avoiding intermittent bootstrap error 5 during source upgrades (#225).
- Build the replacement server before interrupting the installed service, so a
  failed source build or parallel `make install-all` leaves the running server
  available (#225).
- Check that the macOS LaunchAgent remains running before reporting a successful
  server installation; propagate startup errors and point to the server log
  instead of silently accepting an exited job (#225).

## [1.3.27] - 2026-10-27

### Other
- Trust Agent Deck tap in package installs (#221)
- Install Agent Deck with WebMux (#221)
- Complete Agent Deck coordination messaging (#221)

### Added
- Make Agent Deck a complete multi-host coordination path: show conductor and
  worker relationships, waiting/completed/failed attention, safely queue or send
  instructions, follow delivery outcomes, and collect durable remote completion
  records into a selected local conductor (#221).
- Install the qualified Agent Deck release by default with source server
  installs, make it a Homebrew dependency, put its binary directory on the
  service PATH, and document installation on every execution host (#221).

### Changed
- Treat Agent Deck as the recommended session conductor and Rig as the optional
  topology-centric provider without mixing identities or silently falling back
  between them (#221).

### Fixed
- Accept Agent Deck's successful human-readable empty-profile and empty-remote
  responses as empty inventories so a fresh installation opens Coordination
  without a JSON error (#221).

## [1.3.26] - 2026-10-26

### Added
- Replace the fixed Rig project form with a canonical project/session browser,
  explicit workspace picker for local and SSH hosts, active/stopped/history
  state, and direct creation in an existing Rig pod or a new topology (#209).
- Add a versioned coordinator-provider contract and optional Agent Deck adapter
  that inventories local and registered-remote sessions with host-qualified and
  native Codex/Claude identities, then launches or attaches on the owning host
  (#209).
- Make Agent Deck the default coordination experience for new installations,
  add a persistent provider selector in Settings, and let users temporarily
  switch providers in Coordination without silently falling back. Upgrades
  without a saved preference receive a one-time choice (#209).

### Fixed
- Resolve `.local` WebMux server names through mDNS in static Linux
  `webmux-client` builds instead of sending them to unicast DNS (#208).
- Direct client installs to the exact URL shown by the Managed host dialog
  instead of assuming HTTPS on port 8443 with an unspecified certificate name
  (#208).

## [1.3.25] - 2026-10-25

### Added
- Add an optional `webmux-client` that connects outward to an authenticated WebMux server. Pair it explicitly with a saved SSH host to show OS, architecture, CPU count, optional load average, and last check-in; refresh facts, unlink the host, or revoke the client from the Host dialog (#199).
- Publish separate client-only and server-only native archives for macOS, Linux, Windows x64, and Windows ARM64 alongside the combined archives and Windows MSI (#204).

### Changed
- Make `make install` build and install only the client. Use `make install-server` for the server service or `make install-all` for both; corresponding build, package, and uninstall targets are separate (#202).
- Require all three archive variants and their checksums in the native release gate (#204).

### Limitations
- The managed-host channel currently reports host facts. Remote Rig project and seat observation remains follow-up work (#200). The user starts and pairs each client; installation alone does not enroll a host (#199).

## [1.3.24] - 2026-10-24

### Added
- Make Rig seats part of the 2D terminal workflow: choose a seat or host from an empty cell, place a seat in that cell, and use a persistent project and seat navigator to reopen viewers (#194).
- Show Type to All recipients and allow selection of all terminals, one Rig project, individual terminals, or none (#194).

### Changed
- Distinguish opening a Rig viewer from restarting its provider, identify viewer-only reconnect and detach controls, and report unverified provider activity as unknown (#194).
- Keep Rig controls out of non-admin workspaces and explain how the optional tmux-only Agents pane relates to Rig seats (#194).

### Fixed
- Make Rig's Open terminal, reconnect and create actions reveal and focus the requested terminal, including offscreen and minimized views. Restore minimized views into a free grid cell without duplicating the agent or its viewer (#191).
- Let wheel scrolling move each managed Rig terminal's tmux history without moving the whole grid, for local and remote seats (#193).

### Limitations
- Rig seats created outside WebMux still need canonical discovery and adoption; safe stop controls across local and remote providers remain open (#194).

## [1.3.23] - 2026-10-22

### Added
- Add a Rig coordination modal and managed agent terminals in the ordinary grid, with one supervised OpenRig coordinator and SSH execution hosts sharing project identity and messaging. Retain the raw text dashboard as optional diagnostics (#172, #181).
- Show project-worker hierarchy and explicit Rig identity on managed terminal tiles (#178).
- Record and independently verify a live Claude Code/Codex challenge-response-ACK exchange, plus a production Codex coding/review workload against Trickle with WebMux restart persistence (#172, #174).
- Run published-Rig browser coverage for enrollment, attributed messaging, reconnect, grid refresh and tile close/reopen (#174).

### Changed
- Integrate the pending xterm 6, Vite 8 / React plugin 6, and chromedp 0.21 / CDP 0.158.1 upgrades together (#170).
- Require new linked worktrees to use the repository-owned Literate AI worktree root (#182).

### Fixed
- Keep browser-test tmux cleanup outside Playwright's web-server process group. Treat absent or refused private sockets as already stopped while retaining failures for listening endpoints and unexpected cleanup errors (#156, #186).
- Scroll near workspace edges during tile drags, and cancel safely on Escape or focus loss (#180).
- Verify remote Codex relay identity with the qualified OpenRig fork (#176).
- Require tmux 3.4 or newer before using Rig coordination, avoiding tmux 3.2a's server crash when probing a stopped seat; pin and verify the qualified CI binary (#184).
- Preserve service PATH precedence for managed Rig launches and reconnect by exact enrolled session ID (#174).
- Send the current terminal size when a viewer connects so text dashboards render correctly on their first frame (#172).
- Require Go 1.27.3 to address the standard-library certificate-trust advisory reported by the dependency audit (#170).
- Update terminal selection fixtures to xterm 6's interactive surface while retaining clipboard-byte and mouse-routing assertions (#170).
- Migrate remote browser actions and screenshot capture to the typed CDP API, and preserve terminal/browser vendor chunks with Rolldown's supported grouping (#170).

### Limitations
- Coordination requires OpenRig, tmux and supported Node runtimes on the Linux/macOS WebMux server, authenticated provider CLIs, and a Rig client on execution hosts. Remote hosts use OpenSSH and loopback reverse forwarding; no remote daemon is required. Existing independent CLI sessions need explicit adoption. Native Windows coordination hosting is unsupported (#172).
- Positive remote relay identity requires the qualified OpenRig fork; published OpenRig alone does not establish that acceptance (#176).
- macOS production notarization and Windows Authenticode signing remain pending; physical USB/iPhone/Xcode qualification remains open (#152, #63, #120).

## [1.3.22] - 2026-10-17

### Fixed
- Handle disappearing sysfs serial files while the USB integration fixture polls device detach, preserving unexpected I/O failures (#161).

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
