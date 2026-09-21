[CmdletBinding()]
param(
  [string]$OutputDirectory,
  [string]$WixCommand = 'wix',
  [ValidateSet('node', 'go')]
  [string]$Backend = 'go'
)

$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
  throw 'The Windows installer must be built on Windows.'
}
$RuntimeJSON = & node (Join-Path $PSScriptRoot 'packaging-checks.mts') node-runtime
if ($LASTEXITCODE -ne 0) { throw 'Could not inspect the Node.js build runtime.' }
$Runtime = $RuntimeJSON | ConvertFrom-Json
if ($Runtime.major -ne 24) {
  throw 'The Windows installer must be built with Node.js 24.'
}
$NodeArch = $Runtime.arch
$WixArch = switch ($NodeArch) {
  'x64'   { 'x64' }
  'arm64' { 'arm64' }
  default { throw "The published Windows installer supports x64 and arm64 only (got: $NodeArch)." }
}
if (-not (Get-Command $WixCommand -ErrorAction SilentlyContinue)) {
  throw "WiX was not found. Install it with: dotnet tool install --global wix --version 5.0.2"
}

$Repository = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not $OutputDirectory) {
  $OutputDirectory = Join-Path $Repository 'dist'
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$Version = (Get-Content -LiteralPath (Join-Path $Repository 'webmux/backend/package.json') -Raw |
  ConvertFrom-Json).version
$Flavor = if ($Backend -eq 'go') { 'native' } else { 'node24' }
$BundleName = "webmux-$Version-windows-$NodeArch-$Flavor"
$Archive = Join-Path $OutputDirectory "$BundleName.zip"
$InstallerSuffix = if ($Backend -eq 'go') { '-native' } else { '' }
$Installer = Join-Path $OutputDirectory "webmux-$Version-windows-$NodeArch$InstallerSuffix.msi"
$Temporary = Join-Path ([IO.Path]::GetTempPath()) "webmux-msi-$([Guid]::NewGuid())"

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $Temporary | Out-Null

try {
  Push-Location (Join-Path $Repository 'webmux')
  try {
    if ($Backend -eq 'go') { & npm ci --workspace=frontend --include-workspace-root --no-audit --no-fund }
    else { & npm ci --no-audit --no-fund }
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed with exit code $LASTEXITCODE." }
    if ($Backend -eq 'go') { & npm run build --workspace=frontend }
    else { & npm run build:node }
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed with exit code $LASTEXITCODE." }
  } finally {
    Pop-Location
  }

  $Packager = if ($Backend -eq 'go') { 'scripts/package-native.mts' } else { 'scripts/package.mts' }
  & node (Join-Path $Repository $Packager) $OutputDirectory
  if ($LASTEXITCODE -ne 0) { throw "Runtime packaging failed with exit code $LASTEXITCODE." }
  if (-not (Test-Path -LiteralPath $Archive)) { throw "Runtime bundle was not created at $Archive." }

  Expand-Archive -LiteralPath $Archive -DestinationPath $Temporary
  $Stage = Join-Path $Temporary $BundleName
  if (-not (Test-Path -LiteralPath $Stage)) { throw "MSI staging directory was not created at $Stage." }

  & $WixCommand build (Join-Path $Repository 'packaging/windows/webmux.wxs') `
    -arch $WixArch `
    -pdbtype none `
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
