# WebMux Application Workspace

This directory contains the WebMux application workspaces:

- `backend/`: Express, WebSocket, and `node-pty` services
- `frontend/`: React, Vite, xterm.js, VNC, and RDP clients
- `config.defaults/`: templates copied to `WEBMUX_HOME` on first run
- `examples/`: optional configuration examples
- `scripts/`: application build and runtime helpers
- `service/`: Windows service support and macOS/Linux service templates

The repository-level [README](../README.md) is the canonical guide for features, configuration, security, installation, and deployment. Agent workspace setup is documented in [Agent Views](../docs/agent-views.md).

## Development

From this directory:

```bash
npm install
npm run build
npm run typecheck
npm test
npm run lint
```

Run the development servers separately when working on the UI and API:

```bash
npm run dev:backend
npm run dev:frontend
```

The frontend development server proxies API requests to the backend. For browser tests, install Playwright Chromium with `npx playwright install chromium`, then run `npm run test:e2e`. Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` when using an existing compatible Chrome or Chromium installation.

Runtime files do not belong in this directory. WebMux reads configuration and stores state under `~/.config/webmux/` by default; set `WEBMUX_HOME` to use another location.
