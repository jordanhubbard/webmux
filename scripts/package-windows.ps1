[CmdletBinding()]
param(
  [string]$OutputDirectory,
  [string]$WixCommand = 'wix'
)

$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
  throw 'The Windows installer must be built on Windows.'
}
if ((node -p "process.versions.node.split('.')[0]") -ne '24') {
  throw 'The Windows installer must be built with Node.js 24.'
}
if ((node -p 'process.arch') -ne 'x64') {
  throw 'The published Windows installer currently supports x64 only.'
}
if (-not (Get-Command $WixCommand -ErrorAction SilentlyContinue)) {
  throw "WiX was not found. Install it with: dotnet tool install --global wix --version 7.0.0"
}

$Repository = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not $OutputDirectory) {
  $OutputDirectory = Join-Path $Repository 'dist'
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$Version = (Get-Content -LiteralPath (Join-Path $Repository 'webmux/backend/package.json') -Raw |
  ConvertFrom-Json).version
$BundleName = "webmux-$Version-windows-x64-node24"
$Archive = Join-Path $OutputDirectory "$BundleName.zip"
$Installer = Join-Path $OutputDirectory "webmux-$Version-windows-x64.msi"
$Temporary = Join-Path ([IO.Path]::GetTempPath()) "webmux-msi-$([Guid]::NewGuid())"

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $Temporary | Out-Null

try {
  Push-Location (Join-Path $Repository 'webmux')
  try {
    & npm ci --no-audit --no-fund
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed with exit code $LASTEXITCODE." }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed with exit code $LASTEXITCODE." }
  } finally {
    Pop-Location
  }

  & node (Join-Path $Repository 'scripts/package.cjs') $OutputDirectory
  if ($LASTEXITCODE -ne 0) { throw "Runtime packaging failed with exit code $LASTEXITCODE." }
  if (-not (Test-Path -LiteralPath $Archive)) { throw "Runtime bundle was not created at $Archive." }

  Expand-Archive -LiteralPath $Archive -DestinationPath $Temporary
  $Stage = Join-Path $Temporary $BundleName
  if (-not (Test-Path -LiteralPath $Stage)) { throw "MSI staging directory was not created at $Stage." }

  & $WixCommand build (Join-Path $Repository 'packaging/windows/webmux.wxs') `
    -arch x64 `
    -d "Version=$Version" `
    -bindpath "Stage=$Stage" `
    -out $Installer
  if ($LASTEXITCODE -ne 0) { throw "WiX failed with exit code $LASTEXITCODE." }
  if (-not (Test-Path -LiteralPath $Installer)) { throw "MSI was not created at $Installer." }

  $InstallerHash = (Get-FileHash -LiteralPath $Installer -Algorithm SHA256).Hash.ToLowerInvariant()
  "$InstallerHash  $([IO.Path]::GetFileName($Installer))" |
    Set-Content -LiteralPath "$Installer.sha256" -Encoding ascii

  Write-Host $Archive
  Write-Host $Installer
} finally {
  Remove-Item -LiteralPath $Temporary -Recurse -Force -ErrorAction SilentlyContinue
}
