# Running WebMux on Windows

WebMux runs natively on modern Windows using ConPTY through `node-pty`. The backend resolves `ssh.exe` from `PATH`, uses `cmd.exe` for local command templates and scratch shells, and stores runtime state under the hosting user's profile by default.

## Prerequisites

1. A current Windows release with ConPTY support, on x64 or ARM64. Published MSIs are available for both architectures. Installing the Windows service also requires .NET Framework 4.6.1 or newer, included with supported Windows releases.
2. Node.js 24 LTS or newer.
3. Microsoft OpenSSH Client, with `ssh.exe` available through `PATH`.
4. Git only when installing from a source checkout.

Install the common prerequisites from an elevated PowerShell prompt:

```powershell
winget install OpenJS.NodeJS.LTS
Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
```

Open a new PowerShell window and verify the installation:

```powershell
node --version
ssh -V
```

## Install the MSI

Download `webmux-<version>-windows-x64.msi` (or `webmux-<version>-windows-arm64.msi`
on ARM64 Windows) and its `.sha256` file from the
[latest release](https://github.com/jordanhubbard/webmux/releases/latest). Verify
the download from PowerShell, substituting the downloaded filenames:

```powershell
$expected = (Get-Content .\webmux-<version>-windows-x64.msi.sha256).Split()[0]
$actual = (Get-FileHash .\webmux-<version>-windows-x64.msi -Algorithm SHA256).Hash
if ($actual -ne $expected) { throw 'WebMux MSI checksum mismatch' }
```

Double-click the MSI or install it from PowerShell:

```powershell
msiexec.exe /i .\webmux-<version>-windows-x64.msi
```

The per-user installer does not require elevation. It installs under
`%LOCALAPPDATA%\Programs\WebMux`, adds the `webmux` and `webmux-service`
launchers to the user `PATH`, and creates an Add/Remove Programs entry. Open a
new PowerShell window, run `webmux`, and browse to `http://localhost:8080`.
Upgrades replace the application in place; uninstalling preserves configuration
and state under `%USERPROFILE%\.config\webmux`.

The MSI is not yet code-signed, so Windows identifies the publisher as unknown.
Verify the SHA-256 file before accepting that warning.

## Build and Run from Source

Install Git, then clone and build:

```powershell
winget install Git.Git
git clone https://github.com/jordanhubbard/webmux.git
cd webmux\webmux
npm ci
npm run build
npm start
```

Published dependencies normally provide prebuilt Windows binaries. If npm must
compile `node-pty` or `argon2`, install Visual Studio Build Tools with the
**Desktop development with C++** workload and rerun `npm ci`.

Open `http://localhost:8080`. Runtime configuration and state default to `%USERPROFILE%\.config\webmux`.

Override runtime settings for the current PowerShell session when needed:

```powershell
$env:WEBMUX_HOME = 'D:\WebMuxData'
$env:HTTP_PORT = '8080'
$env:JWT_SECRET = '<strong-random-secret>'
npm start
```

## Install as a Service

From an elevated PowerShell window, use the MSI-installed launcher:

```powershell
webmux-service install
webmux-service status
```

From a source checkout in the inner `webmux` directory, use the npm scripts:

```powershell
npm run service:install
npm run service:status
```

The installer prompts for the current Windows account's password, grants that account the **Log on as a service** right, configures automatic delayed startup and failure recovery, and starts WebMux. Running under that account preserves access to its SSH keys and known-hosts data. The password is passed to Windows during registration and is not retained in WebMux's service configuration.

Manage the service with:

```powershell
webmux-service stop
webmux-service start
webmux-service restart
webmux-service uninstall
```

Use the corresponding `npm run service:<action>` command for a source checkout.

For a native migration preview, stop the existing service before replacing the
MSI, then point its existing registration at the new runtime:

```powershell
webmux-service stop
# Install the native MSI here.
webmux-service reconfigure -Backend go
webmux-service start
```

`reconfigure` preserves the registered Windows account, `WEBMUX_HOME`, other
environment variables, and log/recovery settings. It updates the executable,
arguments, working directory and `WEBMUX_ROOT` without asking for the account
password again. A stopped service stays stopped; a running service is stopped
and restarted. The selected runtime must already be installed. This command
does not install an MSI or change the service account or writable home.

The installer downloads and checksum-verifies the stable [WinSW 2.12.0 service wrapper](https://github.com/winsw/winsw/releases/tag/v2.12.0) under `%ProgramData%\WebMux`. The first service installation therefore requires access to GitHub. Service output is written under `%WEBMUX_HOME%\logs`; uninstalling preserves runtime data and logs.

For an isolated test installation that does not need user SSH credentials, use
`webmux-service install -LocalSystem` (or
`npm run service:install -- -LocalSystem` from source). LocalSystem cannot use
the interactive user's SSH keys or known-hosts file and is not recommended for
normal hosting.

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
