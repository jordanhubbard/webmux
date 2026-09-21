# Running WebMux on macOS

The source Makefile runs WebMux's Go backend on macOS using Unix pseudoterminals. It can build the application, run it directly, or install it as a per-user launchd service. `WEBMUX_BACKEND=node` selects the legacy backend for compatibility testing.

## Homebrew installation

Homebrew supports both macOS and Linux:

```bash
brew trust --formula jordanhubbard/webmux/webmux
brew tap jordanhubbard/webmux https://github.com/jordanhubbard/webmux
brew install jordanhubbard/webmux/webmux
brew services start jordanhubbard/webmux/webmux
```

Run these commands as your normal user. Open http://localhost:8080, or run
`webmux` for foreground operation. The formula-specific trust must be recorded
before Homebrew 6 validates the custom-URL tap. See
[packaging](docs/packaging.md) for service requirements, upgrades, migration,
and release bundles. The instructions below cover building and running from a
source checkout.

## Prerequisites

- A currently supported macOS release
- Go 1.26 or newer
- Node.js 24 LTS or newer for frontend/tooling builds
- Git
- The built-in OpenSSH client
- Xcode Command Line Tools if npm must compile a native dependency

Homebrew is a convenient way to install the required tools:

```bash
xcode-select --install
brew install go node git
```

Optional features require additional software:

```bash
brew install mosh          # mosh transport
brew install hudochenkov/sshpass/sshpass  # password-based SSH
brew install guacamole-server             # RDP proxy
brew install tmux                          # agent views
```

SSH keys are preferred over `sshpass`. VNC support does not require a separate local proxy.

## Build and Run

```bash
git clone https://github.com/jordanhubbard/webmux.git
cd webmux
make
make start
```

Open `http://localhost:8080`. Runtime configuration and state default to `~/.config/webmux/`.

## Install as a Service

```bash
make install
make status
```

This installs `~/Library/LaunchAgents/com.webmux.server.plist`, starts WebMux, and configures launchd to start it when the user logs in. It runs as the installing user so it can access that user's SSH configuration and keys.

Use the repository Makefile for service management:

```bash
make restart
make stop
make uninstall
```

Logs are written to `~/.config/webmux/logs/webmux.log` unless `WEBMUX_HOME` is overridden.

## Platform Notes

- macOS includes `ssh`, but not `sshpass`, mosh, tmux, or `guacd` by default.
- `make check-guacd` reports whether the RDP proxy is installed.
- To expose WebMux beyond the local host, use local authentication, set a strong `JWT_SECRET`, configure TLS, and apply appropriate host firewall or network controls.

Return to the [main README](README.md).
