# Changelog

All notable changes to WebMux are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

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
