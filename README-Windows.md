# Running WebMux on Windows

WebMux runs natively on modern Windows using ConPTY through `node-pty`. The backend resolves `ssh.exe` from `PATH`, uses `cmd.exe` for local command templates and scratch shells, and stores runtime state under the hosting user's profile by default.

## Prerequisites

1. A current 64-bit or ARM64 Windows release with ConPTY support. Installing the Windows service also requires .NET Framework 4.6.1 or newer, included with supported Windows releases.
2. Node.js 20 or newer.
3. Microsoft OpenSSH Client, with `ssh.exe` available through `PATH`.
4. Git when installing from a source checkout.

Install the common prerequisites from an elevated PowerShell prompt:

```powershell
winget install OpenJS.NodeJS.LTS
winget install Git.Git
Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
```

Open a new PowerShell window and verify the installation:

```powershell
node --version
ssh -V
git --version
```

Published dependencies normally provide prebuilt Windows binaries. If npm must compile `node-pty` or `argon2`, install Visual Studio Build Tools with the **Desktop development with C++** workload and rerun `npm ci`.

## Build and Run

```powershell
git clone https://github.com/jordanhubbard/webmux.git
cd webmux\webmux
npm ci
npm run build
npm start
```

Open `http://localhost:8080`. Runtime configuration and state default to `%USERPROFILE%\.config\webmux`.

Override runtime settings for the current PowerShell session when needed:

```powershell
$env:WEBMUX_HOME = 'D:\WebMuxData'
$env:HTTP_PORT = '8080'
$env:JWT_SECRET = '<strong-random-secret>'
npm start
```

## Install as a Service

From an elevated PowerShell window in the inner `webmux` directory:

```powershell
npm run service:install
npm run service:status
```

The installer prompts for the current Windows account's password, grants that account the **Log on as a service** right, configures automatic delayed startup and failure recovery, and starts WebMux. Running under that account preserves access to its SSH keys and known-hosts data. The password is passed to Windows during registration and is not retained in WebMux's service configuration.

Manage the service with:

```powershell
npm run service:stop
npm run service:start
npm run service:restart
npm run service:uninstall
```

The installer downloads and checksum-verifies the stable [WinSW 2.12.0 service wrapper](https://github.com/winsw/winsw/releases/tag/v2.12.0) under `%ProgramData%\WebMux`. The first service installation therefore requires access to GitHub. Service output is written under `%WEBMUX_HOME%\logs`; uninstalling preserves runtime data and logs.

For an isolated test installation that does not need user SSH credentials, use `npm run service:install -- -LocalSystem`. LocalSystem cannot use the interactive user's SSH keys or known-hosts file and is not recommended for normal hosting.

## Network Access

For access from other machines, allow the selected port through Windows Defender Firewall and keep authentication and TLS appropriate for the network. For example, from elevated PowerShell:

```powershell
New-NetFirewallRule -DisplayName 'WebMux 8080' -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

## Platform Notes

- Key-based OpenSSH sessions are the recommended native Windows configuration.
- Password-based remote SSH requires `sshpass`, which is not normally available on native Windows.
- Mosh requires compatible `mosh` executables and remains optional.
- tmux-backed agent views require tmux and are intended for macOS/Linux hosts.
- RDP support requires a reachable `guacd`; native Windows does not include it.
- The repository-level `make` commands are macOS/Linux conveniences. Use npm and PowerShell service commands on Windows.

Return to the [main README](README.md).
