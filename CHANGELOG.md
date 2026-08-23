# Changelog

All notable changes to WebMux are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

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
