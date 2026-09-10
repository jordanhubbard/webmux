# Installing WebMux with Homebrew

The same formula supports macOS and Linux. Install [Homebrew](https://brew.sh/), then:

```bash
brew trust --formula jordanhubbard/webmux/webmux
brew tap jordanhubbard/webmux https://github.com/jordanhubbard/webmux
brew install jordanhubbard/webmux/webmux
brew services start jordanhubbard/webmux/webmux
```

Open http://localhost:8080 and create the first administrator account. To run in the
foreground instead, use `webmux`. Linux background services require systemd and an
available user service manager; foreground operation does not.

The formula builds the tagged release using the npm lockfile. It installs Node.js
24 and OpenSSH, compiles the UI and backend, and retains only backend production
dependencies. The first installation requires build tools (Xcode Command Line
Tools on macOS, or a C/C++ toolchain on Linux); Python is supplied by Homebrew.
There are no prebuilt Homebrew bottles yet.

The repository itself is the tap; no separate `homebrew-webmux` repository is
required. The explicit URL in `brew tap` is necessary. Homebrew 6 validates tap
contents immediately after cloning, so record formula-specific trust before the
tap command. Trusting only `jordanhubbard/webmux/webmux` is narrower than
trusting every current and future formula in the tap.

## Services, configuration, and upgrades

Run `brew services` as your normal user. It uses launchd on macOS and a systemd
user service on Linux, preserving access to that user's SSH keys and known hosts.
On Linux, `loginctl enable-linger "$USER"` allows the user service to start without
an interactive login, if permitted by your system administrator.

The package lives in Homebrew's Cellar. Runtime files stay under
`~/.config/webmux/`: settings in `config/`, persistent state in `data/`, and
application logs in `logs/`. Service stdout/stderr go to
`$(brew --prefix)/var/log/webmux.log`.

```bash
brew services info jordanhubbard/webmux/webmux
brew services stop jordanhubbard/webmux/webmux
brew update
brew upgrade jordanhubbard/webmux/webmux
brew services start jordanhubbard/webmux/webmux
```

Stop the service before upgrading to avoid changing its files while it runs.
Upgrades and `brew uninstall webmux` preserve runtime files. To remove the package:

```bash
brew services stop jordanhubbard/webmux/webmux
brew uninstall jordanhubbard/webmux/webmux
```

For foreground use with a different state directory:

```bash
WEBMUX_HOME="$HOME/my-webmux" webmux
```

A service does not inherit environment variables exported in your interactive
shell. For a custom service environment, use Homebrew's supported service
configuration or maintain a custom launchd/systemd unit pointing to
`$(brew --prefix)/opt/webmux/bin/webmux`. Preserve any existing `WEBMUX_HOME`
override when migrating.

## Migrating from a source checkout

From the old checkout, run `make uninstall` if you installed its service, or
`make stop` if you started it manually. Then install and start the Homebrew package.
The default state directory is shared, so existing users, hosts, keys, and layouts
remain available. Do not run both installations against the same state directory.
Use `brew services` to control the packaged installation.

## Optional features

```bash
brew install mosh tmux guacamole-server
```

These enable mosh transport, tmux-backed agent views, and the `guacd` dependency
for RDP respectively; configure and start `guacd` separately. Password-based SSH
needs `sshpass`; see the platform guide for installation instructions. The WebMux
launcher includes Homebrew's bin and sbin directories in PATH so service-launched
sessions can find these tools.

## Runtime release bundles and Windows installer

Releases also build `.tar.gz` runtime bundles for macOS ARM64 and Linux x86-64,
plus a Windows x64 runtime `.zip` and per-user `.msi`, with a `.sha256` file for
each artifact. They contain the compiled application, default configuration,
license, and native production dependencies. Node.js 24 and OpenSSH must be
installed separately; these are not standalone executables.
`bundle.json` records the build platform, CPU, Node ABI, and Linux glibc version.
Linux bundles target glibc systems at least as new as the build environment;
use Homebrew/source builds for other architectures or libc implementations.

Verify the checksum with `shasum -a 256 -c <archive>.sha256` on macOS or
`sha256sum -c <archive>.sha256` on Linux, extract the archive, and run its
`bin/webmux`. Keep the extracted directory intact. Bundles use the same runtime
state directory as the Homebrew package; stop the previous instance before switching.

On Windows, verify a checksum with `Get-FileHash`, then install the MSI by
double-clicking it or running `msiexec.exe /i <installer>.msi`. It installs under
`%LOCALAPPDATA%\Programs\WebMux` and adds `webmux` and `webmux-service` to the
user `PATH`. The MSI is currently unsigned, so verify its checksum before
accepting the unknown-publisher warning. Windows ARM64 remains buildable from
source; release installers are x64 until an ARM64 CI runner can build and test
the native dependencies.

To build a bundle locally using Node.js 24:

```bash
make package
```

Output goes to `dist/`. The build uses `npm ci`; production dependencies are
installed in an isolated staging directory to avoid including developer state.
To test an extracted bundle:

```bash
node scripts/smoke-package.cjs /absolute/path/to/extracted/webmux-version-platform-arch-node24
```

On Windows, install WiX 7 and build both the ZIP and MSI from PowerShell:

```powershell
dotnet tool install --global wix --version 7.0.0
.\scripts\package-windows.ps1
```

## Maintaining releases

The packaging workflow tests macOS, Linux, and Windows on pull requests. It
builds and tests extracted runtime bundles, installs and tests the Homebrew
formula against the PR revision, and installs, tests, and removes the Windows
MSI. Published releases receive bundles, the MSI, and checksums only after all
platforms pass. Release publication must trigger GitHub Actions (a release
created using another workflow's default `GITHUB_TOKEN` does not trigger new
workflows).

After publishing a reviewed release, update the formula on a branch:

```bash
node scripts/update-homebrew.cjs vX.Y.Z
```

This downloads the tagged source archive and updates its URL and SHA-256. Commit
the formula update through a pull request; after merging, users receive it through
`brew update` and `brew upgrade`. Do not point the stable formula at an unpublished
tag. Runtime bundles are published starting with v1.3.9; the Windows MSI starts
with v1.3.10.
