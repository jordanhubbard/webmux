# Running WebMux on Linux

The source Makefile runs WebMux's Go backend on Linux using Unix pseudoterminals. It can build the application, run it directly, or install it as a systemd user service.

## Homebrew installation

Homebrew supports both macOS and Linux:

```bash
brew trust --formula asheshgoplani/tap/agent-deck
brew tap asheshgoplani/tap
brew trust --formula jordanhubbard/webmux/webmux
brew tap jordanhubbard/webmux https://github.com/jordanhubbard/webmux
brew install jordanhubbard/webmux/webmux
brew services start jordanhubbard/webmux/webmux
```

The formula installs Agent Deck and tmux for WebMux's default coordination
experience.

Run these commands as your normal user. Open http://localhost:8080, or run
`webmux` for foreground operation. Record formula-specific trust for Agent Deck
and WebMux before Homebrew 6 validates their taps. See
[packaging](docs/packaging.md) for service requirements, upgrades, migration,
and release bundles. The instructions below cover building and running from a
source checkout.

## Prerequisites

- A supported 64-bit or ARM64 Linux distribution
- Make, tar, curl or wget, and sha256sum
- Go 1.27.4 or newer and Node.js 24.x (automatically provisioned by `make`
  on x64/ARM64 when missing or incompatible)
- Git and OpenSSH Client
- A C/C++ toolchain and Python if npm must compile a native dependency

On Debian or Ubuntu, install the common prerequisites with:

```bash
sudo apt update
sudo apt-get install -y git openssh-client build-essential python3 curl ca-certificates
```

Run `make` to download checksum-verified Go and Node toolchains into
`.build/toolchains/`, without sudo or shell profile changes. Compatible installed
tools are reused. Go caches also default to `.build/`; explicit `GOMODCACHE` and
`GOCACHE` settings override these defaults. Direct npm/Go commands outside Make
need compatible tools on your shell's PATH. To use tools installed separately,
verify them before building:

```bash
node --version
go version
ssh -V
```

Install `tmux` for built-in local terminals and agent views. Source server
installation, start, and restart commands check for it on the server PATH and
stop with installation guidance if it is missing:

```bash
sudo apt-get install -y tmux
```

Optional packages include `sshpass` for password-based SSH, `mosh` for mosh transport, and `guacd` for RDP:

```bash
sudo apt install sshpass mosh guacd
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
make install-server
make status
```

This installs the qualified Agent Deck release in `~/.local/bin`, places that
directory on the service PATH, installs `~/.config/systemd/user/webmux.service`,
and starts WebMux as the installing user so it can access that user's SSH
configuration and keys. Use `INSTALL_AGENT_DECK=no make install-server` only
when configuring Rig instead or disabling coordination.

Install Agent Deck on every registered remote that will own coordinated
sessions. From a WebMux checkout, run `make install-agent-deck`; without a
checkout, use Agent Deck's checksum-verifying installer:

```bash
curl -fsSL https://raw.githubusercontent.com/asheshgoplani/agent-deck/v1.16.39/install.sh | bash -s -- --version v1.16.39 --non-interactive
```

Use the repository Makefile for service management:

```bash
make restart
make stop
make uninstall-server
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

## Headless browser qualification and the desktop keyring

The browser companion retains Chromium's normal encrypted cookie storage. Its
Linux Secret Service login collection must be unlocked when Chrome needs it.
An SSH shell with a locked desktop keyring can stall the first network
navigation before an HTTP connection is opened.

For native tests on a headless Linux worker, install `dbus-user-session` and
`gnome-keyring`, then run an isolated encrypted fixture:

```bash
python3 scripts/with-test-keyring.py make test
```

The wrapper uses a private D-Bus session, private data/control directories and a
random in-memory password. It verifies that the disposable login collection is
unlocked before invoking the command, then stops its own keyring daemon. The
operator's desktop collection and browser security flags remain unchanged.
Production services use the operator's intended desktop Secret Service session.
