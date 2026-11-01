# Running WebMux on macOS

The source Makefile runs WebMux's Go backend on macOS using Unix pseudoterminals. It can build the application, run it directly, or install it as a per-user launchd service.

## Homebrew installation

Homebrew supports both macOS and Linux:

```bash
brew trust --formula jordanhubbard/webmux/webmux
brew tap jordanhubbard/webmux https://github.com/jordanhubbard/webmux
brew install jordanhubbard/webmux/webmux
brew services start jordanhubbard/webmux/webmux
```

The stable formula builds v1.4.1 with built-in WebMux coordination by default.
Agent Deck and Rig are optional integrations and can be enabled together.

Run these commands as your normal user. Open http://localhost:8080, or run
`webmux` for foreground operation. Record formula-specific trust for WebMux
before Homebrew 6 validates its tap. See
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
make install-server
make status
```

This places `~/.local/bin` on the service PATH, installs
`~/Library/LaunchAgents/com.webmux.server.plist`, and configures launchd to
start WebMux when the user logs in. It runs as the installing user so it can
access that user's SSH configuration and keys. Built-in WebMux coordination
is the default. Use `make install-agent-deck` or
`make install-server INSTALL_AGENT_DECK=yes` to install the optional Agent Deck
integration; Rig is installed separately.

If you select Agent Deck, install it on every registered remote that will own
its coordinated sessions. From a WebMux checkout, run `make install-agent-deck`; without a
checkout, use Agent Deck's checksum-verifying installer:

```bash
curl -fsSL https://raw.githubusercontent.com/asheshgoplani/agent-deck/v1.16.34/install.sh | bash -s -- --version v1.16.34 --non-interactive
```

Use the repository Makefile for service management:

```bash
make restart
make stop
make uninstall-server
```

The source installer now installs `~/Applications/WebMux.app` and runs its
embedded server through the LaunchAgent. Configuration remains in
`~/.config/webmux`; rebuilding or replacing the app does not replace your data.

## Mac permissions setup

After installing or upgrading, open WebMux in your browser and sign in as an
administrator. **Mac permissions** opens automatically on first use. If you
skip it, the workspace banner lets you reopen it later. This setup also appears
for existing source, Homebrew, and native installations.

macOS requires you to approve protected-folder access yourself. The wizard
provides the exact installed app or executable path and these two separate steps:

- **Local terminals:** add WebMux under **System Settings → Privacy & Security →
  Full Disk Access**. Full Disk Access covers more than Downloads; grant it when
  your terminal workflows need protected files. A downloaded app should be kept
  at a stable location, such as `~/Applications/WebMux.app`.
- **SSH to this Mac:** under **General → Sharing → Remote Login ⓘ**, enable
  **Allow full disk access for remote users** if your SSH workflows require it.
  This changes access for the users allowed by Remote Login, independently of
  the WebMux app's permission.

The wizard's desktop buttons work when connected locally to WebMux. When using
another computer, follow the same instructions on the server Mac's desktop or
through Screen Sharing. They do not open settings on your browser's computer.

**Check local access** launches a disposable helper from the running service,
not your installation terminal. It attempts to enumerate Desktop, Documents, and
Downloads without returning their filenames. Each folder disappears from the
pending list once access is granted, denied, or the folder is found to be missing.
The dialog closes after all three decisions and remembers that setup is complete,
including denials. SSH setup remains optional through the Mac permissions banner.
The same helper waits up to five minutes for desktop responses; status updates do
not restart it or repeat permission requests. If it times out, retry the check.
No probes run until you request one.

**Check localhost SSH access** uses the current macOS account, localhost port 22,
default SSH identities, and an already trusted host key. It never prompts for a
password, changes host keys, or enables Remote Login. If that connection is not
configured, the wizard gives a manual check to run in your existing SSH terminal.
Other SSH accounts, configured identities, and ports need their own checks.

After changing permissions, use **Restart WebMux and reconnect**, rerun the
checks, and retry in a fresh terminal. The restart detaches clients and preserves
existing tmux shells. Older tmux servers may retain the responsibility context of
the process that originally started them; verify a fresh terminal instead of
killing a shared tmux server. A successful folder probe verifies those folders,
not every location covered by Full Disk Access.

Setup requires an administrator. With authentication disabled, it is available
only through a loopback connection. Skipping is stored in
`~/.config/webmux/config/macos-setup.yaml`. Permission grants remain managed by
macOS; WebMux does not modify TCC databases.

### App signing and release packaging

`make install-server` and macOS native archives assemble `WebMux.app` with bundle ID
`com.webmux.server`, protected-folder usage descriptions, and a verified code
signature. Homebrew runs the packaged app executable on macOS. Source and
Homebrew builds default to a development (ad-hoc) signature: recheck permissions
after rebuilding or upgrading; these builds are not notarized releases.

For a Developer ID signed and notarized app, run packaging on a Mac with an
unlocked signing keychain and a configured `notarytool` keychain profile:

```bash
export WEBMUX_MACOS_SIGN_IDENTITY='Developer ID Application: Your Name (TEAMID)'
export WEBMUX_MACOS_NOTARY_PROFILE='webmux'
make package
```

The packaging helper signs with the hardened runtime, verifies the signature,
submits for notarization, staples the ticket, and checks Gatekeeper acceptance.
Signing or notarization failures fail packaging rather than silently producing
an unsigned substitute. Keep the same Developer ID and bundle identifier across
releases, and qualify permission persistence on upgrades. Do not put signing
credentials in the repository. Managed enterprise Macs can receive the required
privacy policy through MDM.

Logs are written to `~/.config/webmux/logs/webmux.log` unless `WEBMUX_HOME` is overridden.

## Platform Notes

- macOS includes `ssh`, but not `sshpass`, mosh, tmux, or `guacd` by default.
- `make check-guacd` reports whether the RDP proxy is installed.
- To expose WebMux beyond the local host, use local authentication, set a strong `JWT_SECRET`, configure TLS, and apply appropriate host firewall or network controls.

Return to the [main README](README.md).
