[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)][string]$Installer,
  [string]$PreviousInstaller
)
$ErrorActionPreference = 'Stop'
$installerPath = (Resolve-Path -LiteralPath $Installer).Path
$previousPath = if ($PreviousInstaller) { (Resolve-Path -LiteralPath $PreviousInstaller).Path } else { $null }
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\WebMux'
if ((Test-Path -LiteralPath $installRoot) -or
    (Get-Service -Name WebMux -ErrorAction SilentlyContinue) -or
    (Test-Path -LiteralPath (Join-Path $env:ProgramData 'WebMux'))) {
  throw 'MSI smoke requires an isolated runner without an existing WebMux installation or service.'
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) "webmux-native-msi-smoke-$([Guid]::NewGuid())"
[IO.Directory]::CreateDirectory($temporary) | Out-Null
function Get-WebMuxProducts {
  foreach ($key in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall', 'HKCU:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')) {
    Get-ChildItem -LiteralPath $key -ErrorAction SilentlyContinue | ForEach-Object {
      $entry = Get-ItemProperty -LiteralPath $_.PSPath
      if ($entry.DisplayName -eq 'WebMux') { $entry.PSChildName }
    }
  }
}
if (@(Get-WebMuxProducts).Count -ne 0) { throw 'MSI smoke requires no existing per-user WebMux product registration.' }
$activeInstaller = $installerPath
function Invoke-Installer([string]$Operation, [string]$Log, [string]$Package = $installerPath) {
  $arguments = @($Operation, ('"' + $Package + '"'), '/qn', '/norestart', '/l*v', ('"' + $Log + '"'))
  $process = Start-Process msiexec.exe -ArgumentList $arguments -Wait -PassThru
  if ($process.ExitCode -ne 0) {
    if (Test-Path -LiteralPath $Log) { Get-Content -LiteralPath $Log }
    throw "MSI $Operation failed with exit code $($process.ExitCode)."
  }
}
try {
  $previousProduct = $null
  if ($previousPath) {
    $activeInstaller = $previousPath
    Invoke-Installer '/i' (Join-Path $temporary 'previous-install.log') $previousPath
    if (-not (Test-Path -LiteralPath (Join-Path $installRoot 'backend/dist/index.js'))) { throw 'Previous MSI is not the Node runtime.' }
    $products = @(Get-WebMuxProducts)
    if ($products.Count -ne 1) { throw 'Expected one legacy product registration.' }
    $previousProduct = $products[0]
    & node (Join-Path $PSScriptRoot 'smoke-package.mts') $installRoot
    if ($LASTEXITCODE -ne 0) { throw 'Installed legacy runtime failed smoke checks.' }
  }
  Invoke-Installer '/i' (Join-Path $temporary 'install.log')
  $activeInstaller = $installerPath
  $products = @(Get-WebMuxProducts)
  if ($products.Count -ne 1) { throw 'Native installation left duplicate or missing product registrations.' }
  if ($previousProduct -and $products[0] -eq $previousProduct) { throw 'Native upgrade retained the old product registration.' }
  if (-not (Test-Path -LiteralPath (Join-Path $installRoot 'bin\webmux.exe'))) { throw 'MSI omitted the native executable.' }
  if (Test-Path -LiteralPath (Join-Path $installRoot 'node_modules')) { throw 'Native MSI includes Node runtime dependencies.' }
  if (Test-Path -LiteralPath (Join-Path $installRoot 'backend')) { throw 'Native upgrade left legacy backend files.' }
  if (Test-Path -LiteralPath (Join-Path $installRoot 'bin/webmux.js')) { throw 'Native upgrade left the legacy launcher.' }
  & node (Join-Path $PSScriptRoot 'smoke-native-package.mts') $installRoot
  if ($LASTEXITCODE -ne 0) { throw 'Installed native runtime failed smoke checks.' }
  & (Join-Path $PSScriptRoot 'smoke-native-service.ps1') -BundleDirectory $installRoot
} catch {
  Write-Error $_ -ErrorAction Continue
  throw
} finally {
  # Never remove an installation still referenced by a service after a failed
  # service cleanup. Retain its files and logs for diagnosis on the runner.
  if (Get-Service -Name WebMux -ErrorAction SilentlyContinue) {
    throw 'WebMux service remains registered; retaining the installation for diagnosis.'
  }
  if (Test-Path -LiteralPath $installRoot) { Invoke-Installer '/x' (Join-Path $temporary 'uninstall.log') $activeInstaller }
  if ($previousProduct -and (@(Get-WebMuxProducts) -contains $previousProduct)) {
    Invoke-Installer '/x' (Join-Path $temporary 'previous-uninstall.log') $previousPath
  }
}
if (@(Get-WebMuxProducts).Count -ne 0) { throw 'MSI uninstall left a WebMux product registration.' }
if (Test-Path -LiteralPath (Join-Path $installRoot 'bin\webmux.exe')) { throw 'MSI uninstall left the native executable behind.' }
Remove-Item -LiteralPath $temporary -Recurse -Force
Write-Host 'Native MSI passed: installation/upgrade, native runtime, service lifecycle and uninstall.'
