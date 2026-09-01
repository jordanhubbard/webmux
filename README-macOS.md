# Running WebMux on macOS

WebMux runs natively on macOS using Unix pseudoterminals through `node-pty`. The repository Makefile can build the application, run it directly, or install it as a per-user launchd service.

## Prerequisites

- A currently supported macOS release
- Node.js 20 or newer
- Git
- The built-in OpenSSH client
- Xcode Command Line Tools if npm must compile a native dependency

Homebrew is a convenient way to install the required tools:

```bash
xcode-select --install
brew install node git
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
