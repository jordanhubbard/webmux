# WebMux Application Workspace

This directory contains the WebMux application workspaces:

- `server/`: native Go HTTP/WebSocket server and terminal services
- `frontend/`: React, Vite, xterm.js, VNC, and RDP clients
- `config.defaults/`: templates copied to `WEBMUX_HOME` on first run
- `examples/`: optional configuration examples
- `scripts/`: application build and runtime helpers
- `service/`: Windows service support and macOS/Linux service templates

The repository-level [README](../README.md) is the canonical guide for features, configuration, security, installation, and deployment. Agent workspace setup is documented in [Agent Views](../docs/agent-views.md).

## Development

From this directory:

Go 1.26+ and Node.js 24+ are required for source builds. `npm run build` builds
the browser UI and a standalone Go executable at `bin/webmux` (`bin/webmux.exe`
on Windows); `npm start` launches it. The executable embeds the browser assets
and default configuration, so it can be copied and run independently.
`npm run test:e2e` tests the browser against Go. Root `npm test` includes Go,
frontend and helper tests; `npm run lint` includes Go vet and TypeScript lint.
`npm run test:contract` checks API, WebSocket and persisted-state contracts.
`npm run test:visual` checks screenshot repeatability for both auth modes.

```bash
npm install
npm run build
npm run typecheck
npm test
npm run lint
```

Run `npm run dev:backend` to build and start the native backend, then run the
frontend development server in a separate terminal:

```bash
npm run dev:frontend
```

Stop and rerun `npm run dev:backend` after Go changes. `npm start` runs the existing native binary without rebuilding.

The frontend development server proxies API requests to the backend. For browser tests, install Playwright Chromium with `npx playwright install chromium`, then run `npm run test:e2e`. Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` when using an existing compatible Chrome or Chromium installation.

Runtime files do not belong in this directory. WebMux reads configuration and stores state under `~/.config/webmux/` by default; set `WEBMUX_HOME` to use another location.

## Standalone builds and development overrides

After building the frontend, the Go build command can also be used directly:

```bash
cd server
go run ./cmd/build -o ../bin/webmux
# Cross-compile from the same frontend build:
go run ./cmd/build -goos windows -goarch arm64 -o ../bin/webmux-windows-arm64.exe
```

The command stages source and assets privately and embeds only `web/` and
`config.defaults/`. It requires the frontend build first and fails if required
inputs are missing. Node is needed to build the frontend, not to run the result.

`--root <directory>` or `WEBMUX_ROOT` optionally supplies external `web/` and
`config.defaults/` directories. Each existing directory overrides its embedded
counterpart; missing directories use the embedded version. Existing writable
configuration in `WEBMUX_HOME` always takes precedence over defaults. Without
an explicit override, a standalone executable ignores assets beside the binary
or in its working directory. The npm launcher and Make source controls set
`WEBMUX_ROOT` to the checkout so local frontend rebuilds remain visible.

Plain `go build ./cmd/webmux` and `go test ./...` remain available for backend
work without a frontend build. That unembedded development executable needs
`--root` pointing to the application directory; it is not a release artifact.
