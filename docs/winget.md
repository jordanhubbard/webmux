# WinGet publication

[#65](https://github.com/jordanhubbard/webmux/issues/65) tracks upstream
acceptance. The fresh native 1.4.5 submission is
[microsoft/winget-pkgs#482084](https://github.com/microsoft/winget-pkgs/pull/482084).
The old 1.3.10 submission was closed without merging. A generated
manifest, a successful MSI smoke check, or an open submission does not make the
package available from the community source.

After upstream acceptance, the expected commands are:

```powershell
winget show --id WebMux.WebMux --exact --source winget
winget install --id WebMux.WebMux --exact --source winget --scope user
winget upgrade --id WebMux.WebMux --exact --source winget
```

Until `winget show` finds it, use the release MSI instructions in
[README-Windows.md](../README-Windows.md). Installing the per-user MSI does not
register a service; service setup is a separate explicit action.

## Generate a release submission

Use a published stable release and both native MSI assets with their checksum
sidecars. Download the GitHub release API response as well:

```bash
mkdir -p _build/winget/artifacts
gh api repos/jordanhubbard/webmux/releases/tags/v1.4.5 > _build/winget/release.json
gh release download v1.4.5 --pattern '*-windows-*-native.msi*' --dir _build/winget/artifacts
```

On Windows, inspect the downloaded MSI databases without installing or executing
their payloads:

```powershell
powershell.exe -NoProfile -File scripts/inspect-winget-msi.ps1 -ArtifactDirectory _build/winget/artifacts -Version 1.4.5 -Output _build/winget/msi-inspection.json
```

Run the generator with those same downloaded bytes and the inspection JSON:

```bash
node scripts/generate-winget.mts _build/winget/release.json _build/winget/msi-inspection.json _build/winget/artifacts _build/winget/output
```

This writes three YAML files below
`manifests/w/WebMux/WebMux/<version>/`. It refuses an existing version directory.
It binds each architecture's actual MSI product code, product version, publisher,
upgrade family and architecture to its inspected hash, downloaded hash, release
sidecar, GitHub asset digest, filename, size and release URL. A changed package
scope or upgrade family requires an explicit generator review. The native Go MSI
has no Node.js package dependency.

Validate the resulting directory with a current Windows Package Manager:

```powershell
winget validate --manifest _build/winget/output/manifests/w/WebMux/WebMux/1.4.5
```

Also qualify silent install, authenticated installed runtime and uninstall on a
clean disposable Windows machine. Preserve installer logs, exact artifact
identities, security detections and cleanup results. Submit only the three YAML
files through a pull request to `microsoft/winget-pkgs`; keep #65 open until that
PR merges and source discovery succeeds. See Microsoft's
[manifest guidance](https://learn.microsoft.com/en-us/windows/package-manager/package/manifest).

## Published 1.4.5 qualification, 2026-11-17 UTC

Both released MSI databases were read on the Windows 11 x64 worker. Their names
and publisher are `WebMux`, version `1.4.5`, with architecture-specific product
codes and the expected shared upgrade family. SHA-256 values:

| Architecture | MSI SHA-256 |
| --- | --- |
| x64 | `33425bef4816f8aac045af70cc556f0d9ed84e78fd4dd5d04c7fbedea6efa8c5` |
| arm64 | `1ad5ec3cb09f2caa00f870fb2cf2afbf0a1ebc12c1f63ec9df16d9f87ea61d66` |

The x64 MSI silently installed and uninstalled with exit code 0. Its installed
executable (`b31ee5175ddea8d733c58c356ee1d14ff45b8601929478fb8ecf437793d0e13a`)
served the bundled frontend, rejected unauthenticated session access, accepted an
isolated local login, executed a ConPTY command that wrote an independently read
OS file, and deleted the terminal. Uninstall removed the executable and product
registration, restored the user PATH and left no service or install-directory
registry entry. Antivirus and real-time protection remained enabled; neither MSI
nor the installed x64 payload produced a matching Defender detection.

The guest agent runs as SYSTEM, so this exercised an isolated **per-user SYSTEM
installation**, not an interactive user's installation. ARM64 runtime and native
`winget validate` were not executed here (WinGet is unavailable in that agent
context). All three manifests passed Microsoft's 1.28.0 JSON schemas. Upstream
validation and ordinary-user installation remain separate acceptance steps.
These public MSIs are unsigned. This older public release does not qualify the
current installed-service recovery source under #318 or resolve the preview
artifact detection under #384.
