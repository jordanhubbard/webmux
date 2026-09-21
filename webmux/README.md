# WebMux Application Workspace

This directory contains the WebMux application workspaces:

- `server/`: native Go HTTP/WebSocket server and terminal services
- `backend/`: legacy TypeScript backend for compatibility comparisons
- `frontend/`: React, Vite, xterm.js, VNC, and RDP clients
- `config.defaults/`: templates copied to `WEBMUX_HOME` on first run
- `examples/`: optional configuration examples
- `scripts/`: application build and runtime helpers
- `service/`: Windows service support and macOS/Linux service templates

The repository-level [README](../README.md) is the canonical guide for features, configuration, security, installation, and deployment. Agent workspace setup is documented in [Agent Views](../docs/agent-views.md).

## Development

From this directory:

Go 1.26+ and Node.js 24+ are required for source builds. `npm run build` builds
the Go server and browser UI; `npm start` launches the native binary. Use
`build:node` and `start:node` for the legacy server. `npm run test:e2e` tests Go;
`test:e2e:node` tests a separately built legacy backend.

```bash
npm install
npm run build
npm run typecheck
npm test
npm run lint
```

The existing TypeScript backend watch loop remains available for compatibility
development. Run it and the frontend development server separately:

```bash
npm run dev:backend
npm run dev:frontend
```

The frontend development server proxies API requests to the backend. For browser tests, install Playwright Chromium with `npx playwright install chromium`, then run `npm run test:e2e`. Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` when using an existing compatible Chrome or Chromium installation.

Runtime files do not belong in this directory. WebMux reads configuration and stores state under `~/.config/webmux/` by default; set `WEBMUX_HOME` to use another location.
