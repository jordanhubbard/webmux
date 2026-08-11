# WebMux

A web-native terminal multiplexer — think tmux-on-a-jump-box, but it runs in your browser. WebMux gives you a persistent, shared terminal wall: a scrollable 2D grid of live SSH (or mosh) sessions with full terminal emulation, multi-viewer presence, and input broadcast.

## Features

- **2D tiled terminal workspace** — scrollable CSS Grid of fixed-size terminals; click "+" placeholders to add sessions to the right or below any existing tile
- **Configurable terminal size** — default 80×24; adjust columns, rows, and font size from the top bar (persisted to config)
- **Full terminal emulation** — xterm.js with 256-color, clickable links, 5000-line scrollback
- **SSH and mosh transports** — proper PTY via node-pty, with keepalive and auto-reconnect
- **Persistent sessions** — sessions survive browser closes and server reboots; auto-reconnected on startup
- **Saved hosts** — save connection profiles for one-click connect; stored with hostname, port, username, transport, and key
- **Multi-user accounts** — multiple users with separate session collections; Argon2id password hashing
- **Multi-viewer presence** — multiple tabs can watch the same session; click-to-focus controls who has keyboard input
- **Type to All** — broadcast mode sends keystrokes to every open session simultaneously
- **Keyboard terminal cycling** — `Ctrl+Shift+<` and `Ctrl+Shift+>` focus previous/next terminal tiles
- **SSH key and password auth** — managed keys via `keys.yaml`, password-based via `sshpass`
- **Two security modes** — local auth (Argon2id + JWT + HTTPS) or trusted mode for isolated networks
- **OS service integration** — launchd (macOS), systemd (Linux), and Windows Service Control Manager support
- **YAML configuration** — human-editable config in `~/.config/webmux/`, separate from the source tree
- **Audit log** — append-only JSONL event log (logins, session lifecycle)
- **Optional agent views** — disabled-by-default tmux-backed agent session browser with attach and scratch-shell support

## Quick Start

### Prerequisites

- Node.js >= 20
- OpenSSH client (`ssh` on macOS/Linux, `ssh.exe` on `PATH` on Windows)
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

`make install` supports macOS and Linux. See [Hosting on Windows](#hosting-on-windows) for native Windows setup and startup options.

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

## Hosting on Windows

WebMux runs natively on modern Windows using ConPTY through `node-pty`. The backend resolves `ssh.exe` from `PATH`, uses `cmd.exe` for local command templates and scratch shells, and stores runtime state under the hosting user's profile by default.

### Required software

1. **64-bit or ARM64 Windows with ConPTY support.** Keep Windows current; unsupported legacy Windows releases cannot provide the required pseudoterminal API. Installing the Windows service also requires .NET Framework 4.6.1 or newer, which is included with supported Windows releases.
2. **Node.js 20 or newer.** Install the current LTS release from [nodejs.org](https://nodejs.org/) or from an elevated PowerShell prompt:

   ```powershell
   winget install OpenJS.NodeJS.LTS
   ```

3. **Microsoft OpenSSH Client.** Check whether it is already installed:

   ```powershell
   ssh -V
   ```

   If it is missing, install the Windows capability from an elevated PowerShell prompt, then open a new terminal:

   ```powershell
   Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
   ssh -V
   ```

   `ssh.exe` must be visible through `PATH`. WebMux resolves it to an absolute path before starting a ConPTY session.

4. **Git**, when installing from a source checkout. Install it from [git-scm.com](https://git-scm.com/download/win) or with:

   ```powershell
   winget install Git.Git
   ```

The published dependencies normally provide prebuilt Windows binaries. If npm reports that it must compile `node-pty` or `argon2`, install Visual Studio Build Tools with the **Desktop development with C++** workload and rerun `npm ci`.

### Install and run from PowerShell

```powershell
git clone https://github.com/jordanhubbard/webmux.git
cd webmux\webmux
npm ci
npm run build
npm start
```

Open `http://localhost:8080`. The default Windows runtime directory is `%USERPROFILE%\.config\webmux`. Override runtime settings for the current PowerShell session when needed:

```powershell
$env:WEBMUX_HOME = 'D:\WebMuxData'
$env:HTTP_PORT = '8080'
$env:JWT_SECRET = '<strong-random-secret>'
npm start
```

For access from other machines, allow the selected port through Windows Defender Firewall and keep WebMux's authentication/TLS configuration appropriate for the network. For example, from elevated PowerShell:

```powershell
New-NetFirewallRule -DisplayName 'WebMux 8080' -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

### Install as a Windows service

From an elevated PowerShell window in the inner `webmux` directory, install WebMux with Windows Service Control Manager:

```powershell
npm run service:install
npm run service:status
```

The installer prompts for the current Windows account's password, grants that account the **Log on as a service** right, configures automatic delayed startup and failure recovery, and starts WebMux. Using the same account preserves access to that user's SSH keys and known-hosts data. The password is passed to Windows during registration and is not retained in WebMux's service configuration.

Service commands are available for normal administration:

```powershell
npm run service:stop
npm run service:start
npm run service:restart
npm run service:uninstall
```

The installer downloads the stable [WinSW 2.12.0 service wrapper](https://github.com/winsw/winsw/releases/tag/v2.12.0) into `%ProgramData%\WebMux` and verifies its SHA-256 checksum. An internet connection to GitHub is therefore required for the first installation. Service output is written under `%WEBMUX_HOME%\logs`; uninstalling preserves runtime data and logs.

For an isolated test installation that does not need user SSH credentials, `npm run service:install -- -LocalSystem` is also supported. LocalSystem cannot use the interactive user's SSH keys or known-hosts file and is not recommended for normal WebMux hosting.

### Windows feature notes

- Key-based OpenSSH sessions are the recommended native Windows configuration.
- Password-based remote SSH requires `sshpass`, which is not normally available on native Windows. Without it, use SSH keys.
- Mosh requires compatible `mosh` executables and remains optional.
- tmux-backed agent views require tmux and are intended for macOS/Linux hosts. They are disabled by default.
- `make`, `make start`, and `make install` are macOS/Linux administration conveniences. Use the npm and PowerShell service commands above on Windows.

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
```

The `default_term` settings control every terminal tile's dimensions and fixed-width font. Each tile is rendered at a fixed pixel size derived from `cols`, `rows`, and `font_size`. `font_family` accepts a normal comma-separated CSS font-family list and is applied across terminal and monospace UI text. Multi-word family names such as `Custom Mono` are normalized to quoted CSS family names, so `Custom Mono, Monaco, monospace` is returned and saved as `"Custom Mono", Monaco, monospace`. The size values can also be adjusted live from the top bar; changes are saved back to `app.yaml` automatically.

Optional `font_faces` entries let WebMux host local font files for browser clients. `source` paths must be relative paths to `.otf`, `.ttf`, `.woff`, or `.woff2` files; they are resolved relative to the real `app.yaml` path, so symlinked config files can keep fonts beside the shared config. The frontend fetches configured font files through authenticated same-origin routes and registers them before refitting terminals. Once a face is declared, reference its `family` name from `default_term.font_family`.

The optional `terminal_grid` settings cap the number of columns and rows available in the terminals workspace. By default both directions are unlimited. `WEBMUX_TERMINAL_GRID_MAX_COLS` and `WEBMUX_TERMINAL_GRID_MAX_ROWS` override the YAML values at runtime; set either variable to a positive integer, or to `0`/`unlimited` for no limit.

Optional tmux-backed agent views are configured under `app.agents` and are disabled by default. See [Agent Views](docs/agent-views.md) and the sample config at [webmux/examples/agent-views/app.yaml](webmux/examples/agent-views/app.yaml).

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
| `GET` | `/api/agents/config` | Get normalized agent-view config |
| `GET` | `/api/agents/sessions` | List configured agent tmux sessions |
| `GET` | `/api/agents/:agentId/sessions` | List sessions for one configured agent |
| `POST` | `/api/agents/:agentId/attach` | Attach to a validated tmux session |
| `POST` | `/api/agents/:agentId/scratch` | Open or reuse an agent scratch shell |

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

# Run browser tests from the application workspace
cd webmux
npx playwright install chromium
npm run test:e2e

# Lint
cd ..
make lint
```

If Playwright does not publish a bundled Chromium build for the host OS, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` to a compatible installed Chrome or Chromium executable before running `npm run test:e2e`.

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

Once the service is installed, `make start` / `make stop` / `make restart` are **service-manager aware**: they drive `launchctl` (macOS) or `systemctl --user` (Linux) rather than killing the process directly, so a restart is a *deliberate* restart and the service manager does not treat it as a crash. On a host where the service is *not* installed, the same targets fall back to direct pidfile-based process control for development use. On Windows, use the deliberate SCM commands `npm run service:start|stop|restart` (see the Windows hosting section).

On Linux, run `loginctl enable-linger $USER` to start the service at boot without logging in.

## The Totally True and Not At All Embellished History of WebMux

### The continuing adventures of Jordan Hubbard and Sir Reginald von Fluffington III

> *Part 5 of an ongoing chronicle.  [<-- Part 4: Aviation](https://github.com/jordanhubbard/Aviation#the-totally-true-and-not-at-all-embellished-history-of-aviation) | [Part 6: Rocky -->](https://github.com/jordanhubbard/rocky#the-totally-true-and-not-at-all-embellished-history-of-rocky)*
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

What the programmer did next is documented in [Part 6: Rocky](https://github.com/jordanhubbard/rocky#the-totally-true-and-not-at-all-embellished-history-of-rocky) -- specifically, what happens when the programmer deploys a persistent autonomous agent to a remote server in New Jersey and gives it a SOUL.md.

As of this writing, WebMux has been used in production by exactly one person, who also wrote it.  Sir Reginald continues to withhold his endorsement across all six projects, citing "procedural concerns," "insufficient tuna," "a general atmosphere of hubris," "aviation," and, in a new filing delivered by walking across every open terminal session simultaneously in what can only be described as an analog implementation of broadcast mode, "multiplexing."

## License

BSD 2-Clause. See [LICENSE](LICENSE).
