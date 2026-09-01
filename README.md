# WebMux

A browser-based remote workspace for persistent terminal and desktop sessions. WebMux started as tmux-on-a-jump-box: a shared, scrollable wall of SSH and mosh terminals. It now also brings VNC, RDP, and optional tmux-backed coding-agent sessions into the same web interface.

## Features

- **2D tiled terminal workspace** — scrollable CSS Grid of fixed-size terminals; click "+" placeholders to add sessions to the right or below any existing tile
- **Configurable terminal size** — default 80×24; adjust columns, rows, and font size from the top bar (persisted to config)
- **Full terminal emulation** — xterm.js with 256-color, clickable links, 5000-line scrollback
- **SSH and mosh transports** — proper PTY via node-pty, with keepalive and auto-reconnect
- **Persistent sessions** — sessions survive browser closes and server reboots; auto-reconnected on startup
- **Remote desktop workspace** — arrange VNC and RDP sessions in a second tiled workspace, with fullscreen viewing and reconnect controls
- **Saved hosts** — save connection profiles for one-click connect; stored with hostname, port, username, transport, and key
- **Multi-user accounts** — multiple users with separate session collections; Argon2id password hashing
- **Multi-viewer presence** — multiple tabs can watch the same session; click-to-focus controls who has keyboard input
- **Type to All** — broadcast mode sends keystrokes to every open session simultaneously
- **Keyboard terminal cycling** — `Ctrl+Shift+<` and `Ctrl+Shift+>` focus previous/next terminal tiles
- **Workspace navigation** — switch among terminals, desktops, and configured agent views; use the minimap and minimized-session dock to navigate larger layouts
- **Per-window controls** — rename, move, minimize, lock, theme, and control auto-scroll for individual terminal sessions or globally
- **SSH key and password auth** — managed keys via `keys.yaml`, password-based via `sshpass`
- **Two security modes** — local auth (Argon2id + JWT + HTTPS) or trusted mode for isolated networks
- **OS service integration** — launchd (macOS), systemd (Linux), and Windows Service Control Manager support
- **YAML configuration** — human-editable config in `~/.config/webmux/`, separate from the source tree
- **Audit log** — append-only JSONL event log (logins, session lifecycle)
- **Optional agent views** — disabled-by-default tmux-backed agent session browser with attach and scratch-shell support
- **Session expiry handling** — visible JWT countdown and refresh controls prevent expired browser sessions from entering reconnect loops

## Quick Start

### Prerequisites

- Node.js >= 20
- OpenSSH client (`ssh` on macOS/Linux, `ssh.exe` on `PATH` on Windows)
- (Optional) `sshpass` for password-based SSH auth
- (Optional) `mosh` on both ends for mosh transport
- (Optional) a VNC target for VNC sessions
- (Optional) Apache Guacamole's `guacd` for RDP sessions

### Supported Platforms

WebMux runs natively on all three major desktop/server platforms. Each platform guide covers prerequisites, native terminal support, service installation, and platform-specific limitations:

- **macOS**: [README-macOS.md](README-macOS.md) (Unix PTYs and launchd)
- **Linux**: [README-Linux.md](README-Linux.md) (Unix PTYs and systemd user services)
- **Windows**: [README-Windows.md](README-Windows.md) (ConPTY and Windows Service Control Manager)

### Build and Run

```bash
make            # install deps + build
make start      # start in background
```

Open `http://localhost:8080`. On first run with local auth, you'll be prompted to create the first administrator account.

Runtime configuration and state are created under `~/.config/webmux/` by default; the source checkout remains disposable.

### Install as a Service

```bash
make install    # installs launchd (macOS) or systemd (Linux) service
```

WebMux will start automatically on login and auto-reconnect any persistent sessions after a reboot.

```bash
make uninstall  # remove the OS service
```

`make install` supports macOS and Linux. Windows uses the npm service commands documented in [README-Windows.md](README-Windows.md).

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
| `make test` | Run all tests (unit + e2e) |
| `make test-unit` | Typecheck + unit tests (no browser needed) |
| `make test-e2e` | E2E tests (auto-provisions a Chromium, falls back to system Chrome) |
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
    cols: 80            # terminal width in characters
    rows: 24            # terminal height in characters
    font_size: 14       # font size in pixels
    font_family: ui-monospace, "SFMono-Regular", Monaco, Menlo, Consolas, "Liberation Mono", "DejaVu Sans Mono", monospace
  font_faces:
    # Optional local fonts to make available to browser clients.
    # source paths are relative to this app.yaml file's real path.
    # - family: Custom Mono
    #   source: fonts/CustomMono.woff2
    #   weight: 400
    #   style: normal
  terminal_grid:
    max_cols: null      # null, 0, or omitted = unlimited
    max_rows: null      # null, 0, or omitted = unlimited
  transport:
    prefer_mosh: false
    ssh_fallback: true
  guacd:
    host: 127.0.0.1
    port: 4822
```

The `default_term` settings control every terminal tile's dimensions and fixed-width font. Each tile is rendered at a fixed pixel size derived from `cols`, `rows`, and `font_size`. `font_family` accepts a normal comma-separated CSS font-family list and is applied across terminal and monospace UI text. Multi-word family names such as `Custom Mono` are normalized to quoted CSS family names, so `Custom Mono, Monaco, monospace` is returned and saved as `"Custom Mono", Monaco, monospace`. The size values can also be adjusted live from the top bar; changes are saved back to `app.yaml` automatically.

Optional `font_faces` entries let WebMux host local font files for browser clients. `source` paths must be relative paths to `.otf`, `.ttf`, `.woff`, or `.woff2` files; they are resolved relative to the real `app.yaml` path, so symlinked config files can keep fonts beside the shared config. The frontend fetches configured font files through authenticated same-origin routes and registers them before refitting terminals. Once a face is declared, reference its `family` name from `default_term.font_family`.

The optional `terminal_grid` settings cap the number of columns and rows available in the terminals workspace. By default both directions are unlimited. `WEBMUX_TERMINAL_GRID_MAX_COLS` and `WEBMUX_TERMINAL_GRID_MAX_ROWS` override the YAML values at runtime; set either variable to a positive integer, or to `0`/`unlimited` for no limit.

Optional tmux-backed agent views are configured under `app.agents` and are disabled by default. See [Agent Views](docs/agent-views.md) and the sample config at [webmux/examples/agent-views/app.yaml](webmux/examples/agent-views/app.yaml).

RDP sessions are proxied through Apache Guacamole's `guacd`; use `make check-guacd` to check the local installation. VNC sessions connect through WebMux's authenticated WebSocket proxy. Enable either protocol per saved host with its configured port.

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
    vnc_enabled: false
    vnc_port: 5900
    rdp_enabled: false
    rdp_port: 3389
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

Each user gets their own session collection. The first user is created via the bootstrap prompt on first login and becomes an administrator. Administrators can manage additional accounts from the top bar; sign out and sign in as another user to switch session collections.

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
| `POST` | `/api/auth/refresh` | Refresh an authenticated JWT |
| `POST` | `/api/auth/ticket` | Create a short-lived WebSocket ticket |
| `POST` | `/api/auth/register` | Create additional account (requires auth) |
| `GET` | `/api/auth/status` | Auth mode + bootstrap status |
| `GET` | `/api/auth/me` | Get the current account |
| `GET` | `/api/auth/users` | List accounts (administrator only) |
| `DELETE` | `/api/auth/users/:username` | Delete an account (administrator only) |
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
| `GET` | `/api/agents/config` | Get normalized agent-view config |
| `GET` | `/api/agents/sessions` | List configured agent tmux sessions |
| `GET` | `/api/agents/:agentId/sessions` | List sessions for one configured agent |
| `POST` | `/api/agents/:agentId/attach` | Attach to a validated tmux session |
| `POST` | `/api/agents/:agentId/scratch` | Open or reuse an agent scratch shell |
| `GET` | `/api/vnc/sessions` | List VNC sessions |
| `POST` | `/api/vnc/sessions` | Create a VNC session |
| `GET` | `/api/rdp/sessions` | List RDP sessions |
| `POST` | `/api/rdp/sessions` | Create an RDP session |

### WebSocket

The browser obtains a short-lived WebSocket ticket and connects to `/api/term/:sessionId?ticket=<ticket>` for terminal I/O. Token query parameters remain a compatibility fallback. VNC and RDP use `/api/vnc/ws/:sessionId` and `/api/rdp/ws/:sessionId` respectively.

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

# Run browser tests from the application workspace
cd webmux
npx playwright install chromium
npm run test:e2e

# Lint
cd ..
make lint
```

If Playwright does not publish a bundled Chromium build for the host OS, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` to a compatible installed Chrome or Chromium executable before running `npm run test:e2e`.

## Contributing

Public bug reports, feature requests, and proposed changes belong in [GitHub Issues](https://github.com/jordanhubbard/webmux/issues) and pull requests. The project does not use beads. Maintainers use [mac](https://github.com/jordanhubbard/mac) for internal task tracking and work dispatch; contributors do not need mac to file an issue or submit a pull request.

Before opening a pull request, run `make test` and `make lint`; describe the user-visible change and link the issue it addresses.

Thanks to the people who have contributed code and reviews as WebMux grew beyond its original one-person experiment, including [Isaiah Weiner](https://github.com/zoratu), [Shawn Edwards](https://github.com/lesserevil), and [Trent Nelson](https://github.com/tpn). GitHub's [contributors page](https://github.com/jordanhubbard/webmux/graphs/contributors) is the canonical, evolving record.

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
| `WEBMUX_HOME` | `~/.config/webmux` | Runtime config, data, and log directory (`%USERPROFILE%\.config\webmux` on Windows) |
| `WEBMUX_ROOT` | source tree `webmux/` | Install directory (frontend assets, default configs) |
| `HTTP_PORT` | from `app.yaml` | Override HTTP port |
| `HTTPS_PORT` | from `app.yaml` | Override HTTPS port |
| `JWT_SECRET` | dev default | **Change in production** |

### Service Management

```bash
make install      # build + install launchd (macOS) or systemd (Linux) user service
make uninstall    # stop + remove the service
make status       # check if running
make restart      # rebuild + deliberate restart via the service manager
```

The service auto-starts on login and restarts on crash. On startup, all persistent sessions with key/agent-based auth are automatically reconnected. Password-only sessions require manual reconnect since passwords are not persisted.

Once the service is installed, `make start` / `make stop` / `make restart` are **service-manager aware**: they drive `launchctl` (macOS) or `systemctl --user` (Linux) rather than killing the process directly, so a restart is a *deliberate* restart and the service manager does not treat it as a crash. On a host where the service is *not* installed, the same targets fall back to direct pidfile-based process control for development use. On Windows, use the deliberate SCM commands `npm run service:start|stop|restart` described in [README-Windows.md](README-Windows.md).

On Linux, run `loginctl enable-linger $USER` to start the service at boot without logging in.

## The Totally True and Not At All Embellished History of WebMux

### The continuing adventures of Jordan Hubbard and Sir Reginald von Fluffington III

> *Part 5 of an ongoing chronicle.  [<-- Part 4: Aviation](https://github.com/jordanhubbard/Aviation#the-totally-true-and-not-at-all-embellished-history-of-aviation) | [The story continues in mac -->](https://github.com/jordanhubbard/mac)*
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

What the programmer did next eventually became [mac](https://github.com/jordanhubbard/mac), a multi-agent coordinator control plane. The route from one wall of terminals to a fleet of durable agents was neither direct nor sensible, but it is documented there for anyone determined to follow it.

WebMux did not remain a one-person production experiment. Other humans now contribute code, reviews, bug reports, and operating experience. Sir Reginald continues to withhold his endorsement, citing "procedural concerns," "insufficient tuna," "a general atmosphere of hubris," "aviation," and, in a filing delivered by walking across every open terminal session simultaneously in what can only be described as an analog implementation of broadcast mode, "multiplexing."

## License

BSD 2-Clause. See [LICENSE](LICENSE).
