# WebMux

A web-native terminal multiplexer — think tmux-on-a-jump-box, but it runs in your browser. WebMux gives you a persistent, shared terminal wall: a scrollable 2D grid of live SSH (or mosh) sessions with full terminal emulation, multi-viewer presence, and input broadcast.

## Features

- **2D tiled terminal workspace** — CSS Grid layout that fills the viewport; click "+" placeholders to add sessions to the right or below any existing tile
- **Full terminal emulation** — xterm.js with 256-color, clickable links, 5000-line scrollback
- **SSH and mosh transports** — proper PTY via node-pty, with keepalive and auto-reconnect
- **Persistent sessions** — sessions survive browser closes and server reboots; auto-reconnected on startup
- **Saved hosts** — save connection profiles for one-click connect; stored with hostname, port, username, transport, and key
- **Multi-user accounts** — multiple users with separate session collections; Argon2id password hashing
- **Multi-viewer presence** — multiple tabs can watch the same session; click-to-focus controls who has keyboard input
- **Type to All** — broadcast mode sends keystrokes to every open session simultaneously
- **SSH key and password auth** — managed keys via `keys.yaml`, password-based via `sshpass`
- **Two security modes** — local auth (Argon2id + JWT + HTTPS) or trusted mode for isolated networks
- **OS service integration** — `make install` sets up launchd (macOS) or systemd (Linux) for auto-start on boot
- **YAML configuration** — human-editable config in `~/.config/webmux/`, separate from the source tree
- **Audit log** — append-only JSONL event log (logins, session lifecycle)
- **Global font size control** — resize all terminals at once

## Quick Start

### Prerequisites

- Node.js >= 20
- `ssh` on the jump box
- (Optional) `sshpass` for password-based SSH auth
- (Optional) `mosh` on both ends for mosh transport

### Build and Run

```bash
make            # install deps + build
make start      # start in background
```

Open `http://localhost:8080`. On first run with local auth, you'll be prompted to create an account.

### Install as a Service

```bash
make install    # installs launchd (macOS) or systemd (Linux) service
```

WebMux will start automatically on login and auto-reconnect any persistent sessions after a reboot.

```bash
make uninstall  # remove the OS service
```

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make` | Install dependencies and build |
| `make start` | Build and start the server (background) |
| `make stop` | Stop the running server |
| `make restart` | Restart the server |
| `make status` | Check if the server is running |
| `make install` | Install as OS service (launchd/systemd) |
| `make uninstall` | Remove the OS service |
| `make test` | Run all tests |
| `make lint` | Lint all code |
| `make clean` | Stop and remove build artifacts |
| `make configure` | Update runtime config from env/args |
| `make help` | Show help with colors |

Override settings on the command line:

```bash
make start HTTP_PORT=9090
make start AUTH_MODE=none
make start SECURE_MODE=true JWT_SECRET=$(openssl rand -hex 32)
```

## Configuration

Runtime configuration lives in `~/.config/webmux/` (override with `WEBMUX_HOME`). On first run, default config files are copied from `config.defaults/` in the source tree.

### `app.yaml` — Application Settings

```yaml
app:
  listen_host: 0.0.0.0
  http_port: 8080
  https_port: 8443
  secure_mode: false
  trusted_http_allowed: true
  default_term:
    cols: 80
    rows: 24
    font_size: 14
  transport:
    prefer_mosh: false
    ssh_fallback: true
```

### `auth.yaml` — Authentication

```yaml
auth:
  mode: local          # 'none' (trusted) or 'local'
  users: []            # populated on first login via bootstrap
```

### `hosts.yaml` — Saved Hosts

```yaml
hosts:
  - id: build01
    hostname: build01.example.com
    port: 22
    username: deploy
    transport: ssh
    key_id: ''
    tags: [linux, build]
    mosh_allowed: false
```

### `keys.yaml` — SSH Keys

```yaml
keys:
  - id: prod-key
    type: ed25519
    private_key_path: ~/.ssh/prod_key
    encrypted: false
    description: Production deployment key
```

### `layout.yaml` — Tile Positions

Automatically managed. Records which session occupies which grid cell, plus the global font size.

## Authentication Modes

### Trusted Mode (no auth)

Set `auth.mode: none` and `secure_mode: false`. Only use on a network you fully control. The UI displays a warning badge.

### Secure Mode (local auth + HTTPS)

Set `auth.mode: local` and `secure_mode: true`. Place your TLS cert at `~/.config/webmux/config/tls/cert.pem` and key at `tls/key.pem`. Passwords are stored as Argon2id hashes — plaintext is never written to disk.

### Multi-User

Each user gets their own session collection. The first user is created via the bootstrap prompt on first login. Additional accounts can be created via the "+ Account" button in the top bar. Sign out and sign in as a different user to switch session collections.

## Directory Layout

```
~/.config/webmux/               Runtime home (WEBMUX_HOME)
  config/
    app.yaml                    Application settings
    auth.yaml                   Auth mode + user credentials
    hosts.yaml                  Saved SSH host profiles
    keys.yaml                   SSH key references
    layout.yaml                 Tile positions (auto-managed)
    tls/                        TLS cert and key (for secure mode)
  data/
    sessions/                   Persisted session metadata
    events/                     JSONL audit log (one file per day)
  logs/
    webmux.log                  Server log output

webmux/                          Source / install directory (WEBMUX_ROOT)
  config.defaults/               Default config templates (copied on first run)
  backend/                       Node.js / TypeScript backend (Express + ws)
  frontend/                      React / TypeScript frontend (Vite + xterm.js)
  service/                       launchd / systemd service templates
```

## API Reference

### REST Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check |
| `POST` | `/api/auth/bootstrap` | First-run account creation |
| `POST` | `/api/auth/login` | Login (returns JWT) |
| `POST` | `/api/auth/register` | Create additional account (requires auth) |
| `GET` | `/api/auth/status` | Auth mode + bootstrap status |
| `GET` | `/api/sessions` | List sessions (scoped to current user) |
| `POST` | `/api/sessions` | Create session |
| `DELETE` | `/api/sessions/:id` | Delete session |
| `POST` | `/api/sessions/:id/reconnect` | Reconnect a disconnected session |
| `GET` | `/api/hosts` | List saved hosts |
| `POST` | `/api/hosts` | Save a host profile |
| `PUT` | `/api/hosts/:id` | Update host |
| `DELETE` | `/api/hosts/:id` | Delete host |
| `GET` | `/api/keys` | List SSH keys |
| `POST` | `/api/keys` | Add SSH key reference |
| `DELETE` | `/api/keys/:id` | Delete SSH key |
| `GET` | `/api/config` | Get app config |
| `PUT` | `/api/config` | Update app config |
| `GET` | `/api/config/layout` | Get layout |
| `PUT` | `/api/config/layout` | Update layout |

### WebSocket

Connect to `/api/term/:sessionId?token=<jwt>` for terminal I/O.

| Type | Direction | Fields |
|------|-----------|--------|
| `input` | client -> server | `data` |
| `resize` | client -> server | `cols`, `rows` |
| `focus` | client -> server | -- |
| `output` | server -> client | `data` |
| `status` | server -> client | `state`, `message` |
| `viewer_join` | server -> client | `viewer_id`, `viewer_count`, `focus_owner` |
| `viewer_leave` | server -> client | `viewer_id`, `viewer_count`, `focus_owner` |
| `focus` | server -> client | `focus_owner`, `viewer_count` |

## Development

```bash
# Backend in watch mode
npm run dev:backend

# Frontend dev server (proxies /api to backend)
npm run dev:frontend

# Run tests
make test

# Lint
make lint
```

## Security Notes

- Remote **passwords** are held in memory only during session setup (5-minute TTL), then zeroed. They are never written to disk.
- **JWT tokens** expire after 8 hours. Set `JWT_SECRET` to a strong random value in production.
- **Rate limiting**: 300 req/min globally, 10 req/15 min on auth endpoints.
- SSH connections use `StrictHostKeyChecking=accept-new`.
- In secure mode, CORS is restricted to same-origin.

## Deployment

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WEBMUX_HOME` | `~/.config/webmux` | Runtime config, data, and log directory |
| `WEBMUX_ROOT` | source tree `webmux/` | Install directory (frontend assets, default configs) |
| `HTTP_PORT` | from `app.yaml` | Override HTTP port |
| `HTTPS_PORT` | from `app.yaml` | Override HTTPS port |
| `JWT_SECRET` | dev default | **Change in production** |

### Service Management

```bash
make install      # build + install launchd (macOS) or systemd (Linux) user service
make uninstall    # stop + remove the service
make status       # check if running
```

The service auto-starts on login and restarts on crash. On startup, all persistent sessions with key/agent-based auth are automatically reconnected. Password-only sessions require manual reconnect since passwords are not persisted.

On Linux, run `loginctl enable-linger $USER` to start the service at boot without logging in.

## The Totally True and Not At All Embellished History of WebMux

### The continuing adventures of Jordan Hubbard and Sir Reginald von Fluffington III

> *Part 5 of an ongoing chronicle.  [<-- Part 4: Aviation](https://github.com/jordanhubbard/Aviation#the-totally-true-and-not-at-all-embellished-history-of-aviation)*
> *Sir Reginald von Fluffington III appears throughout.  He does not endorse any of it.*

The programmer had, at various points, built a shell extension language, a Scheme interpreter, a programming language from scratch, and eight aviation applications in three languages.  What he had not done, until now, was stare at a wall of terminals and think, "This should be a website."

He was staring at a wall of terminals.

Sir Reginald von Fluffington III was asleep on the keyboard of the one terminal that mattered -- the one currently SSH'd into a production host that was, in the programmer's words, "doing something interesting."  Sir Reginald did not find it interesting.  Sir Reginald found it warm.

"Reggie," the programmer said, in the tone of a man who has just had an idea and has not yet realized that it is large, "I am tired of tmux."

Sir Reginald opened one eye.  He had heard this kind of announcement before -- four times, specifically -- and each time it had resulted in a sustained period of typing, an invocation of the word "elegant," and a new repository.  He closed the eye.

"Not tired of terminal multiplexing," the programmer clarified, because precision mattered to him even when his audience was a cat.  "Tired of terminal multiplexing that requires me to be on the jump box.  What I want is a jump box that runs in a browser.  A persistent, shared terminal wall.  Multiple sessions.  Tiling layout.  Click to focus.  Type to all."  He paused.  "WebSocket-backed xterm.js with node-pty for proper PTY semantics."

Sir Reginald shifted his weight slightly, causing the terminal beneath him to emit a string of characters that, in a different context, might have been interpreted as a command.  In this context, they were interpreted as Sir Reginald's position on the matter.

"It will have two authentication modes," the programmer continued, undeterred.  "Argon2id for password hashing.  JWT tokens.  Rate limiting.  An audit log.  YAML configuration -- human-editable, because I have opinions about TOML that I will not share at this time."  He did not share them.  Sir Reginald noted, in his internal ledger under "small mercies," that the TOML lecture had been deferred.

What emerged was WebMux: a React frontend talking to an Express backend over WebSockets, with xterm.js rendering 256-color terminals in a scrollable 2D grid, and node-pty spawning real PTY processes on the jump box.  SSH sessions with keepalive.  Mosh transport for the adventurous.  Password auth via sshpass for the pragmatic, SSH key auth for the principled.  Split right, split below, reconnect on disconnect.  Five thousand lines of scrollback per terminal, which is approximately four thousand nine hundred more lines than Sir Reginald has ever found useful.

The "Type to All" feature -- broadcast mode, where keystrokes go to every open session simultaneously -- arrived because the programmer had once needed to run the same command on twelve hosts and had done it by switching between twelve tmux panes like a man playing a pipe organ with his forehead.  "Never again," he told Sir Reginald, who was now sitting on the trackpad in a way that kept selecting and deselecting the broadcast toggle.  The orange border that appeared when broadcast mode was active was, the programmer noted, "a visual affordance."  Sir Reginald found it garish.

The multi-viewer presence system -- the part where multiple browser tabs can watch the same session, with click-to-focus determining who has keyboard input -- was described by the programmer as "collaborative."  Sir Reginald, who has never collaborated with anyone on anything, and who considers the concept of shared focus to be a fundamental misunderstanding of how attention works, did not weigh in.  He did, however, sit on the laptop's power cable until it disconnected, which the programmer chose to interpret as unrelated.

The security model was, by the programmer's standards, restrained.  Passwords held in memory for five minutes, then zeroed.  Never written to disk.  Argon2id hashes for stored credentials.  Rate limiting on auth endpoints -- ten requests per fifteen minutes, because the programmer had read enough breach reports to know that the distance between "login form" and "liability" is measured in failed attempts per second.  Trusted mode existed for isolated networks, marked with a warning badge that Sir Reginald would describe, if he described things, as "insufficient."

"The whole thing is portable," the programmer said, gesturing at the `config/` directory.  "YAML files.  Copy the directory.  Run `npm install`.  Start the service.  Lift and shift."  He said "lift and shift" with the satisfaction of a man who has just avoided writing a Dockerfile, and who knows, on some level, that the Dockerfile is coming eventually.

Sir Reginald had, by this point, migrated from the keyboard to the sectional chart that was still on the kitchen table from the Aviation project.  He was lying on the part that showed Class B airspace around SFO, which he considered his territory, and which was now covered in cat hair in a pattern that, if you squinted, resembled a denial-of-service attack on the programmer's ability to plan approaches.

As of this writing, WebMux has been used in production by exactly one person, who also wrote it.  Sir Reginald continues to withhold his endorsement across all five projects, citing "procedural concerns," "insufficient tuna," "a general atmosphere of hubris," "aviation," and, in a new filing delivered by walking across every open terminal session simultaneously in what can only be described as an analog implementation of broadcast mode, "multiplexing."

## License

BSD 2-Clause. See [LICENSE](LICENSE).
