# WebMux

A web-native tmux-on-a-jump-box: a persistent shared terminal wall that runs in your browser.

## Features

- **2D tiled terminal workspace** — scrollable grid of live SSH sessions
- **xterm.js** — full terminal emulation in the browser (256-colour, ligatures, clipboard)
- **node-pty** — proper PTY semantics on the jump box
- **Persistent sessions** — sessions survive browser tab closes; reconnect from any tab
- **Multi-viewer** — multiple browser tabs can watch the same session; one has keyboard focus at a time
- **Split right / split below** — tile your workspace like a tiling window manager
- **Global font size** — resize all terminals at once
- **Two auth modes**:
  - **Secure mode** — local login + HTTPS, Argon2id password hashing
  - **Trusted mode** — no auth, HTTP only, for protected internal networks
- **YAML configuration** — human-editable, lift-and-shift deployment
- **SSH with keepalive** — `ServerAliveInterval`, `ServerAliveCountMax`, `ConnectTimeout`
- **Audit log** — JSONL append-only event log

## Quick Start

### Prerequisites

- Node.js ≥ 20
- `ssh` available on the jump box
- (Optional) `sshpass` for password-based SSH auth

### Install & Build

```bash
cd webmux
npm install
npm run build
```

### Configure

Edit `config/app.yaml`:

```yaml
app:
  listen_host: 0.0.0.0
  http_port: 8080
  secure_mode: false        # true = require login + HTTPS
  trusted_http_allowed: true
  default_term:
    cols: 80
    rows: 24
    font_size: 14
```

Add hosts to `config/hosts.yaml`:

```yaml
hosts:
  - id: build01
    hostname: build01.example.com
    port: 22
    tags: [linux, build]
    mosh_allowed: false
```

### Run

```bash
WEBMUX_ROOT=$(pwd) npm start
```

Open `http://localhost:8080` in your browser.

On first run with `auth.mode: local`, you will be prompted to create an admin account.

## Directory Layout

```
webmux/
  config/
    app.yaml          # Application settings
    auth.yaml         # Auth mode + hashed password
    hosts.yaml        # Known SSH hosts
    layout.yaml       # Tile positions
    keys.yaml         # Saved key references
    tls/
      cert.pem        # TLS certificate (generate separately)
      key.pem         # TLS private key
  data/
    sessions/         # Persisted session metadata
    events/           # JSONL audit log
    cache/
  logs/
  backend/            # Node.js/TypeScript backend
  frontend/           # React/TypeScript frontend
  web/                # Built frontend (served by backend)
```

Everything lives under one root. Copy the directory to another machine, run `npm install && npm run build`, and start the service.

## Authentication Modes

### Trusted Mode (no auth)

```yaml
# config/auth.yaml
auth:
  mode: none
```

```yaml
# config/app.yaml
app:
  secure_mode: false
  trusted_http_allowed: true
```

Only use on a network you fully control. The UI displays a "⚠ Trusted" badge.

### Secure Mode (local auth + HTTPS)

```yaml
# config/auth.yaml
auth:
  mode: local
  bootstrap_required: true   # becomes false after first login
```

```yaml
# config/app.yaml
app:
  secure_mode: true
  https_port: 8443
```

Place your TLS certificate at `config/tls/cert.pem` and `config/tls/key.pem`.

Passwords are stored as Argon2id hashes. Plain-text passwords are never written to disk.

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check |
| `POST` | `/api/auth/login` | Login (returns JWT) |
| `POST` | `/api/auth/bootstrap` | First-run account creation |
| `GET` | `/api/auth/status` | Auth mode + bootstrap status |
| `GET` | `/api/sessions` | List sessions |
| `POST` | `/api/sessions` | Create session |
| `DELETE` | `/api/sessions/:id` | Delete session |
| `POST` | `/api/sessions/:id/reconnect` | Reconnect session |
| `POST` | `/api/sessions/:id/split-right` | Get suggested position (same row, next col) |
| `POST` | `/api/sessions/:id/split-below` | Get suggested position (next row, same col) |
| `GET` | `/api/hosts` | List saved hosts |
| `POST` | `/api/hosts` | Add host |
| `PUT` | `/api/hosts/:id` | Update host |
| `DELETE` | `/api/hosts/:id` | Delete host |
| `GET` | `/api/config` | Get app config |
| `PUT` | `/api/config` | Update app config |
| `GET` | `/api/config/layout` | Get layout |
| `PUT` | `/api/config/layout` | Update layout |
| `WS` | `/api/term/:id?token=<jwt>` | Terminal WebSocket |

### WebSocket Message Types

| Type | Direction | Fields |
|------|-----------|--------|
| `input` | client→server | `data` (string) |
| `resize` | client→server | `cols`, `rows` |
| `focus` | client→server | — |
| `output` | server→client | `data` (string) |
| `status` | server→client | `state`, `message` |
| `viewer_join` | server→client | `viewer_id`, `viewer_count`, `focus_owner` |
| `viewer_leave` | server→client | `viewer_id`, `viewer_count`, `focus_owner` |
| `focus` | server→client | `focus_owner`, `viewer_count` |

## Development

```bash
# Run backend in watch mode
npm run dev:backend

# Run frontend dev server (proxies /api to localhost:8080)
npm run dev:frontend

# Run tests
npm test

# Lint
npm run lint
```

## Security Notes

- Remote **passwords** are never written to disk. They are held in memory only for the duration of session setup (TTL: 5 minutes), then zeroed and discarded.
- **Remote usernames** entered for password-based logins are not persisted.
- **JWT tokens** are signed with `JWT_SECRET` (set via environment variable in production).
- In secure mode, use HTTPS and set `JWT_SECRET` to a strong random value.
- Rate limiting is applied globally (300 req/min) and to the auth endpoints (10 req/15 min).

## Deployment

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WEBMUX_ROOT` | `../..` relative to `backend/dist` | Root directory |
| `HTTP_PORT` | from `app.yaml` | Override HTTP port |
| `HTTPS_PORT` | from `app.yaml` | Override HTTPS port |
| `JWT_SECRET` | `webmux-dev-secret-change-in-production` | **Change in production** |

### systemd

```ini
[Unit]
Description=WebMux Terminal Wall
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/webmux
Environment=WEBMUX_ROOT=/opt/webmux
Environment=JWT_SECRET=<your-secret>
ExecStart=/usr/bin/node backend/dist/index.js
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
