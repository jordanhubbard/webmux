# Running WebMux on Linux

WebMux runs natively on Linux using Unix pseudoterminals through `node-pty`. The repository Makefile can build the application, run it directly, or install it as a systemd user service.

## Prerequisites

- A supported 64-bit or ARM64 Linux distribution
- Node.js 20 or newer
- Git and OpenSSH Client
- A C/C++ toolchain and Python if npm must compile a native dependency

On Debian or Ubuntu, install the common prerequisites with:

```bash
sudo apt update
sudo apt install git openssh-client build-essential python3
```

Install Node.js 20 or newer using your distribution packages, NodeSource, or another trusted Node.js distribution. Verify it before building:

```bash
node --version
ssh -V
```

Optional packages include `sshpass` for password-based SSH, `mosh` for mosh transport, `tmux` for agent views, and `guacd` for RDP:

```bash
sudo apt install sshpass mosh tmux guacd
```

Package names vary by distribution. SSH keys are preferred over `sshpass`. VNC support does not require a separate local proxy.

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

This installs `~/.config/systemd/user/webmux.service`, enables it, and starts WebMux as the installing user so it can access that user's SSH configuration and keys.

Use the repository Makefile for service management:

```bash
make restart
make stop
make uninstall
```

To start the user service during boot without an interactive login:

```bash
loginctl enable-linger "$USER"
```

Logs are written to `~/.config/webmux/logs/webmux.log` unless `WEBMUX_HOME` is overridden.

## Platform Notes

- `make check-guacd` reports whether the RDP proxy is installed.
- Package and firewall commands differ across distributions; expose only the configured WebMux port.
- When exposing WebMux beyond the local host, use local authentication, set a strong `JWT_SECRET`, configure TLS, and apply appropriate firewall or network controls.

Return to the [main README](README.md).
